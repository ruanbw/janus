package db

// 测试库互斥:各包的测试都连同一个 janus_test 库、都会 TRUNCATE 业务表,
// 而 go test 是并行跑各个包的,不加互斥两个包会互相把对方正在用的数据清掉
// (表现为一堆莫名其妙的 "not found" / 外键冲突)。
// 用 Postgres 的会话级咨询锁串行化:拿到锁的包独占测试库,直到它的测试全部结束。

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestLockKey 测试库互斥锁键(各包必须用同一个值)。
const TestLockKey int64 = 0x4A414E55 // "JANU"

// TestLockWait 等待其它包让出测试库的最长时间;超时视为环境异常而不是测试失败。
const TestLockWait = 3 * time.Minute

// LockTestDB 独占测试库,返回的 release 需在测试结束时调用(通常挂在 t.Cleanup 上)。
// 拿不到锁(等超时)时返回错误,由调用方决定跳过还是失败。
func LockTestDB(ctx context.Context, pool *pgxpool.Pool) (release func(), err error) {
	// 咨询锁是会话级的:必须独占一条连接直到释放
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire lock connection: %w", err)
	}
	deadline := time.Now().Add(TestLockWait)
	for {
		var got bool
		if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, TestLockKey).Scan(&got); err != nil {
			conn.Release()
			return nil, fmt.Errorf("try advisory lock: %w", err)
		}
		if got {
			break
		}
		if time.Now().After(deadline) {
			conn.Release()
			return nil, fmt.Errorf("等待测试库互斥锁超过 %s(另一个包的测试可能没释放)", TestLockWait)
		}
		time.Sleep(100 * time.Millisecond)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			// 用独立 context 释放:清理阶段的 ctx 可能已取消
			_, _ = conn.Exec(context.WithoutCancel(ctx),
				`SELECT pg_advisory_unlock($1)`, TestLockKey)
			conn.Release()
		})
	}, nil
}
