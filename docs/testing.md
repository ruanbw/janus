# Janus 测试流程

三层防线:单元/黑盒测试 → 端到端 API 脚本 → 浏览器 UI 实操。

---

## 第 1 层:Go 测试(黑盒,真实 Postgres)

`internal/testutil` 提供黑盒设施:启动 `httptest.Server` + 连真实 `janus_test` 库 + 跑真实迁移,
测试 seam 是 HTTP API 边界,不 mock 内部函数。

```bash
# 需要先起 Postgres 并建测试库
docker compose up -d postgres
docker exec -i janus-postgres-1 psql -U janus -d postgres \
  -c 'CREATE DATABASE janus_test'

go test ./... -count=1
```

各包通过 `db.LockTestDB` 独占测试库(各包都 TRUNCATE 同一张表,并行跑会互相清数据)。

| 包 | 覆盖 |
| --- | --- |
| `internal/httpapi` | 认证/域名/短链/规则/落地页/统计/超管/CSRF/限流/授权 |
| `internal/rules` | 条件求值、字段提取、快照缓存、表达式沙箱、Radix 路由 |
| `internal/store` | 规则存储、配额、meta |
| `internal/domain` | FQDN/slug/短码校验、DNS 校验、后台 worker |
| `internal/geo` | 离线 ip2region 查询 |
| `internal/jwt` `internal/rbac` `internal/config` `internal/mailer` `internal/db` | 各自单测 |

默认注入高阈值限流;测限流本身用 `SetupWithRateLimit`。

---

## 第 2 层:端到端 API 脚本 `scripts/e2e.sh`

自包含:自建数据库 → 编译二进制 → 起独立实例 → 跑完即清理。**不碰开发数据。**

```bash
./scripts/e2e.sh            # 全量(257 断言)
./scripts/e2e.sh --keep     # 保留数据库与日志,便于排查
./scripts/e2e.sh --base URL # 测已运行的服务(需自备数据)
```

隔离手段:独立库 `janus_e2e`、独立端口 `:18080`、独立平台域名 `e2e.janus.test`、
落地页上传目录在临时目录。邮箱验证与超管 setup token 从控制台 mailer 日志里捞。

覆盖范围:

| 分区 | 内容 |
| --- | --- |
| 0 | 健康检查、未认证访问全部受保护端点 |
| 1 | 注册/邮箱验证/登录/登出/改密/忘记密码/超管首登 setup token 一次性 |
| 2 | `/api/config`、`/api/me` 配额 |
| 3 | 域名 CRUD、TXT 指引、停用恢复、平台域名免删 |
| 4 | 短链 CRUD、目标去重、批量删/彻底删、回收站还原、短码不可改 |
| 5 | 跳转、301/302、多目标轮询、停用/删除的失败明细 |
| 6 | 规则 CRUD、四种动作裁决、作用域、零关联不命中、仿真 |
| 7 | 总览聚合 |
| 8 | 落地页 url/upload 两种来源、SDK、点击回传、zip 校验 |
| 9 | 自定义 404/429 页 |
| 10 | 配额上限(短链/域名)、软删仍占配额 |
| 11 | 租户隔离(读/改/删/明细) |
| 12 | JWT 签发/吊销/Bearer 免 CSRF |
| 13 | Caddy on-demand TLS 授权端点 |
| 14 | CSRF 防护 |
| 15 | 平台管理、超管封禁/解封/防自锁 |

跑测试时的注意事项:

- **短码字母表**不含 `0/O/1/l/I`(产品刻意剔除易混淆字符),脚本生成器必须避开,
  否则 `IsValidCode` 拒 400,后续用例连环失败。
- **slug 只允许小写字母/数字/连字符**,生成器不能用大写。
- **改密会吊销在手 JWT**(同事务自增 `token_version`),签发与使用之间不要插入改密。
- **软删的短链仍占配额**(业务口径按「尚未物理删除」计数),断言要按这个口径写。
- **规则快照 TTL 60 秒**:直接改库停用规则不会触发 `Cache.Invalidate`,
  需等满 TTL;走 API 的规则 CRUD 会主动失效缓存。手工改库调试时容易误判。

---

## 第 3 层:浏览器 UI 实操

脚本覆盖不到交互层(路由过渡、确认框、遮罩、组件透传),这一层必须手动跑或脚本驱动浏览器。

覆盖:每个页面的渲染、增删改查、表单校验文案、空状态/错误态。

用 `browser-hand` 驱动真实 Chrome:

```bash
BH="node ~/browser-hand/cli-js/src/cli.js"
$BH doctor                                   # 先确认桥接健康
$BH open  --url http://localhost:5173/links --page-name janus
$BH snapshot --page-name janus
$BH fill   --page-name janus --fields '{"短码":"abc123"}'
$BH click  --page-name janus --text "保存修改"
$BH evaluate --page-name janus --code "document.body.innerText.slice(0,200)"
```

### 这一层已经踩到的坑(测试时必须知道)

1. **CSS 过渡在后台标签页里永不结束** — `AdminLayout.vue` 用
   `<transition mode="out-in">`,浏览器冻结隐藏标签页的动画帧,leave 卡在
   `page-leave-active`,新页面永远不挂载(标题变了、内容还是旧的)。
   已修(接管 `@leave`,隐藏时立即收尾),但**回归验证时仍要专门测隐藏状态**:
   把标签页切到后台 → 点侧边栏导航 → 看 `main` 的子节点数与标题是否同步更新。
   只测前台会漏掉这个问题 —— 这正是它当初能进主干的原因。
2. **点按钮要用真实指针序列**。`browser-hand click` 走完整指针事件;
   直接 `element.click()` 对 reka-ui 组件不可靠(它依赖 pointerdown/mouseup 序列)。
   行内按钮要先用 `title` 或唯一属性打标记,再用 `--selector` 精确点,
   否则会命中同名的其它按钮(如「删除」vs「彻底删除」)。
3. **确认框的按钮标签要现取**,不要照抄文档:不同操作的 `okText` 各不相同
   (「确认逻辑删除」/「彻底物理删除」/「确认还原」/「删除」)。
4. 用 `window.__log` 挂 XHR/fetch 钩子,才能判断「点了有没有发请求」——
   这类「UI 没反应」的问题必须区分开是前端没触发还是后端报错。

---

## 回归检查清单

改动后至少跑:

```bash
go build ./... && go test ./... -count=1   # 第 1 层
./scripts/e2e.sh                          # 第 2 层
cd web && npx vue-tsc --noEmit -p tsconfig.json   # 前端类型
```

改动前端交互时,额外按第 3 层手测受影响的页面。
