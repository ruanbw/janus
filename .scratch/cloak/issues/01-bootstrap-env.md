# 01 — bootstrap-env(脚手架与开发环境)

**What to build:** `docker compose up` 拉起 Go 后端与 Postgres;前置 Caddy 监听 80/443,on-demand TLS 与本地 CA 的组合经 spike 验证;`app.cloak.test`(SwitchHosts 指向 127.0.0.1)通过 HTTPS 访问到 Go 服务;数据库迁移机制就绪;Caddy 授权端点最小实现(平台域名放行)。

**Blocked by:** 无——可立即开工。

**Status:** ready-for-agent

- [ ] `docker compose up` 后 Go 后端与 Postgres 正常启动,数据库迁移自动执行
- [ ] `app.cloak.test` 映射到 127.0.0.1 后,浏览器经 HTTPS 访问到 Go 服务,证书为本地 CA 签发并被信任(执行过 `caddy trust`)
- [ ] Caddy on-demand TLS 与本地 CA(`tls internal`)的组合已通过小 spike 验证
- [ ] Caddy 授权端点最小实现:平台域名放行;端点仅内网可达
- [ ] 环境变量(`CLOAK_PLATFORM_DOMAIN`、`CLOAK_SERVER_PUBLIC_IP`)读取就绪,开发默认值(`cloak.test`、`127.0.0.1`)可用
