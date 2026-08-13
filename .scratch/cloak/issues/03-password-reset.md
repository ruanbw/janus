# 03 — password-reset(密码重置)

**What to build:** 忘记密码 → 邮件重置链接 → 设置新密码 → 重新登录。

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] 提交邮箱后,控制台 mailer 输出一次性重置链接
- [ ] 有效链接可设置新密码,并以新密码重新登录
- [ ] 无效、已使用或过期的重置链接被拒绝
