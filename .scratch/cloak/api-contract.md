# CLOAK API 契约(v1)

后端、后台前端均以实现此契约为准。所有请求/响应为 JSON。

统一错误响应:
```json
{ "code": "E_DOMAIN_LIMIT", "message": "域名数量已达上限 (5/10)" }
```
`details`(可选)补充字段级信息。

## 认证方式

- **后台 API(管理端)**:会话 cookie `cloak_session`(HTTP-only / Secure / SameSite),CSRF 用双提交 token 请求头 `X-CSRF-Token`。未登录访问受保护端点 → 401。
- **跳转路径**:`GET /{code}`(路径首段为短码),由 Host 决定域名,无需鉴权。
- **内部端点**:`GET /internal/caddy/authorize?domain=<fqdn>`,仅内网可达;放行 200,拒绝 403。

## 枚举

- `tenant.status`: `pending` | `active` | `banned`
- `domain.status`: `pending` | `active` | `failed` | `stopped`
- `domain.origin`: `self` | `platform`
- `domain.certStatus`: `pending` | `issued` | `failed`
- `link.status`: `enabled` | `disabled`
- `link.redirectStatus`: `"302"` | `"301"`
- `link.linkType`: `"redirect"` | `"landing"`(默认 "redirect")
- `link.landingSource`: `"url"` | `"upload"`(默认 "url",仅 landing 型有意义)
- `tier`: `{ id, name, maxLinks, maxDomains }`

## 资源形状(要点)

- `tenant`: `{ id, email, slug, status, isSuperAdmin, codeLength, tier, defaultDomain: "<slug>.<平台域名>", createdAt, firstLoginSetup?, usage? }`(`firstLoginSetup` 仅超管首次登录(尚无密码)时为 true;`usage` 见 /api/me)
- `domain`: `{ id, fqdn, description, origin, status, certStatus, activatedAt, createdAt }`(`description` 为创建时填写的备注,可空,最长 200 字)
- `config`: `{ serverIp, platformDomain, usage }`(`serverIp` 为 `CLOAK_SERVER_PUBLIC_IP`,DNS 校验指向地址;`usage` 为当前租户配额用量,按租户返回)
- `link`: `{ id, code, targetUrls: string[], redirectStatus, linkType, landingSource, landingUrl, status, domains: [fqdn...], visits, clicks, landingUploaded, createdAt }`(列表默认不含逻辑删除项)
  - `landingUrl`:仅 landing+url 来源非空;`clicks`:点击计数,仅 landing 型增长(redirect 型恒 0);`landingUploaded`:landing+upload 来源且已成功上传 zip 时为 true
- `visit`: `{ id, linkId, domain, ip, userAgent, referer, createdAt }`(`ip` 为访问者 IP:部署前置 Caddy 时取 `X-Forwarded-For` 首段,否则取 `RemoteAddr`;旧记录为空字符串)

## 端点

### 认证(后台,会话)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| POST | /api/auth/register | `{email, password, slug}` | 201 tenant | 400 slug 非法/密码过短;409 邮箱或 slug 已占用;创建租户(pending)+ 平台默认域名 |
| POST | /api/auth/verify-email | `{token}` | 200 | 无效 token 400;成功后租户 active,触发默认域名证书预签发 |
| POST | /api/auth/login | `{email, password, rememberMe?}` | 200 tenant + Set-Cookie | 401 凭证错误或未验证;403 已封禁;`rememberMe` 省略或 true → 会话 30 天,false → 24 小时(契约调整) |
| POST | /api/auth/logout | - | 204 | 需 X-CSRF-Token |
| GET | /api/auth/me | - | 200 tenant | |
| POST | /api/auth/change-password | `{oldPassword, newPassword}` | 204 | 超管首次登录(无密码)可省略 oldPassword |
| POST | /api/auth/forgot-password | `{email}` | 202 | 始终成功,不泄露存在性 |
| POST | /api/auth/reset-password | `{token, newPassword}` | 204 | 无效/过期 token 400 |

### 域名(后台,会话)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/domains | - | 200 [domain] | 含平台默认域名 |
| POST | /api/domains | `{fqdn, description?}` | 201 domain | 400 fqdn 非法/平台保留域名/描述超 200 字;409 域名已被占用;403 域名配额超限;平台默认域名不可重复添加 |
| GET | /api/domains/{id} | - | 200 domain | 404 |
| POST | /api/domains/{id}/recheck | - | 202 | 手动重新 DNS 校验 |
| PATCH | /api/domains/{id} | `{status: "stopped"\|"active"}` | 200 | 停用/恢复;**平台默认域名可停用/恢复**(spec 故事 55:只能停用不可删除;契约调整);自有域名恢复前校验 DNS 仍指向本机 |
| DELETE | /api/domains/{id} | - | 204 | 存在未删除短链 → 409(附关联数);平台默认域名 → 400 |

### 短链(后台,会话)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/links | query `page,pageSize` | 200 {items, total} | 短链列表,含 visits/clicks |
| POST | /api/links | `{code?, targetUrls: string[], domainIds[], redirectStatus?, linkType?, landingSource?, landingUrl?}` | 201 link | 400 目标少于 1 个/含 CRLF 控制字符(每项任意协议且不含控制字符)/短码非法/linkType 或 landingSource 非法/landing+url 来源缺合法 landingUrl;409 同域名同短码;403 短链配额超限 |
| GET | /api/links/{id} | - | 200 link | 404 |
| PATCH | /api/links/{id} | `{targetUrls?, domainIds?, redirectStatus?, status?, linkType?, landingSource?, landingUrl?}` | 200 | 传 `targetUrls` 时整体替换;切 redirect 清空落地页配置与已上传文件;切 url 来源删除已上传文件 |
| POST | /api/links/{id}/landing | multipart `file`(zip) | 200 link | 替换式上传落地页压缩包(需 X-CSRF-Token);非 landing 型 400;校验失败 400(见下);成功后 landingSource=upload、landingUploaded=true |
| DELETE | /api/links/{id} | - | 204 | 逻辑删除(落地页文件保留) |
| POST | /api/links/{id}/purge | - | 204 | 物理删除(含访问记录与落地页文件) |
| GET | /api/links/{id}/visits | query `page,pageSize` | 200 {items, total} | 访问列表 |
| GET | /api/links/{id}/stats | - | 200 {visits, clicks} | 访问数与点击数 |

> 目标 URL 支持多个;跳转命中后默认按轮询(round-robin)在 `targetUrls` 中选择一个作为重定向目的地。

#### 上传落地页 zip 校验规则

- 仅接受 zip;压缩包必须含 index.html(单层根文件夹自动剥离);
- 拒绝路径穿越(`..`/绝对路径)、符号链接;
- 扩展名白名单:html/htm/css/js/json/txt/png/jpg/jpeg/gif/svg/webp/ico/woff/woff2;
- 默认上限:解压后总大小 ≤ 10MB、文件数 ≤ 500(部署可配);
- 重传 = 清空旧文件整体替换,不保留版本。

### 短链公开路由(无需鉴权,由 Host 决定域名)

| 路径 | 行为 |
| --- | --- |
| GET /{code}(redirect 型) | 记 Visit → 301/302(按 redirectStatus)→ 轮询选目标(现状不变) |
| GET /{code}(landing 型) | 记 Visit → 固定 302 → `landingUrl`(url 来源)或 `/{code}/`(upload 来源) |
| GET /{code}/click | 仅 landing 型:clicks+1(**不**记 Visit)→ 固定 302 → 轮询选目标;其余 404 |
| GET /{code}/sdk.js | 仅 landing 型:返回内嵌点击端点地址的 JS SDK(`Cloak.bind(selector, {newTab?})`);其余 404 |
| GET /{code}/ 与 GET /{code}/* | 仅 landing+upload 型:托管静态文件,根 = index.html;`click`/`sdk.js` 为保留路径;其余 404 |

> 停用/逻辑删除/域名停用/未命中 → 以上公开路由一律 404;landing 型的 Visit 只记在 `GET /{code}` 那一次,子资源/点击端点/SDK 不计 Visit。

### 租户设置(后台,会话)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/me | - | 200 tenant | 含配额用量 `{usage:{links, domains, maxLinks, maxDomains}}` |
| GET | /api/config | - | 200 config | 前端启动配置:服务器 IP/平台域名/当前租户配额(按租户返回);未登录 401 |
| PATCH | /api/me | `{codeLength?}` | 200 tenant | 自动生成短码长度 |

### 平台管理(后台,超管)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/admin/tiers | - | 200 [tier] | 全部等级(供调整等级页选择;非超管 403) |
| GET | /api/admin/tenants | - | 200 [tenant] | 含用量;非超管 403 |
| GET | /api/admin/tenants/{id} | - | 200 tenant | 详情 |
| PATCH | /api/admin/tenants/{id} | `{status?: "banned"\|"active", tierId?}` | 200 | 封禁/解封、调等级 |
| DELETE | /api/admin/domains/{id} | - | 204 | 平台强删违规域名(解除其短链关联) |

### 跳转(公开)
| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /{code} | 命中 → 302/301(Location=目标);未命中/停用/逻辑删除/域名停用 → 404 |
