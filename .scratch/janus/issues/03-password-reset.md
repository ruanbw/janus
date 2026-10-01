# 03 — password-reset(密码重置)

**What to build:** 忘记密码 → 邮件重置链接 → 设置新密码 → 重新登录。

**Blocked by:** 02

**Status:** resolved

- [ ] 提交邮箱后,控制台 mailer 输出一次性重置链接
- [ ] 有效链接可设置新密码,并以新密码重新登录
- [ ] 无效、已使用或过期的重置链接被拒绝

## Comments

- 已完成:`handleForgotPassword`(202 始终成功、不泄露存在性)、`handleResetPassword`(token 1h 有效、一次性,无效/已用/过期一律 400)、`handleChangePassword`(需旧密码;超管首登免旧密码)。
- 控制台 mailer 输出重置链接;重置成功后旧密码失效。
- 黑盒测试:`auth_test.go` 中 TestForgotAndResetPassword / TestResetTokenReuseRejected / TestChangePassword。测试代码未运行,需主会话执行 `go test ./...`。
