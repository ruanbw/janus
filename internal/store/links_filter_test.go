package store

import (
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 本文件锁定短链列表过滤的 SQL 形状(不连库)。
//
// 为什么需要 DryRun:GORM 的 DryRun 只编译语句、不执行,于是"过滤条件究竟拼成了
// 什么"可以在没有 Postgres 的机器上被钉住。这里断言的不是 SQL 字符串的排版,
// 而是三件会静默出错的事:
//  1. 关键词必须同时匹配短码、目标 URL 与承载域名 —— 少一个 EXISTS,用户就搜不到
//     自己明明记得的短链,而界面只会平静地显示"未找到";
//  2. 关键词里的 `%` `_` `\` 必须被转义。不转义时搜 "50%" 会命中所有短链
//     (`%` 成了通配符),既费扫全表又给出错误结果;
//  3. 类型/状态与关键词是独立维度,参数必须各自绑定(不能字符串拼接)。

// dryRunDB 构造一个不会真正连库的 GORM 句柄。
func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		// 只用于解析,DisableAutomaticPing 保证不会真的去连 127.0.0.1:1
		DSN: "host=127.0.0.1 port=1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("构造 DryRun 句柄失败: %v", err)
	}
	return db.Session(&gorm.Session{DryRun: true})
}

// renderLinkListSQL 把过滤条件编译成最终 SQL 与参数。
func renderLinkListSQL(t *testing.T, f LinkListFilter) (string, []any) {
	t.Helper()
	tx := applyLinkListFilter(dryRunDB(t).Model(&Link{}).Where("tenant_id = ?", 9), f)
	var out []*Link
	if err := tx.Find(&out).Error; err != nil {
		t.Fatalf("DryRun 编译失败: %v", err)
	}
	return tx.Statement.SQL.String(), tx.Statement.Vars
}

// TestLikePatternEscapesWildcards 固化关键词的通配符转义。
func TestLikePatternEscapesWildcards(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"普通词原样包裹", "promo", "%promo%"},
		{"百分号不再当统配", "50%", `%50\%%`},
		{"下划线不再当单字符", "a_b", `%a\_b%`},
		{"反斜杠自身也要转义(否则 ESCAPE 吃掉下一个字符)", `a\b`, `%a\\b%`},
		{"组合", `50%_off\`, `%50\%\_off\\%`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := likePattern(tc.in); got != tc.want {
				t.Fatalf("likePattern(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestLinkListFilterKeywordMatchesCodeTargetAndDomain 固化关键词的三处匹配面。
func TestLinkListFilterKeywordMatchesCodeTargetAndDomain(t *testing.T) {
	sql, vars := renderLinkListSQL(t, LinkListFilter{Keyword: "promo"})

	for _, frag := range []string{
		"code ILIKE",
		"FROM link_targets",
		"lt.url ILIKE",
		"FROM link_domains",
		"d.fqdn ILIKE",
	} {
		if !strings.Contains(sql, frag) {
			t.Fatalf("关键词过滤缺少 %q(用户会搜不到短链而界面只说未找到):\n%s", frag, sql)
		}
	}
	// 不能额外过滤 link_deleted:它是 links.deleted_at 的镜像,过滤它会让回收站
	// 里按域名搜不到已删除的短链(而那正是回收站搜索的用途)
	if strings.Contains(sql, "NOT ld.link_deleted") {
		t.Fatalf("域名匹配不应排除 link_deleted:\n%s", sql)
	}
	// 三处匹配必须用同一份转义后的模式
	pattern := "%promo%"
	count := 0
	for _, v := range vars {
		if v == pattern {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("三处 LIKE 应共用同一个模式参数 %q, 实际命中 %d 次: vars=%v", pattern, count, vars)
	}
}

// TestLinkListFilterEscapesUserWildcards 固化"用户输入里的 % 不会被当成通配符"。
func TestLinkListFilterEscapesUserWildcards(t *testing.T) {
	_, vars := renderLinkListSQL(t, LinkListFilter{Keyword: "50%"})
	if len(vars) < 2 {
		t.Fatalf("参数不足: %v", vars)
	}
	if got := vars[1]; got != `%50\%%` {
		t.Fatalf("关键词参数应为转义后的 %q, 得到 %v", `%50\%%`, got)
	}
}

// TestLinkListFilterTypeAndStatus 固化两个精确维度的绑定方式(走参数,不拼字符串)。
func TestLinkListFilterTypeAndStatus(t *testing.T) {
	sql, vars := renderLinkListSQL(t, LinkListFilter{Type: "landing", Status: "enabled"})

	if !strings.Contains(sql, "link_type = $2") {
		t.Fatalf("缺少 link_type 等值过滤:\n%s", sql)
	}
	if !strings.Contains(sql, "status = $3") {
		t.Fatalf("缺少 status 等值过滤:\n%s", sql)
	}
	if len(vars) != 3 || vars[1] != "landing" || vars[2] != "enabled" {
		t.Fatalf("参数绑定错误: %v", vars)
	}
}

// TestLinkListFilterEmptyIsNoop 固化"零值过滤不改变查询"。
func TestLinkListFilterEmptyIsNoop(t *testing.T) {
	sql, _ := renderLinkListSQL(t, LinkListFilter{})
	if strings.Contains(sql, "ILIKE") || strings.Contains(sql, "link_type") || strings.Contains(sql, "status") {
		t.Fatalf("零值过滤不应追加任何条件:\n%s", sql)
	}
}
