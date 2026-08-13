package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"

	"cloak/internal/store"
)

const (
	sessionCookieName = "cloak_session"
	csrfCookieName    = "cloak_csrf"
	sessionTokenBytes = 32
)

// newToken 生成随机 token 及其 SHA-256 哈希(明文仅出现在 cookie/邮件中,库中存哈希)。
func newToken() (string, string) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:])
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// setSessionCookies 下发会话 cookie(HTTP-only)与 CSRF cookie(双提交,非 HttpOnly)。
func (a *API) setSessionCookies(w http.ResponseWriter, token, csrf string, ttl time.Duration) {
	maxAge := int(ttl.Seconds())
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token,
		Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: a.cfg.CookieSecure, MaxAge: maxAge,
	})
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieName, Value: csrf,
		Path: "/", SameSite: http.SameSiteLaxMode,
		Secure: a.cfg.CookieSecure, MaxAge: maxAge,
	})
}

func (a *API) clearSessionCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.CookieSecure, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: "", Path: "/", SameSite: http.SameSiteLaxMode, Secure: a.cfg.CookieSecure, MaxAge: -1})
}

// currentTenant 从 cookie 解析会话并返回租户。
func (a *API) currentTenant(r *http.Request) (*store.Tenant, *store.Session, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return nil, nil, false
	}
	sess, err := a.store.GetSessionByTokenHash(r.Context(), hashToken(c.Value))
	if err != nil {
		return nil, nil, false
	}
	t, err := a.store.GetTenantByID(r.Context(), sess.TenantID)
	if err != nil {
		return nil, nil, false
	}
	// 封禁租户的既有会话立即失效(封禁后其域名也被授权端点拒绝)
	if t.Status == "banned" {
		return nil, nil, false
	}
	return t, sess, true
}

// requireSession 未登录返回 401。
func (a *API) requireSession(w http.ResponseWriter, r *http.Request) (*store.Tenant, *store.Session, bool) {
	t, sess, ok := a.currentTenant(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, errUnauth, "not authenticated")
		return nil, nil, false
	}
	return t, sess, true
}

// requireCSRF 写操作校验双提交 token:X-CSRF-Token 必须等于会话关联的 csrf_token。
func (a *API) requireCSRF(w http.ResponseWriter, r *http.Request, sess *store.Session) bool {
	if r.Header.Get("X-CSRF-Token") != sess.CSRFToken {
		writeErr(w, http.StatusForbidden, errCSRF, "invalid csrf token")
		return false
	}
	return true
}
