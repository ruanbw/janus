package httpapi

// 02/03 — 认证:注册、邮箱验证、登录/登出、me、改密、忘记/重置密码(会话 cookie + CSRF)。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"cloak/internal/domain"
	"cloak/internal/store"
)

const minPasswordLen = 8

// dummyPasswordHash 用于不存在邮箱时的假 bcrypt 比较,抹平"账号不存在/密码错误"
// 的响应时间差,降低邮箱枚举侧信道(登录与忘记密码)。
var dummyPasswordHash = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("cloak-dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		panic("bcrypt unavailable: " + err.Error())
	}
	return h
}()

func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

func validPasswordLen(p string) bool { return len(p) >= minPasswordLen }

// ---------- 注册 ----------

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Slug     string `json:"slug"`
}

// handleRegister 注册:校验 slug 唯一且不与既有域名 FQDN 冲突;
// 创建租户(pending)与平台默认域名;控制台 mailer 输出验证链接。
func (a *API) handleRegister(c *gin.Context) {
	if !a.rateLimit(c, a.registerRate) {
		return
	}
	var req registerReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	slug := strings.TrimSpace(req.Slug)
	if !validEmail(email) {
		writeErr(c, http.StatusBadRequest, errValidation, "邮箱格式非法")
		return
	}
	if !domain.IsValidSlug(slug) {
		writeErrDetails(c, http.StatusBadRequest, errValidation, "slug 非法(小写字母/数字开头结尾,可含连字符,1-63 字符)", map[string]string{"field": "slug"})
		return
	}
	if !validPasswordLen(req.Password) {
		writeErrDetails(c, http.StatusBadRequest, errValidation, "密码至少 8 个字符", map[string]string{"field": "password"})
		return
	}
	// 平台保留域名(app.<平台域名> 承载后台)不得被租户默认域名占用
	if slug+"."+a.cfg.PlatformDomain == a.cfg.PlatformDomain ||
		slug+"."+a.cfg.PlatformDomain == "app."+a.cfg.PlatformDomain {
		writeErrDetails(c, http.StatusBadRequest, errValidation, "slug 与平台保留域名冲突", map[string]string{"field": "slug"})
		return
	}
	ctx := c.Request.Context()

	if _, err := a.store.GetTenantByEmail(ctx, email); err == nil {
		writeErr(c, http.StatusConflict, errConflict, "邮箱已被注册")
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
	tenant, err := a.store.CreateTenant(ctx, email, string(hash), slug, false)
	if err != nil {
		if store.IsUniqueViolation(err) {
			writeErr(c, http.StatusConflict, errConflict, "邮箱或 slug 已被占用")
			return
		}
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if _, err := a.store.CreatePlatformDomain(ctx, tenant.ID, slug+"."+a.cfg.PlatformDomain); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 重新获取租户(含默认域名)
	if tenant, err = a.store.GetTenantByID(ctx, tenant.ID); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 验证邮件(控制台 mailer)
	token, tokenHash := newToken()
	if err := a.store.CreateEmailToken(ctx, tenant.ID, tokenHash, "verify", a.cfg.VerifyTokenTTL); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if err := a.mailer.SendVerifyEmail(email, token); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusCreated, tenant)
}

type verifyEmailReq struct {
	Token string `json:"token"`
}

// handleVerifyEmail 邮箱验证:成功后租户转 active,并触发默认域名证书预签发探活。
func (a *API) handleVerifyEmail(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req verifyEmailReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeErr(c, http.StatusBadRequest, errValidation, "token required")
		return
	}
	tenantID, err := a.store.ConsumeEmailToken(c.Request.Context(), hashToken(strings.TrimSpace(req.Token)), "verify")
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(c, http.StatusBadRequest, errValidation, "无效或已使用的验证 token")
		} else {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		}
		return
	}
	// 已封禁租户不得凭旧验证 token 复活
	tenant, err := a.store.GetTenantByID(c.Request.Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(c, http.StatusBadRequest, errValidation, "无效或已使用的验证 token")
		} else {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		}
		return
	}
	if tenant.Status == "banned" {
		writeErr(c, http.StatusBadRequest, errValidation, "无效或已使用的验证 token")
		return
	}
	if err := a.store.VerifyTenant(c.Request.Context(), tenantID); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 触发平台默认域名证书预签发探活(active 域名由后台任务补探活,这里立即触发)
	go a.probeTenantDefaultDomain(tenantID)
	writeJSON(c, http.StatusOK, map[string]string{"status": "ok"})
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

type loginReq struct {
	Email      string `json:"email"`
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
	email := strings.ToLower(strings.TrimSpace(req.Email))
	tenant, err := a.store.GetTenantByEmail(c.Request.Context(), email)
	if err != nil {
		// 等时化:账号不存在也执行一次 bcrypt 比较
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(req.Password))
		writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
		return
	}
	switch tenant.Status {
	case "pending":
		writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱未验证,请查收验证邮件")
		return
	case "banned":
		writeErr(c, http.StatusForbidden, errForbidden, "账号已被封禁")
		return
	}
	if !tenant.FirstLoginSetup {
		pwHash, err := a.store.TenantPasswordHash(c.Request.Context(), tenant.ID)
		if err != nil || pwHash == nil {
			writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(*pwHash), []byte(req.Password)) != nil {
			writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
			return
		}
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
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	_ = t
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

// ---------- 修改密码 / 忘记密码 / 重置密码(03) ----------

type changePasswordReq struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// handleChangePassword 修改密码;超管首次登录(无密码)可省略旧密码。
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
		writeErr(c, http.StatusBadRequest, errValidation, "新密码至少 8 个字符")
		return
	}
	if !t.FirstLoginSetup {
		pwHash, err := a.store.TenantPasswordHash(c.Request.Context(), t.ID)
		if err != nil || pwHash == nil || bcrypt.CompareHashAndPassword([]byte(*pwHash), []byte(req.OldPassword)) != nil {
			writeErr(c, http.StatusBadRequest, errValidation, "旧密码错误")
			return
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if err := a.store.SetTenantPassword(c.Request.Context(), t.ID, string(hash)); err != nil {
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

type forgotPasswordReq struct {
	Email string `json:"email"`
}

// handleForgotPassword 始终返回 202,不泄露邮箱存在性。
func (a *API) handleForgotPassword(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req forgotPasswordReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if tenant, err := a.store.GetTenantByEmail(c.Request.Context(), email); err == nil {
		// 等时化:账号存在与否都执行一次 bcrypt 比较,避免存在性被时序区分
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(email))
		token, tokenHash := newToken()
		if err := a.store.CreateEmailToken(c.Request.Context(), tenant.ID, tokenHash, "reset", a.cfg.ResetTokenTTL); err == nil {
			_ = a.mailer.SendResetEmail(email, token)
		}
	} else {
		// 等时化:邮箱不存在也执行一次 bcrypt 比较
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(email))
	}
	writeJSON(c, http.StatusAccepted, map[string]string{"status": "accepted"})
}

type resetPasswordReq struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

// handleResetPassword 无效/已使用/过期 token 一律 400。
func (a *API) handleResetPassword(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req resetPasswordReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeErr(c, http.StatusBadRequest, errValidation, "token required")
		return
	}
	if !validPasswordLen(req.NewPassword) {
		writeErr(c, http.StatusBadRequest, errValidation, "新密码至少 8 个字符")
		return
	}
	tenantID, err := a.store.ConsumeEmailToken(c.Request.Context(), hashToken(strings.TrimSpace(req.Token)), "reset")
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "无效、已使用或过期的重置 token")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if err := a.store.SetTenantPassword(c.Request.Context(), tenantID, string(hash)); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 密码重置后吊销该租户全部既有会话,旧会话不得继续访问。
	_ = a.store.DeleteSessionsByTenant(c.Request.Context(), tenantID)
	writeNoContent(c)
}

// ---------- API Bearer(JWT)token 端点 ----------

type tokenReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// tokenResp 契约:签发成功返回 accessToken / tokenType / expiresIn(秒),不写 cookie。
type tokenResp struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int64  `json:"expiresIn"`
}

// handleToken 签发 API Bearer JWT:校验逻辑与 handleLogin 完全一致
// (邮箱小写、pending→401、banned→403、错误凭据→401 等时化 dummy bcrypt),
// 成功后按租户 IsSuperAdmin 计算角色并用 CLOAK_JWT_SECRET/JWTTTL 签发 HS256 JWT。
func (a *API) handleToken(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req tokenReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	tenant, err := a.store.GetTenantByEmail(c.Request.Context(), email)
	if err != nil {
		// 等时化:账号不存在也执行一次 bcrypt 比较
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(req.Password))
		writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
		return
	}
	switch tenant.Status {
	case "pending":
		writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱未验证,请查收验证邮件")
		return
	case "banned":
		writeErr(c, http.StatusForbidden, errForbidden, "账号已被封禁")
		return
	}
	if !tenant.FirstLoginSetup {
		pwHash, err := a.store.TenantPasswordHash(c.Request.Context(), tenant.ID)
		if err != nil || pwHash == nil {
			writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(*pwHash), []byte(req.Password)) != nil {
			writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
			return
		}
	}
	role := "tenant"
	if tenant.IsSuperAdmin {
		role = "superadmin"
	}
	// 复用 server 构建的 jwtMgr(密钥回退逻辑一致:CLOAK_JWT_SECRET 为空时用启动期随机密钥,
	// 与 authenticate 的校验密钥保持一致,避免签发的 token 被 401 拒绝)。
	token, err := a.jwtMgr.Issue(tenant.ID, role)
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
