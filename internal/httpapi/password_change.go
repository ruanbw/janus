package httpapi

// 强制改密闸门:must_change_password=true 的租户(超管种子账号)在改密之前,
// 除白名单端点外一律 403(E_PASSWORD_CHANGE_REQUIRED)。
//
// 为什么放在 authenticate 之后、authorize 之前:它需要已解析出的租户(标志位来自 DB,
// 不信任 token 声明),而在授权之前拦截可以避免把"未改密的租户"放进任何业务 handler。
// 挂载点见 server.go 的 protAuth 组 —— 包括扩展注入的受保护路由。

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"janus/internal/store"
)

// passwordChangeExempt 是强制改密期间仍必须放行的端点白名单(冻结契约):
// GET /api/auth/me、POST /api/auth/change-password、POST /api/auth/logout。
//
// 必须精确匹配 (method, path):/api/me、/api/config 这类同前缀端点不在白名单里 ——
// 「未改密的租户只能自救,不能读业务数据」正是这道闸门的全部意义。
func passwordChangeExempt(method, path string) bool {
	switch {
	case method == http.MethodGet && path == "/api/auth/me":
		return true
	case method == http.MethodPost && path == "/api/auth/change-password":
		return true
	case method == http.MethodPost && path == "/api/auth/logout":
		return true
	default:
		return false
	}
}

// requirePasswordChange 拦截被强制改密的租户,白名单端点与标志位为 false 的请求直接放行。
func (a *API) requirePasswordChange() gin.HandlerFunc {
	return func(c *gin.Context) {
		tv, ok := c.Get(ctxTenantKey)
		if !ok {
			// 未认证(理论上不可达:本闸门挂在 authenticate 之后)→ 交给后续中间件
			c.Next()
			return
		}
		t, ok := tv.(*store.Tenant)
		if !ok || !t.MustChangePassword || passwordChangeExempt(c.Request.Method, c.FullPath()) {
			c.Next()
			return
		}
		writeErr(c, http.StatusForbidden, errPasswordChangeRequired, "首次登录必须修改密码")
		c.Abort()
	}
}
