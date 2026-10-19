package janus_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ruanbw/janus/pkg/janus"
)

// TestRunRetryAfterStartupFailure 钉住失败路径的 running 复位契约:
// Run 因启动失败(此处为数据库不可达)返回后,App 不能永久卡在 running=true,
// 否则第二次 Run 只会回 "already running",调用方只能重建实例。
func TestRunRetryAfterStartupFailure(t *testing.T) {
	t.Run("Given 首次 Run 因数据库不可达失败 When 再次 Run Then 不再返回 already running(状态已复位)", func(t *testing.T) {
		app, err := janus.New(janus.WithConfig(janus.Config{
			DatabaseURL: "postgres://janus:janus@127.0.0.1:99999/janus?sslmode=disable",
			Addr:        "127.0.0.1:0",
		}))
		if err != nil {
			t.Fatalf("构造 App 失败: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		firstErr := app.Run(ctx)
		if firstErr == nil {
			t.Fatal("首次 Run 在数据库不可达时应返回错误")
		}

		// 二次 Run 必须重新尝试(并再次因同一原因失败),而不是被 running 状态挡住。
		retryErr := app.Run(ctx)
		if retryErr == nil {
			t.Fatal("二次 Run 应重新尝试并因数据库不可达失败")
		}
		if strings.Contains(retryErr.Error(), "already running") {
			t.Fatalf("启动失败后 App 仍卡在 running 状态、无法重试: %v", retryErr)
		}
	})
}
