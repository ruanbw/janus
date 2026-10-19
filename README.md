<div align="center">

# Janus

**每一次跳转，皆为裁决。**
*Every jump is a verdict.*

[![license](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)
[![node](https://img.shields.io/badge/Node-%E2%89%A520.19-339933?logo=nodedotjs&logoColor=white)](web/package.json)
[![postgres](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)](docker-compose.yml)

[中文](README.md) · [English](README.en.md) · [产品介绍页](https://ruanbw.github.io/janus/)

</div>

---

## 这是什么

Janus 是一个自托管的**多租户短链服务**。租户注册后管理自己的域名与短链,系统为每个已激活域名自动签发并续期 HTTPS 证书,把 `域名/短码` 的访问重定向到目标 URL。

除跳转外还提供:落地页型短链、访问明细与地理统计、租户级访问处置规则引擎、自定义错误页,以及供脚本调用的 Bearer JWT API。

- **单服务器部署** — Go(Gin + GORM)+ Vue 3 SPA(nginx)+ PostgreSQL 16 + Caddy(on-demand TLS),`docker compose up -d` 全部跑起来。
- **多租户隔离** — 域名、短链、规则、落地页在租户之间完全隔离;部署者持有平台管理员角色。
- **自带证书** — 平台域名配一条泛解析,每个租户自动获得 `<slug>.<平台域名>`;自有域名激活后由 Caddy 自动签发 Let's Encrypt 证书。
- **前后端分离** — 前端独立构建为 nginx 镜像,后端镜像内无任何前端文件(ADR-0006)。

```
Internet → Caddy(80/443, on-demand TLS)
             ├── /            → nginx(前端 SPA)
             └── /api/*, /{code}  → Go 后端 → PostgreSQL 16
```

## 仓库结构

```
cmd/janus/            # 后端入口(加载配置 → 连库 → 初始化脚本 → worker → HTTP)
internal/             # httpapi(routes/handlers)、rules(规则引擎)、store、domain、geo、rbac…
migrations/init.sql   # 唯一 schema 事实来源,启动时自动幂等执行
web/                  # 前端 SPA(Vue 3 + Vite + Tailwind 4)
docker/               # Dockerfile(backend / web 两个目标)+ nginx.conf
docker-compose.yml    # 开发编排(Postgres + Caddy)
docker-compose.prod.yml, Caddyfile.prod  # 生产编排
docs/                 # deploy.md、testing.md、adr/
```

## 环境要求

| 工具 | 版本 |
| --- | --- |
| Docker Desktop(含 Compose v2) | 较新版本 |
| Go | ≥ 1.26 |
| Node.js | ≥ 20.19 |
| pnpm | ≥ 9(仓库锁定 10.x) |
| Caddy CLI(可选) | ≥ 2.x,用于信任本地 CA |

开发环境**基础设施进 Docker**(Postgres、Caddy),**后端与前端在终端跑**,改代码即时生效。

## 快速开始(开发)

### 1. 启动基础设施

```bash
# 从模板生成 .env,并把必填的 on-demand TLS 密钥直接填进去
sed "s/^JANUS_CADDY_ASK_TOKEN=.*/JANUS_CADDY_ASK_TOKEN=$(openssl rand -hex 32)/" .env.example > .env
docker compose up -d    # postgres:5432、caddy:80/443
```

### 2. 本地域名解析

```text
127.0.0.1 app.janus.test
127.0.0.1 alice.janus.test      # 每注册一个租户加一行,hosts 不支持通配符
```

### 3. 信任 Caddy 本地 CA

```bash
mkdir -p certs
docker compose exec caddy cat /data/caddy/pki/authorities/local/root.crt > certs/caddy-root.pem
sudo caddy trust --ca certs/caddy-root.pem
```

### 4. 启动后端

```bash
JANUS_COOKIE_SECURE=false JANUS_ADDR=:8080 go run ./cmd/janus
```

`go run` **不会**读 `.env`(那是 compose 的行为),需要覆盖配置就在命令行 export。

### 5. 启动前端

```bash
cd web && pnpm install && pnpm dev     # http://localhost:5173,/api 代理到 :8080
```

### 6. 验证

```bash
curl -s  http://127.0.0.1:8080/healthz     # → {"status":"ok"}
curl -sk https://app.janus.test/healthz    # 经 Caddy 走通全链路
```

浏览器打开 `http://localhost:5173` 注册账号;未配置 SMTP 时,验证邮件直接打印在后端终端。

> Caddy 占用宿主 80/443,与本机其他绑定这两个端口的服务错开启动。

## 配置

全部配置走环境变量,模板与逐项说明见 [`.env.example`](.env.example),默认值可直接用于开发。常用项:

| 变量 | 开发默认值 | 说明 |
| --- | --- | --- |
| `JANUS_ADDR` | `:8080` | HTTP 监听地址 |
| `JANUS_PLATFORM_DOMAIN` | `janus.test` | 平台裸域名,租户默认域名 `<slug>.<平台域名>` |
| `JANUS_SERVER_PUBLIC_IP` | `127.0.0.1` | 自有域名 DNS 激活校验的比对地址 |
| `JANUS_PUBLIC_BASE_URL` | `https://app.janus.test` | 邮件中验证 / 重置链接前缀 |
| `JANUS_DATABASE_URL` | `postgres://janus:janus@localhost:5432/janus?sslmode=disable` | 后端连接串 |
| `JANUS_SUPERADMIN_EMAIL` | 空 | 超管邮箱,启动时幂等初始化;留空则无超管 |
| `JANUS_COOKIE_SECURE` | `false` | 会话 cookie Secure 标记,http 开发必须 false |
| `JANUS_JWT_SECRET` | 空 | JWT 签名密钥;留空则每次启动随机生成(重启后 token 失效) |
| `JANUS_CADDY_ASK_TOKEN` | 空(**必填**) | on-demand TLS 授权端点共享密钥,compose 与后端同值注入 |
| `JANUS_DNS_RETRY_INTERVAL` / `JANUS_DNS_MAX_AGE` | `5m` / `72h` | 域名激活重试间隔 / 最长等待 |
| `JANUS_VISIT_RETENTION` | `2160h` | 访问记录保留时长(90 天) |
| `JANUS_SMTP_*` | 空 | 配置 `HOST` 才真实发信,否则打印到后端日志 |

> `.env` 含数据库密码与 SMTP 凭据,已在 `.gitignore` 中。

## 生产部署

完整步骤与上线检查清单见 **[`docs/deploy.md`](docs/deploy.md)**。最小流程:

```bash
cp .env.example .env && chmod 600 .env     # 填 JANUS_PLATFORM_DOMAIN / JANUS_SERVER_PUBLIC_IP / JANUS_DB_PASSWORD 等
sed -i 's/example\.com/YOUR-DOMAIN/g' Caddyfile.prod   # 站点地址不支持环境变量
docker compose -f docker-compose.prod.yml up -d --build
```

前置条件是 DNS:平台裸域名与泛解析 `*.<平台域名>` 均指向服务器公网 IP。首次启动自动执行初始化脚本、创建免费档种子、初始化超管;证书按需签发。

## 开发约定

- **提交前**:后端 `gofmt -l internal cmd`、`go vet ./...`、定向 `go test -count=1 ./internal/<pkg>/`;前端 `pnpm type-check` + `pnpm build` + `pnpm check:ui`。
- **测试即规格**:需求以 Given-When-Then 测试沉淀,日常只跑被改动包的定向用例,不跑 `go test ./...`。详见 [`docs/testing.md`](docs/testing.md)。
- **数据库**:`migrations/init.sql` 是唯一 schema 事实来源,不写增量迁移;改表直接改脚本并 DROP 开发库重建。
- **新增受保护路由**:必须同步 `internal/rbac/rbac.go` 的 `policyText`,漏了会被 403。
- **架构取舍**:写 `docs/adr/NNNN-*.md`(背景 → 权衡 → 后果)。

## 文档

| 文档 | 内容 |
| --- | --- |
| [`docs/deploy.md`](docs/deploy.md) | 生产部署指南与上线检查清单 |
| [`docs/testing.md`](docs/testing.md) | 测试流程与分层验证 |
| [`web/README.md`](web/README.md) | 前端技术栈、页面清单、请求约定 |
| [`web/UI_KIT.md`](web/UI_KIT.md) | 前端组件契约与三层设计令牌 |
| [`docs/adr/`](docs/adr/) | 架构决策记录 |

## 许可证

**GNU Affero General Public License v3.0(AGPL-3.0)**,全文见 [`LICENSE`](LICENSE)。

```
Copyright (C) 2026 ruanbw and Janus contributors
SPDX-License-Identifier: AGPL-3.0-only
```

可自由使用、修改、再分发,包括闭源商用;但通过网络向用户提供本程序功能(部署成在线服务)时,必须向这些用户提供对应源码。商业授权 / 闭源分发需求请单独联系作者。
