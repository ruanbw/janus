package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// CreateAPIKey 创建 API Key(keyHash 为 SHA-256 哈希)。
func (s *Store) CreateAPIKey(ctx context.Context, tenantID int64, name, keyHash string) (*APIKey, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO api_keys (tenant_id, name, key_hash) VALUES ($1,$2,$3) RETURNING id`,
		tenantID, name, keyHash).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetAPIKey(ctx, tenantID, id)
}

func (s *Store) GetAPIKey(ctx context.Context, tenantID, id int64) (*APIKey, error) {
	var k APIKey
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, created_at FROM api_keys WHERE id=$1 AND tenant_id=$2 AND revoked_at IS NULL`,
		id, tenantID).Scan(&k.ID, &k.Name, &k.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &k, nil
}

// ListAPIKeys 列出未吊销的 API Key。
func (s *Store) ListAPIKeys(ctx context.Context, tenantID int64) ([]*APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, created_at FROM api_keys WHERE tenant_id=$1 AND revoked_at IS NULL ORDER BY id`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &k)
	}
	return out, rows.Err()
}

// GetTenantIDByAPIKeyHash 按 key 哈希查未吊销的 Key 所属租户。
func (s *Store) GetTenantIDByAPIKeyHash(ctx context.Context, keyHash string) (int64, error) {
	var tenantID int64
	err := s.pool.QueryRow(ctx,
		`SELECT tenant_id FROM api_keys WHERE key_hash=$1 AND revoked_at IS NULL`, keyHash).Scan(&tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return tenantID, nil
}

// RevokeAPIKey 吊销(软删除:置 revoked_at)。
func (s *Store) RevokeAPIKey(ctx context.Context, tenantID, id int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND tenant_id=$2 AND revoked_at IS NULL`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
