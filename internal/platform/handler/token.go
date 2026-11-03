package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// AgentRegister POST /api/v1/agent/register —— Agent 首次注册：
// 携带注册 Token（Bearer）换长期 Token。一次性语义由 service 层保证；
// 与管理 API 分离，后续 JWT 中间件不覆盖本端点（注册 Token 即凭证）。
func (h *Handler) AgentRegister(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	issued, err := h.svc.ExchangeRegisterToken(r.Context(), token)
	if err != nil {
		// 不区分失效原因，防枚举
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	errcode.GenFinalResponse(w, issued, nil)
}

// IssueToken POST /api/v1/clusters/{id}/tokens
func (h *Handler) IssueToken(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "id")
	var req modules.IssueTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	data, rsp := h.svc.IssueToken(r.Context(), clusterID, &req)
	h.auditPlatform(r, "token-issue", "", clusterID, "/api/v1/clusters/"+clusterID+"/tokens", errcode.IsSuccess(rsp), "kind="+string(req.Kind))
	errcode.GenFinalResponse(w, data, rsp)
}

// ListTokens GET /api/v1/clusters/{id}/tokens（仅元数据，无明文/哈希）
func (h *Handler) ListTokens(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.ListTokens(r.Context(), chi.URLParam(r, "id"))
	errcode.GenFinalResponse(w, data, rsp)
}

// RevokeToken DELETE /api/v1/clusters/{id}/tokens/{kind}
func (h *Handler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	clusterID, kind := chi.URLParam(r, "id"), chi.URLParam(r, "kind")
	rsp := h.svc.RevokeToken(r.Context(), clusterID, modules.TokenKind(kind))
	h.auditPlatform(r, "token-revoke", "", clusterID, "/api/v1/clusters/"+clusterID+"/tokens/"+kind, errcode.IsSuccess(rsp), "kind="+kind)
	errcode.GenFinalResponse(w, nil, rsp)
}
