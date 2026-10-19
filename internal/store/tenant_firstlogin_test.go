package store

import "testing"

// 超管首登态(FirstLoginSetup)的判定口径。
//
// 为什么钉它:首登有两条完成路径 —— 在登录页提交 setup token 后调
// /api/auth/change-password,或直接点邮件链接走 /api/auth/reset-password
// (重置端点接受 setup 用途的 token)。两条路径都必须让「首登态」自动消失,
// 而它们改的是同一个 password_hash。
//
// 只要判定口径是「超管 + 无密码哈希」而不是某个独立的布尔列,两条路径就天然一致;
// 一旦有人把它改成读标志位,reset 那一条就会漏更新,首登引导页会反复出现。
func TestNeedsFirstLoginSetupFollowsPasswordHash(t *testing.T) {
	// Given 超管且尚无密码
	sa := &Tenant{IsSuperAdmin: true}
	if !sa.needsFirstLoginSetup() {
		t.Fatal("超管且无密码哈希时应处于首登态")
	}

	// When 通过任一入口设好了密码
	hash := "$2a$10$abcdefghijklmnopqrstuv"
	sa.PasswordHash = &hash

	// Then 首登态消失
	if sa.needsFirstLoginSetup() {
		t.Fatal("已有密码哈希时不应再自称首登态(否则首登引导页会反复出现)")
	}

	// And 空串等同于「没有密码」:其他写入路径留下空哈希时也不能蒙混过关
	empty := ""
	sa.PasswordHash = &empty
	if !sa.needsFirstLoginSetup() {
		t.Fatal("空串密码哈希应按未设置密码处理")
	}

	// And 普通租户即使没有密码也不算首登(它走注册 + 邮箱验证流程)
	plain := &Tenant{IsSuperAdmin: false}
	if plain.needsFirstLoginSetup() {
		t.Fatal("非超管租户不应进入首登引导态")
	}
}
