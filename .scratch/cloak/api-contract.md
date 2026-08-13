# CLOAK API 契约(v1,对齐 spec 决策 #10/#11)

后端、后台前端、公开 API 均以实现此契约为准。所有请求/响应为 JSON。

统一错误响应:
```json
{ "code": "E_DOMAIN_LIMIT", "message": "域名数量已达上限 (5/10)" }
```
`details`(可选)补充字段级信息。

## 认证方式

- **后台 API(管理端)**:会话 cookie `cloak_session`(HTTP-only / Secure / SameSite),CSRF 用双提交 token 请求头 `X-CSRF-Token`。未登录访问受保护端点 → 401。
- **公开 API**:请求头 `Authorization: Bearer <apiKey>`。无效/已吊销 → 401。
- **跳转路径**:`GET /{code}`(路径首段为短码),由 Host 决定域名,无需鉴权。
- **内部端点**:`GET /internal/caddy/authorize?domain=<fqdn>`,仅内网可达;放行 200,拒绝 403。

## 枚举

- `tenant.status`: `pending` | `active` | `banned`
- `domain.status`: `pending` | `active` | `failed` | `stopped`
- `domain.origin`: `self` | `platform`
- `domain.certStatus`: `pending` | `issued` | `failed`
- `link.status`: `enabled` | `disabled`
- `link.redirectStatus`: `"302"` | `"301"`
- `tier`: `{ id, name, maxLinks, maxDomains }`

## 资源形状(要点)

- `tenant`: `{ id, email, slug, status, isSuperAdmin, codeLength, tier, defaultDomain: "<slug>.<平台域名>", createdAt }`
- `domain`: `{ id, fqdn, origin, status, certStatus, activatedAt, createdAt }`
- `link`: `{ id, code, targetUrl, redirectStatus, status, domains: [fqdn...], visits, createdAt }`(列表默认不含逻辑删除项)
- `visit`: `{ id, linkId, domain, userAgent, referer, createdAt }`
- `apiKey`: `{ id, name, createdAt, key? }`(`key` 明文仅在创建响应中出现一次)

## 端点

### 认证(后台,会话)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| POST | /api/auth/register | `{email, password, slug}` | 201 tenant | 400 slug 非法;409 邮箱或 slug 已占用;创建租户(pending)+ 平台默认域名 |
| POST | /api/auth/verify-email | `{token}` | 200 | 无效 token 400;成功后租户 active,触发默认域名证书预签发 |
| POST | /api/auth/login | `{email, password}` | 200 tenant + Set-Cookie | 401 凭证错误或未验证 |
| POST | /api/auth/logout | - | 204 | |
| GET | /api/auth/me | - | 200 tenant | |
| POST | /api/auth/change-password | `{oldPassword, newPassword}` | 204 | |
| POST | /api/auth/forgot-password | `{email}` | 202 | 始终成功,不泄露存在性 |
| POST | /api/auth/reset-password | `{token, newPassword}` | 204 | 无效/过期 token 400 |

### 域名(后台,会话)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/domains | - | 200 [domain] | 含平台默认域名 |
| POST | /api/domains | `{fqdn}` | 201 domain | 409 域名已被他人占用;403 域名配额超限;平台默认域名不可重复添加 |
| GET | /api/domains/{id} | - | 200 domain | 404 |
| POST | /api/domains/{id}/recheck | - | 202 | 手动重新 DNS 校验 |
| PATCH | /api/domains/{id} | `{status: "stopped"\|"active"}` | 200 | 停用/恢复;平台默认域名不可停用则忽略或拒绝 |
| DELETE | /api/domains/{id} | - | 204 | 存在未删除短链 → 409(附关联数);平台默认域名 → 400 |

### 短链(后台,会话)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/links | query `page,pageSize` | 200 {items, total} | 短链列表,含 visits |
| POST | /api/links | `{code?, targetUrl, domainIds[], redirectStatus?}` | 201 link | 400 目标含 CRLF 控制字符/短码非法;409 同域名同短码;403 短链配额超限 |
| GET | /api/links/{id} | - | 200 link | 404 |
| PATCH | /api/links/{id} | `{targetUrl?, domainIds?, redirectStatus?, status?}` | 200 | |
| DELETE | /api/links/{id} | - | 204 | 逻辑删除 |
| POST | /api/links/{id}/purge | - | 204 | 物理删除(含访问记录) |
| GET | /api/links/{id}/visits | query `page,pageSize` | 200 {items, total} | 访问列表 |
| GET | /api/links/{id}/stats | - | 200 {visits} | 访问数 |

### API Key(后台,会话)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/api-keys | - | 200 [apiKey] | |
| POST | /api/api-keys | `{name}` | 201 apiKey | 响应含明文 `key`,仅此一次 |
| DELETE | /api/api-keys/{id} | - | 204 | |

### 租户设置(后台,会话)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/me | - | 200 tenant | 含配额用量 `{usage:{links, domains, maxLinks, maxDomains}}` |
| PATCH | /api/me | `{codeLength?}` | 200 tenant | 自动生成短码长度 |

### 平台管理(后台,超管)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/admin/tenants | - | 200 [tenant] | 含用量;非超管 403 |
| GET | /api/admin/tenants/{id} | - | 200 tenant | 详情 |
| PATCH | /api/admin/tenants/{id} | `{status?: "banned"\|"active", tierId?}` | 200 | 封禁/解封、调等级 |
| DELETE | /api/admin/domains/{id} | - | 204 | 平台强删违规域名(解除其短链关联) |

### 公开 API(Bearer)
| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/v1/links | - | 200 {items, total} | 自己租户的短链 |
| POST | /api/v1/links | `{code?, targetUrl, domainIds[], redirectStatus?}` | 201 link | 同后台创建规则 |
| GET | /api/v1/links/{id} | - | 200 link | |
| DELETE | /api/v1/links/{id} | - | 204 | 逻辑删除 |

### 跳转(公开)
| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /{code} | 命中 → 302/301(Location=目标);未命中/停用/逻辑删除/域名停用 → 404 |
