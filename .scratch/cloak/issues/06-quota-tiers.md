# 06 — quota-tiers(配额与等级)

**What to build:** 等级表(免费档种子数据)、创建短链与自有域名时的配额校验、平台默认域名不计入域名配额、配额用量可查询。

**Blocked by:** 04, 05

**Status:** resolved

- [ ] 种子数据含免费等级(短链 100 / 域名 10);新租户默认免费等级
- [ ] 创建短链按"未物理删除"计数、添加自有域名按"未删除"计数校验配额
- [ ] 超限返回 4xx 且附带当前用量/上限;平台默认域名不计入域名配额
- [ ] 配额用量可查询(后台/API)

## Comments

- 已完成:迁移种子含免费档(短链 100/域名 10),新租户默认免费档;`store.Usage` 短链按"尚未物理删除"计数、自有域名按"未删除"计数(平台默认域名不计);创建短链/添加自有域名时校验配额,超限 403 附 `E_DOMAIN_LIMIT`/`E_LINK_LIMIT` 及 `{usage:{links,domains,maxLinks,maxDomains}}`。
- 用量查询:`GET /api/me` 返回 tenant 含 `usage`;`PATCH /api/me` 支持 `codeLength`(4-32)。
- 黑盒测试:`internal/httpapi/me_test.go` + 各配额断言(domains_test/links_test)。测试代码未运行,需主会话执行 `go test ./...`。
