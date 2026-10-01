package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	// 使用 testutil 默认测试库
	pool, err := Connect(ctx, "postgres://janus:janus@localhost:5432/janus_test?sslmode=disable")
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	defer pool.Close()

	migrationsDir := filepath.Join("..", "..", "migrations")

	// 第一次执行迁移
	if err := Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("first migrate failed: %v", err)
	}

	// 第二次执行迁移，应幂等成功，不报任何错误
	if err := Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("second migrate (idempotent) failed: %v", err)
	}
}
