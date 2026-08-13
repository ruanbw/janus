# 07 — visits-stats(访问统计)

**What to build:** 每次成功跳转记录一条 Visit(短链、域名、UA、来源、时间);短链可查访问数与访问列表;默认保留 90 天、定时清理。

**Blocked by:** 05

**Status:** resolved

- [ ] 每次成功跳转记录一条 Visit(link、domain、UA、referer、时间)
- [ ] 短链可查询访问数与访问列表;计数随访问增长
- [ ] 超过保留期(默认 90 天)的 Visit 被定时清理

## Comments

- 已完成:每次成功跳转同步插入 Visit(link、domain、UA、referer、时间;失败不阻断跳转);`GET /api/links/{id}/visits` 分页列表、`GET /api/links/{id}/stats` 计数、列表页自带 visits 计数;`worker.visitCleanupPass` 每日清理超过保留期(默认 90 天,可配置)的记录。
- 黑盒测试:`links_test.go` 中 TestVisitsRecordedAndCounted(计数/UA/来源/未命中不记录)、TestVisitCleanup(91 天前记录被 worker 清理)。测试代码未运行,需主会话执行 `go test ./...`。
