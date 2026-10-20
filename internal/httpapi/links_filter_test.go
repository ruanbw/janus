package httpapi

import (
	"net/url"
	"strings"
	"testing"

	"github.com/ruanbw/janus/internal/store"
)

// 本文件锁定短链列表筛选参数的解析契约。
//
// 背景:列表接口原先只认 page/pageSize,前端于是在当前页里做前端过滤 ——
// 「共 N 条」「分页」都是全量,搜索只能搜到第 1 页里恰好存在的那几条。
// 下推到服务端后,参数解析必须明确:非法值 400 而不是静默忽略。

func TestParseLinkListFilterAcceptsKnownValues(t *testing.T) {
	q := url.Values{}
	q.Set("keyword", "  promo  ")
	q.Set("type", "landing")
	q.Set("status", "enabled")

	f, err := parseLinkListFilter(q)
	if err != nil {
		t.Fatalf("合法参数不应报错: %v", err)
	}
	want := store.LinkListFilter{Keyword: "promo", Type: "landing", Status: "enabled"}
	if f != want {
		t.Fatalf("解析结果 %+v, want %+v", f, want)
	}
}

func TestParseLinkListFilterEmptyMeansNoFilter(t *testing.T) {
	f, err := parseLinkListFilter(url.Values{})
	if err != nil {
		t.Fatalf("缺省参数不应报错: %v", err)
	}
	if f != (store.LinkListFilter{}) {
		t.Fatalf("缺省参数应为零值过滤, 得到 %+v", f)
	}

	// 显式空串与缺省同义(前端清空输入框后可能发 `keyword=`);纯空白也按"没填"处理,
	// 否则用户边打字边删空格会突然收到 400。
	q := url.Values{"keyword": {"   "}, "type": {""}, "status": {""}}
	if f, err := parseLinkListFilter(q); err != nil || f != (store.LinkListFilter{}) {
		t.Fatalf("空串/纯空白应视为不过滤, 得到 %+v (err=%v)", f, err)
	}
}

func TestParseLinkListFilterRejectsUnknownValues(t *testing.T) {
	cases := []struct {
		name  string
		query url.Values
		hint  string
	}{
		{"未知类型", url.Values{"type": {"bogus"}}, "type"},
		{"未知状态", url.Values{"status": {"paused"}}, "status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseLinkListFilter(tc.query); err == nil {
				t.Fatalf("非法参数应返回错误, 却通过了")
			} else if !strings.Contains(err.Error(), tc.hint) {
				t.Fatalf("错误信息应指出哪个参数不合法, 得到 %q", err.Error())
			}
		})
	}
}

func TestParseLinkListFilterCapsKeywordLength(t *testing.T) {
	long := strings.Repeat("a", maxSearchKeywordLength+1)
	if _, err := parseLinkListFilter(url.Values{"keyword": {long}}); err == nil {
		t.Fatalf("超过 %d 字符的关键词应被拒绝", maxSearchKeywordLength)
	}

	// 边界值本身合法
	ok := strings.Repeat("a", maxSearchKeywordLength)
	if _, err := parseLinkListFilter(url.Values{"keyword": {ok}}); err != nil {
		t.Fatalf("等于上限的关键词应被接受: %v", err)
	}
}
