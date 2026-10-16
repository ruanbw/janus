package store

// schema 断言测试:某些修复只落在 migrations/init.sql 与访问计划上,没有可直接观察的
// store 行为变化。这里直接查真实 Postgres 的目录表,把"索引/约束确实存在"固化下来,
// 而不是靠人工读 SQL 文件。

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// indexDef 返回 public 下指定索引的定义文本(不存在返回空串)。
func indexDef(t *testing.T, s *Store, name string) string {
	t.Helper()
	var def string
	if err := s.db.WithContext(context.Background()).Raw(
		`SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = ?`, name,
	).Scan(&def).Error; err != nil {
		t.Fatalf("查询索引 %s: %v", name, err)
	}
	return def
}

// TestLinkDomainsDomainCodeIndexExists 固化热路径 lookupLink 的索引要求。
//
// 契约:link_domains 上必须存在覆盖 (domain_id, code) 的**非部分**索引。
//
// 为什么不能只靠已有的 link_domains_live_code_uniq:那是部分唯一索引
// (WHERE NOT link_deleted),而 lookupLink 的宽松口径要同时看到"未删除 + 历史软删"两类行
// (Order by ld.link_deleted, l.id DESC),planner 用不上部分索引,只能按 domain_id
// 全量扫短码。每次短链访问都走这条路,缺索引 = 每次访问多一次按域名的全表扫描。
func TestLinkDomainsDomainCodeIndexExists(t *testing.T) {
	s := setupStore(t)

	// Given 已应用初始化脚本的库
	// When 查询 idx_link_domains_domain_code
	def := indexDef(t, s, "idx_link_domains_domain_code")

	// Then 索引存在,且键列包含 domain_id 与 code(顺序不限,查计划只看前缀可用性)
	if def == "" {
		t.Fatalf("link_domains 上缺少 idx_link_domains_domain_code:宽松口径 lookupLink 无可用索引,每次访问按域名全量扫短码")
	}
	if !strings.Contains(def, "domain_id") || !strings.Contains(def, "code") {
		t.Fatalf("idx_link_domains_domain_code 定义不符: %s", def)
	}
}

// TestDeleteDomainCascadesVisits 固化 visits.domain_id 的外键语义。
//
// 契约:直接删 domains 行时,指向它的 visits 明细必须被库内 CASCADE 连带清理,
// 而不是抛外键违约、也不需要调用方"记得先删 visits"。
//
// 为什么必须由库保证:同一张 visits 表里 link_id 已经是 ON DELETE CASCADE,
// domain_id 却是裸 REFERENCES —— 两条外键语义自相矛盾。漏删 visits 的调用路径
// 会在删域名时抛 23503,把一次运维操作变成运行时 500。
func TestDeleteDomainCascadesVisits(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// Given 一条挂在自有 active 域名下、已产生访问明细的短链
	tenantID := newTenant(t, s, "domain-cascade@test.io")
	d, err := s.CreateDomain(ctx, tenantID, "cascade.test", "self", "")
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if err := s.SetDomainActive(ctx, d.ID); err != nil {
		t.Fatalf("activate domain: %v", err)
	}
	l, err := s.CreateLink(ctx, tenantID, "casc01", []string{"https://example.com/casc01"},
		RedirectStatus302, LinkTypeRedirect, LandingSourceURL, "", []int64{d.ID})
	if err != nil {
		t.Fatalf("create link: %v", err)
	}
	if err := s.InsertVisit(ctx, VisitRecord{
		LinkID: l.ID, DomainID: d.ID, Action: VisitActionRedirect, Outcome: VisitOutcomeSuccess,
	}); err != nil {
		t.Fatalf("insert visit: %v", err)
	}

	// When 直接删除域名行
	if err := s.db.WithContext(ctx).Exec(`DELETE FROM domains WHERE id = ?`, d.ID).Error; err != nil {
		t.Fatalf("删除域名失败(visits.domain_id 缺 ON DELETE CASCADE): %v", err)
	}

	// Then 该域名的访问明细被连带清理
	var n int64
	if err := s.db.WithContext(ctx).Model(&Visit{}).Where("domain_id = ?", d.ID).Count(&n).Error; err != nil {
		t.Fatalf("count visits: %v", err)
	}
	if n != 0 {
		t.Fatalf("删域名后仍残留 %d 条 visits:domain_id 外键未级联清理", n)
	}
}

// TestTiersQuotaNonNegativeConstraint 固化 tiers 配额列的非负约束。
//
// 契约:tiers.max_links / max_domains 必须拒绝负数(约束名为
// tiers_max_links_nonneg / tiers_max_domains_nonneg,存量库由 init.sql 的幂等
// DO 块补上加),0 仍合法(表示"不允许创建",与"无限制"由 NULL/-1 之类约定无关)。
//
// 为什么必须由库保证:配额比较是 `links < max_links` 形式的判定,负数会让任何租户
// 永远超限(或按实现不同绕开限制);这两个值来自平台管理配置,写错一个负号不该
// 只能靠前端表单拦住。
func TestTiersQuotaNonNegativeConstraint(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// Then 两个命名约束都必须存在
	for _, name := range []string{"tiers_max_links_nonneg", "tiers_max_domains_nonneg"} {
		var n int64
		if err := s.db.WithContext(ctx).Raw(
			`SELECT count(*) FROM pg_constraint WHERE conname = ?`, name).Scan(&n).Error; err != nil {
			t.Fatalf("query constraint %s: %v", name, err)
		}
		if n != 1 {
			t.Errorf("tiers 上缺少 CHECK 约束 %s", name)
		}
	}

	// When 插入负数配额(名字唯一,避免历史脏数据让断言退化成唯一冲突)
	errLinks := s.db.WithContext(ctx).Exec(
		`INSERT INTO tiers (name, max_links, max_domains) VALUES (?, -1, 10)`, uniqueTierName("neg-links")).Error
	if errLinks == nil {
		t.Error("max_links = -1 未被拒绝:负数配额会让配额判定永远超限")
	} else if !strings.Contains(errLinks.Error(), "tiers_max_links_nonneg") {
		t.Errorf("max_links = -1 报的不是预想的约束: %v", errLinks)
	}
	errDomains := s.db.WithContext(ctx).Exec(
		`INSERT INTO tiers (name, max_links, max_domains) VALUES (?, 10, -1)`, uniqueTierName("neg-domains")).Error
	if errDomains == nil {
		t.Error("max_domains = -1 未被拒绝")
	} else if !strings.Contains(errDomains.Error(), "tiers_max_domains_nonneg") {
		t.Errorf("max_domains = -1 报的不是预想的约束: %v", errDomains)
	}

	// 正向对照:0 与正常值是合法的
	if err := s.db.WithContext(ctx).Exec(
		`INSERT INTO tiers (name, max_links, max_domains) VALUES (?, 0, 0)`, uniqueTierName("zero-quota")).Error; err != nil {
		t.Errorf("0 配额应合法(表示不允许创建): %v", err)
	}
}

// uniqueTierName 给测试用的 tiers.name 加一个唯一后缀(tiers.name 是 UNIQUE)。
func uniqueTierName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// TestEmailTokensTenantKindIndexExists 固化 email_tokens 的租户维度索引。
//
// 契约:email_tokens 上必须存在 (tenant_id, kind) 索引。
//
// 现状与用途:目前唯一的按 token_hash 过滤(已有 UNIQUE 索引),因此这条索引不是
// 为消费路径服务的 —— 它是为"租户维度的 token 管理"准备的:按租户列出/作废某类
// token(如重置密码时作废该租户全部未用 verify token、pending 租户超期清理),
// 这些查询此前只能走 tenant_id 单列(无索引)或全表扫。
func TestEmailTokensTenantKindIndexExists(t *testing.T) {
	s := setupStore(t)

	// Given 已应用初始化脚本的库
	// When 查询 idx_email_tokens_tenant_kind
	def := indexDef(t, s, "idx_email_tokens_tenant_kind")

	// Then 索引存在且键列含 tenant_id / kind
	if def == "" {
		t.Fatalf("email_tokens 上缺少 idx_email_tokens_tenant_kind")
	}
	if !strings.Contains(def, "tenant_id") || !strings.Contains(def, "kind") {
		t.Fatalf("idx_email_tokens_tenant_kind 定义不符: %s", def)
	}
}
