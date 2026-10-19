package janus_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ruanbw/janus/pkg/janus"
)

// TestProtectedRoutesRequireDatabase 钉住「无 DB 模式下受保护路由必须 fail fast」:
// 没有数据库支撑认证/鉴权时,WithProtectedRoutes 注入的 handler 只会挂在恒定 401
// 的死代码后、永远不可达,必须在 New 阶段显式报错。
func TestProtectedRoutesRequireDatabase(t *testing.T) {
	inject := func(rg *gin.RouterGroup) {
		rg.GET("/custom/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	}

	t.Run("Given 无 WithDB 也未配置 postgres 地址、却注册受保护路由 When New Then 返回错误", func(t *testing.T) {
		app, err := janus.New(
			janus.WithConfig(janus.Config{Addr: "127.0.0.1:0"}),
			janus.WithProtectedRoutes(inject),
		)
		if err == nil {
			t.Fatal("无 DB 模式下注册受保护路由应 fail fast,got nil error")
		}
		if app != nil {
			t.Fatal("失败时应返回 nil App")
		}
	})

	t.Run("Given 配置了 postgres DatabaseURL(尚未 Run)且注册受保护路由 When New Then 不报错", func(t *testing.T) {
		_, err := janus.New(
			janus.WithConfig(janus.Config{
				Addr:        "127.0.0.1:0",
				DatabaseURL: "postgres://janus:janus@localhost:5432/janus?sslmode=disable",
			}),
			janus.WithProtectedRoutes(inject),
		)
		if err != nil {
			t.Fatalf("配置了 postgres 地址时 New 不应因受保护路由报错,got %v", err)
		}
	})

	t.Run("Given 无 DB 但只用 WithRoutes(非受保护) When New Then 成功", func(t *testing.T) {
		app, err := janus.New(
			janus.WithConfig(janus.Config{Addr: "127.0.0.1:0"}),
			janus.WithRoutes(func(engine *gin.Engine) {
				engine.GET("/custom/plain", func(c *gin.Context) { c.Status(http.StatusOK) })
			}),
		)
		if err != nil {
			t.Fatalf("普通路由注入不应受影响,got %v", err)
		}
		if app == nil {
			t.Fatal("expected non-nil App")
		}
	})
}
