package rbac

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/casbin/casbin/v2"
)

// TestEnforceErrorFailsClosedWithLog 钉住 Enforce 出错时的契约:
// fail-closed(返回 false)+ 打一条限速告警,而不是静默拒绝。
func TestEnforceErrorFailsClosedWithLog(t *testing.T) {
	prevEval := enforceEval
	enforceEval = func(e *casbin.Enforcer, role, path, method string) (bool, error) {
		return false, errors.New("matcher boom")
	}
	defer func() { enforceEval = prevEval }()

	// 重置限速窗口,确保本次必定记录。
	enforceErrLogMu.Lock()
	enforceErrLastLog = time.Time{}
	enforceErrLogMu.Unlock()

	var buf bytes.Buffer
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prevLogger)

	en, err := New()
	if err != nil {
		t.Fatalf("New() 失败: %v", err)
	}

	if en.Enforce(RoleTenant, "GET", "/api/links") {
		t.Fatal("底层出错时必须 fail-closed(返回 false)")
	}
	if !strings.Contains(buf.String(), "fail-closed") {
		t.Fatalf("底层出错时应打限速告警,实际日志:%q", buf.String())
	}
}
