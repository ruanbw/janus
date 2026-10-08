// Package store 封装全部数据库访问,数据层使用 GORM(ORM)操作 Postgres。
// 黑盒测试以 HTTP API 为 seam,本包不暴露测试专用逻辑。
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

// IsUniqueViolation 判断错误是否为唯一约束冲突(如 (domain_id, code) 或邮箱)。
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type Store struct {
	db *gorm.DB
}

func New(gdb *gorm.DB) *Store { return &Store{db: gdb} }

type Tier struct {
	ID         int64  `json:"id" gorm:"primaryKey"`
	Name       string `json:"name"`
	MaxLinks   int    `json:"maxLinks" gorm:"column:max_links"`
	MaxDomains int    `json:"maxDomains" gorm:"column:max_domains"`
}

type Usage struct {
	Links      int `json:"links"`
	Domains    int `json:"domains"`
	MaxLinks   int `json:"maxLinks"`
	MaxDomains int `json:"maxDomains"`
}

type Tenant struct {
	ID              int64      `json:"id" gorm:"primaryKey"`
	Email           string     `json:"email"`
	PasswordHash    *string    `json:"-" gorm:"column:password_hash"`
	Slug            string     `json:"slug"`
	Status          string     `json:"status"`
	IsSuperAdmin    bool       `json:"isSuperAdmin" gorm:"column:is_super_admin"`
	TokenVersion    int64      `json:"-" gorm:"column:token_version"`
	TierID          int64      `json:"-" gorm:"column:tier_id"`
	Tier            Tier       `json:"tier" gorm:"foreignKey:TierID"`
	VerifiedAt      *time.Time `json:"-" gorm:"column:verified_at"`
	Custom404HTML   string     `json:"custom404Html" gorm:"column:custom_404_html"`
	Custom429HTML   string     `json:"custom429Html" gorm:"column:custom_429_html"`
	CreatedAt       time.Time  `json:"createdAt" gorm:"column:created_at"`
	DefaultDomain   string     `json:"defaultDomain" gorm:"-"`             // 平台默认域名,查询后填充
	FirstLoginSetup bool       `json:"firstLoginSetup,omitempty" gorm:"-"` // 超管首次登录(尚无密码)
	Usage           *Usage     `json:"usage,omitempty" gorm:"-"`
}

// loadTenantMeta 填充平台默认域名与 FirstLoginSetup(超管尚无密码时 true)。
func (s *Store) loadTenantMeta(ctx context.Context, t *Tenant) error {
	var fqdn string
	err := s.db.WithContext(ctx).Model(&Domain{}).
		Where("tenant_id = ? AND origin = 'platform'", t.ID).Order("id").Limit(1).
		Pluck("fqdn", &fqdn).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	t.DefaultDomain = fqdn
	t.FirstLoginSetup = t.needsFirstLoginSetup()
	return nil
}

func (s *Store) getTenant(ctx context.Context, q *gorm.DB) (*Tenant, error) {
	var t Tenant
	if err := q.Preload("Tier").First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.loadTenantMeta(ctx, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) GetFreeTier(ctx context.Context) (*Tier, error) {
	var tier Tier
	if err := s.db.WithContext(ctx).Where("name = 'free'").First(&tier).Error; err != nil {
		return nil, err
	}
	return &tier, nil
}

func (s *Store) GetTier(ctx context.Context, id int64) (*Tier, error) {
	var tier Tier
	if err := s.db.WithContext(ctx).First(&tier, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &tier, nil
}

func (s *Store) ListTiers(ctx context.Context) ([]Tier, error) {
	var out []Tier
	if err := s.db.WithContext(ctx).Order("id").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// CreateTenant 创建租户(默认免费档)。passwordHash 为空表示尚无密码(超管)。
func (s *Store) CreateTenant(ctx context.Context, email, passwordHash, slug string, isSuperAdmin bool) (*Tenant, error) {
	tier, err := s.GetFreeTier(ctx)
	if err != nil {
		return nil, err
	}
	t := Tenant{
		Email: email, Slug: slug, Status: "pending",
		IsSuperAdmin: isSuperAdmin, TierID: tier.ID,
		TokenVersion: 1,
	}
	if passwordHash != "" {
		t.PasswordHash = &passwordHash
	}
	if err := s.db.WithContext(ctx).Create(&t).Error; err != nil {
		return nil, err
	}
	return s.GetTenantByID(ctx, t.ID)
}

func (s *Store) GetTenantByID(ctx context.Context, id int64) (*Tenant, error) {
	return s.getTenant(ctx, s.db.WithContext(ctx).Where("id = ?", id))
}

func (s *Store) GetTenantByEmail(ctx context.Context, email string) (*Tenant, error) {
	return s.getTenant(ctx, s.db.WithContext(ctx).Where("email = ?", email))
}

func (s *Store) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	return s.getTenant(ctx, s.db.WithContext(ctx).Where("slug = ?", slug))
}

// TenantPasswordHash 返回租户密码哈希(NULL 表示尚无密码)。
func (s *Store) TenantPasswordHash(ctx context.Context, id int64) (*string, error) {
	var ns sql.NullString
	res := s.db.WithContext(ctx).Model(&Tenant{}).Select("password_hash").Where("id = ?", id).Scan(&ns)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	if !ns.Valid {
		return nil, nil
	}
	return &ns.String, nil
}

func (s *Store) SetTenantPassword(ctx context.Context, id int64, hash string) error {
	return s.db.WithContext(ctx).Model(&Tenant{}).Where("id = ?", id).Update("password_hash", hash).Error
}

// VerifyTenant 邮箱验证通过:仅把 pending 租户置 active 并记录 verified_at。
// 已封禁/已激活租户不因旧验证 token 被重新激活。
func (s *Store) VerifyTenant(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&Tenant{}).
		Where("id = ? AND status = 'pending'", id).
		Updates(map[string]any{"status": "active", "verified_at": gorm.Expr("COALESCE(verified_at, now())")}).Error
}

func (s *Store) SetTenantStatus(ctx context.Context, id int64, status string) error {
	return s.db.WithContext(ctx).Model(&Tenant{}).Where("id = ?", id).Update("status", status).Error
}

func (s *Store) SetTenantTier(ctx context.Context, id, tierID int64) error {
	return s.db.WithContext(ctx).Model(&Tenant{}).Where("id = ?", id).Update("tier_id", tierID).Error
}

// UpdateTenantErrorPages 更新租户自定义 404 与 429 错误页面。
func (s *Store) UpdateTenantErrorPages(ctx context.Context, tenantID int64, page404, page429 string) error {
	return s.db.WithContext(ctx).Model(&Tenant{}).Where("id = ?", tenantID).Updates(map[string]any{
		"custom_404_html": page404,
		"custom_429_html": page429,
	}).Error
}

// GetTenantErrorPages 获取租户自定义 404 与 429 错误页面。
func (s *Store) GetTenantErrorPages(ctx context.Context, tenantID int64) (page404, page429 string, err error) {
	var t Tenant
	if err := s.db.WithContext(ctx).Select("custom_404_html, custom_429_html").Where("id = ?", tenantID).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	return t.Custom404HTML, t.Custom429HTML, nil
}

// UpdateTenantAdmin 平台管理端更新租户状态与/或等级;两个字段在同一事务中生效,
// 避免其中一个写入失败时出现半更新状态。调用方须先完成权限与业务校验。
func (s *Store) UpdateTenantAdmin(ctx context.Context, id int64, status *string, tierID *int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if status != nil {
			updates := map[string]any{"status": *status}
			if *status == "banned" {
				updates["token_version"] = gorm.Expr("token_version + 1")
			}
			if err := tx.Model(&Tenant{}).Where("id = ?", id).Updates(updates).Error; err != nil {
				return err
			}
		}
		if tierID != nil {
			if err := tx.Model(&Tenant{}).Where("id = ?", id).Update("tier_id", *tierID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SetTenantSuperAdmin 标记租户为平台管理员。
func (s *Store) SetTenantSuperAdmin(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&Tenant{}).Where("id = ?", id).Update("is_super_admin", true).Error
}

// PromoteToSuperAdminResetCredentials 把**既有的非超管**租户提升为超管,并在**同一事务**内
// 作废它原有的全部凭据:清空 password_hash(强制走一次性 setup token 首登流程)、
// 自增 token_version(已签发 JWT 立即失效)、删除全部会话。
//
// 为什么必须原子:分步执行时若进程死在"已置 is_super_admin、尚未清密码"之间,
// 下次启动 bootstrap 看到 IsSuperAdmin=true 就不会再清 —— 账号原持有人(可能是
// 抢先用超管邮箱注册的攻击者)保留原密码直接成为超管,正是这里要堵的洞。
// 不改 status:封禁与否由调用方决定(封禁账号不得被复活)。
func (s *Store) PromoteToSuperAdminResetCredentials(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Tenant{}).Where("id = ?", id).Updates(map[string]any{
			"is_super_admin": true,
			"password_hash":  nil,
			"token_version":  gorm.Expr("token_version + 1"),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return tx.Where("tenant_id = ?", id).Delete(&Session{}).Error
	})
}

// ListTenants 平台管理:全部租户(按创建时间倒序)。
func (s *Store) ListTenants(ctx context.Context) ([]*Tenant, error) {
	var out []*Tenant
	if err := s.db.WithContext(ctx).Preload("Tier").Order("id DESC").Find(&out).Error; err != nil {
		return nil, err
	}
	// 一次查询所有平台默认域名,避免 N+1
	type defDomain struct {
		TenantID int64
		FQDN     string
	}
	var defs []defDomain
	if err := s.db.WithContext(ctx).Model(&Domain{}).
		Select("tenant_id, fqdn").Where("origin = 'platform'").Order("id").Scan(&defs).Error; err != nil {
		return nil, err
	}
	defMap := map[int64]string{}
	for _, d := range defs {
		if _, ok := defMap[d.TenantID]; !ok {
			defMap[d.TenantID] = d.FQDN
		}
	}
	for _, t := range out {
		t.DefaultDomain = defMap[t.ID]
		t.FirstLoginSetup = t.needsFirstLoginSetup()
	}
	return out, nil
}

// needsFirstLoginSetup 判断该租户是否处于"超管尚未设置密码"的引导态。
// 抽成方法是因为 loadTenantMeta 与 ListTenants 各写了一遍同样的判定;
// 两处分处一改就会让 /api/me 与 /api/admin/tenants 对同一租户给出矛盾的结论。
func (t *Tenant) needsFirstLoginSetup() bool {
	return t.IsSuperAdmin && (t.PasswordHash == nil || *t.PasswordHash == "")
}

// ---------- 会话 ----------

type Session struct {
	ID        int64     `gorm:"primaryKey"`
	TenantID  int64     `gorm:"column:tenant_id"`
	TokenHash string    `gorm:"column:token_hash"`
	CSRFToken string    `gorm:"column:csrf_token"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
}

// CreateSession 创建会话并返回会话记录。token 由调用方生成,此处仅存哈希。
func (s *Store) CreateSession(ctx context.Context, tenantID int64, tokenHash, csrfToken string, ttl time.Duration) (*Session, error) {
	sess := Session{
		TenantID: tenantID, TokenHash: tokenHash, CSRFToken: csrfToken,
		ExpiresAt: time.Now().Add(ttl),
	}
	if err := s.db.WithContext(ctx).Create(&sess).Error; err != nil {
		return nil, err
	}
	return &sess, nil
}

// GetSessionByTokenHash 返回未过期的会话。
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	var sess Session
	if err := s.db.WithContext(ctx).Where("token_hash = ? AND expires_at > now()", tokenHash).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sess, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	return s.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Delete(&Session{}).Error
}

// DeleteSessionsByTenant 删除租户的全部会话(密码重置后强制所有终端重新登录)。
func (s *Store) DeleteSessionsByTenant(ctx context.Context, tenantID int64) error {
	return s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(&Session{}).Error
}

// DeleteSessionsByTenantExcept 删除租户除指定会话外的全部会话(改密后保留当前终端)。
func (s *Store) DeleteSessionsByTenantExcept(ctx context.Context, tenantID int64, keepTokenHash string) error {
	return s.db.WithContext(ctx).
		Where("tenant_id = ? AND token_hash <> ?", tenantID, keepTokenHash).
		Delete(&Session{}).Error
}

// DeleteExpiredSessions 惰性清理过期会话。
func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	return s.db.WithContext(ctx).Where("expires_at <= now()").Delete(&Session{}).Error
}

// DeleteExpiredEmailTokens 惰性清理过期邮箱 token(验证/重置)。
func (s *Store) DeleteExpiredEmailTokens(ctx context.Context) error {
	return s.db.WithContext(ctx).Where("expires_at <= now()").Delete(&EmailToken{}).Error
}

// ---------- 邮箱 token(验证/重置) ----------

type EmailToken struct {
	ID        int64      `gorm:"primaryKey"`
	TenantID  int64      `gorm:"column:tenant_id"`
	TokenHash string     `gorm:"column:token_hash"`
	Kind      string     `gorm:"column:kind"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	UsedAt    *time.Time `gorm:"column:used_at"`
}

func (s *Store) CreateEmailToken(ctx context.Context, tenantID int64, tokenHash, kind string, ttl time.Duration) error {
	tok := EmailToken{TenantID: tenantID, TokenHash: tokenHash, Kind: kind, ExpiresAt: time.Now().Add(ttl)}
	return s.db.WithContext(ctx).Create(&tok).Error
}

// ConsumeEmailToken 校验并消费一个 token:有效(未过期、未使用、kind 匹配)返回租户 ID 并标记已用。
// 用单条原子 UPDATE ... WHERE used_at IS NULL RETURNING 保证并发消费只有一个成功;
// "不存在/已用/已过期" 统一返回 ErrNotFound(对外错误语义不变)。
func (s *Store) ConsumeEmailToken(ctx context.Context, tokenHash, kind string) (int64, error) {
	var tenantID int64
	res := s.db.WithContext(ctx).Raw(
		`UPDATE email_tokens SET used_at = now()
		 WHERE token_hash = ? AND kind = ? AND expires_at > now() AND used_at IS NULL
		 RETURNING tenant_id`, tokenHash, kind).Scan(&tenantID)
	if res.Error != nil {
		return 0, res.Error
	}
	// 零行:不存在/已用/已过期,统一 ErrNotFound(对外错误语义不变)
	if res.RowsAffected == 0 {
		return 0, ErrNotFound
	}
	return tenantID, nil
}

// ---------- 用量(配额) ----------

// Usage 返回租户配额用量:短链按"尚未物理删除"计数,域名按"尚未删除"的自有域名计数(平台默认不计)。
func (s *Store) Usage(ctx context.Context, tenantID int64) (*Usage, error) {
	t, err := s.GetTenantByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	// 计数口径与 WithQuotaInTx 内的配额判断共用同一份实现(quota.go)。
	return usageWithDB(ctx, s.db, t)
}
