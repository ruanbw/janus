package plugin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"janus/pkg/plugin"
)

// TestPanicStatisticsObservable 钉住 Fail-Open 的可观测性契约:
// 拦截器/动作处理器 panic 被吞(fail-open)时,必须递增对应的计数,便于运维发现
// 外部插件静默失效。
func TestPanicStatisticsObservable(t *testing.T) {
	t.Run("Given 拦截器 panic When CheckInterceptors Then 拦截器计数递增", func(t *testing.T) {
		plugin.ResetInterceptors()
		defer plugin.ResetInterceptors()

		before := plugin.PanicStatistics().Interceptor
		plugin.RegisterInterceptor(&panicInterceptor{name: "obs_panic"})

		req := httptest.NewRequest(http.MethodGet, "https://example.com/x", nil)
		_, _ = plugin.CheckInterceptors(context.Background(), req, 1, &plugin.Fact{})

		after := plugin.PanicStatistics().Interceptor
		if after != before+1 {
			t.Fatalf("Interceptor panic 计数未递增: before=%d after=%d", before, after)
		}
	})

	t.Run("Given 动作处理器 panic When ExecuteActionHandler Then 动作处理器计数递增", func(t *testing.T) {
		plugin.ResetActionHandlers()
		defer plugin.ResetActionHandlers()

		before := plugin.PanicStatistics().ActionHandler
		plugin.RegisterActionHandler("obs_boom", func(c *gin.Context, dctx plugin.DeliveryContext) bool {
			panic("boom")
		})

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		_ = plugin.ExecuteActionHandler(c, "obs_boom", plugin.DeliveryContext{Target: "https://example.com"})

		after := plugin.PanicStatistics().ActionHandler
		if after != before+1 {
			t.Fatalf("ActionHandler panic 计数未递增: before=%d after=%d", before, after)
		}
	})
}
