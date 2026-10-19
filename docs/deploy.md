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
| `JANUS_DB_PASSWORD` | ✅ | Postgres 密码。注意 `.env.example` 预置的 `janus` 能直接启动(compose 只挡空值,不挡默认值),不会报错 |
| `JANUS_CADDY_ASK_TOKEN` | ✅ | Caddy on-demand TLS 授权端点 `/internal/caddy/authorize` 的共享密钥,建议 `openssl rand -hex 32`。compose 以 `${JANUS_CADDY_ASK_TOKEN:?}` 同时注入 backend 与 caddy 两个容器;留空则 `docker compose up -d` 直接报错退出 |
| `JANUS_TRUSTED_PROXY_CIDRS` | ✅ | 声明「哪些对端的 X-Forwarded-For 可以采信」,生产填 Caddy 所在容器网段(Docker 默认网桥为 `172.16.0.0/12`)。compose 以 `${...:?}` 转发,留空直接启动失败;不配而强行走起来的后果:认证端点限流退化成**全站共享一个桶**(一个人试密码全站吃 429)、落地页 SDK 生成的 click 地址降级为 `http://` |
| `JANUS_JWT_SECRET` | ✅ | API Bearer JWT 签名密钥,`openssl rand -hex 32`(后端要求 ≥ 32 字节,短于 32 字节直接拒绝启动)。留空只告警、不拒绝启动,但每次重启换随机密钥 → 已签发的 API token 全部失效 |
| `JANUS_SUPERADMIN_EMAIL` | 建议 | 平台管理员邮箱(环境变量初始化,首次登录引导设置密码)。邮箱已被普通租户占用时启动**硬失败**(拒绝提权),修正后重启 |
| `JANUS_ACME_EMAIL` | 建议 | Let's Encrypt 账户邮箱 |
| `JANUS_DB_USER` / `JANUS_DB_NAME` | 可选 | 默认 `janus` / `janus` |
| `JANUS_SESSION_TTL` / `JANUS_SESSION_TTL_SHORT` | 可选 | 会话有效期,默认 30 天 / 24 小时 |

其余可选变量见 `.env.example`(token 有效期、DNS 重试、访问保留时长等)。

> **部署前必须修改 Caddyfile.prod**:Caddy 站点地址不支持环境变量占位符,该文件把站点域名硬编码为 `example.com`(注释中已标注),需要把 `example.com` 全部替换为你的平台域名(与 `JANUS_PLATFORM_DOMAIN` 一致)。`JANUS_ACME_EMAIL` 仅在取消注释 Caddyfile.prod 中的 `email` 指令后生效(为空时该指令会导致 Caddy 启动失败)。
>
> 前端注册页的默认域名提示是**构建期**烘进产物的(`VITE_PLATFORM_DOMAIN`):compose 会把 `.env` 的 `JANUS_PLATFORM_DOMAIN` 作为 `--build-arg` 自动传进去,无需手工干预;但**改了平台域名后必须重建 web 镜像**(`docker compose -f docker-compose.prod.yml build web`),否则注册页仍提示旧域名。

## 3. 启动

```bash
cp .env.example .env   # 填写生产值(必填项见第 2 节)
docker compose -f docker-compose.prod.yml up -d --build
```

- 首次启动自动执行数据库初始化脚本、创建免费档种子、按 `JANUS_SUPERADMIN_EMAIL` 初始化超管;
- 证书按需签发:租户域名激活后,访问者首次访问时 Caddy 询问授权端点并签发 Let's Encrypt 证书,到期前自动续期(ADR-0002、ADR-0004);
- 查看状态:`docker compose -f docker-compose.prod.yml ps`、`docker compose -f docker-compose.prod.yml logs -f caddy`

### 首次登录(平台管理员)

`JANUS_SUPERADMIN_EMAIL` 对应的账号在首次启动时被创建为「无密码」状态。首登需要一枚一次性
**setup token**(邮件发送;未配 SMTP 时打印在后端日志)。

#### 浏览器流程

首登被拒时后端会发一封含一次性 **setup token** 的邮件(未配 SMTP 时打印在后端日志),两条路都能走完:

**A. 直接点邮件里的链接(最短)**:链接是 `/reset-password?token=...`,页面里设置新密码即可;
重置端点同时接受 `reset` 与 `setup` 两种用途的 token(`internal/httpapi/auth.go` 的 `resetConsumableKinds`),
设好密码后直接回登录页用「邮箱 + 密码」登录。

**B. 在登录页填校验码**:

1. 打开 `https://<平台域名>`,用超管邮箱登录(账号还没有密码,密码随便填);
2. 首登被拒后页面出现**「首次登录校验码」**输入框,同时自动发出一封邮件;
3. 从邮箱取到校验码填进去,再点一次登录;
4. 登录成功后自动进入 `/account` 设置初始密码。

取不到邮件(未配 SMTP)时,校验码与链接就在容器日志里:

```bash
docker compose -f docker-compose.prod.yml logs backend | grep -A3 'JANUS mailer' | tail -6
```

> 同一邮箱 15 分钟最多发一封;丢了就在登录页重新提交一次会补发。

#### 无浏览器时(API 流程)

```bash
BASE=https://<平台域名>
EMAIL=<JANUS_SUPERADMIN_EMAIL 的值>

# 1) 触发 setup 邮件。返回 401 是预期结果(响应会说明 token 已发出)
curl -s -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"placeholder\"}"

# 2) 取 token:配了 SMTP 就看邮箱;没配 SMTP 就在后端容器日志里(ConsoleMailer)
docker compose -f docker-compose.prod.yml logs backend | grep -A3 'JANUS mailer' | tail -6

# 3) 带 token 登录,拿到会话 cookie(这一步只发会话,不会写入密码)
curl -s -c cookies.txt -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"placeholder\",\"setupToken\":\"<token>\"}"

# 4) 用会话设置密码(首登允许省略旧密码;CSRF 取自 janus_csrf cookie)
CSRF=$(awk '$6=="janus_csrf"{print $7}' cookies.txt)
curl -s -b cookies.txt -X POST "$BASE/api/auth/change-password" \
  -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
  -d '{"newPassword":"<新密码>"}'
```

设好密码后即可删除 `cookies.txt`。若第 3 步一直失败,先确认 `JANUS_SUPERADMIN_EMAIL` 与 token 属于同一个邮箱,并且 token 未被用过。

## 4. 上线前检查清单

按顺序在**生产环境**走通:

- [ ] `curl https://<平台域名>/healthz` 返回 `{"status":"ok"}`(不加 `-k`:证书有问题就该在这里暴露;下一条再确认签发方是 Let's Encrypt)
- [ ] 浏览器访问 `https://<平台域名>` 打开后台,证书为 Let's Encrypt 签发(非本地 CA)
- [ ] 注册新租户(提交 email/password/slug),收到验证邮件(需配置真实 SMTP,见第 6 节;未配置时验证链接打印在后端日志)
- [ ] 邮箱验证后租户转 active,`{slug}.<平台域名>` 默认域名可承载短链
- [ ] 通过 `https://{slug}.<平台域名>/<短码>` 访问短链,返回 302/301 跳转,访问计数增长
- [ ] 添加自有域名:解析指向本服务器后自动激活并签发证书;未指向时状态 `pending`、超时 `failed`
- [ ] 超管登录后可见租户列表,可封禁/解封、调整等级、移除违规域名(首登见第 3 节)
- [ ] 停用短链/域名后访问返回 404;配额超限时创建被拒并提示用量/上限

> 清单里的**自有域名激活、真实 SMTP 发信、Let's Encrypt 签发**三项只能在生产验证:开发环境用 Caddy 本地 CA,
> 自有域名的归属校验走真实 DNS(绕开 /etc/hosts)、证书探活刻意拒绝 loopback(见第 5 节)。
> 开发环境能先走的是 1、5、8 的等价路径(域名换成 `app.janus.test` / `<slug>.janus.test`,邮件从后端日志里取链接)。

## 5. 开发 / 生产差异

| 维度 | 开发 | 生产 |
| --- | --- | --- |
| 编排 | 基础设施 Docker(`docker compose up -d`:postgres + caddy);后端/前端终端启动(`go run` + `pnpm dev`,见根 README「快速开始」) | `docker compose -f docker-compose.prod.yml up -d`(全部容器化) |
| 端口 | 后端 8080;Caddy 映射宿主 443/80(无端口访问) | 标准 80/443;后端不暴露公网 |
| DNS | 用 hosts/SwitchHosts 把 `app.janus.test` 与测试租户子域指向 127.0.0.1(配合 `JANUS_SERVER_PUBLIC_IP=127.0.0.1`);平台默认子域建号即 active,**不做** DNS 校验 | 真实 DNS:裸平台域名与泛解析 `*.<平台域名>` 都指向本服务器 |
| 证书 | Caddy 本地 CA(`tls internal` + `on_demand_tls`),`caddy trust` 信任根证书 | Let's Encrypt(ACME 自动签发/续期) |
| 邮件 | 控制台假 mailer(验证/重置链接打印在后端日志) | 真实 SMTP(部署者提供凭据;mailer 可插拔,见 spec 决策 #3) |
| Cookie | `JANUS_COOKIE_SECURE=false`(http) | `JANUS_COOKIE_SECURE=true`(https,必须) |
| 平台域名 | `janus.test`(RFC 保留测试域) | 真实域名 |

> **开发环境演练不了自有域名激活。** 归属校验是 TXT + A/AAAA 真实 DNS 查询,实现刻意绕开 `/etc/hosts`
> (`internal/domain/dns.go`);证书探活还显式拒绝 loopback / 私网 / 链路本地地址(`internal/domain/cert.go`)。
> 所以 hosts 加行只对**平台默认子域**有用,「自有域名 → 激活 → 自动签发 → 跳转」这条链路只能上生产验证。

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
- 授权端点仅内网可达(`/internal/caddy/authorize`),未激活域名拒绝签发,防止任意域名解析到本机即触发签发(spec 决策 #8);
- 超管首登用的 setup token 与普通重置的 reset token 同性质(都证明能读到该邮箱,
  一次性且限期),因此重置端点两者都收;能读该邮箱的人就能完成首登或改密 —— 这是设计,不是缺陷。

## 8. 产品介绍页(GitHub Pages)

`site/` 是一份自包含的静态产品介绍页(单文件 HTML,无构建步骤、无外部依赖),由 `.github/workflows/pages.yml` 发布到 GitHub Pages:

- 线上地址:<https://ruanbw.github.io/janus/>
- 触发条件:`main` 分支上 `site/**` 或该 workflow 自身发生变化;也可在 Actions 页面手动 `Run workflow`;
- 首次启用需在仓库 Settings → Pages 把 Source 设为 **GitHub Actions**(仅靠 workflow 内 `configure-pages` 的 `enablement` 自动开启,可能因 `GITHUB_TOKEN` 权限不足而失败);
- 与运行时完全解耦:改 `site/` 不会重建任何服务镜像,产物也不经过后端或 nginx。

## 9. 升级与回滚

```bash
git pull                                                  # 或 checkout 到目标 tag/commit
docker compose -f docker-compose.prod.yml up -d --build    # 重建 backend / web,Postgres 不动
```

- **每次启动都会整段重跑 `migrations/init.sql`**(没有版本表;脚本本身幂等)。改表靠改脚本 + 重建库,不是增量迁移。
- 只改前端 / 只改后端:`... up -d --build web` 或 `... up -d --build backend`,两个镜像互不依赖(ADR-0006)。
- 数据卷(`pgdata` / `caddy_data` / `uploads`)不随重建丢失。
- **回滚**:`git checkout <旧 commit>` 后重复上面的 `up -d --build`。先备份(第 10 节)——若新版本改过
  `init.sql` 且与旧代码不兼容,回滚后要恢复数据库,否则旧代码会读到新 schema。

## 10. 备份与恢复

有状态数据全在三个具名卷里,卷名前缀是 compose 项目名(默认 = 仓库目录名,如 `janus_pgdata`):

| 卷 | 内容 | 丢失后果 |
| --- | --- | --- |
| `*_pgdata` | 租户、域名、短链、规则、访问记录、会话 | 业务数据全部丢失 |
| `*_caddy_data` | ACME 账户与已签发证书 | **全部域名重签**;平台域名受 Let's Encrypt「每注册域名每周 50 张」限额约束(见第 7 节),高峰期可能当天签不回来 |
| `*_uploads` | 上传的落地页文件 | 落地页 404 |

数据库(建议每天 cron,备份文件再同步到机器之外):

```bash
docker compose -f docker-compose.prod.yml exec -T postgres \
  sh -c 'pg_dump -U "$POSTGRES_USER" -Fc "$POSTGRES_DB"' > "janus-$(date +%F).dump"
```

恢复(先停 backend,避免写入竞争):

```bash
docker compose -f docker-compose.prod.yml stop backend
docker compose -f docker-compose.prod.yml exec -T postgres \
  sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists' < janus-YYYY-MM-DD.dump
docker compose -f docker-compose.prod.yml start backend
```

证书与上传文件:

```bash
docker volume ls | grep -E 'pgdata|caddy_data|uploads'    # 先确认卷名
docker run --rm -v janus_caddy_data:/data:ro -v "$PWD":/backup alpine tar czf /backup/caddy_data.tgz -C /data .
```

## 11. 故障排查

先看三处:`docker compose -f docker-compose.prod.yml ps`、`... logs backend`、`... logs caddy`。

| 症状 | 原因 | 处理 |
| --- | --- | --- |
| `up` 直接报 `required variable ... is missing a value` | 必填项为空,compose 用 `${VAR:?}` 硬卡 | 按第 2 节填 `.env` 后重试 |
| 后端反复重启,日志 `invalid config: JANUS_JWT_SECRET 至少需要 32 字节` | 密钥太短 | `openssl rand -hex 32` 重新生成 |
| 后端日志 `init superadmin: ... already taken by a non-superadmin tenant` | 该邮箱已被公开注册抢注 | 换一个不可猜的 `JANUS_SUPERADMIN_EMAIL` 后重启(系统拒绝自动提权,这是设计) |
| 浏览器打不开后台、任何域名握手都失败,Caddy 侧看不到明显错误 | 两个容器的 ask token 不一致 | `... exec caddy printenv CADDY_ASK_TOKEN` 与 `... exec backend printenv JANUS_CADDY_ASK_TOKEN` 比对;后端打印 `JANUS_CADDY_ASK_TOKEN 未配置` 说明根本没注入 |
| 单个租户域名握不了手,后端日志 403 `domain not authorized` | 域名不是 active / 租户被封 / DNS 还没指过来 | 后台看域名状态;`pending` 等 DNS 生效(上限 `JANUS_DNS_MAX_AGE`) |
| 登录、注册大面积 429,像「一个人试错全站受限」 | `JANUS_TRUSTED_PROXY_CIDRS` 没覆盖 Caddy 网段,认证限流退化成全站共享一个桶 | 填实际网段:`docker network inspect <项目名>_default` 看 Subnet(默认在 `172.16.0.0/12` 内) |
| 落地页 SDK 生成的 click 地址是 `http://` | 同上,`X-Forwarded-Proto` 不被采信 | 同上 |
| Caddy 起不来,日志 `expanding email address ... is empty` | 取消了 `Caddyfile.prod` 里 `email` 的注释,但 `JANUS_ACME_EMAIL` 为空 | 二选一:填邮箱,或把 `email` 那行重新注释 |
| 注册页提示 `alice.janus.test`,但平台不是这个域名 | 提示文案是构建期烘进产物的 | 改过 `JANUS_PLATFORM_DOMAIN` 就重建 web 镜像(第 2 节) |
| 证书签不出来,或 `address already in use` | 宿主 80/443 被别的 nginx/apache 占着 | 停掉占用者,让 Caddy 独占 80/443 |
| ask 请求没打到 backend(疑似超时/5xx) | 容器继承了宿主的 `HTTP(S)_PROXY` | 确认 `NO_PROXY` 含 `backend`;不要把宿主代理变量传进容器 |
