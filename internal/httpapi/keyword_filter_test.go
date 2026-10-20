package httpapi

import (
	"net/url"
	"strings"
	"testing"
)

// 本文件锁定访问明细关键词参数的解析契约(与 links_filter_test.go 同口径)。
//
// 关键词只做 trim 与长度上限:纯空白 = 不过滤(用户边打字边删空格不该收到 400);
// 超长直接 400,而不是把一整页文本拼接进 ILIKE 模式。

func TestParseKeywordAcceptsAndTrims(t *testing.T) {
	got, err := parseKeyword(url.Values{"keyword": {"  203.0.113.9  "}})
	if err != nil {
		t.Fatalf("合法关键词不应报错: %v", err)
	}
	if got != "203.0.113.9" {
		t.Fatalf("关键词应被 trim, 得到 %q", got)
	}
}

func TestParseKeywordEmptyMeansNoFilter(t *testing.T) {
	for _, q := range []url.Values{{}, {"keyword": {""}}, {"keyword": {"   "}}} {
		got, err := parseKeyword(q)
		if err != nil || got != "" {
			t.Fatalf("缺省/空白关键词应为不过滤, 得到 %q (err=%v)", got, err)
		}
	}
}

func TestParseKeywordCapsLength(t *testing.T) {
	long := strings.Repeat("a", maxSearchKeywordLength+1)
	if _, err := parseKeyword(url.Values{"keyword": {long}}); err == nil {
		t.Fatalf("超过 %d 字符的关键词应被拒绝", maxSearchKeywordLength)
	}

	ok := strings.Repeat("a", maxSearchKeywordLength)
	if _, err := parseKeyword(url.Values{"keyword": {ok}}); err != nil {
		t.Fatalf("等于上限的关键词应被接受: %v", err)
	}
}
