package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// IssueToken POST /api/v1/clusters/{id}/tokens
func (h *Handler) IssueToken(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "id")
	var req modules.IssueTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	data, rsp := h.svc.IssueToken(r.Context(), clusterID, &req)
	errcode.GenFinalResponse(w, data, rsp)
}

// ListTokens GET /api/v1/clusters/{id}/tokens（仅元数据，无明文/哈希）
func (h *Handler) ListTokens(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.ListTokens(r.Context(), chi.URLParam(r, "id"))
	errcode.GenFinalResponse(w, data, rsp)
}

// RevokeToken DELETE /api/v1/clusters/{id}/tokens/{kind}
func (h *Handler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	rsp := h.svc.RevokeToken(r.Context(), chi.URLParam(r, "id"), modules.TokenKind(chi.URLParam(r, "kind")))
	errcode.GenFinalResponse(w, nil, rsp)
}
