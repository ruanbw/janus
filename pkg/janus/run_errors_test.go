package janus_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ruanbw/janus/pkg/janus"
)

// TestRunFailsWhenDatabaseUnreachable 钉住契约:App.Run 不得吞掉启动错误。
// 数据库不可达时,Run 必须返回非 nil error,而不是「静默继续监听 HTTP」。
// 不依赖真实 DB:这里用必然连接失败的 DSN(端口越界会在 pgx 解析阶段即失败,
// 因此既不需要 30s 重试窗口,也不需要取消上下文来制造返回值)。
func TestRunFailsWhenDatabaseUnreachable(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
	}{
		{
			name: "postgres:// 前缀",
			dsn:  "postgres://janus:janus@127.0.0.1:99999/janus?sslmode=disable",
		},
		{
			name: "postgresql:// 前缀",
			dsn:  "postgresql://janus:janus@127.0.0.1:99999/janus?sslmode=disable",
		},
	}

	for _, tc := range cases {
		t.Run("Given "+tc.name+" 的不可达 DatabaseURL When Run Then 返回非 nil 错误而不是继续提供 HTTP 服务", func(t *testing.T) {
			// Given 一个不可达的 DatabaseURL(连接必然失败)
			app, err := janus.New(janus.WithConfig(janus.Config{
				DatabaseURL: tc.dsn,
				Addr:        "127.0.0.1:0",
			}))
			if err != nil {
				t.Fatalf("构造 App 失败: %v", err)
			}

			// When 启动 Run(上下文刻意不取消:若 Run 吞掉错误,它会一直服务 HTTP 而不返回)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			runErrCh := make(chan error, 1)
			go func() { runErrCh <- app.Run(ctx) }()

			// Then Run 必须把数据库启动失败当作返回值,而不是继续服务
			select {
			case runErr := <-runErrCh:
				if runErr == nil {
					t.Fatal("数据库不可达时 Run 必须返回非 nil error,而不是静默继续提供 HTTP 服务")
				}
				if errors.Is(runErr, context.Canceled) {
					t.Fatalf("Run 返回的是上下文取消而不是数据库启动错误: %v", runErr)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("DatabaseURL %q 不可达时 Run 仍在提供 HTTP 服务(未返回错误),启动错误被吞掉", tc.dsn)
			}
		})
	}
}
