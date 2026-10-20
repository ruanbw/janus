package httpapi

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// maxSearchKeywordLength 列表搜索关键词的长度上限(短链列表与访问明细共用)。
//
// 上限不是为了省索引(ILIKE 本来就要扫),而是防止把一整页文本当关键词发过来:
// 那样每次请求都会拼出一个巨大的 LIKE 模式,既费 CPU 也没有任何搜索意义。
const maxSearchKeywordLength = 100

// parseKeyword 取出并校验 `keyword` 查询参数。
//
// 空串与纯空白都视为"没有筛选":前端清空输入框、或者用户边打字边删空格时,
// 都不该突然收到一个 400。超长则明确报错 —— 静默截断会让用户以为搜的是完整词,
// 结果却按前缀匹配,找不到时完全无从判断原因。
func parseKeyword(q url.Values) (string, error) {
	keyword := strings.TrimSpace(q.Get("keyword"))
	if utf8.RuneCountInString(keyword) > maxSearchKeywordLength {
		return "", fmt.Errorf("keyword 过长(最多 %d 个字符)", maxSearchKeywordLength)
	}
	return keyword, nil
}
