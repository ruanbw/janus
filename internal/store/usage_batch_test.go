package store

import (
	"context"
	"testing"
)

// FillUsages 的批量口径必须与 usageWithDB 逐条算出的完全一致。
//
// 不一致的后果很隐蔽:管理端看到的配额与真正拦截写入时用的配额对不上,超管会以为
// 租户还有配额而用户被拒,或反之。所以这里对同一批租户两种算法逐字段比对。
func TestFillUsages_MatchesPerTenantUsage(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// 建三个租户,各自建不同数量的短链与 1 个自有域名
	counts := []int{0, 2, 5}
	var tenants []*Tenant
	for i, n := range counts {
		suffix := string(rune('a' + i))
		tn, err := s.CreateTenant(ctx, "bulk"+suffix+"@example.com", "hash", "bulk"+suffix, false)
		if err != nil {
			t.Fatalf("建租户 %d: %v", i, err)
		}
		tenants = append(tenants, tn)
		for j := 0; j < n; j++ {
			if _, err := s.CreateLink(ctx, tn.ID, "bulk"+suffix+string(rune('0'+j)),
				[]string{"https://example.com"}, RedirectStatus302, LinkTypeRedirect, LandingSourceURL, "", nil); err != nil {
				t.Fatalf("建短链: %v", err)
			}
		}
		if _, err := s.CreateDomain(ctx, tn.ID, "d"+suffix+".example.com", "self", ""); err != nil {
			t.Fatalf("建域名: %v", err)
		}
	}

	// 逐条口径(作为期望值)
	want := make([]*Usage, len(tenants))
	for i, tn := range tenants {
		u, err := s.Usage(ctx, tn.ID)
		if err != nil {
			t.Fatalf("Usage(%d): %v", tn.ID, err)
		}
		want[i] = u
	}

	// 批量口径
	if err := s.FillUsages(ctx, tenants); err != nil {
		t.Fatalf("FillUsages: %v", err)
	}

	for i, tn := range tenants {
		got := tn.Usage
		if got == nil {
			t.Fatalf("租户 %d 的 Usage 未被填充", tn.ID)
		}
		if got.Links != want[i].Links {
			t.Errorf("租户 %d: Links = %d, want %d", tn.ID, got.Links, want[i].Links)
		}
		if got.Domains != want[i].Domains {
			t.Errorf("租户 %d: Domains = %d, want %d", tn.ID, got.Domains, want[i].Domains)
		}
		if got.MaxLinks != want[i].MaxLinks || got.MaxDomains != want[i].MaxDomains {
			t.Errorf("租户 %d: 配额上限 = (%d,%d), want (%d,%d)",
				tn.ID, got.MaxLinks, got.MaxDomains, want[i].MaxLinks, want[i].MaxDomains)
		}
	}
}

func TestFillUsages_EmptyInput(t *testing.T) {
	s := setupStore(t)
	if err := s.FillUsages(context.Background(), nil); err != nil {
		t.Fatalf("空输入应直接返回 nil,got %v", err)
	}
}
