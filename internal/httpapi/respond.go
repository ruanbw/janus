package httpapi

import (
	"net/http"

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
	c.AbortWithStatusJSON(status, ErrorBody{Code: code, Message: msg, Details: details})
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
)
