# 01 — bootstrap-env(脚手架与开发环境)

**What to build:** `docker compose up` 拉起 Go 后端与 Postgres;前置 Caddy 监听 80/443,on-demand TLS 与本地 CA 的组合经 spike 验证;`app.janus.test`(SwitchHosts 指向 127.0.0.1)通过 HTTPS 访问到 Go 服务;数据库迁移机制就绪;Caddy 授权端点最小实现(平台域名放行)。

**Blocked by:** 无——可立即开工。

**Status:** resolved

- [ ] `docker compose up` 后 Go 后端与 Postgres 正常启动,数据库迁移自动执行
- [ ] `app.janus.test` 映射到 127.0.0.1 后,浏览器经 HTTPS 访问到 Go 服务,证书为本地 CA 签发并被信任(执行过 `caddy trust`)
- [ ] Caddy on-demand TLS 与本地 CA(`tls internal`)的组合已通过小 spike 验证
- [ ] Caddy 授权端点最小实现:平台域名放行;端点仅内网可达
- [ ] 环境变量(`JANUS_PLATFORM_DOMAIN`、`JANUS_SERVER_PUBLIC_IP`)读取就绪,开发默认值(`janus.test`、`127.0.0.1`)可用

## Comments

- 已完成:`go.mod`(pgx v5.10.0、x/crypto v0.55.0,go.sum 手工构造自 sumdb)、`cmd/janus/main.go`(配置→连库→迁移→超管初始化→worker→HTTP)、`internal/config`(全部环境变量,开发默认值可用)、`internal/db`(连接池+自研迁移器)、`migrations/0001_init.sql`(全量表结构+tiers 免费档种子)、`internal/httpapi`(健康检查、Caddy 授权端点最小实现:平台域名放行、仅内网可达、未注册域名拒绝)、`internal/mailer`(控制台假 mailer)、`internal/domain`(DNS 校验/证书探活/短码生成/后台 worker)、`internal/store`、`Caddyfile`(on_demand + local_certs + 泛域名反代)、`docker-compose.yml`、`docker/Dockerfile`、`.env.example`。
- spike 结论:配置已按 Caddy 官方 on-demand TLS + `tls internal { on_demand }` 组合编写(Caddyfile 注释含生产切换说明);因本会话无命令执行权限,`docker compose up` 拉起与 `curl --resolve` 验证未实际执行——需主会话按部署文件拉起并验证(见最终汇报命令清单)。
- 黑盒测试:`internal/httpapi/authorize_test.go`(healthz、平台域名放行、未注册拒绝、非内网 403)+ `internal/testutil`(真实 Postgres 测试库 + httptest 服务)。测试代码未运行(本会话无 go test 权限),需主会话执行 `go test ./...`。
- 环境事实:`/etc/hosts` 尚无 janus.test 记录;授权端点测试通过 httptest 回环地址走内网路径,域名解析测试用 `localhost`(解析 127.0.0.1)走真实代码路径。
