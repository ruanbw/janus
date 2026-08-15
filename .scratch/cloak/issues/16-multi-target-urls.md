# 16 — multi-target-urls(短链多目标 URL,默认轮询)

**What to build:** 短链目标 URL 可配置多个,跳转默认按轮询选择;创建/编辑/列表/跳转全链路支持。

**Blocked by:** 05, 12

**Status:** resolved

## Comments

- 契约:Link.targetUrl → targetUrls: string[](至少 1 个,顺序即轮询顺序);POST/PATCH 同步。
- 数据:0006_multi_targets.sql 建 link_targets(link_id,url,position) 并迁移旧 target_url;links 增 rr_index 游标。
- 实现:store.CreateLink/UpdateLink/ResolveRedirect(原子 UPDATE rr_index 轮询)、httpapi 校验与请求体、redirect 按轮询结果 Location;前端表单多目标输入。
