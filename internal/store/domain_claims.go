package store

// 域名认领(防抢注)与 expired 行回收。
//
// 问题:domains.fqdn 是全局 UNIQUE。任何租户都能先插一条 pending 行占住别人的
// 域名 —— pending 超期变 expired,expired 又能被手动"重新校验"复活,于是这个
// 全局唯一的槽位可以被永久占住,真正的域名所有者永远加不进来(409 域名已被占用)。
//
// 方案取舍:
//   - 放宽 UNIQUE 为"只对已证明归属/active 的行唯一"(部分唯一索引)也能解决,
//     但跳转热路径、Caddy 授权端点、错误页、cert 探活等读路径全部按 fqdn 取
//     "那一行",一旦同名多行,每条读路径都要补"挑哪一行"的规则,改动面大且容易漏。
//   - 这里选择保留"一个 FQDN 至多一行",改为**凭归属证明驱逐**:被别的租户占住、
//     但从未证明过归属(ownership_verified_at IS NULL)且不在服务中
//     (pending / expired / failed)的行,可以被一个能在 _janus-verify.<fqdn>
//     发布认领方自己 TXT token 的租户接管。能发布那条 TXT 的人就是 zone 的控制者,
//     占位行对他没有任何正当性可言。
//   - 认领方的 token 存在 domain_claims(按 租户+域名 唯一),与被占行的
//     verify_token 完全隔离:占位者看不到、也改不了认领方的挑战。
//   - 另外 worker 定期回收长期 expired 且从未挂过短链的行,占位不会无限累积。

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ExpiredDomainRetention expired 自有域名(从未完成归属证明)保留多久后被回收。
const ExpiredDomainRetention = 30 * 24 * time.Hour

// ErrDomainNotClaimable 目标域名不满足被认领条件(已证明归属/正在服务/仍挂着短链等)。
var ErrDomainNotClaimable = errors.New("domain not claimable")

// DomainClaim 某租户对某个被占用域名的认领挑战。
type DomainClaim struct {
	ID          int64     `json:"id" gorm:"primaryKey"`
	TenantID    int64     `json:"-" gorm:"column:tenant_id"`
	FQDN        string    `json:"fqdn" gorm:"column:fqdn"`
	VerifyToken string    `json:"verifyToken" gorm:"column:verify_token"`
	CreatedAt   time.Time `json:"createdAt" gorm:"column:created_at"`
}

// IsDomainClaimable 判断 d 是否可以被 claimant 租户凭归属证明接管(纯逻辑)。
//
// 条件全部满足才可认领:
//   - 属于别的租户(自己的行走正常的"重新校验",不走驱逐);
//   - 自有域名(平台默认域名由平台泛解析决定,不存在"抢注");
//   - 从未证明过归属(ownership_verified_at 为空)—— 证明过的行背后是一个曾经控制
//     过 zone 的人,不能被另一次 TXT 静默顶掉(那是易主,要走复检降级 + 删除);
//   - 不在服务中:pending / expired / failed。active 与 stopped 都意味着租户曾经
//     正常持有并使用它(含升级前按旧规则激活的存量域名),不参与认领。
func IsDomainClaimable(d *Domain, claimantTenantID int64) bool {
	if d == nil || d.TenantID == claimantTenantID {
		return false
	}
	if d.Origin != "self" || d.OwnershipVerifiedAt != nil {
		return false
	}
	switch d.Status {
	case "pending", "expired", "failed":
		return true
	default:
		return false
	}
}

// GetOrMintDomainClaim 返回租户对该域名的认领挑战;不存在或已超过 maxAge 时用
// token 新建/轮换(旧值立即失效)。fqdn 须已归一化。
func (s *Store) GetOrMintDomainClaim(ctx context.Context, tenantID int64, fqdn, token string, maxAge time.Duration) (*DomainClaim, error) {
	fqdn = NormalizeFQDN(fqdn)
	db := s.db.WithContext(ctx)
	// 只在"不存在"或"已过期"时写入:未过期的挑战必须保持稳定,
	// 否则租户每提交一次就换一枚 token,刚发布的 TXT 永远对不上。
	if err := db.Exec(`
		INSERT INTO domain_claims (tenant_id, fqdn, verify_token) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, fqdn) DO UPDATE
		   SET verify_token = EXCLUDED.verify_token, created_at = now()
		 WHERE domain_claims.created_at < now() - $4::interval`,
		tenantID, fqdn, token, maxAge.String()).Error; err != nil {
		return nil, err
	}
	var c DomainClaim
	if err := db.Where("tenant_id = ? AND fqdn = ?", tenantID, fqdn).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ClaimDomainInTx 在事务内接管一个被占用的域名:锁住旧行、复核可认领、驱逐旧行、
// 为认领方建行并直接置为"已证明归属的 active"。
//
// 调用方必须已经用认领挑战 token 通过了归属校验(DNS 查询不放在事务里:
// 它最长数秒,压在租户行锁里就是一条可被慢 DNS 放大的串行化长事务)。
// 事务内的复核负责兜住"校验之后、加锁之前"旧行状态的变化(例如被占位者
// 并发激活 —— 那说明占位者也能发布 TXT,此时以先到者为准,返回不可认领)。
//
// 旧行若还挂着任何短链关联(含已删除短链),一律不驱逐:那些关联属于另一个
// 租户的数据,不能因为认领被连带删除。从未证明归属的域名本来也挂不上短链
// (创建短链要求域名 active),这一条只为极少数存量数据兜底。
func ClaimDomainInTx(tx *gorm.DB, tenantID int64, fqdn, token, description string) (*Domain, error) {
	fqdn = NormalizeFQDN(fqdn)
	var old Domain
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("fqdn = ?", fqdn).First(&old).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 占位行已经消失(被删/被回收):按普通新建处理。
	case err != nil:
		return nil, err
	default:
		if !IsDomainClaimable(&old, tenantID) {
			return nil, ErrDomainNotClaimable
		}
		var refs int64
		if err := tx.Table("link_domains").Where("domain_id = ?", old.ID).Count(&refs).Error; err != nil {
			return nil, err
		}
		if refs > 0 {
			return nil, ErrDomainNotClaimable
		}
		// 访问记录对 domains 是无级联外键;从未激活的域名不会有访问记录,这里仍一并清掉。
		if err := tx.Where("domain_id = ?", old.ID).Delete(&Visit{}).Error; err != nil {
			return nil, err
		}
		if err := tx.Delete(&Domain{}, old.ID).Error; err != nil {
			return nil, err
		}
	}
	d, err := CreateDomainChalleged(tx, tenantID, fqdn, token, description)
	if err != nil {
		return nil, err
	}
	// 归属已经被认领方的 TXT 证明:与 ActivateDomainVerified 同一组状态迁移。
	if err := tx.Model(&Domain{}).Where("id = ?", d.ID).Updates(map[string]any{
		"status":                  "active",
		"ownership_verified_at":   gorm.Expr("now()"),
		"dns_checked_at":          gorm.Expr("now()"),
		"activated_at":            gorm.Expr("COALESCE(activated_at, now())"),
		"verify_token_created_at": nil,
		"recheck_failures":        0,
	}).Error; err != nil {
		return nil, err
	}
	// 该域名已有明确归属:所有租户对它的认领挑战都作废。
	if err := tx.Where("fqdn = ?", fqdn).Delete(&DomainClaim{}).Error; err != nil {
		return nil, err
	}
	var out Domain
	if err := tx.First(&out, d.ID).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteStaleDomainClaims 删除超过 maxAge 的认领挑战(过期挑战会在下次认领时重签,
// 留着只占空间),返回删除行数。
func (s *Store) DeleteStaleDomainClaims(ctx context.Context, maxAge time.Duration) (int64, error) {
	res := s.db.WithContext(ctx).Exec(
		`DELETE FROM domain_claims WHERE created_at < now() - $1::interval`, maxAge.String())
	return res.RowsAffected, res.Error
}

// GCExpiredDomains 回收长期 expired 的自有域名行,返回删除行数(单轮最多 limit 行)。
//
// 只删同时满足下列条件的行:
//   - status = 'expired' 且 origin = 'self';
//   - 进入 expired(dns_checked_at)已超过 olderThan;
//   - 没有任何短链关联、没有任何访问记录(从未承载过流量,删掉不丢租户数据,
//     也就不需要清理落地页文件)。
//
// 外层 WHERE 复核 status:子查询选中之后、DELETE 之前若被租户手动"重新校验"
// 复活成 pending,Postgres 会对并发更新过的行重新求值外层条件,不会误删。
func (s *Store) GCExpiredDomains(ctx context.Context, olderThan time.Duration, limit int) (int64, error) {
	res := s.db.WithContext(ctx).Exec(`
		DELETE FROM domains
		 WHERE status = 'expired' AND origin = 'self'
		   AND id IN (
			SELECT d.id FROM domains d
			 WHERE d.status = 'expired' AND d.origin = 'self'
			   AND COALESCE(d.dns_checked_at, d.created_at) < now() - $1::interval
			   AND NOT EXISTS (SELECT 1 FROM link_domains ld WHERE ld.domain_id = d.id)
			   AND NOT EXISTS (SELECT 1 FROM visits v WHERE v.domain_id = d.id)
			 ORDER BY COALESCE(d.dns_checked_at, d.created_at)
			 LIMIT $2
			 FOR UPDATE SKIP LOCKED)`,
		olderThan.String(), limit)
	return res.RowsAffected, res.Error
}
