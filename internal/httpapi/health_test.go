package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ruanbw/janus/internal/config"
	"github.com/ruanbw/janus/internal/metrics"
)

// 本文件覆盖新增的探活/指标端点(/livez、/readyz、/metrics)以及 /healthz 的兼容性。
//
// 全部用 httptest 直连 httpapi.New,不连库:探活接口的正确性恰恰不能依赖一个
// "能连上的 Postgres" 才能被验证,否则它和它要检查的东西一起坏掉。
//
// Store 保持 nil 是刻意的 —— New 对 nil Store 是安全的(方法值绑定不会解引用),
// 而探活 handler 一旦顺手查了库,这里就会直接 panic,这正是要拦住的回归。

// newProbeAPI 只装配探活/指标相关的依赖,其余字段留零值。
func newProbeAPI(t *testing.T, ready func(context.Context) error, reg *metrics.Registry, metricsToken string) http.Handler {
	t.Helper()
	return New(Deps{
		Cfg:          config.Config{PlatformDomain: "janus.test"},
		Ready:        ready,
		Metrics:      reg,
		MetricsToken: metricsToken,
	})
}

// doGet 发一个 GET;remoteAddr 为空时保留 httptest 的默认对端。
func doGet(h http.Handler, target, remoteAddr, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// statusField 解出响应体里 {"status": "..."} 的取值。
func statusField(t *testing.T, body []byte) string {
	t.Helper()
	var v struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("响应体不是期望的 JSON: %q (%v)", body, err)
	}
	return v.Status
}

// TestLivezIgnoresReadinessFailure 固化 livez 与 DB 完全解耦。
//
// 为什么重要:liveness 只回答"进程还活着吗"。若它跟着 DB 一起失败,编排器会在
// Postgres 抖动时重启一个完全健康的进程 —— 重启解决不了 DB 故障,只会让本已
// 过载的实例在启动风暴里再挂一遍。/livez 必须无条件 200。
func TestLivezIgnoresReadinessFailure(t *testing.T) {
	h := newProbeAPI(t, func(context.Context) error {
		return errors.New("db down")
	}, nil, "")

	w := doGet(h, "/livez", "127.0.0.1:1234", "")
	if w.Code != http.StatusOK {
		t.Fatalf("/livez 必须恒为 200(与 DB 解耦), 得到 %d: %s", w.Code, w.Body.String())
	}
	if got := statusField(t, w.Body.Bytes()); got != "ok" {
		t.Fatalf(`/livez 响应体应为 {"status":"ok"}, 得到 status=%q`, got)
	}
}

// TestReadyzOK 固化就绪探针的成功路径。
//
// 为什么重要:这个 200 是编排器(compose/k8s)把流量切给实例的唯一依据。
func TestReadyzOK(t *testing.T) {
	h := newProbeAPI(t, func(context.Context) error { return nil }, nil, "")

	w := doGet(h, "/readyz", "127.0.0.1:1234", "")
	if w.Code != http.StatusOK {
		t.Fatalf("Ready 返回 nil 时 /readyz 应为 200, 得到 %d: %s", w.Code, w.Body.String())
	}
	if got := statusField(t, w.Body.Bytes()); got != "ok" {
		t.Fatalf(`/readyz 成功响应体应为 {"status":"ok"}, 得到 status=%q`, got)
	}
}

// TestReadyzHidesFailureDetail 固化"503 但不回显细节"。
//
// 为什么重要:/readyz 是公网可达的(k8s/LB 直接扫,常被顺手暴露)。把 err.Error()
// 原样回显,等于把 DSN、内网主机名、账号、SQL 语句免费送给任何能访问这个端口的人 ——
// 一个运维用的健康检查变成信息泄露面。要排障就看服务端日志,不要把细节塞进响应体。
func TestReadyzHidesFailureDetail(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		leaked []string
	}{
		{
			name:   "Given Ready 返回含 DSN 的错误, When 请求 /readyz, Then 503 且不回显 DSN",
			err:    errors.New("postgres://janus:s3cr3t@db.internal:5432/janus: dial tcp 10.0.0.5:5432: connect: connection refused"),
			leaked: []string{"s3cr3t", "db.internal", "postgres://", "10.0.0.5", "connection refused"},
		},
		{
			name:   "Given Ready 返回含 SQL 语句的错误, When 请求 /readyz, Then 503 且不回显 SQL",
			err:    errors.New(`ERROR: relation "tenants" does not exist (SQLSTATE 42P01)`),
			leaked: []string{"tenants", "SQLSTATE", "42P01", "relation"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newProbeAPI(t, func(context.Context) error { return tc.err }, nil, "")

			w := doGet(h, "/readyz", "127.0.0.1:1234", "")
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("Ready 返回错误时 /readyz 应为 503(否则 compose 永远 healthy), 得到 %d: %s", w.Code, w.Body.String())
			}
			body := w.Body.String()
			for _, secret := range tc.leaked {
				if strings.Contains(body, secret) {
					t.Fatalf("503 响应体回显了故障细节 %q, 完整响应: %q", secret, body)
				}
			}
			// 响应体只允许有状态字段:多一个字段就多一条泄露渠道。
			var payload map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatalf("503 响应体不是 JSON: %q (%v)", body, err)
			}
			if len(payload) != 1 || payload["status"] != "unavailable" {
				t.Fatalf(`503 响应体应恰为 {"status":"unavailable"}, 得到 %v`, payload)
			}
		})
	}
}

// TestMetricsEndpointAuth 用一张表覆盖 /metrics 的全部鉴权维度。
//
// 为什么重要:/metrics 的文本里有租户数、域名状态、规则 fail-open 次数 —— 足以
// 反推出业务规模与故障。它必须是 fail-closed 的:没配 token 就一律 403(而不是
// "没配就放行"),token 错就 403,来源不是内网也 403。
func TestMetricsEndpointAuth(t *testing.T) {
	const token = "metrics-token-for-test"

	// 每个用例一份独立注册表:注入的 janus_test_total 是"这个响应确实来自
	// 我们注入的 Registry"的证据,而不是碰巧吐了点别的东西。
	newRegistry := func(t *testing.T) *metrics.Registry {
		t.Helper()
		reg := metrics.New()
		reg.Counter("janus_test_total", "测试注入的指标。").Inc()
		return reg
	}

	cases := []struct {
		name            string
		configuredToken string
		authorization   string
		target          string
		remoteAddr      string
		wantStatus      int
	}{
		{
			name:       "Given 未配置 MetricsToken, When 匿名请求 /metrics, Then 403(fail-closed)",
			target:     "/metrics",
			remoteAddr: "127.0.0.1:1234",
			wantStatus: http.StatusForbidden,
		},
		{
			name:            "Given 已配置 token, When 不带任何凭据, Then 403",
			configuredToken: token,
			target:          "/metrics",
			remoteAddr:      "127.0.0.1:1234",
			wantStatus:      http.StatusForbidden,
		},
		{
			name:            "Given 已配置 token, When Bearer 错误, Then 403",
			configuredToken: token,
			authorization:   "Bearer wrong-token",
			target:          "/metrics",
			remoteAddr:      "127.0.0.1:1234",
			wantStatus:      http.StatusForbidden,
		},
		{
			name:            "Given token 正确, When 来源是公网地址, Then 403",
			configuredToken: token,
			authorization:   "Bearer " + token,
			target:          "/metrics",
			remoteAddr:      "203.0.113.7:1234",
			wantStatus:      http.StatusForbidden,
		},
		{
			name:            "Given token 正确, When Bearer 头携带, Then 200 且是 0.0.4 文本格式",
			configuredToken: token,
			authorization:   "Bearer " + token,
			target:          "/metrics",
			remoteAddr:      "127.0.0.1:1234",
			wantStatus:      http.StatusOK,
		},
		{
			name:            "Given token 正确, When 走 ?token= 查询参数, Then 200(与非浏览器抓取方式一致)",
			configuredToken: token,
			target:          "/metrics?token=" + token,
			remoteAddr:      "127.0.0.1:1234",
			wantStatus:      http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newProbeAPI(t, func(context.Context) error { return nil }, newRegistry(t), tc.configuredToken)

			w := doGet(h, tc.target, tc.remoteAddr, tc.authorization)
			if w.Code != tc.wantStatus {
				t.Fatalf("状态码不符: 期望 %d, 得到 %d: %s", tc.wantStatus, w.Code, w.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				return
			}

			// 0.0.4 是 Prometheus 抓取端唯一保证兼容的文本版本;少了 version 参数,
			// 部分抓取端会按最老的 0.0.1 解析,标签/直方图语法直接不认。
			contentType := w.Header().Get("Content-Type")
			if !strings.Contains(contentType, "text/plain; version=0.0.4") {
				t.Fatalf("Content-Type 应含 text/plain; version=0.0.4, 得到 %q", contentType)
			}
			// 注入的指标名必须出现:否则 200 可能只是回了一句空文本,
			// handler 根本没走 Deps.Metrics。
			if !strings.Contains(w.Body.String(), "janus_test_total") {
				t.Fatalf("响应体未包含注入的指标 janus_test_total, 说明 handler 没导出 Deps.Metrics:\n%s", w.Body.String())
			}
		})
	}
}

// TestHealthzStaysOK 固化对既有 /healthz 的兼容。
//
// 为什么重要:docker-compose 的 healthcheck 打的是 /healthz(见 docker-compose.yml)。
// 新增 /livez /readyz 时顺手把它删掉或改成依赖 DB,会让既有部署整体 unhealthy ——
// 这是一个只会在上线后才暴露的破坏性变更。
func TestHealthzStaysOK(t *testing.T) {
	h := newProbeAPI(t, func(context.Context) error {
		return errors.New("db down")
	}, nil, "")

	w := doGet(h, "/healthz", "127.0.0.1:1234", "")
	if w.Code != http.StatusOK {
		t.Fatalf("/healthz 必须保持 200(compose healthcheck 兼容), 得到 %d: %s", w.Code, w.Body.String())
	}
	if got := statusField(t, w.Body.Bytes()); got != "ok" {
		t.Fatalf(`/healthz 响应体应为 {"status":"ok"}, 得到 status=%q`, got)
	}
}
