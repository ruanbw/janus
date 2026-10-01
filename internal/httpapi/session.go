package httpapi

// 会话与认证:会话 cookie(双提交 CSRF)为浏览器后台的认证方式;
// Bearer JWT 为 API/脚本的认证方式(公开端点 POST /api/auth/token 签发)。
// 二者在 authenticate 中间件中归一为同一组 context 值,授权统一交给
// Casbin RBAC(authorize 中间件)。

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/authz"
	"github.com/gin-gonic/gin"

	"cloak/internal/rbac"
	"cloak/internal/store"
)

const (
	sessionCookieName = "cloak_session"
	csrfCookieName    = "cloak_csrf"
	sessionTokenBytes = 32
)

// context keys:authenticate 中间件写入,requireSession/authorize 等读取。
// ctxTenantKey 为 *store.Tenant(DB 对象);ctxRoleKey 为角色字符串
// ("tenant"/"superadmin",以 DB 的 is_super_admin 为准);
// ctxMethodKey 为认证方式("jwt"/"cookie")。
const (
	ctxTenantKey = "auth.tenant"
	ctxRoleKey   = "auth.role"
	ctxMethodKey = "auth.method"
	// ctxSessionKey 为内部 key:cookie 认证方式下额外保存会话对象,
	// 供 requireSession/requireCSRF 做双提交校验(Bearer 方式无会话,不写入)。
	// 说明:契约只列举了前三个 key,但 cookie 方式必须保留会话对象,
	// 否则 requireCSRF 无法区分 Bearer(免疫 CSRF)与 cookie(需校验)场景。
	ctxSessionKey = "auth.session"
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
	// csrf cookie 刻意**不**设 HttpOnly:前端要读它做双提交校验,
	// 与 setSessionCookies 保持一致。(HttpOnly 不参与删除时的 cookie 匹配,
	// 所以两种写法都能删掉;但这处不一致会让将来"修正"的人以为 csrf 该设 HttpOnly,
	// 从而真的把双提交校验打死。)
	http.SetCookie(c.Writer, &http.Cookie{Name: csrfCookieName, Value: "", Path: "/", SameSite: http.SameSiteLaxMode, Secure: a.cfg.CookieSecure, MaxAge: -1})
}

// authenticate 认证中间件,两种认证方式归一为同一组 context 值:
//
//	① Authorization 头以 "Bearer " 开头 → JWT 解析 → 租户 → 封禁检查 → method="jwt";
//	② 无 Bearer → 现有会话 cookie 流程 → method="cookie";
//	③ 两者皆无(或解析/查找失败)→ 401 并 Abort。
//
// 角色一律以 DB 的 is_super_admin 为准,不信任 token 内 role 声明:
// 防止陈旧声明(降级/提权后旧 token 仍携带旧角色)与伪造提权。
func (a *API) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			claims, err := a.jwtMgr.Parse(strings.TrimPrefix(authHeader, "Bearer "))
			if err != nil {
				writeErr(c, http.StatusUnauthorized, errUnauth, "not authenticated")
				c.Abort()
				return
			}
			tenantID, err := strconv.ParseInt(claims.Subject, 10, 64)
			if err != nil {
				writeErr(c, http.StatusUnauthorized, errUnauth, "not authenticated")
				c.Abort()
				return
			}
			t, err := a.store.GetTenantByID(c.Request.Context(), tenantID)
			// 租户不存在(已删除)或已封禁 → 一律 401
			if err != nil || t.Status == "banned" {
				writeErr(c, http.StatusUnauthorized, errUnauth, "not authenticated")
				c.Abort()
				return
			}
			// token_version 吊销:签发时快照的版本必须仍等于库里的当前值。
			// 改密/重置密码自增该值 → 别人手里的旧 accessToken 立刻失效,不必等 TTL。
			// (没有 tv 声明的历史 token 解析为 0,与库中 ≥1 恒不相等 → 一并作废。)
			cur, err := a.store.TenantTokenVersion(c.Request.Context(), tenantID)
			if err != nil || cur != claims.TokenVersion {
				writeErr(c, http.StatusUnauthorized, errUnauth, "not authenticated")
				c.Abort()
				return
			}
			role := rbac.RoleTenant
			if t.IsSuperAdmin {
				role = rbac.RoleSuperadmin
			}
			c.Set(ctxTenantKey, t)
			c.Set(ctxRoleKey, role)
			c.Set(ctxMethodKey, "jwt")
			c.Next()
			return
		}

		// ② 无 Bearer:现有 cookie 会话流程(会话对象一并存入 context,供 CSRF 校验)
		t, sess, ok := a.cookieTenant(c)
		if !ok {
			writeErr(c, http.StatusUnauthorized, errUnauth, "not authenticated")
			c.Abort()
			return
		}
		role := rbac.RoleTenant
		if t.IsSuperAdmin {
			role = rbac.RoleSuperadmin
		}
		c.Set(ctxTenantKey, t)
		c.Set(ctxRoleKey, role)
		c.Set(ctxMethodKey, "cookie")
		c.Set(ctxSessionKey, sess)
		c.Next()
	}
}

// cookieTenant 按会话 cookie 解析租户与会话(旧 cookie 流程,
// 含封禁检查;封禁租户的既有会话立即失效)。
func (a *API) cookieTenant(c *gin.Context) (*store.Tenant, *store.Session, bool) {
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

// currentTenant 返回当前请求的租户与会话。
// 优先读 context(authenticate 中间件已认证;cookie 方式附带了会话对象,
// Bearer 方式无会话返回 nil);context 缺失时回退旧 cookie 逻辑
// (防御路径:未挂 authenticate 中间件的路由,行为与改造前一致)。
func (a *API) currentTenant(c *gin.Context) (*store.Tenant, *store.Session, bool) {
	if tv, ok := c.Get(ctxTenantKey); ok {
		t := tv.(*store.Tenant)
		if sv, ok := c.Get(ctxSessionKey); ok {
			return t, sv.(*store.Session), true
		}
		return t, nil, true // Bearer 方式:无会话对象
	}
	return a.cookieTenant(c)
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
// sess==nil(即 Bearer/JWT 认证方式)时直接放行——header 认证天然免疫 CSRF
// (跨站请求无法伪造 Authorization 头)。
func (a *API) requireCSRF(c *gin.Context, sess *store.Session) bool {
	if sess == nil {
		return true
	}
	// 恒定时间比较,避免通过响应耗时侧信道逐字节猜测 CSRF token。
	if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-CSRF-Token")), []byte(sess.CSRFToken)) != 1 {
		writeErr(c, http.StatusForbidden, errCSRF, "invalid csrf token")
		return false
	}
	return true
}

// authorize 授权中间件:把 context 中的角色作为 subject 交给 Casbin,
// 按 (role, method, path) 判权;未放行 → 403 统一 JSON 错误体。
func (a *API) authorize() gin.HandlerFunc {
	az := authz.NewAuthorizer(a.rbacEnforcer.Enforcer())
	return func(c *gin.Context) {
		role, ok := c.Get(ctxRoleKey)
		if !ok {
			// 防御:缺少角色(未认证不应到达这里)→ 拒绝
			writeErr(c, http.StatusForbidden, errForbidden, "permission denied")
			c.Abort()
			return
		}
		// gin-contrib/authz(v1.0.7)仅从 Basic Auth 取 subject(用户名),故先注入:
		// subject 即角色字符串,密码位留空。参见该库源码 GetUserName/CheckPermission。
		c.Request.SetBasicAuth(role.(string), "")
		az(c)
		// 该库未放行时 AbortWithStatus(403) 不写 body,这里补写统一错误体
		// (enforcer 内部异常会 panic,由全局 panicRecoverAndLog 兜底为 500)。
		if c.IsAborted() && c.Writer.Status() == http.StatusForbidden {
			writeErr(c, http.StatusForbidden, errForbidden, "permission denied")
		}
	}
}
