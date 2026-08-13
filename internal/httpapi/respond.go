package httpapi

import (
	"encoding/json"
	"net/http"
)

// errorBody 统一错误响应:{code, message, details?}(见 API 契约)。
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeErrDetails(w, status, code, msg, nil)
}

func writeErrDetails(w http.ResponseWriter, status int, code, msg string, details any) {
	writeJSON(w, status, ErrorBody{Code: code, Message: msg, Details: details})
}

func writeNoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

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
