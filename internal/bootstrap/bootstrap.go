// Package bootstrap 承载部署期初始化逻辑(超管账号)。
package bootstrap

import (
	"context"
	"errors"
	"log"
	"strings"

	"janus/internal/domain"
	"janus/internal/store"
)

// tenantStore 是 bootstrap 用到的最小存储接口(*store.Store 实现),
// 抽出来是为了让提权/封禁分支能在无数据库的单元测试里覆盖。
type tenantStore interface {
	GetTenantByEmail(ctx context.Context, email string) (*store.Tenant, error)
	CreateTenant(ctx context.Context, email, passwordHash, slug string, isSuperAdmin bool) (*store.Tenant, error)
	SetTenantStatus(ctx context.Context, id int64, status string) error
	PromoteToSuperAdminResetCredentials(ctx context.Context, id int64) error
}

// Superadmin 按环境变量指定的邮箱初始化超管租户:
// 不存在则创建(is_super_admin、active、暂无密码);已存在则确保标志位。
// 首次登录引导设置密码(见 spec 决策 #4、票据 08)。
//
// 已存在且**尚不是超管**的账号被提升时,原有凭据一律作废(清空密码、自增
// token_version、删除会话),必须重新走 setup token 首登流程。原实现保留原密码:
// 任何人只要抢在运维配置 JANUS_SUPERADMIN_EMAIL 之前用该邮箱注册(注册不需要
// 能收信),重启后就带着自己设的密码直接成为超管 —— 整个平台被接管。
//
// 封禁(banned)状态永不被覆盖:bootstrap 只把 pending 收敛为 active,
// 不替运维"解封"一个被封禁的账号。
func Superadmin(ctx context.Context, st *store.Store, email string) error {
	return superadmin(ctx, st, email)
}

func superadmin(ctx context.Context, st tenantStore, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("empty superadmin email")
	}
	t, err := st.GetTenantByEmail(ctx, email)
	if err == nil {
		return ensureExisting(ctx, st, t)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	// 生成唯一 slug(超管专用,带随机后缀降低冲突概率)
	slug := superadminSlug(email)
	created := false
	for i := 0; i < 5; i++ {
		suffix := domain.GenerateCode(4)
		_, err = st.CreateTenant(ctx, email, "", slug+"-"+suffix, true)
		if err == nil {
			created = true
			break
		}
		if !store.IsUniqueViolation(err) {
			return err
		}
		// 可能是 slug 冲突,也可能是另一个副本抢先建好了同邮箱租户:
		// 先按邮箱查一遍,查到就走「已存在」分支,不再把邮箱唯一冲突
		// 当成 slug 冲突重试(重试 5 次全是邮箱冲突,最终误以为失败)。
		if _, gerr := st.GetTenantByEmail(ctx, email); gerr == nil {
			break
		}
	}
	if err != nil && !created {
		if _, gerr := st.GetTenantByEmail(ctx, email); gerr != nil {
			return err
		}
	}
	nt, err := st.GetTenantByEmail(ctx, email)
	if err != nil {
		return err
	}
	if !created {
		// 不是本进程建的(另一个副本,或恰好有人并发用该邮箱注册了普通租户):
		// 一律按「已存在」分支处理 —— 普通租户要先作废凭据再提权,封禁不复活。
		// 原实现在这里无条件置 active,并发注册的普通账号会被免验证激活。
		return ensureExisting(ctx, st, nt)
	}
	// 超管无平台默认域名需求;直接置 active(可登录引导设置密码)。
	return st.SetTenantStatus(ctx, nt.ID, "active")
}

// ensureExisting 处理「超管邮箱对应的租户已存在」:标志位与状态都要确保就绪——
// 进程可能死在 CreateTenant 与 SetTenantStatus 之间,此时租户存在但 status=pending,
// 重启应把它修复,而不是因 IsSuperAdmin 已是 true 就当作「已是超管」直接返回。
func ensureExisting(ctx context.Context, st tenantStore, t *store.Tenant) error {
	if !t.IsSuperAdmin {
		log.Printf("WARNING: JANUS_SUPERADMIN_EMAIL 对应的既有普通租户(id=%d)将被提升为超管;"+
			"其原有密码、会话与已签发 token 已全部作废,须通过发往该邮箱的一次性 setup token 重新设置密码", t.ID)
		if err := st.PromoteToSuperAdminResetCredentials(ctx, t.ID); err != nil {
			return err
		}
	}
	switch t.Status {
	case "active":
		return nil
	case "banned":
		log.Printf("WARNING: 超管邮箱对应的租户(id=%d)处于封禁状态,bootstrap 不会自动解封;如需启用请由运维显式处理", t.ID)
		return nil
	default:
		return st.SetTenantStatus(ctx, t.ID, "active")
	}
}

func superadminSlug(email string) string {
	at := email
	for i := 0; i < len(at); i++ {
		if at[i] == '@' {
			at = at[:i]
			break
		}
	}
	if len(at) > 20 {
		at = at[:20]
	}
	var sb strings.Builder
	for _, c := range at {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			sb.WriteRune(c)
		case c >= 'A' && c <= 'Z':
			sb.WriteRune(c + 32)
		case sb.Len() > 0 && c != '.' && c != '_':
			sb.WriteByte('-')
		}
	}
	slug := sb.String()
	// 归一化:折叠连续连字符、去掉首尾连字符,防止生成 "a--b"/"a-" 等不合法或不美观的 slug。
	// 说明:CreateTenant 自身不做 slug 校验(校验在 API 层),超管 slug 由内部生成,此处保证自身合法。
	slug = strings.Trim(slug, "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	if slug == "" {
		slug = "sa"
	}
	return slug
}
