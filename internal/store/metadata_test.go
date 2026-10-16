package store

// Metadata.Scan 的容错语义测试:坏 JSON 必须"不 panic + 归零 + 留日志"三者兼备。

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestMetadataScanInvalidJSONLogsAndReturnsEmpty 固化坏 JSON 的处置方式。
//
// 契约:Scan 解析失败时仍然返回空字典、不返回错误(读路径不能因为一行坏数据
// 就整体失败),但必须留下一条 warn 日志 —— 否则调用方看到的 metadata 是空字典,
// 无从区分"本来就没有元数据"和"库里那行 JSON 坏了"。
//
// 日志只带长度/类型,不带原文:metadata 是租户业务数据,可能含敏感信息。
func TestMetadataScanInvalidJSONLogsAndReturnsEmpty(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	// Given 一段坏 JSON(手工改库或外部导入写坏 JSONB 是现实场景)
	bad := []byte(`{"campaign": "spring",`)

	// When 扫描它
	var m Metadata
	err := m.Scan(bad)

	// Then 不报错、不 panic,得到空字典
	if err != nil {
		t.Fatalf("坏 JSON 不应向上返回错误(读路径要能继续): %v", err)
	}
	if m == nil {
		t.Fatal("坏 JSON 后 Metadata 不能是 nil map(调用方会直接写它)")
	}
	if len(m) != 0 {
		t.Fatalf("坏 JSON 解析出了 %d 个键, want 0", len(m))
	}
	// Then 留有 warn 日志,且不泄露原文
	logged := buf.String()
	if !strings.Contains(logged, "level=WARN") || !strings.Contains(logged, "metadata") {
		t.Fatalf("坏 JSON 未留下 warn 日志: %q", logged)
	}
	if strings.Contains(logged, "campaign") {
		t.Fatalf("日志泄露了元数据原文: %q", logged)
	}
}

// TestMetadataScanValidJSONQuiet 是正向对照:合法 JSON 必须正常解析且不产生 warn。
func TestMetadataScanValidJSONQuiet(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	var m Metadata
	if err := m.Scan([]byte(`{"a": 1, "b": "x"}`)); err != nil {
		t.Fatalf("scan valid json: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("合法 JSON 解析出 %d 个键, want 2", len(m))
	}
	if buf.Len() != 0 {
		t.Fatalf("合法 JSON 不应产生 any 日志: %q", buf.String())
	}

	// nil / 空 / "null" 都归零且不告警(库里的 NULL 是正常状态)
	for _, in := range []any{nil, []byte{}, []byte("null"), ""} {
		var e Metadata
		if err := e.Scan(in); err != nil {
			t.Fatalf("scan %v: %v", in, err)
		}
		if e == nil || len(e) != 0 {
			t.Fatalf("scan %v 得到 %v, want 空字典", in, e)
		}
	}
	if buf.Len() != 0 {
		t.Fatalf("空值/NULL 不应产生 warn 日志: %q", buf.String())
	}
}
