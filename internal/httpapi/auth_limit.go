package httpapi

// 账号级失败退避(与 ratelimit.go 的每 IP 限流正交)。
//
// 为什么需要:登录 / token 端点原先只有每 IP 限流。攻击者换 IP(代理池、云主机、
// 移动网络)就能对**同一个账号**持续撞库 —— 每 IP 的桶各自独立,单账号视角下流量
// 完全没有上界。按归一化邮箱累计失败次数后,跨 IP 的持续猜测会被退避拦住。
//
// 与每 IP 限流的分工:IP 限流挡"一个来源打很多账号"(喷洒),账号退避挡"很多来源
// 打一个账号"(撞库)。两者都必须有。

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// loginFailThreshold 连续失败达到该次数后开始退避。
	// 取 10:正常用户打错几次密码不受影响,而撞库的失败密度远高于此。
	loginFailThreshold = 10
	// loginFailBaseDelay 是第 threshold 次失败后的退避时长,之后按 2 的幂递增,
	// 上限 loginFailMaxDelay。指数退避对正常用户几乎无感(连错 10 次本就该停手),
	// 却把自动化猜测的成本抬到不可接受。
	loginFailBaseDelay = time.Second
	loginFailMaxDelay  = 15 * time.Minute
	// loginFailIdleTTL 条目在最后一次失败后的保留时长;超过即清理,防止内存无界。
	loginFailIdleTTL = 6 * time.Hour
	// maxLoginFailKeys 条目上限:超限时先清理过期条目,仍超限则不再新增
	// (退避的意义是"慢下来",退化成不计数也好过被撑爆内存)。
	maxLoginFailKeys = 8192
)

type loginFailEntry struct {
	fails        int
	blockedUntil time.Time
	lastFail     time.Time
}

// loginFailureLimiter 按 key(登录场景是归一化邮箱)累计失败次数并指数退避。
type loginFailureLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginFailEntry
	now     func() time.Time
}

func newLoginFailureLimiter() *loginFailureLimiter {
	return &loginFailureLimiter{entries: make(map[string]*loginFailEntry), now: time.Now}
}

// Allow 判断该 key 是否放行;被退避时返回剩余等待时长。
func (l *loginFailureLimiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return true, 0
	}
	now := l.now()
	if now.Before(e.blockedUntil) {
		return false, e.blockedUntil.Sub(now)
	}
	return true, 0
}

// Fail 记一次失败;累计到阈值后设置指数退避窗口。
func (l *loginFailureLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if _, ok := l.entries[key]; !ok {
		if len(l.entries) >= maxLoginFailKeys {
			l.purgeExpired(now)
		}
		if len(l.entries) >= maxLoginFailKeys {
			return
		}
	}
	e, ok := l.entries[key]
	if !ok {
		e = &loginFailEntry{}
		l.entries[key] = e
	}
	e.fails++
	e.lastFail = now
	if step := e.fails - loginFailThreshold; step >= 0 {
		e.blockedUntil = now.Add(loginFailBackoff(step))
	}
}

// Succeed 认证成功:清零该 key 的失败记录(正常用户不被历史失败拖累)。
func (l *loginFailureLimiter) Succeed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// purgeExpired 删除长期无失败的条目。调用方须持有 mu。
func (l *loginFailureLimiter) purgeExpired(now time.Time) {
	for k, e := range l.entries {
		if now.Sub(e.lastFail) >= loginFailIdleTTL {
			delete(l.entries, k)
		}
	}
}

// loginFailBackoff 返回第 step(从 0 起)次超额失败对应的退避时长:base × 2^step,封顶。
func loginFailBackoff(step int) time.Duration {
	if step >= 30 {
		return loginFailMaxDelay
	}
	d := loginFailBaseDelay << uint(step)
	if d <= 0 || d > loginFailMaxDelay {
		return loginFailMaxDelay
	}
	return d
}

// accountFailureBuckets 保存"每个 API 实例一份"的账号级失败退避器。
//
// 为什么不直接用包级单例:账号退避的状态必须与 API 实例同生命周期。生产进程只有
// 一个实例,两者等价;但测试在同一个进程里先后起多个 httptest 实例,包级单例会把
// 上一个用例的失败计数带进下一个用例(同一个邮箱在后续用例里被前一个用例的退避
// 挡成 429),既有测试会随机失败。
//
// 为什么不把字段加进 API 结构体:API 定义在 server.go,本次修复的文件所有权不含
// 该文件(并发开发下改动它容易与其他人冲突)。用 *API 作键与"每实例一个字段"语义
// 完全等价,且状态集中在本文件内。
var accountFailureBuckets sync.Map // *API → *loginFailureLimiter

// accountFailures 返回该 API 实例的账号级失败退避器(懒创建)。
func (a *API) accountFailures() *loginFailureLimiter {
	if v, ok := accountFailureBuckets.Load(a); ok {
		return v.(*loginFailureLimiter)
	}
	actual, _ := accountFailureBuckets.LoadOrStore(a, newLoginFailureLimiter())
	return actual.(*loginFailureLimiter)
}

// writeAccountThrottled 账号被退避时的统一响应:429 + Retry-After。
func writeAccountThrottled(c *gin.Context, retryAfter time.Duration) {
	secs := int(retryAfter.Seconds())
	if secs < 1 {
		secs = 1
	}
	c.Header("Retry-After", strconv.Itoa(secs))
	writeErr(c, http.StatusTooManyRequests, errRateLimited, "该账号登录失败次数过多,请稍后再试")
}
