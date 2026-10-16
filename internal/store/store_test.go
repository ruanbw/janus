package store

// store.go 平台管理/清理任务的语义测试:连真实 Postgres,验证"改不存在的行"必须报错、
// 过期清理必须分批且清干净。

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// TestUpdateTenantAdminMissingTenantReturnsNotFound 固化平台管理端写操作的 RowsAffected 契约。
//
// 契约:对不存在的租户执行状态/等级更新,必须返回 ErrNotFound,而不是静默返回成功。
//
// 为什么必须报错:UPDATE ... WHERE id=? 命中 0 行时 GORM 不报错,调用方看到的是 200
// "封禁成功",而目标租户根本没被改(打错 id、租户已被删都会这样)。更糟的是被复用到
// worker 路径时,静默失败没有任何告警,只表现为"状态莫名其妙没生效"。
func TestUpdateTenantAdminMissingTenantReturnsNotFound(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	status := "banned"
	tierID := int64(1)
	for name, tc := range map[string]struct {
		status *string
		tierID *int64
	}{
		"仅状态": {status: &status},
		"仅等级": {tierID: &tierID},
		"两者":  {status: &status, tierID: &tierID},
	} {
		t.Run(name, func(t *testing.T) {
			// Given 库里不存在该租户
			// When 平台管理端更新它
			err := s.UpdateTenantAdmin(ctx, 9_999_999, tc.status, tc.tierID)

			// Then 必须返回 ErrNotFound
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("改不存在的租户返回 err=%v, want ErrNotFound(静默成功等于谎报成功)", err)
			}
		})
	}
}

// TestUpdateTenantAdminExistingTenantSucceeds 是上面那条契约的正向对照:
// 租户存在时更新必须照常成功,不能被"加检查"误伤成误报 not found。
func TestUpdateTenantAdminExistingTenantSucceeds(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// Given 一个存在的租户
	tenantID := newTenant(t, s, "admin-update@test.io")

	// When 封禁它(封禁必须同时自增 token_version 吊销在手 token)
	status := "banned"
	if err := s.UpdateTenantAdmin(ctx, tenantID, &status, nil); err != nil {
		t.Fatalf("更新存在的租户失败: %v", err)
	}

	// Then 状态已生效且 token_version 被自增
	got, err := s.GetTenantByID(ctx, tenantID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.Status != "banned" {
		t.Fatalf("status = %q, want banned", got.Status)
	}
	if got.TokenVersion != 2 {
		t.Fatalf("token_version = %d, want 2(封禁必须吊销在手 token)", got.TokenVersion)
	}
}

// TestExpiredCleanupBatched 固化过期会话/邮箱 token 清理的分批语义。
//
// 契约:清理必须分批独立提交(返回累计删除行数),且分批不得漏删 ——
// 批次之间要清空全部过期行,同时一行都不能误删未过期的。
//
// 为什么必须分批:一条 `DELETE ... WHERE expires_at <= now()` 会把全部过期行锁到
// 事务提交,高流量租户上是长事务 + WAL 暴涨,还与登录热路径的 INSERT 争行锁。
// 分批后单批的锁持有时间与影响面都有界(与 CleanupVisitsBeforeBatched 同一理由)。
func TestExpiredCleanupBatched(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "cleanup-batch@test.io")

	// Given 5 条已过期 + 2 条未过期的会话,以及 3 条已过期 + 1 条未过期的邮箱 token
	expiredSessions := []string{}
	for i := 0; i < 5; i++ {
		h := fmt.Sprintf("sess-expired-%d", i)
		if _, err := s.CreateSession(ctx, tenantID, h, "csrf", -time.Minute); err != nil {
			t.Fatalf("create expired session: %v", err)
		}
		expiredSessions = append(expiredSessions, h)
	}
	liveSessions := []string{}
	for i := 0; i < 2; i++ {
		h := fmt.Sprintf("sess-live-%d", i)
		if _, err := s.CreateSession(ctx, tenantID, h, "csrf", time.Hour); err != nil {
			t.Fatalf("create live session: %v", err)
		}
		liveSessions = append(liveSessions, h)
	}
	for i := 0; i < 3; i++ {
		if err := s.CreateEmailToken(ctx, tenantID, fmt.Sprintf("tok-expired-%d", i), "verify", -time.Minute); err != nil {
			t.Fatalf("create expired email token: %v", err)
		}
	}
	if err := s.CreateEmailToken(ctx, tenantID, "tok-live", "verify", time.Hour); err != nil {
		t.Fatalf("create live email token: %v", err)
	}

	// When 以极小的批大小(2)分批清理:必须真的走多批
	nSessions, err := s.DeleteExpiredSessionsBatched(ctx, 2)
	if err != nil {
		t.Fatalf("delete expired sessions: %v", err)
	}
	nTokens, err := s.DeleteExpiredEmailTokensBatched(ctx, 2)
	if err != nil {
		t.Fatalf("delete expired email tokens: %v", err)
	}

	// Then 过期行被清空、未过期行保留
	if nSessions != 5 {
		t.Errorf("删除的过期会话数 = %d, want 5(批大小 2 时分批不得漏删)", nSessions)
	}
	if nTokens != 3 {
		t.Errorf("删除的过期邮箱 token 数 = %d, want 3", nTokens)
	}
	var remaining int64
	if err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM sessions WHERE tenant_id = ?`, tenantID).Scan(&remaining).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if remaining != int64(len(liveSessions)) {
		t.Errorf("剩余会话数 = %d, want %d(未过期会话不得被误删)", remaining, len(liveSessions))
	}
	for _, h := range liveSessions {
		var n int64
		if err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM sessions WHERE token_hash = ?`, h).Scan(&n).Error; err != nil {
			t.Fatalf("count session %s: %v", h, err)
		}
		if n != 1 {
			t.Errorf("未过期会话 %s 被误删", h)
		}
	}
	for _, h := range expiredSessions {
		var n int64
		if err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM sessions WHERE token_hash = ?`, h).Scan(&n).Error; err != nil {
			t.Fatalf("count session %s: %v", h, err)
		}
		if n != 0 {
			t.Errorf("过期会话 %s 未被清理", h)
		}
	}
	var liveTokens int64
	if err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM email_tokens WHERE tenant_id = ?`, tenantID).Scan(&liveTokens).Error; err != nil {
		t.Fatalf("count email tokens: %v", err)
	}
	if liveTokens != 1 {
		t.Errorf("剩余邮箱 token 数 = %d, want 1", liveTokens)
	}
}

// TestCreateSessionExpiryComputedByDatabase 固化会话过期时间的计算位置。
//
// 契约:expires_at 必须由数据库在同一语句内用 now() 算出,而不是 Go 侧算好再写 ——
// 校验侧(GetSessionByTokenHash 的 `expires_at > now()`)用的是 PG 时钟,
// 两边用不同时钟时,漂移会让会话提前失效或超期存活。
//
// 怎么在单机测试里把这两者区分开:同一个语句里 created_at 的 DEFAULT now() 与
// expires_at 的 now() 是同一个事务时间戳,因此
// `expires_at - created_at` 必须**精确等于** ttl。若 expires_at 由 Go 在发语句
// 之前算好,差值必然比 ttl 少一个往返延迟(纳秒级随机量),不可能恰好相等。
func TestCreateSessionExpiryComputedByDatabase(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "session-clock@test.io")

	// Given 一个 90 分钟的 TTL
	ttl := 90 * time.Minute

	// When 创建会话
	sess, err := s.CreateSession(ctx, tenantID, "tok-clock", "csrf", ttl)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Then 库里 expires_at - created_at 精确等于 ttl(两者同出一个 DB 时钟)
	var delta float64
	if err := s.db.WithContext(ctx).Raw(
		`SELECT EXTRACT(EPOCH FROM (expires_at - created_at)) FROM sessions WHERE token_hash = ?`,
		"tok-clock").Scan(&delta).Error; err != nil {
		t.Fatalf("measure expiry delta: %v", err)
	}
	if diff := delta - ttl.Seconds(); diff > 1e-6 || diff < -1e-6 {
		t.Fatalf("expires_at - created_at = %fs, want %fs(差值说明 expires_at 不是数据库时钟算的)",
			delta, ttl.Seconds())
	}

	// Then 返回给调用方的值必须等于库里存的值(逐微秒一致)
	var stored time.Time
	if err := s.db.WithContext(ctx).Raw(
		`SELECT expires_at FROM sessions WHERE token_hash = ?`, "tok-clock").Scan(&stored).Error; err != nil {
		t.Fatalf("read stored expiry: %v", err)
	}
	if !stored.Equal(sess.ExpiresAt) {
		t.Fatalf("返回的 expires_at(%v) 与库里的(%v) 不一致", sess.ExpiresAt, stored)
	}

	// Then TTL 语义:正 TTL 的会话有效,负 TTL 的会话立即过期(校验侧用的是 now())
	if _, err := s.GetSessionByTokenHash(ctx, "tok-clock"); err != nil {
		t.Fatalf("刚创建的有效会话查不到: %v", err)
	}
	if _, err := s.CreateSession(ctx, tenantID, "tok-expired", "csrf", -time.Minute); err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	if _, err := s.GetSessionByTokenHash(ctx, "tok-expired"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("已过期会话仍能取到(err=%v),want ErrNotFound", err)
	}
}

// TestListTenantsPaged 固化平台管理租户列表的分页契约。
//
// 契约:按 id 倒序分页返回,同时给出总数;每页仍要带上各自的平台默认域名与
// 超管引导态(域名查询必须收敛到本页的 tenant_id 集合,而不是每次拉全表);
// page/pageSize 越界要有确定行为(页码从 1 起,非法值取默认,超大页仍返回空而非报错)。
func TestListTenantsPaged(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// Given 3 个租户,最早的第一个带平台默认域名
	first := newTenant(t, s, "page-a@test.io")
	second := newTenant(t, s, "page-b@test.io")
	third := newTenant(t, s, "page-c@test.io")
	if _, err := s.CreatePlatformDomain(ctx, first, "paged-a.platform.test"); err != nil {
		t.Fatalf("create platform domain: %v", err)
	}

	// When 取第一页(每页 2 条)
	items, total, err := s.ListTenantsPaged(ctx, 1, 2)
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}

	// Then 总数是 3、本页按 id 倒序是 [third, second],且不越界带出别人的域名
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(items) != 2 {
		t.Fatalf("page 1 size = %d, want 2", len(items))
	}
	if items[0].ID != third || items[1].ID != second {
		t.Errorf("page 1 ids = [%d %d], want [%d %d](按 id 倒序)", items[0].ID, items[1].ID, third, second)
	}
	for _, it := range items {
		if it.DefaultDomain != "" {
			t.Errorf("租户 %d 不应有平台默认域名,实际 %q", it.ID, it.DefaultDomain)
		}
	}

	// When 取第二页
	items, total, err = s.ListTenantsPaged(ctx, 2, 2)
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}

	// Then 只剩 1 条(第一个租户),且它的平台默认域名仍被正确填充
	if total != 3 {
		t.Errorf("page 2 total = %d, want 3", total)
	}
	if len(items) != 1 || items[0].ID != first {
		t.Fatalf("page 2 = %+v, want 单条租户 %d", items, first)
	}
	if items[0].DefaultDomain != "paged-a.platform.test" {
		t.Errorf("第二页没带出平台默认域名: %q(域名查询必须覆盖本页 tenant_id)", items[0].DefaultDomain)
	}

	// 越界页码:空页而不是报错
	items, total, err = s.ListTenantsPaged(ctx, 99, 2)
	if err != nil {
		t.Fatalf("list far page: %v", err)
	}
	if len(items) != 0 || total != 3 {
		t.Errorf("越界页返回 %d 条(total=%d),want 0 条/total=3", len(items), total)
	}

	// 非法分页参数:page < 1 归一到 1,pageSize <= 0 取默认值
	items, _, err = s.ListTenantsPaged(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list with defaults: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("page=0/pageSize=0 返回 %d 条, want 3(归一为第 1 页 + 默认每页条数)", len(items))
	}
}
