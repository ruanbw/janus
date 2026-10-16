package httpapi_test

// 登录链路的「数据库故障 ≠ 账号不存在」黑盒测试。
//
// 用一条"连得上但查不到业务表"的连接串(search_path 指向不存在的 schema)制造
// 真实的 DB 故障:查询报 42P01,而不是"零行"。修复前这种故障会被当成
// "账号不存在"回 401 —— 表现成"全员密码错误",把一次数据库故障伪装成用户问题。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"janus/internal/db"
	"janus/internal/geo"
	"janus/internal/httpapi"
	"janus/internal/store"
	"janus/internal/testutil"
)

// serveWithStore 复用测试配置,但换成调用方指定的 store(可注入故障)。
func serveWithStore(t *testing.T, env *testutil.Env, st *store.Store) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Store:     st,
		Mailer:    failingMailer{},
		Cfg:       env.Cfg,
		GeoLookup: geo.Disabled,
	}))
	t.Cleanup(srv.Close)
	return srv
}

// brokenStore 返回一个连得上库、但查询业务表必然报错的 store。
func brokenStore(t *testing.T) *store.Store {
	t.Helper()
	// search_path 指向不存在的 schema:连接与 SELECT version() 正常,业务表查询 42P01。
	gdb, err := db.OpenGORM(testutil.TestDatabaseURL + "&search_path=janus_no_such_schema")
	if err != nil {
		t.Fatalf("open gorm with broken search_path: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return store.New(gdb)
}

// TestLoginDBErrorReturns500 数据库故障必须 500(并记日志),不能被当成账号不存在。
func TestLoginDBErrorReturns500(t *testing.T) {
	env := testutil.Setup(t)
	logs := captureLogs(t)
	srv := serveWithStore(t, env, brokenStore(t))

	resp := postJSON(t, srv.URL+"/api/auth/login", map[string]string{
		"email": "whoever@example.com", "password": "password123",
	})
	assertStatus(t, resp, http.StatusInternalServerError)
	_ = resp.Body.Close()

	if !strings.Contains(logs(), "查询租户失败") {
		t.Fatalf("数据库故障未记日志: %s", logs())
	}
}

// TestTokenDBErrorReturns500 token 端点与登录共用凭据校验,同样不得把故障当 401。
func TestTokenDBErrorReturns500(t *testing.T) {
	env := testutil.Setup(t)
	srv := serveWithStore(t, env, brokenStore(t))

	resp := postJSON(t, srv.URL+"/api/auth/token", map[string]string{
		"email": "whoever@example.com", "password": "password123",
	})
	assertStatus(t, resp, http.StatusInternalServerError)
	_ = resp.Body.Close()
}
