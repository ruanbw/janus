package main

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestWaitForShutdown 钉住退出路径的选择契约:退出信号与 HTTP 服务异常退出都要
// 汇入同一优雅退出流程,而不是在 goroutine 里 log.Fatalf 绕过清理。
func TestWaitForShutdown(t *testing.T) {
	t.Run("Given 收到退出信号 When waitForShutdown Then 返回 nil", func(t *testing.T) {
		stop := make(chan os.Signal, 1)
		serverErrCh := make(chan error, 1)
		stop <- syscall.SIGTERM

		if err := waitForShutdown(stop, serverErrCh); err != nil {
			t.Fatalf("信号退出应返回 nil,got %v", err)
		}
	})

	t.Run("Given HTTP 服务异常退出 When waitForShutdown Then 返回该错误", func(t *testing.T) {
		stop := make(chan os.Signal, 1)
		serverErrCh := make(chan error, 1)
		want := errors.New("listen tcp: bind: address already in use")
		serverErrCh <- want

		if err := waitForShutdown(stop, serverErrCh); !errors.Is(err, want) {
			t.Fatalf("应返回服务错误,got %v", err)
		}
	})

	t.Run("Given HTTP 服务正常关闭(err=nil) When waitForShutdown Then 返回 nil", func(t *testing.T) {
		stop := make(chan os.Signal, 1)
		serverErrCh := make(chan error, 1)
		serverErrCh <- nil

		if err := waitForShutdown(stop, serverErrCh); err != nil {
			t.Fatalf("正常关闭应返回 nil,got %v", err)
		}
	})

	t.Run("Given 两个通道都无事件 When 命中信号 Then 及时返回", func(t *testing.T) {
		stop := make(chan os.Signal, 1)
		serverErrCh := make(chan error, 1)
		done := make(chan error, 1)
		go func() { done <- waitForShutdown(stop, serverErrCh) }()
		time.Sleep(20 * time.Millisecond)
		stop <- os.Interrupt
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("got %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("waitForShutdown 未及时返回")
		}
	})
}
