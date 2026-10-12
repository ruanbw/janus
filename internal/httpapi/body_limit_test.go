package httpapi_test

// 05 — 请求体大小上限黑盒测试:所有接受 JSON 请求体的端点都必须有 body 上限,
// 超限返回 400(不是 500、不是 OOM、不是静默接受)。
//
// 既有约定见 rules_hardening_test.go 的 TestRuleEndpointsRejectOversizeBody:
// 超大 body 必须是 400。这里补齐尚未设限的端点 —— 未认证的 POST /api/auth/login
// 与已认证的写端点。json.Decoder 会把整份 body 读进内存,没有上限时一个几十 MB
// 的匿名请求就能把内存打满。

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"janus/internal/httpapi"
	"janus/internal/store"
	"janus/internal/testutil"
)

// oversizePaddingBytes 测试用超大 body 体积(2MiB)。
//
// 必须**远大于**任何既有上限,才能确定测到的是「body 上限」而不是逐字段上限:
//   - maxRuleBodyBytes = 1MiB(internal/httpapi/rules.go)
//   - maxCreateLinkBodyBytes() 默认约 266KiB(links.go,50 个目标 × 4KB + 余量)
//
// 2MiB 同时超过两者。
const oversizePaddingBytes = 2 << 20

// oversizeBody 构造一个远超任何 body 上限的 JSON 请求体。
//
// 撑大的字段名用未知的 padding,而不是把某个已知字段拉长:逐字段上限
// (custom404Html 512KB、目标 URL 数量/长度)会先行返回 400,把 body 上限
// 这条路径盖住。未知字段被 encoding/json 静默忽略,只有 body 上限能拦住它。
func oversizeBody() map[string]any {
	return map[string]any{"padding": strings.Repeat("x", oversizePaddingBytes)}
}

// assertOversizeRejected 断言超限请求被拒:400 + E_VALIDATION
// (仓库内所有 JSON 解析失败统一走这个错误码)。
func assertOversizeRejected(t *testing.T, resp *http.Response) {
	t.Helper()
	assertStatus(t, resp, http.StatusBadRequest)
	if body := decodeBody[httpapi.ErrorBody](t, resp); body.Code != "E_VALIDATION" {
		t.Fatalf("code = %q, want E_VALIDATION", body.Code)
	}
}

// TestOversizeJSONBodyRejectedOnLogin 未认证端点 POST /api/auth/login 也要有上限。
// json.Decoder 在任何认证/校验之前就把整份 body 读进内存,没有上限时匿名请求
// 就能用它打满内存。
func TestOversizeJSONBodyRejectedOnLogin(t *testing.T) {
	env := testutil.Setup(t)
	loggedInTenant(t, env, "alice") // 注册 + 验证,让凭据合法

	// Given 一个远超上限的 JSON body,凭据完全合法
	body := oversizeBody()
	body["email"] = "alice@example.com"
	body["password"] = "password123"

	// When 匿名客户端打登录端点
	resp := newClient(env).post("/api/auth/login", body)

	// Then 400 —— 限的是体积,不是凭据
	assertOversizeRejected(t, resp)
}

// TestOversizeJSONBodyRejectedOnAuthenticatedWrites 已认证写端点同样受限:
// 覆盖 PATCH /api/links/:id、PATCH /api/me/error-pages、POST /api/links/batch-delete。
func TestOversizeJSONBodyRejectedOnAuthenticatedWrites(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)
	tenantID := tenantIDOf(t, c)

	link := createLink(t, c, map[string]any{
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  ids[:1],
	})

	// Given 一个远超上限的 JSON body;When 打 PATCH /api/links/:id
	// (改动本身合法,只是 body 被撑大);Then 400,且短链不被改动。
	t.Run("PATCH /api/links/:id", func(t *testing.T) {
		body := oversizeBody()
		body["status"] = "disabled"
		assertOversizeRejected(t, c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), body))

		got, err := env.Store.GetLinkByID(t.Context(), tenantID, link.ID)
		if err != nil {
			t.Fatalf("GetLinkByID: %v", err)
		}
		if got.Status != "enabled" {
			t.Fatalf("status = %q, want enabled(超限请求不应生效)", got.Status)
		}
	})

	// Given 同上;When 打 PATCH /api/me/error-pages;Then 400,且错误页不被改动。
	t.Run("PATCH /api/me/error-pages", func(t *testing.T) {
		body := oversizeBody()
		body["custom404Html"] = "<h1>404</h1>"
		assertOversizeRejected(t, c.patch("/api/me/error-pages", body))

		resp := c.get("/api/me/error-pages")
		assertStatus(t, resp, http.StatusOK)
		if got := decodeBody[errorPagesBody](t, resp).Custom404HTML; got != "" {
			t.Fatalf("custom404Html = %q, want empty(超限请求不应生效)", got)
		}
	})

	// Given 同上;When 打 POST /api/links/batch-delete;Then 400,且短链不被删除。
	t.Run("POST /api/links/batch-delete", func(t *testing.T) {
		body := oversizeBody()
		body["ids"] = []int64{link.ID}
		assertOversizeRejected(t, c.post("/api/links/batch-delete", body))

		if exists, soft := linkRowState(t, env, link.ID); !exists || soft {
			t.Fatalf("link %d exists=%v soft-deleted=%v, want exists && !soft-deleted", link.ID, exists, soft)
		}
	})
}

// errorPagesBody /api/me/error-pages 响应体。
type errorPagesBody struct {
	Custom404HTML string `json:"custom404Html"`
	Custom429HTML string `json:"custom429Html"`
}

// TestNormalSizeJSONBodyStillAccepted 边界回归:上限不能把正常请求一起打死。
// 逐个端点用合法大小、语义正确的请求确认功能仍在。
func TestNormalSizeJSONBodyStillAccepted(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)

	// 登录:正常大小的合法凭据仍 200
	loginResp := newClient(env).post("/api/auth/login", map[string]string{
		"email": "alice@example.com", "password": "password123",
	})
	assertStatus(t, loginResp, http.StatusOK)
	_ = loginResp.Body.Close()

	link := createLink(t, c, map[string]any{
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  ids[:1],
	})

	// PATCH /api/links/:id:小 body 的合法改动仍 200 且生效
	resp := c.patch("/api/links/"+strconv.FormatInt(link.ID, 10), map[string]any{"status": "disabled"})
	assertStatus(t, resp, http.StatusOK)
	if got := decodeBody[store.Link](t, resp); got.Status != "disabled" {
		t.Fatalf("status = %q, want disabled", got.Status)
	}

	// PATCH /api/me/error-pages:小 body 仍 200 且生效
	resp = c.patch("/api/me/error-pages", map[string]any{"custom404Html": "<h1>404</h1>"})
	assertStatus(t, resp, http.StatusOK)
	if got := decodeBody[errorPagesBody](t, resp).Custom404HTML; got != "<h1>404</h1>" {
		t.Fatalf("custom404Html = %q, want <h1>404</h1>", got)
	}

	// POST /api/links/batch-delete:小 body 仍 200
	resp = c.post("/api/links/batch-delete", map[string]any{"ids": []int64{link.ID}})
	assertStatus(t, resp, http.StatusOK)
	if got := decodeBody[batchResult](t, resp).Deleted; got != 1 {
		t.Fatalf("deleted = %d, want 1", got)
	}
}
