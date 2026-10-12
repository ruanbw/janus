package bootstrap_test

// bootstrap 包的黑盒测试:超管初始化不得把「已存在的、非由 bootstrap 创建的」账号提权为超管。
//
// seam 是包级公开函数 bootstrap.Superadmin:cmd/janus 与 pkg/janus 在启动时按环境变量
// JANUS_SUPERADMIN_EMAIL 调用它。也就是说,谁抢先注册了配置里那个可猜的邮箱字符串,
// 谁就决定了这一行 —— 所以这里必须钉死:该函数只收敛它自己创建的租户,不碰别人的账号。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"janus/internal/bootstrap"
	"janus/internal/domain"
	"janus/internal/store"
	"janus/internal/testutil"
)

const superadminEmail = "admin@janus.test"

// registerTenant 走公开注册接口建一个普通租户(有密码、is_super_admin=false、status=pending)。
// 用真实 HTTP 而不是直接写库:抢注超管邮箱的人走的就是这条路。
func registerTenant(t *testing.T, env *testutil.Env, email, slug string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"email": email, "password": "password123", "slug": slug,
	})
	if err != nil {
		t.Fatalf("marshal register body: %v", err)
	}
	resp, err := http.Post(env.Server.URL+"/api/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("register request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

// TestSuperadminDoesNotPromoteExistingNormalTenant 已存在的普通租户不得被提权为超管。
//
// Given 攻击者用可猜的超管邮箱(admin@janus.test)通过公开注册接口注册了普通账号
// (is_super_admin=false、status=pending、有自己的密码);
// When 平台重启,按配置调用 bootstrap.Superadmin 处理同一个邮箱;
// Then 返回错误,且该行的 is_super_admin 仍为 false、status 仍停在 pending(未被激活)。
//
// 修复前:函数只看 !t.IsSuperAdmin 就 SetTenantSuperAdmin、只看 t.Status != "active"
// 就置 active,没有任何「这行是否由 bootstrap 创建」的判别 —— 抢注一个可猜邮箱即可平台接管。
func TestSuperadminDoesNotPromoteExistingNormalTenant(t *testing.T) {
	env := testutil.Setup(t)

	// Given:攻击者抢注(用他自己的 slug,不伪造超管 slug 形态)
	registerTenant(t, env, superadminEmail, "attacker")
	before, err := env.Store.GetTenantByEmail(testutil.Ctx(), superadminEmail)
	if err != nil {
		t.Fatalf("get tenant by email: %v", err)
	}
	if before.IsSuperAdmin || before.Status != "pending" || before.PasswordHash == nil {
		t.Fatalf("基线不成立: is_super_admin=%v status=%q password_hash=%v, want false/pending/非空",
			before.IsSuperAdmin, before.Status, before.PasswordHash)
	}

	// When
	err = bootstrap.Superadmin(context.Background(), env.Store, superadminEmail)

	// Then:必须返回错误,而不是「确保标志位」;且不能把 Not Found(行明明存在)当结论传出去
	if err == nil {
		t.Error("Superadmin 对已存在的普通租户返回 nil;抢注可猜邮箱即可在重启时被提权为超管")
	}
	if errors.Is(err, store.ErrNotFound) {
		t.Errorf("Superadmin 返回 ErrNotFound(%v), want 明确的「邮箱已被非超管租户占用」错误", err)
	}
	// Then:副作用必须为零 —— 不提权、不激活、密码不被清
	after, gerr := env.Store.GetTenantByEmail(testutil.Ctx(), superadminEmail)
	if gerr != nil {
		t.Fatalf("re-get tenant by email: %v", gerr)
	}
	if after.IsSuperAdmin {
		t.Error("普通租户被提权为平台超管(is_super_admin=true)")
	}
	if after.Status != "pending" {
		t.Errorf("status = %q, want %q(status 未被改成 active)", after.Status, "pending")
	}
	if after.PasswordHash == nil {
		t.Error("password_hash 被清空, want 保留原密码")
	}
}

// TestSuperadminRepairsOwnHalfCreatedTenant bootstrap 自己创建的半成品租户必须被修复。
//
// 这是上一条的回归保护。bootstrap.go 的注释明确说明这段恢复逻辑是有意为之:
// 进程可能死在 CreateTenant 与 SetTenantStatus 之间,重启应把它修复为 active,
// 而不是「因 IsSuperAdmin 已是 true 就当作已是超管直接返回」。修 (a) 不能破坏 (b)。
//
// Given 上一次启动留下的崩溃现场:行已存在且 is_super_admin=true、status=pending、
// password_hash=NULL(尚无密码 = 还没走首次登录引导),slug 形如
// superadminSlug(email) + "-" + 随机后缀(CreateTenant 调用点的实际形态);
// When Superadmin 再次被调用;
// Then 无错误返回,且该行收敛为 status=active、is_super_admin 仍为 true。
func TestSuperadminRepairsOwnHalfCreatedTenant(t *testing.T) {
	env := testutil.Setup(t)

	// Given:直接复刻崩溃现场的行 —— 等价于 CreateTenant(email,"",slug,true) 已成功、
	// SetTenantStatus 尚未执行。passwordHash 传空串即 store 层的「尚无密码」。
	slug := "admin-" + domain.GenerateCode(4)
	if _, err := env.Store.CreateTenant(testutil.Ctx(), superadminEmail, "", slug, true); err != nil {
		t.Fatalf("seed crashed bootstrap tenant: %v", err)
	}

	// When
	err := bootstrap.Superadmin(context.Background(), env.Store, superadminEmail)

	// Then
	if err != nil {
		t.Fatalf("Superadmin 未能修复自己创建的半成品租户: %v", err)
	}
	after, err := env.Store.GetTenantByEmail(testutil.Ctx(), superadminEmail)
	if err != nil {
		t.Fatalf("get tenant by email: %v", err)
	}
	if after.Status != "active" {
		t.Errorf("status = %q, want %q(死在两步之间,重启应修复而非拒绝)", after.Status, "active")
	}
	if !after.IsSuperAdmin {
		t.Error("is_super_admin = false, want true(修复不得降级既有超管)")
	}
}
