package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Domain struct {
	ID          int64  `json:"id" gorm:"primaryKey"`
	TenantID    int64  `json:"-" gorm:"column:tenant_id"`
	FQDN        string `json:"fqdn"`
	Description string `json:"description"`
	Origin      string `json:"origin"`
	Status      string `json:"status"`
	CertStatus  string `json:"certStatus" gorm:"column:cert_status"`
	// VerifyToken 是当前的归属挑战 token:租户须在 _janus-verify.<fqdn> 发布它。
	// 校验通过后不删除 —— 它是"这个 zone 的控制者同意把该域名交给本平台"的
	// 既有证据,active 域名的低频复检要靠它比对;但它不能再被当作"新挑战"使用
	// (重新签发 token 会让旧值立即失效)。
	VerifyToken          string     `json:"verifyToken,omitempty" gorm:"column:verify_token"`
	VerifyTokenCreatedAt *time.Time `json:"verifyTokenCreatedAt,omitempty" gorm:"column:verify_token_created_at"`
	OwnershipVerifiedAt  *time.Time `json:"ownershipVerifiedAt,omitempty" gorm:"column:ownership_verified_at"`
	ActivatedAt          *time.Time `json:"activatedAt" gorm:"column:activated_at"`
	CreatedAt            time.Time  `json:"createdAt" gorm:"column:created_at"`
}

// ErrPlatformDomain 平台默认域名不可删除。
var ErrPlatformDomain = errors.New("platform domain is protected")

// NormalizeFQDN 统一域名形态:去空白、转小写、去尾点。
//
// 放在数据层而不是只在 handler 做:读路径(GetDomainByFQDN / GetDomainAuth)同样要过它,
// 否则 Host 头带尾点、大小写不同的请求查不到已归一化的行,而入库路径与读路径的
// 形态不一致时 UNIQUE 索引会放过 "a.com"/"a.com." 这种同主机双份记录。
func NormalizeFQDN(s string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(s)), ".")
}

// CreateDomain 创建域名记录(fqdn 唯一冲突返回唯一约束错误)。
func (s *Store) CreateDomain(ctx context.Context, tenantID int64, fqdn, origin, description string) (*Domain, error) {
	d := Domain{TenantID: tenantID, FQDN: NormalizeFQDN(fqdn), Origin: origin, Status: "pending", CertStatus: "pending", Description: description}
	if err := s.db.WithContext(ctx).Create(&d).Error; err != nil {
		return nil, err
	}
	return s.GetDomainByID(ctx, d.ID)
}

// CreateDomainChalleged 在事务内创建自有域名,并同时落下 TXT 挑战 token。
// 必须用传入的 tx(WithQuotaInTx 给的那条):配额判断与写入要原子。
func CreateDomainChalleged(tx *gorm.DB, tenantID int64, fqdn, token, description string) (*Domain, error) {
	now := time.Now()
	d := Domain{
		TenantID:             tenantID,
		FQDN:                 NormalizeFQDN(fqdn),
		Origin:               "self",
		Status:               "pending",
		CertStatus:           "pending",
		Description:          description,
		VerifyToken:          token,
		VerifyTokenCreatedAt: &now,
	}
	if err := tx.Create(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// CreatePlatformDomain 创建平台默认域名:创建即 active(泛域名解析已指向本机),cert_status=pending。
func (s *Store) CreatePlatformDomain(ctx context.Context, tenantID int64, fqdn string) (*Domain, error) {
	d := Domain{TenantID: tenantID, FQDN: fqdn, Origin: "platform", Status: "active", CertStatus: "pending", ActivatedAt: nowPtr()}
	if err := s.db.WithContext(ctx).Create(&d).Error; err != nil {
		return nil, err
	}
	return s.GetDomainByID(ctx, d.ID)
}

func nowPtr() *time.Time {
	t := time.Now()
	return &t
}

func (s *Store) GetDomainByID(ctx context.Context, id int64) (*Domain, error) {
	var d Domain
	if err := s.db.WithContext(ctx).First(&d, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (s *Store) GetDomainByFQDN(ctx context.Context, fqdn string) (*Domain, error) {
	var d Domain
	if err := s.db.WithContext(ctx).Where("fqdn = ?", NormalizeFQDN(fqdn)).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

// GetDomainByFQDNAnyStatus 按 FQDN 取域名,不过滤状态。
//
// 跳转路由定位域名用的是"仅 active",因为只有 active 域名才应该承载短链访问。
// 但渲染访客可见的错误页时需要另一套语义:域名存在但被租户停用时,访问者
// 仍应看到该租户自己的 404 页,而不是系统内置页 —— 租户之所以配置自定义
// 错误页,就是为了让"我的域名出错了"长成自己的品牌。所以那条路径需要一个
// 不看状态的查询,并且要把 tenant_id 带回给渲染层。
func (s *Store) GetDomainByFQDNAnyStatus(ctx context.Context, fqdn string) (*Domain, error) {
	var d Domain
	if err := s.db.WithContext(ctx).Where("fqdn = ?", NormalizeFQDN(fqdn)).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (s *Store) ListDomainsByTenant(ctx context.Context, tenantID int64) ([]*Domain, error) {
	var out []*Domain
	if err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("id").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// ListDomainsByStatus 供后台任务扫描(如 DNS 重试队列)。
func (s *Store) ListDomainsByStatus(ctx context.Context, statuses ...string) ([]*Domain, error) {
	var out []*Domain
	if err := s.db.WithContext(ctx).Where("status IN ?", statuses).Order("id").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) SetDomainStatus(ctx context.Context, id int64, status string) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Update("status", status).Error
}

// SetDomainActive 标记 DNS 校验通过:置 active、记录校验时间与激活时间。
func (s *Store) SetDomainActive(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"status":         "active",
		"dns_checked_at": gorm.Expr("now()"),
		"activated_at":   gorm.Expr("COALESCE(activated_at, now())"),
	}).Error
}

// ActivateDomainVerified 归属校验(TXT 挑战)通过后的状态迁移。
//
// 与 SetDomainActive 的差别只有一处,但那一处是重点:ownership_verified_at 记录
// "归属被证明过"这个持续事实。active 域名的低频复检只对有过该时间的域名做 ——
// 存量 active 域名(升级前就已激活)没有它,不会被降级,租户也不会莫名其妙掉线。
// verify_token 保留不删:它是"该 zone 的控制者把域名交给本平台"的既有证据,
// 复检要靠它比对 TXT;同时 verify_token_created_at 清空,表示挑战已被消费
// (不再有 TTL,不会被判成超期)。
func (s *Store) ActivateDomainVerified(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"status":                  "active",
		"ownership_verified_at":   gorm.Expr("now()"),
		"dns_checked_at":          gorm.Expr("now()"),
		"activated_at":            gorm.Expr("COALESCE(activated_at, now())"),
		"verify_token_created_at": nil,
	}).Error
}

// MintVerifyToken 为域名签发新的 TXT 挑战 token(旧值立即失效)。
//
// 重新签发就是让旧 token 一次性失效的机制:被旁路观测到的历史 token 无法重放,
// 租户每次重新校验拿到的都是新值,后台展示的也是新值。
func (s *Store) MintVerifyToken(ctx context.Context, id int64, token string) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"verify_token":            token,
		"verify_token_created_at": gorm.Expr("now()"),
	}).Error
}

// ReviveDomain 把终态/失败的域名放回重试队列,并重置校验时钟。
//
// dns_checked_at 必须重写:扫描 SQL 用 COALESCE(dns_checked_at, created_at) 算
// "距上次校验多久",一个两年前创建、刚被手动复活的域名若沿用旧 created_at,
// 下一轮就会被判超期再打回终态。
func (s *Store) ReviveDomain(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"status":         "pending",
		"dns_checked_at": gorm.Expr("now()"),
	}).Error
}

// MarkDomainExpired 把超期仍未完成归属证明的域名置为终态 expired。
func (s *Store) MarkDomainExpired(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"status":         "expired",
		"dns_checked_at": gorm.Expr("now()"),
	}).Error
}

// DegradeDomain 归属复检不通过时的降级。
//
// 落点是 failed 而不是 stopped:failed 仍在重试队列里,租户把 DNS 修好之后
// 会自动重新激活;stopped 的语义是"租户主动暂停",系统不该替租户做这个决定。
// cert_status 一并打回 pending:证书状态描述的是上一次探活的结果,而这次
// 探活的对象已经不存在了;恢复后需要重新探活才谈得上 issued。
func (s *Store) DegradeDomain(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"status":         "failed",
		"cert_status":    "pending",
		"dns_checked_at": gorm.Expr("now()"),
	}).Error
}

// DomainScanRow 是后台扫描用的轻量行(只取处理所需列,不载入整表)。
type DomainScanRow struct {
	ID          int64  `json:"id" gorm:"column:id"`
	FQDN        string `json:"fqdn" gorm:"column:fqdn"`
	Status      string `json:"status" gorm:"column:status"`
	CertStatus  string `json:"certStatus" gorm:"column:cert_status"`
	VerifyToken string `json:"verifyToken" gorm:"column:verify_token"`
	// LastAt 本行的调度时钟(COALESCE(dns_checked_at, created_at) 或
	// COALESCE(cert_probed_at, created_at)),用来在 SQL 里做退避与排序。
	LastAt time.Time `json:"lastAt" gorm:"column:last_at"`
	// Overdue 是否已超过最长等待:应转入终态,不再重试。
	Overdue bool `json:"overdue" gorm:"column:overdue"`
	// VerifyTokenCreatedAt 当前挑战签发时间;挑战过期判定要用它,
	// 不能用 LastAt(每轮复检都刷新,时间会被重置,判不出真实挑战年龄)。
	VerifyTokenCreatedAt *time.Time `json:"verifyTokenCreatedAt,omitempty" gorm:"column:verify_token_created_at"`
}

// DefaultDomainScanLimit 单轮扫描的行数上限。
//
// 原实现是 ListDomainsByStatus 全表载入 + 无条件逐条写库,pending+failed 的规模
// 与出网请求数完全由租户注册量决定。每一轮都必须有界。
const DefaultDomainScanLimit = 500

// ListDomainsForDNSCheck 取本轮到期需要复检的自有域名(pending/failed),带 LIMIT。
// Overdue 表示"距建域名已超过 maxAge 仍未完成归属证明",该行会被转入 expired 终态。
//
// 两个时间基准不能混用,这是本函数最容易写错的地方:
//
//   - 复检节奏(选不选这一行)按 COALESCE(dns_checked_at, created_at) 算,
//     即"距上次校验多久了"。每轮校验完都会把 dns_checked_at 刷新成 now,
//     所以它天然就是一个每 interval 一次的固定节奏(对应文档里的"每 5 分钟重试一次")。
//   - 超期判定(overdue)按 **verify_token_created_at**(当前这枚挑战签发于何时)
//     算,不能用上面那个时钟,也不能用 created_at:
//     dns_checked_at 每轮被重置,拿它算年龄会让 age 永远≈0 —— 域名因此永远
//     达不到 expired 终态,72h 之后仍在队列里被无意义地反复校验、反复写库;
//     而 created_at 是域名的创建时间,租户在 expired 之后手动重新校验、拿到
//     新挑战时它不会变,下一轮(默认 5 分钟后)就会把刚复活的域名再打回终态,
//     显式复活变成一句空话。verify_token_created_at 每次签发挑战都会重置,
//     正好表达"当前这次尝试已经挂了多久"。
//
// 顺带说明:曾经这里写过一段
// `age_s > $1 OR age_s >= LEAST(6, floor(age_s/$2))*$2` 的"指数退避"。
// 它推不出真正的退避 —— age 既然每轮被重置,floor(age/interval) 就恒定在
// 刚过一轮的量级,窗口永远长不大。真正的退避需要 attempts 计数字段(每次
// 校验自增,窗口按 attempts 增长);在此之前宁可保持"固定节奏"这一条
// 真实且可预期的语义,也不留一段看起来在做退避、实际没做的代码。
func (s *Store) ListDomainsForDNSCheck(ctx context.Context, maxAge, interval time.Duration, limit int) ([]DomainScanRow, error) {
	return s.scanDomains(ctx, `
		SELECT id, fqdn, status, cert_status, verify_token, last_at,
		       verify_token_created_at,
		       (EXTRACT(EPOCH FROM (now() - COALESCE(verify_token_created_at, created_at))) > $1) AS overdue
		FROM (
			SELECT id, fqdn, status, cert_status, verify_token,
			       created_at,
			       verify_token_created_at,
			       COALESCE(dns_checked_at, created_at) AS last_at,
			       GREATEST(0, EXTRACT(EPOCH FROM (now() - COALESCE(dns_checked_at, created_at)))) AS age_s
			FROM domains
			WHERE origin = 'self' AND status IN ('pending','failed')
		) c
		WHERE age_s >= $2
		ORDER BY last_at
		LIMIT $3`,
		maxAge.Seconds(), interval.Seconds(), limit)
}

// ListDomainsForCertProbe 取本轮需要探活的域名:active、证书未签发、且所属租户 active。
//
// 租户状态这一条是必须的:平台默认域名在注册时就是 active(邮箱验证后才激活租户),
// 于是每个从未验证过的租户都会让 certProbePass 每 5 分钟发一次 HTTPS,Caddy 因
// pending 返 403 → 探活失败 → 永远重试。注册 spam 因此线性放大成出网请求。
func (s *Store) ListDomainsForCertProbe(ctx context.Context, interval time.Duration, limit int) ([]DomainScanRow, error) {
	return s.scanDomains(ctx, `
		SELECT d.id, d.fqdn, d.status, d.cert_status, d.verify_token,
		       COALESCE(d.cert_probed_at, d.created_at) AS last_at, false AS overdue
		FROM domains d
		JOIN tenants t ON t.id = d.tenant_id
		WHERE d.status = 'active'
		  AND d.cert_status IN ('pending','failed')
		  AND t.status = 'active'
		  AND COALESCE(d.cert_probed_at, d.created_at) <= now() - $1::interval
		ORDER BY COALESCE(d.cert_probed_at, d.created_at)
		LIMIT $2`,
		interval.String(), limit)
}

// ListDomainsForOwnershipRecheck 取本轮需要复检归属的 active 自有域名。
//
// 只取 ownership_verified_at 非空的:那表示这条记录是按新规则(TXT 挑战)激活的,
// "激活"因而是一个需要持续维护的状态而不是一次性快照。存量 active 域名没有这个
// 时间,不会被降级(升级不该让老租户的服务凭空中断;租户点一次"重新校验"即
// 进入新规则)。
func (s *Store) ListDomainsForOwnershipRecheck(ctx context.Context, interval time.Duration, limit int) ([]DomainScanRow, error) {
	return s.scanDomains(ctx, `
		SELECT d.id, d.fqdn, d.status, d.cert_status, d.verify_token,
		       COALESCE(d.ownership_verified_at, d.created_at) AS last_at, false AS overdue
		FROM domains d
		JOIN tenants t ON t.id = d.tenant_id
		WHERE d.origin = 'self'
		  AND d.status = 'active'
		  AND d.ownership_verified_at IS NOT NULL
		  AND t.status = 'active'
		  AND COALESCE(d.ownership_verified_at, d.created_at) <= now() - $1::interval
		ORDER BY COALESCE(d.ownership_verified_at, d.created_at)
		LIMIT $2`,
		interval.String(), limit)
}

func (s *Store) scanDomains(ctx context.Context, query string, args ...any) ([]DomainScanRow, error) {
	var out []DomainScanRow
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) MarkDomainDNSChecked(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).Update("dns_checked_at", gorm.Expr("now()")).Error
}

func (s *Store) SetDomainCertStatus(ctx context.Context, id int64, status string) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("id = ?", id).
		Updates(map[string]any{"cert_status": status, "cert_probed_at": gorm.Expr("now()")}).Error
}

// SetDomainCertStatusByFQDN 按 fqdn 更新证书状态(验证通过后触发探活用)。
func (s *Store) SetDomainCertStatusByFQDN(ctx context.Context, fqdn, status string) error {
	return s.db.WithContext(ctx).Model(&Domain{}).Where("fqdn = ?", fqdn).
		Updates(map[string]any{"cert_status": status, "cert_probed_at": gorm.Expr("now()")}).Error
}

// DeleteDomain 物理删除域名(调用方须先保证其短链关联已清空)。
func (s *Store) DeleteDomain(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Delete(&Domain{}, id).Error
}

// DomainAuth 返回授权端点所需的域名与租户状态。
type DomainAuth struct {
	Status       string
	Origin       string
	TenantStatus string
}

// GetDomainAuth 按 fqdn 查询域名与所属租户状态(用于 Caddy 授权端点)。
func (s *Store) GetDomainAuth(ctx context.Context, fqdn string) (*DomainAuth, error) {
	var a DomainAuth
	res := s.db.WithContext(ctx).Raw(
		`SELECT d.status, d.origin, t.status AS tenant_status FROM domains d
		 JOIN tenants t ON t.id = d.tenant_id WHERE d.fqdn = ?`, NormalizeFQDN(fqdn)).Scan(&a)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &a, nil
}

// CountNonDeletedLinksOnDomain 统计某域名上"未删除"短链的关联数。
func (s *Store) CountNonDeletedLinksOnDomain(ctx context.Context, domainID int64) (int, error) {
	var n int64
	err := s.db.WithContext(ctx).Table("link_domains").
		Joins("JOIN links l ON l.id = link_domains.link_id").
		Where("link_domains.domain_id = ? AND l.deleted_at IS NULL", domainID).Count(&n).Error
	return int(n), err
}

// DeleteDomainIfUnused 在**一个事务**内完成"还有未删除短链?"的计数与 detach。
//
// 为什么必须合并:原先 CountNonDeletedLinksOnDomain 与 DetachDomain 是两次独立调用,
// 中间没有锁。两次请求交错时(创建短链只校验 status == 'active',不做任何锁定),
// 新建的 link_domains 行会被 detach 里的 `DELETE FROM link_domains WHERE domain_id = ?`
// 连带删掉:短链存活但零域名、不可达、仍占短链配额,而外键方向是
// link_domains → domains 的 CASCADE,任何一层都不会报错。
//
// FOR UPDATE 锁住 domains 行之后,并发创建短链若也锁同一行(见报告里的跨文件清单)
// 就会被串行化;本路径至少保证"计数与删除之间不存在可插入的窗口"。
//
// 返回值:n>0 时域名未被删除,调用方据此返回 409。平台默认域名一律拒绝
// (ErrPlatformDomain):租户端点原本有这道约束,超管强删路径漏了,删掉后租户既没有
// 默认域名也没有任何接口能重建。
func (s *Store) DeleteDomainIfUnused(ctx context.Context, domainID int64) (purged []int64, links int, err error) {
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var d Domain
		// FOR UPDATE:与创建短链路径锁同一行,让"计数 → 删除"成为临界区。
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", domainID).First(&d).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if d.Origin == "platform" {
			return ErrPlatformDomain
		}
		var n int64
		if err := tx.Table("link_domains").
			Joins("JOIN links l ON l.id = link_domains.link_id").
			Where("link_domains.domain_id = ? AND l.deleted_at IS NULL", domainID).
			Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			links = int(n)
			return errDomainInUseRollback
		}
		var err error
		purged, err = detachInTx(tx, domainID)
		return err
	})
	if errors.Is(err, errDomainInUseRollback) {
		return nil, links, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return purged, 0, nil
}

// errDomainInUseRollback 只用于在事务里表达"检测到占用,回滚并把计数带出去"。
var errDomainInUseRollback = errors.New("domain in use")

// TryAdvisoryLock 尝试取一个会话级咨询锁,返回是否取到与释放函数。
//
// 为什么需要:worker 的每一轮(重试队列 / 证书探活 / 归属复检)都会发出网请求。
// 多副本部署时每一轮都会被执行 N 遍 —— N 倍的 DNS/ACME 出网、N 倍的库写入,
// 而单副本时这个问题完全看不见。取锁的副本执行本轮,没取到的直接跳过。
// 会话级锁必须绑定同一条连接,所以这里单独占一条并由 release 关闭。
func (s *Store) TryAdvisoryLock(ctx context.Context, key int64) (release func(), acquired bool, err error) {
	sqlDB, err := s.db.DB()
	if err != nil {
		return nil, false, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	var got bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if !got {
		_ = conn.Close()
		return func() {}, false, nil
	}
	return func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key)
		_ = conn.Close()
	}, true, nil
}

// detachInTx 在事务内清空域名关联并删除域名行,对"仅关联该域名"且已逻辑删除的
// 短链做物理清除(连同访问记录),返回被物理清除的短链 ID 供调用方清理落地页目录。
func detachInTx(tx *gorm.DB, domainID int64) ([]int64, error) {
	var purged []int64
	if err := tx.Raw(`SELECT l.id FROM links l
		JOIN link_domains ld ON ld.link_id = l.id AND ld.domain_id = ?
		WHERE l.deleted_at IS NOT NULL
		  AND (SELECT count(*) FROM link_domains WHERE link_id = l.id) = 1`,
		domainID).Scan(&purged).Error; err != nil {
		return nil, err
	}
	if len(purged) > 0 {
		if err := tx.Exec(`DELETE FROM links WHERE id = ANY($1)`, purged).Error; err != nil {
			return nil, err
		}
	}
	if err := tx.Where("domain_id = ?", domainID).Delete(&LinkDomain{}).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("domain_id = ?", domainID).Delete(&Visit{}).Error; err != nil {
		return nil, err
	}
	if err := tx.Delete(&Domain{}, domainID).Error; err != nil {
		return nil, err
	}
	return purged, nil
}

// DetachDomain 强制删除域名(超管移除违规域名):不要求该域名上没有未删除短链,
// 直接清空全部关联后删除,返回被物理清除的短链 ID。
func (s *Store) DetachDomain(ctx context.Context, domainID int64) ([]int64, error) {
	var purged []int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 与租户删除路径锁同一行:创建短链若也锁这一行,就不会出现
		// "detach 删光关联的同时新建短链写进 link_domains"这种悬空行。
		var d Domain
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", domainID).First(&d).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		// 平台默认域名与租户端点一致地拒绝:超管路径原本漏了这道约束,
		// 删掉之后租户既没有默认域名,也没有任何接口能重建它。
		if d.Origin == "platform" {
			return ErrPlatformDomain
		}
		var err error
		purged, err = detachInTx(tx, domainID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return purged, nil
}

// DomainFQDNExists 判断 fqdn 是否已存在(用于注册 slug 与域名冲突校验)。
func (s *Store) DomainFQDNExists(ctx context.Context, fqdn string) (bool, error) {
	return DomainFQDNExistsWithDB(s.db.WithContext(ctx), fqdn)
}

// DomainFQDNExistsWithDB 在给定 db(可以是 WithQuotaInTx 事务里的 tx)上判断 fqdn 是否已存在。
//
// 创建域名的路径必须在事务内复查一次唯一性:事务外的预检查只是为了给出更友好的
// 409,判定必须以锁内这次为准,否则并发创建仍会靠 UNIQUE 索引兜底并把内部错误
// 暴露成 500。
func DomainFQDNExistsWithDB(db *gorm.DB, fqdn string) (bool, error) {
	var n int64
	if err := db.Model(&Domain{}).Where("fqdn = ?", NormalizeFQDN(fqdn)).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}
