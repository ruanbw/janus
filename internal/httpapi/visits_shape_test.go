package httpapi

import (
	"encoding/json"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/ruanbw/janus/internal/store"
)

// webVisitFields 手工镜像 web/src/types/api.ts 里 `export interface Visit` 声明的字段名
// (按集合比较,与顺序无关)。为什么把 TS 侧写死成一张表:Go 测试在编译期拿不到 TS 类型,
// 这张表就是"后端 JSON 形状 = 前端声明形状"唯一可执行的契约 —— 任何一侧多一列 / 少一列
// 都会在这里失败,而不是等到浏览器里把缺失字段读成 undefined。
//
// 来源:ADR-0012 §2.1 收窄后的 Visit 接口(已删除 isDatacenter / asn)。
var webVisitFields = []string{
	"id",
	"linkId",
	"domain",
	"ip",
	"userAgent",
	"referer",
	"action",
	"outcome",
	"reason",
	"targetUrl",
	"country",
	"lang",
	"ruleId",
	"ruleAction",
	"createdAt",
}

// TestVisitJSONMatchesWebType 守住前后端字段形状一致:后端 store.Visit 的 JSON 键集合
// 必须与前端 Visit 接口逐字段对应。典型坏味道是后端删了列、前端类型还留着
// (或反过来),TypeScript 侧不会报错,页面只是永远显示空值。
func TestVisitJSONMatchesWebType(t *testing.T) {
	ruleID := int64(11)
	visit := store.Visit{
		ID:         42,
		LinkID:     7,
		Domain:     "go.example.com",
		IP:         "203.0.113.9",
		UserAgent:  "Mozilla/5.0",
		Referer:    "https://ref.example/asn",
		Action:     store.VisitActionRedirect,
		Outcome:    store.VisitOutcomeSuccess,
		Reason:     "",
		TargetURL:  "https://target.example/p",
		Country:    "CN",
		Lang:       "zh-CN",
		RuleID:     &ruleID,
		RuleAction: "pass",
		CreatedAt:  time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
	}

	payload, err := json.Marshal(visit)
	if err != nil {
		t.Fatalf("marshal store.Visit: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal store.Visit 顶层键: %v", err)
	}

	got := make([]string, 0, len(decoded))
	for key := range decoded {
		got = append(got, key)
	}
	sort.Strings(got)
	want := slices.Clone(webVisitFields)
	sort.Strings(want)

	t.Run("Given store.Visit When json.Marshal Then 顶层键集合恰好等于前端 Visit 接口声明的 15 个字段", func(t *testing.T) {
		var extra, missing []string
		for _, key := range got {
			if !slices.Contains(want, key) {
				extra = append(extra, key)
			}
		}
		for _, field := range want {
			if !slices.Contains(got, field) {
				missing = append(missing, field)
			}
		}
		if len(extra) > 0 || len(missing) > 0 {
			t.Fatalf("前后端字段形状不一致:后端多出 %v,后端缺少 %v\n后端实际键 = %v\n前端声明   = %v", extra, missing, got, want)
		}
	})

	t.Run("Given 前端 Visit 字段表 When 检查网络字段 Then 表中不含 isDatacenter 与 asn", func(t *testing.T) {
		// 这张表本身也要守住:若有人把两列抄回表里,上面"形状一致"会跟着假绿
		// (后端与表同时多出同两列,集合比较照样通过)。
		for _, banned := range []string{"isDatacenter", "asn"} {
			if slices.Contains(webVisitFields, banned) {
				t.Errorf("前端 Visit 字段表仍声明 %q:visits 表已删除该列,接口声明留着会让页面读到一个恒 undefined/false 的字段", banned)
			}
		}
	})

	t.Run("Given 后端实际 JSON When 直接搜索网络字段键 Then isDatacenter 与 asn 都不出现", func(t *testing.T) {
		// 白名单之外的显式黑名单断言:失败信息直接点名是哪个键,比"键集合多一个元素"好读,
		// 也防止有人顺手把表改宽来"修绿"上一条用例。
		for _, banned := range []string{"isDatacenter", "asn"} {
			if raw, ok := decoded[banned]; ok {
				t.Errorf("API JSON 仍回传 %q(值=%s):前端类型已不再声明该字段,回传只会成为死数据", banned, raw)
			}
		}
	})
}
