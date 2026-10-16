package jwt

import (
	"errors"
	"testing"
	"time"
)

// TestIssuePropagatesJTIError 钉住「crypto/rand 失败不 panic,而是向 Issue 传播错误」。
// 通过注入失败的随机源触发错误分支。
func TestIssuePropagatesJTIError(t *testing.T) {
	prev := randRead
	randRead = func(b []byte) (int, error) { return 0, errors.New("entropy exhausted") }
	defer func() { randRead = prev }()

	m := NewManager(testSecret, time.Hour)

	// 不得 panic;必须返回非 nil 错误。
	token, err := m.Issue(1, "tenant", 1)
	if err == nil {
		t.Fatal("crypto/rand 失败时 Issue 应返回错误")
	}
	if token != "" {
		t.Fatalf("失败时不应返回 token,got %q", token)
	}
}
