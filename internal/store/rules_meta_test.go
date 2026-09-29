package store

// 本文件覆盖 03/04/05 三张工单在 store 层新增的能力:
//   - 短链侧的关联写入口 SetLinkRules(与规则侧的 SetRuleLinks 是同一份 rule_links 的两个方向);
//   - 「24h 命中」从 visits 读时聚合 CountRuleHits24h(表上没有任何命中计数器);
//   - 短链列表的适用规则元数据(Link.RuleCount / Link.RuleNames);
//   - 关联写入前的归属校验辅助(RulesByIDs / CountOwnedLiveLinks)。
// 沿用 setupStore 的测试库互斥约定,不引入 testutil(不依赖 httpapi)。

import (
	"context"
	"testing"
	"time"
)

// newDomain 建一个 active 域名并返回其 id(构造带域名/明细的测试数据用)。
func newDomainID(t *testing.T, s *Store, tenantID int64, fqdn string) int64 {
	t.Helper()
	d, err := s.CreateDomain(context.Background(), tenantID, fqdn, "self", "")
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if err := s.SetDomainActive(context.Background(), d.ID); err != nil {
		t.Fatalf("activate domain: %v", err)
	}
	return d.ID
}

// insertVisit 直接落一行明细(rule_id 可空;createdAt 传零值表示"现在")。
// 命中计数与 rule_action 的细节在 CountRuleHits24h 的用例里逐项验证。
func insertVisit(t *testing.T, s *Store, linkID, domainID int64, ruleID *int64, ruleAction, outcome string, createdAt time.Time) {
	t.Helper()
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	if err := s.db.WithContext(context.Background()).Exec(
		`INSERT INTO visits (link_id, domain_id, rule_id, rule_action, action, outcome, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		linkID, domainID, ruleID, ruleAction, VisitActionRedirect, outcome, createdAt).Error; err != nil {
		t.Fatalf("insert visit: %v", err)
	}
}

// TestSetLinkRulesReplacesAssociation 短链侧整体替换关联:
// 保留要留的、删掉要去掉的、补上缺的;空数组 = 解除全部;跨租户规则 id 整笔回滚。
func TestSetLinkRulesReplacesAssociation(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "put@test.io")
	other := newTenant(t, s, "put-other@test.io")
	l1 := newLink(t, s, tenantID, "p1")
	l2 := newLink(t, s, tenantID, "p2")

	keep, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "保留", Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l2}})
	if err != nil {
		t.Fatalf("create keep: %v", err)
	}
	drop, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "去掉", Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l1}})
	if err != nil {
		t.Fatalf("create drop: %v", err)
	}
	_ = drop // drop.ID 用不到,但它必须真实存在,否则下面的整体替换测不到"删掉这条"
	add, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "补上", Scope: RuleScopeLinks, Action: RuleActionPass})
	if err != nil {
		t.Fatalf("create add: %v", err)
	}
	foreign, err := s.CreateRule(ctx, other, Rule{
		Name: "别人的", Scope: RuleScopeLinks, Action: RuleActionPass})
	if err != nil {
		t.Fatalf("create foreign: %v", err)
	}

	if err := s.SetLinkRules(ctx, tenantID, l1, []int64{keep.ID, add.ID}); err != nil {
		t.Fatalf("set link rules: %v", err)
	}
	ids, err := s.RuleIDsForLink(ctx, l1)
	if err != nil {
		t.Fatalf("rule ids for link: %v", err)
	}
	if len(ids) != 2 || ids[0] != keep.ID || ids[1] != add.ID {
		t.Fatalf("l1 关联 = %v, want [%d %d]", ids, keep.ID, add.ID)
	}
	// 只保留一条:整体替换而不是叠加
	if err := s.SetLinkRules(ctx, tenantID, l1, []int64{keep.ID}); err != nil {
		t.Fatalf("set link rules (replace): %v", err)
	}
	ids, _ = s.RuleIDsForLink(ctx, l1)
	if len(ids) != 1 || ids[0] != keep.ID {
		t.Fatalf("整体替换后 l1 关联 = %v, want [%d]", ids, keep.ID)
	}
	// 重复 id 去重,不产生重复行
	if err := s.SetLinkRules(ctx, tenantID, l1, []int64{keep.ID, keep.ID, add.ID}); err != nil {
		t.Fatalf("set link rules (dup): %v", err)
	}
	ids, _ = s.RuleIDsForLink(ctx, l1)
	if len(ids) != 2 {
		t.Fatalf("去重后 l1 关联 = %v, want 2 条", ids)
	}
	// l2 的关联不受影响(按短链替换,不是全局重写)
	ids2, _ := s.RuleIDsForLink(ctx, l2)
	if len(ids2) != 1 || ids2[0] != keep.ID {
		t.Fatalf("l2 关联被改动 = %v, want [%d]", ids2, keep.ID)
	}
	// 跨租户规则 id:整笔回滚,不能出现"写进去一半"
	if err := s.SetLinkRules(ctx, tenantID, l1, []int64{add.ID, foreign.ID}); err != ErrForeignRule {
		t.Fatalf("跨租户规则 id 错误 = %v, want ErrForeignRule", err)
	}
	ids, _ = s.RuleIDsForLink(ctx, l1)
	if len(ids) != 2 {
		t.Fatalf("失败写入后 l1 关联 = %v, want 2 条(回滚)", ids)
	}
	// 空数组 = 解除全部
	if err := s.SetLinkRules(ctx, tenantID, l1, nil); err != nil {
		t.Fatalf("clear link rules: %v", err)
	}
	ids, _ = s.RuleIDsForLink(ctx, l1)
	if len(ids) != 0 {
		t.Fatalf("解除后 l1 关联 = %v, want 空", ids)
	}
}

// CountRuleHits24h 从明细读时聚合:24h 窗口内、带 rule_id 的行(成功的与被规则拦下的都算命中);
// 窗口外的、无规则的、别的规则的行都不计入。零关联规则恒为 0。
func TestRuleHits24hAggregatedFromVisits(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "hits@test.io")
	linkID := newLink(t, s, tenantID, "h1")
	domainID := newDomainID(t, s, tenantID, "h1.hits.test")

	r1, err := s.CreateRule(ctx, tenantID, Rule{Name: "r1", Scope: RuleScopeGlobal, Action: RuleActionNotfound})
	if err != nil {
		t.Fatalf("create r1: %v", err)
	}
	r2, err := s.CreateRule(ctx, tenantID, Rule{Name: "r2", Scope: RuleScopeGlobal, Action: RuleActionThrottle})
	if err != nil {
		t.Fatalf("create r2: %v", err)
	}
	orphan, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "零关联", Scope: RuleScopeLinks, Action: RuleActionPass})
	if err != nil {
		t.Fatalf("create orphan: %v", err)
	}

	insertVisit(t, s, linkID, domainID, &r1.ID, RuleActionNotfound, VisitOutcomeFailed, time.Time{})
	insertVisit(t, s, linkID, domainID, &r1.ID, RuleActionRedirect, VisitOutcomeSuccess, time.Time{})
	insertVisit(t, s, linkID, domainID, &r2.ID, RuleActionThrottle, VisitOutcomeFailed, time.Time{})
	// 无规则的访问不计入任何规则
	insertVisit(t, s, linkID, domainID, nil, "", VisitOutcomeSuccess, time.Time{})
	// 25 小时前的命中落在窗口外
	insertVisit(t, s, linkID, domainID, &r1.ID, RuleActionNotfound, VisitOutcomeFailed,
		time.Now().Add(-25*time.Hour))

	hits, err := s.CountRuleHits24h(ctx, []int64{r1.ID, r2.ID, orphan.ID})
	if err != nil {
		t.Fatalf("count hits: %v", err)
	}
	if hits[r1.ID] != 2 {
		t.Fatalf("r1 24h 命中 = %d, want 2(窗口内两行,含被拦下的)", hits[r1.ID])
	}
	if hits[r2.ID] != 1 {
		t.Fatalf("r2 24h 命中 = %d, want 1", hits[r2.ID])
	}
	if hits[orphan.ID] != 0 {
		t.Fatalf("零关联规则 24h 命中 = %d, want 0", hits[orphan.ID])
	}
	// 空入参不生成 IN ()
	if got, err := s.CountRuleHits24h(ctx, nil); err != nil || len(got) != 0 {
		t.Fatalf("空入参 = %v (%v), want 空", got, err)
	}
}

// 规则被物理删除后历史明细保留,rule_id 置空(spec D8:ON DELETE SET NULL)。
func TestVisitRuleIDSurvivesRuleDelete(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "del@test.io")
	linkID := newLink(t, s, tenantID, "v1")
	domainID := newDomainID(t, s, tenantID, "v1.del.test")

	r, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "会被删掉", Scope: RuleScopeGlobal, Action: RuleActionNotfound})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	insertVisit(t, s, linkID, domainID, &r.ID, RuleActionNotfound, VisitOutcomeFailed, time.Time{})

	if err := s.DeleteRule(ctx, tenantID, r.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	var n int64
	if err := s.db.WithContext(ctx).Model(&Visit{}).Count(&n).Error; err != nil {
		t.Fatalf("count visits: %v", err)
	}
	if n != 1 {
		t.Fatalf("规则删除后明细行数 = %d, want 1(历史明细保留)", n)
	}
	// rule_action 留痕(判定是这次裁决做的),rule_id 因 ON DELETE SET NULL 置空
	var ruleIDIsNull bool
	if err := s.db.WithContext(ctx).Raw(
		`SELECT rule_id IS NULL FROM visits LIMIT 1`).Scan(&ruleIDIsNull).Error; err != nil {
		t.Fatalf("read rule_id: %v", err)
	}
	if !ruleIDIsNull {
		t.Fatal("规则删除后明细的 rule_id 仍指向它, want NULL(ON DELETE SET NULL)")
	}
	var ruleAction string
	if err := s.db.WithContext(ctx).Raw(
		`SELECT rule_action FROM visits LIMIT 1`).Scan(&ruleAction).Error; err != nil {
		t.Fatalf("read rule_action: %v", err)
	}
	if ruleAction != RuleActionNotfound {
		t.Fatalf("rule_action = %q, want %q", ruleAction, RuleActionNotfound)
	}
}

// 短链列表元数据:适用规则 = 全局规则 + 显式关联的规则,名字最多 3 个、计数是全量。
func TestLinkRuleMeta(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "meta@test.io")
	l1 := newLink(t, s, tenantID, "m1")
	l2 := newLink(t, s, tenantID, "m2")

	if _, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "全局", Priority: 10, Scope: RuleScopeGlobal, Action: RuleActionPass}); err != nil {
		t.Fatalf("create global: %v", err)
	}
	// 4 条 scoped 规则挂 l1(优先级 20/30/40/50),l2 一条都不挂
	for i, p := range []int{20, 30, 40, 50} {
		name := "scoped" + string(rune('a'+i))
		if _, err := s.CreateRule(ctx, tenantID, Rule{
			Name: name, Priority: p, Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l1}}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	// 与 l2 无关的规则不能算进 l2 的适用规则
	if _, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "别处", Priority: 5, Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l2}}); err != nil {
		t.Fatalf("create elsewhere: %v", err)
	}

	link, err := s.GetLinkByID(ctx, tenantID, l1)
	if err != nil {
		t.Fatalf("get link: %v", err)
	}
	if link.RuleCount != 5 { // 1 全局 + 4 scoped
		t.Fatalf("l1 ruleCount = %d, want 5", link.RuleCount)
	}
	want := []string{"全局", "scopeda", "scopedb"} // 优先级顺序,只回前 3 个(第 4 条走计数)
	if len(link.RuleNames) != maxLinkRuleNames {
		t.Fatalf("l1 ruleNames = %v, want %d 个", link.RuleNames, maxLinkRuleNames)
	}
	for i := range want {
		if link.RuleNames[i] != want[i] {
			t.Fatalf("l1 ruleNames = %v, want %v", link.RuleNames, want)
		}
	}
	link2, err := s.GetLinkByID(ctx, tenantID, l2)
	if err != nil {
		t.Fatalf("get link 2: %v", err)
	}
	if link2.RuleCount != 2 { // 全局 + 别处
		t.Fatalf("l2 ruleCount = %d, want 2", link2.RuleCount)
	}
	// 列表页(fillLinksMeta 的批量补齐)与详情口径必须一致
	links, _, err := s.ListLinksByTenant(ctx, tenantID, 1, 10)
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	byID := make(map[int64]*Link, len(links))
	for _, l := range links {
		byID[l.ID] = l
	}
	if byID[l1].RuleCount != 5 || len(byID[l1].RuleNames) != maxLinkRuleNames {
		t.Fatalf("列表页 l1 规则元数据 = %d / %v, want 5 / %d 个",
			byID[l1].RuleCount, byID[l1].RuleNames, maxLinkRuleNames)
	}
	if byID[l2].RuleCount != 2 {
		t.Fatalf("列表页 l2 ruleCount = %d, want 2", byID[l2].RuleCount)
	}
}

// CountOwnedLiveLinks:逻辑删除的短链不算"属于该租户"(关联一条看不见的短链只是误解来源)。
func TestCountOwnedLiveLinksExcludesDeleted(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "own@test.io")
	other := newTenant(t, s, "own-other@test.io")
	l1 := newLink(t, s, tenantID, "o1")
	deleted := newLink(t, s, tenantID, "o2")
	foreign := newLink(t, s, other, "o3")
	if err := s.SoftDeleteLink(ctx, tenantID, deleted); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	n, err := s.CountOwnedLiveLinks(ctx, tenantID, []int64{l1})
	if err != nil || n != 1 {
		t.Fatalf("正常短链 = %d (%v), want 1", n, err)
	}
	if n, _ := s.CountOwnedLiveLinks(ctx, tenantID, []int64{deleted}); n != 0 {
		t.Fatalf("已逻辑删除的短链 = %d, want 0", n)
	}
	if n, _ := s.CountOwnedLiveLinks(ctx, tenantID, []int64{foreign}); n != 0 {
		t.Fatalf("跨租户短链 = %d, want 0", n)
	}
	if n, _ := s.CountOwnedLiveLinks(ctx, tenantID, nil); n != 0 {
		t.Fatalf("空入参 = %d, want 0", n)
	}
}

// RulesByIDs / ListAllRules:批量取规则(租户隔离)与不分页的全量列表。
func TestRulesByIDsAndListAll(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "bulk@test.io")
	other := newTenant(t, s, "bulk-other@test.io")
	l1 := newLink(t, s, tenantID, "b1")

	a, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "a", Priority: 20, Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l1}})
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := s.CreateRule(ctx, tenantID, Rule{Name: "b", Priority: 10, Scope: RuleScopeGlobal, Action: RuleActionPass})
	if err != nil {
		t.Fatalf("create b: %v", err)
	}
	foreign, err := s.CreateRule(ctx, other, Rule{Name: "foreign", Action: RuleActionPass})
	if err != nil {
		t.Fatalf("create foreign: %v", err)
	}

	got, err := s.RulesByIDs(ctx, tenantID, []int64{a.ID, b.ID, foreign.ID, 999999})
	if err != nil {
		t.Fatalf("rules by ids: %v", err)
	}
	if len(got) != 2 || got[0].ID != b.ID || got[1].ID != a.ID {
		t.Fatalf("RulesByIDs = %+v, want b,a(按 priority 升序,不含他人/不存在)", got)
	}
	all, err := s.ListAllRules(ctx, tenantID)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 || all[0].Name != "b" || all[1].Name != "a" {
		t.Fatalf("ListAllRules = %+v, want b,a", all)
	}
	if len(all[1].LinkIDs) != 1 || all[1].LinkIDs[0] != l1 {
		t.Fatalf("ListAllRules 的关联 = %v, want [%d]", all[1].LinkIDs, l1)
	}
	if got, err := s.RulesByIDs(ctx, tenantID, nil); err != nil || len(got) != 0 {
		t.Fatalf("空入参 = %v (%v), want 空", got, err)
	}
}

// 规则名称在租户内唯一(API 层据此回 409)。
func TestRuleNameUniquePerTenantForAPI(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "uq2@test.io")
	if _, err := s.CreateRule(ctx, tenantID, Rule{Name: "唯一", Action: RuleActionPass}); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err := s.CreateRule(ctx, tenantID, Rule{Name: "唯一", Action: RuleActionPass})
	if !IsUniqueViolation(err) {
		t.Fatalf("重名错误 = %v, want 唯一约束冲突", err)
	}
}
