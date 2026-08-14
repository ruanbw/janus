package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

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
func (a *API) setSessionCookies(c *gin.Context, token, csrf string, ttl time.Duration) {
	maxAge := int(ttl.Seconds())
	http.SetCookie(c.Writer, &http.Cookie{
		Name: sessionCookieName, Value: token,
		Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: a.cfg.CookieSecure, MaxAge: maxAge,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name: csrfCookieName, Value: csrf,
		Path: "/", SameSite: http.SameSiteLaxMode,
		Secure: a.cfg.CookieSecure, MaxAge: maxAge,
	})
}

func (a *API) clearSessionCookies(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.CookieSecure, MaxAge: -1})
	http.SetCookie(c.Writer, &http.Cookie{Name: csrfCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.CookieSecure, MaxAge: -1})
}

// currentTenant 从 cookie 解析会话并返回租户。
func (a *API) currentTenant(c *gin.Context) (*store.Tenant, *store.Session, bool) {
	ck, err := c.Request.Cookie(sessionCookieName)
	if err != nil || ck.Value == "" {
		return nil, nil, false
	}
	sess, err := a.store.GetSessionByTokenHash(c.Request.Context(), hashToken(ck.Value))
	if err != nil {
		return nil, nil, false
	}
	t, err := a.store.GetTenantByID(c.Request.Context(), sess.TenantID)
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
func (a *API) requireSession(c *gin.Context) (*store.Tenant, *store.Session, bool) {
	t, sess, ok := a.currentTenant(c)
	if !ok {
		writeErr(c, http.StatusUnauthorized, errUnauth, "not authenticated")
		return nil, nil, false
	}
	return t, sess, true
}

// requireCSRF 写操作校验双提交 token:X-CSRF-Token 必须等于会话关联的 csrf_token。
func (a *API) requireCSRF(c *gin.Context, sess *store.Session) bool {
	if c.GetHeader("X-CSRF-Token") != sess.CSRFToken {
		writeErr(c, http.StatusForbidden, errCSRF, "invalid csrf token")
		return false
	}
	return true
}
