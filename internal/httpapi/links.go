package httpapi

// 05/06/07 — 短链:创建(自动/自定义短码、关联 active 域名、配额校验)、查询、
// 更新、逻辑删除/物理删除、访问列表与统计。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"

	"janus/internal/domain"
	"janus/internal/store"
)

// validTargetURL 目标 URL 任意协议(开放重定向),但拒绝控制字符(CRLF header 注入防护)。
func validTargetURL(s string) bool {
	if s == "" || len(s) > 4096 {
		return false
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// dedupeTargetURLs 按首次出现顺序去重(保持轮询顺序稳定)。
//
// 为什么必须去重:link_targets 的唯一约束是 (link_id, position),不是 url,
// 所以 ["a","a","b"] 会原样存成三行,轮询时 a 拿到 2/3 的流量。
// 用户几乎不可能**想要**这个分布(真想要就该写两条不同的出口),而"配重了"
// 是手滑就能犯的错 —— 静默加权比直接报错更危险,因为它完全不可见。
func dedupeTargetURLs(urls []string) []string {
	out := make([]string, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))
	for _, u := range urls {
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

// validTargetURLs 校验目标 URL 列表:至少 1 个、数量 ≤ max,逐项沿用 validTargetURL 规则。
// 返回去重后的列表(调用方必须用返回值落库)。
//
// max 来自 cfg.MaxTargetURLs:没有它,一次请求就能提交几十万条目标,
// 换算到库里就是几十万次 INSERT(体积侧由 maxCreateLinkBodyBytes 兜住)。
func validTargetURLs(urls []string, max int) ([]string, error) {
	if len(urls) == 0 {
		return nil, errors.New("targetUrls 至少需要一个目标 URL")
	}
	urls = dedupeTargetURLs(urls)
	if max > 0 && len(urls) > max {
		return nil, fmt.Errorf("目标 URL 最多 %d 个(去重后 %d 个)", max, len(urls))
	}
	for _, u := range urls {
		if !validTargetURL(u) {
			return nil, errors.New("目标 URL 非法(不能为空、超长或包含控制字符)")
		}
	}
	return urls, nil
}

// maxTargetURLs 本次可接受的目标 URL 数量上限。
// 配置缺失/非正时回落到兜底值而不是"不限制":0 会被读成"没有上限",
// 那等于把"一次请求能写多少行"的决定权交还给调用方。
func (a *API) maxTargetURLs() int {
	if a.cfg.MaxTargetURLs > 0 {
		return a.cfg.MaxTargetURLs
	}
	return 50
}

// maxCreateLinkBodyBytes 创建端点的请求体上限。
//
// 按「最多 max 个目标 × 单个 URL 上限 4096 + JSON 转义开销」推算,再加一批余量给
// domainIds / code 等其余字段。只加数量校验不加体积校验是不够的:十万条 5 字节的
// URL 数量上完全合规,体积上照样能把请求体撑到几百 MB。
func (a *API) maxCreateLinkBodyBytes() int64 {
	const (
		perURLBudget = 4096 + 32 // URL 上限 + 引号/逗号/转义余量
		fieldBudget  = 64 << 10  // code/domainIds/落地页字段等其余部分
		cap          = 8 << 20   // 配置被调得很大时的硬顶,避免算出几百 MB 的上限
	)
	n := int64(a.maxTargetURLs())*perURLBudget + fieldBudget
	if n > cap {
		return cap
	}
	return n
}

// RedirectStatus 跳转方式(契约枚举:"301" | "302")。
// 兼容字符串("301"/"302")与数字(301/302)两种 JSON 表示。
type RedirectStatus string

// UnmarshalJSON 接受字符串或数字形式的 301/302。
func (s *RedirectStatus) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch n := v.(type) {
	case string:
		*s = RedirectStatus(n)
	case float64:
		*s = RedirectStatus(strconv.Itoa(int(n)))
	default:
		return fmt.Errorf("redirectStatus must be \"301\" or \"302\"")
	}
	return nil
}

type createLinkReq struct {
	Code           string          `json:"code"`
	TargetURLs     []string        `json:"targetUrls"`
	DomainIDs      []int64         `json:"domainIds"`
	RedirectStatus *RedirectStatus `json:"redirectStatus"`
	LinkType       string          `json:"linkType"`
	LandingSource  string          `json:"landingSource"`
	LandingURL     string          `json:"landingUrl"`
}

// normalizeLanding 校验并归一化落地页字段(16):返回 linkType/landingSource/landingURL。
func normalizeLandingFields(linkType, landingSource, landingURL string) (string, string, string, error) {
	if linkType == "" {
		linkType = store.LinkTypeRedirect
	}
	if landingSource == "" {
		landingSource = store.LandingSourceURL
	}
	if linkType != store.LinkTypeRedirect && linkType != store.LinkTypeLanding {
		return "", "", "", errors.New("linkType 必须为 redirect 或 landing")
	}
	if landingSource != store.LandingSourceURL && landingSource != store.LandingSourceUpload {
		return "", "", "", errors.New("landingSource 必须为 url 或 upload")
	}
	switch {
	case linkType == store.LinkTypeRedirect:
		landingSource, landingURL = store.LandingSourceURL, ""
	case landingSource == store.LandingSourceUpload:
		landingURL = ""
	default: // landing + url
		if !validTargetURL(landingURL) {
			return "", "", "", errors.New("落地页型短链(url 来源)必须填写合法的落地页地址")
		}
	}
	return linkType, landingSource, landingURL, nil
}

// normalizeRedirectStatus 归一落库的跳转方式:落地页型一律 302。
//
// 背景:landing 型的正常访问走硬编码 302,只有规则裁决命中 action=redirect 时才会
// 走 redirectToTarget,而那里读的是 link.RedirectStatus。"301 跳转型 → 落地页型"
// 的切换里前端不发 redirectStatus,UpdateLink 对 nil 字段保留原值,301 就留在库里
// (store/links.go 的 effectiveRedirectStatus 在写入侧再兜一次,双保险)。
func normalizeRedirectStatus(redirectStatus store.RedirectStatus, linkType string) store.RedirectStatus {
	if linkType == store.LinkTypeLanding {
		return store.RedirectStatus302
	}
	return redirectStatus
}

func (a *API) handleCreateLink(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, a.maxCreateLinkBodyBytes())
	var req createLinkReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, createLinkBodyErrMsg(err))
		return
	}
	link, err := a.createLink(c, t, req)
	if err != nil {
		writeAPIError(c, err)
		return
	}
	writeJSON(c, http.StatusCreated, a.withLandingUploaded(link))
}

// createLinkBodyErrMsg 请求体解析失败的对外文案(体积超限与语法错误分开说)。
func createLinkBodyErrMsg(err error) string {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return "请求体过大:目标 URL 过多或单个过长"
	}
	return "invalid JSON body"
}

// autoCodeAttempts 自动生成短码时的撞码重试次数。
const autoCodeAttempts = 10

// createLinkParams 一次创建所需的全部已校验参数(校验与落库分离,便于重试复用)。
type createLinkParams struct {
	code           string // 空串 = 自动生成
	targetURLs     []string
	redirectStatus store.RedirectStatus
	linkType       string
	landingSource  string
	landingURL     string
	domainIDs      []int64
}

// createLink 供后台短链创建共用。
func (a *API) createLink(c *gin.Context, t *store.Tenant, req createLinkReq) (*store.Link, error) {
	targetURLs, err := validTargetURLs(req.TargetURLs, a.maxTargetURLs())
	if err != nil {
		return nil, apiErr{http.StatusBadRequest, errValidation, err.Error(), nil}
	}
	redirectStatus := store.RedirectStatus302
	if req.RedirectStatus != nil {
		if *req.RedirectStatus != "301" && *req.RedirectStatus != "302" {
			return nil, apiErr{http.StatusBadRequest, errValidation, "redirectStatus 必须为 301 或 302", nil}
		}
		redirectStatus = store.RedirectStatus(*req.RedirectStatus)
	}
	linkType, landingSource, landingURL, err := normalizeLandingFields(req.LinkType, req.LandingSource, req.LandingURL)
	if err != nil {
		return nil, apiErr{http.StatusBadRequest, errValidation, err.Error(), nil}
	}
	// 落地页 + upload 来源但还没托管文件 → 400。
	// 创建流程是「建链 → 再上传」两次独立请求:先建链再上传,中间失败就留下一条
	// source=upload 却无文件的短链,此后每次访问都记一行 landing_missing,永远 404。
	// 新建的短链不可能已有托管文件,所以这一条在创建时必然成立(见 issue 04)。
	if linkType == store.LinkTypeLanding && landingSource == store.LandingSourceUpload {
		return nil, apiErr{http.StatusBadRequest, errValidation,
			"落地页型短链(upload 来源)必须先上传落地页压缩包", nil}
	}
	if len(req.DomainIDs) == 0 {
		return nil, apiErr{http.StatusBadRequest, errValidation, "至少关联一个域名", nil}
	}
	// 域名必须属于当前租户且 active
	domainIDs, err := a.validateLinkDomains(c, t.ID, req.DomainIDs)
	if err != nil {
		return nil, err
	}
	params := createLinkParams{
		code: req.Code, targetURLs: targetURLs,
		redirectStatus: normalizeRedirectStatus(redirectStatus, linkType),
		linkType:       linkType, landingSource: landingSource, landingURL: landingURL,
		domainIDs: domainIDs,
	}
	if params.code != "" {
		if !domain.IsValidCode(req.Code) {
			return nil, apiErr{http.StatusBadRequest, errValidation,
				"短码非法(字符集不含 0/O/1/l/I,长度 1-64)", nil}
		}
		link, err := a.createLinkWithQuota(c, t.ID, params, params.code)
		if err != nil {
			if qerr := quotaAPIError(err); qerr != nil {
				return nil, qerr
			}
			if store.IsUniqueViolation(err) {
				return nil, apiErr{http.StatusConflict, errConflict, "同一域名下短码已存在", nil}
			}
			return nil, err
		}
		return link, nil
	}
	// 自动生成短码:随机生成直到无冲突。
	//
	// 重试必须包在 WithQuotaInTx **外面**:唯一约束冲突会中止当前事务,事务回滚后
	// 租户行锁随之释放。放在事务内重试等于在一个已经失败的事务里继续写,
	// 第二轮起每条语句都会撞 "current transaction is aborted"。
	// 所以每轮都是一次全新的 WithQuotaInTx:重新开事务 → 重新锁租户行 → 重新判配额。
	for i := 0; i < autoCodeAttempts; i++ {
		code := domain.GenerateCode(domain.AutoCodeLength)
		link, err := a.createLinkWithQuota(c, t.ID, params, code)
		if err == nil {
			return link, nil
		}
		// 配额错误不是"撞码",立刻返回:重试只会再撞一次同样的上限
		if qerr := quotaAPIError(err); qerr != nil {
			return nil, qerr
		}
		if store.IsUniqueViolation(err) {
			continue
		}
		return nil, err
	}
	return nil, apiErr{http.StatusInternalServerError, errInternal, "生成短码失败,请重试", nil}
}

// createLinkWithQuota 在「锁租户行 → 读配额 → 判上限 → 插入」的单事务里创建短链。
//
// 为什么不能是「Usage() 读一次 + CreateLink() 插一次」:两者不在同一事务、没有锁,
// 是典型 TOCTOU。free 档 100 条时,并发 20 个 POST /api/links 可以在计数停在 99 的
// 窗口里全部通过检查,写出 119 条。现在 SELECT ... FOR UPDATE 把同一租户的消费串行化,
// 后到的请求必须等前一个事务提交,然后在锁内重新读到已含前一笔结果的计数。
// 跨租户互不阻塞。
//
// 返回值里的 Link 在事务**提交后**再读(fillLinkMeta 要看到已提交的关联行);
// 所以 linkID 由闭包捕获,而不是在事务内就把整条 Link 装配好。
func (a *API) createLinkWithQuota(c *gin.Context, tenantID int64, p createLinkParams, code string) (*store.Link, error) {
	ctx := c.Request.Context()
	var linkID int64
	err := a.store.WithQuotaInTx(ctx, tenantID, store.QuotaLinks, func(tx *gorm.DB) error {
		id, err := a.store.CreateLinkInTx(ctx, tx, tenantID, code, p.targetURLs, p.redirectStatus,
			p.linkType, p.landingSource, p.landingURL, p.domainIDs)
		if err != nil {
			return err
		}
		linkID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	return a.store.GetLinkByID(ctx, tenantID, linkID)
}

// quotaAPIError 把 store 的配额错误翻成对外响应(403 + E_LINK_LIMIT + details.usage),
// 非配额错误返回 nil。details 的形状与原先手工读 Usage() 时完全一致,前端不变。
func quotaAPIError(err error) error {
	var qe *store.QuotaError
	if errors.As(err, &qe) && qe.Kind == store.QuotaLinks {
		return apiErr{http.StatusForbidden, errLinkQuota,
			"短链数量已达上限", map[string]any{"usage": qe.Usage}}
	}
	return nil
}

// validateLinkDomains 校验域名归属与激活状态,返回去重后的 domainID 列表。
// 创建与编辑都要求至少保留一个关联域名;传空数组意味着短链将无法通过任何域名访问。
func (a *API) validateLinkDomains(c *gin.Context, tenantID int64, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, apiErr{http.StatusBadRequest, errValidation, "至少关联一个域名", nil}
	}
	seen := map[int64]bool{}
	var out []int64
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		d, err := a.store.GetDomainByID(c.Request.Context(), id)
		if err != nil || d.TenantID != tenantID {
			return nil, apiErr{http.StatusBadRequest, errValidation, "域名不存在或不属于当前租户", nil}
		}
		if d.Status != "active" {
			return nil, apiErr{http.StatusBadRequest, errValidation, "域名未激活,无法关联短链", nil}
		}
		out = append(out, id)
	}
	return out, nil
}

func pageParams(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.Request.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ = strconv.Atoi(c.Request.URL.Query().Get("pageSize"))
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func (a *API) handleListLinks(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)
	// includeDeleted(默认 false)打开回收站视图:列出已逻辑删除的短链。
	// 省略该参数时行为与从前完全一致,既有客户端不受影响。
	includeDeleted, err := parseBoolQuery(c, "includeDeleted")
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, err.Error())
		return
	}
	ctx := c.Request.Context()
	var items []*store.Link
	var total int
	if includeDeleted {
		items, total, err = a.store.ListDeletedLinksByTenant(ctx, t.ID, page, pageSize)
	} else {
		items, total, err = a.store.ListLinksByTenant(ctx, t.ID, page, pageSize)
	}
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if items == nil {
		items = []*store.Link{}
	}
	for _, l := range items {
		a.withLandingUploaded(l)
	}
	writeJSON(c, http.StatusOK, map[string]any{"items": items, "total": total})
}

// parseBoolQuery 解析布尔查询参数。参数缺省时返回 def;显式写了非法值时 400,
// 而不是悄悄当成 false —— "?includeDeleted=yes" 被当成"没开回收站"会让人以为
// 列表坏了而去找别的 bug。
func parseBoolQuery(c *gin.Context, name string) (bool, error) {
	raw := c.Request.URL.Query().Get(name)
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s 必须为 true 或 false", name)
	}
	return v, nil
}

func (a *API) handleGetLink(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	link, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id)
	if err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	writeJSON(c, http.StatusOK, a.withLandingUploaded(link))
}

type patchLinkReq struct {
	TargetURLs     *[]string       `json:"targetUrls"`
	DomainIDs      *[]int64        `json:"domainIds"`
	RedirectStatus *RedirectStatus `json:"redirectStatus"`
	LinkType       *string         `json:"linkType"`
	LandingSource  *string         `json:"landingSource"`
	LandingURL     *string         `json:"landingUrl"`
	Status         *string         `json:"status"`
	RulesEnabled   *bool           `json:"rulesEnabled"`
	// DeletedAt 是三态字段:未提供(长度为 0)/ 显式 null(内容为 "null")/ 其他值。
	// 显式 null 的语义是「还原这条逻辑删除的短链」—— PATCH 改资源字段,
	// 把 deleted_at 写回 NULL 就是撤销删除。
	//
	// 这里必须用值类型 json.RawMessage 而不是 *json.RawMessage:encoding/json
	// 遇到 JSON null 时对**指针**字段的处理是「把指针置 nil」,不会调用
	// UnmarshalJSON,于是「显式 null」和「未提供」被压成同一个状态,还原请求
	// 会被当成普通 PATCH 走 GetLinkByID → 对已软删短链返回 404。
	// 值类型才能让 UnmarshalJSON 收到 "null" 这四个字节,把三态区分开。
	DeletedAt json.RawMessage `json:"deletedAt"`
}

func (a *API) handlePatchLink(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	var req patchLinkReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	if len(req.DeletedAt) > 0 {
		a.restoreLink(c, t, id, req)
		return
	}
	// 落地页字段组合校验(16):按当前值叠加请求值算出有效组合
	cur, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id)
	if err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	linkType, landingSource, landingURL := cur.LinkType, cur.LandingSource, cur.LandingURL
	if req.LinkType != nil {
		linkType = *req.LinkType
	}
	if req.LandingSource != nil {
		landingSource = *req.LandingSource
	}
	if req.LandingURL != nil {
		landingURL = *req.LandingURL
	}
	linkType, landingSource, landingURL, nerr := normalizeLandingFields(linkType, landingSource, landingURL)
	if nerr != nil {
		writeErr(c, http.StatusBadRequest, errValidation, nerr.Error())
		return
	}
	// 切到 upload 来源时必须已有托管文件,否则这条短链会永久 404
	// (每次访问记一行 landing_missing),见 issue 04。
	if linkType == store.LinkTypeLanding && landingSource == store.LandingSourceUpload &&
		!a.landingUploaded(id) {
		writeErr(c, http.StatusBadRequest, errValidation,
			"落地页型短链(upload 来源)必须先上传落地页压缩包")
		return
	}
	// 只下发**请求真正改动过**的字段。
	//
	// 上面那三个值是从事务外读到的 cur 推导出来的,不能无条件写回:两个并发
	// PATCH 各自带着自己的旧快照进来时,"只改 status"的那个请求也会把 linkType
	// 覆写成旧值 —— store 层 UpdateLink 里的 SELECT ... FOR UPDATE + 读当前值
	// 合并就这样被绕过去了,丢更新又回来了(见 TestConcurrentPatchKeepsBothWrites)。
	upd := store.LinkUpdate{}
	if linkType != cur.LinkType {
		upd.LinkType = &linkType
	}
	if landingSource != cur.LandingSource {
		upd.LandingSource = &landingSource
	}
	if landingURL != cur.LandingURL {
		upd.LandingURL = &landingURL
	}
	if req.TargetURLs != nil {
		targets, err := validTargetURLs(*req.TargetURLs, a.maxTargetURLs())
		if err != nil {
			writeErr(c, http.StatusBadRequest, errValidation, err.Error())
			return
		}
		upd.TargetURLs = &targets
	}
	// 跳转方式:只在请求显式给出时才下发。
	// 「落地页型一律 302」这条不变式由 store 在锁内按生效 linkType 归一
	// (effectiveRedirectStatus),handler 不必再实现一遍。
	if req.RedirectStatus != nil {
		if *req.RedirectStatus != "301" && *req.RedirectStatus != "302" {
			writeErr(c, http.StatusBadRequest, errValidation, "redirectStatus 必须为 301 或 302")
			return
		}
		rs := store.RedirectStatus(*req.RedirectStatus)
		upd.RedirectStatus = &rs
	}
	if req.Status != nil {
		if *req.Status != "enabled" && *req.Status != "disabled" {
			writeErr(c, http.StatusBadRequest, errValidation, "status 必须为 enabled 或 disabled")
			return
		}
		upd.Status = req.Status
	}
	if req.RulesEnabled != nil {
		upd.RulesEnabled = req.RulesEnabled
	}
	if req.DomainIDs != nil {
		ids, err := a.validateLinkDomains(c, t.ID, *req.DomainIDs)
		if err != nil {
			writeAPIError(c, err)
			return
		}
		upd.DomainIDs = &ids
	}
	link, err := a.store.UpdateLink(c.Request.Context(), t.ID, id, upd)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(c, http.StatusNotFound, errNotFound, "link not found")
			return
		}
		if store.IsUniqueViolation(err) {
			writeErr(c, http.StatusConflict, errConflict, "同一域名下短码已存在")
			return
		}
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 切到 redirect 型或 url 来源时删除已上传落地页文件(16)
	if link.LinkType == store.LinkTypeRedirect || link.LandingSource == store.LandingSourceURL {
		a.removeLandingFiles(id)
	}
	writeJSON(c, http.StatusOK, a.withLandingUploaded(link))
}

// restoreLink 还原逻辑删除的短链(由 PATCH {"deletedAt": null} 触发)。
//
// 短码是否还能拿回来由数据库回答:还原会把 link_domains.link_deleted 翻回 false,
// 若该 (domain_id, code) 已被另一条存活短链占用,部分唯一索引直接报 23505 → 409。
// 落地页文件在软删期间一直保留,所以还原后落地页原样可用,不需要重新上传。
func (a *API) restoreLink(c *gin.Context, t *store.Tenant, id int64, req patchLinkReq) {
	if strings.TrimSpace(string(req.DeletedAt)) != "null" {
		writeErr(c, http.StatusBadRequest, errValidation, "deletedAt 只支持显式 null(还原逻辑删除的短链)")
		return
	}
	if err := a.store.RestoreLink(c.Request.Context(), t.ID, id); err != nil {
		if store.IsUniqueViolation(err) {
			writeErr(c, http.StatusConflict, errConflict, "短码已被占用,无法还原该短链")
			return
		}
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	link, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, a.withLandingUploaded(link))
}

func (a *API) handleDeleteLink(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if err := a.store.SoftDeleteLink(c.Request.Context(), t.ID, id); err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	writeNoContent(c)
}

func (a *API) handlePurgeLink(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if err := a.store.PurgeLink(c.Request.Context(), t.ID, id); err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	a.removeLandingFiles(id) // 彻底删除连同落地页文件(16)
	writeNoContent(c)
}

// maxBatchLinkIDs 单次批量删除的短链数量上限(与前端多选交互上限一致)。
const maxBatchLinkIDs = 200

type batchLinkIDsReq struct {
	IDs []int64 `json:"ids"`
}

// parseBatchLinkIDs 解析并校验批量操作请求体:ids 非空、数量 ≤ maxBatchLinkIDs、
// 每项均为正整数;去重并保持请求顺序。非法时写 400 并返回 ok=false。
func parseBatchLinkIDs(c *gin.Context) ([]int64, bool) {
	var req batchLinkIDsReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return nil, false
	}
	if len(req.IDs) == 0 {
		writeErr(c, http.StatusBadRequest, errValidation, "ids 不能为空")
		return nil, false
	}
	if len(req.IDs) > maxBatchLinkIDs {
		writeErr(c, http.StatusBadRequest, errValidation, "单次最多处理 200 个短链")
		return nil, false
	}
	seen := make(map[int64]bool, len(req.IDs))
	ids := make([]int64, 0, len(req.IDs))
	for _, id := range req.IDs {
		if id <= 0 {
			writeErr(c, http.StatusBadRequest, errValidation, "ids 只能包含正整数")
			return nil, false
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, true
}

// handleBatchDeleteLinks POST /api/links/batch-delete — 批量逻辑删除(前端多选)。
// 等价于逐条 SoftDeleteLink:deleted_at 置位,记录/关联/访问明细保留;
// 跨租户/已删除/不存在的 id 静默跳过(幂等),deleted 为实际置位行数。
func (a *API) handleBatchDeleteLinks(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	ids, ok := parseBatchLinkIDs(c)
	if !ok {
		return
	}
	n, err := a.store.SoftDeleteLinks(c.Request.Context(), t.ID, ids)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{"deleted": n})
}

// handleBatchPurgeLinks POST /api/links/batch-purge — 批量物理删除(前端多选)。
// 连同 visits/link_targets/link_domains(库内 ON DELETE CASCADE)与已上传落地页文件;
// 跨租户/不存在的 id 静默跳过(幂等),deleted 为实际删除行数。
// 注意:落地页文件按短链 ID 存放在租户间共享的目录,只能清理本次真正删掉的那些 id,
// 否则跨租户混入的 id 会把他人的落地页文件一并删掉。
func (a *API) handleBatchPurgeLinks(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	ids, ok := parseBatchLinkIDs(c)
	if !ok {
		return
	}
	purged, err := a.store.PurgeLinks(c.Request.Context(), t.ID, ids)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	for _, id := range purged {
		a.removeLandingFiles(id) // 彻底删除连同落地页文件(16)
	}
	writeJSON(c, http.StatusOK, map[string]any{"deleted": len(purged)})
}

// ---------- 访问列表与统计(07) ----------

// ---------- 公开跳转/点击路径的按 IP 限流(issue 07) ----------

// 公开跳转与点击端点没有任何认证,限流器此前只挂在注册/登录那几个认证端点上,
// 于是「对已知短码循环 GET /{code}」是完全免费且不受限的:每次调用都会
//  1. 强制跑一次 ip2region 地理解析(CPU),
//  2. 往 visits 插入一整行明细(DB 写放大 + 表膨胀)。
//
// ADR-0005 接受的是「点击数可能虚增」,并没有同意"用一个短码把 visits 表撑爆"。
const (
	// visitorPerMinute 每个 IP 每分钟允许的公开路径请求数。
	visitorPerMinute = 240
	// visitorWindow 计数窗口。配合 ratelimit.go 的滚动窗口实现,语义是
	// 「任意连续 60 秒内同一 IP 最多 240 次」—— 首次涌入 240 次不会被拒,
	// 但刷量脚本拿不到比正常访客更多的额度。
	visitorWindow = time.Minute
)

// visitorLimiters 按 *API 实例持有访客限流器。
//
// 为什么挂在 map 上而不是 API 结构体字段:API 结构体与限流器字段都在
// internal/httpapi/server.go(本次不可改动的文件)里。新增一个字段会让本文件
// 与那个文件产生耦合,任何并行修改都会撞车;包级 map 则是自包含的。
// key 是 *API(每进程只有一个生产实例,测试里每个 env 一个),value 是限流器本身,
// 状态天然按实例隔离 —— 这点很重要:测试之间若共用一个桶,先跑的用例会把桶打空,
// 后面无关的用例莫名其妙收到 429。
var visitorLimiters sync.Map // *API -> *rateLimiter

// visitorLimiter 返回本实例的访客限流器(惰性创建,进程内复用)。
func (a *API) visitorLimiter() *rateLimiter {
	if v, ok := visitorLimiters.Load(a); ok {
		return v.(*rateLimiter)
	}
	v, _ := visitorLimiters.LoadOrStore(a, newRateLimiter(visitorPerMinute, visitorWindow))
	return v.(*rateLimiter)
}

// allowVisitor 公开跳转/点击路径的按 IP 令牌桶准入判断。
//
// 阈值取舍(240 次/分钟 ≈ 4 次/秒,突发 240):
//
//	· 下限不能再低。运营商 CGNAT、公司出口、校园网都把大量互不相识的人聚合成
//	  一个源 IP;一个热门落地页在同一出口后面可能有几十上百人在点。把阈值压到
//	  个位数/分钟,会把正常访客误伤成 429 —— 那是比刷量严重得多的生产事故。
//	· 上限不能再高。限流的目的是给"写 visits + 跑地理解析"这条链路封顶:
//	  4 次/秒 意味着单个源 IP 每分钟最多给 visits 添 240 行、给 CPU 添 240 次解析,
//	  相对无节制刷量(可达数万次/秒)是 3 个数量级以上的削减,足以让"脚本刷量"
//	  失去意义(它拿不到比正常用户更多的额度)。
//	· 换句话说:正常用户感知不到,脚本刷不动,这就是这组数字的目标。
func (a *API) allowVisitor(c *gin.Context) bool {
	return a.visitorLimiter().Allow(a.clientIPForVisitor(c.Request))
}

// ---------- 落地页静态资源 / SDK 的按 IP 限流 ----------

// landingAssetPerMinute 落地页二级路径(sdk.js 与静态文件)每个 IP 每分钟的上限。
//
// 为什么单列一个更宽松的限流器,而不是复用 visitorPerMinute(240):
// 一次落地页浏览 = 1 次 /{code} + 1 次 /{code}/ + N 次静态资源。资源多一点的
// 落地页(N 到几十)会让 240 的额度被单次浏览就吃掉相当一部分,同一出口
// (CGNAT/公司网)下的正常访客互相挤兑立刻吃 429。
//
// 但它的目的与访客限流一样:这些路径没有任何认证,而每个请求都要做一次
// "域名 + 短码"的 DB 查询(静态文件还要读盘)。没有上限时,单个 IP 可以把
// 查询/读盘放大到无界。1200/分 ≈ 20 次/秒:足够一个含 500 个资源的落地页
// 在一分钟内被加载两次(极端但合法),又给这条链路封了顶。
const landingAssetPerMinute = 1200

// landingAssetLimiters 与 visitorLimiters 同构:按 *API 实例隔离,避免测试之间互相打空桶。
var landingAssetLimiters sync.Map // *API -> *rateLimiter

func (a *API) landingAssetLimiter() *rateLimiter {
	if v, ok := landingAssetLimiters.Load(a); ok {
		return v.(*rateLimiter)
	}
	v, _ := landingAssetLimiters.LoadOrStore(a, newRateLimiter(landingAssetPerMinute, visitorWindow))
	return v.(*rateLimiter)
}

// allowLandingAsset 落地页 sdk.js / 静态文件的准入判断。
func (a *API) allowLandingAsset(c *gin.Context) bool {
	return a.landingAssetLimiter().Allow(a.clientIPForVisitor(c.Request))
}

// visitorGuard 是 allowVisitor 的中间件形态,供公开跳转路由挂载:
//
//	r.GET("/:code", a.visitorGuard(), a.handleRedirect)
//
// `/{code}` 的 handler(handleRedirect)与路由表分别在 internal/httpapi/redirect.go
// 与 internal/httpapi/server.go —— 都是本次不可改动的他人文件,所以这里只把
// 中间件备好,接线由持有那两个文件的人补上。语义与 handleLandingClick 里内联的那份一致。
func (a *API) visitorGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.allowVisitor(c) {
			// tenantID 传 0:取租户自定义 429 页要查一次库,而那正是限流要省掉的成本。
			a.renderVisitorError(c, http.StatusTooManyRequests, 0, nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (a *API) handleListVisits(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if _, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id); err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	// action 可选过滤(redirect / landing_view / click),省略则不过滤;
	// 非法值 400,避免拼错参数时静默返回全量明细
	action := c.Request.URL.Query().Get("action")
	if !store.ValidVisitAction(action) {
		writeErr(c, http.StatusBadRequest, errValidation, "action 必须为 redirect / landing_view / click")
		return
	}
	// outcome 可选过滤(success / failed),省略则不过滤。
	// 前端要"只统计成功访问"时必须显式传:不过滤会把点击行与失败行一起算进去,
	// 同一访客被计两次,而点击与失败都不计入访问次数(见 CONTEXT.md 计数口径)。
	outcome := c.Request.URL.Query().Get("outcome")
	if !store.ValidVisitOutcome(outcome) {
		writeErr(c, http.StatusBadRequest, errValidation, "outcome 必须为 success / failed")
		return
	}
	page, pageSize := pageParams(c)
	items, total, err := a.store.ListVisitsByLinkFiltered(c.Request.Context(), id,
		store.VisitFilter{Action: action, Outcome: outcome}, page, pageSize)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if items == nil {
		items = []*store.Visit{}
	}
	writeJSON(c, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (a *API) handleLinkStats(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	link, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id)
	if err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "link not found")
		return
	}
	// visits 与 clicks 必须同源同期,否则保留期清理后 CTR 会虚高到 100% 以上:
	//
	//   - visits      来自 visits 表(action IN redirect/landing_view AND outcome=success),
	//                  受 JANUS_VISIT_RETENTION 约束;
	//   - clickVisits 同样来自 visits 表(action='click' AND outcome=success),同窗口;
	//   - clicks      是 links.clicks 这个**永久计数器**,不随保留期衰减。
	//
	// 原先这里回的是 clicks(永久),与 visits(会衰减)配对 —— 清理后台面 CTR 必然虚高,
	// 这正是 store.Link.ClickVisits 注释里明令禁止的组合。分子分母统一走 visits 表。
	//
	// 两个都返回:clicks 是契约字段(永久累计,租户看 lifetime 总量),
	// clickVisits 才是算 CTR 的那个。GetLinkByID 已把两者都填好,直接取。
	writeJSON(c, http.StatusOK, map[string]any{
		"visits":      link.Visits,
		"clicks":      link.ClickVisits,
		"clicksTotal": link.Clicks,
	})
}

// ---------- 错误封装 ----------

type apiErr struct {
	status  int
	code    string
	message string
	details any
}

func (e apiErr) Error() string { return e.message }

func writeAPIError(c *gin.Context, err error) {
	var ae apiErr
	if errors.As(err, &ae) {
		if ae.details != nil {
			writeErrDetails(c, ae.status, ae.code, ae.message, ae.details)
		} else {
			writeErr(c, ae.status, ae.code, ae.message)
		}
		return
	}
	writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
}
