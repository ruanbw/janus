# 15 — prod-deploy(生产环境配置与上线检查清单)

**What to build:** 生产 Compose/Caddyfile(Let's Encrypt on-demand)、平台域名/公网 IP/泛解析配置说明、上线前检查清单;开发/生产配置差异文档化收尾。

**Blocked by:** 14

**Status:** ready-for-agent

- [ ] 生产 Compose 与 Caddyfile(Let's Encrypt on-demand)就绪
- [ ] `CLOAK_PLATFORM_DOMAIN`、`CLOAK_SERVER_PUBLIC_IP`、泛域名解析(`*.<平台域名>`)配置说明齐全
- [ ] 上线前检查清单:开发环境全流程通过(注册 → 邮箱验证 → 默认域名激活 → 证书签发 → 创建/关联短链 → 访问跳转 → 统计)
- [ ] 开发/生产差异文档化(SwitchHosts ↔ DNS、本地 CA ↔ Let's Encrypt、控制台 mailer ↔ SMTP)
