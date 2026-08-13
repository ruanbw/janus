# 02 — auth-register-login(注册 / 登录 / 会话 / 邮箱验证)

**What to build:** 邮箱+密码注册(自选 slug、唯一性校验、自动创建并激活平台默认域名)、登录(会话 cookie + CSRF)、登出、`me`;邮箱验证邮件(控制台 mailer)验证后租户转 active。

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] 注册提交 email/password/slug:slug 非法、重复或与既有域名 FQDN 冲突时拒绝并提示
- [ ] 注册成功后自动创建 `{slug}.cloak.test` 平台默认域名(active、不计配额),租户状态 pending
- [ ] 控制台 mailer 输出验证链接;点击后租户转 active,并触发默认域名证书预签发探活
- [ ] 已验证租户用邮箱+密码登录成功,颁发 HTTP-only/Secure/SameSite 会话 cookie,CSRF 防护生效
- [ ] 登录后可访问受保护端点;登出后失效;未登录访问受保护端点返回 401
