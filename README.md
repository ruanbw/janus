<div align="center">

# Janus

**每一次跳转，皆为裁决。**  
*Every jump is a verdict.*

[![license](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)
[![node](https://img.shields.io/badge/Node-%E2%89%A520.19-339933?logo=nodedotjs&logoColor=white)](web/package.json)
[![pnpm](https://img.shields.io/badge/pnpm-10-F69220?logo=pnpm&logoColor=white)](web/package.json)
[![postgres](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)](docker-compose.yml)
[![caddy](https://img.shields.io/badge/Caddy-2-2B6CB?logo=caddy&logoColor=white)](Caddyfile.prod)

[中文](README.md) · [English](README.en.md)

</div>

---

Janus 是一个自托管的**多租户短链服务**:租户注册后管理自己的域名与短链,系统为每个已激活域名**自动签发并续期 HTTPS 证书**,把 `域名/短码` 的访问重定向到目标 URL。除跳转外,还提供**落地页型短链**、**访问明细与地理统计**、**租户级访问处置规则引擎**与**自定义错误页**,以及给脚本用的 **Bearer JWT API**。

- **单服务器部署** — Go 后端(Gin + GORM)+ Vue 3 SPA(nginx)+ PostgreSQL 16 + Caddy(on-demand TLS,Let's Encrypt 自动签发/续期),`docker compose up -d` 全部跑起来。
- **多租户隔离** — 租户之间的域名、短链、规则、落地页完全隔离;部署者持有平台管理员角色,可治理全平台。
- **前后端分离** — 前端独立构建为 nginx 镜像,后端镜像内无任何前端文件;改前端不必重打后端(ADR-0006)。
- **开箱即用** — 邮箱注册即用,平台自动为每个租户发放 `<slug>.<平台域名>` 测试域名,无需任何证书与 DNS 手工操作。


---

## 目录

- [1. 项目简介](#1-项目简介)
- [2. 功能列表](#2-功能列表)
- [3. 技术架构](#3-技术架构)
- [4. 仓库结构](#4-仓库结构)
- [5. 环境要求](#5-环境要求)
- [6. 快速开始(开发环境)](#6-快速开始开发环境)
- [7. 开发规范](#7-开发规范)
- [8. 测试](#8-测试)
- [9. 开发 / 生产差异](#9-开发--生产差异)
- [10. 生产部署(速览)](#10-生产部署速览)
- [11. 故障排查](#11-故障排查)
- [12. API 概览](#12-api-概览)
- [13. 参与贡献](#13-参与贡献)
- [14. 许可证](#14-许可证)
- [15. 相关文档](#15-相关文档)

---

## 1. 项目简介

### 1.1 它解决什么问题

市面上的短链服务要么按点击收费,要么不给你自己的域名。Janus 的取舍很直接:**部署在自己服务器上,租户带自己的域名进来**。

| 需求 | Janus 的做法 |
| --- | --- |
| 短链要用自己的域名 | 自有域名加进来做 DNS 激活校验,证书由 Caddy on-demand TLS 自动签发与续期 |
| 不想为每个租户手工配证书 | 平台域名配一条 `*.<平台域名>` 泛解析,每个租户自动获得 `<slug>.<平台域名>` |
| 部署不能依赖外部服务 | 唯一外部依赖是 Let's Encrypt(签发/续期)与可选的 SMTP;数据库与证书都在本机 |
| 要能对接脚本 / 第三方系统 | 会话 cookie 之外,另发 Bearer JWT(24h)给脚本用,免 CSRF |
| 跳转流量要有明细可查 | 每次访问落一行:动作(跳转/落地页/点击)、成功失败与原因、IP、设备、国家、来源、时间 |
| 需要按访客画像处置访问 | 租户级规则引擎:13 个访客字段 + 条件组合,裁决为放行/改写目标/404/限流 |

### 1.2 一次访问的处理链路

`GET https://<域名>/<短码>` 的完整处理顺序(实现见 [`internal/httpapi/redirect.go`](internal/httpapi/redirect.go)):

```
请求到达
  │
  ├─ 1. Host → 定位 active 域名        ── 未命中 → 404(不记明细)
  │
  ├─ 2. 短码 → 定位短链 + 目标池       ── 未命中 → 404(不记明细)
  │
  ├─ 3. 短链可用性检查                ── 停用/已逻辑删除/无目标/落地页文件缺失
  │      (规则不参与)                       → 404 + 记一行 outcome=failed 明细
  │
  ├─ 4. 规则求值(短链 rules_enabled 时)  规则快照按租户整份缓存在内存,零 DB 查询
  │      priority 升序,首条命中即裁决      → pass 继续 / redirect 改写目标
  │                                            / notfound 404 / throttle 429
  │      一条都没命中 → 走原跳转流程
  │
  ├─ 5. 落地页型 → 302 到外部落地页,或 302 到 /<短码>/(平台托管的上传页)
  │      跳转型   → 从目标池轮询选一个 → 302(默认)或 301(永久)
  │
  └─ 6. 落一行访问明细(action / outcome / IP / UA / 来源 / 语言 / 国家 / 命中规则)
```

三条热路径上的硬约束(改动 `redirect.go` 时必须守住):

1. **零 DB 查询** — 规则集合来自按租户缓存的内存快照;租户没有规则时连访客画像都不构造。
2. **零写放大** — 规则命中不 UPDATE 任何计数表,只多写本次访问那一行明细("24h 命中"由列表接口读时聚合)。
3. **fail-open** — 快照加载失败或求值 panic 一律按"未命中"继续;风控规则不该把线上短链打成 500。

---

## 2. 功能列表

### 2.1 租户与认证

- **邮箱注册 + 邮箱验证**(防垃圾注册):注册后状态 `pending`,验证通过转 `active`,并自动激活其平台默认域名。
- **登录 / 登出 / 记住我**:记住我 30 天,普通会话 24 小时;超管首次登录引导设置密码。
- **修改密码 / 忘记密码 / 重置密码**:重置 token 1 小时有效。
- **平台管理员(超管)**:由 `JANUS_SUPERADMIN_EMAIL` 初始化,启动时幂等创建。
- **两种认证方式**:后台会话 cookie(HTTP-only + CSRF 双提交 token);`POST /api/auth/token` 换 Bearer JWT 供脚本/CLI 使用(header 认证,免疫 CSRF)。
- **等级(Tier)与配额**:等级决定短链数与自有域名数上限;平台默认域名不计入配额;超限返回明确错误(含当前用量 / 上限)。

### 2.2 域名与证书

- **平台默认域名**:每个租户自动获得 `<slug>.<平台域名>`,邮箱验证后自动激活,不计配额,可停用、不可删除。
- **自有域名**:租户自行添加,需通过"DNS 指向本服务器"的激活校验;激活后由 Caddy 自动签发 Let's Encrypt 证书并自动续期。
- **激活校验重试**:每 5 分钟重试一次,最长 72 小时;未通过置 `failed`;后台可手动「重新校验」。
- **生命周期**:停用 / 恢复 / 删除(物理删除,删除前须先清空其上的短链关联)。
- **证书状态可查**:后台可见签发状态。

### 2.3 短链

- **自定义短码**或自动生成(6 位);同一短码在不同域名下可指向不同目标,同一租户的多条短链可共用短码。
- **多目标**:一条短链可配置多个目标 URL,访问时按**轮询(round-robin)**选择其一。
- **跳转方式**:临时 302(默认)或永久 301,每条短链独立配置。
- **两种类型**:**跳转型**(直接重定向到目标 URL)与**落地页型**(先到落地页,点击后到目标 URL)。类型在创建时选定,**之后仍可修改**(`PATCH /api/links/:id` 传 `linkType`,前端短链编辑页可改)。修改后按新类型立即生效。
- **启停 / 删除**:默认逻辑删除(记录、关联与访问信息保留),另提供彻底删除(purge);前端支持批量逻辑删除与批量物理删除。
- **规则总开关**:每条短链可单独关闭规则裁决,关闭后完全跳过规则求值。

### 2.4 落地页型短链

落地页来源二选一、可切换(ADR-0005):

- **`url`** — 填写外部落地页地址,访问时 302 过去。
- **`upload`** — 上传 zip(必须含 `index.html`),平台托管在 `短码/` 路径下;限制解压后总大小(默认 10MB)与文件数(默认 500),并有扩展名白名单。

配套能力:

- **每条短链一份 JS SDK**(`GET /<短码>/sdk.js`,内嵌该短链的绝对点击地址):落地页引入后,按钮点击自动绑定。
- **点击回传端点** `GET /<短码>/click`:点击计数 +1,并落一行 `action=click` 的访问明细(带 IP、设备与来源),点击**不**计入访问次数。
- 点击行为最终把访问者送到目标 URL。

### 2.5 访问明细与统计

- **访问明细**(ADR-0007):每次访问记录**动作**(跳转 / 落地页视图 / 点击)、**结果**(成功 / 失败 + 失败原因:短链已停用 / 已逻辑删除 / 没有可用目标 / 落地页文件缺失)、访问者 IP、User-Agent、来源页、语言、国家与时间。
- **计数口径**:只有**成功**的跳转与落地页视图计入访问次数;点击与失败都不计入。
- **可归属的失败不丢**:能归属到具体短链的失败会留痕;短码压根不存在的未命中访问不记。
- **总览页**:KPI 卡片 + 热门短链排行 / 设备 / 系统 / 浏览器 / 来源 / 国家分布 + **世界地图** choropleth(随包国界 + `d3-geo` 投影,懒加载,ADR-0010)。
- **总览统计走专用聚合端点** `GET /api/visits/overview`:一次 SQL 出全租户计数与各维度分布(全量 `GROUP BY`,不抽样),前端不再「拉明细自己数」——否则口径必然与 KPI 分叉,且会把最近 50 行当全量、只覆盖短链列表前 100 条。UA 维度只回**原始 UA 串 + 次数**,设备/系统/浏览器标签由前端 `ua-parser-js` 翻译(Go 侧不再实现第二套 UA 解析)。
- **CTR 口径**:分子分母同源同期,都取自 `visits` 表同一段 SQL、同一保留期窗口;分母只取**落地页访问**(跳转型短链不产生点击,算进去会系统性压低 CTR)。不再用 `links.clicks` 那个永久计数器当分子(它永不衰减而分母会被 90 天清理削掉,CTR 会单调虚高到 100% 以上)。
- **地理归属**:内置 ip2region 离线库(V4 + V6)解析访问者国家码,16 分片双代缓存(负结果也缓存,ADR-0009)。
- **保留策略**:访问记录默认保留 90 天,后台任务定时清理(`JANUS_VISIT_RETENTION` 可调)。
- **单链明细页**:按短链查看访问明细,含动作、结果、命中规则等字段。

### 2.6 规则引擎

租户级的**访问处置规则**:一个条件集合 + 一个动作(ADR-0008)。

- **13 个可求值访客字段**:`ip`(支持 CIDR)、`ipattr`(private / loopback / linklocal)、`country`、`asn`、`lang`、`ref`、`utm`、`ua`、`devtype`(bot / mobile / tablet / desktop)、`os`、`browser`、`path`、`domain`。
- **操作符**:属于 / 不属于 / 等于 / 不等于 / 包含 / 不包含 / 开头是(`starts_with`)/ 结尾是(`ends_with`)/ 大于 / 小于 / 正则(RE2,大小写敏感)/ 落在 IP 网段(`in_cidr`,仅 `ip` 字段);`ip` 字段按 CIDR 网段或字面量匹配,数值比较遇到非数值恒不命中。v1 不做嵌套分组,条件之间由 `logic` 决定「全部满足」或「任一满足」。
- **取不到数据恒不命中**(两个编辑方式一致):字段值为空时条件不成立,**取反写法也不例外**——`country != "US"` 在 `country` 为空时不命中,否则「拦截非美国访客」会变成拦截所有人。`asn` 当前无数据源(`fields.go` 标为恒空),凡引用它的条件恒不成立。零条件规则是**有意的兜底规则**(无条件即命中)。
- **两种编辑方式**:`visual`(可视化条件树,默认)与 `expression`(Expr 表达式,给高级用户,`POST /api/rules/validate-expr` 校验语法)。
- **动作(裁决)**:放行 `pass`(记录命中后继续原跳转流程)/ 改写目标 `redirect`(改写地址不参与短链目标池轮询)/ 返回 404 `notfound`(记 `outcome=failed`、`reason=rule_blocked`)/ 限流 `throttle`(返回 429,记 `reason=rule_throttled`);后两者不计入访问量。
- **作用域**:**全局**对租户所有短链生效;**指定短链**只对被显式关联的短链生效——关联由规则侧声明,规则是这份关联的唯一写入口(短链侧看到的是同一份数据)。指定短链但零关联的规则永远不命中,界面显式标出该状态。
- **裁决语义**:按 `priority` 升序求值,**首条命中即裁决**,不做多规则叠加;一条都没命中则按短链原有的目标选择流程跳转。裁决排在短链可用性之后。
- **规则仿真**:`POST /api/rules/simulate` 用同源旁路回放求值,逐条回答"这个访客会命中哪条规则、凭什么",条件明细文案由后端出,避免前端复刻判定漂移。
- **上限**:单租户 200 条规则;规则集合按租户整份缓存在内存(默认 TTL 1 分钟),**任何规则写入或关联变更都必须失效对应租户的快照**。

### 2.7 自定义错误页

- **租户全局**:可自定义 404 与 429 错误页 HTML。
- **规则专属**:单条规则可指定自己的错误页与模式(`default` / `custom`)。
- 决议优先级:规则专属 → 租户全局 → 系统内置默认页(自适应深色模式的静态页)。
- 适用场景:未命中、短链停用/删除/无目标/落地页文件缺失、规则 `notfound` 裁决(404)、规则 `throttle` 裁决(429)。
- **按租户内存缓存**:租户错误页与规则快照同构(惰性加载 + 原子替换 + TTL 兜底 + 改配置后显式失效),未命中这条最易触发的路径不再每次实时查两个 512KB 的 TEXT 字段(见 README 1.2 硬约束第 1 条)。
- **响应头**:错误页带 `Content-Security-Policy: sandbox allow-scripts allow-forms`(脚本可跑但处于不透明来源,读不到本站会话)与 `X-Content-Type-Options: nosniff`;404/429 均 `Cache-Control: no-store`,429 另带 `Retry-After: 60`。管理端预览 iframe 的 sandbox 与线上策略一致。

### 2.8 平台管理(超管)

- 查看全部租户、租户详情、等级列表;
- 封禁 / 解封账号,调整租户等级;
- 移除违规域名。

### 2.9 API 与安全

- **Casbin RBAC**:两档角色(`tenant` / `superadmin`),按 (角色, HTTP 方法, 路径) 三元组判权,策略在内存装载不落盘;`superadmin` 一条 `*` 覆盖 `/api/*`。
- **限流**:注册端点每 IP 每分钟 10 次;登录 / 验证 / 找回 / 重置每 IP 每分钟 20 次(`golang.org/x/time/rate` 令牌桶)。
- **密码**:bcrypt;账号不存在时也做一次假比较,抹平时序差异。
- **防 header 注入**:目标 URL 任意协议(开放重定向)但拒绝控制字符(CRLF)。
- **授权端点**:`/internal/caddy/authorize` 仅内网可达,未激活域名拒绝签发证书——防止任意域名解析到本机就触发签发。
- **统一错误响应**:`{ code, message, details }`,错误码稳定、消息面向人。

### 2.10 平台化运维

- **迁移**:启动时自动执行 `migrations/*.sql`(goose,幂等)。
- **后台 worker**:DNS 重试、证书探活、访问记录清理、会话与邮箱 token 清理。
- **可插拔邮件**:配置 SMTP 用真实发送;未配置时用控制台 mailer(邮件内容打印到后端日志),便于无外网环境开发。
- **可插拔 geo 数据源**:按能力抽象(ADR-0009),当前落地的是内嵌 ip2region 离线库,`asn` / `is_datacenter` 无数据源时恒为空(规则里相应条件恒不命中,而不是"当成匹配")。

---

## 3. 技术架构

### 3.1 请求拓扑

```
                            Internet
                               │
                  ┌────────────┴─────────────┐
                  │  Caddy (80/443)          │
                  │  on-demand TLS           │   生产:Let's Encrypt
                  │  ask → 授权端点          │   开发:本地 CA
                  └───────┬───────────┬──────┘
                          │           │
              平台裸域名(后台)      *.<平台域名> 租户子域 + 自有域名
                          │           │
                  ┌───────┴───────────┴──────┐
                  │  Caddy: TLS + 路径分流    │
                  └───────┬───────────┬──────┘
                          │           │
              ┌───────────▼──┐   ┌────▼─────────────────┐
              │ nginx (web)  │   │  Go 后端 (backend)   │
              │ /            │   │  /api/*   REST API   │
              │ SPA + 静态资源│   │  /{code}  跳转路由    │
              │              │   │  /{code}/… 落地页     │
              │              │   │  /internal/caddy/…   │
              │              │   │  后台 worker:        │
              │              │   │   DNS 重试/证书探活/  │
              │              │   │   记录与 token 清理    │
              └──────────────┘   └────┬─────────────────┘
                                        │
                       ┌────────────────┴───────────────┐
                       │  PostgreSQL 16                 │
                       │  + JANUS_LANDING_UPLOAD_DIR 持久卷│
                       └────────────────────────────────┘
```

### 3.2 组件职责

| 组件 | 职责 | 关键点 |
| --- | --- | --- |
| **Caddy** | TLS 终止与证书生命周期 | on-demand TLS:陌生域名首次握手时问应用 `/internal/caddy/authorize` 该域名是否已激活,是则签发 Let's Encrypt 证书并自动续期(ADR-0002);应用不直接操作 Caddy 配置 |
| **Go 后端** | 业务逻辑 + REST API + 跳转热路径 | 启动顺序:加载配置 → 连接 Postgres → 执行迁移 → 初始化超管 → 启动后台 worker → 监听 HTTP |
| **nginx(web 镜像)** | SPA 静态资源 + history 路由回退 | 独立于后端镜像,构建期自行 `pnpm build` |
| **PostgreSQL** | 全部持久状态 | 唯一数据源;迁移文件随镜像发布 |
| **前端 SPA** | 后台管理界面 | Vue 3 + TypeScript + Vite + Tailwind CSS 4 + Reka UI,自研 `App*` 组件库 |

### 3.3 技术栈

**后端**(Go 1.26)

| 关注点 | 选型 |
| --- | --- |
| HTTP 框架 | `gin-gonic/gin` |
| 授权 | `casbin/casbin` + `gin-contrib/authz`(策略内存装载) |
| 数据访问 | `gorm.io/gorm` + `gorm.io/driver/postgres`(`pgx/v5` 驱动) |
| 迁移 | `pressly/goose/v3` |
| 规则表达式 | `expr-lang/expr` |
| JWT | `golang-jwt/jwt/v5` |
| 限流 | `golang.org/x/time/rate` |
| 密码 | `golang.org/x/crypto/bcrypt` |
| 邮件 | `wneessen/go-mail`(SMTP) |
| 配置 | `caarlos0/env/v11` |
| CIDR 前缀树 | `yl2chen/cidranger` |
| GeoIP | `ip2region`(离线 xdb,V4 + V6,随二进制发布) |
| 测试 | 标准库 `testing` + `stretchr/testify`;黑盒 HTTP 测试跑在 `httptest.Server` + 真实 Postgres 上 |

**前端**(Vue 3.5 + TypeScript)

| 关注点 | 选型 |
| --- | --- |
| 构建 | Vite 6 |
| 样式 | Tailwind CSS 4(设计令牌 + 深色模式) |
| 无头交互组件 | Reka UI(替代已移除的 ant-design-vue) |
| 图标 | `@lucide/vue` |
| 状态 / 路由 | Pinia / Vue Router 4 |
| 请求 | Axios(统一封装 + CSRF 头 + `ApiError`) |
| 校验 / 工具 | `zod` / `@vueuse/core` / `ipaddr.js` |
| 图表与地图 | `d3-geo` + `topojson-client` + `world-atlas` + `i18n-iso-countries`(总览页懒加载) |

---

## 4. 仓库结构

```
.
├── cmd/janus/                  # 后端入口(加载配置 → 连库 → 迁移 → worker → HTTP)
├── internal/
│   ├── bootstrap/              # 部署期初始化(超管幂等创建)
│   ├── config/                 # 环境变量配置(caarlos0/env)
│   ├── db/                     # Postgres 连接 + goose 迁移
│   ├── domain/                 # 域名激活校验、证书探活、后台 worker
│   ├── geo/                    # IP 地理值(ip2region 离线库 + 分片缓存)
│   ├── httpapi/                # HTTP 路由与处理器(黑盒测试所在)
│   │   └── templates/          # 内置 404 / 429 自适应错误页
│   ├── jwt/                    # JWT 签发与校验
│   ├── mailer/                 # 可插拔邮件(SMTP / 控制台)
│   ├── rbac/                   # Casbin 策略文本与授权中间件
│   ├── rules/                  # 规则引擎:字段、条件求值、租户快照与仿真
│   ├── store/                  # 数据访问层(租户 / 域名 / 短链 / 规则 / 访问)
│   └── testutil/               # 测试基础设施(httptest + 真实 Postgres)
├── migrations/                 # 数据库初始化 SQL (启动时自动执行)
├── web/                        # 前端 SPA(见 web/README.md、web/UI_KIT.md)
│   ├── src/
│   │   ├── api/  types/  utils/ # 请求封装、类型、错误与工具
│   │   ├── components/          # app/(项目组件 App*)+ ui/(shadcn 原语,kebab-case)
│   │   ├── layouts/  views/     # 后台骨架与各功能页
│   │   └── styles/              # Tailwind 入口与三层设计令牌
│   ├── scripts/check-ui-consistency.mjs  # UI 规范门禁
│   └── dist/                   # 构建产物(不入库,镜像构建期生成)
├── docker/
│   ├── Dockerfile              # 多阶段:backend(Go)与 web(nginx)两个独立目标
│   └── nginx.conf              # SPA 回退 + 静态资源强缓存
├── docker-compose.yml          # 开发编排(Postgres + Caddy)
├── docker-compose.prod.yml     # 生产编排(Postgres + backend + web + caddy)
├── Caddyfile / Caddyfile.prod  # 开发(本地 CA)/ 生产(Let's Encrypt)
├── .env.example                # 环境变量模板
├── LICENSE                     # AGPL-3.0
└── docs/
    ├── deploy.md               # 生产部署指南
    ├── testing.md              # 测试流程与防线说明
    └── adr/                    # 架构决策记录
```

---

## 5. 环境要求

| 工具 | 版本 | 用途 |
| --- | --- | --- |
| Docker Desktop(含 Compose v2) | 较新版本 | 基础设施:Postgres / Caddy |
| Go | ≥ 1.26(`go.mod`) | 终端启动后端与运行后端测试 |
| Node.js | ≥ 20.19(`web/package.json` engines) | 前端开发与构建 |
| pnpm | ≥ 9(仓库锁定 10.x) | 前端依赖管理 |
| Caddy CLI(可选) | ≥ 2.x | 开发环境信任本地 CA(`caddy trust`) |

> 开发环境**基础设施进 Docker**(Postgres、Caddy),**后端与前端在终端跑**(`go run` + `pnpm dev`),改代码即时生效;生产则全部容器化。

---

## 6. 快速开始(开发环境)

### 6.1 启动基础设施

```bash
docker compose up -d
```

| 服务 | 容器内端口 | 宿主机映射 | 说明 |
| --- | --- | --- | --- |
| `postgres` | 5432 | `127.0.0.1:5432` | PostgreSQL 16,开发凭据 `janus/janus` |
| `caddy` | 443 | `127.0.0.1:443`、`127.0.0.1:80` | HTTPS 入口,on-demand TLS + 本地 CA,反代宿主机 `:8080`(`host.docker.internal`) |

> Caddy 占用宿主 80/443;若本机另有服务(如 openresty)也绑定这两个端口,两者错开启动。
> 无端口访问依赖 hosts 把 `*.janus.test` 指向 `127.0.0.1`(见 6.2)。

### 6.2 本地域名解析

开发支持 **localhost 直连**与**域名访问**两种入口。域名访问需要把平台域名与测试租户子域指向本机,用 SwitchHosts 或 `/etc/hosts` 均可:

```text
127.0.0.1 app.janus.test
127.0.0.1 alice.janus.test
127.0.0.1 bob.janus.test
```

```bash
# 手动追加(每注册一个新租户就加一行;hosts 不支持通配符)
sudo sh -c 'echo "127.0.0.1 alice.janus.test" >> /etc/hosts'
```

- `app.janus.test` 承载后台;`<slug>.janus.test` 是租户的平台默认域名,用于验证短链跳转。
- 测试自有域名时同样加一行(如 `127.0.0.1 links.example.test`)。`JANUS_SERVER_PUBLIC_IP=127.0.0.1` 时,Go 的 DNS 校验会读 hosts,走**真实代码路径**。
- 不想改 hosts 时可用 `curl --resolve` 临时解析(见 8.3)。
- Vite Dev Server 默认拒绝非 localhost 的 Host(防 DNS rebinding);项目已在 `vite.config.ts` 用 `server.allowedHosts` 放行 `.janus.test`(由 `web/.env.development` 的 `VITE_PLATFORM_DOMAIN` 控制),换平台域名时同步修改。

### 6.3 信任 Caddy 本地 CA

开发证书由 Caddy 本地 CA 签发,浏览器默认不信任:

```bash
mkdir -p certs
docker compose exec caddy cat /data/caddy/pki/authorities/local/root.crt > certs/caddy-root.pem
sudo caddy trust --ca certs/caddy-root.pem
# macOS 备选:
# sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/caddy-root.pem
```

> `certs/` 已在 `.gitignore` 中;重建 Caddy 数据卷后需重新信任。

### 6.4 启动后端

```bash
JANUS_COOKIE_SECURE=false JANUS_ADDR=:8080 go run ./cmd/janus
```

- `JANUS_COOKIE_SECURE=false`:开发走 http,Secure cookie 不生效;
- `JANUS_ADDR=:8080`:与 Vite 代理、Caddy 反代、生产 compose 保持一致;改端口时需同步 Vite 的代理目标与 Caddy 反代地址;
- 其余配置用代码默认值即可(Postgres `postgres://janus:janus@localhost:5432/janus`、平台域名 `janus.test`、公网 IP `127.0.0.1`、控制台 mailer);
- 覆盖配置用环境变量,例如 `JANUS_SUPERADMIN_EMAIL=admin@example.com go run ./cmd/janus`;
- `go run` **不会**自动读 `.env`(那是 compose 的行为),需要时在命令行 export;完整变量见 [.env.example](.env.example) 与 `internal/config/config.go`。
- 未配置 SMTP 时,验证 / 重置邮件直接打印在这个终端(见 8.3 第 2 步)。

验证:

```bash
curl -s http://127.0.0.1:8080/healthz          # → {"status":"ok"}
curl -sk https://app.janus.test/healthz         # 经 Caddy 走通全链路
```

改后端代码后 `Ctrl+C` 再 `go run` 即可;前端改动由 Dev Server 热更新。

### 6.5 启动前端

```bash
cd web
pnpm install     # 首次
pnpm dev         # http://localhost:5173,/api 代理到 http://localhost:8080
```

浏览器打开 `http://localhost:5173`(或 `http://app.janus.test:5173`)。开发环境 `JANUS_COOKIE_SECURE=false`,http 下 cookie 正常生效。

### 6.6 访问入口

| 入口 | 地址 | 说明 |
| --- | --- | --- |
| 后台(localhost) | `http://localhost:5173` | Vite Dev Server,热更新 |
| 后台(域名) | `http://app.janus.test:5173` | 同上,经 hosts 域名访问 |
| 后台(域名 + HTTPS) | `https://app.janus.test` | Caddy → Vite,验证域名 / TLS / 证书形态 |
| 健康检查 | `https://app.janus.test/healthz` | `{"status":"ok"}` |
| 后端直连 | `http://127.0.0.1:8080` | 绕过 Caddy / Vite,调试用 |

### 6.7 环境变量

开发默认值开箱即用;需要覆盖时 `cp .env.example .env` 后修改(compose 会读 `.env`,`go run` 不会)。

| 变量 | 开发默认值 | 说明 |
| --- | --- | --- |
| `JANUS_ADDR` | `:8080` | HTTP 监听地址 |
| `JANUS_PLATFORM_DOMAIN` | `janus.test` | 平台裸域名;租户默认域名 `<slug>.<平台域名>` |
| `JANUS_SERVER_PUBLIC_IP` | `127.0.0.1` | DNS 激活校验比对的地址 |
| `JANUS_PUBLIC_BASE_URL` | `https://app.janus.test` | 邮件里验证 / 重置链接的前缀 |
| `JANUS_DATABASE_URL` | `postgres://janus:janus@localhost:5432/janus?sslmode=disable` | 后端直连 Postgres |
| `JANUS_TEST_DATABASE_URL` | `…/janus_test…` | 测试库连接串(与开发库分离) |
| `JANUS_MIGRATIONS_DIR` | `migrations` | 迁移文件目录(生产容器内为 `/app/migrations`) |
| `JANUS_SUPERADMIN_EMAIL` | 空 | 超管邮箱,启动时初始化(留空则无超管) |
| `JANUS_COOKIE_SECURE` | `false` | 会话 cookie Secure 标记;开发 http 必须 false |
| `JANUS_SESSION_TTL` / `JANUS_SESSION_TTL_SHORT` | `720h` / `24h` | 记住我 / 普通会话 |
| `JANUS_VERIFY_TOKEN_TTL` / `JANUS_RESET_TOKEN_TTL` | `24h` / `1h` | 验证 / 重置 token 有效期 |
| `JANUS_JWT_SECRET` / `JANUS_JWT_TTL` | 空 / `24h` | JWT 签名密钥与有效期;密钥留空则每次启动随机生成(重启后已签发 token 失效),生产必须配置 |
| `JANUS_DNS_RETRY_INTERVAL` / `JANUS_DNS_MAX_AGE` | `5m` / `72h` | DNS 重试间隔 / 最长等待 |
| `JANUS_VISIT_RETENTION` / `JANUS_VISIT_CLEANUP_INTERVAL` | `2160h` / `24h` | 访问记录保留 / 清理间隔 |
| `JANUS_LANDING_UPLOAD_DIR` | `uploads` | 上传落地页存放目录(生产挂持久卷) |
| `JANUS_LANDING_MAX_ZIP_BYTES` / `JANUS_LANDING_MAX_FILES` | `10485760` / `500` | 落地页压缩包解压后总大小 / 文件数上限 |
| `JANUS_SMTP_HOST/PORT/USERNAME/PASSWORD/FROM` | 空 / `465` | 配置 HOST 才启用真实 SMTP,否则控制台 mailer |
| `JANUS_ACME_EMAIL` | 空 | Let's Encrypt 账户邮箱(仅生产,配合 Caddyfile.prod) |
| `JANUS_DB_USER` / `JANUS_DB_PASSWORD` / `JANUS_DB_NAME` | `janus` ×3 | 给生产 compose 建库并拼连接串;生产必须改密码 |

> ⚠️ `.env` 含数据库密码与 SMTP 凭据,已在 `.gitignore` 中,**不要提交**。

---

## 7. 开发规范

### 7.1 提交前必须跑通

```bash
# 后端:格式化 + 测试
gofmt -l internal cmd            # 应无输出
go vet ./...
go test ./...                    # 需要 janus_test 库(见 8.1)

# 前端:类型 + UI 门禁 + 构建
cd web
pnpm type-check
pnpm build                        # 必须先 build
pnpm check:ui                     # 0 违规才通过(第 8 项要读 dist 产物,dist 缺失即失败)
```

> `pnpm build` 必须在 `pnpm check:ui` **之前**:第 8 项检查动效变体在编译产物里是否存在,
> dist 不存在时它会计入违规并非零退出(不再静默跳过)。上述顺序由
> `.github/workflows/ci.yml` 与 `docker/Dockerfile` 强制执行。

### 7.2 后端约定

- **分层**:`httpapi`(HTTP 与鉴权/校验/限流)→ `store`(数据访问与领域约束)→ Postgres;`rules`、`geo`、`domain`、`jwt`、`mailer` 是被复用的独立包。跳转热路径的代码在 `httpapi/redirect.go`,规则求值在 `rules` 包,两者通过窄接口解耦。
- **注释写"为什么"**:本仓库的注释习惯是记录被否决的方案与不变式(参见 `internal/httpapi/redirect.go`、`internal/geo/geo.go` 顶部注释),而不是复述代码。改这段链路前先读注释。
- **新增受保护路由必须同步 Casbin 策略**:在 `internal/rbac/rbac.go` 的 `policyText` 追加 `p, tenant, <path>, <METHOD>`;**漏了会被授权中间件 403 拒绝**,即使登录成功。注意 `keyMatch3` 下裸路径与 `/*` 是两条不同策略,需要成对列出。
- **统一错误响应**:用 `respond.go` 的助手返回 `{ code, message, details }`;错误码稳定、消息面向人。
- **不要在跳转热路径引入 DB 查询或计数表写入**(见 1.2 的三条硬约束)。
- **不重造轮子**:网络、解析、限流、邮件、迁移、配置、CIDR 匹配一律用成熟库(见 3.3);新增依赖前先确认标准库或已有依赖不能解决。
- **不要给 `geo` 填假值**:查不到就是空值,空值下规则条件恒不命中(ADR-0009)。

### 7.3 数据库迁移

- 迁移文件放在 `migrations/`,命名 `NNNN_snake_case.sql`,由 **goose** 执行;文件头写 `-- +goose Up` / `-- +goose Down`。
- 只增不改:已发布的迁移**不修改**,新增一条向后兼容的迁移。
- 当前 14 条迁移只提供 `-- +goose Up`(v1 不支持 `goose down`),回滚靠反向迁移。
- 启动时自动 `goose up`,幂等;新增迁移后同时更新 `internal/store` 的读写代码与相关测试。

### 7.4 前端约定

完整契约见 [`web/UI_KIT.md`](web/UI_KIT.md);门禁脚本 `web/scripts/check-ui-consistency.mjs` 会强制以下规则:

- **三层设计令牌**:`main.css` 的 `:root` / `.dark` 定义项目层(surface / ink / line / ok-warn-err)、shadcn 语义层(background / primary / destructive / …)、控件状态层(control-bg / control-track / control-thumb)三层,再经 `@theme inline` 映射成 Tailwind 工具类。
- **组件状态色一律走语义层**:`components/app/` 内部**禁止**写 `dark:` 补丁类名,所有主题差异由令牌自身换值保证(例:开关轨道用 `data-[state=unchecked]:bg-control-track`,不是 `bg-surface-muted dark:bg-…`)。
- **只用 `App*` 组件**:业务视图里不写裸 `<button>` / `<input>` / `<select>` / `<table>`,统一用 `components/app/` 的封装。两层分工(ADR-0011):`components/ui/` 是 shadcn/Reka UI 原语(kebab-case 文件,可整目录替换),`components/app/` 是项目组件(`App*` 前缀),**`app/` 不得直接 import `reka-ui`**,要新原语先在 `ui/` 包一层。
- **禁止重新引入 ant-design-vue**;禁止 legacy 类名与 legacy 令牌;禁止硬编码纯白 `bg-white` / `text-white`(带 alpha 的合法)。
- **响应式一律用 Tailwind 断点工具类**,不在 JS 里判断视口宽度;需要按自身宽度换挡的用 CSS container query。
- **深色模式**:`html.dark` 由 `stores/theme.ts` 控制并持久化到 localStorage。
- 图标统一 `@lucide/vue`;总览页世界地图依赖必须动态 `import()`(仅该页加载)。

### 7.5 加一个新功能的顺序

1. 架构决策先对齐:涉及系统设计或技术选型取舍时写 `docs/adr/NNNN-*.md`。
2. 迁移 → `store` → `httpapi`(路由 + RBAC 策略 + 错误码)。
3. 涉及判定逻辑(如规则求值)时,补黑盒测试:`internal/httpapi/*_test.go` 走 `testutil.Setup`。
4. 前端:API 类型 → 页面 → 接进路由与导航。
5. 更新本 README 的功能列表与 `docs/`。

---

## 8. 测试

### 8.1 后端自动化测试

后端测试是**黑盒 HTTP 测试**:`httptest.Server` + **真实 Postgres** + **真实迁移**,不 mock 内部函数(`internal/testutil/testutil.go`)。需要 `janus_test` 测试库,不可用时用例自动跳过。

```bash
docker compose up -d postgres
docker compose exec postgres psql -U janus -d janus -c 'CREATE DATABASE janus_test'

go test ./...                                   # 全部
go test ./internal/httpapi/ -count=1 -v        # 单包 + 详细输出
go test ./... -cover                            # 覆盖率
JANUS_TEST_DATABASE_URL='postgres://janus:janus@localhost:5432/other_test?sslmode=disable' go test ./...
```

每个测试的 `Setup` 会连接测试库、执行迁移、`TRUNCATE` 业务表并重新插入免费档种子(短链 100 / 域名 10),用例之间互不干扰,可重复运行。**看到全部 SKIP 就是连不上测试库**,先建库。

### 8.2 前端质量门禁

```bash
cd web
pnpm type-check   # vue-tsc --noEmit
pnpm build        # 先产出 dist —— 第 8 项要读它
pnpm check:ui     # UI 规范门禁:legacy 类名/令牌、未定义 CSS 变量、调色板泄漏、
                  # 任意字号、裸 HTML 原语、状态色对比度(双主题)、硬编码纯白
pnpm build        # 产物到 web/dist(不入库,镜像构建期自行重建)
```

前端没有单元测试框架,质量靠类型检查 + 门禁脚本 + 端到端验证。

### 8.3 端到端验证(curl)

未配置 SMTP 时,验证链接打印在启动后端的终端。

```bash
BASE=https://app.janus.test

# 1. 注册 → 201,status=pending,返回 defaultDomain:"alice.janus.test"
curl -sk -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123","slug":"alice"}'

# 2. 从后端终端复制验证链接([Janus mailer] 段落)
#    https://app.janus.test/verify-email?token=<token>
curl -sk -X POST "$BASE/api/auth/verify-email" -H 'Content-Type: application/json' \
  -d '{"token":"<token>"}'                                # → {"status":"ok"}

# 3. 登录并保存 cookie
JAR=$(mktemp /tmp/janus-cookies.XXXXXX)
curl -sk -c "$JAR" -b "$JAR" -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123","rememberMe":true}'

# 4. 脚本用 Bearer JWT(免 cookie / CSRF)
TOKEN=$(curl -sk -X POST "$BASE/api/auth/token" -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123"}' \
  | grep -o '"accessToken":"[^"]*"' | cut -d'"' -f4)
curl -sk "$BASE/api/auth/me" -H "Authorization: Bearer $TOKEN"

# 5. 读域名(Bearer);写操作需 CSRF 双提交 token
curl -sk "$BASE/api/domains" -H "Authorization: Bearer $TOKEN"
CSRF=$(awk '$6=="janus_csrf"{print $7}' "$JAR")
curl -sk -c "$JAR" -b "$JAR" "$BASE/api/domains"

# 6. 建短链(多目标按轮询)
curl -sk -c "$JAR" -b "$JAR" -X POST "$BASE/api/links" \
  -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
  -d '{"targetUrls":["https://example.com","https://example.org"],"domainIds":[1]}'

# 7. 跳转:用 --resolve 临时解析子域,不用改 hosts
curl -sk -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  --resolve alice.janus.test:443:127.0.0.1 "https://alice.janus.test/<短码>"
# → 302 https://example.com/ 或 https://example.org/(轮询)

# 8. 未命中 → 404
curl -sk -o /dev/null -w '%{http_code}\n' \
  --resolve alice.janus.test:443:127.0.0.1 "https://alice.janus.test/not-exist"
```

浏览器端到端验证顺序(与生产上线清单同构):打开 `https://app.janus.test` → 注册 → 验证 → 登录看到默认域名 `active` → 建短链 → 访问短码看 302 与统计 +1 → 访问不存在短码得 404 → (可选)加自有域名并走停用/恢复/删除 → (可选)用超管邮箱登录看平台管理。

### 8.4 测试数据与重置

- 开发库 `janus` 与测试库 `janus_test` 分离,`go test` 只动 `janus_test`。
- 彻底清空开发数据(含数据卷,证书缓存与本地 CA 会一起没,之后需重新信任):

```bash
docker compose down -v && docker compose up -d
```

---

## 9. 开发 / 生产差异

| 维度 | 开发 | 生产 |
| --- | --- | --- |
| 编排 | 基础设施 Docker(Postgres + Caddy);后端/前端终端跑(`go run` + `pnpm dev`) | `docker compose -f docker-compose.prod.yml up -d`(全部容器化) |
| 端口 | 后端 8080;Caddy 映射宿主 443/80 | 标准 80/443;后端不映射宿主机端口 |
| 域名解析 | hosts 把 `app.janus.test` 与测试子域指向 `127.0.0.1`(`JANUS_SERVER_PUBLIC_IP=127.0.0.1`,Go 读 hosts 走真实路径) | 真实 DNS 泛解析 `*.<平台域名>` |
| 证书 | Caddy 本地 CA(`tls internal` + on-demand),`caddy trust` 信任根证书 | Let's Encrypt(ACME 自动签发 / 续期) |
| 邮件 | 控制台 mailer(链接打印在后端日志) | 真实 SMTP(部署者提供凭据) |
| Cookie | `JANUS_COOKIE_SECURE=false`(http) | 强制 `true`(https) |
| 平台域名 | `janus.test`(RFC 保留测试域) | 真实域名 |
| 前端产物 | Vite Dev Server(5173) | nginx 镜像内的静态产物(构建期生成,不入库) |
| 落地页目录 | `uploads/` 本地目录 | 持久卷 `uploads` |

---

## 10. 生产部署(速览)

完整步骤见 **[`docs/deploy.md`](docs/deploy.md)**。最小流程:

```bash
# 1. DNS:平台裸域名 A/AAAA + 泛解析 *.<平台域名> 指向服务器公网 IP
dig +short example.com && dig +short any.example.com

# 2. 配置环境变量
cp .env.example .env && chmod 600 .env
#    必填:JANUS_PLATFORM_DOMAIN、JANUS_SERVER_PUBLIC_IP、JANUS_DB_PASSWORD
#    建议:JANUS_SUPERADMIN_EMAIL、JANUS_ACME_EMAIL、JWT 密钥(见 6.7)

# 3. Caddyfile.prod 的站点地址不支持环境变量,替换成你的平台域名
sed -i '' 's/example\.com/YOUR-DOMAIN/g' Caddyfile.prod   # macOS
sed -i 's/example\.com/YOUR-DOMAIN/g' Caddyfile.prod      # Linux

# 4. 启动
docker compose -f docker-compose.prod.yml config --quiet
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml ps
```

**首次启动自动完成**:连库 → goose 迁移 → 插入免费档种子 → 按 `JANUS_SUPERADMIN_EMAIL` 幂等初始化超管 → 启动后台 worker → 监听 8080。证书是**按需签发**的:租户域名激活后,首次访问时 Caddy 询问授权端点并签发 Let's Encrypt 证书,到期前自动续期。

**发布与回滚**:改前端只重建 `web` 镜像,改后端只重建 `backend` 镜像(前后端分离,互不牵连);镜像无版本 tag,回滚 = `git checkout <上一个正常提交>` 后重新 `up -d --build`。

**备份**:`pgdata` 必须备份(租户/短链/访问/规则全在里面);`caddy_data` 建议备份(丢失会触发 Let's Encrypt 重新签发额度);`uploads` 落地页目录需挂持久卷并单独备份。

**上线检查**:`/healthz` 返回 ok、后台证书为 Let's Encrypt 签发、注册能收到验证邮件、`<slug>.<平台域名>` 短链可跳转且计数增长、自有域名 pending → active 正常、超管可治理。

### 已知限制

- **Let's Encrypt 额度**:平台默认域名按子域逐个签发,受"每注册域名每周 50 张证书(含续期)"限制,额度按平台域名聚合(ADR-0004);接近上限需另择方案。
- **访问记录保留 90 天**,后台任务清理,可用 `JANUS_VISIT_RETENTION` 调整。
- **geo 为离线库**:`asn` / `is_datacenter` 无数据源,依赖它们的规则条件恒不命中(设计如此,不猜值);`internal/geo/data/` 的 xdb 需要手动更新才能识别新 IP 段。
- **授权端点仅内网可达**:未激活域名拒绝签发证书;不要暴露到公网。
- **规则上限**:单租户 200 条;规则快照有 1 分钟 TTL,写入时会主动失效(漏失效会让刚保存的规则短暂不生效且无报错)。

---

## 11. 故障排查

**开发环境**

| 症状 | 原因 / 处理 |
| --- | --- |
| 证书不受信任 / `curl: (60)` | 未信任本地 CA:执行 6.3;临时排查用 `curl -k` |
| 域名访问 Vite 403 | Vite Host 校验未放行:确认 `web/.env.development` 的 `VITE_PLATFORM_DOMAIN` 与访问域名一致 |
| Caddy 502 | 后端没起:确认 6.4 的 `go run` 在跑且监听 8080(`curl -s http://127.0.0.1:8080/healthz`) |
| 注册返回 409 | 邮箱或 slug 被占用(开发库有历史数据):换 slug,或 8.4 重置 |
| 自有域名一直 `pending` | hosts 未加该域名 / 未指向 127.0.0.1;或未到 5 分钟重试周期,可在后台点「重新校验」 |
| 域名 `failed` | 72 小时内未通过校验:检查 hosts 与 `JANUS_SERVER_PUBLIC_IP` |
| 跳转 404 | 短码不存在 / 域名停用 / 短链停用;或访问的 Host 不是该租户的 active 域名 |
| 收不到邮件,终端也没有 `[Janus mailer]` | 启动后端时 export 了 `JANUS_SMTP_*`,走了真实 SMTP;不 export 即回落到控制台 mailer |
| 改 Go 代码不生效 | 开发后端不在 Docker 里:`Ctrl+C` 后重新 `go run` |
| 改前端不生效 | 用 6.5 的 Dev Server;若看的是镜像里的旧页面,需 `pnpm build` 后重建 `web` 镜像 |
| 测试全部 SKIP | `janus_test` 库不存在:按 8.1 第 2 步创建 |
| 80/443 被占用 | 开发 compose 的 Caddy 占用 80/443,与其他绑定这两个端口的服务错开;后端 8080 被占则改 `JANUS_ADDR` 并同步代理配置 |
| `pnpm check:ui` 报状态色对比度 | 改令牌值时没同时看 `:root` 与 `.dark` 两块;详见 `web/UI_KIT.md` |

**生产环境**

| 症状 | 原因 / 处理 |
| --- | --- |
| compose 报 `JANUS_XXX 必须设置` | `.env` 缺必填变量(compose `:?` 强制):对照 10 与 6.7 补全 |
| Caddy 启动失败 `expanding email address ... is empty` | `JANUS_ACME_EMAIL` 为空但 `Caddyfile.prod` 启用了 `email` 指令:设置变量或注释指令 |
| 证书不签发 / `cert_status=failed` | 看 `docker compose logs caddy`;确认 DNS 生效、80/443 可达、授权端点放行 |
| 平台默认域名无法访问 | 泛解析 `*.<平台域名>` 未配置或未生效:`dig` 验证 |
| 访问者看到 404 | 域名/短链被停用或删除;或规则 `notfound` 裁决命中;或 Host 不匹配 active 域名 |
| 访问者看到 429 | 规则 `throttle` 裁决命中:去规则列表看优先级与条件 |
| 收不到验证/重置邮件 | 检查 `.env` SMTP 配置与 `docker compose logs backend` |
| 部署后页面还是旧的 | 改了前端但没重建 `web` 镜像:`docker compose -f docker-compose.prod.yml up -d --build web` |
| 磁盘不足 | 镜像与日志堆积:`docker system prune`(谨慎);关注 `pgdata` 与访问量 |

---

## 12. API 概览

**认证方式**

- 后台:会话 cookie `janus_session`(HTTP-only / Secure / SameSite=Lax);写方法需带 `X-CSRF-Token`(值取自 `janus_csrf` cookie,双提交)。
- 脚本:`POST /api/auth/token` 换 `accessToken`,之后带 `Authorization: Bearer <token>`(header 认证免疫 CSRF)。
- 跳转:`GET /{code}`,由 Host 决定域名,无需鉴权。
- 内部:`GET /internal/caddy/authorize?domain=<fqdn>`,仅内网可达。

**主要端点**

| 分组 | 端点 |
| --- | --- |
| 认证 | `POST /api/auth/register`、`verify-email`、`login`、`token`、`logout`、`change-password`、`forgot-password`、`reset-password`;`GET /api/auth/me` |
| 域名 | `GET/POST /api/domains`;`GET/PATCH/DELETE /api/domains/{id}`;`POST /api/domains/{id}/recheck` |
| 短链 | `GET/POST /api/links`;`GET/PATCH/DELETE /api/links/{id}`;`POST /api/links/{id}/purge`;`POST /api/links/batch-delete`;`POST /api/links/batch-purge`;`POST /api/links/{id}/landing`;`GET /api/links/{id}/visits`;`GET /api/links/{id}/stats`;`GET/PUT /api/links/{id}/rules` |
| 规则 | `GET/POST /api/rules`;`GET /api/rules/options`;`GET/PATCH/DELETE /api/rules/{id}`;`POST /api/rules/simulate`;`POST /api/rules/validate-expr` |
| 租户设置 | `GET/PATCH /api/me`;`GET/PATCH /api/me/error-pages`;`GET /api/config` |
| 平台管理 | `GET /api/admin/tenants`、`/api/admin/tenants/{id}`、`/api/admin/tiers`;`PATCH /api/admin/tenants/{id}`;`DELETE /api/admin/domains/{id}` |
| 跳转(公开) | `GET /{code}`;落地页型另有 `GET /{code}/`、`/{code}/click`、`/{code}/sdk.js`、`/{code}/<静态文件>` |
| 运维 | `GET /healthz` |

**授权模型**:Casbin,角色 `tenant` / `superadmin`,按 (角色, 方法, 路径) 判权;策略在 `internal/rbac/rbac.go` 的 `policyText` 内存装载。**新增受保护路由必须同步追加策略**,否则登录后仍会被 403。

**统一错误响应**

```json
{ "code": "E_DOMAIN_LIMIT", "message": "域名数量已达上限 (5/10)", "details": {} }
```

---

## 13. 参与贡献

**开发准备**:按 [6. 快速开始](#6-快速开始开发环境)把开发环境跑起来,并确认 8.1 的测试库已创建。

**提交前检查清单**

```bash
gofmt -l internal cmd      # 无输出
go vet ./...
go test ./...
cd web && pnpm type-check && pnpm build && pnpm check:ui
```

**代码风格**

- Go:`gofmt`;注释解释"为什么"与不变式,不写复述代码的注释;新依赖前先确认标准库/已有依赖不能解决(仓库已明确不自造轮子:迁移、限流、邮件、配置、CIDR 匹配、表达式引擎都用成熟库)。
- Vue:三层令牌 + `App*` 组件(见 7.4);不引入 ant-design-vue;不在 `components/app/` 写 `dark:` 补丁;不写裸 HTML 原语;响应式只用 Tailwind 断点或 container query。
- 数据库:迁移只增不改,已发布迁移不修改。

**架构决策**:架构取舍写 `docs/adr/NNNN-*.md`,格式参照现有 ADR(背景 → 权衡 → 后果)。不显然的决策要有 ADR,不要只写在提交信息里。过程性功能设计与实施草稿不随仓库提交,避免随迭代失效。

---

## 14. 许可证

Janus 采用 **GNU Affero General Public License v3.0(AGPL-3.0)**,许可证全文见 [`LICENSE`](LICENSE)。

```
Copyright (C) 2026 ruanbw and Janus contributors
SPDX-License-Identifier: AGPL-3.0-only
```

要点:

- 你可以自由使用、修改、再分发与集成,包括闭源商用(保留版权与许可证声明,提供源码的方式见 AGPL §4~6)。
- **AGPL 与 GPL 的关键差异**:通过网络向用户提供本程序的功能(即把它部署成 SaaS / 在线服务)时,必须向这些用户提供**对应源码**。自托管多租户服务尤其要注意:对外提供 Janus 的在线跳转/管理服务,需要开放修改后的源码。
- 无任何担保,作者不对使用后果负责(见 AGPL §15~17)。
- 商业授权 / 闭源分发需求请单独联系作者。

**第三方组件**:本项目使用若干第三方开源组件(Go 生态:gin、GORM、casbin、goose、go-mail 等;前端:Vue、Tailwind CSS、Reka UI、d3-geo、topojson-client、world-atlas 等),它们各自保留其许可证;其中 `world-atlas` 为 ISC 许可(允许再分发),随包发布时其许可声明需一并保留。

---

## 15. 相关文档

| 文档 | 内容 |
| --- | --- |
| [`docs/deploy.md`](docs/deploy.md) | 生产部署指南与上线检查清单 |
| [`docs/testing.md`](docs/testing.md) | 测试流程、分层验证与测试库要求 |
| [`web/README.md`](web/README.md) | 前端技术栈、页面清单、认证与请求约定 |
| [`web/UI_KIT.md`](web/UI_KIT.md) | 前端组件契约与三层设计令牌规范 |
| [`docs/adr/`](docs/adr/) | 架构决策记录(技术选型与关键系统决策) |
| [`.env.example`](.env.example) | 环境变量模板(带逐项说明) |

**一句话总结**:Janus 是一台"接上 DNS 就能用"的短链服务 —— 租户带域名进来,证书与统计与规则都自带。
