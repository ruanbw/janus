package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// QuotaKind 配额种类(对应 Usage 里的两个计数)。
type QuotaKind string

const (
	QuotaLinks   QuotaKind = "links"
	QuotaDomains QuotaKind = "domains"
)

// QuotaError 表示配额已用尽。Kind 决定对外错误码
// (E_LINK_LIMIT / E_DOMAIN_LIMIT),Usage 原样回给前端展示当前用量与上限。
//
// 语义要点:它由 WithQuotaInTx 在**持有租户行锁**的事务内返回,
// 因此"已达上限"这个判断与随后的插入是原子的,不存在并发绕过。
type QuotaError struct {
	Kind  QuotaKind
	Usage Usage
}

func (e *QuotaError) Error() string {
	switch e.Kind {
	case QuotaLinks:
		return fmt.Sprintf("link quota exceeded: %d/%d", e.Usage.Links, e.Usage.MaxLinks)
	case QuotaDomains:
		return fmt.Sprintf("domain quota exceeded: %d/%d", e.Usage.Domains, e.Usage.MaxDomains)
	}
	return "quota exceeded"
}

// WithQuotaInTx 在一个事务里完成「锁租户行 → 读配额 → 判上限 → 执行写入」。
//
// 为什么需要它:原先各 handler 的写法是 Usage() 读一次计数、再 Create* 插一次,
// 两者之间没有事务也没有锁,属于典型 TOCTOU。free 档 100 条时并发 20 个
// POST /api/links,可以在计数停在 99 时全部通过检查,写出 119 条。
//
// 现在 SELECT ... FOR UPDATE 锁住 tenants 行,把同一租户的所有配额消费操作串行化:
// 后到的请求必须等前一个事务提交,然后在锁内重新读到已含前一笔的结果。
// 不同租户之间互不阻塞。
//
// fn 必须使用传入的 tx 做写入,不要用 s.db —— 用 s.db 会跑在事务外,锁就白加了。
// fn 返回错误(业务错误或唯一约束冲突)都会回滚,由调用方决定如何映射成 HTTP 响应;
// 因此调用方若要"重试"(如自动生成短码撞码),重试必须包在 WithQuotaInTx 外面。
func (s *Store) WithQuotaInTx(ctx context.Context, tenantID int64, kind QuotaKind, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked Tenant
		// FOR UPDATE:同一租户串行,跨租户并行。
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Tier").
			Where("id = ?", tenantID).
			First(&locked).Error; err != nil {
			return err
		}

		u, err := usageWithDB(ctx, tx, &locked)
		if err != nil {
			return err
		}
		switch kind {
		case QuotaLinks:
			if u.Links >= u.MaxLinks {
				return &QuotaError{Kind: QuotaLinks, Usage: *u}
			}
		case QuotaDomains:
			if u.Domains >= u.MaxDomains {
				return &QuotaError{Kind: QuotaDomains, Usage: *u}
			}
		default:
			return fmt.Errorf("unknown quota kind %q", kind)
		}
		return fn(tx)
	})
}

// usageWithDB 在给定 db(pgx/gorm 皆可,gorm)上统计用量并带上等级上限。
// 抽出来是为了让「事务内的配额判断」与「只读的 Usage 接口」共用同一份计数口径,
// 避免同一条规则出现两份实现后各自漂移(历史上 CountActiveLinks 就是这样变成死代码的)。
func usageWithDB(ctx context.Context, db *gorm.DB, t *Tenant) (*Usage, error) {
	u := &Usage{}
	var links, domains int64
	if err := db.WithContext(ctx).Model(&Link{}).Where("tenant_id = ?", t.ID).Count(&links).Error; err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&Domain{}).
		Where("tenant_id = ? AND origin = 'self'", t.ID).Count(&domains).Error; err != nil {
		return nil, err
	}
	u.Links = int(links)
	u.Domains = int(domains)
	u.MaxLinks = t.Tier.MaxLinks
	u.MaxDomains = t.Tier.MaxDomains
	return u, nil
}
