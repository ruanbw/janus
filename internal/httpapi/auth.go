package httpapi

// 02/03 — 认证:注册、登录/登出、me、改密(会话 cookie + CSRF)。

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"janus/internal/domain"
	"janus/internal/store"
)

const (
	minPasswordLen = 8
	// maxPasswordLen 是 bcrypt 的硬上限:golang.org/x/crypto/bcrypt 对超过 72 字节的
	// 密码直接返回 ErrPasswordTooLong。原先只校验下界,注册/改密提交长密码
	// 会走到 GenerateFromPassword 拿 500 —— 用户看到的是"服务器错误",而不是
	// "密码太长"。这里显式挡住,返回 E_VALIDATION。
	maxPasswordLen = 72
)

// dummyPasswordHash 用于不存在账号时的假 bcrypt 比较,抹平"账号不存在/密码错误"
// 的响应时间差,降低用户名枚举侧信道(登录与 token)。
var dummyPasswordHash = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("janus-dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		panic("bcrypt unavailable: " + err.Error())
	}
	return h
}()

// validPasswordLen 校验密码字节长度(bcrypt 的口径是字节,不是 rune)。
func validPasswordLen(p string) bool {
	return len(p) >= minPasswordLen && len(p) <= maxPasswordLen
}

const passwordLenMsg = "密码长度需为 8-72 字节"

// ---------- 注册 ----------

type registerReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Slug     string `json:"slug"`
}

// handleRegister 注册:校验 username 唯一且不与既有域名 FQDN 冲突;
// 创建 active 租户与平台默认域名(status=active,无需邮箱验证)。
func (a *API) handleRegister(c *gin.Context) {
	if !a.rateLimit(c, a.registerRate) {
		return
	}
	var req registerReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	username := domain.NormalizeUsername(req.Username)
	if !domain.IsValidUsername(username) {
		writeErrDetails(c, http.StatusBadRequest, errValidation, "用户名非法(3-32 字符,仅小写字母/数字/下划线/连字符,首尾须为字母或数字)", map[string]string{"field": "username"})
		return
	}
	slug := strings.TrimSpace(req.Slug)
	if !domain.IsValidSlug(slug) {
		writeErrDetails(c, http.StatusBadRequest, errValidation, "slug 非法(小写字母/数字开头结尾,可含连字符,1-63 字符)", map[string]string{"field": "slug"})
		return
	}
	if !validPasswordLen(req.Password) {
		writeErrDetails(c, http.StatusBadRequest, errValidation, passwordLenMsg, map[string]string{"field": "password"})
		return
	}
	// 平台保留子域不得被租户默认域名占用:app.<平台域名> 承载后台站点。
	if slug == "app" {
		writeErrDetails(c, http.StatusBadRequest, errValidation, "slug 与平台保留域名冲突", map[string]string{"field": "slug"})
		return
	}
	ctx := c.Request.Context()

	if _, err := a.store.GetTenantByUsername(ctx, username); err == nil {
		writeErr(c, http.StatusConflict, errConflict, "用户名已被注册")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	slug = strings.ToLower(slug)
	if _, err := a.store.GetTenantBySlug(ctx, slug); err == nil {
		writeErr(c, http.StatusConflict, errConflict, "slug 已被占用")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// slug 与既有域名 FQDN 冲突(平台默认域名或他人自有域名)
	for _, fqdn := range []string{slug + "." + a.cfg.PlatformDomain, slug} {
		exists, err := a.store.DomainFQDNExists(ctx, fqdn)
		if err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
		if exists {
			writeErr(c, http.StatusConflict, errConflict, "slug 与既有域名冲突")
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 建租户与建默认域名必须原子:中间失败若留下租户行,用户重试注册只会拿到 409,
	// 而默认域名不存在,无法自助恢复。两步同事务后,任一失败整组回滚(见 store.RegisterTenant)。
	tenant, err := a.store.RegisterTenant(ctx, username, string(hash), slug,
		slug+"."+a.cfg.PlatformDomain, false)
	if err != nil {
		// TOCTOU:预检(DomainFQDNExists / GetTenantBySlug)与事务内插入之间有窗口,
		// 并发的另一路注册可能抢先占位 username、slug 或默认域名。此时唯一索引是最终
		// 裁决者,撞车是"冲突"而不是"服务器错误" —— 漏成 500 会让用户看到"服务器
		// 错误",重试又被 409 挡死。
		if store.IsUniqueViolation(err) {
			writeErr(c, http.StatusConflict, errConflict, "用户名、slug 或默认域名已被占用")
			return
		}
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 注册即 active 且直接登录:前端注册后不再经过登录页,
	// 所以这里要随响应下发会话 cookie(与 handleLogin 同一路径)。
	// 平台默认域名的证书预签发探活原先由邮箱验证成功触发,现在在这里触发。
	go a.probeTenantDefaultDomain(tenant.ID)
	token, tokenHash := newToken()
	csrf, _ := newToken()
	sess, err := a.store.CreateSession(ctx, tenant.ID, tokenHash, csrf, a.cfg.SessionTTL)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	a.setSessionCookies(c, token, sess.CSRFToken, a.cfg.SessionTTL)
	writeJSON(c, http.StatusCreated, tenant)
}

func (a *API) probeTenantDefaultDomain(tenantID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tenant, err := a.store.GetTenantByID(ctx, tenantID)
	if err != nil || tenant.DefaultDomain == "" {
		return
	}
	if domain.ProbeCert(ctx, tenant.DefaultDomain) {
		_ = a.store.SetDomainCertStatusByFQDN(ctx, tenant.DefaultDomain, "issued")
	} else {
		_ = a.store.SetDomainCertStatusByFQDN(ctx, tenant.DefaultDomain, "failed")
	}
}

// ---------- 登录 / 登出 / me ----------

// verifyCredentials 是 handleLogin 与 handleToken 共用的凭据校验(成功返回租户,
// 失败时已写好错误响应)。
//
// 顺序刻意是「先跑一次 bcrypt,再看租户状态」。原先 switch tenant.Status 排在最前,
// 于是 banned 租户 ~1ms 就返回,不存在的账号却要付一次 dummy bcrypt
// (~60-100ms)—— 攻击者据此就能区分「用户名不存在」与「存在但已封禁」,
// 配合注册接口的 409 即可枚举租户。现在无论账号处于什么状态,响应时间都落在
// 同一个量级上,而对外的 401/403 与错误文案保持不变。
//
// 这里也是**账号级失败退避**的统一入口(见 auth_limit.go):每 IP 限流挡不住
// "换 IP 打同一个账号"的撞库,所以先把"该账号已被退避"挡在 bcrypt 之前,再按
// 校验结果记账(成功清零、失败累加)。login 与 token 共用本函数,不会出现
// "换个端点绕过退避"。
func (a *API) verifyCredentials(c *gin.Context, username, password string) (*store.Tenant, bool) {
	failures := a.loginFailures
	if ok, retryAfter := failures.Allow(username); !ok {
		writeAccountThrottled(c, retryAfter)
		return nil, false
	}
	tenant, ok := a.verifyCredentialsChecked(c, username, password)
	if !ok {
		failures.Fail(username)
		return nil, false
	}
	failures.Succeed(username)
	return tenant, true
}

func (a *API) verifyCredentialsChecked(c *gin.Context, username, password string) (*store.Tenant, bool) {
	ctx := c.Request.Context()
	tenant, err := a.store.GetTenantByUsername(ctx, username)
	if err != nil {
		// 等时化:账号不存在也执行一次 bcrypt 比较(bcrypt 先于分类,两条路径同价)。
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
		if errors.Is(err, store.ErrNotFound) {
			writeErr(c, http.StatusUnauthorized, errUnauth, "用户名或密码错误")
		} else {
			// 数据库故障绝不能伪装成"账号不存在":那样一次连库抖动会表现成
			// "全员密码错误",既误导用户也让排障无从下手。
			slog.Error("verifyCredentials: 查询租户失败", "err", err)
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		}
		return nil, false
	}

	pwHash, hashErr := a.store.TenantPasswordHash(ctx, tenant.ID)

	// 恒定花掉一次 bcrypt:有密码就真比,没有就比 dummy。
	credOK := false
	if hashErr == nil && pwHash != nil {
		credOK = bcrypt.CompareHashAndPassword([]byte(*pwHash), []byte(password)) == nil
	} else {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
	}

	// 状态检查放在 bcrypt 之后(时序理由见函数注释),对外语义不变。
	switch tenant.Status {
	case "pending":
		writeErr(c, http.StatusUnauthorized, errUnauth, "账号未激活")
		return nil, false
	case "banned":
		writeErr(c, http.StatusForbidden, errForbidden, "账号已被封禁")
		return nil, false
	}

	if !credOK {
		writeErr(c, http.StatusUnauthorized, errUnauth, "用户名或密码错误")
		return nil, false
	}
	return tenant, true
}

type loginReq struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RememberMe *bool  `json:"rememberMe"` // 契约调整:可选;true/省略 30 天,false 24 小时
}

func (a *API) handleLogin(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req loginReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	username := domain.NormalizeUsername(req.Username)
	tenant, ok := a.verifyCredentials(c, username, req.Password)
	if !ok {
		return
	}
	ttl := a.cfg.SessionTTL
	if req.RememberMe != nil && !*req.RememberMe {
		ttl = a.cfg.SessionTTLShort
	}
	token, tokenHash := newToken()
	csrf, _ := newToken()
	sess, err := a.store.CreateSession(c.Request.Context(), tenant.ID, tokenHash, csrf, ttl)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	a.setSessionCookies(c, token, sess.CSRFToken, ttl)
	writeJSON(c, http.StatusOK, tenant)
}

func (a *API) handleLogout(c *gin.Context) {
	_, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	if ck, err := c.Request.Cookie(sessionCookieName); err == nil {
		_ = a.store.DeleteSession(c.Request.Context(), hashToken(ck.Value))
	}
	a.clearSessionCookies(c)
	writeNoContent(c)
}

func (a *API) handleMe(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	writeJSON(c, http.StatusOK, t)
}

// ---------- 修改密码(03) ----------

type changePasswordReq struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// checkOldPassword 校验旧密码,并把"密码不对"与"查不动库"分开:
// ok=true 表示旧密码正确;dbErr=true 表示查询本身失败(调用方应回 500 + 记日志)。
//
// 原先三种情况(err != nil、pwHash == nil、bcrypt 比较失败)被 || 混为一谈,
// 数据库故障会被当成"旧密码错误"回 400 —— 用户看到的是"我密码打错了",
// 排障却看不到任何库侧线索。
func checkOldPassword(pwHash *string, err error, oldPassword string) (ok, dbErr bool) {
	if err != nil {
		return false, true
	}
	if pwHash == nil {
		// 无密码哈希正常不会走到这里(调用方已按 must_change_password 短路);
		// 保守按"旧密码错误"处理,不泄露内部状态。
		return false, false
	}
	return bcrypt.CompareHashAndPassword([]byte(*pwHash), []byte(oldPassword)) == nil, false
}

// handleChangePassword 修改密码;被强制改密的租户(首登种子口令)可省略旧密码。
func (a *API) handleChangePassword(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	var req changePasswordReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	if !validPasswordLen(req.NewPassword) {
		writeErr(c, http.StatusBadRequest, errValidation, "新密码"+passwordLenMsg)
		return
	}
	if !t.MustChangePassword {
		// 旧密码猜测限流:复用登录的账号级退避机制,但用独立命名空间 ——
		// 改密失败不该把同一账号的登录一起锁死(反之亦然)。
		key := "chpw:" + t.Username
		failures := a.loginFailures
		if allowed, retryAfter := failures.Allow(key); !allowed {
			writeAccountThrottled(c, retryAfter)
			return
		}
		pwHash, err := a.store.TenantPasswordHash(c.Request.Context(), t.ID)
		oldOK, dbErr := checkOldPassword(pwHash, err, req.OldPassword)
		if dbErr {
			slog.Error("change-password: 读取密码哈希失败", "tenantId", t.ID, "err", err)
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
		if !oldOK {
			failures.Fail(key)
			writeErr(c, http.StatusBadRequest, errValidation, "旧密码错误")
			return
		}
		failures.Succeed(key)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 改密 = 自增 token_version(此前签发的 JWT 立即作废) + 清除 must_change_password。
	// 三者与"删 sessions 表"一起覆盖两种认证方式;同一事务,避免半更新导致租户被
	// 强制改密闸门永久挡在门外。
	if err := a.store.SetTenantPasswordAndBumpTokenVersion(c.Request.Context(), t.ID, string(hash)); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 改密后吊销其他终端会话,保留当前会话(无会话的 Bearer 请求则吊销全部浏览器会话)。
	if sess != nil {
		_ = a.store.DeleteSessionsByTenantExcept(c.Request.Context(), t.ID, sess.TokenHash)
	} else {
		_ = a.store.DeleteSessionsByTenant(c.Request.Context(), t.ID)
	}
	writeNoContent(c)
}

// ---------- API Bearer(JWT)token 端点 ----------

type tokenReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// tokenResp 契约:签发成功返回 accessToken / tokenType / expiresIn(秒),不写 cookie。
type tokenResp struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int64  `json:"expiresIn"`
}

// handleToken 签发 API Bearer JWT:校验逻辑与 handleLogin 完全一致
// (username 归一化、pending→401、banned→403、错误凭据→401 等时化 dummy bcrypt),
// 成功后按租户 IsSuperAdmin 计算角色并用 JANUS_JWT_SECRET/JWTTTL 签发 HS256 JWT。
func (a *API) handleToken(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req tokenReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	username := domain.NormalizeUsername(req.Username)
	tenant, ok := a.verifyCredentials(c, username, req.Password)
	if !ok {
		return
	}
	role := "tenant"
	if tenant.IsSuperAdmin {
		role = "superadmin"
	}
	// 复用 server 构建的 jwtMgr(密钥回退逻辑一致:JANUS_JWT_SECRET 为空时用启动期随机密钥,
	// 与 authenticate 的校验密钥保持一致,避免签发的 token 被 401 拒绝)。
	tokenVersion, err := a.store.TenantTokenVersion(c.Request.Context(), tenant.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	token, err := a.jwtMgr.Issue(tenant.ID, role, tokenVersion)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, tokenResp{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(a.cfg.JWTTTL.Seconds()),
	})
}
