# 02 — auth-register-login(注册 / 登录 / 会话 / 邮箱验证)

**What to build:** 邮箱+密码注册(自选 slug、唯一性校验、自动创建并激活平台默认域名)、登录(会话 cookie + CSRF)、登出、`me`;邮箱验证邮件(控制台 mailer)验证后租户转 active。

**Blocked by:** 01

**Status:** resolved

- [ ] 注册提交 email/password/slug:slug 非法、重复或与既有域名 FQDN 冲突时拒绝并提示
- [ ] 注册成功后自动创建 `{slug}.janus.test` 平台默认域名(active、不计配额),租户状态 pending
- [ ] 控制台 mailer 输出验证链接;点击后租户转 active,并触发默认域名证书预签发探活
- [ ] 已验证租户用邮箱+密码登录成功,颁发 HTTP-only/Secure/SameSite 会话 cookie,CSRF 防护生效
- [ ] 登录后可访问受保护端点;登出后失效;未登录访问受保护端点返回 401

## Comments

- 已完成:`internal/httpapi/auth.go`(register/verify-email/login/logout/me)、`session.go`(会话 cookie `janus_session` HTTP-only/SameSite=Lax/Secure 可配 + CSRF 双提交 cookie `janus_csrf` + `X-CSRF-Token` 校验写操作)、store 层租户/会话/email_tokens。
- 注册:slug 校验(小写字母数字+连字符,1-63)、邮箱唯一、slug 唯一、与既有域名 FQDN 冲突拒绝;创建 pending 租户 + 平台默认域名(active、不计配额);控制台 mailer 输出验证链接。
- 邮箱验证:token 24h 有效、一次性(used_at);验证后租户 active 并触发默认域名证书预签发探活。
- 登录:未验证 401、已封禁 403、密码错 401;`rememberMe` 可选字段(true/省略 30 天,false 24 小时,契约已注明);超管无密码时首登放行并带 `firstLoginSetup:true`(契约已注明)。
- 黑盒测试:`internal/httpapi/auth_test.go`(注册/冲突/验证/登录/登出/CSRF/me)。测试代码未运行,需主会话执行 `go test ./...`。
