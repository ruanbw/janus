package store

// 认领条件的纯逻辑单测(不连库):哪些被占用的域名可以凭 TXT 归属证明接管。

import (
	"testing"
	"time"
)

func TestIsDomainClaimable(t *testing.T) {
	now := time.Now()
	const claimant int64 = 2
	base := func(mut func(*Domain)) *Domain {
		d := &Domain{TenantID: 1, Origin: "self", Status: "pending"}
		if mut != nil {
			mut(d)
		}
		return d
	}
	cases := []struct {
		name string
		d    *Domain
		want bool
	}{
		{"他人 pending 占位", base(nil), true},
		{"他人 expired 占位", base(func(d *Domain) { d.Status = "expired" }), true},
		{"他人 failed 且从未证明归属", base(func(d *Domain) { d.Status = "failed" }), true},
		{"他人已证明归属(降级后的 failed)", base(func(d *Domain) { d.Status = "failed"; d.OwnershipVerifiedAt = &now }), false},
		{"他人 active", base(func(d *Domain) { d.Status = "active" }), false},
		{"他人 stopped(曾正常持有)", base(func(d *Domain) { d.Status = "stopped" }), false},
		{"平台默认域名", base(func(d *Domain) { d.Origin = "platform" }), false},
		{"自己的行", base(func(d *Domain) { d.TenantID = claimant }), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		if got := IsDomainClaimable(tc.d, claimant); got != tc.want {
			t.Errorf("%s: IsDomainClaimable = %v, want %v", tc.name, got, tc.want)
		}
	}
}
