// Package bootstrap 承载部署期初始化逻辑(超管账号)。
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"janus/internal/domain"
	"janus/internal/store"
)

const (
	// SuperadminUsername 是引导创建的超管账号名(冻结契约:用户名固定为 superadmin)。
	SuperadminUsername = "superadmin"

	// superadminSeedPassword 是 bootstrap 直写的超管初始密码。
	//
	// 它短于常规密码下界(8),由 bootstrap 豁免长度校验 —— 它不是长期凭据:该租户的
	// must_change_password 被置为 true,首登必须改密后才放行其它受保护端点。
	// 之所以要有一个固定种子:超管是全平台唯一的自救入口,如果初始密码需要人工生成再
	// 传进容器,部署链路就多一个必须先完成才能登录的前置步骤。
	superadminSeedPassword = "123456"
)

// Superadmin 确保存在超管租户:
// username=superadmin、is_super_admin=true、status=active、密码 bcrypt(种子口令)、
// must_change_password=true(首登必须改密)。
//
// 幂等:已存在且是超管并且已有密码 → 不覆盖密码、不重置 must_change_password
// (否则运维每重启一次就把超管改回种子口令);已存在但不是超管 → 硬失败(绝不提权,
// 该 username 已被别人的账号占用)。
func Superadmin(ctx context.Context, st *store.Store) error {
	t, err := st.GetTenantByUsername(ctx, SuperadminUsername)
	if err == nil {
		return convergeOwnTenant(ctx, st, t)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(superadminSeedPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("bootstrap: 生成超管种子密码哈希失败: %w", err)
	}
	// 生成唯一 slug(超管专用,带随机后缀降低冲突概率)
	slug := superadminSlug(SuperadminUsername)
	created := false
	for i := 0; i < 5; i++ {
		suffix := domain.GenerateCode(4)
		_, err = st.CreateTenant(ctx, SuperadminUsername, string(hash), slug+"-"+suffix, true, true)
		if err == nil {
			created = true
			break
		}
		if !store.IsUniqueViolation(err) {
			return err
		}
		// 可能是 slug 冲突,也可能是另一个副本抢先建好了同 username 租户:
		// 先按 username 查一遍,查到就走「已存在」分支,不再把 username 唯一冲突
		// 当成 slug 冲突重试(重试 5 次全是 username 冲突,最终误以为失败)。
		if _, gerr := st.GetTenantByUsername(ctx, SuperadminUsername); gerr == nil {
			break
		}
	}
	if err != nil && !created {
		if _, gerr := st.GetTenantByUsername(ctx, SuperadminUsername); gerr != nil {
			return err
		}
	}
	// 超管无平台默认域名需求;直接置 active(可登录,首登强制改密)。
	// 即使是另一个副本创建的,这一步也必须做,保证 status=active 收敛。
	nt, err := st.GetTenantByUsername(ctx, SuperadminUsername)
	if err != nil {
		return err
	}
	return convergeOwnTenant(ctx, st, nt)
}

// superadminPlan 描述 bootstrap 对已存在的超管租户应做的收敛动作。
//
// 抽成纯函数(planSuperadminFields)是为了能不起数据库单测幂等口径:这段判定原先只
// 写在连库流程里,一旦被改成「每次启动都覆盖密码」,除了读代码没有任何回归手段。
type superadminPlan struct {
	// refuse 表示该租户不是超管:硬失败,绝不提权,也不产生任何副作用。
	refuse bool
	// activate 表示 status 不是 active:收敛为 active(进程可能死在创建与激活之间)。
	activate bool
	// seedPassword 表示尚无密码:直写种子口令并要求首登改密。
	seedPassword bool
}

// planSuperadminFields 依据租户当前字段决定收敛动作(纯函数,见 superadminPlan)。
func planSuperadminFields(isSuperAdmin bool, status string, hasPassword bool) superadminPlan {
	if !isSuperAdmin {
		return superadminPlan{refuse: true}
	}
	return superadminPlan{
		activate:     status != "active",
		seedPassword: !hasPassword,
	}
}

// convergeOwnTenant 收敛 bootstrap 自己创建的超管租户:标志位、状态与密码都要确保就绪。
//
// IsSuperAdmin=false 的行不是 bootstrap 创建的:该 username 被别人的账号占用(如公开注册
// 抢注了 superadmin)。此时必须拒绝,且不产生任何副作用——提权等于把平台交给抢注者,
// 顺手把 status 改成 active 也是在替别人的账号激活。
func convergeOwnTenant(ctx context.Context, st *store.Store, t *store.Tenant) error {
	hasPassword := t.PasswordHash != nil && *t.PasswordHash != ""
	plan := planSuperadminFields(t.IsSuperAdmin, t.Status, hasPassword)
	if plan.refuse {
		return fmt.Errorf("superadmin username %q is already taken by a non-superadmin tenant, refusing to promote", t.Username)
	}
	if plan.seedPassword {
		hash, err := bcrypt.GenerateFromPassword([]byte(superadminSeedPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("bootstrap: 生成超管种子密码哈希失败: %w", err)
		}
		if err := st.SetTenantPasswordAndRequireChange(ctx, t.ID, string(hash)); err != nil {
			return err
		}
	}
	if plan.activate {
		return st.SetTenantStatus(ctx, t.ID, "active")
	}
	return nil
}

// superadminSlug 由超管 username 派生 slug:[a-z0-9] 原样保留,其余字符折叠成单个连字符,
// 去掉首尾连字符。username 本身已按 [a-z0-9_-] 校验,这里的折叠只为去掉分隔符。
func superadminSlug(username string) string {
	var sb strings.Builder
	for _, c := range username {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			sb.WriteRune(c)
		case sb.Len() > 0:
			sb.WriteByte('-')
		}
	}
	// 归一化:折叠连续连字符、去掉首尾连字符,防止生成 "a--b"/"a-" 等不合法或不美观的 slug。
	// 说明:CreateTenant 自身不做 slug 校验(校验在 API 层),超管 slug 由内部生成,此处保证自身合法。
	slug := strings.Trim(sb.String(), "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	if slug == "" {
		slug = "sa"
	}
	return slug
}
