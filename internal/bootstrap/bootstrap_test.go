package bootstrap

import "testing"

// 超管 bootstrap 的幂等/密码策略。
//
// 这些判定原先只存在于要连库的流程里,DB 测试层移除后无法回归;抽成纯函数后
// 可以直接钉住三条契约:
//  1. 已存在且是超管、已有密码 → 什么都不做(不覆盖密码、不重置 must_change_password);
//  2. 已存在且是超管、没有密码 → 写种子口令并要求首登改密;
//  3. 已存在但不是超管 → 拒绝(绝不提权,也不产生任何副作用)。
func TestPlanSuperadmin(t *testing.T) {
	cases := []struct {
		name        string
		superAdmin  bool
		status      string
		hasPassword bool
		want        superadminPlan
	}{
		{
			name:       "active 超管且已有密码:完全 no-op",
			superAdmin: true, status: "active", hasPassword: true,
			want: superadminPlan{},
		},
		{
			name:       "超管进程中途死在激活前:只收敛 status",
			superAdmin: true, status: "pending", hasPassword: true,
			want: superadminPlan{activate: true},
		},
		{
			name:       "超管尚无密码:写种子口令并强制改密",
			superAdmin: true, status: "active", hasPassword: false,
			want: superadminPlan{seedPassword: true},
		},
		{
			name:       "超管既无密码又未激活:两步都做",
			superAdmin: true, status: "pending", hasPassword: false,
			want: superadminPlan{activate: true, seedPassword: true},
		},
		{
			name:       "已被非超管占用:拒绝提权",
			superAdmin: false, status: "active", hasPassword: true,
			want: superadminPlan{refuse: true},
		},
		{
			name:       "被非超管占用且无密码:仍然拒绝,不得顺手播种",
			superAdmin: false, status: "pending", hasPassword: false,
			want: superadminPlan{refuse: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := planSuperadminFields(tc.superAdmin, tc.status, tc.hasPassword)
			if got != tc.want {
				t.Fatalf("planSuperadminFields = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSuperadminUsernameIsFrozen(t *testing.T) {
	// 冻结契约:超管账号名固定为 superadmin。改这里等于改前端与运维文档的登录方式。
	if SuperadminUsername != "superadmin" {
		t.Fatalf("SuperadminUsername = %q, want \"superadmin\"", SuperadminUsername)
	}
}

// 种子口令短于常规密码下界(8):bootstrap 直写、豁免长度校验,代价是必须强制改密。
func TestSuperadminSeedPasswordRequiresChange(t *testing.T) {
	if len(superadminSeedPassword) >= 8 {
		t.Fatalf("种子口令 %q 长度 %d,若不短于常规下界(8)则无需强制改密豁免", superadminSeedPassword, len(superadminSeedPassword))
	}
	if plan := planSuperadminFields(true, "active", false); !plan.seedPassword {
		t.Fatal("无密码的超管必须被写入种子口令")
	}
}

func TestSuperadminSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"superadmin", "superadmin"},
		{"a_b", "a-b"},
		{"a--b", "a-b"},
		{"a-", "a"},
		{"---", "sa"},
		{"", "sa"},
	}
	for _, c := range cases {
		if got := superadminSlug(c.in); got != c.want {
			t.Errorf("superadminSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
