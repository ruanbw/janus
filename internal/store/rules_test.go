package store

// rules 层的测试直接打真实 Postgres(不经过 httpapi):规则关联是纯数据层语义,
// 用黑盒 HTTP 测只能间接覆盖。这里不引入 testutil,避免 store 的测试依赖 httpapi。

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cloak/internal/db"
)

// setupStore 连接测试库、执行迁移、清空业务表,返回一个只带 store 的环境。
// 测试库不可用时跳过(需先起 postgres 并建好 cloak_test 库)。
func setupStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	url := os.Getenv("CLOAK_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://cloak:cloak@localhost:5432/cloak_test?sslmode=disable"
	}
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("test database not available (%v)", err)
	}
	t.Cleanup(pool.Close)
	// 独占测试库到本包测试结束:httpapi 的测试也连同一个库并 TRUNCATE,
	// 而 go test 并行跑各包,不加互斥会互相把对方的数据清掉
	release, err := db.LockTestDB(ctx, pool)
	if err != nil {
		t.Fatalf("lock test database: %v", err)
	}
	t.Cleanup(release)
	if err := db.Migrate(ctx, pool, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 关联表随 tenants/links 一起 CASCADE 清掉(TRUNCATE ... CASCADE 覆盖外键引用方)
	if _, err := pool.Exec(ctx, `TRUNCATE tenants, tiers RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO tiers (name, max_links, max_domains) VALUES ('free', 100, 10)`); err != nil {
		t.Fatalf("insert free tier: %v", err)
	}
	gdb, err := db.OpenGORM(url)
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return New(gdb)
}

// newTenant 建一个测试租户。
func newTenant(t *testing.T, s *Store, email string) int64 {
	t.Helper()
	tn, err := s.CreateTenant(context.Background(), email, "hash-"+email, email, false)
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tn.ID
}

// newLink 建一条跳转型短链(挂在租户自有的 active 域名下)。
func newLink(t *testing.T, s *Store, tenantID int64, code string) int64 {
	t.Helper()
	ctx := context.Background()
	d, err := s.CreateDomain(ctx, tenantID, code+".rule.test", "self", "")
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
	return l.ID
}

func TestRuleConditionsJSONBRoundTrip(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "cond@test.io")

	conds := RuleConditions{
		{Field: "country", Operator: "in", Values: []string{"US", "CA"}},
		{Field: "ua", Operator: "regex", Values: []string{"(?i)bot|crawler"}},
	}
	r, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "境外爬虫", Scope: RuleScopeGlobal, Enabled: true,
		Logic: RuleLogicAny, Action: RuleActionNotfound, Conditions: conds,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	got, err := s.GetRule(ctx, tenantID, r.ID)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if len(got.Conditions) != 2 {
		t.Fatalf("条件组长度 = %d, want 2", len(got.Conditions))
	}
	if got.Conditions[0].Field != "country" || got.Conditions[0].Operator != "in" {
		t.Fatalf("首条条件 = %+v", got.Conditions[0])
	}
	if len(got.Conditions[0].Values) != 2 || got.Conditions[0].Values[1] != "CA" {
		t.Fatalf("首条条件 values = %v", got.Conditions[0].Values)
	}
	if got.Conditions[1].Values[0] != "(?i)bot|crawler" {
		t.Fatalf("正则条件值 = %v", got.Conditions[1].Values)
	}
}

// 空条件组必须写成 "[]" 而不是 null,且读回仍是空切片(而不是 nil)。
func TestRuleEmptyConditionsStoredAsEmptyArray(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "empty@test.io")

	r, err := s.CreateRule(ctx, tenantID, Rule{Name: "无条件", Action: RuleActionPass})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	var raw string
	if err := s.db.Raw(`SELECT conditions::text FROM rules WHERE id = ?`, r.ID).Scan(&raw).Error; err != nil {
		t.Fatalf("read raw conditions: %v", err)
	}
	if raw != "[]" {
		t.Fatalf("conditions 列 = %s, want []", raw)
	}
	got, err := s.GetRule(ctx, tenantID, r.ID)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if got.Conditions == nil || len(got.Conditions) != 0 {
		t.Fatalf("条件组 = %#v, want 空切片", got.Conditions)
	}
}

func TestRuleListOrderAndPaging(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "list@test.io")
	other := newTenant(t, s, "other@test.io")

	for _, r := range []Rule{
		{Name: "b-p50", Priority: 50, Action: RuleActionPass},
		{Name: "a-p10", Priority: 10, Action: RuleActionPass},
		{Name: "a-p10b", Priority: 10, Action: RuleActionPass},
	} {
		if _, err := s.CreateRule(ctx, tenantID, r); err != nil {
			t.Fatalf("create rule: %v", err)
		}
	}
	// 他人租户的同名规则不应出现在列表里
	if _, err := s.CreateRule(ctx, other, Rule{Name: "a-p10", Action: RuleActionPass}); err != nil {
		t.Fatalf("create other rule: %v", err)
	}

	all, total, err := s.ListRules(ctx, tenantID, 1, 10)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if total != 3 {
		t.Fatalf("总数 = %d, want 3", total)
	}
	want := []string{"a-p10", "a-p10b", "b-p50"}
	for i, w := range want {
		if all[i].Name != w {
			t.Fatalf("第 %d 条 = %q, want %q(按 priority 升序,同优先级按 id)", i, all[i].Name, w)
		}
	}
	page1, _, err := s.ListRules(ctx, tenantID, 1, 2)
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	page2, _, err := s.ListRules(ctx, tenantID, 2, 2)
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if len(page1) != 2 || len(page2) != 1 || page2[0].Name != "b-p50" {
		t.Fatalf("分页结果 = %v / %v", page1, page2)
	}
}

// 租户隔离:按 id 取/改/删都必须带 tenant_id 条件,他人规则一律 ErrNotFound。
func TestRuleTenantIsolation(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "iso-a@test.io")
	other := newTenant(t, s, "iso-b@test.io")

	r, err := s.CreateRule(ctx, tenantID, Rule{Name: "私有规则", Action: RuleActionNotfound})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := s.GetRule(ctx, other, r.ID); err != ErrNotFound {
		t.Fatalf("他人 GetRule 错误 = %v, want ErrNotFound", err)
	}
	desc := "改了不该改的"
	if _, err := s.UpdateRule(ctx, other, r.ID, RuleUpdate{Description: &desc}); err != ErrNotFound {
		t.Fatalf("他人 UpdateRule 错误 = %v, want ErrNotFound", err)
	}
	if err := s.DeleteRule(ctx, other, r.ID); err != ErrNotFound {
		t.Fatalf("他人 DeleteRule 错误 = %v, want ErrNotFound", err)
	}
	if err := s.SetRuleLinks(ctx, other, r.ID, nil); err != ErrNotFound {
		t.Fatalf("他人 SetRuleLinks 错误 = %v, want ErrNotFound", err)
	}
	// 确认原规则没被动过
	got, err := s.GetRule(ctx, tenantID, r.ID)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if got.Description != "" {
		t.Fatalf("描述被跨租户改写 = %q", got.Description)
	}
}

func TestRuleUpdatePartialFields(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "upd@test.io")

	r, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "原名", Description: "原描述", Priority: 10, Action: RuleActionPass,
		Destination: "", Enabled: true,
		Conditions: RuleConditions{{Field: "devtype", Operator: "eq", Values: []string{"bot"}}},
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	enabled := false
	name := "新名"
	empty := ""
	got, err := s.UpdateRule(ctx, tenantID, r.ID, RuleUpdate{
		Name: &name, Enabled: &enabled, Destination: &empty,
	})
	if err != nil {
		t.Fatalf("update rule: %v", err)
	}
	if got.Name != "新名" || got.Enabled || got.Destination != "" {
		t.Fatalf("更新结果 = %+v", got)
	}
	// 未传的字段保持原值
	if got.Description != "原描述" || got.Priority != 10 {
		t.Fatalf("未传字段被改写 = %+v", got)
	}
	if len(got.Conditions) != 1 || got.Conditions[0].Field != "devtype" {
		t.Fatalf("条件组被改写 = %+v", got.Conditions)
	}
	// 空 destination 必须真的写进去(零值不被 Updates 跳过)
	if v := rawColumn(t, s, r.ID, "destination"); v != "" {
		t.Fatalf("库内 destination = %q, want 空串", v)
	}
	if v := rawColumn(t, s, r.ID, "enabled"); v != "false" {
		t.Fatalf("库内 enabled = %q, want false", v)
	}
	// updated_at 由 GORM 维护
	if v := rawColumn(t, s, r.ID, "updated_at"); v == "" {
		t.Fatal("updated_at 为空")
	}
}

// rawColumn 读一列原始值(验证零值真的落库,而不只是内存里改了)。
func rawColumn(t *testing.T, s *Store, id int64, col string) string {
	t.Helper()
	var v string
	// col 只由测试内常量传入,不拼接外部输入
	if err := s.db.Raw(`SELECT `+col+`::text FROM rules WHERE id = ?`, id).Scan(&v).Error; err != nil {
		t.Fatalf("read %s: %v", col, err)
	}
	return v
}

// SetRuleLinks 整体替换 + 归属校验:跨租户短链 id 必须报错而不是静默丢弃。
func TestSetRuleLinksReplacesAndValidates(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "assoc@test.io")
	other := newTenant(t, s, "assoc-other@test.io")
	l1 := newLink(t, s, tenantID, "a1")
	l2 := newLink(t, s, tenantID, "a2")
	foreign := newLink(t, s, other, "b1")

	r, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "活动短链", Scope: RuleScopeLinks, Action: RuleActionRedirect, Destination: "https://a.test/",
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	// 建规则时不带关联:零关联的 scoped 规则是合法配置(不兜底成全局)
	if len(r.LinkIDs) != 0 {
		t.Fatalf("新建规则的关联 = %v, want 空", r.LinkIDs)
	}
	if err := s.SetRuleLinks(ctx, tenantID, r.ID, []int64{l1, l2}); err != nil {
		t.Fatalf("set rule links: %v", err)
	}
	// 整体替换:只留 l2
	if err := s.SetRuleLinks(ctx, tenantID, r.ID, []int64{l2, l2}); err != nil {
		t.Fatalf("replace rule links: %v", err)
	}
	links, err := s.ListRuleLinks(ctx, r.ID)
	if err != nil {
		t.Fatalf("list rule links: %v", err)
	}
	if len(links) != 1 || links[0].LinkID != l2 {
		t.Fatalf("关联 = %+v, want 仅 l2(去重 + 整体替换)", links)
	}
	// 跨租户短链:报错,且原有关联不受影响
	if err := s.SetRuleLinks(ctx, tenantID, r.ID, []int64{l1, foreign}); err != ErrForeignLink {
		t.Fatalf("跨租户 SetRuleLinks 错误 = %v, want ErrForeignLink", err)
	}
	links, err = s.ListRuleLinks(ctx, r.ID)
	if err != nil {
		t.Fatalf("list rule links: %v", err)
	}
	if len(links) != 1 || links[0].LinkID != l2 {
		t.Fatalf("失败写入后关联被改动 = %+v", links)
	}
	// 不存在的短链 id 同样报错
	if err := s.SetRuleLinks(ctx, tenantID, r.ID, []int64{l1, 99999999}); err != ErrForeignLink {
		t.Fatalf("不存在的短链 SetRuleLinks 错误 = %v, want ErrForeignLink", err)
	}
	// 传空数组 = 清空关联(合法)
	if err := s.SetRuleLinks(ctx, tenantID, r.ID, []int64{}); err != nil {
		t.Fatalf("clear rule links: %v", err)
	}
	links, err = s.ListRuleLinks(ctx, r.ID)
	if err != nil {
		t.Fatalf("list rule links: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("清空后关联 = %+v", links)
	}
}

func TestCreateRuleRejectsForeignLinks(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "cf@test.io")
	other := newTenant(t, s, "cf-other@test.io")
	foreign := newLink(t, s, other, "x1")

	// 关联写入失败必须让整条规则回滚,不能留下"规则已建、关联写不进去"的半成品
	if _, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "跨租户关联", Scope: RuleScopeLinks, Action: RuleActionPass,
		LinkIDs: []int64{foreign},
	}); err != ErrForeignLink {
		t.Fatalf("CreateRule 错误 = %v, want ErrForeignLink", err)
	}
	n, err := s.CountTenantRules(ctx, tenantID)
	if err != nil {
		t.Fatalf("count rules: %v", err)
	}
	if n != 0 {
		t.Fatalf("失败创建后仍留下 %d 条规则, want 0", n)
	}
}

func TestUpdateRuleReplacesLinks(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "url@test.io")
	other := newTenant(t, s, "url-other@test.io")
	l1 := newLink(t, s, tenantID, "u1")
	l2 := newLink(t, s, tenantID, "u2")
	foreign := newLink(t, s, other, "u3")

	r, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "可改关联", Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l1},
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	ids := []int64{l2}
	got, err := s.UpdateRule(ctx, tenantID, r.ID, RuleUpdate{LinkIDs: &ids})
	if err != nil {
		t.Fatalf("update links: %v", err)
	}
	if len(got.LinkIDs) != 1 || got.LinkIDs[0] != l2 {
		t.Fatalf("关联 = %v, want [%d]", got.LinkIDs, l2)
	}
	// 可读标识形如「短码@域名」:短码在租户内不唯一,只给短码会让同码短链无法区分
	if got.LinkNames[0] != "u2@u2.rule.test" || got.LinkCount != 1 {
		t.Fatalf("关联短码/计数 = %v / %d", got.LinkNames, got.LinkCount)
	}
	// 跨租户:整次更新回滚,规则本身也不该被改
	bad := []int64{l1, foreign}
	if _, err := s.UpdateRule(ctx, tenantID, r.ID, RuleUpdate{LinkIDs: &bad}); err != ErrForeignLink {
		t.Fatalf("跨租户更新关联错误 = %v, want ErrForeignLink", err)
	}
	got, err = s.GetRule(ctx, tenantID, r.ID)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if len(got.LinkIDs) != 1 || got.LinkIDs[0] != l2 {
		t.Fatalf("失败更新后关联 = %v, want [%d]", got.LinkIDs, l2)
	}
}

// TestUpdateRuleScopeToGlobalClearsLinks 锁住 spec D1/契约的关键取舍:
// scope 切到 global 必须清空关联。否则 rule_links 会留下「规则适用于全部短链」与
// 「规则只适用于这几条短链」两行互相矛盾的数据,租户日后切回 links 时旧关联会静默复活。
func TestUpdateRuleScopeToGlobalClearsLinks(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "scope@test.io")
	l1 := newLink(t, s, tenantID, "s1")
	l2 := newLink(t, s, tenantID, "s2")

	r, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "可收窄", Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l1, l2},
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// 只改 scope、不传 linkIds → 关联仍应被清空
	global := RuleScopeGlobal
	got, err := s.UpdateRule(ctx, tenantID, r.ID, RuleUpdate{Scope: &global})
	if err != nil {
		t.Fatalf("switch to global: %v", err)
	}
	if got.Scope != RuleScopeGlobal {
		t.Fatalf("scope = %q, want global", got.Scope)
	}
	if len(got.LinkIDs) != 0 || got.LinkCount != 0 {
		t.Fatalf("切到 global 后关联 = %v (count=%d), want 空", got.LinkIDs, got.LinkCount)
	}

	// 同时传 scope=global 与 linkIds → 仍然清空(矛盾状态不允许存在)
	back := RuleScopeLinks
	got, err = s.UpdateRule(ctx, tenantID, r.ID, RuleUpdate{Scope: &back, LinkIDs: &[]int64{l1}})
	if err != nil {
		t.Fatalf("switch back to links: %v", err)
	}
	if len(got.LinkIDs) != 1 || got.LinkIDs[0] != l1 {
		t.Fatalf("切回 links 后关联 = %v, want [%d]", got.LinkIDs, l1)
	}
	ids := []int64{l2}
	got, err = s.UpdateRule(ctx, tenantID, r.ID, RuleUpdate{Scope: &global, LinkIDs: &ids})
	if err != nil {
		t.Fatalf("switch to global with linkIds: %v", err)
	}
	if len(got.LinkIDs) != 0 {
		t.Fatalf("scope=global 同时传 linkIds 后关联 = %v, want 空(矛盾状态)", got.LinkIDs)
	}

	// 已是 global 时再传 linkIds:不动关联(没有发生作用域切换)
	got, err = s.UpdateRule(ctx, tenantID, r.ID, RuleUpdate{LinkIDs: &[]int64{l1}})
	if err != nil {
		t.Fatalf("patch links while global: %v", err)
	}
	if len(got.LinkIDs) != 1 || got.LinkIDs[0] != l1 {
		t.Fatalf("已 global 时改关联 = %v, want [%d]", got.LinkIDs, l1)
	}
}

func TestRuleIDsForLinkAndRulesForLink(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "forlink@test.io")
	other := newTenant(t, s, "forlink-other@test.io")
	l1 := newLink(t, s, tenantID, "f1")
	l2 := newLink(t, s, tenantID, "f2")
	foreign := newLink(t, s, other, "f3")

	global, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "全局", Priority: 20, Scope: RuleScopeGlobal, Action: RuleActionPass})
	if err != nil {
		t.Fatalf("create global: %v", err)
	}
	scoped, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "限定", Priority: 10, Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l1}})
	if err != nil {
		t.Fatalf("create scoped: %v", err)
	}
	// 不在 l1 上,也不启用:应出现在列表里(后台要看得见停用中的规则)
	disabled, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "停用", Priority: 5, Scope: RuleScopeLinks, Enabled: false,
		Action: RuleActionPass, LinkIDs: []int64{l1}})
	if err != nil {
		t.Fatalf("create disabled: %v", err)
	}
	// 与 l1 无关的 scoped 规则:不出现在 l1 的适用列表里
	if _, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "别处", Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l2}}); err != nil {
		t.Fatalf("create elsewhere: %v", err)
	}

	ids, err := s.RuleIDsForLink(ctx, l1)
	if err != nil {
		t.Fatalf("rule ids for link: %v", err)
	}
	if len(ids) != 2 || ids[0] != scoped.ID || ids[1] != disabled.ID {
		t.Fatalf("l1 关联规则 id = %v, want [%d %d]", ids, scoped.ID, disabled.ID)
	}

	rules, err := s.RulesForLink(ctx, tenantID, l1)
	if err != nil {
		t.Fatalf("rules for link: %v", err)
	}
	got := make([]string, 0, len(rules))
	for _, r := range rules {
		got = append(got, r.Name)
	}
	want := []string{"停用", "限定", "全局"}
	if len(got) != len(want) {
		t.Fatalf("l1 适用规则 = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("l1 适用规则 = %v, want %v(按 priority 升序)", got, want)
		}
	}
	// 全局规则即使关联为空也必然适用(spec D2:只有 scoped 规则零关联才不命中)
	rules, err = s.RulesForLink(ctx, tenantID, l2)
	if err != nil {
		t.Fatalf("rules for link l2: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("l2 适用规则数 = %d, want 2(全局 + 别处)", len(rules))
	}
	// 他人租户看不到:传别人的短链 id 只得到本租户的全局规则
	rules, err = s.RulesForLink(ctx, tenantID, foreign)
	if err != nil {
		t.Fatalf("rules for foreign link: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != global.ID {
		t.Fatalf("跨租户短链的适用规则 = %+v, want 仅全局规则 %d", rules, global.ID)
	}
}

// 短链物理删除后 rule_links 随之消失(库内 CASCADE),不留孤儿关联行。
func TestRuleLinksCascadeOnLinkPurge(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "cascade@test.io")
	l1 := newLink(t, s, tenantID, "c1")

	r, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "级联", Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l1}})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if err := s.PurgeLink(ctx, tenantID, l1); err != nil {
		t.Fatalf("purge link: %v", err)
	}
	links, err := s.ListRuleLinks(ctx, r.ID)
	if err != nil {
		t.Fatalf("list rule links: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("短链物理删除后关联 = %+v, want 空", links)
	}
	got, err := s.GetRule(ctx, tenantID, r.ID)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if got.LinkCount != 0 {
		t.Fatalf("关联计数 = %d, want 0", got.LinkCount)
	}
}

// 规则删除后关联随之消失;租户整体清空后规则也随之消失。
func TestRuleLinksCascadeOnRuleAndTenantDelete(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "cascade2@test.io")
	l1 := newLink(t, s, tenantID, "d1")

	r, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "级联", Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l1}})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if err := s.DeleteRule(ctx, tenantID, r.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	var n int64
	if err := s.db.Model(&RuleLink{}).Where("rule_id = ?", r.ID).Count(&n).Error; err != nil {
		t.Fatalf("count rule links: %v", err)
	}
	if n != 0 {
		t.Fatalf("规则删除后仍留 %d 条关联", n)
	}
	if err := s.DeleteRule(ctx, tenantID, r.ID); err != ErrNotFound {
		t.Fatalf("重复删除错误 = %v, want ErrNotFound", err)
	}

	// 重建一条后删租户,验证 rules/rule_links 一起走
	if _, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "级联2", Scope: RuleScopeLinks, Action: RuleActionPass, LinkIDs: []int64{l1}}); err != nil {
		t.Fatalf("create rule 2: %v", err)
	}
	if err := s.db.Exec(`DELETE FROM tenants WHERE id = ?`, tenantID).Error; err != nil {
		t.Fatalf("delete tenant: %v", err)
	}
	if err := s.db.Model(&Rule{}).Where("tenant_id = ?", tenantID).Count(&n).Error; err != nil {
		t.Fatalf("count rules: %v", err)
	}
	if n != 0 {
		t.Fatalf("租户删除后仍留 %d 条规则", n)
	}
	if err := s.db.Model(&RuleLink{}).Count(&n).Error; err != nil {
		t.Fatalf("count rule links: %v", err)
	}
	if n != 0 {
		t.Fatalf("租户删除后仍留 %d 条关联", n)
	}
}

func TestRuleUniqueNamePerTenant(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "uniq@test.io")
	other := newTenant(t, s, "uniq-other@test.io")

	if _, err := s.CreateRule(ctx, tenantID, Rule{Name: "同名", Action: RuleActionPass}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := s.CreateRule(ctx, tenantID, Rule{Name: "同名", Action: RuleActionPass}); !IsUniqueViolation(err) {
		t.Fatalf("同租户重名错误 = %v, want 唯一约束冲突", err)
	}
	// 他人租户可以同名
	if _, err := s.CreateRule(ctx, other, Rule{Name: "同名", Action: RuleActionPass}); err != nil {
		t.Fatalf("他租户同名创建失败: %v", err)
	}
}

func TestRulesForTenantLoader(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "loader@test.io")
	other := newTenant(t, s, "loader-other@test.io")
	l1 := newLink(t, s, tenantID, "l1")

	if _, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "关掉的", Priority: 1, Enabled: false, Action: RuleActionPass}); err != nil {
		t.Fatalf("create disabled: %v", err)
	}
	if _, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "b", Priority: 30, Enabled: true, Scope: RuleScopeGlobal, Action: RuleActionPass}); err != nil {
		t.Fatalf("create b: %v", err)
	}
	if _, err := s.CreateRule(ctx, tenantID, Rule{
		Name: "a", Priority: 20, Enabled: true, Scope: RuleScopeLinks,
		Action: RuleActionPass, LinkIDs: []int64{l1}}); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := s.CreateRule(ctx, other, Rule{Name: "别人的", Action: RuleActionPass}); err != nil {
		t.Fatalf("create other: %v", err)
	}

	rules, err := s.RulesForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("rules for tenant: %v", err)
	}
	// 只返回启用的、按 priority 升序、带关联短链
	if len(rules) != 2 || rules[0].Name != "a" || rules[1].Name != "b" {
		t.Fatalf("快照加载结果 = %+v", rules)
	}
	if len(rules[0].LinkIDs) != 1 || rules[0].LinkIDs[0] != l1 {
		t.Fatalf("关联 = %v, want [%d]", rules[0].LinkIDs, l1)
	}
	if len(rules[1].LinkIDs) != 0 {
		t.Fatalf("全局规则的关联 = %v, want 空", rules[1].LinkIDs)
	}
}

func TestCountTenantRules(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "count@test.io")
	other := newTenant(t, s, "count-other@test.io")

	n, err := s.CountTenantRules(ctx, tenantID)
	if err != nil || n != 0 {
		t.Fatalf("空租户规则数 = %d (%v), want 0", n, err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.CreateRule(ctx, tenantID, Rule{
			Name: "r" + string(rune('a'+i)), Action: RuleActionPass}); err != nil {
			t.Fatalf("create rule: %v", err)
		}
	}
	if _, err := s.CreateRule(ctx, other, Rule{Name: "别人的", Action: RuleActionPass}); err != nil {
		t.Fatalf("create other: %v", err)
	}
	n, err = s.CountTenantRules(ctx, tenantID)
	if err != nil || n != 3 {
		t.Fatalf("规则数 = %d (%v), want 3", n, err)
	}
	if MaxRulesPerTenant != 200 {
		t.Fatalf("MaxRulesPerTenant = %d, want 200", MaxRulesPerTenant)
	}
}
