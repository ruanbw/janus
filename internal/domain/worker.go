package domain

import (
	"context"
	"log"
	"time"

	"cloak/internal/config"
	"cloak/internal/store"
)

// Worker 运行后台任务:DNS 重试队列、证书预签发探活、访问记录清理。
type Worker struct {
	store *store.Store
	cfg   config.Config
	dns   *DNSChecker
}

func NewWorker(st *store.Store, cfg config.Config) *Worker {
	return &Worker{store: st, cfg: cfg, dns: &DNSChecker{ExpectedIP: cfg.ServerPublicIP}}
}

// Run 启动全部后台循环(阻塞直到 ctx 取消)。
func (w *Worker) Run(ctx context.Context) {
	go w.loop(ctx, w.cfg.DNSRetryInterval, w.dnsRetryPass, "dns-retry")
	go w.loop(ctx, w.cfg.DNSRetryInterval, w.certProbePass, "cert-probe")
	go w.loop(ctx, w.cfg.VisitCleanupEvery, w.visitCleanupPass, "visit-cleanup")
	<-ctx.Done()
}

func (w *Worker) loop(ctx context.Context, interval time.Duration, pass func(context.Context), name string) {
	pass(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pass(ctx)
		}
	}
}

// dnsRetryPass 自有域名 DNS 重试队列:每轮重新校验 pending/failed 且未超时的域名;
// 校验通过置 active;超过最长重试时长(默认 72h)置 failed。
func (w *Worker) dnsRetryPass(ctx context.Context) {
	domains, err := w.store.ListDomainsByStatus(ctx, "pending", "failed")
	if err != nil {
		log.Printf("worker dns-retry: list domains: %v", err)
		return
	}
	for _, d := range domains {
		if d.Origin != "self" {
			continue
		}
		if time.Since(d.CreatedAt) > w.cfg.DNSMaxAge {
			if err := w.store.SetDomainStatus(ctx, d.ID, "failed"); err != nil {
				log.Printf("worker dns-retry: mark failed %s: %v", d.FQDN, err)
			}
			continue
		}
		ok, err := w.dns.Check(ctx, d.FQDN)
		if err != nil {
			log.Printf("worker dns-retry: check %s: %v", d.FQDN, err)
			continue
		}
		if ok {
			if err := w.store.SetDomainActive(ctx, d.ID); err != nil {
				log.Printf("worker dns-retry: activate %s: %v", d.FQDN, err)
			}
		} else if err := w.store.MarkDomainDNSChecked(ctx, d.ID); err != nil {
			log.Printf("worker dns-retry: mark checked %s: %v", d.FQDN, err)
		}
	}
}

// certProbePass 对 active 且 cert_status=pending 的域名发起 HTTPS 探活,触发 Caddy on-demand 签发。
func (w *Worker) certProbePass(ctx context.Context) {
	domains, err := w.store.ListDomainsByStatus(ctx, "active")
	if err != nil {
		log.Printf("worker cert-probe: list domains: %v", err)
		return
	}
	for _, d := range domains {
		if d.CertStatus != "pending" {
			continue
		}
		if ProbeCert(ctx, d.FQDN) {
			if err := w.store.SetDomainCertStatus(ctx, d.ID, "issued"); err != nil {
				log.Printf("worker cert-probe: mark issued %s: %v", d.FQDN, err)
			}
		} else if err := w.store.SetDomainCertStatus(ctx, d.ID, "failed"); err != nil {
			log.Printf("worker cert-probe: mark failed %s: %v", d.FQDN, err)
		}
	}
}

// visitCleanupPass 清理超过保留期(默认 90 天)的访问记录。
func (w *Worker) visitCleanupPass(ctx context.Context) {
	n, err := w.store.CleanupVisitsBefore(ctx, time.Now().Add(-w.cfg.VisitRetention))
	if err != nil {
		log.Printf("worker visit-cleanup: %v", err)
		return
	}
	if n > 0 {
		log.Printf("worker visit-cleanup: removed %d visits", n)
	}
}
