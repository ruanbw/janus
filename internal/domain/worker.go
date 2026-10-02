package domain

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"janus/internal/config"
	"janus/internal/store"
)

// 后台循环的固定节奏。
const (
	// OwnershipRecheckInterval active 自有域名的归属复检周期。
	//
	// "激活"在领域定义里是一个持续状态,实现却曾是一次性快照:一旦 active,
	// dnsRetryPass 永不回头,域名过期被他人注册并指向本机后,平台会继续用它发跳转、
	// 并让 Caddy 持续续签证书。24h 足够便宜(每域名一天两次 DNS 查询)又能及时发现
	// 易主 —— 攻击者接管后,证书最长续签窗口是 Let's Encrypt 的签发时长量级,
	// 24h 的复检把"易主到停止服务"的窗口压到可接受范围。
	OwnershipRecheckInterval = 24 * time.Hour
	// certProbeInterval 证书探活节奏(与归属复检一致:一个域名一天至少探一次)。
	certProbeInterval = 5 * time.Minute
)

// workerAdvisoryLockKey 后台任务的会话级咨询锁键。
//
// 多副本部署时每一轮 DNS 校验 / 证书探活都会被执行 N 遍(N 倍出网、N 倍写库),
// 而单副本 compose 下这个问题完全看不见 —— 加一个取不到的副本直接跳过本轮即可。
const workerAdvisoryLockKey int64 = 0x1c0a2

// lockKeyFor 按 pass 名字派生独立的咨询锁键。
//
// 为什么不能三个循环共用一个键:咨询锁要解决的是**跨副本**重复执行,不是让
// 同一进程内的不同后台任务互相排队。共用一个键时,慢的那个(cert-probe 要发
// HTTPS、ownership-recheck 要发 DNS)会把快的那个(dns-retry,50ms 一轮)反复挡掉,
// 日志里表现为"咨询锁被其它副本持有,本轮跳过"—— 而实际上根本没有别的副本。
// 派生方式必须跨进程稳定(同名 → 同键),所以用 FNV-1a 而不是进程内计数器。
func lockKeyFor(name string) int64 {
	var h uint32 = 2166136261
	for i := 0; i < len(name); i++ {
		h ^= uint32(name[i])
		h *= 16777619
	}
	return workerAdvisoryLockKey + int64(h%100000)
}

// Worker 运行后台任务:DNS 重试队列、证书预签发探活、访问/会话清理。
type Worker struct {
	store *store.Store
	cfg   config.Config
	dns   *DNSChecker
}

func NewWorker(st *store.Store, cfg config.Config) *Worker {
	return &Worker{store: st, cfg: cfg, dns: &DNSChecker{ExpectedIP: cfg.ServerPublicIP}}
}

// ValidateIntervals 校验后台循环用到的所有间隔/时长必须为正。
//
// 为什么要有这道:time.NewTicker 对 <=0 的时长直接 panic,而 loop 跑在 goroutine 里 ——
// 未捕获的 panic 会终止整个进程,连带 HTTP 服务一起死,而不是"某个循环停掉"。
// config.Validate() 在 main 里已经覆盖同一批变量,这里是嵌入方(testutil / 其他
// 构造 Worker 的调用方)的兜底:loop 自己也会再挡一次,保证任何构造方式都不会
// 让 ticker panic。
func ValidateIntervals(cfg config.Config) error {
	durations := []struct {
		name string
		v    time.Duration
	}{
		{"JANUS_DNS_RETRY_INTERVAL", cfg.DNSRetryInterval},
		{"JANUS_DNS_MAX_AGE", cfg.DNSMaxAge},
		{"JANUS_VISIT_RETENTION", cfg.VisitRetention},
		{"JANUS_VISIT_CLEANUP_INTERVAL", cfg.VisitCleanupEvery},
		{"证书探活间隔", certProbeInterval},
		{"归属复检间隔", OwnershipRecheckInterval},
	}
	for _, d := range durations {
		if d.v <= 0 {
			return fmt.Errorf("%s 必须为正数,当前为 %v(非正时长会让 time.NewTicker panic 并终止进程)", d.name, d.v)
		}
	}
	return nil
}

// Run 启动全部后台循环(阻塞直到 ctx 取消)。
func (w *Worker) Run(ctx context.Context) {
	// 间隔非法就不启动对应循环,而不是让 time.NewTicker panic 把整个进程带走。
	if err := ValidateIntervals(w.cfg); err != nil {
		log.Printf("worker 配置非法,拒绝启动: %v", err)
		return
	}
	// 这三个循环都会出网(DNS 查询 / HTTPS 探活),加咨询锁做副本间抢占:
	// 多副本时每轮只由一个副本执行,其余跳过。
	//
	// 用 WaitGroup 收口:ctx 取消后 Run 必须等这些循环真正退出才返回。
	// 否则 Run 一返回、调用方以为 worker 已经停掉,而循环还在跑最后一轮 ——
	// 它持有的会话级咨询锁会把下一个(测试里就是下一个用例的)worker 全部挡掉,
	// 表现为"dns-retry: 咨询锁被其它副本持有,本轮跳过"并让断言随机失败。
	var wg sync.WaitGroup
	start := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	start(func() { w.guardedLoop(ctx, w.cfg.DNSRetryInterval, w.dnsRetryPass, "dns-retry") })
	start(func() { w.guardedLoop(ctx, certProbeInterval, w.certProbePass, "cert-probe") })
	start(func() { w.guardedLoop(ctx, OwnershipRecheckInterval, w.ownershipRecheckPass, "ownership-recheck") })
	start(func() { w.loop(ctx, w.cfg.VisitCleanupEvery, w.visitCleanupPass, "visit-cleanup") })
	// 过期会话与邮箱 token 清理与访问清理同一节奏(默认 24h)。
	// email_tokens 仅消费时惰性校验过期,需定期物理清理防表膨胀;spec 未禁止,属合理运维。
	start(func() { w.loop(ctx, w.cfg.VisitCleanupEvery, w.sessionCleanupPass, "session-cleanup") })
	<-ctx.Done()
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context, interval time.Duration, pass func(context.Context), name string) {
	w.tickLoop(ctx, interval, func(c context.Context) { w.safePass(c, pass, name) }, name)
}

// guardedLoop 与 loop 相同,但每一轮先抢会话级咨询锁(多副本只跑一份)。
func (w *Worker) guardedLoop(ctx context.Context, interval time.Duration, pass func(context.Context), name string) {
	w.tickLoop(ctx, interval, func(c context.Context) { w.guarded(c, pass, name) }, name)
}

// tickLoop 立即跑一轮,然后按 interval 定时跑。间隔非法时拒绝启动循环 ——
// 兜底:非正时长会让 time.NewTicker panic,而 panic 发生在 goroutine 里会终止
// 整个进程(连带 HTTP 服务),不是"这个循环停掉"。
func (w *Worker) tickLoop(ctx context.Context, interval time.Duration, run func(context.Context), name string) {
	if interval <= 0 {
		log.Printf("worker %s: 间隔 %v 非法,循环不启动", name, interval)
		return
	}
	run(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run(ctx)
		}
	}
}

// safePass 执行一轮 pass 并吞掉 panic。
//
// goroutine 内未捕获的 panic 会终止整个进程,不是"这个循环停掉" —— 一次空指针
// 就让 HTTP 服务跟着死,而且没有任何告警。HTTP 侧有 panicRecoverAndLog,worker
// 侧缺失属于不对称遗漏,这里补上。
func (w *Worker) safePass(ctx context.Context, pass func(context.Context), name string) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("worker %s panic: %v\n%s", name, rec, debug.Stack())
		}
	}()
	pass(ctx)
}

// guarded 在执行 pass 前抢一次会话级咨询锁:多副本时只有取到锁的那个副本跑本轮。
// 取锁失败不是错误,直接跳过(记一行日志便于排障)。
func (w *Worker) guarded(ctx context.Context, pass func(context.Context), name string) {
	release, ok, err := w.store.TryAdvisoryLock(ctx, lockKeyFor(name))
	if err != nil {
		log.Printf("worker %s: 取咨询锁失败,本轮跳过: %v", name, err)
		return
	}
	if !ok {
		log.Printf("worker %s: 咨询锁被其它副本持有,本轮跳过", name)
		return
	}
	defer release()
	w.safePass(ctx, pass, name)
}

// dnsRetryPass 自有域名重试队列:每轮取到期的 pending/failed 域名重新做归属校验。
// 通过则激活并探活证书;超过最长等待(默认 72h)转入终态 expired 退出队列。
//
// 每条记录的错误都被吞掉并继续下一条:一条坏数据不该让整轮停摆(更不该让
// 后面的域名永远排不上队)。
func (w *Worker) dnsRetryPass(ctx context.Context) {
	rows, err := w.store.ListDomainsForDNSCheck(ctx, w.cfg.DNSMaxAge, w.cfg.DNSRetryInterval, store.DefaultDomainScanLimit)
	if err != nil {
		log.Printf("worker dns-retry: list domains: %v", err)
		return
	}
	for _, d := range rows {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("worker dns-retry: domain %s panic: %v\n%s", d.FQDN, rec, debug.Stack())
				}
			}()
			w.verifyAndActivate(ctx, d)
		}()
	}
}

// verifyAndActivate 对单个重试队列条目做归属校验并落地状态。
func (w *Worker) verifyAndActivate(ctx context.Context, d store.DomainScanRow) {
	if d.Overdue {
		if err := w.store.MarkDomainExpired(ctx, d.ID); err != nil {
			log.Printf("worker dns-retry: mark expired %s: %v", d.FQDN, err)
		} else {
			log.Printf("worker dns-retry: %s 超过最长等待 %v 未完成归属证明,置为 expired(终态;租户可手动重新校验复活)", d.FQDN, w.cfg.DNSMaxAge)
		}
		return
	}
	// 挑战缺失或已过期 → 重新签发一枚,让租户手上永远有一份可用的 TXT。
	// 签发后本轮不再校验(新的 TXT 还没机会生效),等下一轮。
	if d.VerifyToken == "" || challengeExpired(d, w.cfg.DNSMaxAge) {
		token, err := MintVerifyToken()
		if err != nil {
			log.Printf("worker dns-retry: mint token %s: %v", d.FQDN, err)
			return
		}
		if err := w.store.MintVerifyToken(ctx, d.ID, token); err != nil {
			log.Printf("worker dns-retry: mint token %s: %v", d.FQDN, err)
			return
		}
		log.Printf("worker dns-retry: %s 已重新签发 TXT 挑战(_janus-verify.%s)", d.FQDN, d.FQDN)
		return
	}
	res, err := w.dns.Verify(ctx, d.FQDN, d.VerifyToken)
	if err != nil {
		log.Printf("worker dns-retry: verify %s: %v", d.FQDN, err)
		return
	}
	if !res.Verified() {
		if err := w.store.MarkDomainDNSChecked(ctx, d.ID); err != nil {
			log.Printf("worker dns-retry: mark checked %s: %v", d.FQDN, err)
		}
		return
	}
	if err := w.store.ActivateDomainVerified(ctx, d.ID); err != nil {
		log.Printf("worker dns-retry: activate %s: %v", d.FQDN, err)
		return
	}
	w.probeCert(ctx, d.ID, d.FQDN)
}

// challengeExpired 挑战是否已超过最长等待(与 DNSMaxAge 同一条时钟,配置上就是
// "租户最多等多久")。
func challengeExpired(d store.DomainScanRow, maxAge time.Duration) bool {
	// 挑战签发时间才是它是否过期的判据:LastAt 每轮复检都会被刷新,
	// 用它判挑战年龄永远判不过期(于是永不重新签发、也与 overdue 终态
	// 判定脱节)。没有记录时退回 LastAt 兜底。
	if d.VerifyTokenCreatedAt != nil {
		return time.Since(*d.VerifyTokenCreatedAt) > maxAge
	}
	if maxAge <= 0 {
		return false
	}
	return time.Since(d.LastAt) > maxAge
}

// ownershipRecheckPass 低频复检 active 自有域名的归属。
//
// 通过:刷新 ownership_verified_at(顺带记录本次校验时间)。
// 不通过:降级为 failed(仍在重试队列里,租户修好 DNS 后自动恢复)。
//
// 为什么是 failed 而不是 stopped:stopped 的语义是"租户主动暂停",系统不该替租户
// 做这个决定;failed 表示"系统复检发现它不成立",并且 DNS 一修好就能自动回来。
// 为什么不主动吊销证书:证书在 Caddy 手里(ADR-0002 明确"应用不直接操作 Caddy
// 配置"),平台唯一的控制点是授权端点 —— 状态一旦不是 active,授权端点就拒绝,
// Caddy 便不再续签。已签发的那张证书会按其自身有效期自然过期。
func (w *Worker) ownershipRecheckPass(ctx context.Context) {
	rows, err := w.store.ListDomainsForOwnershipRecheck(ctx, OwnershipRecheckInterval, store.DefaultDomainScanLimit)
	if err != nil {
		log.Printf("worker ownership-recheck: list domains: %v", err)
		return
	}
	for _, d := range rows {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("worker ownership-recheck: domain %s panic: %v\n%s", d.FQDN, rec, debug.Stack())
				}
			}()
			res, err := w.dns.Verify(ctx, d.FQDN, d.VerifyToken)
			if err != nil {
				log.Printf("worker ownership-recheck: verify %s: %v", d.FQDN, err)
				return
			}
			if res.Verified() {
				if err := w.store.ActivateDomainVerified(ctx, d.ID); err != nil {
					log.Printf("worker ownership-recheck: refresh %s: %v", d.FQDN, err)
				}
				return
			}
			log.Printf("worker ownership-recheck: %s 归属复检未通过(%s,TXT 未匹配),降级为 failed 并停止授权端点放行", d.FQDN, res.Status)
			if err := w.store.DegradeDomain(ctx, d.ID); err != nil {
				log.Printf("worker ownership-recheck: degrade %s: %v", d.FQDN, err)
			}
		}()
	}
}

// certProbePass 对 active 且证书尚未签发(pending/failed)的域名发起 HTTPS 探活,
// 触发 Caddy on-demand 签发(ADR-0002/0004)。
// 探活失败置 failed(与 httpapi.probeDomainAsync 语义一致),但 failed 仍会被后续轮次重试:
// 域名激活后应持续尝试直到签发成功(issued),一次瞬时失败(如 Caddy 重启、ACME 抖动)
// 不应永久放弃。
//
// 扫描条件已经把"租户非 active"和"上次探活不到 interval"过滤在 SQL 里:
// 未验证租户的平台默认域名在注册时就是 active,若不加租户状态这一条,注册 spam
// 会线性放大成每 5 分钟一批的出网 HTTPS 请求。
func (w *Worker) certProbePass(ctx context.Context) {
	rows, err := w.store.ListDomainsForCertProbe(ctx, certProbeInterval, store.DefaultDomainScanLimit)
	if err != nil {
		log.Printf("worker cert-probe: list domains: %v", err)
		return
	}
	for _, d := range rows {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("worker cert-probe: domain %s panic: %v\n%s", d.FQDN, rec, debug.Stack())
				}
			}()
			w.probeCert(ctx, d.ID, d.FQDN)
		}()
	}
}

// probeCert 对单个域名发起证书探活并落库(每条记录的错误被调用方吞掉)。
func (w *Worker) probeCert(ctx context.Context, id int64, fqdn string) {
	ok := ProbeCert(ctx, fqdn)
	status := "failed"
	if ok {
		status = "issued"
	}
	if err := w.store.SetDomainCertStatus(ctx, id, status); err != nil {
		log.Printf("worker cert-probe: mark %s %s: %v", status, fqdn, err)
	}
}

// visitCleanupPass 清理超过保留期(默认 90 天)的访问记录。
func (w *Worker) visitCleanupPass(ctx context.Context) {
	// 分批删除:一条 DELETE 会把全部过期行的锁持有到提交,高流量租户上是长事务,
	// 还与热路径的 INSERT 争同一批行锁。
	n, err := w.store.CleanupVisitsBeforeBatched(ctx, time.Now().Add(-w.cfg.VisitRetention), w.cfg.VisitCleanupBatch)
	if err != nil {
		log.Printf("worker visit-cleanup: %v", err)
		return
	}
	if n > 0 {
		log.Printf("worker visit-cleanup: removed %d visits", n)
	}
}

// sessionCleanupPass 清理已过期会话与邮箱 token。
func (w *Worker) sessionCleanupPass(ctx context.Context) {
	if err := w.store.DeleteExpiredSessions(ctx); err != nil {
		log.Printf("worker session-cleanup: %v", err)
	}
	if err := w.store.DeleteExpiredEmailTokens(ctx); err != nil {
		log.Printf("worker session-cleanup: %v", err)
	}
}
