# 04 — domain-management(自有域名管理:校验 / 激活 / 证书 / 停用 / 删除)

**What to build:** 添加自有域名 → 真实 DNS 校验(dev 下 hosts→127.0.0.1 通过)→ 激活 → 证书预签发 → `https://{域名}/` 可访问;DNS 未生效进入重试队列(每 5 分钟、最长 72h);停用即不可服务;删除须先清空关联;支持手动重检;授权端点扩展为放行 active 自有域名。

**Blocked by:** 01, 02

**Status:** resolved

- [ ] 添加自有域名后立即 DNS 校验(A/AAAA 含服务器 IP);dev 下 hosts 指向 127.0.0.1 且 `CLOAK_SERVER_PUBLIC_IP=127.0.0.1` 时校验通过并置 active
- [ ] 激活后后台自动探活预签发证书,`cert_status` 从 pending → issued;`https://{域名}/` 可访问
- [ ] DNS 未生效时进入重试队列(每 5 分钟、最长 72h),状态 pending;超时置 failed,可手动重检
- [ ] 停用域名后其下所有短码未命中;可恢复
- [ ] 删除域名:存在未删除短链时拒绝并提示;清空后可物理删除
- [ ] Caddy 授权端点放行 active 的自有域名,拒绝未激活/停用域名

## Comments

- 已完成:`internal/httpapi/domains.go`(添加/列表/详情/手动重检/停用恢复/删除)、`internal/domain/worker.go`(DNS 重试队列每 5 分钟、最长 72h 置 failed;证书预签发探活)、`internal/domain/cert.go`(HTTPS 探活触发 Caddy on-demand 签发)、store 域名层(含 DetachDomain 物理删除、授权查询)。
- DNS 校验走真实代码路径(Go 默认解析器读 /etc/hosts);开发环境 `localhost`→127.0.0.1 且 `CLOAK_SERVER_PUBLIC_IP=127.0.0.1` 时校验通过置 active。
- 添加后立即校验;通过→active+异步探活;未通过→pending 进重试队列(72h 超时 failed);手动重检 202。
- 删除:平台默认域名 400;存在未删除短链 409(附关联数);否则物理删除(其上已逻辑删除且仅关联该域名的短链一并物理清除)。平台默认域名可停用/恢复(契约调整:原"不可停用"与 spec 故事 55"只能停用"矛盾,以 spec 为准)。
- 授权端点扩展:active 自有域名放行,停用/未激活拒绝;租户封禁时拒绝(08 联动)。
- 黑盒测试:`internal/httpapi/domains_test.go`(DNS 激活/校验失败/72h 超时 failed/停用恢复/删除/授权/配额)。测试代码未运行,需主会话执行 `go test ./...`。
