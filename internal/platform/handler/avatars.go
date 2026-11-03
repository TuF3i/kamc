package handler

// 头像上传与签名 URL 服务：
//   - POST /api/v1/users/{username}/avatar  上传图片（管理面，raw image body）
//   - GET  /avatars/{uid}?token=…           公开读取（token 为 HMAC 签名，
//     供 <img src> 直接展示——img 无法携带 Authorization 头）
//
// 签名密钥来自 AM_AVATAR_SECRET；未配置时由部署配置派生（databasURL/JWT
// issuer），保证重启后已入库的头像 URL 仍然有效。

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// maxAvatarSize 头像文件上限（2MB）
const maxAvatarSize = 2 << 20

// avatarExt 允许的头像类型 → 扩展名
var avatarExt = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// signAvatar 生成头像读取 token（HMAC(uid)）
func (h *Handler) signAvatar(uid string) string {
	mac := hmac.New(sha256.New, h.avatarSecret)
	mac.Write([]byte(uid))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyAvatarToken 恒定时间校验
func (h *Handler) verifyAvatarToken(uid, token string) bool {
	if uid == "" || token == "" || len(h.avatarSecret) == 0 {
		return false
	}
	want := h.signAvatar(uid)
	return hmac.Equal([]byte(token), []byte(want))
}

// UploadAvatar POST /api/v1/users/{username}/avatar —— raw 图片体上传，
// 保存为 <avatarDir>/<uid>，并写入签名 URL 到用户 avatar_url。
func (h *Handler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	if h.avatarDir == "" || len(h.avatarSecret) == 0 {
		http.Error(w, "avatar storage disabled", http.StatusServiceUnavailable)
		return
	}
	username := chi.URLParam(r, "username")
	user, rsp := h.svc.GetUser(r.Context(), username)
	if rsp != nil {
		errcode.GenFinalResponse(w, nil, rsp)
		return
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if _, ok := avatarExt[ct]; !ok {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxAvatarSize+1))
	if err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(err))
		return
	}
	if len(body) == 0 || len(body) > maxAvatarSize {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	// 以文件头复核类型（防 Content-Type 伪造）
	if detected := http.DetectContentType(body); !strings.HasPrefix(detected, "image/") {
		errcode.GenFinalResponse(w, nil, errcode.ErrInvalidParam)
		return
	}
	path := filepath.Join(h.avatarDir, user.UID)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		errcode.GenFinalResponse(w, nil, errcode.ServerInternal(fmt.Errorf("write avatar: %w", err)))
		return
	}
	url := "/avatars/" + user.UID + "?token=" + h.signAvatar(user.UID)
	if rsp := h.svc.SetAvatarURL(r.Context(), username, url); rsp != nil {
		errcode.GenFinalResponse(w, nil, rsp)
		return
	}
	h.auditPlatform(r, "avatar-upload", username, "", "/api/v1/users/"+username+"/avatar", true, ct)
	errcode.GenFinalResponse(w, map[string]string{"avatar_url": url}, nil)
}

// ServeAvatar GET /avatars/{uid}?token=… —— 公开读取（签名即凭证）
func (h *Handler) ServeAvatar(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	if !h.verifyAvatarToken(uid, r.URL.Query().Get("token")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	data, err := os.ReadFile(filepath.Join(h.avatarDir, uid))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}

// avatarRoutes 上传（管理面）与公开读取
func avatarRoutes(r chi.Router, admin chi.Router, h *Handler) {
	r.Get("/avatars/{uid}", h.ServeAvatar)
	admin.Post("/users/{username}/avatar", h.UploadAvatar)
}
