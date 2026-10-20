package httpapi

import (
	"testing"
	"time"
)

// 本文件锁定访问明细时间范围参数的解析(纯函数,不连库)。
//
// 为什么值得单独钉住:
//   - 「今日」必须是**自然日**的 00:00,而不是"24 小时前"。凌晨打开页面时,
//     24 小时窗口会把昨晚 23:50 的访问算进"今日",与总览页的口径直接打架;
//   - 空串/all 必须表示"不限",否则默认进入页面会莫名只看最近 7 天;
//   - 非法值要能被识别(由 handler 映射 400),而不是静默退化成"全部" ——
//     拼错窗口时给人另一个窗口的数字,比报错更糟。

// fixedNow 固定在月末附近,便于暴露 AddDate 的跨月/跨年边界。
func fixedNow() time.Time {
	return time.Date(2026, 3, 1, 5, 30, 0, 0, time.Local)
}

func TestVisitRangeAllMeansNoLowerBound(t *testing.T) {
	for _, raw := range []string{"", "all", "  all  "} {
		from, active, ok := visitRangeFrom(raw, fixedNow())
		if !ok {
			t.Fatalf("range=%q 应合法", raw)
		}
		if active || !from.IsZero() {
			t.Fatalf("range=%q 应表示不限, 得到 from=%v active=%v", raw, from, active)
		}
	}
}

func TestVisitRangeTodayStartsAtLocalMidnight(t *testing.T) {
	f := fixedNow()
	from, active, ok := visitRangeFrom("today", f)
	if !ok || !active {
		t.Fatalf("today 应合法且生效, ok=%v active=%v", ok, active)
	}

	want := time.Date(f.Year(), f.Month(), f.Day(), 0, 0, 0, 0, f.Location())
	if !from.Equal(want) {
		t.Fatalf("today 应等于本地 00:00 %v, 得到 %v", want, from)
	}
	// 凌晨 05:30 打开页面时,昨晚 23:50 不属于今日
	if !from.After(f.Add(-6 * time.Hour)) {
		t.Fatalf("today 不能用「24 小时前」近似, 得到 %v", from)
	}
}

func TestVisitRangeNDaysIncludesToday(t *testing.T) {
	// 2026-03-01 的「近 7 天」= 02-23 00:00 起(含今日共 7 个自然日),跨月边界
	from, active, ok := visitRangeFrom("7d", fixedNow())
	if !ok || !active {
		t.Fatalf("7d 应合法且生效, ok=%v active=%v", ok, active)
	}
	want := time.Date(2026, 2, 23, 0, 0, 0, 0, time.Local)
	if !from.Equal(want) {
		t.Fatalf("7d 左端应为 %v, 得到 %v", want, from)
	}
}

func TestVisitRangeRejectsUnknownValues(t *testing.T) {
	for _, raw := range []string{"2d", "week", "7", "30D"} {
		if _, _, ok := visitRangeFrom(raw, fixedNow()); ok {
			t.Fatalf("range=%q 应被拒绝", raw)
		}
	}
}
