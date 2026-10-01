package httpapi

// 04 — 域名管理:添加(真实 DNS 校验)→ 激活 → 证书预签发探活;重试队列;停用/恢复;删除。
//
// 激活判据是"DNS TXT 一次性挑战 + A/AAAA 指向本机"(ADR-0002 的闸门从"解析到本机"
// 收紧为"证明归属"),所以添加域名后域名先是 pending,并带回一条待发布的 TXT 指引;
// 租户补完 DNS 后点"重新校验"才激活。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cloak/internal/domain"
	"cloak/internal/store"
)

// validFQDN 校验域名格式(实现在 domain 包,与入库归一化共用同一份规则)。
func validFQDN(s string) bool { return domain.IsValidFQDN(s) }

// domainView 是租户侧看到的域名视图:在 store.Domain 之外补上"TXT 该怎么填"的指引。
//
// 前端不需要自己拼 "_cloak-verify." 前缀,也不需要判断"这个域名该不该做归属证明":
// verifyRecord 非空就是"请把 verifyValue 发布到 verifyRecord 这条 TXT 上"。
type domainView struct {
	*store.Domain
	// VerifyRecord 待添加的 TXT 主机名(如 _cloak-verify.example.com);无需证明时为空。
	VerifyRecord string `json:"verifyRecord,omitempty"`
	// VerifyValue 待添加的 TXT 值(与 Domain.VerifyToken 同值,换个面向 DNS 的名字)。
	VerifyValue string `json:"verifyValue,omitempty"`
	// VerifyNeeds 下一步该做什么:need_dns(先把 A 记录指向本机)/ need_txt(补 TXT)/ verified。
	// 只有刚刚跑过一次校验的请求(添加/重检/恢复)才带这个字段。
	VerifyNeeds domain.VerifyStatus `json:"verifyNeeds,omitempty"`
	// ServerIP A 记录该指向的地址;PlatformDomain 供前端拼 TXT 主机名做兜底展示。
	ServerIP       string `json:"serverIp,omitempty"`
	PlatformDomain string `json:"platformDomain,omitempty"`
}

// domainViewOf 把 store 记录渲染成带指引的视图。
// needs 非空时(刚刚校验过)一并带上三态结果。
func (a *API) domainViewOf(d *store.Domain, needs domain.VerifyStatus) domainView {
	v := domainView{
		Domain:         d,
		VerifyNeeds:    needs,
		ServerIP:       a.cfg.ServerPublicIP,
		PlatformDomain: a.cfg.PlatformDomain,
	}
	// 平台默认域名靠泛解析,不需要归属证明,因此不展示 TXT 指引。
	if d != nil && d.Origin == "self" && d.VerifyToken != "" {
		v.VerifyRecord = domain.VerifyRecordName(d.FQDN)
		v.VerifyValue = d.VerifyToken
	}
	return v
}

func (a *API) handleListDomains(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	domains, err := a.store.ListDomainsByTenant(c.Request.Context(), t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	out := make([]domainView, 0, len(domains))
	for _, d := range domains {
		out = append(out, a.domainViewOf(d, ""))
	}
	writeJSON(c, http.StatusOK, out)
}

type createDomainReq struct {
	FQDN        string `json:"fqdn"`
	Description string `json:"description"`
}

// errDomainTaken 事务内判定域名已被占用(返回 409)。
var errDomainTaken = errors.New("domain already taken")

func (a *API) handleCreateDomain(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	var req createDomainReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	// 入库前统一形态:小写 + 去尾点。否则 "a.com" 与 "a.com." 在 UNIQUE 索引下
	// 是两行,同一个真实主机能被两个租户各绑一份、各计一份配额。
	fqdn := store.NormalizeFQDN(req.FQDN)
	if !validFQDN(fqdn) {
		writeErr(c, http.StatusBadRequest, errValidation, "域名格式非法")
		return
	}
	desc := strings.TrimSpace(req.Description)
	if len([]rune(desc)) > domain.MaxDomainDescriptionLen {
		writeErr(c, http.StatusBadRequest, errValidation, "描述过长(最多 200 字)")
		return
	}
	// 保留判断必须是"等于平台域名 或 是其子域":只做全等比较时,
	// www.<平台域名> 与 mail.<平台域名> 都能绕过,而后者因为平台泛解析存在,
	// 会通过 DNS 校验并被授权端点放行 —— 租户可以占住平台自己的子域。
	if domain.IsReservedFQDN(fqdn, a.cfg.PlatformDomain) {
		writeErr(c, http.StatusBadRequest, errValidation, "平台域名及其子域为保留域名,不可添加")
		return
	}
	ctx := c.Request.Context()
	// 事务外预检查只为给出更友好的 409;真正的判定在事务内(见下)。
	if exists, err := a.store.DomainFQDNExists(ctx, fqdn); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	} else if exists {
		writeErr(c, http.StatusConflict, errConflict, "域名已被占用")
		return
	}
	// 归属校验放在事务之前:校验要做 DNS 查询(最长数秒),压在租户行锁里等于
	// 一条可被慢 DNS 放大的串行化长事务。此时还没有 token,TXT 判不了,
	// 所以只能得到"A 记录指向了没有" —— 恰好也是租户要做的第一步。
	res, err := a.dns.Verify(ctx, fqdn, "")
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	token, err := domain.MintVerifyToken()
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 配额与唯一性放进同一个事务:原先 Usage() 读一次、CreateDomain 插一次,
	// 中间没有事务也没有锁 —— UNIQUE 索引能挡住"同域名被绑两次",
	// 挡不住"同一租户并发创建多个不同域名突破 max_domains"。
	var d *store.Domain
	err = a.store.WithQuotaInTx(ctx, t.ID, store.QuotaDomains, func(tx *gorm.DB) error {
		if exists, err := store.DomainFQDNExistsWithDB(tx, fqdn); err != nil {
			return err
		} else if exists {
			return errDomainTaken
		}
		created, err := store.CreateDomainChalleged(tx, t.ID, fqdn, token, desc)
		if err != nil {
			return err
		}
		d = created
		return nil
	})
	if err != nil {
		a.writeDomainCreateError(c, err)
		return
	}
	// 记录本次校验时间:worker 的重试队列据此调度(退避),不再是无条件全表扫。
	if err := a.store.MarkDomainDNSChecked(ctx, d.ID); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusCreated, a.domainViewOf(d, res.Status))
}

// writeDomainCreateError 把创建域名的内部错误映射成对外契约里的错误码。
func (a *API) writeDomainCreateError(c *gin.Context, err error) {
	var qe *store.QuotaError
	switch {
	case errors.Is(err, errDomainTaken):
		writeErr(c, http.StatusConflict, errConflict, "域名已被占用")
	case errors.As(err, &qe) && qe.Kind == store.QuotaDomains:
		writeErrDetails(c, http.StatusForbidden, errDomainQuota,
			"域名数量已达上限", map[string]any{"usage": qe.Usage})
	case store.IsUniqueViolation(err):
		writeErr(c, http.StatusConflict, errConflict, "域名已被占用")
	default:
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
	}
}

func (a *API) handleGetDomain(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	d, err := a.store.GetDomainByID(c.Request.Context(), id)
	if err != nil || d.TenantID != t.ID {
		writeErr(c, http.StatusNotFound, errNotFound, "domain not found")
		return
	}
	writeJSON(c, http.StatusOK, a.domainViewOf(d, ""))
}

// handleRecheckDomain 手动触发归属重新校验(202)。
//
// 挑战的轮换规则:已激活的域名沿用已验证的 token(重检必须幂等,否则点一次
// "重新校验"就会把租户的 TXT 记录作废);未激活的域名在挑战缺失或超期时换新
// token —— 挑战是一次性的:过期即失效,复活必须重新发布。
func (a *API) handleRecheckDomain(c *gin.Context) {
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
	d, err := a.store.GetDomainByID(c.Request.Context(), id)
	if err != nil || d.TenantID != t.ID {
		writeErr(c, http.StatusNotFound, errNotFound, "domain not found")
		return
	}
	if d.Origin == "self" && needsNewChallenge(d, a.cfg.DNSMaxAge) {
		token, err := domain.MintVerifyToken()
		if err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
		if err := a.store.MintVerifyToken(c.Request.Context(), d.ID, token); err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
		// 复活:终态/失败域名重新进入重试队列。dns_checked_at 一起重置,
		// 否则一个很久以前创建的域名会因"距上次校验太久"被立刻打回终态。
		if err := a.store.ReviveDomain(c.Request.Context(), d.ID); err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
		d, err = a.store.GetDomainByID(c.Request.Context(), d.ID)
		if err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
	}
	a.enqueueVerify(d.ID, d.FQDN, d.VerifyToken)
	writeJSON(c, http.StatusAccepted, a.domainViewOf(d, ""))
}

// needsNewChallenge 判断重检时是否要换一枚新的 TXT 挑战(纯逻辑)。
func needsNewChallenge(d *store.Domain, maxAge time.Duration) bool {
	if d.Origin != "self" {
		return false
	}
	// 已激活:沿用已验证的证据(它同时是低频复检的比对基准)。
	if d.Status == "active" {
		return false
	}
	if d.VerifyToken == "" || d.VerifyTokenCreatedAt == nil {
		return true
	}
	return maxAge > 0 && time.Since(*d.VerifyTokenCreatedAt) > maxAge
}

// enqueueVerify 把一次归属校验排进有界任务队列(按域名 ID 去重)。
//
// 原先是 `go a.recheckDomain(...)`:每次请求一个 goroutine、一个 15s 超时、
// 内含 DNS 查询与 HTTPS 探活,而 /api/domains* 没有限流 —— 并发 N 次就是 N 倍
// 出网请求。队列并发固定,同一域名同时只跑一个。
func (a *API) enqueueVerify(id int64, fqdn, token string) {
	a.dns.Tasks().Submit("domain-verify:"+strconv.FormatInt(id, 10), func(context.Context) {
		a.verifyDomain(id, fqdn, token)
	})
}

// verifyDomain 后台做一次归属校验并落库(DNS 查询 + HTTPS 探活都在这里)。
func (a *API) verifyDomain(id int64, fqdn, token string) {
	if token == "" {
		// 调用方没带 token(worker 之外的老调用路径):从库里取当前挑战。
		d, err := a.store.GetDomainByID(context.Background(), id)
		if err != nil {
			return
		}
		token = d.VerifyToken
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := a.dns.Verify(ctx, fqdn, token)
	if err != nil {
		return
	}
	if res.Verified() {
		if err := a.store.ActivateDomainVerified(ctx, id); err == nil {
			a.probeDomainAsync(id)
		}
		return
	}
	d, err := a.store.GetDomainByID(ctx, id)
	if err != nil {
		return
	}
	// 校验失败:未激活的域名保持当前状态(继续进重试队列);
	// 已激活的域名意味着归属不再成立 → 降级,授权端点随即拒绝它。
	if d.Status == "failed" || d.Status == "pending" {
		_ = a.store.MarkDomainDNSChecked(ctx, id)
	} else if d.Status == "active" && d.Origin == "self" {
		_ = a.store.DegradeDomain(ctx, id)
	}
}

type patchDomainReq struct {
	Status string `json:"status"`
}

// handlePatchDomain 停用/恢复域名。平台默认域名可停用(契约注明),不可删除。
func (a *API) handlePatchDomain(c *gin.Context) {
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
	var req patchDomainReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	if req.Status != "stopped" && req.Status != "active" {
		writeErr(c, http.StatusBadRequest, errValidation, "status must be stopped or active")
		return
	}
	d, err := a.store.GetDomainByID(c.Request.Context(), id)
	if err != nil || d.TenantID != t.ID {
		writeErr(c, http.StatusNotFound, errNotFound, "domain not found")
		return
	}
	// 恢复(active)前要求归属仍然成立:同步做一次 TXT + A/AAAA 校验。
	// "active"是一个持续状态,不能只靠"当初验证过一次"就恢复服务 ——
	// 域名到期被他人注册并指向本机后,"恢复"按钮就是一条绕过闸门的路。
	// 平台默认域名除外(泛域名解析由部署者配置,租户无权改动平台 DNS)。
	if req.Status == "active" && d.Status != "active" && d.Origin == "self" {
		res, err := a.dns.Verify(c.Request.Context(), d.FQDN, d.VerifyToken)
		if err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
		if !res.Verified() {
			// details 直接带上 TXT 指引:租户在这一步就能看到该填什么。
			writeErrDetails(c, http.StatusConflict, errConflict,
				verifyNeedsMessage(res.Status), a.domainViewOf(d, res.Status))
			return
		}
	}
	if err := a.store.SetDomainStatus(c.Request.Context(), id, req.Status); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if req.Status == "active" {
		// 恢复激活后确保证书探活
		a.probeDomainAsync(id)
	}
	updated, err := a.store.GetDomainByID(c.Request.Context(), id)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, a.domainViewOf(updated, ""))
}

// verifyNeedsMessage 把三态校验结果翻译成租户看得懂的下一步。
func verifyNeedsMessage(s domain.VerifyStatus) string {
	switch s {
	case domain.VerifyNeedTXT:
		return "归属未证明:请按 details 里的 verifyRecord / verifyValue 添加 TXT 记录后重试"
	case domain.VerifyNeedDNS:
		return "DNS 未指向本服务器:请添加 A/AAAA 记录指向 details.serverIp 后重试"
	default:
		return "域名归属校验未通过"
	}
}

// handleDeleteDomain 删除域名:平台默认域名 400;存在未删除短链 409;否则物理删除。
func (a *API) handleDeleteDomain(c *gin.Context) {
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
	d, err := a.store.GetDomainByID(c.Request.Context(), id)
	if err != nil || d.TenantID != t.ID {
		writeErr(c, http.StatusNotFound, errNotFound, "domain not found")
		return
	}
	// 计数与删除在同一个事务里完成(store.DeleteDomainIfUnused),并且事务内
	// SELECT ... FOR UPDATE 锁住该域名行:原先"先 count 再 detach"是两次独立
	// 调用,新建短链的关联行会被 detach 连带删掉,短链存活但零域名、不可达、
	// 仍占短链配额,而外键方向是 link_domains → domains 的 CASCADE,不会报错。
	purged, links, err := a.store.DeleteDomainIfUnused(c.Request.Context(), id)
	switch {
	case errors.Is(err, store.ErrPlatformDomain):
		writeErr(c, http.StatusBadRequest, errValidation, "平台默认域名不可删除(可停用)")
		return
	case errors.Is(err, store.ErrNotFound):
		writeErr(c, http.StatusNotFound, errNotFound, "domain not found")
		return
	case err != nil:
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if links > 0 {
		writeErrDetails(c, http.StatusConflict, errDomainInUse,
			"该域名下仍有未删除的短链,请先解除关联或删除短链",
			map[string]any{"links": links})
		return
	}
	// 域名删除会物理清除"仅关联该域名"的已删除短链,连同其落地页文件一并清理。
	for _, linkID := range purged {
		a.removeLandingFiles(linkID)
	}
	writeNoContent(c)
}

// probeDomainAsync 后台触发证书预签发探活(HTTPS 访问触发 Caddy on-demand 签发)。
func (a *API) probeDomainAsync(id int64) {
	a.dns.Tasks().Submit("domain-probe:"+strconv.FormatInt(id, 10), func(context.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d, err := a.store.GetDomainByID(ctx, id)
		if err != nil {
			return
		}
		if domain.ProbeCert(ctx, d.FQDN) {
			_ = a.store.SetDomainCertStatus(ctx, id, "issued")
		} else {
			_ = a.store.SetDomainCertStatus(ctx, id, "failed")
		}
	})
}
