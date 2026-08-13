# 13 — admin-ui-stats(后台界面:统计页)

**What to build:** 短链访问数展示与访问列表(时间、UA、来源)。

**Blocked by:** 12, 07

**Status:** resolved

- [ ] 短链列表/详情展示访问数
- [ ] 访问列表(时间、UA、来源)可查看

## Comments

- 已实现 `web/src/views/stats/StatsView.vue`:顶部短链选择器(短码+访问数,支持搜索),选中后展示短链概览(短码/访问数/目标 URL/状态)与访问列表表格(访问时间、域名、User-Agent、来源 referer,均截断+tooltip),分页;短链列表页访问数列可点击直达统计页(`/stats?linkId=`)。链接页列表已展示访问数。
- 用到的契约端点:GET /api/links(列表,含 visits 字段)、GET /api/links/{id}/visits(访问列表);GET /api/links/{id}/stats 已在 `web/src/api/visits.ts` 封装(当前页面直接用列表的 visits 字段,未单独调用)。
- 待联调:需真实访问产生 Visit 记录后验证访问列表(时间/UA/来源)展示与分页。
