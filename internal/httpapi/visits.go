package httpapi

// 访问明细写入助手:从请求补齐 IP / User-Agent / 来源页 / 语言,写入 visits 行。
// 统计写入永远不阻断跳转(见 recordVisit),否则数据库抖动会把短链变成 500。

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cloak/internal/store"
)

// maxLangLen 语言标签最大长度。Accept-Language 是任意长度的客户端可控请求头,
// 不截断会让一行明细被单个超长头撑爆(明细表按条保留 90 天)。
const maxLangLen = 64

// recordVisit 记录一次访问/点击明细(IP、UA、来源页、语言由请求补齐)。
// 统计失败只吞掉错误:短链是否跳转是访问者的事,不能被写入失败牵连。
func (a *API) recordVisit(c *gin.Context, rec store.VisitRecord) {
	rec.IP = clientIP(c.Request)
	rec.UserAgent = c.Request.UserAgent()
	rec.Referer = c.Request.Referer()
	rec.Lang = clientLang(c.Request)
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
