package httpapi

// 09 — API Key 管理(后台,会话):生成(明文仅一次)/列表/吊销。

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
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

func (a *API) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	t, _, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	keys, err := a.store.ListAPIKeys(r.Context(), t.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if keys == nil {
		keys = []*store.APIKey{}
	}
	writeJSON(w, http.StatusOK, keys)
}

type createAPIKeyReq struct {
	Name string `json:"name"`
}

// handleCreateAPIKey 创建 API Key;响应中的 key 明文仅出现一次。
func (a *API) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	t, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	if !a.requireCSRF(w, r, sess) {
		return
	}
	var req createAPIKeyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 64 {
		writeErr(w, http.StatusBadRequest, errValidation, "name 必填且不超过 64 字符")
		return
	}
	key, keyHash := newAPIKey()
	k, err := a.store.CreateAPIKey(r.Context(), t.ID, req.Name, keyHash)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	k.Key = key
	writeJSON(w, http.StatusCreated, k)
}

func (a *API) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	t, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	if !a.requireCSRF(w, r, sess) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errValidation, "invalid id")
		return
	}
	if err := a.store.RevokeAPIKey(r.Context(), t.ID, id); err != nil {
		writeErr(w, http.StatusNotFound, errNotFound, "api key not found")
		return
	}
	writeNoContent(w)
}
