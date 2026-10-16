package janus

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"janus/internal/config"
	"janus/internal/db"
)

// TestConsoleMailerDiscardWarns 钉住「ConsoleMailer 丢弃输出必须告警」契约:
// 未配置 SMTP 时 writer 为 io.Discard,验证/重置链接会静默丢失,至少要有一条告警。
func TestConsoleMailerDiscardWarns(t *testing.T) {
	dsn := os.Getenv("JANUS_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://janus:janus@localhost:5432/janus_test_pkg?sslmode=disable"
	}
	ctx := context.Background()
	probe, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	if err := db.Migrate(ctx, probe, filepath.Join("..", "..", "migrations")); err != nil {
		probe.Close()
		t.Fatalf("migrate: %v", err)
	}
	probe.Close()

	gdb, err := db.OpenGORM(dsn)
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	// SMTPHost 留空 → ConsoleMailer + io.Discard。
	_, err = New(WithDB(gdb), WithConfig(config.Config{Addr: "127.0.0.1:0"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if !strings.Contains(buf.String(), "丢弃") {
		t.Fatalf("未配置 SMTP 时应告警邮件输出被丢弃,实际日志:%q", buf.String())
	}
}
