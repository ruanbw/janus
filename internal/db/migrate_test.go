package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const migrationsDir = "../../migrations"

// TestInitScriptIsTheOnlySchemaArtifact 固化「只留初始化脚本」:
// migrations/ 下只有 init.sql,且它是可整段执行的纯 SQL,不依赖 goose 迁移框架。
func TestInitScriptIsTheOnlySchemaArtifact(t *testing.T) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var sqlFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			sqlFiles = append(sqlFiles, e.Name())
		}
	}

	t.Run("Given migrations 目录 When 枚举 SQL 文件 Then 只有唯一的初始化脚本 init.sql", func(t *testing.T) {
		if len(sqlFiles) != 1 || sqlFiles[0] != "init.sql" {
			t.Fatalf("期望 migrations/ 下只有 init.sql 一份初始化脚本,实际 %v", sqlFiles)
		}
	})

	t.Run("Given 初始化脚本 When 检查内容 Then 不依赖 goose 扩展(不含 +goose 注解,可整段执行)", func(t *testing.T) {
		if len(sqlFiles) == 0 {
			t.Fatal("migrations/ 下没有任何 SQL 初始化脚本")
		}
		content, err := os.ReadFile(filepath.Join(migrationsDir, sqlFiles[0]))
		if err != nil {
			t.Fatalf("read %s: %v", sqlFiles[0], err)
		}
		if strings.Contains(string(content), "+goose") {
			t.Fatalf("%s 含 goose 注解:初始化脚本应是不依赖迁移框架的纯 SQL", sqlFiles[0])
		}
	})
}

// TestInitScriptDropsNetworkColumns 固化 ADR-0012 §2.1:visits 表不再有
// is_datacenter / asn 两列(平台没有这两个值的数据源,见 internal/geo),但 country
// 必须保留(离线 ip2region 仍然提供国家码)。
//
// 同时守住 AGENTS.md 的改表约定:开发期改表是"直接改 init.sql 并重建库",
// 所以整份脚本里不能出现 ALTER TABLE ... DROP COLUMN —— 那是给存量库做就地升级,
// 与"migrations/ 下只有唯一初始化脚本"的模型冲突,而且 DROP COLUMN 会重写整表、
// 在每次启动都执行的路径上锁大表。
func TestInitScriptDropsNetworkColumns(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(migrationsDir, "init.sql"))
	if err != nil {
		t.Fatalf("read init.sql: %v", err)
	}
	sqlText := string(content)
	block := visitCreateTableBlock(t, sqlText)
	// 剥掉 "--" 行注释后再断言:块里有用中文解释约束语义的注释,
	// 不剥掉的话,注释里出现被禁列名就会假红(列已删、注释还提着)。
	stripped := stripSQLLineComments(block)

	t.Run("Given init.sql 的 visits 建表块 When 按顺序取列名 Then 恰好是收窄后保留的 15 列", func(t *testing.T) {
		got := visitColumnNames(stripped)
		want := []string{
			"id", "link_id", "domain_id", "ip", "user_agent", "referer",
			"action", "outcome", "reason", "target_url", "lang", "country",
			"rule_id", "rule_action", "created_at",
		}
		if len(got) != len(want) {
			t.Fatalf("visits 列数 = %d,期望 %d:\n got = %v\nwant = %v", len(got), len(want), got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("visits 第 %d 列 = %q,期望 %q:\n got = %v\nwant = %v", i+1, got[i], want[i], got, want)
			}
		}
	})

	t.Run("Given visits 建表块 When 搜索网络列 Then 既没有 is_datacenter 也没有 asn", func(t *testing.T) {
		// 这是本轮的 Red 信号:两列目前仍在建表块里,且没有任何数据源会填它们。
		for _, col := range []string{"is_datacenter", "asn"} {
			if re := visitColumnDefinition(col); re.MatchString(stripped) {
				t.Errorf("visits 建表块仍有列 %q:该列的值恒为默认值,留着只会让基座看起来具备机房/ASN 识别能力", col)
			}
		}
	})

	t.Run("Given visits 建表块 When 搜索国家列 Then country 仍然保留", func(t *testing.T) {
		// 反方向的漏删防线:country 与那两个占位列同属"地理"字段,
		// 但它是离线 ip2region 真能给出的国家码,删掉会让明细少一半信息。
		if !visitColumnDefinition("country").MatchString(stripped) {
			t.Fatalf("visits 建表块缺少 country 列:离线 ip2region 仍提供国家码,该列必须保留")
		}
	})

	t.Run("Given 整份 init.sql When 大小写不敏感地搜索 DROP COLUMN Then 没有任何就地删列", func(t *testing.T) {
		// 这条目前就成立,它挡的是"另一种实现路线":用 ALTER TABLE ... DROP COLUMN
		// 就地升级存量库。那样既违背"只留唯一初始化脚本"的模型,也会在启动路径上锁表。
		if re := regexp.MustCompile(`(?i)\bdrop\s+column\b`); re.MatchString(sqlText) {
			t.Errorf("init.sql 出现 DROP COLUMN:开发期改表应直接改建表块并重建库,不做就地升级")
		}
	})
}

// visitCreateTableBlock 从 init.sql 里截出 visits 的建表块(含结尾的 ");")。
// 用"第一个 CREATE TABLE IF NOT EXISTS visits ("到其后的 "\n);" 定界,而不是整份文件
// 扫关键字:文件后面还有大量 visits 索引、以及 DO $$ 块里的外键重建逻辑,
// 只有把断言限定在列定义块内,才能区分"列"与"索引/迁移注释里顺带提到的词"。
func visitCreateTableBlock(t *testing.T, sqlText string) string {
	t.Helper()
	const marker = "CREATE TABLE IF NOT EXISTS visits ("
	start := strings.Index(sqlText, marker)
	if start < 0 {
		t.Fatalf("init.sql 找不到 %q", marker)
	}
	const blockEnd = "\n);"
	end := strings.Index(sqlText[start:], blockEnd)
	if end < 0 {
		t.Fatalf("init.sql 的 visits 建表块没有以 \\n); 结尾")
	}
	return sqlText[start : start+end+len(blockEnd)]
}

// visitColumnDefinition 匹配"某一行以该列名开头、后面跟着类型"的形状。
// 行首锚定 + 只吃行内空白([^\S\n],不是 \s,后者会跨行)是刻意的:
// 它只认列定义,不会命中注释或索引里出现同名词的地方。
func visitColumnDefinition(column string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^[^\S\n]*` + regexp.QuoteMeta(column) + `[^\S\n]+[A-Za-z]`)
}

// visitColumnLine 是带捕获组的列名行形状,供 visitColumnNames 按顺序取列名。
var visitColumnLine = regexp.MustCompile(`(?m)^[^\S\n]*([a-z][a-z0-9_]*)[^\S\n]+[A-Za-z]`)

// visitColumnNames 按出现顺序取出建表块里的列名(表级约束行不以列名+类型开头,不会误命中)。
func visitColumnNames(strippedBlock string) []string {
	matches := visitColumnLine.FindAllStringSubmatch(strippedBlock, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

// stripSQLLineComments 去掉 "--" 行注释(SQL 字符串字面量里不含该序列,本文件也没有跨行注释)。
func stripSQLLineComments(block string) string {
	var b strings.Builder
	for _, line := range strings.Split(block, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
