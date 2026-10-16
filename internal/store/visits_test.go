package store

// InsertVisit 入口校验与截断的测试:连真实 Postgres,验证"客户端可控的超长字段"
// 不会原样落库、非法枚举不会以 CHECK 违约的形式打到热路径。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// newVisitFixture 建一条挂在自有 active 域名下的短链,返回 (linkID, domainID)。
func newVisitFixture(t *testing.T, s *Store, email, code string) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	tenantID := newTenant(t, s, email)
	d, err := s.CreateDomain(ctx, tenantID, code+".visit.test", "self", "")
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if err := s.SetDomainActive(ctx, d.ID); err != nil {
		t.Fatalf("activate domain: %v", err)
	}
	l, err := s.CreateLink(ctx, tenantID, code, []string{"https://example.com/" + code},
		RedirectStatus302, LinkTypeRedirect, LandingSourceURL, "", []int64{d.ID})
	if err != nil {
		t.Fatalf("create link: %v", err)
	}
	return l.ID, d.ID
}

// TestInsertVisitTruncatesClientControlledFields 固化明细入库的字段上限。
//
// 契约:UA / Referer / TargetURL / Lang / Country 一律按 **rune** 截断到
// 512 / 2048 / 2048 / 64 / 2;超长输入不得原样落库,也不得切出非法 UTF-8。
//
// 为什么在 store 入口做:这几个字段里 UA/Referer 是任意长度的客户端可控请求头,
// 而 visits 按条保留 90 天 —— 一行被单个超长头撑爆会同时放大存储与聚合扫描成本。
// 按 rune 而不是按 byte 截断:按字节切会把多字节字符切成半个(U+FFFD);
// 历史审查里已经出现过一次"语言标签按字节截断切坏 UTF-8"。
func TestInsertVisitTruncatesClientControlledFields(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	linkID, domainID := newVisitFixture(t, s, "visit-truncate@test.io", "trunc1")

	// Given 一段远超上限、且是多字节字符的客户端可控输入
	longUA := strings.Repeat("汉", 1000)
	longReferer := strings.Repeat("参", 3000)
	longTarget := strings.Repeat("目", 3000)
	longLang := strings.Repeat("l", 100)
	country := "USA" // 超过 2 字符的国家码

	// When 写入明细
	if err := s.InsertVisit(ctx, VisitRecord{
		LinkID: linkID, DomainID: domainID,
		UserAgent: longUA, Referer: longReferer, TargetURL: longTarget,
		Lang: longLang, Country: country,
		Action: VisitActionRedirect, Outcome: VisitOutcomeSuccess,
	}); err != nil {
		t.Fatalf("insert visit: %v", err)
	}

	// Then 落库值被按 rune 截断到上限,且仍是合法 UTF-8
	var got struct {
		UserAgent string
		Referer   string
		TargetURL string
		Lang      string
		Country   string
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT user_agent, referer, target_url, lang, country
		  FROM visits WHERE link_id = ? ORDER BY id DESC LIMIT 1`, linkID).Scan(&got).Error; err != nil {
		t.Fatalf("read visit: %v", err)
	}
	for _, tc := range []struct {
		name     string
		got      string
		wantRune int
	}{
		{"user_agent", got.UserAgent, maxVisitUserAgentLen},
		{"referer", got.Referer, maxVisitRefererLen},
		{"target_url", got.TargetURL, maxVisitTargetURLLen},
		{"lang", got.Lang, maxVisitLangLen},
		{"country", got.Country, maxVisitCountryLen},
	} {
		if n := utf8.RuneCountInString(tc.got); n != tc.wantRune {
			t.Errorf("%s 落库 rune 数 = %d, want %d", tc.name, n, tc.wantRune)
		}
		if !utf8.ValidString(tc.got) {
			t.Errorf("%s 落库值不是合法 UTF-8(按字节截断切坏了多字节字符)", tc.name)
		}
	}
}

// TestInsertVisitRejectsInvalidEnums 固化写入侧枚举白名单。
//
// 契约:action / outcome / reason 不在枚举内时返回明确的错误,而不是把非法值交给
// 库里的 CHECK 约束 —— CHECK 违约是一条 SQLSTATE 23502/23514,与"数据库故障"
// 混在一起,排查时看不出是调用方写错了值;而且热路径上每一次都白白开一次事务再回滚。
func TestInsertVisitRejectsInvalidEnums(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	linkID, domainID := newVisitFixture(t, s, "visit-enum@test.io", "enum01")

	base := VisitRecord{LinkID: linkID, DomainID: domainID}

	for name, tc := range map[string]struct {
		mutate func(*VisitRecord)
		want   error
	}{
		"非法 action": {
			mutate: func(r *VisitRecord) { r.Action, r.Outcome = "explode", VisitOutcomeSuccess },
			want:   ErrInvalidAction,
		},
		"空 action": {
			mutate: func(r *VisitRecord) { r.Action, r.Outcome = "", VisitOutcomeSuccess },
			want:   ErrInvalidAction,
		},
		"非法 outcome": {
			mutate: func(r *VisitRecord) { r.Action, r.Outcome = VisitActionRedirect, "maybe" },
			want:   ErrInvalidOutcome,
		},
		"非法 reason": {
			mutate: func(r *VisitRecord) {
				r.Action, r.Outcome, r.Reason = VisitActionRedirect, VisitOutcomeFailed, "whatever"
			},
			want: ErrInvalidReason,
		},
	} {
		t.Run(name, func(t *testing.T) {
			rec := base
			tc.mutate(&rec)

			// When 写入一条枚举非法的明细
			err := s.InsertVisit(ctx, rec)

			// Then 返回对应错误(不是 23514 之类的库错误),且不落库
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			var n int64
			if cerr := s.db.WithContext(ctx).Raw(
				`SELECT count(*) FROM visits WHERE link_id = ?`, linkID).Scan(&n).Error; cerr != nil {
				t.Fatalf("count visits: %v", cerr)
			}
			if n != 0 {
				t.Fatalf("枚举非法却落库了 %d 行明细", n)
			}
		})
	}

	// 正向对照:合法枚举必须照常写入(含带合法 reason 的失败行)
	if err := s.InsertVisit(ctx, VisitRecord{
		LinkID: linkID, DomainID: domainID,
		Action: VisitActionRedirect, Outcome: VisitOutcomeFailed, Reason: VisitReasonNoTarget,
	}); err != nil {
		t.Fatalf("合法枚举写入失败: %v", err)
	}
}
