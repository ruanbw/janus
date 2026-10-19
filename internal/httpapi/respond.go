package httpapi

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// errorBody 统一错误响应:{code, message, details?}(见 API 契约)。
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeJSON(c *gin.Context, status int, v any) {
	c.JSON(status, v)
}

func writeErr(c *gin.Context, status int, code, msg string) {
	writeErrDetails(c, status, code, msg, nil)
}

func writeErrDetails(c *gin.Context, status int, code, msg string, details any) {
	// 429 必须带 Retry-After(RFC 6585 §4):否则客户端无从得知何时该重试,只能自己猜。
	// 调用方能用 writeErrRetryAfter 给出按 key 精确计算的等待时间;没有精确值时用
	// 这个兜底窗口,保证**所有** 429(账号退避、域名校验限流等不走通用限流器的点)
	// 都有该头。
	if status == http.StatusTooManyRequests && c.Writer.Header().Get("Retry-After") == "" {
		c.Header("Retry-After", strconv.Itoa(fallbackRetryAfterSec))
	}
	c.AbortWithStatusJSON(status, ErrorBody{Code: code, Message: msg, Details: details})
}

// fallbackRetryAfterSec 是拿不到精确等待时间时用的保守重试秒数(= 默认认证限流窗口)。
// 取整窗口而不是更小的值:宁可让客户端多等,也不要它在 Retry-After 到点后再次撞限流。
const fallbackRetryAfterSec = 60

// writeErrRetryAfter 是 429 的精确重试时长版本:在 writeErr 的统一行为之上,把该
// key 最早命中滑出窗口所需的等待时间写进 Retry-After(向上取整到秒)。
func (a *API) writeErrRetryAfter(c *gin.Context, code, msg string, wait time.Duration) {
	sec := int(math.Ceil(wait.Seconds()))
	if sec < 1 {
		sec = 1
	}
	c.Header("Retry-After", strconv.Itoa(sec))
	writeErr(c, http.StatusTooManyRequests, code, msg)
}

func writeNoContent(c *gin.Context) { c.Status(http.StatusNoContent) }

// 常见错误码(契约统一错误结构)。
const (
	errValidation  = "E_VALIDATION"
	errConflict    = "E_CONFLICT"
	errNotFound    = "E_NOT_FOUND"
	errUnauth      = "E_UNAUTHORIZED"
	errForbidden   = "E_FORBIDDEN"
	errInternal    = "E_INTERNAL"
	errQuota       = "E_QUOTA"
	errDomainQuota = "E_DOMAIN_LIMIT"
	errLinkQuota   = "E_LINK_LIMIT"
	errCSRF        = "E_CSRF"
	errDomainInUse = "E_DOMAIN_IN_USE"
	errRateLimited = "E_RATE_LIMITED"
	// errPasswordChangeRequired 是强制改密闸门的错误码:must_change_password=true 的
	// 租户在改密前访问白名单之外的受保护端点。
	errPasswordChangeRequired = "E_PASSWORD_CHANGE_REQUIRED"
)
