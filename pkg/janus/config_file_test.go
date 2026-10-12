package janus

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWithConfigFileLoadsFileValues 钉住契约:janus.WithConfigFile 必须真的加载配置文件。
// 只存在于文件里(进程环境没有)的值必须反映到 App 的 cfg 上。
//
// 观察点说明:App.cfg 未导出,且本轮(RED)不允许给实现新增访问器,因此这里用包内测试
// 直接读取 app.cfg —— 这是当前唯一能精确钉住「cfg 反映文件值」的观察点。
func TestWithConfigFileLoadsFileValues(t *testing.T) {
	t.Run("Given 只存在于配置文件里的值 When janus.New(WithConfigFile(path)) Then App 的 cfg 反映文件值", func(t *testing.T) {
		// Given 进程环境里没有这两个变量(隔离真实环境,包括 shell 里 source 过的 .env)
		unsetEnvForTest(t, "JANUS_PLATFORM_DOMAIN")
		unsetEnvForTest(t, "JANUS_SERVER_PUBLIC_IP")

		path := filepath.Join(t.TempDir(), "janus.env")
		content := "JANUS_PLATFORM_DOMAIN=from-config-file.example\n" +
			"JANUS_SERVER_PUBLIC_IP=203.0.113.7\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("写入配置文件失败: %v", err)
		}

		// When 通过 WithConfigFile 指定该配置文件构造 App
		app, err := New(WithConfigFile(path))
		if err != nil {
			t.Fatalf("New(WithConfigFile(%q)) 失败: %v", path, err)
		}

		// Then App 的 cfg 必须反映配置文件里的值
		if app.cfg.PlatformDomain != "from-config-file.example" {
			t.Fatalf("cfg.PlatformDomain 应反映配置文件值 %q,实际为 %q(配置文件未被加载)",
				"from-config-file.example", app.cfg.PlatformDomain)
		}
		if app.cfg.ServerPublicIP != "203.0.113.7" {
			t.Fatalf("cfg.ServerPublicIP 应反映配置文件值 %q,实际为 %q(配置文件未被加载)",
				"203.0.113.7", app.cfg.ServerPublicIP)
		}
	})

	t.Run("Given 不存在的配置文件路径 When janus.New(WithConfigFile(path)) Then 返回错误", func(t *testing.T) {
		// Given 一个不存在的配置文件路径
		missing := filepath.Join(t.TempDir(), "does-not-exist.env")

		// When
		app, err := New(WithConfigFile(missing))

		// Then 必须返回错误且不产出 App
		if err == nil {
			t.Fatal("配置文件不存在时 New(WithConfigFile(...)) 必须返回错误")
		}
		if app != nil {
			t.Fatal("配置文件不存在时 New 必须返回 nil App")
		}
	})
}

// unsetEnvForTest 临时清空环境变量,并在测试结束时恢复,避免污染同进程内的其他测试。
func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	orig, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("清空环境变量 %s 失败: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, orig)
			return
		}
		_ = os.Unsetenv(key)
	})
}
