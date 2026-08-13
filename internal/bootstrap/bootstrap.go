// Package bootstrap 承载部署期初始化逻辑(超管账号)。
package bootstrap

import (
	"context"
	"errors"
	"strings"

	"cloak/internal/domain"
	"cloak/internal/store"
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
		if !t.IsSuperAdmin {
			return st.SetTenantSuperAdmin(ctx, t.ID)
		}
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	// 生成唯一 slug(超管专用,带随机后缀降低冲突概率)
	slug := superadminSlug(email)
	for i := 0; i < 5; i++ {
		suffix := domain.GenerateCode(4)
		_, err = st.CreateTenant(ctx, email, "", slug+"-"+suffix, true)
		if err == nil {
			break
		}
		if !store.IsUniqueViolation(err) {
			return err
		}
	}
	if err != nil {
		return err
	}
	// 超管无平台默认域名需求;直接置 active(可登录引导设置密码)
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
