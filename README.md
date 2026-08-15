# CLOAK — 自托管多租户短链服务

CLOAK 是一个自托管的多租户短链服务:租户管理自己的域名与短链,系统为每个已激活域名自动签发并续期 HTTPS 证书,并把「域名/短码」的访问重定向到目标 URL。

- **单服务器部署**:Go 后端(Gin RESTful API + GORM ORM + 内嵌前端)+ PostgreSQL + Caddy(on-demand TLS,Let's Encrypt 自动签发/续期)。
- **多租户隔离**:租户之间的域名与短链完全隔离;部署者拥有平台管理员角色,可治理全平台。
- **单二进制交付**:前端构建产物(`web/dist`)经 `go:embed` 内嵌进 Go 二进制,镜像即服务。

本文档覆盖:**开发环境测试**(第 4~7 节)与**生产部署**(第 8~10 节)的完整流程。更简洁的部署速览见 [`docs/deploy.md`](docs/deploy.md)。

---

## 目录

1. [功能特性](#1-功能特性)
2. [技术架构](#2-技术架构)
3. [仓库结构](#3-仓库结构)
4. [环境要求](#4-环境要求)
5. [快速开始(开发环境)](#5-快速开始开发环境)
6. [开发环境测试](#6-开发环境测试)
   - [6.1 后端自动化测试](#61-后端自动化测试)
   - [6.2 前端类型检查与构建](#62-前端类型检查与构建)
   - [6.3 端到端手动验证(浏览器)](#63-端到端手动验证浏览器)
   - [6.4 端到端验证(curl)](#64-端到端验证curl)
   - [6.5 测试数据与重置](#65-测试数据与重置)
7. [开发 / 生产差异](#7-开发--生产差异)
8. [生产部署](#8-生产部署)
   - [8.1 前置条件](#81-前置条件)
   - [8.2 DNS 配置](#82-dns-配置)
   - [8.3 配置 .env](#83-配置-env)
   - [8.4 修改 Caddyfile.prod](#84-修改-caddyfileprod)
   - [8.5 准备前端产物(如需改动)](#85-准备前端产物如需改动)
   - [8.6 启动部署](#86-启动部署)
   - [8.7 首次启动行为](#87-首次启动行为)
   - [8.8 上线验证清单](#88-上线验证清单)
   - [8.9 SMTP 配置(邮件)](#89-smtp-配置邮件)
   - [8.10 日常运维](#810-日常运维)
   - [8.11 备份与恢复](#811-备份与恢复)
   - [8.12 安全建议](#812-安全建议)
   - [8.13 已知限制](#813-已知限制)
9. [故障排查](#9-故障排查)
10. [API 概览](#10-api-概览)
11. [相关文档](#11-相关文档)

---

## 1. 功能特性

**注册与认证**
- 邮箱注册(防垃圾注册:注册后需邮箱验证)、登录、记住我(30 天/24 小时)、登出、修改密码、忘记/重置密码。
- 平台管理员(超管)由环境变量初始化,首次登录引导设置密码。

**域名管理**
- 平台默认域名:每个租户自动获得 `<slug>.<平台域名>`,邮箱验证后自动激活,不计配额。
- 自有域名:添加后自动做 DNS 激活校验(每 5 分钟重试,最长 72 小时),激活后自动签发 HTTPS 证书,到期自动续期;支持停用/恢复/删除。
- 手动重新校验(`recheck`),证书签发状态可查。

**短链管理**
- 自定义短码或自动生成(长度可配),一条短链可关联多个域名,同一短码在不同域名下可指向不同目标。
- 临时跳转(302)与永久跳转(301)可配;启停、逻辑删除(记录保留)、彻底删除。
- 配额:按等级限制短链数与自有域名数,超限返回明确错误(含当前用量/上限)。

**访问统计**
- 记录每次访问的 User-Agent、来源页与时间,短链访问计数与访问列表。

**平台管理(超管)**
- 查看全部租户、封禁/解封、调整等级、强删违规域名。

**安全**
- 会话 cookie(HTTP-only)+ CSRF 双提交 token;注册/登录/忘记密码限流;目标 URL 拒绝 CRLF 防 header 注入;Caddy 授权端点仅内网可达,未激活域名拒绝签发证书。

> 术语(租户、短链、短码、目标 URL、域名、激活、访问、证书、后台等)以 [`CONTEXT.md`](CONTEXT.md) 词汇表为准。

---

## 2. 技术架构

```
                        Internet
                           │
              ┌────────────┴─────────────┐
              │  Caddy (80/443)          │
              │  on-demand TLS           │  生产:Let's Encrypt
              │  ask → 授权端点          │  开发:本地 CA (80/443)
              └───────┬───────────┬──────┘
                      │           │
              平台域名/后台    *.<平台域名> 租户子域
              (SPA + API)    (短链跳转)
                      │           │
              ┌───────┴───────────┴──────┐
              │  Go 后端 (单二进制)       │
              │  /api/*  REST API        │
              │  /{code}  跳转路由        │
              │  /internal/caddy/authorize│
              │  内嵌前端 (go:embed)      │
              │  后台 worker:            │
              │   DNS 重试/证书探活/清理  │
              └───────────┬──────────────┘
                          │
                 ┌────────┴────────┐
                 │  PostgreSQL 16  │
                 └─────────────────┘
```

- **Go 后端**:`cmd/cloak` 启动 → 加载配置 → 连接 Postgres → 执行迁移 → 初始化超管 → 启动后台任务 → HTTP 服务。所有环境差异由环境变量承载(见 [spec 决策 #14](.scratch/cloak/spec.md))。
- **Caddy**:TLS 终止与证书生命周期交给 Caddy on-demand TLS。收到陌生域名的首个 TLS 握手时,向应用内部授权端点(`/internal/caddy/authorize`)询问该域名是否已激活,是则自动签发并续期 Let's Encrypt 证书(ADR-0002)。
- **前端**:Vue 3 + TypeScript + Vite + Ant Design Vue 的 SPA,构建产物 `web/dist` 经 `go:embed` 内嵌进 Go 二进制(ADR-0003)。后台域名(裸平台域名或 `app.<平台域名>`)下非 API 路径由 Go 直接服务 SPA(含 history 路由回退)。

---

## 3. 仓库结构

```
.
├── cmd/cloak/                  # 后端入口
├── internal/
│   ├── bootstrap/              # 部署期初始化(超管)
│   ├── config/                 # 环境变量配置
│   ├── db/                     # Postgres 连接 + 自研迁移器
│   ├── domain/                 # DNS 校验、证书探活、短码生成、后台 worker
│   ├── httpapi/                # HTTP 路由/处理器(+ 黑盒测试)
│   ├── mailer/                 # 可插拔邮件(SMTP / 控制台)
│   ├── store/                  # 数据访问层
│   └── testutil/               # 测试基础设施(httptest + 真实 Postgres)
├── migrations/                 # SQL 迁移(启动时自动执行)
├── web/                        # 前端 SPA(见 web/README.md)
│   ├── src/                    # Vue 源码
│   ├── dist/                   # 构建产物(入库,go:embed 使用)
│   └── embed.go                # go:embed 内嵌 dist
├── docker/
│   └── Dockerfile              # 多阶段构建,单二进制 + 迁移文件
├── docker-compose.yml          # 开发环境编排
├── docker-compose.prod.yml     # 生产环境编排
├── Caddyfile                   # 开发 Caddy 配置(本地 CA)
├── Caddyfile.prod              # 生产 Caddy 配置(Let's Encrypt)
├── .env.example                # 环境变量模板(复制为 .env)
├── CONTEXT.md                  # 领域词汇表
└── docs/
    ├── deploy.md               # 生产部署指南
    ├── adr/                    # 架构决策记录
    └── agents/                 # agent 协作约定
```

---

## 4. 环境要求

| 工具 | 版本 | 用途 |
| --- | --- | --- |
| Docker Desktop(含 Compose v2) | 任意较新版本 | 基础设施:Postgres / Caddy |
| Go | ≥ 1.26(见 `go.mod`) | 终端启动后端(`go run`)与自动化测试 |
| Node.js | ≥ 20.19(见 `web/package.json` engines) | 前端开发/构建 |
| pnpm | ≥ 9 | 前端依赖管理 |
| Caddy 命令行(可选) | ≥ 2.11 | 开发环境信任本地 CA(`caddy trust`) |

> 开发环境后端用 `go run` 在终端启动、前端用 `pnpm dev` 启动,因此 `go` 与 Node/pnpm 都是必需;Docker 只承载 Postgres 与 Caddy。

---

## 5. 快速开始(开发环境)

### 5.1 启动基础设施(Docker)

开发环境的分工:**基础设施放 Docker**(Postgres、Caddy 反代),**Go 后端与前端在终端直接启动**,改代码即时调试(5.5 / 5.6)。

```bash
docker compose up -d
```

| 服务 | 容器内 | 宿主机映射 | 说明 |
| --- | --- | --- | --- |
| `postgres` | 5432 | `127.0.0.1:5432` | PostgreSQL 16,开发凭据 `cloak/cloak` |
| `caddy` | 443 | `127.0.0.1:443` / `127.0.0.1:80` | HTTPS 入口,on-demand TLS + 本地 CA;反代到宿主机 `:8081` 的后端(`host.docker.internal`) |

> Caddy 映射宿主 **443**(https 无端口访问)与 **80**(Caddy 自动 308 跳 https)。
> 无端口访问依赖 SwitchHosts 把 `*.cloak.test` 指向 127.0.0.1;后端在终端监听 **8081**(见 5.6)。
> 注意:flow-filtering 项目的 openresty 容器绑定宿主 80/443/8080,开发时才启动它,与本服务错开。

### 5.2 配置本地域名解析(SwitchHosts / `/etc/hosts`)

开发环境支持两种访问入口:**localhost 直连**与**域名访问**(把域名指向本机)。域名解析用 SwitchHosts 等 hosts 管理工具或手动改 `/etc/hosts` 均可;Caddy 按 Host/SNI 路由、Vite 按 Host 放行,都需要把平台后台域名和你要测试的租户子域指向本机。

**SwitchHosts**(推荐):新增一条规则并开启,内容与 `/etc/hosts` 相同:

```text
127.0.0.1 app.cloak.test
127.0.0.1 alice.cloak.test
127.0.0.1 bob.cloak.test
```

**手动追加到 `/etc/hosts`**:

```bash
sudo sh -c 'echo "127.0.0.1 app.cloak.test" >> /etc/hosts'
# 每注册一个测试租户(如 slug=alice),把它的子域也加进去:
sudo sh -c 'echo "127.0.0.1 alice.cloak.test bob.cloak.test" >> /etc/hosts'
```

- `app.cloak.test` 承载后台(SPA + API),域名入口两种方式都可用:`http://app.cloak.test:5173`(Vite Dev Server,热更新)与 `https://app.cloak.test`(Caddy + Go 内嵌产物,验证域名/TLS 形态,见 5.4)。
- `<slug>.cloak.test` 是租户的**平台默认域名**,用于验证短链跳转。
- 测试**自有域名**时,同样把它加进 hosts(如 `127.0.0.1 links.example.test`);`CLOAK_SERVER_PUBLIC_IP=127.0.0.1`,Go 的 DNS 校验会读取 hosts,走真实代码路径(spec 决策 #14)。

> hosts 不支持通配符,每个测试子域都要单独一行。也可以用 `curl --resolve`(见 6.4)临时解析,无需改 hosts。
> Vite Dev Server 默认拒绝非 localhost 的 Host(防 DNS rebinding,返回 403);项目已在 `vite.config.ts` 用 `server.allowedHosts` 放行 `.cloak.test`(由 `web/.env.development` 的 `VITE_PLATFORM_DOMAIN` 控制),换平台域名时同步修改。

### 5.3 信任 Caddy 本地 CA

开发环境证书由 Caddy 本地 CA 签发(浏览器默认不信任)。把根证书导出并加入系统信任:

```bash
mkdir -p certs
docker compose exec caddy cat /data/caddy/pki/authorities/local/root.crt > certs/caddy-root.pem
sudo caddy trust --ca certs/caddy-root.pem
```

macOS 备选(钥匙串导入并信任):

```bash
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/caddy-root.pem
```

> `certs/` 已在 `.gitignore` 中,不会入库。重新创建 Caddy 数据卷后需重新信任。

### 5.4 访问服务(双入口)

开发环境提供 **localhost 直连**与**域名访问**(SwitchHosts)两套入口:

| 入口 | 地址 | 说明 |
| --- | --- | --- |
| 前端/后台(localhost) | `http://localhost:5173` | Vite Dev Server,热更新;`/api` 代理到 8081 |
| 前端/后台(域名) | `http://app.cloak.test:5173` | 同上,经 SwitchHosts 域名访问(需 5.2 hosts) |
| 后台(域名 + HTTPS) | `https://app.cloak.test` | Caddy → Go 内嵌产物,验证域名/TLS/证书形态;前端改动需先 `pnpm build`(见 8.5) |
| 健康检查 | `https://app.cloak.test/healthz` | 期望 `{"status":"ok"}` |
| 后端 API(直连) | `http://127.0.0.1:8081` | 绕过 Caddy/Vite,开发调试用 |

### 5.5 前端本地开发(热更新)

后端按 5.6 在终端跑着,前端用 Vite Dev Server(自带 `/api` 代理到 `http://localhost:8081`):

```bash
cd web
pnpm install        # 首次
pnpm dev            # http://localhost:5173,/api 代理到后端 8081
```

- 浏览器访问 `http://localhost:5173` 或域名入口 `http://app.cloak.test:5173`(见 5.2),`/api/*` 自动代理到 Go 后端,`Set-Cookie` 透传(会话/CSRF cookie 正常)。
- 开发环境 `CLOAK_COOKIE_SECURE=false`,http 下 cookie 生效。
- 修改后端代码后在启动后端的终端 `Ctrl+C` 停掉,再重新 `go run` 即可(见 5.6);前端代码由 Dev Server 自动热更新。
- 修改前端源码后 Dev Server 自动热更新;发布前需重新构建 `web/dist`(见 8.5)。

### 5.6 终端启动后端(推荐)

后端直接在终端用 `go run` 启动,改代码后 `Ctrl+C` 重启即可,日志(含控制台 mailer 输出)直接显示在终端:

```bash
# 前提:Postgres 已启动(5.1);在仓库根目录执行
CLOAK_COOKIE_SECURE=false CLOAK_ADDR=:8081 go run ./cmd/cloak
```

- `CLOAK_COOKIE_SECURE=false`:开发走 http(Vite Dev Server / 直连 8081),Secure cookie 不生效;
- `CLOAK_ADDR=:8081`:与前端代理、Caddy 反代保持一致(本机 8080 常被 nginx 占用);
- 其余配置用代码默认值即可:数据库 `postgres://cloak:cloak@localhost:5432/cloak`、平台域名 `cloak.test`、公网 IP `127.0.0.1`、控制台 mailer(见 6.4);
- 需要覆盖时用环境变量,例如 `CLOAK_SUPERADMIN_EMAIL=admin@example.com CLOAK_COOKIE_SECURE=false CLOAK_ADDR=:8081 go run ./cmd/cloak`;
- 完整变量见 [.env.example](.env.example) 与 `internal/config/config.go`。`go run` 不会自动读取 `.env`(那是 compose 的行为),需要覆盖时在命令行 export。

启动后验证:

```bash
curl -s http://127.0.0.1:8081/healthz        # → {"status":"ok"}
curl -sk https://app.cloak.test/healthz # 经 Caddy 走通全链路
```

### 5.7 环境变量

开发环境默认值开箱即用,需要覆盖时从模板复制:

```bash
cp .env.example .env   # 可选;compose 会读取 .env 覆盖默认值
```

主要变量见下表(完整列表见 [.env.example](.env.example) 与 `internal/config/config.go`):

| 变量 | 开发默认值 | 说明 |
| --- | --- | --- |
| `CLOAK_PLATFORM_DOMAIN` | `cloak.test` | 平台域名;租户默认域名 `<slug>.<平台域名>` |
| `CLOAK_SERVER_PUBLIC_IP` | `127.0.0.1` | DNS 激活校验比对地址(开发:hosts 指向本机) |
| `CLOAK_ADDR` | `:8080` | HTTP 监听地址(compose 内) |
| `CLOAK_DATABASE_URL` | `postgres://cloak:cloak@localhost:5432/cloak?sslmode=disable` | Postgres 连接串 |
| `CLOAK_COOKIE_SECURE` | `false` | 会话 cookie Secure 标记;开发 http 必须 false |
| `CLOAK_SUPERADMIN_EMAIL` | 空 | 超管邮箱,启动时初始化(留空则无超管) |
| `CLOAK_PUBLIC_BASE_URL` | `https://app.cloak.test` | 邮件验证/重置链接前缀 |
| `CLOAK_SMTP_*` | 空 | 配置后走真实 SMTP,否则控制台 mailer(见 8.9) |
| `CLOAK_SESSION_TTL` / `CLOAK_SESSION_TTL_SHORT` | `720h` / `24h` | 记住我 30 天 / 24 小时 |
| `CLOAK_VERIFY_TOKEN_TTL` / `CLOAK_RESET_TOKEN_TTL` | `24h` / `1h` | 验证/重置 token 有效期 |
| `CLOAK_DNS_RETRY_INTERVAL` / `CLOAK_DNS_MAX_AGE` | `5m` / `72h` | DNS 重试间隔 / 最长重试时长 |
| `CLOAK_VISIT_RETENTION` / `CLOAK_VISIT_CLEANUP_INTERVAL` | `2160h` / `24h` | 访问记录保留 / 清理间隔 |

> ⚠️ `.env` 已在 `.gitignore` 中,不要提交(里面可能含真实 SMTP 凭据)。

---

## 6. 开发环境测试

### 6.1 后端自动化测试

测试是「运行中的服务(httptest.Server)+ 真实 Postgres + 真实迁移」的黑盒 HTTP 测试(见 `internal/testutil/testutil.go`),不 mock 内部函数。**需要本地 Postgres 可用,且存在 `cloak_test` 测试库**;测试库不可用时用例自动跳过。

```bash
# 1. 确保 Postgres 已启动
docker compose up -d postgres

# 2. 首次创建测试库(与开发库 cloak 分离;之后可复用)
docker compose exec postgres psql -U cloak -d cloak -c 'CREATE DATABASE cloak_test'

# 3. 运行全部测试
go test ./...

# 常用变体
go test ./internal/httpapi/ -count=1 -v     # 单包,禁用缓存 + 详细输出
go test ./... -cover                        # 覆盖率
CLOAK_TEST_DATABASE_URL='postgres://cloak:cloak@localhost:5432/other_test?sslmode=disable' go test ./...   # 指定测试库
```

- 每个测试 `Setup` 会连接测试库、执行迁移、`TRUNCATE` 业务表并重新插入免费档种子(`free`:短链 100 / 域名 10),测试互不干扰,可放心重复运行。
- 若看到用例全部 SKIP,说明连不上测试库:执行上面第 2 步后重跑。

### 6.2 前端类型检查与构建

```bash
cd web
pnpm type-check     # vue-tsc --noEmit,类型检查
pnpm build          # 产物输出到 web/dist(生产镜像经 go:embed 使用)
```

- `web/dist` 已入库(`git` 可见),这是单二进制部署的前提——改完前端源码后必须重新 `pnpm build` 并把新的 `dist` 提交,否则 Docker 镜像内嵌的是旧产物。
- 前端没有单元测试框架;质量保障靠 `pnpm type-check` + 构建 + 6.3/6.4 端到端验证。

### 6.3 端到端手动验证(浏览器)

按下列顺序在开发环境走通一遍(与生产上线清单同构):

1. 打开 `https://app.cloak.test`,确认证书受信任、页面正常加载。
2. 注册新租户(邮箱 + 密码 + slug,如 `alice`)。
3. 查看验证链接:未配置 SMTP 时,验证邮件直接打印在启动后端的终端(见 6.4 第 2 步);点击链接完成邮箱验证。
4. 登录后台:看到「域名」页包含平台默认域名 `alice.cloak.test`(状态 `active`)。
5. 「短链」页新建短链:目标 URL 填 `https://example.com`,关联 `alice.cloak.test`,提交后得到短码。
6. 浏览器访问 `https://alice.cloak.test/<短码>`(需 hosts 已加 `alice.cloak.test`),应 302 跳到目标地址;「统计」页能看到该短链访问数 +1。
7. 尝试访问不存在的短码,应 404。
8. (可选)添加自有域名:hosts 里加 `127.0.0.1 links.example.test`,后台添加后自动激活并签发证书;停用/恢复/删除流程各走一遍。
9. (可选)设置 `CLOAK_SUPERADMIN_EMAIL`(如 `CLOAK_SUPERADMIN_EMAIL=admin@example.com CLOAK_COOKIE_SECURE=false CLOAK_ADDR=:8081 go run ./cmd/cloak`)重启后端,用该邮箱登录,首次登录引导设置密码,进入平台管理页查看租户列表。

### 6.4 端到端验证(curl)

以下命令已在开发环境实测通过。未配置 SMTP 时验证链接打印在启动后端的终端:

```bash
BASE=https://app.cloak.test

# 1. 注册(邮箱、密码、slug)
curl -sk -X POST "$BASE/api/auth/register" \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123","slug":"alice"}'
# → 201,status=pending,返回 defaultDomain:"alice.cloak.test"

# 2. 从启动后端的终端读取验证链接(控制台 mailer)
# 终端启动后端默认不读取 .env 的 SMTP 配置,邮件直接打印在运行 go run 的终端:
# 输出形如:
#   [CLOAK mailer] 邮箱验证 alice@example.com
#     token: <40+ 位 token>
#     验证地址: https://app.cloak.test/verify-email?token=<token>

# 3. 邮箱验证
curl -sk -X POST "$BASE/api/auth/verify-email" \
  -H 'Content-Type: application/json' \
  -d '{"token":"<token>"}'
# → {"status":"ok"},租户转 active

# 4. 登录(保存 cookie)
JAR=$(mktemp /tmp/cloak-cookies.XXXXXX)
curl -sk -c "$JAR" -b "$JAR" -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123","rememberMe":true}'
# → 200;cookie 文件里应有 cloak_session 与 cloak_csrf

# 5. 写方法需要 CSRF 双提交 token(从 cookie 读取)
CSRF=$(awk '$6=="cloak_csrf"{print $7}' "$JAR")
curl -sk -c "$JAR" -b "$JAR" "$BASE/api/domains"
# → 200 [{"id":1,"fqdn":"alice.cloak.test","origin":"platform","status":"active",...}]

# 6. 在平台默认域名上创建短链(domainIds 用第 5 步返回的 id)
curl -sk -c "$JAR" -b "$JAR" -X POST "$BASE/api/links" \
  -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: $CSRF" \
  -d '{"targetUrl":"https://example.com","domainIds":[1]}'
# → 201,返回自动生成的短码(如 yomNzx)

# 7. 跳转验证:不需要改 hosts,用 --resolve 把子域临时解析到本机
curl -sk -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  --resolve alice.cloak.test:443:127.0.0.1 \
  "https://alice.cloak.test/<短码>"
# → 302 https://example.com/

# 8. 未命中 → 404
curl -sk -o /dev/null -w '%{http_code}\n' \
  --resolve alice.cloak.test:443:127.0.0.1 \
  "https://alice.cloak.test/not-exist"
# → 404
```

### 6.5 测试数据与重置

- 开发数据库与测试数据库分离(`cloak` / `cloak_test`);`go test` 只动 `cloak_test`。
- 想彻底清空开发数据(包括全部容器与数据卷):

```bash
docker compose down -v
docker compose up -d
```

> `-v` 会删除 `pgdata`、`caddy_data`、`caddy_config` 卷,所有租户、短链、证书缓存都会消失,之后需重新信任本地 CA(5.3)。

---

## 7. 开发 / 生产差异

| 维度 | 开发 | 生产 |
| --- | --- | --- |
| 编排 | 基础设施 Docker(`docker compose up -d`:postgres + caddy);后端/前端终端启动(`go run` + `pnpm dev`) | `docker compose -f docker-compose.prod.yml up -d`(全部容器化) |
| 端口 | 后端 8081;Caddy 映射宿主 443/80 | 标准 80/443;后端不暴露公网 |
| 域名解析 | `/etc/hosts` 把 `app.cloak.test` 与测试子域指向 `127.0.0.1`(`CLOAK_SERVER_PUBLIC_IP=127.0.0.1`,Go 读 hosts 走真实代码路径) | 真实 DNS 泛解析 `*.<平台域名>` |
| 证书 | Caddy 本地 CA(`tls internal` + `on_demand_tls`),`caddy trust` 信任根证书 | Let's Encrypt(ACME 自动签发/续期) |
| 邮件 | 控制台假 mailer(验证/重置链接打印在后端日志) | 真实 SMTP(部署者提供凭据;mailer 可插拔) |
| Cookie | `CLOAK_COOKIE_SECURE=false`(http) | `CLOAK_COOKIE_SECURE=true`(https,必须) |
| 平台域名 | `cloak.test`(RFC 保留测试域) | 真实域名 |
| 后台入口 | `app.cloak.test`(开发约定) | 裸平台域名(生产 Caddyfile 只配裸域名) |

---

## 8. 生产部署

### 8.1 前置条件

- 一台公网服务器,开放 **80/443** 端口(80 用于 Let's Encrypt HTTP-01 验证,443 用于 HTTPS);
- 一个域名(下称**平台域名**,例如 `example.com`),DNS 托管商支持 A/AAAA 记录;
- 服务器可访问外网(拉镜像、访问 Let's Encrypt);
- 安装 Docker Engine + Compose v2;
- 建议在服务器上安装 `git`,从仓库直接部署(构建上下文需要仓库内容,含 `web/dist` 与 `migrations`)。

### 8.2 DNS 配置

部署前完成 DNS 配置并等待生效:

| 记录 | 类型 | 值 | 用途 |
| --- | --- | --- | --- |
| `*.<平台域名>` | A / AAAA | 服务器公网 IP | 泛域名解析:所有租户的**平台默认域名**由此生效 |
| `<平台域名>` | A / AAAA | 服务器公网 IP | 裸平台域名:承载后台(SPA + API) |
| 租户自有域名 | A / AAAA | 服务器公网 IP | 由租户自行解析;系统校验其记录是否包含本服务器 IP |

验证:

```bash
dig +short example.com
dig +short test.example.com    # 随便一个子域
# 都应返回服务器公网 IP
```

> 泛域名解析是平台默认域名的前提(spec 决策 #13)。自有域名不需要泛解析,由租户各自配置。

### 8.3 配置 .env

```bash
cp .env.example .env
vim .env
```

生产必填/建议项:

| 变量 | 必填 | 生产值示例 | 说明 |
| --- | --- | --- | --- |
| `CLOAK_PLATFORM_DOMAIN` | ✅ | `example.com` | 裸平台域名,租户默认域名 `<slug>.example.com` |
| `CLOAK_SERVER_PUBLIC_IP` | ✅ | `1.2.3.4` | 服务器公网 IP,DNS 激活校验比对地址 |
| `CLOAK_DB_PASSWORD` | ✅ | 强随机密码 | Postgres 密码(compose 用 `:?` 强制,缺失直接报错) |
| `CLOAK_SUPERADMIN_EMAIL` | 建议 | `admin@example.com` | 超管邮箱,首次启动初始化(留空则无超管) |
| `CLOAK_ACME_EMAIL` | 建议 | `admin@example.com` | Let's Encrypt 账户邮箱(到期提醒;需配合 Caddyfile.prod 取消注释 `email` 指令) |
| `CLOAK_DB_USER` / `CLOAK_DB_NAME` | 可选 | 默认 `cloak` / `cloak` | 数据库用户/库名 |
| `CLOAK_COOKIE_SECURE` | — | 生产 compose 强制 `true` | 无需在 .env 配置 |
| `CLOAK_PUBLIC_BASE_URL` | — | 生产 compose 自动设为 `https://<平台域名>` | 无需配置 |
| `CLOAK_SMTP_HOST/PORT/USERNAME/PASSWORD/FROM` | 可选 | 见 8.9 | 真实邮件 |

其余可选变量(token 有效期、DNS 重试、访问保留时长等)见 [.env.example](.env.example),默认值与代码一致。

> ⚠️ `.env` 含数据库密码与 SMTP 凭据,务必保持不入库(仓库 `.gitignore` 已忽略)。生产服务器上设置文件权限:`chmod 600 .env`。

### 8.4 修改 Caddyfile.prod

Caddy 站点地址**不支持环境变量占位符**,`Caddyfile.prod` 把站点域名硬编码为 `example.com`。部署前把文件中的 `example.com` **全部**替换为你的平台域名(与 `CLOAK_PLATFORM_DOMAIN` 一致):

```bash
sed -i '' 's/example\.com/YOUR-DOMAIN/g' Caddyfile.prod   # macOS
# 或 sed -i 's/example\.com/YOUR-DOMAIN/g' Caddyfile.prod # Linux
grep -n "YOUR-DOMAIN" Caddyfile.prod                      # 确认替换
```

可选:取消注释 `email {env.ACME_EMAIL}` 指令以接收 Let's Encrypt 到期提醒(必须确保 `.env` 已设置 `CLOAK_ACME_EMAIL`,否则 Caddy 会因空邮箱启动失败)。

> `Caddyfile.prod` 随仓库版本管理,请用你自己的域名提交这份修改,不要在生产服务器上只改不提交。

### 8.5 准备前端产物(如需改动)

生产镜像构建时直接用仓库里的 `web/dist`(经 `go:embed` 编进二进制)。**如果前端源码有改动,先重新构建并提交 dist,再构建镜像**:

```bash
cd web
pnpm install
pnpm build      # 更新 web/dist
cd ..
git add web/dist web/index.html web/public
git commit -m "build: 更新前端产物"
```

不改前端则跳过此步。

### 8.6 启动部署

```bash
# 校验 compose 配置(变量缺失/语法错误会在这里暴露)
docker compose -f docker-compose.prod.yml config --quiet

# 启动
docker compose -f docker-compose.prod.yml up -d --build

# 查看状态与日志
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f backend caddy
```

首次启动会自动完成:连接 Postgres → 执行迁移(`migrations/`)→ 创建免费档种子 → 按 `CLOAK_SUPERADMIN_EMAIL` 初始化超管 → 启动后台 worker → 监听 8080(Caddy 反代)。

证书是**按需签发**的:租户域名激活后,访问者首次访问时 Caddy 询问授权端点并签发 Let's Encrypt 证书,到期前自动续期(ADR-0002、ADR-0004)。无需手动执行 certbot 等操作。

### 8.7 首次启动行为

- **迁移**:`db.Migrate` 启动时自动执行 `migrations/*.sql`(自研迁移器,幂等,重复启动安全)。
- **免费档种子**:`free` 等级(短链 100 / 域名 10),新租户默认免费档。
- **超管初始化**:`CLOAK_SUPERADMIN_EMAIL` 指定的邮箱若不存在则创建超管租户(active、暂无密码),已存在则确保超管标记;超管首次登录时引导设置密码(`firstLoginSetup`)。重复启动幂等。
- **后台任务**(`internal/domain/worker.go`):DNS 重试(每 5 分钟)、证书预签发探活、过期访问/会话/邮箱 token 清理(每 24 小时)。
- **邮件**:配置了 SMTP 用真实发送;未配置时验证/重置链接打印到后端容器 stdout(可通过 `docker compose ... logs backend` 查看,仅用于排查)。

### 8.8 上线验证清单

按顺序在**生产环境**走通(与 [docs/deploy.md](docs/deploy.md) 一致):

- [ ] `curl -k https://<平台域名>/healthz` 返回 `{"status":"ok"}`(首次可 `-k`,证书签发后应能去掉)
- [ ] 浏览器访问 `https://<平台域名>` 打开后台,证书为 Let's Encrypt 签发(非本地 CA)
- [ ] 注册新租户(提交 email/password/slug),收到验证邮件(需真实 SMTP,见 8.9;未配置时验证链接打印在后端日志)
- [ ] 邮箱验证后租户转 `active`,`<slug>.<平台域名>` 默认域名可承载短链
- [ ] 通过 `https://<slug>.<平台域名>/<短码>` 访问短链,返回 302/301 跳转,访问计数增长
- [ ] 添加自有域名:解析指向本服务器后自动激活并签发证书;未指向时状态 `pending`、超时 `failed`
- [ ] 超管登录后可见租户列表,可封禁/解封、调整等级、移除违规域名
- [ ] 停用短链/域名后访问返回 404;配额超限时创建被拒并提示用量/上限
- [ ] `docker compose -f docker-compose.prod.yml ps` 三个服务均 `healthy`/`running`

### 8.9 SMTP 配置(邮件)

mailer 为可插拔实现:未配置 SMTP 时使用控制台假 mailer(邮件内容打印到后端日志)。生产接入真实 SMTP 时,在 `.env` 配置:

| 变量 | 说明 |
| --- | --- |
| `CLOAK_SMTP_HOST` | SMTP 服务器地址,如 `smtp.example.com` |
| `CLOAK_SMTP_PORT` | 默认 `465`(隐式 TLS);`587` 必须支持 STARTTLS,否则报错(拒绝明文 AUTH) |
| `CLOAK_SMTP_USERNAME` / `CLOAK_SMTP_PASSWORD` | 认证凭据;用户名留空则不发送 AUTH |
| `CLOAK_SMTP_FROM` | 发件人地址;留空回退为 Username,两者都空则发送报错 |

compose 会把上述变量转发给后端容器。修改 `.env` 后重建/重启后端使其生效:

```bash
docker compose -f docker-compose.prod.yml up -d backend
```

### 8.10 日常运维

```bash
# 查看状态
docker compose -f docker-compose.prod.yml ps

# 查看日志(后端 + Caddy)
docker compose -f docker-compose.prod.yml logs -f backend
docker compose -f docker-compose.prod.yml logs -f caddy

# 健康检查(单次)
curl -s https://<平台域名>/healthz

# 发布新版本(拉代码 → 如前端有改动先构建 dist → 重建)
git pull
docker compose -f docker-compose.prod.yml up -d --build

# 重启单个服务(如修改 .env 后)
docker compose -f docker-compose.prod.yml up -d backend

# 停止(数据卷保留)
docker compose -f docker-compose.prod.yml down
```

**回滚**:CLOAK 镜像名固定、无版本 tag;回滚 = 切回旧代码再重建:

```bash
git checkout <上一个正常提交>
docker compose -f docker-compose.prod.yml up -d --build
```

### 8.11 备份与恢复

三个命名卷都需要纳入备份策略:

| 卷 | 内容 | 重要程度 |
| --- | --- | --- |
| `pgdata` | Postgres 全部数据(租户/短链/访问) | ★★★ 必须 |
| `caddy_data` | Let's Encrypt 证书、ACME 账户、本地 CA | ★★ 建议(丢失后证书会自动重新签发,但会触发 LE 签发额度) |
| `caddy_config` | Caddy 自动保存的配置 | ★ 可选 |

**Postgres 逻辑备份**(推荐,可恢复性最好):

```bash
# 备份(默认用户/库名均为 cloak;若 .env 修改过 CLOAK_DB_USER / CLOAK_DB_NAME,把下方 cloak 换成实际值)
docker compose -f docker-compose.prod.yml exec -T postgres pg_dump -U cloak cloak > cloak-backup-$(date +%Y%m%d-%H%M%S).sql

# 恢复(目标为新部署/已清空库;注意先确保服务已停止写库)
docker compose -f docker-compose.prod.yml exec -T postgres psql -U cloak -d cloak < cloak-backup-YYYYmmdd-HHMMSS.sql
```

**数据卷快照备份**(可选,含证书):

```bash
# 卷名形如 <compose 项目名>_pgdata(项目名默认是部署目录名 cloak;改过 COMPOSE_PROJECT_NAME 请相应替换)
docker run --rm -v cloak_pgdata:/data -v "$(pwd)":/backup alpine \
  tar czf /backup/pgdata-$(date +%Y%m%d).tar.gz -C /data .
```

建议:每日定时 `pg_dump` + 定期快照(证书卷),并做一次恢复演练。

### 8.12 安全建议

- 服务器只开放 **80/443**;生产 compose 中后端不映射宿主机端口,不要手动暴露 8081。
- `CLOAK_COOKIE_SECURE` 生产必须为 `true`(compose 已强制)。
- `.env` 文件权限 `chmod 600`,不要提交、不要进镜像(`.dockerignore` 已排除)。
- 使用强数据库密码与 SMTP 凭据。
- 授权端点 `/internal/caddy/authorize` 仅内网可达(应用侧校验来源 IP),不要把它暴露到公网。
- 平台管理员邮箱只给信任的人;超管首次登录务必设置强密码。
- 及时更新镜像(base 镜像与依赖),关注 CVE。
- 监控:对接你的告警系统检查 `/healthz`、容器健康状态与磁盘(证书/日志/数据库)。

### 8.13 已知限制

- **Let's Encrypt 额度**:平台默认域名按子域逐个签发,受每注册域名每周 50 张证书(含续期)限制,额度按平台域名聚合;接近上限时需迁移到泛域名证书方案(ADR-0004)。
- **访问保留**:访问记录默认保留 90 天,后台定时清理(`CLOAK_VISIT_RETENTION` 可调)。
- **授权端点**:仅内网可达,未激活域名拒绝签发——防止任意域名解析到本机即触发签发(spec 决策 #8)。
- **前端产物**:生产镜像使用已提交的 `web/dist`,前端改动必须重新构建并提交后部署(见 8.5)。

---

## 9. 故障排查

### 开发环境

| 症状 | 原因 / 处理 |
| --- | --- |
| 浏览器/curl 报证书不受信任 | 未信任本地 CA:执行 5.3;或把 `certs/caddy-root.pem` 导入钥匙串并信任 |
| `curl: (60) SSL certificate problem` | 同上;临时排查可用 `-k` |
| 域名访问 Vite 返回 403 | Vite 的 Host 校验未放行:确认 `web/.env.development` 的 `VITE_PLATFORM_DOMAIN` 与访问域名一致(默认已放行 `.cloak.test`,见 5.2 提示) |
| Caddy 502 Bad Gateway | 后端没在终端启动:确认已执行 5.6 的 `go run` 且监听 8081(`curl -s http://127.0.0.1:8081/healthz`) |
| 注册返回 409 | 邮箱/slug 已被占用(开发库有历史数据):换 slug,或 6.5 重置 |
| 自有域名一直 `pending` | hosts 未加该域名/未指向 127.0.0.1;或未到 5 分钟重试周期,可在后台点「重新校验」 |
| 域名 `failed` | 72 小时未通过校验;检查 hosts、`CLOAK_SERVER_PUBLIC_IP` |
| 跳转 404 | 短码未创建/域名停用/短链停用;或访问的 Host 不是该租户的 active 域名 |
| 邮件收不到、终端也没有 `[CLOAK mailer]` | 启动后端时 export 了 `CLOAK_SMTP_*`,走了真实 SMTP;不 export 即回到控制台 mailer(邮件打印在终端) |
| 后端启动失败/连不上数据库 | 确认 Postgres 已启动(`docker compose up -d postgres`、`docker compose ps`);检查 5.6 的启动命令与 `CLOAK_DATABASE_URL` |
| 修改 Go 代码不生效 | 在启动后端的终端 `Ctrl+C` 后重新 `go run`;开发环境后端不在 Docker 里(见 5.6) |
| 修改前端不生效 | Vite Dev Server 用 5.5;若看的是 Caddy 上的旧页面,需 `pnpm build` 后重建镜像 |
| 8080/443/80 被占用 | 开发 compose 中 Caddy 占用 80/443;openresty(flow-filtering)也绑定 80/443,不要同时启动 |

### 生产环境

| 症状 | 原因 / 处理 |
| --- | --- |
| `docker compose -f docker-compose.prod.yml up` 报 `CLOAK_XXX 必须设置` | `.env` 缺必填变量(compose `:?` 强制);对照 8.3 补全 |
| Caddy 启动失败:`expanding email address ... is empty` | `Caddyfile.prod` 取消了 `email` 指令但 `CLOAK_ACME_EMAIL` 为空;设置变量或注释指令 |
| 证书一直不签发 / `cert_status=failed` | 看 `docker compose logs caddy`;确认 DNS 已生效、80/443 可达、授权端点放行(租户已验证且未封禁) |
| 平台默认域名无法访问 | 泛域名 `*.<平台域名>` 未配置或未生效;`dig` 验证 |
| 自有域名一直 `pending` | 租户的 A/AAAA 未指向 `CLOAK_SERVER_PUBLIC_IP`;worker 每 5 分钟重试,后台可手动 recheck;超 72h 置 `failed` |
| 访问者看到 404 | 域名/短链被停用或删除;或 Host 不匹配 active 域名 |
| 收不到验证/重置邮件 | 检查 `.env` SMTP 配置、`docker compose logs backend`;未配置 SMTP 时链接打印在日志 |
| 部署后页面还是旧的 | 前端改了但没重新 `pnpm build` 并提交 dist(见 8.5);重建镜像后再试 |
| 磁盘空间不足 | 日志与镜像堆积:`docker system prune`(谨慎)、清理旧镜像;`pgdata` 只增,关注访问量 |

---

## 10. API 概览

完整契约见 [`.scratch/cloak/api-contract.md`](.scratch/cloak/api-contract.md)。要点:

**认证方式**

- 后台 API:会话 cookie `cloak_session`(HTTP-only / Secure / SameSite=Lax);写方法需在请求头附 `X-CSRF-Token`(值来自 `cloak_csrf` cookie,双提交 token)。
- 跳转路径:`GET /{code}`,由 Host 决定域名,无需鉴权。
- 内部端点:`GET /internal/caddy/authorize?domain=<fqdn>`,仅内网可达。

**主要端点**

| 分组 | 端点 |
| --- | --- |
| 认证 | `POST /api/auth/register`、`verify-email`、`login`、`logout`、`GET /api/auth/me`、`POST /api/auth/change-password`、`forgot-password`、`reset-password` |
| 域名 | `GET/POST /api/domains`、`GET /api/domains/{id}`、`POST /api/domains/{id}/recheck`、`PATCH /api/domains/{id}`、`DELETE /api/domains/{id}` |
| 短链 | `GET/POST /api/links`、`GET/PATCH/DELETE /api/links/{id}`、`POST /api/links/{id}/purge`、`GET /api/links/{id}/visits`、`GET /api/links/{id}/stats` |
| 租户设置 | `GET/PATCH /api/me` |
| 平台管理 | `GET /api/admin/tenants`、`GET/PATCH /api/admin/tenants/{id}`、`DELETE /api/admin/domains/{id}` |
| 跳转 | `GET /{code}`(公开) |

**统一错误响应**

```json
{ "code": "E_DOMAIN_LIMIT", "message": "域名数量已达上限 (5/10)", "details": {} }
```

---

## 11. 相关文档

| 文档 | 内容 |
| --- | --- |
| [`CONTEXT.md`](CONTEXT.md) | 领域词汇表(租户/短链/域名/激活等术语定义) |
| [`docs/deploy.md`](docs/deploy.md) | 生产部署精简指南与上线检查清单 |
| [`web/README.md`](web/README.md) | 前端技术栈、页面清单、认证/请求约定 |
| [`.scratch/cloak/api-contract.md`](.scratch/cloak/api-contract.md) | API 完整契约 |
| [`.scratch/cloak/spec.md`](.scratch/cloak/spec.md) | 需求规格、决策、测试决策 |
| [`docs/adr/`](docs/adr/) | 架构决策记录(Go 后端、Caddy on-demand TLS、Vben Admin UI、默认域名证书方案) |
| `.scratch/cloak/issues/` | 逐功能票据(01~15),含实现与验证记录 |
