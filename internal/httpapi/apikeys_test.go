package httpapi_test

// 09 — public-api 黑盒测试:API Key 生成/列表/吊销;公开 REST 短链 CRUD;租户隔离。

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"cloak/internal/store"
	"cloak/internal/testutil"
)

type apiKeyResp struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	Key       string `json:"key,omitempty"`
}

func createAPIKey(t *testing.T, c *testClient, name string) apiKeyResp {
	t.Helper()
	resp := c.post("/api/api-keys", map[string]string{"name": name})
	assertStatus(t, resp, http.StatusCreated)
	k := decodeBody[apiKeyResp](t, resp)
	if k.Key == "" || !strings.HasPrefix(k.Key, "cloak_") {
		t.Fatalf("created key missing plaintext prefix: %+v", k)
	}
	return k
}

func TestAPIKeyLifecycle(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")

	k := createAPIKey(t, c, "ci")

	// 列表不含明文 key
	resp := c.get("/api/api-keys")
	assertStatus(t, resp, http.StatusOK)
	keys := decodeBody[[]apiKeyResp](t, resp)
	if len(keys) != 1 || keys[0].Key != "" {
		t.Fatalf("keys = %+v, want 1 without plaintext", keys)
	}

	// 用 key 访问公开 API
	keyClient := newBearerClient(env, k.Key)
	resp = keyClient.get("/api/v1/links")
	assertStatus(t, resp, http.StatusOK)

	// 吊销后拒绝
	resp = c.del("/api/api-keys/" + strconv.FormatInt(k.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = keyClient.get("/api/v1/links")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}

// bearerClient 以 Authorization: Bearer 访问公开 API。
type bearerClient struct {
	env *testutil.Env
	key string
}

func newBearerClient(env *testutil.Env, key string) *bearerClient {
	return &bearerClient{env: env, key: key}
}

func (b *bearerClient) do(method, path string, body any) *http.Response {
	var rd io.Reader
	if body != nil {
		// 简化:body 直接 JSON 字符串
		if s, ok := body.(string); ok {
			rd = strings.NewReader(s)
		}
	}
	req, err := http.NewRequest(method, b.env.Server.URL+path, rd)
	if err != nil {
		panic(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+b.key)
	resp, err := b.env.Server.Client().Do(req)
	if err != nil {
		panic(err)
	}
	return resp
}

func (b *bearerClient) get(path string) *http.Response { return b.do(http.MethodGet, path, nil) }
func (b *bearerClient) post(path, body string) *http.Response {
	return b.do(http.MethodPost, path, body)
}
func (b *bearerClient) del(path string) *http.Response { return b.do(http.MethodDelete, path, nil) }

func TestV1LinksCRUD(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	k := createAPIKey(t, c, "ci")
	ids := domainIDsOf(t, c)
	api := newBearerClient(env, k.Key)

	// 创建
	body := `{"code":"vqabc","targetUrl":"https://api.example.com/x","domainIds":[` +
		strconv.FormatInt(ids[0], 10) + `]}`
	resp := api.post("/api/v1/links", body)
	assertStatus(t, resp, http.StatusCreated)
	link := decodeBody[store.Link](t, resp)
	if link.Code != "vqabc" {
		t.Fatalf("code = %s", link.Code)
	}

	// 列表
	resp = api.get("/api/v1/links")
	assertStatus(t, resp, http.StatusOK)
	list := decodeBody[struct {
		Items []*store.Link `json:"items"`
		Total int           `json:"total"`
	}](t, resp)
	if list.Total != 1 {
		t.Fatalf("total = %d, want 1", list.Total)
	}

	// 按 id 查询
	resp = api.get("/api/v1/links/" + strconv.FormatInt(link.ID, 10))
	assertStatus(t, resp, http.StatusOK)

	// 删除(逻辑)
	resp = api.del("/api/v1/links/" + strconv.FormatInt(link.ID, 10))
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
	resp = api.get("/api/v1/links/" + strconv.FormatInt(link.ID, 10))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestV1TenantIsolation(t *testing.T) {
	env := testutil.Setup(t)
	a := loggedInTenant(t, env, "alice")
	b := loggedInTenant(t, env, "bob")

	ka := createAPIKey(t, a, "a")
	kb := createAPIKey(t, b, "b")

	idsA := domainIDsOf(t, a)
	linkA := createLink(t, a, map[string]any{"targetUrl": "https://a.example.com", "domainIds": idsA[:1]})

	apiB := newBearerClient(env, kb.Key)
	// B 的 key 看不到 A 的短链
	resp := apiB.get("/api/v1/links/" + strconv.FormatInt(linkA.ID, 10))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
	resp = apiB.get("/api/v1/links")
	assertStatus(t, resp, http.StatusOK)
	list := decodeBody[struct {
		Items []*store.Link `json:"items"`
		Total int           `json:"total"`
	}](t, resp)
	if list.Total != 0 {
		t.Fatalf("tenant B sees %d links, want 0", list.Total)
	}
	// B 不能删除 A 的短链
	resp = apiB.del("/api/v1/links/" + strconv.FormatInt(linkA.ID, 10))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
	// A 的 key 仍可用
	resp = newBearerClient(env, ka.Key).get("/api/v1/links")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

func TestV1InvalidKey(t *testing.T) {
	env := testutil.Setup(t)
	// 无 header
	resp := newClient(env).get("/api/v1/links")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
	// 伪造 key
	resp = newBearerClient(env, "cloak_forgedkey123").get("/api/v1/links")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}

func TestV1APIKeyRejectedWhenTenantBanned(t *testing.T) {
	env := testutil.Setup(t)
	admin := superadminClient(t, env)
	c := loggedInTenant(t, env, "alice")
	k := createAPIKey(t, c, "ci")
	api := newBearerClient(env, k.Key)

	// 封禁前可用
	resp := api.get("/api/v1/links")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 超管封禁租户后,API Key 立即失效(与 currentTenant 一致)
	resp = admin.patch("/api/admin/tenants/"+strconv.FormatInt(tenantIDOf(t, c), 10), map[string]any{"status": "banned"})
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	resp = api.get("/api/v1/links")
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
}
