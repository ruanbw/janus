// Package httpapi 实现全部 HTTP 端点(后台会话 API、公开 Bearer API、
// Caddy 授权内部端点与短链跳转路由)。
package httpapi

import (
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"cloak/internal/config"
	"cloak/internal/domain"
	"cloak/internal/mailer"
	"cloak/internal/store"
)

type Deps struct {
	Store  *store.Store
	Mailer mailer.Mailer
	Cfg    config.Config
}

type API struct {
	store  *store.Store
	mailer mailer.Mailer
	cfg    config.Config
	dns    *domain.DNSChecker
}

func New(d Deps) http.Handler {
	a := &API{
		store:  d.Store,
		mailer: d.Mailer,
		cfg:    d.Cfg,
		dns:    &domain.DNSChecker{ExpectedIP: d.Cfg.ServerPublicIP},
	}
	mux := http.NewServeMux()

	// 健康检查(docker compose healthcheck)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Caddy on-demand TLS 授权端点(仅内网可达)
	mux.HandleFunc("GET /internal/caddy/authorize", a.handleCaddyAuthorize)

	// 认证(会话)——02:注册/邮箱验证/登录/登出/me
	mux.HandleFunc("POST /api/auth/register", a.handleRegister)
	mux.HandleFunc("POST /api/auth/verify-email", a.handleVerifyEmail)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/auth/me", a.handleMe)
	// 03:修改密码 / 忘记密码 / 重置密码
	mux.HandleFunc("POST /api/auth/change-password", a.handleChangePassword)
	mux.HandleFunc("POST /api/auth/forgot-password", a.handleForgotPassword)
	mux.HandleFunc("POST /api/auth/reset-password", a.handleResetPassword)

	// 域名(会话)——04
	mux.HandleFunc("GET /api/domains", a.handleListDomains)
	mux.HandleFunc("POST /api/domains", a.handleCreateDomain)
	mux.HandleFunc("GET /api/domains/{id}", a.handleGetDomain)
	mux.HandleFunc("POST /api/domains/{id}/recheck", a.handleRecheckDomain)
	mux.HandleFunc("PATCH /api/domains/{id}", a.handlePatchDomain)
	mux.HandleFunc("DELETE /api/domains/{id}", a.handleDeleteDomain)

	// 短链(会话)——05/07
	mux.HandleFunc("GET /api/links", a.handleListLinks)
	mux.HandleFunc("POST /api/links", a.handleCreateLink)
	mux.HandleFunc("GET /api/links/{id}", a.handleGetLink)
	mux.HandleFunc("PATCH /api/links/{id}", a.handlePatchLink)
	mux.HandleFunc("DELETE /api/links/{id}", a.handleDeleteLink)
	mux.HandleFunc("POST /api/links/{id}/purge", a.handlePurgeLink)
	mux.HandleFunc("GET /api/links/{id}/visits", a.handleListVisits)
	mux.HandleFunc("GET /api/links/{id}/stats", a.handleLinkStats)

	// 跳转(公开):路径首段为短码,由 Host 决定域名
	mux.HandleFunc("GET /{code}", a.handleRedirect)

	// API Key(会话)——09
	mux.HandleFunc("GET /api/api-keys", a.handleListAPIKeys)
	mux.HandleFunc("POST /api/api-keys", a.handleCreateAPIKey)
	mux.HandleFunc("DELETE /api/api-keys/{id}", a.handleDeleteAPIKey)

	// 租户设置(会话)——06
	mux.HandleFunc("GET /api/me", a.handleGetMe)
	mux.HandleFunc("PATCH /api/me", a.handlePatchMe)

	// 平台管理(超管)——08
	mux.HandleFunc("GET /api/admin/tenants", a.handleAdminListTenants)
	mux.HandleFunc("GET /api/admin/tenants/{id}", a.handleAdminGetTenant)
	mux.HandleFunc("PATCH /api/admin/tenants/{id}", a.handleAdminPatchTenant)
	mux.HandleFunc("DELETE /api/admin/domains/{id}", a.handleAdminDeleteDomain)

	// 公开 API(Bearer)——09
	mux.HandleFunc("GET /api/v1/links", a.handleV1ListLinks)
	mux.HandleFunc("POST /api/v1/links", a.handleV1CreateLink)
	mux.HandleFunc("GET /api/v1/links/{id}", a.handleV1GetLink)
	mux.HandleFunc("DELETE /api/v1/links/{id}", a.handleV1DeleteLink)

	return a.withMiddleware(mux)
}

// withMiddleware 全局中间件:panic 恢复与访问日志。
func (a *API) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v\n%s", rec, debug.Stack())
				writeErr(sw, http.StatusInternalServerError, errInternal, "internal error")
			}
			log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, sw.status, time.Since(start))
		}()
		next.ServeHTTP(sw, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// isPrivateAddr 判定来源地址是否为内网/回环地址(授权端点"仅内网可达")。
func isPrivateAddr(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}
