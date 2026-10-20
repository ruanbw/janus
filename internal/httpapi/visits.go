package httpapi

// 访问明细的写入助手与只读聚合端点。
//
// 写入:从请求补齐 IP / User-Agent / 来源页 / 语言,写入 visits 行。
// 统计写入永远不阻断跳转(见 recordVisit),否则数据库抖动会把短链变成 500。
//
// 读:GET /api/visits/overview —— 总览页的一次性聚合端点(见 handleVisitsOverview)。

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/ruanbw/janus/internal/geo"
	"github.com/ruanbw/janus/internal/store"
)

// overviewRangeDays 时间范围快捷值 → 窗口跨度(自然日,含今日)。
//
// 键是对外契约的一部分(前端快捷选择直接传这些值),改动等于破坏 API。
// 一年按 365 个自然日算:窗口右端是请求时刻,所以 1y 必然覆盖不到未来的部分,
// 但对"按天看趋势"来说这就是用户要的窗口。
var overviewRangeDays = map[string]int{
	"today": 1,
	"3d":    3,
	"7d":    7,
	"30d":   30,
	"1y":    365,
}

// defaultOverviewRange 未指定 range 时的默认窗口。
//
// 存在意义是给非界面调用方(脚本/第三方)一个确定的缺省口径:总览页自己永远
// 显式传当前选中值。选 7d 而不是"保留期内全部",是因为后者的时长随部署配置
// 变化,同一个请求在不同部署上含义不同。
const defaultOverviewRange = "7d"

// overviewWindowResp 窗口回显:让前端如实标注"这些数字是哪一段的"。
//
// RetentionDays 是访问明细保留期(天)。1y 窗口必然超出它(默认 90 天),
// 前端据此说明"更早的数据已被清理任务删除",而不是把不足一年的数据冒充一年。
type overviewWindowResp struct {
	Range         string    `json:"range"`
	From          time.Time `json:"from"`
	To            time.Time `json:"to"`
	RetentionDays int       `json:"retentionDays"`
}

// overviewResp GET /api/visits/overview 的响应体。
//
// 聚合形状与 store.OverviewStats 一致,中间没有二次加工:handler 只做"取会话租户 →
// 解析窗口 → 调 store → 写 JSON",口径判断全部留在 SQL 里,避免出现"前端算一遍、
// 后端再算一遍"的两套实现;handler 加上的只有窗口本身(时区与日历属于展示口径)。
type overviewResp struct {
	Window overviewWindowResp `json:"window"`
	store.OverviewStats
}

// localDayStart 返回 t 所在自然日的本地 00:00。
// "今日"以服务器本地时区为准(生产容器用 TZ 指定);展示端不自己推算窗口,
// 直接用响应里的 from/to,避免浏览器时区与服务器时区不一致时算出两个"今日"。
func localDayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// visitRangeFrom 把访问明细的 range 参数解析成时间窗口左端。
//
// 返回 (from, active, ok):
//   - 空串与 "all" → active=false(不限时间),这是列表页的默认值;
//   - today/3d/7d/30d/1y → active=true,左端是**自然日** 00:00 起算,
//     与总览页 overviewRangeDays 的口径完全一致(两页不能各有一套"今日");
//   - 其他值 → ok=false,由 handler 映射 400,而不是静默退化成"全部"。
func visitRangeFrom(raw string, now time.Time) (time.Time, bool, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || value == "all" {
		return time.Time{}, false, true
	}
	days, ok := overviewRangeDays[value]
	if !ok {
		return time.Time{}, false, false
	}
	return localDayStart(now).AddDate(0, 0, -(days - 1)), true, true
}

// handleVisitsOverview GET /api/visits/overview?range=today|3d|7d|30d|1y — 总览页的全量聚合。
//
// 为什么不用"前端拉明细自己数"(这是本端点存在的原因,也是它与 handleListVisits
// 的分工):
//
//  1. 口径必然漂移。link.visits 由服务端按 action IN ('redirect','landing_view')
//     AND outcome='success' 过滤。前端那份等价实现历史上漏过一次条件(不传 action),
//     于是落地页型短链的同一次访问在五张分布图里各被计两次 —— 同屏 KPI 用的是
//     link.visits(服务端已正确过滤),两套数字必然对不上,且没有任何测试会发现。
//  2. 抽样当全量。原实现取"访问量最高的 10 条短链 × 最近 50 条明细";点击量大的
//     落地页,最近 50 行可能全是 click,分布图等于只统计了点击者。
//  3. 覆盖不全。总量累加 listLinks(1,100)(后端 pageSize 硬上限 100),
//     超过 100 条短链的租户看到的是偏小的总数,热门排行也只是这 100 条里的前 5。
//
// 代价是几条覆盖索引上的聚合扫描(见迁移 0019 的部分索引),换来的是分布图与 KPI
// 同源同口径,以及超过 100 条短链的租户也能看到真实总量。
//
// range 省略或为空按 defaultOverviewRange;非法值 400,而不是静默退回默认 ——
// 拼错参数时给人一个"别的窗口"的数字,比报错更糟。
func (a *API) handleVisitsOverview(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}

	rangeValue := c.Request.URL.Query().Get("range")
	if rangeValue == "" {
		rangeValue = defaultOverviewRange
	}
	days, ok := overviewRangeDays[rangeValue]
	if !ok {
		writeErr(c, http.StatusBadRequest, errValidation, "range 必须为 today / 3d / 7d / 30d / 1y")
		return
	}

	// 窗口右端取请求时刻(含右端之后写入的行本就不存在);左端是自然日 00:00,
	// 因此 today 就是"今日 00:00 到现在", Nd 是"含今日在内的 N 个自然日"。
	now := time.Now()
	from := localDayStart(now).AddDate(0, 0, -(days - 1))
	// 上一周期按同样的自然日跨度左移:7d 的上一周期就是再往前 7 个自然日。
	previousFrom := from.AddDate(0, 0, -days)

	stats, err := a.store.OverviewStatsForTenant(c.Request.Context(), t.ID, store.OverviewWindow{
		From:         from,
		PreviousFrom: previousFrom,
	})
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, overviewResp{
		Window: overviewWindowResp{
			Range:         rangeValue,
			From:          from,
			To:            now,
			RetentionDays: int(a.cfg.VisitRetention / (24 * time.Hour)),
		},
		OverviewStats: *stats,
	})
}

// visitGeoKey 请求级地理值的缓存键(gin.Context,生命周期就是一个请求)。
const visitGeoKey = "visit_geo"

// visitGeo 取本次访问的地理值,同一个请求内只查一次。
//
// 规则求值(ruleDecision)与明细写入(recordVisit)都要用它,而这两处在一次访问里
// 都会执行:一次省不掉的重复查询之外,更重要的是两边必须看到同一份结果——
// 裁决用的国家与明细记的国家如果不一致,排查"为什么这条规则命中了"时就会被
// 带到错误的方向上。
func (a *API) visitGeo(c *gin.Context) geo.Info {
	if v, ok := c.Get(visitGeoKey); ok {
		if info, ok := v.(geo.Info); ok {
			return info
		}
	}
	info := a.geo.Lookup(a.clientIPForVisitor(c.Request))
	c.Set(visitGeoKey, info)
	return info
}

// maxLangLen 语言标签最大长度。Accept-Language 是任意长度的客户端可控请求头,
// 不截断会让一行明细被单个超长头撑爆(明细表按条保留 90 天)。
const maxLangLen = 64

// recordVisit 记录一次访问/点击明细(IP、UA、来源页、语言、国家由请求补齐)。
// 统计失败只吞掉错误:短链是否跳转是访问者的事,不能被写入失败牵连。
//
// 机房/ASN 属性不在这里填:基座没有数据源(ADR 0009/0012),对应列已从 schema 与
// API 中移除。需要这两个维度的部署应通过扩展点(FactEnricher)提供,而不是让
// 每个访问明细都留两列恒为默认值的死数据。
func (a *API) recordVisit(c *gin.Context, rec store.VisitRecord) {
	rec.IP = a.clientIPForVisitor(c.Request)
	rec.UserAgent = c.Request.UserAgent()
	rec.Referer = c.Request.Referer()
	rec.Lang = clientLang(c.Request)
	rec.Country = a.visitGeo(c).Country
	// 异步路径:把明细交给有界队列批量落库,跳转响应不再等待 INSERT。
	// Enqueue 自己保证不丢(队列满 → 同步直写,失败才返回错误),所以这里不需要
	// 再补一次同步写 —— 那会在 Sink 失败时写出重复行。
	if a.visits != nil {
		if err := a.visits.Enqueue(c.Request.Context(), rec); err != nil {
			slog.Error("访问明细入队失败", "err", err, "linkId", rec.LinkID)
		}
		if a.metrics != nil {
			a.metrics.CounterVec("janus_visit_write_total", "访问明细写入方式。", "mode").Inc("async")
		}
		TriggerPostVisitHooks(c.Request.Context(), c.Request, rec)
		return
	}
	if a.metrics != nil {
		a.metrics.CounterVec("janus_visit_write_total", "访问明细写入方式。", "mode").Inc("sync")
	}
	if err := a.store.InsertVisit(c.Request.Context(), rec); err != nil {
		if rec.RuleID == nil {
			slog.Error("写入访问明细失败", "err", err, "linkId", rec.LinkID)
		} else {
			// 规则 id 不存在(插件拦截器返回了一个已删除的 id)会外键违约:
			// 丢的是整行明细,比丢一个 rule_id 严重得多 —— 降级为"无规则裁决"再写一次。
			ruleID := *rec.RuleID
			rec.RuleID = nil
			if err2 := a.store.InsertVisit(c.Request.Context(), rec); err2 != nil {
				slog.Error("写入访问明细失败", "err", err2, "linkId", rec.LinkID)
			} else {
				slog.Warn("规则裁决指向不存在的规则,明细已按无规则裁决落库", "ruleId", ruleID, "err", err)
			}
		}
	}
	TriggerPostVisitHooks(c.Request.Context(), c.Request, rec)
}

// clientLang 取 Accept-Language 的首个语言标签:
// "zh-CN,zh;q=0.9,en;q=0.8" → "zh-CN";"en-US;q=0.9" → "en-US";无该头则返回空串。
// 只留首标签(明细只需要"主要语言",完整列表要另存一列),并做长度上限保护。
func clientLang(r *http.Request) string {
	first, _, _ := strings.Cut(r.Header.Get("Accept-Language"), ",")
	tag, _, _ := strings.Cut(first, ";") // 剥离 ";q=0.9" 权重参数
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	// 按 rune(而非字节)截断:Accept-Language 里的语言标签可能含多字节字符,
	// 按字节切会把一个 UTF-8 序列从中间截断,落库后成为一个非法编码字符串
	// (后续读取/导出/JSON 序列化都可能因此出错)。
	if utf8.RuneCountInString(tag) > maxLangLen {
		tag = string([]rune(tag)[:maxLangLen])
	}
	return tag
}
