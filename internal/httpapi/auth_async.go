package httpapi

// 认证链路的邮件投递:把发送挪出 HTTP 响应路径。
//
// 为什么必须异步:forgot-password / resend-verification 对外恒返回 202,不泄露
// 邮箱是否存在 —— 而这套"等时化"成立的前提是**响应时间与是否存在无关**。真实
// SMTP 的 DialAndSend 是秒级网络 I/O;留在响应路径上时,"邮箱存在 → 真发信 → 慢"
// 与"邮箱不存在 → 不发信 → 快"的差值是秒级,可被稳定测量,等时化形同虚设。
//
// 为什么不把发送结果返回给调用方:这两个端点的语义本来就是"已受理"(202),
// 注册路径的验证 token 也已按 AUTH2 落在事务里(发信失败可由 resend-verification
// 自助恢复)。因此投递失败不再影响任何对外响应,只记日志。

import "log/slog"

// dispatchMail 在后台 goroutine 里执行一次邮件发送,调用方立即返回。
//
// msg 是失败日志正文(要写清"哪个端点的哪一步失败"),tenantID 用于归因 ——
// 线上"用户收不到邮件"必须能直接从日志定位到租户与调用点,而不是只有一句
// "发送失败"。
func dispatchMail(msg string, tenantID int64, send func() error) {
	go func() {
		if err := send(); err != nil {
			slog.Error(msg, "tenantId", tenantID, "err", err)
		}
	}()
}
