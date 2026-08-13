# 09 — public-api(API Key + 公开 REST API)

**What to build:** 后台生成/吊销 API Key(明文仅展示一次、库中存哈希);以 API Key 调用创建、查询列表、按 id 查询、删除短链的公开 REST API,数据按租户隔离。

**Blocked by:** 02, 05

**Status:** ready-for-agent

- [ ] 后台生成 API Key,明文仅展示一次,库中存哈希;可吊销
- [ ] 用 API Key 可创建、查询列表、按 id 查询、删除短链
- [ ] 数据按租户隔离(租户 A 的 Key 访问不到租户 B 的数据)
- [ ] 无效/已吊销的 Key 被拒绝
