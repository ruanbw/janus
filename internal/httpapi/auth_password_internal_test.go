package httpapi

// 改密链路「旧密码校验」的错误分类单测。
//
// 为什么用单测:要区分的"数据库故障 → 500"分支要求"已认证会话 + 随后查询失败",
// 而认证本身要走同一张表 —— 黑盒无法只让 TenantPasswordHash 失败。这里的分类
// 逻辑是纯函数,直接断言即可。

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/ruanbw/janus/internal/store"
)

func hashPtr(t *testing.T, pw string) *string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	s := string(h)
	return &s
}

func TestCheckOldPasswordClassification(t *testing.T) {
	cases := []struct {
		name      string
		pwHash    *string
		err       error
		old       string
		wantOK    bool
		wantDBErr bool
	}{
		{"查询故障 → 500 语义", nil, errors.New("connection refused"), "whatever", false, true},
		{"租户行缺失 → 500 语义", nil, store.ErrNotFound, "whatever", false, true},
		{"无密码哈希 → 旧密码错误(400)", nil, nil, "whatever", false, false},
		{"旧密码不匹配 → 旧密码错误(400)", hashPtr(t, "correct-horse"), nil, "wrong", false, false},
		{"旧密码匹配 → 通过", hashPtr(t, "correct-horse"), nil, "correct-horse", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, dbErr := checkOldPassword(tc.pwHash, tc.err, tc.old)
			if ok != tc.wantOK || dbErr != tc.wantDBErr {
				t.Fatalf("checkOldPassword = (ok=%v, dbErr=%v), want (%v, %v)",
					ok, dbErr, tc.wantOK, tc.wantDBErr)
			}
		})
	}
}
