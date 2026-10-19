package db

import (
	"os"
	"path/filepath"
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
