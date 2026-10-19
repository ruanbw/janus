package store

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestVisitJSONOmitsNetworkColumns 固化 ADR-0012 §2.1 的基座字段收窄:
// visits.is_datacenter / visits.asn 在平台内没有数据源(internal/geo 的静态库
// 不提供 ASN 与机房标记,写入侧 VisitRecord 也刻意不带这两列),继续挂在 Visit 上
// 只会让开源基座看起来像"斗篷"雏形,而值恒为 false / ""。
//
// 两个刻意的取舍:
//   - 用 map[string]json.RawMessage 比较顶层键,不用 strings.Contains(payload, ...):
//     UA / referer / targetUrl 都是客户端可控字符串,下面就把 "isDatacenter" 和
//     "asn" 直接写进了值里 —— 子串判断会把"值里恰好有这个词"当成"字段还在",
//     得到假红;反过来改个名也照样漏判。
//   - 本文件的字面量不引用 IsDatacenter / ASN 两个字段:实现删掉它们之后本文件
//     必须仍然能编译。若在这里写 IsDatacenter: false,就等于"改实现顺带改测试",
//     这条契约也就没有约束力了。
func TestVisitJSONOmitsNetworkColumns(t *testing.T) {
	ruleID := int64(11)
	full := Visit{
		ID:         42,
		LinkID:     7,
		DomainID:   3,
		Domain:     "go.example.com",
		IP:         "203.0.113.9",
		UserAgent:  "Mozilla/5.0 (isDatacenter probe)",
		Referer:    "https://ref.example/asn",
		Action:     VisitActionRedirect,
		Outcome:    VisitOutcomeSuccess,
		Reason:     "",
		TargetURL:  "https://target.example/p?asn=AS15169",
		Country:    "CN",
		Lang:       "zh-CN",
		RuleID:     &ruleID,
		RuleAction: "pass",
		CreatedAt:  time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
	}

	payload, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal visit: %v", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(payload, &keys); err != nil {
		t.Fatalf("unmarshal visit 顶层键: %v", err)
	}

	t.Run("Given 一个填满所有字段的 Visit When json.Marshal Then 顶层键不含 isDatacenter 与 asn", func(t *testing.T) {
		for _, banned := range []string{"isDatacenter", "asn"} {
			if raw, ok := keys[banned]; ok {
				t.Errorf("JSON 仍暴露 %q 字段(值=%s):visits 表没有这一列的数据源,前端只会读到恒 false / 恒空字符串", banned, raw)
			}
		}
	})

	t.Run("Given 同一个 Visit When json.Marshal Then 仍保留 country / ruleAction / targetUrl / createdAt", func(t *testing.T) {
		// 反方向的漏删防线:收窄字段时最容易顺手把同属"地理/规则"的相邻字段一起删掉,
		// 但 country 由离线 ip2region 提供、ruleAction 是本次裁决唯一的留痕列。
		for _, kept := range []string{"country", "ruleAction", "targetUrl", "createdAt"} {
			if _, ok := keys[kept]; !ok {
				t.Errorf("JSON 缺少 %q 字段:该列仍在 visits 表里,API 必须继续回传", kept)
			}
		}
	})

	t.Run("Given Visit 类型 When 枚举结构体字段与其 json 标签 Then 不存在 IsDatacenter / ASN", func(t *testing.T) {
		// 为什么用反射而不是编译期断言(例如把 Visit 转换成只含保留字段的等价 struct):
		// 字段还在时那样写会让整个 store 包编译失败,同包其它用例一起消失,
		// 失败信息也只剩编译器一句话;反射能把"还留着哪个字段"直接打进断言输出。
		typ := reflect.TypeOf(Visit{})
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			jsonName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			// 字段名与 json 标签都要查:只删标签、留着 Go 字段同样会让人以为有数据源。
			if field.Name == "IsDatacenter" || field.Name == "ASN" ||
				jsonName == "isDatacenter" || jsonName == "asn" {
				t.Errorf("Visit 仍保留字段 %s (json=%q):ADR-0012 要求删掉这两列,字段留着等于继续宣称平台有 ASN/机房数据", field.Name, jsonName)
			}
		}
	})

	t.Run("Given 只含保留字段的 wire 结构体 When 解码同一份 JSON Then 各字段值原样回来", func(t *testing.T) {
		// 这条守的是"别顺手改坏其它列":json 标签一旦被改名(比如 country → countryCode),
		// 前端会静默读到 undefined,而 Go 侧结构体断言看不出来。
		// DomainID 是 json:"-"(内部外键,不外发),所以从 full 里带回来。
		var wire struct {
			ID         int64     `json:"id"`
			LinkID     int64     `json:"linkId"`
			Domain     string    `json:"domain"`
			IP         string    `json:"ip"`
			UserAgent  string    `json:"userAgent"`
			Referer    string    `json:"referer"`
			Action     string    `json:"action"`
			Outcome    string    `json:"outcome"`
			Reason     string    `json:"reason"`
			TargetURL  string    `json:"targetUrl"`
			Country    string    `json:"country"`
			Lang       string    `json:"lang"`
			RuleID     *int64    `json:"ruleId"`
			RuleAction string    `json:"ruleAction"`
			CreatedAt  time.Time `json:"createdAt"`
		}
		if err := json.Unmarshal(payload, &wire); err != nil {
			t.Fatalf("按 wire 结构体解码 visit: %v", err)
		}
		roundTripped := Visit{
			ID:         wire.ID,
			LinkID:     wire.LinkID,
			DomainID:   full.DomainID,
			Domain:     wire.Domain,
			IP:         wire.IP,
			UserAgent:  wire.UserAgent,
			Referer:    wire.Referer,
			Action:     wire.Action,
			Outcome:    wire.Outcome,
			Reason:     wire.Reason,
			TargetURL:  wire.TargetURL,
			Country:    wire.Country,
			Lang:       wire.Lang,
			RuleID:     wire.RuleID,
			RuleAction: wire.RuleAction,
			CreatedAt:  wire.CreatedAt,
		}
		if !reflect.DeepEqual(roundTripped, full) {
			t.Fatalf("往返后字段值不一致:\n got = %+v\nwant = %+v", roundTripped, full)
		}
	})
}
