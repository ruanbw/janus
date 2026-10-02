package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("JANUS_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://janus:janus@localhost:5432/janus_test?sslmode=disable"
	}
	pool, err := Connect(ctx, dsn)
	if err != nil {
		if os.Getenv("JANUS_TEST_DB_REQUIRED") == "1" {
			t.Fatalf("test database not available: %v", err)
		}
		t.Skipf("test database not available: %v", err)
	}
	defer pool.Close()

	release, err := LockTestDB(ctx, pool)
	if err != nil {
		t.Fatalf("lock test database: %v", err)
	}
	defer release()

	migrationsDir := filepath.Join("..", "..", "migrations")

	// 第一次执行迁移
	if err := Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("first migrate failed: %v", err)
	}

	// 第二次执行迁移，应幂等成功，不报任何错误
	if err := Migrate(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("second migrate (idempotent) failed: %v", err)
	}

	// 不只是「没报错」:版本表里应当确实记录了 0001 与 0002。
	// 只断言这两版存在,不锁死整表内容 —— 历史库可能残留更早的版本行(0 号版本等),
	// 那是历史事实,不是本次迁移的失败。
	rows, err := pool.Query(ctx, `
		SELECT EXISTS(SELECT 1 FROM goose_db_version WHERE version_id = 1 AND is_applied),
		       EXISTS(SELECT 1 FROM goose_db_version WHERE version_id = 2 AND is_applied)`)
	if err != nil {
		t.Fatalf("query goose_db_version: %v", err)
	}
	defer rows.Close()
	var v1, v2 bool
	if !rows.Next() {
		t.Fatal("goose_db_version 没有行")
	}
	if err := rows.Scan(&v1, &v2); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !v1 || !v2 {
		t.Fatalf("期望 goose_db_version 已记录 0001 与 0002,实际 v1=%v v2=%v", v1, v2)
	}

	// 迁移的真正效果:visits(domain_id) 索引必须存在(detachInTx 按 domain_id 清 visits)。
	var idx bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.idx_visits_domain') IS NOT NULL`).Scan(&idx); err != nil {
		t.Fatalf("query idx_visits_domain: %v", err)
	}
	if !idx {
		t.Fatal("期望 idx_visits_domain 索引存在(0002 迁移未生效)")
	}
}
