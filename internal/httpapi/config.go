package httpapi

// 前端启动配置:GET /api/config。
// 前端启动时默认请求一次,用于展示 DNS 指向说明(服务器公网 IP)、平台域名与当前租户配额用量;
// 每个租户按自身信息返回不同的 usage(超管也是租户,同样适用)。

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cloak/internal/store"
)

type appConfigResp struct {
	ServerIP       string      `json:"serverIp"`       // 本服务器公网 IP,DNS 校验比对地址(CLOAK_SERVER_PUBLIC_IP)
	PlatformDomain string      `json:"platformDomain"` // 平台域名,如 cloak.test
	Usage          store.Usage `json:"usage"`          // 当前租户配额用量
}

// handleGetConfig 返回当前会话租户的启动配置(未登录 401)。
func (a *API) handleGetConfig(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	usage, err := a.store.Usage(c.Request.Context(), t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, appConfigResp{
		ServerIP:       a.cfg.ServerPublicIP,
		PlatformDomain: a.cfg.PlatformDomain,
		Usage:          *usage,
	})
}
