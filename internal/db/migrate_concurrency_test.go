package db

// goose 全局状态的并发安全测试。
//
// goose 把方言/版本表名/BaseFS 放在包级变量里,没有任何并发保护;而进程内可能同时
// 有多个迁移入口在跑(内置初始化 + 外部专有迁移、SDK 多实例共用进程)。
// 这里剥离掉数据库层的咨询锁(那是**跨进程**互斥,与本条无关),直接并发调用
// goose 那一段,在 -race 下暴露全局状态写写竞态。

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
)

// gooseProbeMigration 一份最小可用的 goose 迁移:只建一张探测表。
const gooseProbeMigration = `-- +goose Up
CREATE TABLE IF NOT EXISTS ws_store_goose_probe (id BIGINT PRIMARY KEY);

-- +goose Down
DROP TABLE IF EXISTS ws_store_goose_probe;
`

// TestApplyExtraMigrationsConcurrentSafe 固化 goose 段落的串行化。
//
// 契约:并发调用外部迁移(各自带 BaseFS)不得让 goose 的全局状态互相覆盖 ——
// 必须由包级互斥把整个 Set*/Up 段落串起来。
//
// 断言方式:用 -race 跑(竞态会被 race detector 直接报出来)。迁移本身是幂等的,
// 无锁实现下除了竞态还可能因为 BaseFS 被覆盖而报 "file does not exist"。
func TestApplyExtraMigrationsConcurrentSafe(t *testing.T) {
	ctx := context.Background()

	pool, err := Connect(ctx, testDatabaseURL())
	if err != nil {
		t.Skipf("test database not available (%v)", err)
	}
	t.Cleanup(pool.Close)
	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close() })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0001_probe.sql"), []byte(gooseProbeMigration), 0o600); err != nil {
		t.Fatalf("write temp migration: %v", err)
	}
	fsys := os.DirFS(dir)

	// Given 多个并发调用各自带 BaseFS 的外部迁移入口
	const n = 4
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = applyExtraMigrations(ctx, sqlDB, fsys)
		}(i)
	}
	wg.Wait()

	// Then 全部成功(无竞态、无表名/FS 互覆导致的失败)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("并发外部迁移 #%d 失败: %v", i, err)
		}
	}
}
