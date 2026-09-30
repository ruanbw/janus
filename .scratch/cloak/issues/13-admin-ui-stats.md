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
- **2026 移除**：该页是访问明细页的整页复制品(选短链 → 汇总卡 → 同一张明细表),两张同源表各自演化必然漂移。已删除 `web/src/views/stats/StatsView.vue`、`/stats` 路由、`getLinkStats` 前端封装(最后一个调用方是访问明细页顶部那三张摘要卡,已随 `3231137` 删除),并把总览页「查看统计 →」改指到 `/links`。后端 `GET /api/links/{id}/stats`(`internal/httpapi/server.go:140` + 黑盒测试)保留不动。访问明细的唯一入口是短链列表那一行的「访问」列。
- **配额去重**(`48b61a7`)：同一份租户配额原本在 4 页 5 处重复(总览 KPI 副标题 / 短链列表 KPI 卡 / 短链列表表格底栏 / 域名池 `<QuotaBar>` / 账号设置「资源配额使用监控」)。根因是数据源不一致——只有总览每次 `loadData` 走 `auth.fetchMe()` 刷新,其余读 `GET /api/config` 的陈旧快照。现已只留总览一处「资源配额」面板(80% 提醒、100% 告警),删 `web/src/components/QuotaBar.vue` 与四处展示,并清掉短链列表里 4 处只为刷配额存在的 `auth.fetchMe()`(每次 3 个请求)。
- **状态列合并**(`3ae012d`)：短链列表「状态」徽标与「启用」开关读同一个 `link.status`,合并成一格(开关 + 文案),9 列 → 8 列。规则开关经核实**已完整实现**(列表 `RulesListView.vue:283-297`、表单 `RuleFormView.vue:475-481`、后端 `snapshot.go:103` 过滤 + `rules.go:263-266` 立即失效缓存),无需改动。
