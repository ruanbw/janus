package domain

// 清理循环的咨询锁行为测试:多副本部署下同一轮只由一个副本执行。
//
// 为什么要打真库:咨询锁是 Postgres 的会话级锁,用假 store 只能验证"调用方调了
// 加锁函数",验证不了"另一个副本持锁时这一轮真的什么都不做"。这里就用真实的
// pg_try_advisory_lock 扮演另一个副本。

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"janus/internal/store"
)

func countVisits(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM visits`).Scan(&n); err != nil {
		t.Fatalf("count visits: %v", err)
	}
	return n
}

// visit-cleanup 必须走咨询锁:另一个副本持锁时,本副本的清理轮次不得执行。
func TestVisitCleanupRunsUnderAdvisoryLock(t *testing.T) {
	st, pool := setupDomainStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tenantID := newActiveTenant(t, st, "cleanup-lock@test.io", "cleanup-lock")

	// Given 一条超过保留期(默认 90 天)的访问记录
	d, err := st.CreateDomain(ctx, tenantID, "cleanup-lock.test", "self", "")
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if err := st.SetDomainActive(ctx, d.ID); err != nil {
		t.Fatalf("activate domain: %v", err)
	}
	link, err := st.CreateLink(ctx, tenantID, "cln001", []string{"https://example.com/"},
		store.RedirectStatus302, store.LinkTypeRedirect, store.LandingSourceURL, "", []int64{d.ID})
	if err != nil {
		t.Fatalf("create link: %v", err)
	}
	if err := st.InsertVisit(ctx, store.VisitRecord{LinkID: link.ID, DomainID: d.ID, Action: "redirect", Outcome: "success"}); err != nil {
		t.Fatalf("insert visit: %v", err)
	}
	execSQL(t, pool, `UPDATE visits SET created_at = now() - interval '400 days'`)
	if n := countVisits(t, pool); n != 1 {
		t.Fatalf("前置条件不成立:visits 应有 1 行,实际 %d 行", n)
	}

	// And 另一个副本正持有 visit-cleanup 的咨询锁
	release, ok, err := st.TryAdvisoryLock(ctx, lockKeyFor("visit-cleanup"))
	if err != nil || !ok {
		t.Fatalf("测试无法扮演另一个副本:ok=%v err=%v", ok, err)
	}

	cfg := testCfg()
	cfg.VisitCleanupEvery = 50 * time.Millisecond
	w := workerWithCfg(st, cfg, dnsErrorChecker())
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		w.Run(ctx)
	}()

	// When 等到清理循环跑过若干轮(50ms 一轮)
	time.Sleep(400 * time.Millisecond)

	// Then 本副本什么都没删:锁在别的副本手里,这一轮不该由它执行
	if n := countVisits(t, pool); n != 1 {
		t.Fatalf("持锁期间本副本仍执行了清理(visits 剩 %d 行):多副本会重复删同一批数据", n)
	}

	// When 另一个副本停机释放锁
	release()

	// Then 下一轮由本副本接手,过期记录被清掉
	deadline := time.Now().Add(3 * time.Second)
	for countVisits(t, pool) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("释放锁后清理未接手:过期访问记录仍在(清理循环可能根本没跑)")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 收尾:worker 必须能随 ctx 取消退出(不留下继续写库的循环)
	cancel()
	select {
	case <-workerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("worker 未在 ctx 取消后退出")
	}
}
