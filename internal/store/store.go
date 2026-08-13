// Package store 封装全部数据库访问。
// 黑盒测试以 HTTP API 为 seam,本包不暴露测试专用逻辑。
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

// IsUniqueViolation 判断错误是否为唯一约束冲突(如 (domain_id, code) 或邮箱)。
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Tier struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	MaxLinks   int    `json:"maxLinks"`
	MaxDomains int    `json:"maxDomains"`
}

type Usage struct {
	Links      int `json:"links"`
	Domains    int `json:"domains"`
	MaxLinks   int `json:"maxLinks"`
	MaxDomains int `json:"maxDomains"`
}

type Tenant struct {
	ID              int64     `json:"id"`
	Email           string    `json:"email"`
	Slug            string    `json:"slug"`
	Status          string    `json:"status"`
	IsSuperAdmin    bool      `json:"isSuperAdmin"`
	CodeLength      int       `json:"codeLength"`
	Tier            Tier      `json:"tier"`
	DefaultDomain   string    `json:"defaultDomain"`
	CreatedAt       time.Time `json:"createdAt"`
	FirstLoginSetup bool      `json:"firstLoginSetup,omitempty"` // 超管首次登录(尚无密码)
	Usage           *Usage    `json:"usage,omitempty"`
}

type Domain struct {
	ID          int64      `json:"id"`
	TenantID    int64      `json:"-"`
	FQDN        string     `json:"fqdn"`
	Origin      string     `json:"origin"`
	Status      string     `json:"status"`
	CertStatus  string     `json:"certStatus"`
	ActivatedAt *time.Time `json:"activatedAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// RedirectStatus 跳转方式(契约枚举:"301" | "302";JSON 序列化为字符串)。
type RedirectStatus string

const (
	RedirectStatus301 RedirectStatus = "301"
	RedirectStatus302 RedirectStatus = "302"
)

type Link struct {
	ID             int64          `json:"id"`
	TenantID       int64          `json:"-"`
	Code           string         `json:"code"`
	TargetURL      string         `json:"targetUrl"`
	RedirectStatus RedirectStatus `json:"redirectStatus"`
	Status         string         `json:"status"`
	Domains        []string       `json:"domains"`
	Visits         int64          `json:"visits"`
	CreatedAt      time.Time      `json:"createdAt"`
}

type Visit struct {
	ID        int64     `json:"id"`
	LinkID    int64     `json:"linkId"`
	Domain    string    `json:"domain"`
	UserAgent string    `json:"userAgent"`
	Referer   string    `json:"referer"`
	CreatedAt time.Time `json:"createdAt"`
}

type APIKey struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	Key       string    `json:"key,omitempty"` // 明文仅在创建响应中出现一次
}

// ---------- 租户 ----------

const tenantColumns = ` t.id, t.email, t.slug, t.status, t.is_super_admin, t.code_length,
	tier.id, tier.name, tier.max_links, tier.max_domains, t.verified_at, t.created_at, t.password_hash,
	COALESCE((SELECT d.fqdn FROM domains d WHERE d.tenant_id = t.id AND d.origin='platform' ORDER BY d.id LIMIT 1), '')`

func scanTenant(row pgx.Row) (*Tenant, error) {
	var t Tenant
	var verifiedAt *time.Time
	var pwHash *string
	err := row.Scan(&t.ID, &t.Email, &t.Slug, &t.Status, &t.IsSuperAdmin, &t.CodeLength,
		&t.Tier.ID, &t.Tier.Name, &t.Tier.MaxLinks, &t.Tier.MaxDomains, &verifiedAt, &t.CreatedAt, &pwHash,
		&t.DefaultDomain)
	if err != nil {
		return nil, err
	}
	t.FirstLoginSetup = t.IsSuperAdmin && (pwHash == nil || *pwHash == "")
	return &t, nil
}

const tenantJoin = ` FROM tenants t JOIN tiers tier ON tier.id = t.tier_id`

func (s *Store) GetFreeTier(ctx context.Context) (*Tier, error) {
	var tier Tier
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, max_links, max_domains FROM tiers WHERE name='free'`,
	).Scan(&tier.ID, &tier.Name, &tier.MaxLinks, &tier.MaxDomains)
	if err != nil {
		return nil, err
	}
	return &tier, nil
}

func (s *Store) GetTier(ctx context.Context, id int64) (*Tier, error) {
	var tier Tier
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, max_links, max_domains FROM tiers WHERE id=$1`, id,
	).Scan(&tier.ID, &tier.Name, &tier.MaxLinks, &tier.MaxDomains)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &tier, nil
}

func (s *Store) ListTiers(ctx context.Context) ([]Tier, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, max_links, max_domains FROM tiers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tier
	for rows.Next() {
		var tier Tier
		if err := rows.Scan(&tier.ID, &tier.Name, &tier.MaxLinks, &tier.MaxDomains); err != nil {
			return nil, err
		}
		out = append(out, tier)
	}
	return out, rows.Err()
}

// CreateTenant 创建租户(默认免费档)。passwordHash 为空表示尚无密码(超管)。
func (s *Store) CreateTenant(ctx context.Context, email, passwordHash, slug string, isSuperAdmin bool) (*Tenant, error) {
	tier, err := s.GetFreeTier(ctx)
	if err != nil {
		return nil, err
	}
	var id int64
	err = s.pool.QueryRow(ctx,
		`INSERT INTO tenants (email, password_hash, tier_id, status, slug, is_super_admin)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		email, nullableStr(passwordHash), tier.ID, "pending", slug, isSuperAdmin,
	).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetTenantByID(ctx, id)
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) GetTenantByID(ctx context.Context, id int64) (*Tenant, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT`+tenantColumns+tenantJoin+` WHERE t.id=$1`, id)
	t, err := scanTenant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (s *Store) GetTenantByEmail(ctx context.Context, email string) (*Tenant, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT`+tenantColumns+tenantJoin+` WHERE t.email=$1`, email)
	t, err := scanTenant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (s *Store) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT`+tenantColumns+tenantJoin+` WHERE t.slug=$1`, slug)
	t, err := scanTenant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

// TenantPasswordHash 返回租户密码哈希(NULL 表示尚无密码)。
func (s *Store) TenantPasswordHash(ctx context.Context, id int64) (*string, error) {
	var h *string
	err := s.pool.QueryRow(ctx, `SELECT password_hash FROM tenants WHERE id=$1`, id).Scan(&h)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (s *Store) SetTenantPassword(ctx context.Context, id int64, hash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE tenants SET password_hash=$1 WHERE id=$2`, hash, id)
	return err
}

// VerifyTenant 邮箱验证通过:仅把 pending 租户置 active 并记录 verified_at。
// 已封禁/已激活租户不因旧验证 token 被重新激活。
func (s *Store) VerifyTenant(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE tenants SET status='active', verified_at=COALESCE(verified_at, now())
		 WHERE id=$1 AND status='pending'`, id)
	return err
}

func (s *Store) SetTenantStatus(ctx context.Context, id int64, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE tenants SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (s *Store) SetTenantTier(ctx context.Context, id, tierID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE tenants SET tier_id=$1 WHERE id=$2`, tierID, id)
	return err
}

func (s *Store) SetTenantCodeLength(ctx context.Context, id int64, length int) error {
	_, err := s.pool.Exec(ctx, `UPDATE tenants SET code_length=$1 WHERE id=$2`, length, id)
	return err
}

// SetTenantSuperAdmin 标记租户为平台管理员。
func (s *Store) SetTenantSuperAdmin(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE tenants SET is_super_admin=true WHERE id=$1`, id)
	return err
}

// ListTenants 平台管理:全部租户(按创建时间倒序)。
func (s *Store) ListTenants(ctx context.Context) ([]*Tenant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT`+tenantColumns+tenantJoin+` ORDER BY t.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Tenant
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---------- 会话 ----------

type Session struct {
	ID        int64
	TenantID  int64
	TokenHash string
	CSRFToken string
	ExpiresAt time.Time
}

// CreateSession 创建会话并返回会话记录。token 由调用方生成,此处仅存哈希。
func (s *Store) CreateSession(ctx context.Context, tenantID int64, tokenHash, csrfToken string, ttl time.Duration) (*Session, error) {
	expires := time.Now().Add(ttl)
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO sessions (tenant_id, token_hash, csrf_token, expires_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		tenantID, tokenHash, csrfToken, expires,
	).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &Session{ID: id, TenantID: tenantID, TokenHash: tokenHash, CSRFToken: csrfToken, ExpiresAt: expires}, nil
}

// GetSessionByTokenHash 返回未过期的会话。
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	var sess Session
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, token_hash, csrf_token, expires_at FROM sessions
		 WHERE token_hash=$1 AND expires_at > now()`, tokenHash,
	).Scan(&sess.ID, &sess.TenantID, &sess.TokenHash, &sess.CSRFToken, &sess.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sess, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash)
	return err
}

// DeleteExpiredSessions 惰性清理过期会话。
func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	return err
}

// DeleteExpiredEmailTokens 惰性清理过期邮箱 token(验证/重置)。
func (s *Store) DeleteExpiredEmailTokens(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM email_tokens WHERE expires_at <= now()`)
	return err
}

// ---------- 邮箱 token(验证/重置) ----------

func (s *Store) CreateEmailToken(ctx context.Context, tenantID int64, tokenHash, kind string, ttl time.Duration) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO email_tokens (tenant_id, token_hash, kind, expires_at) VALUES ($1,$2,$3,$4)`,
		tenantID, tokenHash, kind, time.Now().Add(ttl))
	return err
}

// ConsumeEmailToken 校验并消费一个 token:有效(未过期、未使用、kind 匹配)返回租户 ID 并标记已用。
// 用单条原子 UPDATE ... WHERE used_at IS NULL RETURNING 保证并发消费只有一个成功;
// "不存在/已用/已过期" 统一返回 ErrNotFound(对外错误语义不变)。
func (s *Store) ConsumeEmailToken(ctx context.Context, tokenHash, kind string) (int64, error) {
	var tenantID int64
	err := s.pool.QueryRow(ctx,
		`UPDATE email_tokens SET used_at=now()
		 WHERE token_hash=$1 AND kind=$2 AND expires_at > now() AND used_at IS NULL
		 RETURNING tenant_id`, tokenHash, kind,
	).Scan(&tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return tenantID, nil
}

// ---------- 用量(配额) ----------

// Usage 返回租户配额用量:短链按"尚未物理删除"计数,域名按"尚未删除"的自有域名计数(平台默认不计)。
func (s *Store) Usage(ctx context.Context, tenantID int64) (*Usage, error) {
	u := &Usage{}
	err := s.pool.QueryRow(ctx,
		`SELECT
			(SELECT count(*) FROM links WHERE tenant_id=$1),
			(SELECT count(*) FROM domains WHERE tenant_id=$1 AND origin='self')`,
		tenantID,
	).Scan(&u.Links, &u.Domains)
	if err != nil {
		return nil, err
	}
	t, err := s.GetTenantByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	u.MaxLinks = t.Tier.MaxLinks
	u.MaxDomains = t.Tier.MaxDomains
	return u, nil
}
