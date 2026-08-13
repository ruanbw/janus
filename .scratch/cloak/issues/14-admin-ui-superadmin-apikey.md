# 14 — admin-ui-superadmin-apikey(后台界面:超管页 + API Key 页)

**What to build:** 超管租户列表/封禁解封/调整等级/移除域名页面;API Key 生成(展示一次明文)与吊销页面。

**Blocked by:** 10, 08, 09

**Status:** resolved

- [ ] 超管页面:租户列表、封禁/解封、调整等级、移除域名
- [ ] API Key 页面:生成(明文仅展示一次)、吊销

## Comments

- 已实现 `web/src/views/admin/AdminTenantsView.vue`(仅平台管理员可见,路由守卫 + 403 兜底):租户列表(邮箱、前缀、状态、等级、用量 短链/域名、注册时间、平台管理员标记)、封禁/解封(PATCH status)、调整等级(等级选项从租户列表 tier 去重收集——契约无独立等级列表端点)、移除违规域名(契约无超管域名列表端点,弹窗需手工输入域名 ID 后调 DELETE,契约缺口已上报)。
- 已实现 `web/src/views/apikeys/ApiKeysView.vue`:列表(名称/创建时间)、生成(明文仅展示一次 + 复制按钮 + 警示)、吊销(确认弹窗)。API Key 公开 API 调用方式在 README 说明(Bearer)。
- 用到的契约端点:GET /api/api-keys、POST /api/api-keys、DELETE /api/api-keys/{id}、GET /api/admin/tenants、PATCH /api/admin/tenants/{id}、DELETE /api/admin/domains/{id};账号设置页另用 GET /api/me、PATCH /api/me(自动生成短码长度)、POST /api/auth/change-password。
- 待联调:超管初始化需后端以 `CLOAK_SUPERADMIN_EMAIL` 重启后,用超管账号登录验证租户列表/封禁/调等级;API Key 生成→明文展示→吊销流程需主会话 dev 后验证。
