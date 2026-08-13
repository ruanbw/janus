package httpapi

// 02/03 — 认证:注册、邮箱验证、登录/登出、me、改密、忘记/重置密码(会话 cookie + CSRF)。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"cloak/internal/domain"
	"cloak/internal/store"
)

const minPasswordLen = 8

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
func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	slug := strings.TrimSpace(req.Slug)
	if !validEmail(email) {
		writeErr(w, http.StatusBadRequest, errValidation, "邮箱格式非法")
		return
	}
	if !domain.IsValidSlug(slug) {
		writeErrDetails(w, http.StatusBadRequest, errValidation, "slug 非法(小写字母/数字开头结尾,可含连字符,1-63 字符)", map[string]string{"field": "slug"})
		return
	}
	if !validPasswordLen(req.Password) {
		writeErrDetails(w, http.StatusBadRequest, errValidation, "密码至少 8 个字符", map[string]string{"field": "password"})
		return
	}
	ctx := r.Context()

	if _, err := a.store.GetTenantByEmail(ctx, email); err == nil {
		writeErr(w, http.StatusConflict, errConflict, "邮箱已被注册")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	slug = strings.ToLower(slug)
	if _, err := a.store.GetTenantBySlug(ctx, slug); err == nil {
		writeErr(w, http.StatusConflict, errConflict, "slug 已被占用")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// slug 与既有域名 FQDN 冲突(平台默认域名或他人自有域名)
	if exists, _ := a.store.DomainFQDNExists(ctx, slug+"."+a.cfg.PlatformDomain); exists {
		writeErr(w, http.StatusConflict, errConflict, "slug 与既有域名冲突")
		return
	}
	if exists, _ := a.store.DomainFQDNExists(ctx, slug); exists {
		writeErr(w, http.StatusConflict, errConflict, "slug 与既有域名冲突")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	tenant, err := a.store.CreateTenant(ctx, email, string(hash), slug, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if _, err := a.store.CreatePlatformDomain(ctx, tenant.ID, slug+"."+a.cfg.PlatformDomain); err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 重新获取租户(含默认域名)
	if tenant, err = a.store.GetTenantByID(ctx, tenant.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 验证邮件(控制台 mailer)
	token, tokenHash := newToken()
	if err := a.store.CreateEmailToken(ctx, tenant.ID, tokenHash, "verify", a.cfg.VerifyTokenTTL); err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if err := a.mailer.SendVerifyEmail(email, token); err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, tenant)
}

type verifyEmailReq struct {
	Token string `json:"token"`
}

// handleVerifyEmail 邮箱验证:成功后租户转 active,并触发默认域名证书预签发探活。
func (a *API) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeErr(w, http.StatusBadRequest, errValidation, "token required")
		return
	}
	tenantID, err := a.store.ConsumeEmailToken(r.Context(), hashToken(strings.TrimSpace(req.Token)), "verify")
	if err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "无效或已使用的验证 token")
		return
	}
	if err := a.store.VerifyTenant(r.Context(), tenantID); err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 触发平台默认域名证书预签发探活(active 域名由后台任务补探活,这里立即触发)
	go a.probeTenantDefaultDomain(tenantID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	tenant, err := a.store.GetTenantByEmail(r.Context(), email)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
		return
	}
	switch tenant.Status {
	case "pending":
		writeErr(w, http.StatusUnauthorized, errUnauth, "邮箱未验证,请查收验证邮件")
		return
	case "banned":
		writeErr(w, http.StatusForbidden, errForbidden, "账号已被封禁")
		return
	}
	if !tenant.FirstLoginSetup {
		pwHash, err := a.store.TenantPasswordHash(r.Context(), tenant.ID)
		if err != nil || pwHash == nil {
			writeErr(w, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(*pwHash), []byte(req.Password)) != nil {
			writeErr(w, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
			return
		}
	}
	ttl := a.cfg.SessionTTL
	if req.RememberMe != nil && !*req.RememberMe {
		ttl = a.cfg.SessionTTLShort
	}
	token, tokenHash := newToken()
	csrf, _ := newToken()
	sess, err := a.store.CreateSession(r.Context(), tenant.ID, tokenHash, csrf, ttl)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	a.setSessionCookies(w, token, sess.CSRFToken, ttl)
	writeJSON(w, http.StatusOK, tenant)
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	t, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	if !a.requireCSRF(w, r, sess) {
		return
	}
	_ = t
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = a.store.DeleteSession(r.Context(), hashToken(c.Value))
	}
	a.clearSessionCookies(w)
	writeNoContent(w)
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	t, _, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ---------- 修改密码 / 忘记密码 / 重置密码(03) ----------

type changePasswordReq struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// handleChangePassword 修改密码;超管首次登录(无密码)可省略旧密码。
func (a *API) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	t, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	if !a.requireCSRF(w, r, sess) {
		return
	}
	var req changePasswordReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	if !validPasswordLen(req.NewPassword) {
		writeErr(w, http.StatusBadRequest, errValidation, "新密码至少 8 个字符")
		return
	}
	if !t.FirstLoginSetup {
		pwHash, err := a.store.TenantPasswordHash(r.Context(), t.ID)
		if err != nil || pwHash == nil || bcrypt.CompareHashAndPassword([]byte(*pwHash), []byte(req.OldPassword)) != nil {
			writeErr(w, http.StatusBadRequest, errValidation, "旧密码错误")
			return
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if err := a.store.SetTenantPassword(r.Context(), t.ID, string(hash)); err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeNoContent(w)
}

type forgotPasswordReq struct {
	Email string `json:"email"`
}

// handleForgotPassword 始终返回 202,不泄露邮箱存在性。
func (a *API) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if tenant, err := a.store.GetTenantByEmail(r.Context(), email); err == nil {
		token, tokenHash := newToken()
		if err := a.store.CreateEmailToken(r.Context(), tenant.ID, tokenHash, "reset", a.cfg.ResetTokenTTL); err == nil {
			_ = a.mailer.SendResetEmail(email, token)
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

type resetPasswordReq struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

// handleResetPassword 无效/已使用/过期 token 一律 400。
func (a *API) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeErr(w, http.StatusBadRequest, errValidation, "token required")
		return
	}
	if !validPasswordLen(req.NewPassword) {
		writeErr(w, http.StatusBadRequest, errValidation, "新密码至少 8 个字符")
		return
	}
	tenantID, err := a.store.ConsumeEmailToken(r.Context(), hashToken(strings.TrimSpace(req.Token)), "reset")
	if err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "无效、已使用或过期的重置 token")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if err := a.store.SetTenantPassword(r.Context(), tenantID, string(hash)); err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeNoContent(w)
}
