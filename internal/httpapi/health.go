package httpapi

// 探活与指标端点:GET /livez、GET /readyz、GET /metrics。
//
// 为什么要把三者分开(而不是继续只有一个 /healthz):
//   - liveness 回答"进程还活着吗"。它跟着 DB 一起失败,编排器就会在 Postgres 抖动时
//     重启一个完全健康的进程 —— 重启解决不了 DB 故障,只会让过载的实例再挂一遍。
//   - readiness 回答"现在能对外服务吗"。它才应该探 DB,失败时从负载均衡里摘掉,
//     而不是杀掉进程。
//   - /healthz 保留原语义(恒定 200)以兼容既有 compose healthcheck,
//     不做破坏性改名。
//
// /metrics 的安全模型与 /internal/caddy/authorize 一致:仅内网 + 共享 token,
// 未配置 token 时 fail-closed。指标里有租户数、域名状态、规则 fail-open 次数,
// 足以反推业务规模与故障,不能是公网匿名可读的。

import (
	"bytes"
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// readinessTimeout 单次探活的独立时限。
//
// 与请求 ctx 分离的必要性不大(探针端总会超时断开),但 2s 能保证一个卡住的
// 连接不会让 readiness 线程堆满:编排器每几秒就打一次,没有上限时会积累等待者。
const readinessTimeout = 2 * time.Second

// handleLivez 恒 200,不碰任何外部依赖。
func (a *API) handleLivez(c *gin.Context) {
	writeJSON(c, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz 探活(默认走 store.Ping),失败返回 503。
//
// 503 的响应体只有一个 status 字段:这个端点常常被顺手暴露在公网(LB/探针直连),
// 回显 err.Error() 等于把 DSN、内网主机名、SQL 语句免费送出去。排障看服务端日志。
func (a *API) handleReadyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), readinessTimeout)
	defer cancel()
	if a.ready != nil {
		if err := a.ready(ctx); err != nil {
			writeJSON(c, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
	}
	writeJSON(c, http.StatusOK, map[string]string{"status": "ok"})
}

// handleMetrics 导出 Prometheus 文本格式指标。
//
// 两道门(顺序无关,都 fail-closed):
//  1. 来源必须是内网/回环 —— 与 /internal/caddy/authorize 同一条边界;
//  2. 共享 token(Authorization: Bearer 或 ?token=),常量时间比较。
//
// token 走查询参数是为非浏览器抓取(Prometheus 的 static_configs 不便加头)留的口子,
// 与 Caddy ask 端点保持一致;查询参数会进访问日志,部署时按需脱敏。
func (a *API) handleMetrics(c *gin.Context) {
	if !isPrivateAddr(c.Request.RemoteAddr) {
		writeErr(c, http.StatusForbidden, errForbidden, "internal endpoint only")
		return
	}
	if !a.metricsTokenOK(c) {
		return
	}
	if a.metrics == nil {
		// New 保证非 nil;这里只防嵌入方手工构造 API。
		c.Data(http.StatusOK, metricsContentType, nil)
		return
	}
	// 先把缓存/队列的瞬时状态同步进注册表,再导出。WriteText 本身是纯读
	// (连续两次抓取必须逐字节相同),所以同步只能发生在这里。
	a.stats.collect()
	var buf bytes.Buffer
	if err := a.metrics.WriteText(&buf); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	c.Data(http.StatusOK, metricsContentType, buf.Bytes())
}

// metricsContentType 是 Prometheus 文本格式 0.0.4。
//
// version 参数不能省:少了它部分抓取端按最老的 0.0.1 解析,标签与直方图语法直接不认。
const metricsContentType = "text/plain; version=0.0.4; charset=utf-8"

// metricsTokenOK 校验 /metrics 的共享 token;不通过时直接写回 403。
func (a *API) metricsTokenOK(c *gin.Context) bool {
	want := a.metricsToken
	if want == "" {
		// 未配置即拒绝:没有 token 的 /metrics 是匿名可读的业务情报。
		writeErr(c, http.StatusForbidden, errForbidden, "metrics token not configured")
		return false
	}
	got := c.Query("token")
	if got == "" {
		got = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		writeErr(c, http.StatusForbidden, errForbidden, "invalid metrics token")
		return false
	}
	return true
}
