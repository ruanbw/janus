// Package bootstrap 承载部署期初始化逻辑(超管账号)。
package bootstrap

import (
	"context"
	"errors"
	"strings"

	"janus/internal/domain"
	"janus/internal/store"
)

// Superadmin 按环境变量指定的邮箱初始化超管租户:
// 不存在则创建(is_super_admin、active、暂无密码);已存在则确保标志位。
// 首次登录引导设置密码(见 spec 决策 #4、票据 08)。
func Superadmin(ctx context.Context, st *store.Store, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("empty superadmin email")
	}
	t, err := st.GetTenantByEmail(ctx, email)
	if err == nil {
		// 已存在:标志位与状态都要确保就绪——进程可能死在 CreateTenant 与
		// SetTenantStatus 之间,此时租户存在但 status=pending,重启应把它修复,
		// 而不是因 IsSuperAdmin 已是 true 就当作「已是超管」直接返回。
		if !t.IsSuperAdmin {
			if err := st.SetTenantSuperAdmin(ctx, t.ID); err != nil {
				return err
			}
		}
		if t.Status != "active" {
			return st.SetTenantStatus(ctx, t.ID, "active")
		}
		return nil
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
	// 超管无平台默认域名需求;直接置 active(可登录引导设置密码)。
	// 即使是另一个副本创建的,这一步也必须做,保证 status=active 收敛。
	nt, err := st.GetTenantByEmail(ctx, email)
	if err != nil {
		return err
	}
	return st.SetTenantStatus(ctx, nt.ID, "active")
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
