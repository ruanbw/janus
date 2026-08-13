package store

import (
	"context"
	"time"
)

// InsertVisit 记录一次成功跳转。
func (s *Store) InsertVisit(ctx context.Context, linkID, domainID int64, userAgent, referer string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO visits (link_id, domain_id, user_agent, referer) VALUES ($1,$2,$3,$4)`,
		linkID, domainID, userAgent, referer)
	return err
}

func (s *Store) CountVisitsByLink(ctx context.Context, linkID int64) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM visits WHERE link_id=$1`, linkID).Scan(&n)
	return n, err
}

// ListVisitsByLink 分页访问列表(按时间倒序)。
func (s *Store) ListVisitsByLink(ctx context.Context, linkID int64, page, pageSize int) ([]*Visit, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM visits WHERE link_id=$1`, linkID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT v.id, v.link_id, d.fqdn, v.user_agent, v.referer, v.created_at
		 FROM visits v JOIN domains d ON d.id = v.domain_id
		 WHERE v.link_id=$1 ORDER BY v.id DESC LIMIT $2 OFFSET $3`,
		linkID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*Visit
	for rows.Next() {
		var v Visit
		if err := rows.Scan(&v.ID, &v.LinkID, &v.Domain, &v.UserAgent, &v.Referer, &v.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &v)
	}
	return out, total, rows.Err()
}

// CleanupVisitsBefore 清理保留期前的访问记录。
func (s *Store) CleanupVisitsBefore(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM visits WHERE created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
