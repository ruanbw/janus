package httpapi

// 访问明细写入助手:从请求补齐 IP / User-Agent / 来源页 / 语言,写入 visits 行。
// 统计写入永远不阻断跳转(见 recordVisit),否则数据库抖动会把短链变成 500。

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cloak/internal/geo"
	"cloak/internal/store"
)

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
	info := a.geo.Lookup(clientIP(c.Request))
	c.Set(visitGeoKey, info)
	return info
}

// maxLangLen 语言标签最大长度。Accept-Language 是任意长度的客户端可控请求头,
// 不截断会让一行明细被单个超长头撑爆(明细表按条保留 90 天)。
const maxLangLen = 64

// recordVisit 记录一次访问/点击明细(IP、UA、来源页、语言、国家由请求补齐)。
// 统计失败只吞掉错误:短链是否跳转是访问者的事,不能被写入失败牵连。
//
// asn / is_datacenter 不在这里填:当前没有数据源(ADR 0009),写进去的只会是空值与 false,
// 与数据库默认值一样,反而像是"查到了但没值"。真要支持这两个值时,得先在
// store.VisitRecord 里加字段并在这里一并填。
func (a *API) recordVisit(c *gin.Context, rec store.VisitRecord) {
	rec.IP = clientIP(c.Request)
	rec.UserAgent = c.Request.UserAgent()
	rec.Referer = c.Request.Referer()
	rec.Lang = clientLang(c.Request)
	rec.Country = a.visitGeo(c).Country
	_ = a.store.InsertVisit(c.Request.Context(), rec)
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
	if len(tag) > maxLangLen {
		tag = tag[:maxLangLen]
	}
	return tag
}
