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
- `rule.scope`: `global` | `links`
- `rule.action`: `pass` | `redirect` | `notfound` | `throttle`
- `rule.logic`: `all` | `any`
- `tier`: `{ id, name, maxLinks, maxDomains }`

## 资源形状(要点)

- `tenant`: `{ id, email, slug, status, isSuperAdmin, codeLength, tier, defaultDomain: "<slug>.<平台域名>", createdAt, firstLoginSetup?, usage? }`(`firstLoginSetup` 仅超管首次登录(尚无密码)时为 true;`usage` 见 /api/me)
- `domain`: `{ id, fqdn, description, origin, status, certStatus, activatedAt, createdAt }`(`description` 为创建时填写的备注,可空,最长 200 字)
- `config`: `{ serverIp, platformDomain, usage }`(`serverIp` 为 `CLOAK_SERVER_PUBLIC_IP`,DNS 校验指向地址;`usage` 为当前租户配额用量,按租户返回)
- `link`: `{ id, code, targetUrls: string[], redirectStatus, linkType, landingSource, landingUrl, status, domains: [fqdn...], visits, clicks, landingUploaded, createdAt }`(列表默认不含逻辑删除项)
  - `landingUrl`:仅 landing+url 来源非空;`clicks`:点击计数,仅 landing 型增长(redirect 型恒 0);`landingUploaded`:landing+upload 来源且已成功上传 zip 时为 true;`visits`:只统计**成功**的跳转/落地页视图(点击行与失败行均不计入)
- `visit`: `{ id, linkId, domain, ip, userAgent, referer, action, outcome, reason, targetUrl, country, isDatacenter, asn, lang, ruleId, ruleAction, createdAt }`(`ip` 为访问者 IP:部署前置 Caddy 时取 `X-Forwarded-For` 首段,否则取 `RemoteAddr`;旧记录为空字符串。`action` 为 `redirect`(跳转)| `landing_view`(落地页)| `click`(点击),`outcome` 为 `success` | `failed`,`failed` 时 `reason` 为 `link_disabled` | `link_deleted` | `no_target` | `landing_missing`;`targetUrl` 为本次动作最终抵达的地址;`country` / `isDatacenter` / `asn` 为地理占位,当前恒为空;`lang` 取 `Accept-Language` 首标签;`ruleId` / `ruleAction` 为本次访问的规则裁决结果,无规则参与时均为空)
  - 规则裁决导致的失败:`reason` 另有 `rule_blocked`(裁决 `notfound`)| `rule_throttled`(裁决 `throttle`),此时 `ruleId` / `ruleAction` 非空,`outcome='failed'` 且**不**计入访问次数
- `rule`: `{ id, name, description, priority, scope, enabled, logic, action, destination, conditions: [{field, operator, values: string[]}], linkCount, linkNames, linkIds, hits24h, createdAt, updatedAt }`
  - `action`: `pass`(放行)| `redirect`(改写目标为 `destination`)| `notfound`(返回 404)| `throttle`(返回 429);`destination` 仅 `action='redirect'` 有意义
  - `scope`: `global` 对本租户全部短链生效;`links` 只对关联的短链生效
  - `linkCount` / `linkNames` / `linkIds`:关联的短链数、**前 3 个可读标识**、**完整 id 列表**;`scope='global'` 时三者恒为空/0。**`scope='links'` 且 `linkCount=0` 是合法状态:该规则永远不会命中**,界面须显式标出「未关联短链 · 不会命中」
    - `linkIds` 是**完整**列表(不受 `linkNames` 的前 3 个限制),供规则编辑器精确回填关联;**没有它,界面只能靠短码猜**——而短码在租户内不唯一(见 CONTEXT 短码:唯一性是「同一域名下短码不重复」),同一条短码可以属于不同短链,猜出来的关联会有歧义
    - `linkNames` 形如 `短码@域名`,**不能只给短码**,理由同上
  - `conditions[].field` 仅接受 v1 字段集:`ip` | `ipattr` | `country` | `asn` | `lang` | `ref` | `utm` | `ua` | `devtype` | `os` | `browser` | `path` | `domain`;其中 `country` / `asn` 的数据源尚未接入,恒不命中
  - `conditions[].operator`: `in` | `not_in` | `eq` | `neq` | `contains` | `not_contains` | `gt` | `lt` | `regex` | `duplicated`
  - `hits24h`:近 24 小时命中次数,**读时从 `visits` 按 `rule_id` 聚合**(`created_at > now() - interval '24 hours'`),精确滚动窗口;`rules` 表上**不存计数器**——命中发生在跳转热路径上,每次命中写一次库等于给最高 QPS 的链路加写放大,且计数器重启即丢、会与明细表对不上。零关联规则恒为 0
- `ruleOption`(下拉用精简项):`{ id, name, scope, action, priority, enabled, hits24h }`
- `linkRule`(某条短链适用的规则):`{ id, name, scope, action, priority, enabled, source }`(`source` 为 `inherited`(来自 `scope='global'`)| `scoped`(来自关联))
  - **`source` 是权威字段,不可由前端推断**:`inherited` 表示全局规则,在短链侧恒为勾选且不可取消(见 `PUT` 一节);`source` 判错会直接变成"全局规则被取消得掉"的假象

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
| POST | /api/links/batch-delete | `{ids: number[]}` | 200 {deleted} | 批量逻辑删除(等价逐条 DELETE,`deleted_at` 置位,记录/关联/访问明细保留);400 ids 为空/数量 > 200/含 ≤ 0 的 id;不属于本租户、已删除或不存在的 id 静默跳过(幂等),`deleted` 为实际置位行数;需 X-CSRF-Token |
| POST | /api/links/batch-purge | `{ids: number[]}` | 200 {deleted} | 批量物理删除(连同 visits/link_targets/link_domains 走库内 ON DELETE CASCADE,并清理各短链落地页文件);400 条件同上;跨租户/不存在的 id 静默跳过(幂等),`deleted` 为实际删除行数(已逻辑删除的行同样被物理清除);需 X-CSRF-Token |
| GET | /api/links/{id}/visits | query `page,pageSize,action?` | 200 {items, total} | 访问明细列表(含跳转/落地页/点击三类动作);`action` 可选 `redirect` / `landing_view` / `click`,省略则不过滤,非法值 400 |
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

### 规则(后台,会话)
规则是**租户级**资源,归属于某个租户;单租户规则数上限 **200**,超出拒绝创建。规则参与访问裁决:按 `priority` 升序求值,**首条命中即定**,不做多规则叠加;裁决排在短链可用性之后(短链自身 404 的访问不参与规则)。

| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/rules | query `page,pageSize` | 200 {items, total} | 规则列表,分页约定与 /api/links 一致;列表项含 `scope` / `linkCount` / `linkNames`(前 3 个)/ `hits24h` |
| POST | /api/rules | `{name, description?, priority?, scope?, enabled?, logic?, action, destination?, conditions?, linkIds?}` | 201 rule | 400 见下;409 同租户规则重名;403 单租户规则数已达 200 上限;`scope='links'` 时 `linkIds` 建立关联,**允许为空**(零关联是合法状态,后端不拦,告警由 UI 暴露) |
| GET | /api/rules/options | - | 200 [ruleOption] | 供下拉使用的精简列表(裸数组) |
| GET | /api/rules/{id} | - | 200 rule | 含**完整未截断**的 `conditions` 与 `linkIds`;404 规则不存在或不属于本租户。**两者都不能只回前 N 个**——读取不全会被前端原样写回,静默截断真实条件/关联且无报错 |
| PATCH | /api/rules/{id} | 同 POST(除 linkIds 外均可选) | 200 rule | 400 见下;404 规则不存在或不属于本租户;**传 `linkIds` 时整体替换**该规则与短链的关联(省略则不动关联);`scope` 从 `links` 改回 `global` 时,关联清空 |
| DELETE | /api/rules/{id} | - | 204 | 级联删除其关联(`rule_links`);404 规则不存在或不属于本租户 |

> **写入校验(POST / PATCH 的 400 触发条件)**:① `name` 缺失或为空白;② `priority` < 0;③ `scope` 不在 `global`/`links`;④ `action` 不在 `pass`/`redirect`/`notfound`/`throttle`;⑤ `action='redirect'` 而 `destination` 缺失或不是合法 URL(含 CRLF 控制字符);⑥ `logic` 不在 `all`/`any`;⑦ `conditions` 中出现 v1 字段集之外的 `field`;⑧ `conditions[].operator` 不在运算符白名单;⑨ `linkIds` 中含跨租户或不存在(或已逻辑删除)的短链 id。
> 任何规则变更或关联变更后,必须失效该租户的规则快照缓存,否则新配置在快照重建前不生效。

> **有效作用域是 `global` 时,关联恒为空**:`POST` 传 `scope='global'` + `linkIds`、或对已为 `global` 的规则传 `linkIds`,都**静默清空关联**而不报错。理由:留着关联行就是让「适用于全部短链」与「只适用于这几条」同时成立,数据模型自相矛盾;而报错会让"改个作用域"这种无害操作失败。前端从不发这种组合(切全局时本来就传 `[]`),这条主要是给脚本调用方兜底。

### 短链 ↔ 规则关联(后台,会话)

| 方法 | 路径 | 请求 | 成功 | 说明 |
| --- | --- | --- | --- | --- |
| GET | /api/links/{id}/rules | - | 200 {items: [linkRule]} | 该短链适用的规则 = 租户全部 `scope='global'` 规则 ∪ 与本短链关联的 `scope='links'` 规则;`source` 标记继承或关联;404 短链不存在或不属于本租户 |
| PUT | /api/links/{id}/rules | `{ruleIds: number[]}` | 200 {items: [linkRule]} | 整体替换该短链与指定短链作用域规则的关联;空数组 = 解除全部;需 X-CSRF-Token |

> **`PUT /api/links/{id}/rules` 的错误码**:
> - 400:`ruleIds` 中含 `scope='global'` 的规则 id(全局规则不该被"关掉",误传说明调用方理解错了);`ruleIds` 中含跨租户或不存在的规则 id(配置写入不静默跳过:静默丢弃会让租户以为规则挂上了)。
> - 404:短链不存在或不属于本租户。
> - `scope='global'` 的规则既不接受被取消,也不受此端点影响。

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
| GET | /{code} | 命中 → 规则求值(首条命中即定)→ 302/301(Location=目标);未命中/停用/逻辑删除/域名停用 → 404(不参与规则求值) |

> 规则的裁决同样改变跳转结果:`redirect` 改写 Location 为规则的 `destination`(不参与目标轮询)、`notfound` → 404、`throttle` → 429、`pass` → 继续按原目标选择流程。裁决结果记入访问明细(`ruleId` / `ruleAction`,失败时 `reason` 为 `rule_blocked` / `rule_throttled`,不计入访问次数)。

> **规则对两种短链类型一视同仁**:跳转型与落地页型都参与规则裁决。`notfound` 拦掉落地页型时记 `action=landing_view / outcome=failed / reason=rule_blocked`;`redirect` 会让访客直接去规则的目标地址(绕过落地页)。理由:规则是租户级的访问处置控制,「把这条落地页链路的访问掐掉」是合理诉求;按短链类型分叉反而会产生"为什么这条拦了那条没拦"的意外。
