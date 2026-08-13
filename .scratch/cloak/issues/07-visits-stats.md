# 07 — visits-stats(访问统计)

**What to build:** 每次成功跳转记录一条 Visit(短链、域名、UA、来源、时间);短链可查访问数与访问列表;默认保留 90 天、定时清理。

**Blocked by:** 05

**Status:** ready-for-agent

- [ ] 每次成功跳转记录一条 Visit(link、domain、UA、referer、时间)
- [ ] 短链可查询访问数与访问列表;计数随访问增长
- [ ] 超过保留期(默认 90 天)的 Visit 被定时清理
