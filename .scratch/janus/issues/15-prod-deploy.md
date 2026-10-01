# 15 — prod-deploy(生产环境配置与上线检查清单)

**What to build:** 生产 Compose/Caddyfile(Let's Encrypt on-demand)、平台域名/公网 IP/泛解析配置说明、上线前检查清单;开发/生产配置差异文档化收尾。

**Blocked by:** 14

**Status:** resolved

- [x] 生产 Compose 与 Caddyfile(Let's Encrypt on-demand)就绪
- [x] `JANUS_PLATFORM_DOMAIN`、`JANUS_SERVER_PUBLIC_IP`、泛域名解析(`*.<平台域名>`)配置说明齐全
- [x] 上线前检查清单:开发环境全流程通过(注册 → 邮箱验证 → 默认域名激活 → 证书签发 → 创建/关联短链 → 访问跳转 → 统计)
- [x] 开发/生产差异文档化(SwitchHosts ↔ DNS、本地 CA ↔ Let's Encrypt、控制台 mailer ↔ SMTP)

## Comments

- 新增 `docker-compose.prod.yml`(标准 80/443、Let's Encrypt、backend 不暴露公网、`JANUS_COOKIE_SECURE=true`、必需变量用 `:?` 强制)与 `Caddyfile.prod`(on_demand_tls + ask 授权端点、ACME;站点地址不支持 env 占位符,示例域名 example.com 需替换为实际平台域名);
- 新增 `docs/deploy.md`:前置条件(DNS 泛解析 + 裸域名 A 记录)、环境变量表、启动步骤、上线前检查清单(9 项)、开发/生产差异表(SwitchHosts ↔ DNS、本地 CA ↔ Let's Encrypt、控制台 mailer ↔ SMTP、端口/Cookie 差异)、SMTP 说明与已知限制;
- 验证:`caddy adapt` 对 Caddyfile.prod 输出含 on_demand permission=authorize 端点的合法 JSON;`docker compose -f docker-compose.prod.yml config --quiet` 通过;
- 开发环境全流程(注册 → 邮箱验证 → 默认域名激活 → 证书签发 → 创建/关联短链 → 访问跳转 → 统计)已在主会话端到端验证通过(见 01–09 票据 Comments 与后端提交 bca9444),生产清单项按此对齐;
- 遗留:生产 SMTP 凭据需部署者提供(mailer 可插拔,见 spec 决策 #3);生产服务器上的真实 Let's Encrypt 签发需在具备 80/443 的公网环境验证(本机无公网域名)。
