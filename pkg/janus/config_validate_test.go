package janus_test

import (
	"strings"
	"testing"
	"time"

	"janus/pkg/janus"
)

// TestValidateAppConfigZeroTolerance 钉住 validateAppConfig 的校验契约:
//  1. 非正时长/计数不再被「符号启发式」漏掉 —— 必须显式报错;
//  2. 「整块未配置」的零值配置属于 SDK 轻量场景(只注入 Addr/JWTSecret),
//     必须容忍 —— 这是 Config.Validate 的零值语义。
func TestValidateAppConfigZeroTolerance(t *testing.T) {
	t.Run("Given 负时长配置(旧启发式会因 SessionTTL<0 且 LandingMaxFiles=0 而跳过) When New Then 返回错误而不是静默通过", func(t *testing.T) {
		// 旧实现:cfg.SessionTTL(= 负) > 0 为 false,LandingMaxFiles(= 0) > 0 为 false,
		// 于是整段 Validate 被跳过,负时长被放行 —— 这里必须变成显式错误。
		_, err := janus.New(janus.WithConfig(janus.Config{
			Addr:             "127.0.0.1:0",
			SessionTTL:       -time.Hour,
			DNSRetryInterval: -time.Second,
		}))
		if err == nil {
			t.Fatal("负时长配置必须被 validateAppConfig 拒绝,got nil error")
		}
	})

	t.Run("Given 负计数配置 When New Then 返回错误", func(t *testing.T) {
		_, err := janus.New(janus.WithConfig(janus.Config{
			Addr:              "127.0.0.1:0",
			VisitCleanupBatch: -1,
		}))
		if err == nil {
			t.Fatal("负计数配置必须被 validateAppConfig 拒绝,got nil error")
		}
	})

	t.Run("Given 全零值轻量配置 When New Then 成功(整块未配置须容忍)", func(t *testing.T) {
		app, err := janus.New(janus.WithConfig(janus.Config{
			Addr: "127.0.0.1:0",
		}))
		if err != nil {
			t.Fatalf("全零值轻量配置应通过校验,got %v", err)
		}
		if app == nil {
			t.Fatal("expected non-nil App")
		}
	})

	t.Run("Given SDK 只注入 JWTSecret/JWTTTL 的部分配置 When New Then 成功(其余整块为零须容忍)", func(t *testing.T) {
		// 对齐 protected_routes_test 的用法:只配 JWT 相关字段,运行期时长整块为零。
		app, err := janus.New(janus.WithConfig(janus.Config{
			JWTSecret: strings.Repeat("s", 40),
			JWTTTL:    time.Hour,
		}))
		if err != nil {
			t.Fatalf("部分配置应通过校验,got %v", err)
		}
		if app == nil {
			t.Fatal("expected non-nil App")
		}
	})

	t.Run("Given 正常完整配置 When New Then 成功", func(t *testing.T) {
		_, err := janus.New(janus.WithConfig(janus.Config{
			Addr:                 "127.0.0.1:0",
			SessionTTL:           12 * time.Hour,
			SessionTTLShort:      time.Hour,
			VerifyTokenTTL:       time.Hour,
			ResetTokenTTL:        time.Hour,
			JWTTTL:               time.Hour,
			DNSRetryInterval:     time.Minute,
			DNSMaxAge:            time.Hour,
			VisitRetention:       time.Hour,
			VisitCleanupEvery:    time.Hour,
			VisitCleanupBatch:    1000,
			LandingMaxFiles:      100,
			MaxTargetURLs:        10,
			LandingMaxZipBytes:   1 << 20,
			LandingMaxTotalBytes: 1 << 20,
		}))
		if err != nil {
			t.Fatalf("正常完整配置应通过校验,got %v", err)
		}
	})
}
