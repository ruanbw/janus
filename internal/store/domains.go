package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

const domainColumns = `id, tenant_id, fqdn, origin, status, cert_status, activated_at, created_at`

func scanDomain(row pgx.Row) (*Domain, error) {
	var d Domain
	err := row.Scan(&d.ID, &d.TenantID, &d.FQDN, &d.Origin, &d.Status, &d.CertStatus, &d.ActivatedAt, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// CreateDomain 创建域名记录(fqdn 唯一冲突返回唯一约束错误)。
func (s *Store) CreateDomain(ctx context.Context, tenantID int64, fqdn, origin string) (*Domain, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO domains (tenant_id, fqdn, origin, status) VALUES ($1,$2,$3,$4) RETURNING id`,
		tenantID, fqdn, origin, "pending",
	).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetDomainByID(ctx, id)
}

// CreatePlatformDomain 创建平台默认域名:创建即 active(泛域名解析已指向本机),cert_status=pending。
func (s *Store) CreatePlatformDomain(ctx context.Context, tenantID int64, fqdn string) (*Domain, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO domains (tenant_id, fqdn, origin, status, activated_at) VALUES ($1,$2,'platform','active',now()) RETURNING id`,
		tenantID, fqdn,
	).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetDomainByID(ctx, id)
}

func (s *Store) GetDomainByID(ctx context.Context, id int64) (*Domain, error) {
	d, err := scanDomain(s.pool.QueryRow(ctx,
		`SELECT `+domainColumns+` FROM domains WHERE id=$1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return d, nil
}

func (s *Store) GetDomainByFQDN(ctx context.Context, fqdn string) (*Domain, error) {
	d, err := scanDomain(s.pool.QueryRow(ctx,
		`SELECT `+domainColumns+` FROM domains WHERE fqdn=$1`, fqdn))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return d, nil
}

func (s *Store) ListDomainsByTenant(ctx context.Context, tenantID int64) ([]*Domain, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+domainColumns+` FROM domains WHERE tenant_id=$1 ORDER BY id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Domain
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDomainsByStatus 供后台任务扫描(如 DNS 重试队列)。
func (s *Store) ListDomainsByStatus(ctx context.Context, statuses ...string) ([]*Domain, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+domainColumns+` FROM domains WHERE status = ANY($1) ORDER BY id`, statuses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Domain
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) SetDomainStatus(ctx context.Context, id int64, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE domains SET status=$1 WHERE id=$2`, status, id)
	return err
}

// SetDomainActive 标记 DNS 校验通过:置 active、记录校验时间与激活时间。
func (s *Store) SetDomainActive(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE domains SET status='active', dns_checked_at=now(), activated_at=COALESCE(activated_at, now()) WHERE id=$1`, id)
	return err
}

func (s *Store) MarkDomainDNSChecked(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE domains SET dns_checked_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) SetDomainCertStatus(ctx context.Context, id int64, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE domains SET cert_status=$1, cert_probed_at=now() WHERE id=$2`, status, id)
	return err
}

// SetDomainCertStatusByFQDN 按 fqdn 更新证书状态(验证通过后触发探活用)。
func (s *Store) SetDomainCertStatusByFQDN(ctx context.Context, fqdn, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE domains SET cert_status=$1, cert_probed_at=now() WHERE fqdn=$2`, status, fqdn)
	return err
}

// DeleteDomain 物理删除域名(调用方须先保证其短链关联已清空)。
func (s *Store) DeleteDomain(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM domains WHERE id=$1`, id)
	return err
}

// DomainAuth 返回授权端点所需的域名与租户状态。
type DomainAuth struct {
	Status      string
	Origin      string
	TenantStatus string
}

// GetDomainAuth 按 fqdn 查询域名与所属租户状态(用于 Caddy 授权端点)。
func (s *Store) GetDomainAuth(ctx context.Context, fqdn string) (*DomainAuth, error) {
	var a DomainAuth
	err := s.pool.QueryRow(ctx,
		`SELECT d.status, d.origin, t.status FROM domains d JOIN tenants t ON t.id=d.tenant_id WHERE d.fqdn=$1`, fqdn,
	).Scan(&a.Status, &a.Origin, &a.TenantStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

// CountNonDeletedLinksOnDomain 统计某域名上"未删除"短链的关联数。
func (s *Store) CountNonDeletedLinksOnDomain(ctx context.Context, domainID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM link_domains ld
		 JOIN links l ON l.id = ld.link_id
		 WHERE ld.domain_id=$1 AND l.deleted_at IS NULL`, domainID,
	).Scan(&n)
	return n, err
}

// DetachDomain 删除域名:清空该域名全部关联;对"仅关联该域名"且已逻辑删除的短链做物理清除(连同访问记录)。
// 调用方须先确认无未删除短链关联(CountNonDeletedLinksOnDomain == 0)。
func (s *Store) DetachDomain(ctx context.Context, domainID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 该域名上"已逻辑删除"的短链,若其关联数(全部域名)为 1(即仅此域名),物理删除
	_, err = tx.Exec(ctx, `
		DELETE FROM links WHERE id IN (
			SELECT l.id FROM links l
			JOIN link_domains ld ON ld.link_id = l.id AND ld.domain_id = $1
			WHERE l.deleted_at IS NOT NULL
			  AND (SELECT count(*) FROM link_domains WHERE link_id = l.id) = 1
		)`, domainID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM link_domains WHERE domain_id=$1`, domainID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM visits WHERE domain_id=$1`, domainID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM domains WHERE id=$1`, domainID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DomainFQDNExists 判断 fqdn 是否已存在(用于注册 slug 与域名冲突校验)。
func (s *Store) DomainFQDNExists(ctx context.Context, fqdn string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM domains WHERE fqdn=$1)`, fqdn).Scan(&exists)
	return exists, err
}
