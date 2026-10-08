package httpapi

// 限流器内部单元测试(无需数据库):容量满时不得拒绝新来源、IPv6 按 /64 归并。

import (
	"fmt"
	"testing"
	"time"
)

// fakeClock 可手动推进的时钟。
type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }

// TestRateLimiterFullDoesNotLockOutNewClients 回归:表满且条目都未过期时,
// 原实现对**任何新 key** 一律返回 false —— 攻击者喷满 8192 个来源即可锁死全部新访客。
// 现在应淘汰最久未活动的条目,新来源照常放行。
func TestRateLimiterFullDoesNotLockOutNewClients(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	l := newRateLimiter(3, time.Minute)
	l.now = clk.now
	l.maxKeys = 64

	// 喷满:每个 key 打一次,时间逐步推进(全部仍在窗口内,purgeExpired 清不掉)
	for i := 0; i < l.maxKeys; i++ {
		clk.t = clk.t.Add(time.Millisecond)
		if !l.Allow(fmt.Sprintf("spray-%d", i)) {
			t.Fatalf("spray key %d should be allowed", i)
		}
	}
	if len(l.buckets) != l.maxKeys {
		t.Fatalf("buckets = %d, want %d", len(l.buckets), l.maxKeys)
	}

	// 合法新客户端:必须放行
	clk.t = clk.t.Add(time.Millisecond)
	if !l.Allow("legit-new-client") {
		t.Fatal("new client must not be locked out when the table is full")
	}
	if len(l.buckets) > l.maxKeys {
		t.Fatalf("buckets = %d, must stay <= %d", len(l.buckets), l.maxKeys)
	}
	// 被淘汰的是最旧的那批
	if _, ok := l.buckets["spray-0"]; ok {
		t.Fatal("oldest entry spray-0 should have been evicted")
	}
	if _, ok := l.buckets[fmt.Sprintf("spray-%d", l.maxKeys-1)]; !ok {
		t.Fatal("most recent spray entry should be kept")
	}
}

// TestRateLimiterFullKeepsActiveBucketCount 表满淘汰不应冲掉正在活跃的 key 的计数:
// 持续打同一 key 的来源 lastSeen 最新,不会落进"最旧一批"。
func TestRateLimiterFullKeepsActiveBucketCount(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	l := newRateLimiter(2, time.Minute)
	l.now = clk.now
	l.maxKeys = 32

	for i := 0; i < l.maxKeys-1; i++ {
		clk.t = clk.t.Add(time.Millisecond)
		l.Allow(fmt.Sprintf("spray-%d", i))
	}
	clk.t = clk.t.Add(time.Millisecond)
	if !l.Allow("hot") || !l.Allow("hot") {
		t.Fatal("first two hits of hot key should pass")
	}
	// 表已满,新 key 触发淘汰
	clk.t = clk.t.Add(time.Millisecond)
	if !l.Allow("newcomer") {
		t.Fatal("newcomer should pass")
	}
	if l.Allow("hot") {
		t.Fatal("hot key must still be limited after eviction (its bucket must survive)")
	}
}

// TestRateLimiterExpiredPurgedFirst 表满时优先清理过期条目。
func TestRateLimiterExpiredPurgedFirst(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	l := newRateLimiter(1, time.Minute)
	l.now = clk.now
	l.maxKeys = 8
	for i := 0; i < l.maxKeys; i++ {
		l.Allow(fmt.Sprintf("old-%d", i))
	}
	clk.t = clk.t.Add(3 * time.Minute)
	if !l.Allow("fresh") {
		t.Fatal("fresh key should pass")
	}
	if len(l.buckets) != 1 {
		t.Fatalf("expired entries should be purged, buckets = %d", len(l.buckets))
	}
}

func TestRateLimitKeyForIP(t *testing.T) {
	cases := []struct{ in, want string }{
		{"203.0.113.7", "203.0.113.7"},
		{"::ffff:203.0.113.7", "203.0.113.7"},
		{"2001:db8:1:2:aaaa:bbbb:cccc:dddd", "2001:db8:1:2::/64"},
		{"2001:db8:1:2::1", "2001:db8:1:2::/64"},
		{"[2001:db8:1:2::1]", "2001:db8:1:2::/64"},
		{"2001:db8:1:3::1", "2001:db8:1:3::/64"},
		{"fe80::1%eth0", "fe80::/64"},
		{"not-an-ip", "not-an-ip"},
	}
	for _, c := range cases {
		if got := rateLimitKeyForIP(c.in); got != c.want {
			t.Errorf("rateLimitKeyForIP(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRateLimiterIPv6SameSlash64SharesBucket 同一 /64 内轮换地址不能换到新桶。
func TestRateLimiterIPv6SameSlash64SharesBucket(t *testing.T) {
	l := newRateLimiter(2, time.Minute)
	if !l.AllowIP("2001:db8:1:2::1") || !l.AllowIP("2001:db8:1:2::2") {
		t.Fatal("first two hits in the /64 should pass")
	}
	if l.AllowIP("2001:db8:1:2:ffff::3") {
		t.Fatal("third hit from the same /64 (different address) must be limited")
	}
	if !l.AllowIP("2001:db8:1:3::1") {
		t.Fatal("a different /64 has its own bucket")
	}
}
