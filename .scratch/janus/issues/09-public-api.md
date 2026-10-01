# 09 — public-api(API Key + 公开 REST API)

**What to build:** 后台生成/吊销 API Key(明文仅展示一次、库中存哈希);以 API Key 调用创建、查询列表、按 id 查询、删除短链的公开 REST API,数据按租户隔离。

**Blocked by:** 02, 05

**Status:** resolved

- [ ] 后台生成 API Key,明文仅展示一次,库中存哈希;可吊销
- [ ] 用 API Key 可创建、查询列表、按 id 查询、删除短链
- [ ] 数据按租户隔离(租户 A 的 Key 访问不到租户 B 的数据)
- [ ] 无效/已吊销的 Key 被拒绝

## Comments

- 已完成:`internal/httpapi/apikeys.go`(生成 `janus_` 前缀随机 Key、库中存 SHA-256 哈希、明文仅创建响应一次、列表不含明文、吊销软删除)、`v1.go`(Bearer 鉴权 + /api/v1/links 创建/列表/按 id/删除)。
- 数据严格按租户隔离(所有查询带 tenant_id 过滤);无效/已吊销 Key → 401。
- 黑盒测试:`internal/httpapi/apikeys_test.go`(生命周期/CRUD/租户隔离/无效 Key)。测试代码未运行,需主会话执行 `go test ./...`。
