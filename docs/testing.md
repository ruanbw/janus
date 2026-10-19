# Janus 测试流程

当前只保留**纯逻辑单元测试**（不依赖 Postgres、不建库、不起服务）。

已移除的层：DB 黑盒测试（`internal/testutil`）、`scripts/e2e.sh`、`scripts/quality.sh`
（全量隔离库 / CRAP / 变异测试）。仓库不再有全量或端到端测试代码。

---

## 运行方式

只跑被改动包的定向用例，单次控制在 30 秒以内：

```bash
go test -count=1 ./internal/rules/ 
go test -count=1 -run '^TestIsValidSlug$' ./internal/domain/
cd web && npx vue-tsc --noEmit
```

不要运行 `go test ./...`。

## 保留的测试范围

| 包 | 覆盖 |
| --- | --- |
| `internal/rules` | 条件求值、字段提取、快照缓存、表达式沙箱、Radix 路由 |
| `internal/domain` | FQDN/slug/短码校验、DNS 校验、worker 调度等纯逻辑 |
| `internal/httpapi` | 纯函数/中间件单测（不连库） |
| `internal/geo` | 离线 ip2region 查询 |
| `internal/jwt` `internal/rbac` `internal/config` `internal/fqdn` `internal/db` | 各自纯逻辑单测 |
| `pkg/janus` `pkg/event` `pkg/plugin` | SDK / 事件 / 插件契约 |

`cmd/janus` 目前没有测试;`internal/bootstrap`(超管幂等收敛)与 `internal/store`(首登态判定)已有纯逻辑单测。

---

## 浏览器 UI 手动实操

仅在用户显式要求做真实浏览器联调或涉及关键交互重构时执行。

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

### 这一层已经踩到的坑

1. **CSS 过渡在后台标签页里永不结束** — `AdminLayout.vue` 用
   `<transition mode="out-in">`,浏览器冻结隐藏标签页的动画帧,leave 卡在
   `page-leave-active`,新页面永远不挂载。回归验证时要专门测隐藏状态:
   把标签页切到后台 → 点侧边栏导航 → 看 `main` 的子节点数与标题是否同步更新。
2. **点按钮要用真实指针序列**。`browser-hand click` 走完整指针事件;
   直接 `element.click()` 对 reka-ui 组件不可靠。行内按钮先打标记再用 `--selector` 精确点。
3. **确认框的按钮标签要现取**,不同操作的 `okText` 各不相同。
4. 用 `window.__log` 挂 XHR/fetch 钩子,区分「前端没触发」和「后端报错」。
