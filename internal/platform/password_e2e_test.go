package platform_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TuF3i/kamc/infrastructure/postgres"
	"github.com/TuF3i/kamc/internal/platform"
	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// resetSysadminPassword 清空 sysadmin 的库内密码哈希（测试库跨运行持久，
// 改密 e2e 需要从“内置管理员仅有环境变量密码”的初始态开始）。
func resetSysadminPassword(t *testing.T, dsn string) {
	t.Helper()
	db, err := postgres.NewPostgresClient(postgres.WithDSN(dsn))
	if err != nil {
		t.Fatalf("connect for reset: %v", err)
	}
	if err := db.Exec("UPDATE platform_users SET password_hash = '' WHERE username = 'sysadmin'").Error; err != nil {
		t.Fatalf("reset sysadmin password: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// postJSONRaw 发送 JSON 请求并返回原始响应（不解析信封——401 纯文本用）。
func postJSONRaw(t *testing.T, rt *platform.Runtime, path string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(rec, req)
	return rec.Result()
}

// passwordLogin 密码登录：返回 (httpStatus, token, envelope)；401 纯文本时 envelope 为 nil。
func passwordLogin(t *testing.T, rt *platform.Runtime, username, password string) (int, string, *apiResponse) {
	t.Helper()
	resp := postJSONRaw(t, rt, "/auth/password-login", map[string]string{
		"username": username, "password": password,
	})
	var ar apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return resp.StatusCode, "", nil
	}
	token := ""
	if ar.Data != nil {
		var issued struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(ar.Data, &issued); err == nil {
			token = issued.Token
		}
	}
	return resp.StatusCode, token, &ar
}

// TestPasswordChangeE2E 修改密码全链路（TEST_PG_DSN 门控）：
// 环境密码登录 → 错旧密码 40012 → 改密成功并吊销旧 JWT →
// 旧密码 401/新密码登录成功 → 登录同步不冲掉哈希 → 可用新密码再改。
func TestPasswordChangeE2E(t *testing.T) {
	dsn := getenvOrSkip(t)
	resetSysadminPassword(t, dsn)
	// 测试库跨用例共享：结束后恢复“仅有环境变量密码”的初始态，
	// 不影响 TestPasswordLoginE2E 等按默认密码登录的用例。
	t.Cleanup(func() { resetSysadminPassword(t, dsn) })
	rt := newTestRuntime(t)

	// 环境变量密码（默认 sysadmin/sysadmin123）登录
	status, token, ar := passwordLogin(t, rt, "sysadmin", "sysadmin123")
	if status != http.StatusOK || ar == nil || ar.Status != errcode.CodeSuccess || token == "" {
		t.Fatalf("login with env password: http=%d body=%+v", status, ar)
	}

	// profile.password_set：内置管理员（未落哈希）由 handler 标记为可改
	_, me := call(t, rt, http.MethodGet, "/api/v1/profile", nil, token)
	if me.Status != errcode.CodeSuccess {
		t.Fatalf("profile: %+v", me)
	}
	var profile struct {
		Username    string `json:"username"`
		PasswordSet bool   `json:"password_set"`
	}
	if err := json.Unmarshal(me.Data, &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if !profile.PasswordSet {
		t.Fatalf("builtin admin profile should mark password_set, got %+v", profile)
	}

	// 错误的当前密码
	resp, ar := call(t, rt, http.MethodPost, "/api/v1/profile/password",
		map[string]string{"old_password": "wrong-old", "new_password": "brand-new-pass"}, token)
	if ar.Status != errcode.CodePasswordWrong {
		t.Fatalf("wrong old password: http=%d body=%+v", resp.StatusCode, ar)
	}

	// 新密码过短：校验在吊销之前完成，失败后当前 Token 仍然有效
	resp, ar = call(t, rt, http.MethodPost, "/api/v1/profile/password",
		map[string]string{"old_password": "sysadmin123", "new_password": "short"}, token)
	if ar.Status != errcode.CodeInvalidParam {
		t.Fatalf("short new password: http=%d body=%+v", resp.StatusCode, ar)
	}
	if resp := callRaw(t, rt, http.MethodGet, "/auth/me", token); resp.StatusCode != http.StatusOK {
		t.Fatalf("token must survive invalid new password: want 200, got %d", resp.StatusCode)
	}

	// 正确修改：成功后旧 JWT 立即吊销
	resp, ar = call(t, rt, http.MethodPost, "/api/v1/profile/password",
		map[string]string{"old_password": "sysadmin123", "new_password": "brand-new-pass"}, token)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("change password: http=%d body=%+v", resp.StatusCode, ar)
	}
	if resp := callRaw(t, rt, http.MethodGet, "/auth/me", token); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old token after password change: want 401, got %d", resp.StatusCode)
	}

	// 旧密码（环境变量）登录必须失败——库内哈希已生效
	status, _, _ = passwordLogin(t, rt, "sysadmin", "sysadmin123")
	if status != http.StatusUnauthorized {
		t.Fatalf("login with env password after change: want 401, got %d", status)
	}

	// 新密码登录成功
	status, fresh, ar := passwordLogin(t, rt, "sysadmin", "brand-new-pass")
	if status != http.StatusOK || ar == nil || ar.Status != errcode.CodeSuccess || fresh == "" {
		t.Fatalf("login with new password: http=%d body=%+v", status, ar)
	}

	// 登录同步（UpsertUser）不冲掉密码哈希：用新密码作为旧密码再次修改
	resp, ar = call(t, rt, http.MethodPost, "/api/v1/profile/password",
		map[string]string{"old_password": "brand-new-pass", "new_password": "third-pass-9"}, fresh)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("change again after relogin: http=%d body=%+v", resp.StatusCode, ar)
	}
	if resp := callRaw(t, rt, http.MethodGet, "/auth/me", fresh); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token after second change: want 401, got %d", resp.StatusCode)
	}
	status, _, _ = passwordLogin(t, rt, "sysadmin", "third-pass-9")
	if status != http.StatusOK {
		t.Fatalf("login with third password: want 200, got %d", status)
	}
}

// TestPasswordChangeNotSet 未设置密码的本地用户（管理员手建后经 dev-token 登录）
// 修改密码应返回“未设置密码”，而不是误报密码错误。
func TestPasswordChangeNotSet(t *testing.T) {
	rt := newTestRuntime(t)
	admin := adminToken(t, rt)

	user := fmt.Sprintf("e2e-pwd-%d", time.Now().UnixNano())
	resp, ar := call(t, rt, http.MethodPost, "/api/v1/users",
		map[string]any{"username": user, "nickname": "pwd", "groups": []string{}}, admin)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("create user: http=%d body=%+v", resp.StatusCode, ar)
	}

	userToken := devToken(t, rt, user, nil)
	resp, ar = call(t, rt, http.MethodPost, "/api/v1/profile/password",
		map[string]string{"old_password": "whatever-1", "new_password": "brand-new-pass"}, userToken)
	if ar.Status != errcode.CodePasswordNotSet {
		t.Fatalf("password not set: http=%d body=%+v", resp.StatusCode, ar)
	}
}
