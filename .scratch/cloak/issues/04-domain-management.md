# 04 — domain-management(自有域名管理:校验 / 激活 / 证书 / 停用 / 删除)

**What to build:** 添加自有域名 → 真实 DNS 校验(dev 下 hosts→127.0.0.1 通过)→ 激活 → 证书预签发 → `https://{域名}/` 可访问;DNS 未生效进入重试队列(每 5 分钟、最长 72h);停用即不可服务;删除须先清空关联;支持手动重检;授权端点扩展为放行 active 自有域名。

**Blocked by:** 01, 02

**Status:** ready-for-agent

- [ ] 添加自有域名后立即 DNS 校验(A/AAAA 含服务器 IP);dev 下 hosts 指向 127.0.0.1 且 `CLOAK_SERVER_PUBLIC_IP=127.0.0.1` 时校验通过并置 active
- [ ] 激活后后台自动探活预签发证书,`cert_status` 从 pending → issued;`https://{域名}/` 可访问
- [ ] DNS 未生效时进入重试队列(每 5 分钟、最长 72h),状态 pending;超时置 failed,可手动重检
- [ ] 停用域名后其下所有短码未命中;可恢复
- [ ] 删除域名:存在未删除短链时拒绝并提示;清空后可物理删除
- [ ] Caddy 授权端点放行 active 的自有域名,拒绝未激活/停用域名
