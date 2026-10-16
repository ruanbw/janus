package httpapi_test

// 审查项 High-1 黑盒回归:authz 中间件曾有一个 "janus.auth.is_protected_extension"
// 放行分支。全仓没有写入点,所以它永不触发 —— 但它是一个"谁设置谁绕过"的后门开关:
// 任何能往 gin.Context 塞这个 key 的代码(扩展、中间件)都能一步跳过整条授权链却毫无
// 痕迹。已删除。
//
// 本测试钉住"删除后仍成立的前提":扩展路由组(ProtectedRoutes 注入的那组)只挂
// authenticate、不挂 Casbin authorize,因此未认证请求必须被 401 挡在门外,绝不能
// 因为某个旁路被放行到 handler 内部。基座契约:基座只做认证,授权由扩展自行完成。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"janus/internal/geo"
	"janus/internal/httpapi"
	"janus/internal/testutil"
)

func TestProtectedExtensionRoute_NotBypassedByAuthz(t *testing.T) {
	env := testutil.Setup(t)

	hit := false
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Store:     env.Store,
		Cfg:       env.Cfg,
		GeoLookup: geo.Disabled,
		ProtectedRoutes: []func(rg *gin.RouterGroup){
			func(rg *gin.RouterGroup) {
				rg.GET("/ext/probe", func(c *gin.Context) { hit = true; c.Status(http.StatusOK) })
			},
		},
	}))
	t.Cleanup(srv.Close)

	// 扩展路由挂在 /api 组下(protAuth),路径需带上该前缀。
	resp, err := http.Get(srv.URL + "/api/ext/probe")
	if err != nil {
		t.Fatalf("GET /ext/probe: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("未认证访问扩展路由 = %d, want 401(扩展路由组只挂 authenticate)", resp.StatusCode)
	}
	if hit {
		t.Errorf("未认证请求被放行到了扩展 handler,授权链被绕过")
	}
}
