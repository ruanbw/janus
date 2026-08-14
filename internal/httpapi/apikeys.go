package httpapi

// 09 — API Key 管理(后台,会话):生成(明文仅一次)/列表/吊销。

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"strconv"
	"strings"

	"cloak/internal/store"
)

// newAPIKey 生成 "cloak_" 前缀的随机 Key 及其 SHA-256 哈希(库中只存哈希)。
func newAPIKey() (string, string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	key := "cloak_" + base64.RawURLEncoding.EncodeToString(b)
	return key, hashToken(key)
}

func (a *API) handleListAPIKeys(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	keys, err := a.store.ListAPIKeys(c.Request.Context(), t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if keys == nil {
		keys = []*store.APIKey{}
	}
	writeJSON(c, http.StatusOK, keys)
}

type createAPIKeyReq struct {
	Name string `json:"name"`
}

// handleCreateAPIKey 创建 API Key;响应中的 key 明文仅出现一次。
func (a *API) handleCreateAPIKey(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	var req createAPIKeyReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 64 {
		writeErr(c, http.StatusBadRequest, errValidation, "name 必填且不超过 64 字符")
		return
	}
	key, keyHash := newAPIKey()
	k, err := a.store.CreateAPIKey(c.Request.Context(), t.ID, req.Name, keyHash)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	k.Key = key
	writeJSON(c, http.StatusCreated, k)
}

func (a *API) handleDeleteAPIKey(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if err := a.store.RevokeAPIKey(c.Request.Context(), t.ID, id); err != nil {
		writeErr(c, http.StatusNotFound, errNotFound, "api key not found")
		return
	}
	writeNoContent(c)
}
