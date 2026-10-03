# Janus 生产部署指南

自托管、多租户短链服务。单台服务器:Go 后端(RESTful API)+ Postgres + nginx(前端静态服务)+ Caddy 前置(on-demand TLS,Let's Encrypt 自动签发/续期)。前后端分离(见 ADR-0006):前端为独立构建的 nginx 服务,后端镜像内无任何前端文件,两者独立发版;生产环境为多容器编排,不存在单二进制部署形态。

## 1. 前置条件

- 一台公网服务器,开放 **80/443** 端口;
- 一个域名(下称**平台域名**,例如 `example.com`);
- DNS 配置(部署前完成,等待生效):
  - **泛域名解析**:`*.<平台域名>` → 服务器公网 IP(A/AAAA 记录)—— 所有租户子域由此生效(spec 决策 #13);
  - **裸平台域名**:`<平台域名>` → 服务器公网 IP(承载后台);
  - 自有域名由租户自行解析(添加域名时系统校验其 A/AAAA 是否包含本服务器 IP)。

## 2. 环境变量(复制 `.env.example` 为 `.env` 后填写)

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `JANUS_PLATFORM_DOMAIN` | ✅ | 裸平台域名,如 `example.com`。租户默认域名形如 `<slug>.<平台域名>` |
| `JANUS_SERVER_PUBLIC_IP` | ✅ | 服务器公网 IP,DNS 激活校验比对地址 |
| `JANUS_DB_PASSWORD` | ✅ | Postgres 密码 |
| `JANUS_CADDY_ASK_TOKEN` | ✅ | Caddy on-demand TLS 授权端点 `/internal/caddy/authorize` 的共享密钥,建议 `openssl rand -hex 32`。compose 以 `${JANUS_CADDY_ASK_TOKEN:?}` 同时注入 backend 与 caddy 两个容器;留空则 `docker compose up -d` 直接报错退出 |
| `JANUS_SUPERADMIN_EMAIL` | 建议 | 平台管理员邮箱(环境变量初始化,首次登录引导设置密码) |
| `JANUS_ACME_EMAIL` | 建议 | Let's Encrypt 账户邮箱 |
| `JANUS_DB_USER` / `JANUS_DB_NAME` | 可选 | 默认 `janus` / `janus` |
| `JANUS_SESSION_TTL` / `JANUS_SESSION_TTL_SHORT` | 可选 | 会话有效期,默认 30 天 / 24 小时 |

其余可选变量见 `.env.example`(token 有效期、DNS 重试、访问保留时长等)。

> **部署前必须修改 Caddyfile.prod**:Caddy 站点地址不支持环境变量占位符,该文件把站点域名硬编码为 `example.com`(注释中已标注),需要把 `example.com` 全部替换为你的平台域名(与 `JANUS_PLATFORM_DOMAIN` 一致)。`JANUS_ACME_EMAIL` 仅在取消注释 Caddyfile.prod 中的 `email` 指令后生效(为空时该指令会导致 Caddy 启动失败)。

## 3. 启动

```bash
cp .env.example .env   # 填写生产值
docker compose -f docker-compose.prod.yml up -d --build
```

- 首次启动自动执行数据库迁移、创建免费档种子、按 `JANUS_SUPERADMIN_EMAIL` 初始化超管;
- 证书按需签发:租户域名激活后,访问者首次访问时 Caddy 询问授权端点并签发 Let's Encrypt 证书,到期前自动续期(ADR-0002、ADR-0004);
- 查看状态:`docker compose -f docker-compose.prod.yml ps`、`docker compose -f docker-compose.prod.yml logs -f caddy`。

## 4. 上线前检查清单

按顺序在**生产环境**走通(与 spec Testing Decisions 一致):

- [ ] `curl -k https://<平台域名>/healthz` 返回 `{"status":"ok"}`
- [ ] 浏览器访问 `https://<平台域名>` 打开后台,证书为 Let's Encrypt 签发(非本地 CA)
- [ ] 注册新租户(提交 email/password/slug),收到验证邮件(需配置真实 SMTP,见第 6 节;未配置时验证链接打印在后端日志)
- [ ] 邮箱验证后租户转 active,`{slug}.<平台域名>` 默认域名可承载短链
- [ ] 通过 `https://{slug}.<平台域名>/<短码>` 访问短链,返回 302/301 跳转,访问计数增长
- [ ] 添加自有域名:解析指向本服务器后自动激活并签发证书;未指向时状态 `pending`、超时 `failed`
- [ ] 超管登录后可见租户列表,可封禁/解封、调整等级、移除违规域名
- [ ] 停用短链/域名后访问返回 404;配额超限时创建被拒并提示用量/上限

> 上线前先在**开发环境**按清单走通并完成测试(见 docs/testing.md)，再切生产。

## 5. 开发 / 生产差异

| 维度 | 开发 | 生产 |
| --- | --- | --- |
| 编排 | 基础设施 Docker(`docker compose up -d`:postgres + caddy);后端/前端终端启动(`go run` + `pnpm dev`,见根 README §5) | `docker compose -f docker-compose.prod.yml up -d`(全部容器化) |
| 端口 | 后端 8080;Caddy 映射宿主 443/80(无端口访问) | 标准 80/443;后端不暴露公网 |
| DNS | SwitchHosts 把 `app.janus.test` 与测试租户子域指向 127.0.0.1(`JANUS_SERVER_PUBLIC_IP=127.0.0.1`,Go 读 /etc/hosts 走真实代码路径) | 真实 DNS 泛解析 `*.<平台域名>` |
| 证书 | Caddy 本地 CA(`tls internal` + `on_demand_tls`),`caddy trust` 信任根证书 | Let's Encrypt(ACME 自动签发/续期) |
| 邮件 | 控制台假 mailer(验证/重置链接打印在后端日志) | 真实 SMTP(部署者提供凭据;mailer 可插拔,见 spec 决策 #3) |
| Cookie | `JANUS_COOKIE_SECURE=false`(http) | `JANUS_COOKIE_SECURE=true`(https,必须) |
| 平台域名 | `janus.test`(RFC 保留测试域) | 真实域名 |

## 6. SMTP 配置(邮件发送)

mailer 为可插拔实现(spec 决策 #3):未配置 SMTP 时使用控制台假 mailer(邮件内容打印到后端日志)。生产接入真实 SMTP 时,在 `.env` 配置:

| 变量 | 说明 |
| --- | --- |
| `JANUS_SMTP_HOST` | SMTP 服务器地址,如 `smtp.example.com` |
| `JANUS_SMTP_PORT` | 默认 `465`(隐式 TLS);`587` 必须支持 STARTTLS,否则报错(拒绝明文 AUTH) |
| `JANUS_SMTP_USERNAME` / `JANUS_SMTP_PASSWORD` | 认证凭据;用户名留空则不发送 AUTH |
| `JANUS_SMTP_FROM` | 发件人地址;留空回退为 Username,两者都空则发送报错 |

compose 会把上述变量转发给后端容器。

## 7. 已知限制

- 平台默认域名按子域逐个签发,受 Let's Encrypt 每注册域名每周 50 张证书限制,额度按平台域名聚合;接近上限时迁移到泛域名证书方案(ADR-0004);
- 访问记录默认保留 90 天,后台定时清理;
- 授权端点仅内网可达(`/internal/caddy/authorize`),未激活域名拒绝签发,防止任意域名解析到本机即触发签发(spec 决策 #8)。

## 8. IP 地理与情报服务配置(可选)

平台默认内嵌离线 ip2region 库(支持 IPv4 + IPv6),零网络依赖、零外部调用,作为兜底实现。若需更高精度的机房/代理识别或 ASN 数据,可接入外部商业 SaaS 厂商(ADR-0009):

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `JANUS_GEO_PROVIDER` | `local` | 数据源: `local`(仅内嵌库)、`ipinfo`、`ipqualityscore`、`ipapi` |
| `JANUS_GEO_API_KEY` | 空 | 对应 SaaS 厂商的 API Token / Key |
| `JANUS_GEO_TIMEOUT` | `500ms` | 单次外部查询超时上限,超时自动降级至本地离线库 |

在启用 SaaS 厂商时,系统内部提供分片双代缓存并记住负结果,保护外部调用配额;当网络故障、接口报错或超时时,将自动毫秒级降级至本地离线库,保证短链跳转服务的高可用与低延迟。
