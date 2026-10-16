package store

// schema 断言测试:某些修复只落在 migrations/init.sql 与访问计划上,没有可直接观察的
// store 行为变化。这里直接查真实 Postgres 的目录表,把"索引/约束确实存在"固化下来,
// 而不是靠人工读 SQL 文件。

import (
	"context"
	"strings"
	"testing"
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
