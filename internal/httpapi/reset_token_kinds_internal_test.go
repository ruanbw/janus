package httpapi

// 重置密码端点的「token 用途」契约。
//
// 背景:超管首登的 setup token 复用重置邮件模板(issueSetupToken → SendResetEmail),
// 邮件正文里给出的就是 /reset-password?token=... 链接。但端点只消费 kind=reset,
// 于是管理员点开邮件里的链接必然拿到 400「无效、已使用或过期的重置 token」——
// 首登只剩「在登录页手工填校验码」一条路,邮件链接始终是死链。
//
// 契约:
//  1. reset 与 setup 都可以用来设置密码:两者都证明「能读到这个邮箱」,都是一次性 + 限期;
//  2. verify 不可以:验证邮箱与设置密码是两件事,不接受跨用途消费;
//  3. 先试 reset、再试 setup:普通用户的重置不该先打 setup 分支;
//  4. 全部用途都没命中时返回 ErrNotFound(调用方回 400),绝不把零值 tenantID 当成功;
//  5. 命中即停:成功后不再尝试后续用途(不做无谓的第二次库往返)。

import (
	"errors"
	"testing"

	"janus/internal/store"
)

func TestResetPasswordConsumableTokenKinds(t *testing.T) {
	// Given 端点声明的可消费用途
	kinds := resetConsumableKinds()

	// Then 恰好两种,且 reset 先于 setup
	if len(kinds) != 2 {
		t.Fatalf("可消费用途应恰好为 2 种,当前为 %v", kinds)
	}
	if kinds[0] != "reset" {
		t.Fatalf("首个尝试的用途应为 reset(普通重置的主路径),当前为 %q", kinds[0])
	}
	if kinds[1] != emailTokenPurposeSetup {
		t.Fatalf("第二个尝试的用途应为 setup(超管首登),当前为 %q", kinds[1])
	}

	// And verify 不在其中:验证邮箱的 token 不能拿来改密
	for _, k := range kinds {
		if k == "verify" {
			t.Fatal("verify 用途的 token 不得用于设置密码")
		}
	}
}

func TestConsumeTokenKindsFallsBackToSetup(t *testing.T) {
	// Given 库里只有一枚 setup 用途的 token(reset 查不到)
	var tried []string
	consume := func(kind string) (int64, error) {
		tried = append(tried, kind)
		if kind == emailTokenPurposeSetup {
			return 42, nil
		}
		return 0, store.ErrNotFound
	}

	// When 走重置流程
	id, err := consumeTokenKinds(resetConsumableKinds(), consume)

	// Then 命中 setup token 对应的租户,且尝试顺序是 reset → setup
	if err != nil {
		t.Fatalf("setup token 应被接受,却返回错误: %v", err)
	}
	if id != 42 {
		t.Fatalf("返回的租户 ID = %d, want 42", id)
	}
	if len(tried) != 2 || tried[0] != "reset" || tried[1] != emailTokenPurposeSetup {
		t.Fatalf("尝试顺序 = %v, want [reset setup]", tried)
	}
}

func TestConsumeTokenKindsStopsAtFirstHit(t *testing.T) {
	// Given reset token 有效
	var tried []string
	consume := func(kind string) (int64, error) {
		tried = append(tried, kind)
		return 7, nil
	}

	// When 走重置流程
	if _, err := consumeTokenKinds(resetConsumableKinds(), consume); err != nil {
		t.Fatalf("有效 reset token 不应报错: %v", err)
	}

	// Then 只尝试了一次(setup 分支不产生额外的库调用)
	if len(tried) != 1 {
		t.Fatalf("命中后仍继续尝试了 %v,应当在第一个命中处停止", tried)
	}
}

func TestConsumeTokenKindsFailsClosed(t *testing.T) {
	// Given 所有用途都查不到(无效 / 已用 / 过期 / 用途不匹配)
	errDB := errors.New("db down")
	cases := []struct {
		name string
		ret  error
	}{
		{"not found", store.ErrNotFound},
		{"db error", errDB},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			consume := func(kind string) (int64, error) { return 0, tc.ret }

			// When
			id, err := consumeTokenKinds(resetConsumableKinds(), consume)

			// Then 必须返回错误,且不得把零值租户 ID 当成功传出去
			if err == nil {
				t.Fatal("没有任何用途命中时必须返回错误")
			}
			if !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("对外错误应统一为 ErrNotFound(端点据此回 400),当前为 %v", err)
			}
			if id != 0 {
				t.Fatalf("失败时租户 ID 应为零值,当前为 %d", id)
			}
		})
	}
}
