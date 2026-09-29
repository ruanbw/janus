package httpapi_test

// 05 — 短链批量操作黑盒测试:批量逻辑删除(batch-delete)/批量物理删除(batch-purge)、
// 幂等与跨租户静默跳过、请求参数校验(空数组/超上限/非法 id)。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"cloak/internal/testutil"
)

// batchResult 与批量接口响应体 {deleted} 对应。
type batchResult struct {
	Deleted int64 `json:"deleted"`
}

// batchIDs 调批量接口并断言 200,返回 {deleted}。
func batchIDs(t *testing.T, c *testClient, path string, ids []int64) int64 {
	t.Helper()
	resp := c.post(path, map[string]any{"ids": ids})
	assertStatus(t, resp, http.StatusOK)
	return decodeBody[batchResult](t, resp).Deleted
}

// linkRowState 直查库确认短链行是否存在、deleted_at 是否置位。
func linkRowState(t *testing.T, env *testutil.Env, id int64) (exists bool, softDeleted bool) {
	t.Helper()
	var deletedAt *string
	if err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT deleted_at::text FROM links WHERE id = $1`, id).Scan(&deletedAt); err != nil {
		return false, false
	}
	return true, deletedAt != nil
}

// visitCountOf 统计短链的访问记录条数(物理删除后应被级联清除)。
func visitCountOf(t *testing.T, env *testutil.Env, linkID int64) int {
	t.Helper()
	var n int
	if err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT count(*) FROM visits WHERE link_id = $1`, linkID).Scan(&n); err != nil {
		t.Fatalf("count visits of link %d: %v", linkID, err)
	}
	return n
}

// TestBatchDeleteLinks 批量逻辑删除:只删选中的两条,列表只剩一条,
// 且库中两行仍在、deleted_at 已置位(记录与关联保留)。
func TestBatchDeleteLinks(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)

	l1 := createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1]})
	l2 := createLink(t, c, map[string]any{"targetUrls": []string{"https://b.example.com"}, "domainIds": ids[:1]})
	l3 := createLink(t, c, map[string]any{"targetUrls": []string{"https://c.example.com"}, "domainIds": ids[:1]})

	if n := batchIDs(t, c, "/api/links/batch-delete", []int64{l1.ID, l2.ID}); n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}

	// 列表只剩未删除的 l3
	got := listLinks(t, c)
	if len(got) != 1 || got[0].ID != l3.ID {
		t.Fatalf("links after batch delete = %+v, want only %d", got, l3.ID)
	}
	// 详情接口同样 404
	resp := c.get("/api/links/" + strconv.FormatInt(l1.ID, 10))
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()

	// 库中两条仍在,仅 deleted_at 置位(逻辑删除,非物理删除)
	for _, id := range []int64{l1.ID, l2.ID} {
		exists, soft := linkRowState(t, env, id)
		if !exists {
			t.Errorf("link %d row missing: batch-delete must not purge rows", id)
			continue
		}
		if !soft {
			t.Errorf("link %d deleted_at is null after batch-delete", id)
		}
	}
	// 幂等:重复提交同一批 → deleted=0
	if n := batchIDs(t, c, "/api/links/batch-delete", []int64{l1.ID, l2.ID}); n != 0 {
		t.Errorf("deleted on repeat batch-delete = %d, want 0", n)
	}
}

// TestBatchDeleteKeepsVisits 逻辑删除不清理访问明细(与物理删除区分)。
func TestBatchDeleteKeepsVisits(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	link := createLink(t, c, map[string]any{
		"targetUrls": []string{"https://a.example.com"},
		"domainIds":  []int64{localhostDomainID(t, c)},
	})
	resp := redirectGet(t, env, "localhost", "/"+link.Code)
	assertStatus(t, resp, http.StatusFound)
	_ = resp.Body.Close()
	if n := visitCountOf(t, env, link.ID); n != 1 {
		t.Fatalf("visits before batch delete = %d, want 1", n)
	}

	if n := batchIDs(t, c, "/api/links/batch-delete", []int64{link.ID}); n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	if n := visitCountOf(t, env, link.ID); n != 1 {
		t.Errorf("visits after batch-delete = %d, want 1(逻辑删除保留访问明细)", n)
	}
	// 关联与目标行仍在
	var targets, domains int
	if err := env.Pool.QueryRow(testutil.Ctx(),
		`SELECT (SELECT count(*) FROM link_targets WHERE link_id = $1),
		        (SELECT count(*) FROM link_domains WHERE link_id = $1)`, link.ID).
		Scan(&targets, &domains); err != nil {
		t.Fatalf("count link children: %v", err)
	}
	if targets == 0 || domains == 0 {
		t.Errorf("link_targets=%d link_domains=%d after batch-delete, want both kept", targets, domains)
	}
}

// TestBatchPurgeLinks 批量物理删除:连同访问明细与关联一起清除,详情 404。
func TestBatchPurgeLinks(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	addDomain(t, c, "localhost")
	localID := localhostDomainID(t, c)

	l1 := createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": []int64{localID}})
	l2 := createLink(t, c, map[string]any{"targetUrls": []string{"https://b.example.com"}, "domainIds": []int64{localID}})
	keep := createLink(t, c, map[string]any{"targetUrls": []string{"https://c.example.com"}, "domainIds": []int64{localID}})

	// 造访问记录(走公开跳转路径)
	for _, code := range []string{l1.Code, l2.Code} {
		resp := redirectGet(t, env, "localhost", "/"+code)
		assertStatus(t, resp, http.StatusFound)
		_ = resp.Body.Close()
	}
	if n := visitCountOf(t, env, l1.ID); n != 1 {
		t.Fatalf("visits of l1 = %d, want 1", n)
	}

	if n := batchIDs(t, c, "/api/links/batch-purge", []int64{l1.ID, l2.ID}); n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}

	// 列表只剩未 purge 的 keep
	got := listLinks(t, c)
	if len(got) != 1 || got[0].ID != keep.ID {
		t.Fatalf("links after batch-purge = %+v, want only %d", got, keep.ID)
	}
	for _, id := range []int64{l1.ID, l2.ID} {
		resp := c.get("/api/links/" + strconv.FormatInt(id, 10))
		assertStatus(t, resp, http.StatusNotFound)
		_ = resp.Body.Close()
		if exists, _ := linkRowState(t, env, id); exists {
			t.Errorf("link %d row still present after batch-purge", id)
		}
		if n := visitCountOf(t, env, id); n != 0 {
			t.Errorf("visits of purged link %d = %d, want 0", id, n)
		}
		var targets, domains int
		if err := env.Pool.QueryRow(testutil.Ctx(),
			`SELECT (SELECT count(*) FROM link_targets WHERE link_id = $1),
			        (SELECT count(*) FROM link_domains WHERE link_id = $1)`, id).
			Scan(&targets, &domains); err != nil {
			t.Fatalf("count link children: %v", err)
		}
		if targets != 0 || domains != 0 {
			t.Errorf("link %d: link_targets=%d link_domains=%d after purge, want 0/0", id, targets, domains)
		}
	}
	// 幂等:重复 purge 同一批 → deleted=0
	if n := batchIDs(t, c, "/api/links/batch-purge", []int64{l1.ID, l2.ID}); n != 0 {
		t.Errorf("deleted on repeat batch-purge = %d, want 0", n)
	}
}

// TestBatchDeleteSkipsOtherTenant 跨租户 id 静默跳过:只影响本租户短链。
func TestBatchDeleteSkipsOtherTenant(t *testing.T) {
	env := testutil.Setup(t)
	alice := loggedInTenant(t, env, "alice")
	bob := loggedInTenant(t, env, "bob")
	aliceIDs := domainIDsOf(t, alice)
	bobIDs := domainIDsOf(t, bob)

	a1 := createLink(t, alice, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": aliceIDs[:1]})
	a2 := createLink(t, alice, map[string]any{"targetUrls": []string{"https://b.example.com"}, "domainIds": aliceIDs[:1]})
	b1 := createLink(t, bob, map[string]any{"targetUrls": []string{"https://c.example.com"}, "domainIds": bobIDs[:1]})

	// alice 提交含 bob 短链 id 的批次
	if n := batchIDs(t, alice, "/api/links/batch-delete", []int64{a1.ID, b1.ID}); n != 1 {
		t.Fatalf("deleted = %d, want 1(仅本租户短链)", n)
	}
	if len(listLinks(t, alice)) != 1 {
		t.Errorf("alice links = %d, want 1", len(listLinks(t, alice)))
	}
	// bob 的短链不受影响
	bobLinks := listLinks(t, bob)
	if len(bobLinks) != 1 || bobLinks[0].ID != b1.ID {
		t.Fatalf("bob links = %+v, want only %d", bobLinks, b1.ID)
	}
	if exists, soft := linkRowState(t, env, b1.ID); !exists || soft {
		t.Errorf("bob link %d affected by alice batch-delete (exists=%v soft=%v)", b1.ID, exists, soft)
	}

	// 不存在的 id 静默跳过,不报错
	if n := batchIDs(t, alice, "/api/links/batch-delete", []int64{99999999}); n != 0 {
		t.Errorf("deleted for unknown id = %d, want 0", n)
	}
	// purge 同样只删本租户
	if n := batchIDs(t, alice, "/api/links/batch-purge", []int64{a2.ID, b1.ID}); n != 1 {
		t.Fatalf("purge deleted = %d, want 1", n)
	}
	if exists, _ := linkRowState(t, env, b1.ID); !exists {
		t.Fatalf("bob link %d purged by alice", b1.ID)
	}
	if len(listLinks(t, bob)) != 1 {
		t.Errorf("bob links = %d, want 1", len(listLinks(t, bob)))
	}
}

// TestBatchLinkIDsDeduplicated 重复 id 去重:deleted 只计实际受影响行数。
func TestBatchLinkIDsDeduplicated(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)
	l := createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1]})

	if n := batchIDs(t, c, "/api/links/batch-delete", []int64{l.ID, l.ID, l.ID}); n != 1 {
		t.Errorf("deleted with duplicated ids = %d, want 1", n)
	}
}

// TestBatchLinkIDsValidation 校验:空数组/超 200/非正数/非法 JSON → 400。
func TestBatchLinkIDsValidation(t *testing.T) {
	env := testutil.Setup(t)
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)
	l := createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1]})

	tooMany := make([]int64, 0, 201)
	for i := int64(1); i <= 201; i++ {
		tooMany = append(tooMany, i)
	}
	cases := []struct {
		name string
		path string
		body map[string]any
	}{
		{"empty ids", "/api/links/batch-delete", map[string]any{"ids": []int64{}}},
		{"too many ids", "/api/links/batch-delete", map[string]any{"ids": tooMany}},
		{"zero id", "/api/links/batch-delete", map[string]any{"ids": []int64{0}}},
		{"negative id", "/api/links/batch-delete", map[string]any{"ids": []int64{l.ID, -1}}},
		{"empty ids purge", "/api/links/batch-purge", map[string]any{"ids": []int64{}}},
		{"too many ids purge", "/api/links/batch-purge", map[string]any{"ids": tooMany}},
		{"missing ids field", "/api/links/batch-purge", map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := c.post(tc.path, tc.body)
			assertStatus(t, resp, http.StatusBadRequest)
			_ = resp.Body.Close()
		})
	}

	// 校验失败不应误删:短链仍在
	if got := listLinks(t, c); len(got) != 1 || got[0].ID != l.ID {
		t.Fatalf("links after invalid batch requests = %+v, want only %d", got, l.ID)
	}
}

// TestBatchLinksRequireAuth 未登录 401、缺 CSRF 403。
func TestBatchLinksRequireAuth(t *testing.T) {
	env := testutil.Setup(t)

	// 未登录
	anon := newClient(env)
	resp := anon.post("/api/links/batch-delete", map[string]any{"ids": []int64{1}})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()
	resp = anon.post("/api/links/batch-purge", map[string]any{"ids": []int64{1}})
	assertStatus(t, resp, http.StatusUnauthorized)
	_ = resp.Body.Close()

	// 已登录但不带 CSRF 头
	c := loggedInTenant(t, env, "alice")
	ids := domainIDsOf(t, c)
	l := createLink(t, c, map[string]any{"targetUrls": []string{"https://a.example.com"}, "domainIds": ids[:1]})

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]any{"ids": []int64{l.ID}}); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/links/batch-delete", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	noCSRF, err := env.Server.Client().Do(req)
	if err != nil {
		t.Fatalf("POST batch-delete without csrf: %v", err)
	}
	assertStatus(t, noCSRF, http.StatusForbidden)
	_ = noCSRF.Body.Close()

	// 短链未被删除
	if got := listLinks(t, c); len(got) != 1 {
		t.Fatalf("links after csrf-rejected request = %d, want 1", len(got))
	}
}
