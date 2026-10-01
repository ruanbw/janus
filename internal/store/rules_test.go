package store

// rules 层的测试直接打真实 Postgres(不经过 httpapi):规则关联是纯数据层语义,
// 用黑盒 HTTP 测只能间接覆盖。这里不引入 testutil,避免 store 的测试依赖 httpapi。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	conds := Conditions(
		RuleCondition{Field: "country", Operator: "in", Values: []string{"US", "CA"}},
		RuleCondition{Field: "ua", Operator: "regex", Values: []string{"(?i)bot|crawler"}})
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
	if got.Conditions.Len() != 2 {
		t.Fatalf("条件组长度 = %d, want 2", got.Conditions.Len())
	}
	if got.Conditions.Leaves[0].Field != "country" || got.Conditions.Leaves[0].Operator != "in" {
		t.Fatalf("首条条件 = %+v", got.Conditions.Leaves[0])
	}
	if len(got.Conditions.Leaves[0].Values) != 2 || got.Conditions.Leaves[0].Values[1] != "CA" {
		t.Fatalf("首条条件 values = %v", got.Conditions.Leaves[0].Values)
	}
	if got.Conditions.Leaves[1].Values[0] != "(?i)bot|crawler" {
		t.Fatalf("正则条件值 = %v", got.Conditions.Leaves[1].Values)
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
	if got.Conditions.IsZero() || got.Conditions.Len() != 0 {
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
		Conditions: Conditions(RuleCondition{Field: "devtype", Operator: "eq", Values: []string{"bot"}}),
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
	if got.Conditions.Len() != 1 || got.Conditions.Leaves[0].Field != "devtype" {
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

func TestRuleCustomErrorPages(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "errpages@test.io")

	r, err := s.CreateRule(ctx, tenantID, Rule{
		Name:       "404拦下",
		Enabled:    true,
		Action:     RuleActionNotfound,
		PageMode:   "custom",
		CustomHTML: "<h1>Denied</h1>",
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if r.PageMode != "custom" || r.CustomHTML != "<h1>Denied</h1>" {
		t.Fatalf("created rule page = %q / %q, want custom / <h1>Denied</h1>", r.PageMode, r.CustomHTML)
	}

	got, err := s.GetRule(ctx, tenantID, r.ID)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if got.PageMode != "custom" || got.CustomHTML != "<h1>Denied</h1>" {
		t.Fatalf("got rule page = %q / %q", got.PageMode, got.CustomHTML)
	}

	// Update Rule pageMode & customHTML
	newMode := "default"
	newHTML := "<h2>Default Fallback</h2>"
	upd, err := s.UpdateRule(ctx, tenantID, r.ID, RuleUpdate{
		PageMode:   &newMode,
		CustomHTML: &newHTML,
	})
	if err != nil {
		t.Fatalf("update rule: %v", err)
	}
	if upd.PageMode != "default" || upd.CustomHTML != newHTML {
		t.Fatalf("updated rule page = %q / %q", upd.PageMode, upd.CustomHTML)
	}

	// RulesForTenant also preserves these fields
	rules, err := s.RulesForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("rules for tenant: %v", err)
	}
	if len(rules) != 1 || rules[0].PageMode != "default" || rules[0].CustomHTML != newHTML {
		t.Fatalf("rules for tenant page fields = %+v", rules)
	}
}

func TestTenantErrorPages(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "tenant-errpages@test.io")

	p404, p429, err := s.GetTenantErrorPages(ctx, tenantID)
	if err != nil {
		t.Fatalf("get initial error pages: %v", err)
	}
	if p404 != "" || p429 != "" {
		t.Fatalf("initial error pages = %q / %q, want empty", p404, p429)
	}

	if err := s.UpdateTenantErrorPages(ctx, tenantID, "<h1>Global 404</h1>", "<h1>Global 429</h1>"); err != nil {
		t.Fatalf("update error pages: %v", err)
	}

	p404, p429, err = s.GetTenantErrorPages(ctx, tenantID)
	if err != nil {
		t.Fatalf("get updated error pages: %v", err)
	}
	if p404 != "<h1>Global 404</h1>" || p429 != "<h1>Global 429</h1>" {
		t.Fatalf("updated error pages = %q / %q", p404, p429)
	}
}

// ---------- 条件 JSONB 的两种历史形态 ----------

// 历史数据全是扁平数组。读回来必须是扁平形态、回写也必须是扁平数组:
// 只要回写形态变了,一次无关的改名就会把所有老规则的 JSONB 改写成树。
func TestRuleConditionsJSONFlatStaysFlat(t *testing.T) {
	raw := `[{"field":"country","operator":"in","values":["CN","US"]},` +
		`{"field":"ua","operator":"contains","values":["bot"]}]`
	var c RuleConditions
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unmarshal flat: %v", err)
	}
	if c.IsTree() {
		t.Fatalf("扁平数组不该被解析成条件树: %#v", c)
	}
	if c.Len() != 2 || c.Leaves[0].Field != "country" || c.Leaves[1].Values[0] != "bot" {
		t.Fatalf("扁平解析结果 = %#v", c)
	}
	out, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != raw {
		t.Fatalf("回写形态变了:\n got %s\nwant %s", out, raw)
	}
}

// 树形态:组可以带 type:"group" 包一层 group/children,也可以把组字段直接铺在节点上。
// 两种写法都要认,且都能原样回写成树(形状不统一时以归一化后的树为准)。
func TestRuleConditionsJSONTree(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		logic   string
		wantLen int
		leaves  []string
	}{
		{
			name:    "显式 type+group",
			raw:     `{"type":"group","group":{"logic":"any","children":[{"type":"leaf","leaf":{"field":"country","operator":"eq","values":["CN"]}},{"type":"group","group":{"logic":"all","children":[{"type":"leaf","leaf":{"field":"path","operator":"contains","values":["/promo"]}}]}}]}}`,
			logic:   "any",
			wantLen: 2,
			leaves:  []string{"country", "path"},
		},
		{
			name:    "裸字段节点",
			raw:     `{"logic":"all","children":[{"field":"country","operator":"eq","values":["CN"]},{"field":"path","operator":"contains","values":["/promo"]}]}`,
			logic:   "all",
			wantLen: 2,
			leaves:  []string{"country", "path"},
		},
		{
			name:    "缺失 logic 按 all",
			raw:     `{"children":[{"field":"country","operator":"eq","values":["CN"]}]}`,
			logic:   "all",
			wantLen: 1,
			leaves:  []string{"country"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c RuleConditions
			if err := json.Unmarshal([]byte(tc.raw), &c); err != nil {
				t.Fatalf("unmarshal tree: %v", err)
			}
			if !c.IsTree() || c.Root == nil {
				t.Fatalf("没解析成条件树: %#v", c)
			}
			if c.Root.Group == nil || c.Root.Group.Logic != tc.logic {
				t.Fatalf("根组 logic = %#v, want %s", c.Root.Group, tc.logic)
			}
			// 叶子按深度优先摊平,顺序就是求值顺序
			if c.Len() != tc.wantLen {
				t.Fatalf("叶子数 = %d, want %d (%#v)", c.Len(), tc.wantLen, c.Leaves)
			}
			for i, want := range tc.leaves {
				if c.Leaves[i].Field != want {
					t.Fatalf("第 %d 个叶子 = %q, want %q", i, c.Leaves[i].Field, want)
				}
			}
			out, err := json.Marshal(c)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back RuleConditions
			if err := json.Unmarshal(out, &back); err != nil {
				t.Fatalf("回写不是合法 JSON: %v (%s)", err, out)
			}
			if !back.IsTree() || back.Len() != tc.wantLen || back.Root.Group.Logic != tc.logic {
				t.Fatalf("回写后形态变了: %s", out)
			}
		})
	}
}

// 脏数据一律降级成"无条件组"而不是让整份规则加载失败(与 Scan 的既有约定一致)。
func TestRuleConditionsJSONGarbageDegrades(t *testing.T) {
	// 注意:语法错误(截断的 JSON)由 encoding/json 自己挡下、根本不会进 UnmarshalJSON,
	// 这里只覆盖"合法但形状不对"的值——那才是历史脏数据真正的样子。
	for _, raw := range []string{`null`, `{}`, `[]`, `"条件"`, `123`, `[1,2,3]`, `{"foo":1}`, `[{"field":1}]`} {
		var c RuleConditions
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatalf("%q 不该报错: %v", raw, err)
		}
		if c.Len() != 0 {
			t.Fatalf("%q 解析出 %d 条叶子, want 0", raw, c.Len())
		}
	}
	var c RuleConditions
	if out, err := json.Marshal(c); err != nil || string(out) != "[]" {
		t.Fatalf("零值回写 = %s (%v), want []", out, err)
	}
	// null / 空输入必须保持零值(PATCH 用它表示"本次不动条件列")
	var zero RuleConditions
	if err := json.Unmarshal([]byte("null"), &zero); err != nil || !zero.IsZero() {
		t.Fatalf("null 后 = %#v (err=%v), want 零值", zero, err)
	}
}

// 树形态的 Value()(写库用)必须能被自己读回来——GORM 的读写都走这一条路。
func TestRuleConditionsValueRoundTripTree(t *testing.T) {
	tree := Tree(Group("any",
		Leaf(RuleCondition{Field: "country", Operator: "eq", Values: []string{"CN"}}),
		Group("all",
			Leaf(RuleCondition{Field: "path", Operator: "contains", Values: []string{"/promo"}}),
			Leaf(RuleCondition{Field: "ua", Operator: "regex", Values: []string{"(?i)bot"}}),
		),
	))
	raw, err := tree.Value()
	if err != nil {
		t.Fatalf("value: %v", err)
	}
	text := fmt.Sprint(raw)
	if !strings.HasPrefix(text, "{") {
		t.Fatalf("树形态的 Value() 不是对象: %s", text)
	}
	var back RuleConditions
	if err := back.Scan(text); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !back.IsTree() || back.Len() != 3 {
		t.Fatalf("读回来 = %#v", back)
	}
	if back.Root.Group.Logic != "any" || back.Root.Group.Children[1].Group.Logic != "all" {
		t.Fatalf("嵌套组结构丢了: %#v", back.Root)
	}
}

func TestRuleTypeAndExpressionPersistence(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tenantID := newTenant(t, s, "expr@test.io")

	// 1. 创建 Expr 规则
	created, err := s.CreateRule(ctx, tenantID, Rule{
		Name:       "Expr 规则",
		Scope:      RuleScopeGlobal,
		Action:     RuleActionPass,
		RuleType:   RuleTypeExpression,
		Expression: `Country in ["US", "CA"] && DevType == "bot"`,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if created.RuleType != RuleTypeExpression || created.Expression != `Country in ["US", "CA"] && DevType == "bot"` {
		t.Fatalf("创建后返回字段异常: %+v", created)
	}

	// 从库中重新查询
	got, err := s.GetRule(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	if got.RuleType != RuleTypeExpression || got.Expression != created.Expression {
		t.Fatalf("查询库结果字段异常: %+v", got)
	}

	// 2. 更新为其他表达式
	newExpr := `ip in_cidr "10.0.0.0/8"`
	updated, err := s.UpdateRule(ctx, tenantID, created.ID, RuleUpdate{
		Expression: &newExpr,
	})
	if err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}
	if updated.Expression != newExpr || updated.RuleType != RuleTypeExpression {
		t.Fatalf("更新后结果异常: %+v", updated)
	}

	gotAfterUpdate, err := s.GetRule(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("GetRule after update: %v", err)
	}
	if gotAfterUpdate.Expression != newExpr {
		t.Fatalf("更新后查库异常: %+v", gotAfterUpdate)
	}
}
