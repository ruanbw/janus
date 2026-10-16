package httpapi

// 发信异步派发器的包内单测:黑盒断言"响应不等待 SMTP"依赖真实时钟,脆弱;
// 这里直接断言派发器的可观测行为 —— 调用方不被阻塞、后台确实执行了发送、
// 失败有日志。

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// asyncLogBuf 并发安全的日志缓冲(后台 goroutine 会写)。
type asyncLogBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *asyncLogBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *asyncLogBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureAsyncLogs(t *testing.T) func() string {
	t.Helper()
	buf := &asyncLogBuf{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf.String
}

// waitFor 轮询等待条件成立(避免测试包引入 testutil 造成 import cycle)。
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// TestDispatchMailDoesNotBlockCaller 即使 mailer 永不返回,dispatchMail 也必须立即
// 返回 —— 调用方是 HTTP handler,同步等 SMTP 正是被修的等时化漏洞。
func TestDispatchMailDoesNotBlockCaller(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{})

	start := time.Now()
	dispatchMail("reset: 发送重置邮件失败", 1, func() error {
		close(started)
		// 上界 3s:同步实现会在这里耗尽预算 → 判定失败,而不是把测试挂死。
		select {
		case <-release:
		case <-time.After(3 * time.Second):
		}
		return nil
	})
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("dispatchMail 阻塞了调用方 %v", elapsed)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("发送没有被后台执行")
	}
}

// TestDispatchMailLogsFailure 后台发送失败必须留痕,不能静默吞掉。
func TestDispatchMailLogsFailure(t *testing.T) {
	logs := captureAsyncLogs(t)
	dispatchMail("reset: 发送重置邮件失败", 1, func() error { return errors.New("smtp boom") })
	waitFor(t, 2*time.Second, "发信失败日志", func() bool {
		return strings.Contains(logs(), "smtp boom")
	})
}
