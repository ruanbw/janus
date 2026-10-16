package httpapi_test

// 注册的 TOCTOU 回归:slug 与默认域名的预检通过之后、插入之前,若另一个注册
// 抢先落库了同一个默认域名 FQDN,唯一冲突必须以 409 暴露,不能漏成 500。
//
// 竞态窗口无法靠"多起几个 goroutine"稳定复现(请求会被连接池/调度排队成准串行),
// 这里用一条**未提交**的独立事务精确制造窗口:未提交行对预检不可见(READ COMMITTED),
// 但注册的 INSERT 会撞上 UNIQUE 索引并阻塞,直到持有事务提交。

import (
	"context"
	"net/http"
	"testing"
	"time"

	"janus/internal/testutil"
)

// holdPlatformDomainTx 在独立事务里插入一个 pending 租户 + 未提交的默认域名行,
// 返回持有事务。返回的 commit 用来放行(制造"提交后才撞车"的时序)。
func holdPlatformDomainTx(t *testing.T, env *testutil.Env, fqdn string) func() {
	t.Helper()
	ctx := context.Background()
	tx, err := env.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin hold tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	var holderID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO tenants (email, slug, status, token_version, tier_id)
		VALUES ('holder@example.com', 'holder', 'pending', 1,
		        (SELECT id FROM tiers WHERE name = 'free'))
		RETURNING id`).Scan(&holderID); err != nil {
		t.Fatalf("insert holder tenant: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO domains (tenant_id, fqdn, origin, status, cert_status)
		VALUES ($1, $2, 'platform', 'active', 'pending')`, holderID, fqdn); err != nil {
		t.Fatalf("insert holder domain: %v", err)
	}
	return func() {
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit hold tx: %v", err)
		}
	}
}

// waitForBlockedDomainsInsert 轮询到注册请求真正卡在 domains 的 INSERT 上。
func waitForBlockedDomainsInsert(t *testing.T, env *testutil.Env, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var n int
		err := env.Pool.QueryRow(context.Background(), `
			SELECT count(*) FROM pg_stat_activity
			 WHERE wait_event_type = 'Lock' AND query LIKE '%INSERT INTO "domains"%'`).Scan(&n)
		if err == nil && n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("注册请求未阻塞在 domains INSERT 上,TOCTOU 时序未复现")
}

// TestRegisterDomainConflictRaceReturns409 并发撞车必须 409(而不是 500)。
func TestRegisterDomainConflictRaceReturns409(t *testing.T) {
	env := testutil.Setup(t)
	commitHolder := holdPlatformDomainTx(t, env, "race.janus.test")

	done := make(chan int, 1)
	go func() {
		resp := newClient(env).post("/api/auth/register", map[string]string{
			"email": "racer@example.com", "password": "password123", "slug": "race",
		})
		done <- resp.StatusCode
		_ = resp.Body.Close()
	}()

	waitForBlockedDomainsInsert(t, env, 10*time.Second)
	commitHolder()

	select {
	case code := <-done:
		if code != http.StatusConflict {
			t.Fatalf("并发撞车状态码 = %d, want %d", code, http.StatusConflict)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("注册请求未返回")
	}
}
