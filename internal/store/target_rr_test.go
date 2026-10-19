package store

// 轮询选路的纯函数规格（TDD 红阶段：TargetForRR 还未实现）。
//
// 背景：跳转热路径目前是 LookupLinkForVisit(含 JOIN + 一次 link_targets 查询)
// → PickTarget(**又查一次** link_targets + 一次 `UPDATE links SET rr_index` 行锁)。
// 短链快照缓存要成立，选目标必须能在内存里的 []string 上完成，
// 于是把 PickTarget 里"按 rr_index 取值"的那一步抽成纯函数。
//
// 公式必须与 SQL 侧逐位一致：
//
//	UPDATE links SET rr_index = rr_index + 1 ... RETURNING rr_index
//	targets[((cur-1)%n + n)%n]
//
// 差一位不是"小偏差"：每条多目标短链会稳定地少用一个目标（或反复命中同一个），
// 投放方看到的是某个落地页永远没有量，而日志里一切正常。

import (
	"fmt"
	"testing"
)

func TestTargetForRR(t *testing.T) {
	three := []string{"https://a.example.com", "https://b.example.com", "https://c.example.com"}
	four := []string{"w", "x", "y", "z"}
	single := []string{"https://only.example.com"}

	cases := []struct {
		name    string
		targets []string
		rr      int64
		want    string
	}{
		{
			name:    "Given nil 目标列表 When 任意 rr Then 返回空串(调用方据此判 no_target)",
			targets: nil,
			rr:      1,
			want:    "",
		},
		{
			name:    "Given 空切片 When rr=0 Then 返回空串",
			targets: []string{},
			rr:      0,
			want:    "",
		},
		{
			name:    "Given 单目标 When rr=0 Then 恒返回该目标",
			targets: single,
			rr:      0,
			want:    "https://only.example.com",
		},
		{
			name:    "Given 单目标 When rr=1 Then 恒返回该目标",
			targets: single,
			rr:      1,
			want:    "https://only.example.com",
		},
		{
			name:    "Given 单目标 When rr 远大(99) Then 仍返回该目标,不回绕成越界",
			targets: single,
			rr:      99,
			want:    "https://only.example.com",
		},
		{
			name:    "Given 单目标 When rr 为负(-7) Then 仍返回该目标",
			targets: single,
			rr:      -7,
			want:    "https://only.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=1 Then 取第 0 个",
			targets: three,
			rr:      1,
			want:    "https://a.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=2 Then 取第 1 个",
			targets: three,
			rr:      2,
			want:    "https://b.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=3 Then 取第 2 个",
			targets: three,
			rr:      3,
			want:    "https://c.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=4 Then 回绕到第 0 个",
			targets: three,
			rr:      4,
			want:    "https://a.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=6(整轮结束) Then 取第 2 个",
			targets: three,
			rr:      6,
			want:    "https://c.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=7 Then 回绕到第 0 个",
			targets: three,
			rr:      7,
			want:    "https://a.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=0 Then 按取模回绕到第 2 个",
			targets: three,
			rr:      0,
			want:    "https://c.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=-1 Then 回绕到第 1 个",
			targets: three,
			rr:      -1,
			want:    "https://b.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=-2 Then 回绕到第 0 个",
			targets: three,
			rr:      -2,
			want:    "https://a.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=-3 Then 回绕到第 2 个",
			targets: three,
			rr:      -3,
			want:    "https://c.example.com",
		},
		{
			name:    "Given 3 个目标 When rr=-4 Then 回绕到第 1 个",
			targets: three,
			rr:      -4,
			want:    "https://b.example.com",
		},
		{
			name:    "Given 4 个目标 When rr=4 Then 取第 3 个",
			targets: four,
			rr:      4,
			want:    "z",
		},
		{
			name:    "Given 4 个目标 When rr=8 Then 取第 3 个(整轮末位)",
			targets: four,
			rr:      8,
			want:    "z",
		},
		{
			name:    "Given 4 个目标 When rr=9 Then 回绕到第 0 个",
			targets: four,
			rr:      9,
			want:    "w",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TargetForRR(tc.targets, tc.rr); got != tc.want {
				t.Fatalf("TargetForRR(%v, %d) = %q, want %q", tc.targets, tc.rr, got, tc.want)
			}
		})
	}
}

// TestTargetForRRDoesNotMutateTargets 固化"入参只读"。
//
// 入参将来是短链快照缓存里共享的 []string：多个并发跳转持有同一个底层数组。
// 任何就地换序（排序、rotate）都等于在没有锁的情况下写共享内存，
// 并且会把这条短链的轮询顺序永久写坏 —— 表现是"轮询跳到某个目标后就固定住"。
func TestTargetForRRDoesNotMutateTargets(t *testing.T) {
	backing := []string{"a", "b", "c"}
	want := []string{"a", "b", "c"}
	targets := backing[:len(backing)]

	for _, rr := range []int64{-4, -1, 0, 1, 2, 3, 4, 99} {
		TargetForRR(targets, rr)

		t.Run(fmt.Sprintf("Given rr=%d When 调用 Then 入参与底层数组均不变", rr), func(t *testing.T) {
			if len(targets) != len(want) {
				t.Fatalf("调用后 len(targets) = %d, want %d", len(targets), len(want))
			}
			// 一并检查底层数组：只比较 targets 会漏掉"改的是备份切片"这类实现
			for i := range want {
				if backing[i] != want[i] {
					t.Fatalf("rr=%d 调用后底层数组被改写: %v, want %v", rr, backing, want)
				}
			}
		})
	}
}
