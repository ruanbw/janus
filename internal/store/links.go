package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const linkColumns = `id, tenant_id, code, target_url, redirect_status, status, deleted_at, created_at`

func scanLink(row pgx.Row) (*Link, error) {
	var l Link
	var deletedAt *time.Time
	var redirectStatus int
	err := row.Scan(&l.ID, &l.TenantID, &l.Code, &l.TargetURL, &redirectStatus, &l.Status, &deletedAt, &l.CreatedAt)
	if err != nil {
		return nil, err
	}
	l.RedirectStatus = RedirectStatus(strconv.Itoa(redirectStatus))
	return &l, nil
}

// fillLinkMeta 补充 domains(fqdn 列表)与 visits 计数。
func (s *Store) fillLinkMeta(ctx context.Context, l *Link) error {
	domains, err := s.linkDomainFQDNs(ctx, l.ID)
	if err != nil {
		return err
	}
	l.Domains = domains
	n, err := s.CountVisitsByLink(ctx, l.ID)
	if err != nil {
		return err
	}
	l.Visits = n
	return nil
}

func (s *Store) linkDomainFQDNs(ctx context.Context, linkID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT d.fqdn FROM link_domains ld JOIN domains d ON d.id=ld.domain_id WHERE ld.link_id=$1 ORDER BY d.id`, linkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// CreateLink 创建短链并关联域名。
// 任一 (domain_id, code) 与既有关联冲突时返回唯一约束错误(整个创建回滚)。
func (s *Store) CreateLink(ctx context.Context, tenantID int64, code, targetURL string, redirectStatus RedirectStatus, domainIDs []int64) (*Link, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	redirectStatusInt, _ := strconv.Atoi(string(redirectStatus))
	var linkID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO links (tenant_id, code, target_url, redirect_status) VALUES ($1,$2,$3,$4) RETURNING id`,
		tenantID, code, targetURL, redirectStatusInt,
	).Scan(&linkID)
	if err != nil {
		return nil, err
	}
	for _, dID := range domainIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO link_domains (link_id, domain_id, code) VALUES ($1,$2,$3)`, linkID, dID, code); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetLinkByID(ctx, tenantID, linkID)
}

// GetLinkByID 按 id 查询短链(租户隔离;不含逻辑删除)。
func (s *Store) GetLinkByID(ctx context.Context, tenantID, id int64) (*Link, error) {
	l, err := scanLink(s.pool.QueryRow(ctx,
		`SELECT `+linkColumns+` FROM links WHERE id=$1 AND tenant_id=$2 AND deleted_at IS NULL`, id, tenantID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.fillLinkMeta(ctx, l); err != nil {
		return nil, err
	}
	return l, nil
}

// ListLinksByTenant 分页列出租户短链(不含逻辑删除),按创建时间倒序。
func (s *Store) ListLinksByTenant(ctx context.Context, tenantID int64, page, pageSize int) ([]*Link, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM links WHERE tenant_id=$1 AND deleted_at IS NULL`, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+linkColumns+` FROM links WHERE tenant_id=$1 AND deleted_at IS NULL
		 ORDER BY id DESC LIMIT $2 OFFSET $3`, tenantID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*Link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for _, l := range out {
		if err := s.fillLinkMeta(ctx, l); err != nil {
			return nil, 0, err
		}
	}
	return out, total, nil
}

// LinkUpdate 短链局部更新字段(指针非空才更新)。
type LinkUpdate struct {
	TargetURL      *string
	RedirectStatus *RedirectStatus
	Status         *string
	// DomainIDs 非空时整体替换关联域名(空数组 = 清空关联,由调用方保证不合法场景已拦截)。
	DomainIDs *[]int64
}

// UpdateLink 更新短链(租户隔离)。domainIDs 替换时若与既有关联冲突返回唯一约束错误。
func (s *Store) UpdateLink(ctx context.Context, tenantID, id int64, upd LinkUpdate) (*Link, error) {
	cur, err := s.GetLinkByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	targetURL, redirectStatus, status := cur.TargetURL, cur.RedirectStatus, cur.Status
	if upd.TargetURL != nil {
		targetURL = *upd.TargetURL
	}
	if upd.RedirectStatus != nil {
		redirectStatus = *upd.RedirectStatus
	}
	if upd.Status != nil {
		status = *upd.Status
	}
	redirectStatusInt, _ := strconv.Atoi(string(redirectStatus))
	if _, err := tx.Exec(ctx,
		`UPDATE links SET target_url=$1, redirect_status=$2, status=$3 WHERE id=$4 AND tenant_id=$5`,
		targetURL, redirectStatusInt, status, id, tenantID); err != nil {
		return nil, err
	}
	if upd.DomainIDs != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM link_domains WHERE link_id=$1`, id); err != nil {
			return nil, err
		}
		for _, dID := range *upd.DomainIDs {
			if _, err := tx.Exec(ctx,
				`INSERT INTO link_domains (link_id, domain_id, code) VALUES ($1,$2,$3)`, id, dID, cur.Code); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetLinkByID(ctx, tenantID, id)
}

// SoftDeleteLink 逻辑删除(deleted_at 置位,记录保留)。
func (s *Store) SoftDeleteLink(ctx context.Context, tenantID, id int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE links SET deleted_at=now() WHERE id=$1 AND tenant_id=$2 AND deleted_at IS NULL`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// PurgeLink 物理删除短链(连同关联与访问记录)。
func (s *Store) PurgeLink(ctx context.Context, tenantID, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM links WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CountActiveLinks 按"尚未物理删除"计数(含逻辑删除行),用于配额校验。
func (s *Store) CountActiveLinks(ctx context.Context, tenantID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM links WHERE tenant_id=$1`, tenantID).Scan(&n)
	return n, err
}

// ResolveRedirect 跳转路由:在 active 域名下按短码命中未删除、启用的短链。
// 返回短链与命中的域名记录;未命中返回 ErrNotFound。
func (s *Store) ResolveRedirect(ctx context.Context, domainID int64, code string) (*Link, *Domain, error) {
	var l Link
	var d Domain
	var redirectStatus int
	err := s.pool.QueryRow(ctx,
		`SELECT l.id, l.tenant_id, l.code, l.target_url, l.redirect_status, l.status, l.created_at,
		        d.id, d.tenant_id, d.fqdn, d.origin, d.status, d.cert_status, d.activated_at, d.created_at
		 FROM link_domains ld
		 JOIN links l ON l.id = ld.link_id
		 JOIN domains d ON d.id = ld.domain_id
		 WHERE ld.domain_id=$1 AND ld.code=$2
		   AND l.deleted_at IS NULL AND l.status='enabled' AND d.status='active'`,
		domainID, code,
	).Scan(&l.ID, &l.TenantID, &l.Code, &l.TargetURL, &redirectStatus, &l.Status, &l.CreatedAt,
		&d.ID, &d.TenantID, &d.FQDN, &d.Origin, &d.Status, &d.CertStatus, &d.ActivatedAt, &d.CreatedAt)
	l.RedirectStatus = RedirectStatus(strconv.Itoa(redirectStatus))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	return &l, &d, nil
}
