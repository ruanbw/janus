# 08 — superadmin(平台管理员)

**What to build:** 超管经环境变量初始化、首次登录引导设置密码;查看全部租户、封禁/解封、调整租户等级、移除违规域名;封禁后授权端点拒绝该租户域名。

**Blocked by:** 02, 04, 06

**Status:** resolved

- [ ] 环境变量(`CLOAK_SUPERADMIN_EMAIL`)指定超管邮箱;首次登录引导设置密码
- [ ] 超管可查看全部租户及其数据;非超管访问超管端点被拒
- [ ] 封禁租户后,其平台默认域名与自有域名均不再服务(授权端点拒绝);解封恢复
- [ ] 超管可调整租户等级;可移除违规域名

## Comments

- 已完成:`internal/bootstrap`(环境变量 CLOAK_SUPERADMIN_EMAIL 启动初始化:创建/标记 is_super_admin、active、无密码首登引导设置密码)、`internal/httpapi/admin.go`(租户列表含用量/详情/封禁解封/调等级、平台强删违规域名解除关联)。
- 封禁联动:授权端点对封禁租户的默认域名与自有域名均拒绝(GetDomainAuth 检查 tenant.status),登录拒绝;解封恢复。
- 非超管访问超管端点 403;调等级校验 tier 存在。
- 黑盒测试:`internal/httpapi/admin_test.go`(首登设密/列表与权限/封禁解封联动授权/调等级/强删域名)。测试代码未运行,需主会话执行 `go test ./...`。
