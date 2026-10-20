package store

import (
	"strings"
	"testing"
)

// 本文件锁定访问明细关键词过滤的 SQL 形状(不连库,同 links_filter_test.go)。
//
// 为什么值得钉住:访问明细页的搜索原本只在**当前页**的内存副本上做 ——
// 用户找一个 IP 时,如果它不在第 1 页就显示「无记录」。下推到 SQL 后
// 三个会静默出错的点:
//  1. 匹配面:IP / UA / 来源页 / 目标地址都要能搜到(少一个就是"明明有却搜不到");
//  2. 关键词里的 `%` `_` `\` 必须转义,否则搜 "50%" 会命中整条短链的全部明细;
//  3. count 与明细两段查询必须用**同一组**条件,否则 total 与列表条数对不上。

// renderVisitFilterSQL 把访问明细过滤条件编译成 count / 明细两段 SQL。
func renderVisitFilterSQL(t *testing.T, f VisitFilter) (countSQL string, listSQL string, countVars, listVars []any) {
	t.Helper()
	db := dryRunDB(t)

	countQ := db.Model(&Visit{}).Where("link_id = ?", 3)
	listQ := db.Table("visits v").Where("v.link_id = ?", 3)
	countQ, listQ = applyVisitFilter(countQ, listQ, f)

	var total int64
	if err := countQ.Count(&total).Error; err != nil {
		t.Fatalf("count DryRun 编译失败: %v", err)
	}
	var out []*Visit
	if err := listQ.Select("v.id").Joins("JOIN domains d ON d.id = v.domain_id").Find(&out).Error; err != nil {
		t.Fatalf("list DryRun 编译失败: %v", err)
	}
	return countQ.Statement.SQL.String(), listQ.Statement.SQL.String(),
		countQ.Statement.Vars, listQ.Statement.Vars
}

// TestVisitFilterKeywordMatchesAllSearchableColumns 固化关键词的四个匹配面。
func TestVisitFilterKeywordMatchesAllSearchableColumns(t *testing.T) {
	countSQL, listSQL, countVars, listVars := renderVisitFilterSQL(t, VisitFilter{Keyword: "203.0.113"})

	for _, frag := range []string{"ip ILIKE", "user_agent ILIKE", "referer ILIKE", "target_url ILIKE"} {
		if !strings.Contains(countSQL, frag) {
			t.Fatalf("count 查询缺少 %q:\n%s", frag, countSQL)
		}
		if !strings.Contains(listSQL, frag) {
			t.Fatalf("明细查询缺少 %q(与 count 条件不一致会让 total 与列表对不上):\n%s", frag, listSQL)
		}
	}

	// 两段查询各四个占位参数,且都必须是同一份模式
	pattern := "%203.0.113%"
	for name, vars := range map[string][]any{"count": countVars, "list": listVars} {
		n := 0
		for _, v := range vars {
			if v == pattern {
				n++
			}
		}
		if n != 4 {
			t.Fatalf("%s 查询应有 4 个关键词参数, 实际 %d 个: %v", name, n, vars)
		}
	}
}

// TestVisitFilterKeywordEscapesWildcards 固化用户输入里的通配符不被解释。
func TestVisitFilterKeywordEscapesWildcards(t *testing.T) {
	_, _, _, listVars := renderVisitFilterSQL(t, VisitFilter{Keyword: "a_b%"})
	want := `%a\_b\%%`
	found := false
	for _, v := range listVars {
		if v == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("关键词参数应为转义后的 %q, 得到 %v", want, listVars)
	}
}

// TestVisitFilterCombinesActionOutcomeKeyword 固化三个维度可以同时生效。
func TestVisitFilterCombinesActionOutcomeKeyword(t *testing.T) {
	countSQL, _, countVars, _ := renderVisitFilterSQL(t, VisitFilter{
		Action:  "redirect",
		Outcome: "failed",
		Keyword: "203.0.113",
	})

	for _, frag := range []string{"action = ", "outcome = ", "ip ILIKE"} {
		if !strings.Contains(countSQL, frag) {
			t.Fatalf("组合过滤缺少 %q:\n%s", frag, countSQL)
		}
	}
	if len(countVars) != 7 { // link_id + action + outcome + 4 个关键词
		t.Fatalf("参数个数应为 7, 得到 %d: %v", len(countVars), countVars)
	}
}

// TestVisitFilterEmptyKeywordIsNoop 固化"空关键词不改变查询"。
func TestVisitFilterEmptyKeywordIsNoop(t *testing.T) {
	countSQL, _, countVars, _ := renderVisitFilterSQL(t, VisitFilter{})
	if strings.Contains(countSQL, "ILIKE") {
		t.Fatalf("空过滤不应出现 ILIKE:\n%s", countSQL)
	}
	if len(countVars) != 1 {
		t.Fatalf("只有 link_id 一个参数, 得到 %v", countVars)
	}
}
