package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
	"github.com/TuF3i/kamc/internal/platform/service"
)

const defaultRobotTokenTTL = 365 * 24 * time.Hour

// newRobotID 生成 robot-<16 hex> 标识；随机源失败时退回时间戳（仍保证唯一性可用）。
func newRobotID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("robot-%d", time.Now().UnixNano())
	}
	return "robot-" + hex.EncodeToString(b[:])
}

// ListRobots GET /api/v1/robots
func (h *Handler) ListRobots(w http.ResponseWriter, r *http.Request) {
	data, rsp := h.svc.ListRobots(r.Context())
	errcode.GenFinalResponse(w, data, rsp)
}

// CreateRobot POST /api/v1/robots —— 签发长期 Robot kubeconfig（明文只在本次响应）。
func (h *Handler) CreateRobot(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		http.Error(w, "auth disabled", http.StatusServiceUnavailable)
		return
	}
	var req modules.CreateRobotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	name := strings.TrimSpace(req.Name)
	clusterID := strings.TrimSpace(req.ClusterID)
	if name == "" || clusterID == "" {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}

	id := newRobotID()
	sub := "robot:" + id
	ttl := h.robotTokenTTL
	if ttl <= 0 {
		ttl = defaultRobotTokenTTL
	}
	token, jti, issuedAt, expiresAt, err := h.auth.SignLongLived(sub, "", []string{"robot"}, ttl)
	if err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}

	robot := &modules.Robot{
		ID:        id,
		Name:      name,
		ClusterID: clusterID,
		Sub:       sub,
		JTI:       jti,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}
	dto, rsp := h.svc.CreateRobot(r.Context(), robot)
	if rsp != nil {
		_ = h.auth.RevokeToken(r.Context(), jti, time.Until(expiresAt))
		errcode.GenFinalResponse(w, nil, rsp)
		return
	}

	data, rsp := h.svc.GenerateKubeconfig(r.Context(), service.KubeconfigParams{
		ClusterID:   clusterID,
		Mode:        service.KubeconfigModeStatic,
		ExternalURL: externalURL(r),
		CallerToken: token,
		CallerSub:   sub,
		CAData:      h.caData,
	})
	if rsp != nil {
		_, _ = h.svc.DeleteRobot(r.Context(), id)
		_ = h.auth.RevokeToken(r.Context(), jti, time.Until(expiresAt))
		errcode.GenFinalResponse(w, nil, rsp)
		return
	}

	h.auditPlatform(r, "robot-create", "", clusterID, "/api/v1/robots", true, name)
	errcode.GenFinalResponse(w, map[string]any{
		"robot":      dto,
		"kubeconfig": string(data),
	}, nil)
}

// DeleteRobot DELETE /api/v1/robots/{id} —— 吊销 JWT、清理机器人策略并删除记录。
func (h *Handler) DeleteRobot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	robot, rsp := h.svc.GetRobotModel(r.Context(), id)
	if rsp != nil {
		errcode.GenFinalResponse(w, nil, rsp)
		return
	}
	if h.auth != nil {
		if ttl := time.Until(robot.ExpiresAt); ttl > 0 {
			if err := h.auth.RevokeToken(r.Context(), robot.JTI, ttl); err != nil {
				errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
				return
			}
		}
	}
	if _, rsp := h.svc.DeleteRobot(r.Context(), id); rsp != nil {
		errcode.GenFinalResponse(w, nil, rsp)
		return
	}
	h.auditPlatform(r, "robot-delete", "", robot.ClusterID, "/api/v1/robots/"+id, true, robot.Name)
	errcode.GenFinalResponse(w, nil, nil)
}

// GetRobotKubeconfig GET /api/v1/robots/{id}/kubeconfig —— 重新生成同一 jti 的 static kubeconfig。
func (h *Handler) GetRobotKubeconfig(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		http.Error(w, "auth disabled", http.StatusServiceUnavailable)
		return
	}
	robot, rsp := h.svc.GetRobotModel(r.Context(), chi.URLParam(r, "id"))
	if rsp != nil {
		http.Error(w, rsp.Message, http.StatusBadRequest)
		return
	}
	if !robot.ExpiresAt.After(time.Now()) {
		http.Error(w, errcode.ErrRobotExpired.Message, http.StatusBadRequest)
		return
	}
	token, err := h.auth.SignWithClaims(robot.Sub, "", []string{"robot"}, robot.JTI, robot.IssuedAt, robot.ExpiresAt)
	if err != nil {
		http.Error(w, "sign error", http.StatusInternalServerError)
		return
	}
	data, rsp := h.svc.GenerateKubeconfig(r.Context(), service.KubeconfigParams{
		ClusterID:   robot.ClusterID,
		Mode:        service.KubeconfigModeStatic,
		ExternalURL: externalURL(r),
		CallerToken: token,
		CallerSub:   robot.Sub,
		CAData:      h.caData,
	})
	if rsp != nil {
		http.Error(w, rsp.Message, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition", "attachment; filename=kubeconfig-robot-"+robot.Name+".yaml")
	_, _ = w.Write(data)
}

// robotsRoutes 挂载机器人管理路由（管理面保护组内调用）
func robotsRoutes(admin chi.Router, h *Handler) {
	admin.Get("/robots", h.ListRobots)
	admin.Post("/robots", h.CreateRobot)
	admin.Delete("/robots/{id}", h.DeleteRobot)
	admin.Get("/robots/{id}/kubeconfig", h.GetRobotKubeconfig)
}
