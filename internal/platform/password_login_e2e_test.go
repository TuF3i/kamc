package platform_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// TestPasswordLoginE2E 内置账号密码登录：默认 sysadmin/sysadmin123。
func TestPasswordLoginE2E(t *testing.T) {
	rt := newTestRuntime(t)

	resp, ar := call(t, rt, http.MethodPost, "/auth/password-login",
		map[string]string{"username": "sysadmin", "password": "sysadmin123"})
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("password login: http=%d body=%+v", resp.StatusCode, ar)
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(ar.Data, &issued); err != nil || issued.Token == "" {
		t.Fatalf("decode issued token: %v", err)
	}

	_, me := call(t, rt, http.MethodGet, "/auth/me", nil, issued.Token)
	if me.Status != errcode.CodeSuccess {
		t.Fatalf("password token /auth/me: %+v", me)
	}
	var claims struct {
		Sub    string   `json:"sub"`
		Groups []string `json:"groups"`
	}
	if err := json.Unmarshal(me.Data, &claims); err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	if claims.Sub != "sysadmin" {
		t.Fatalf("sub: want sysadmin, got %q", claims.Sub)
	}
	if len(claims.Groups) != 1 || claims.Groups[0] != "am:admin" {
		t.Fatalf("groups: want [am:admin], got %v", claims.Groups)
	}

	// 错误密码：纯文本 401
	raw := []byte(`{"username":"sysadmin","password":"wrong"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/password-login", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: want 401, got %d", rec.Code)
	}
}

// TestPasswordLoginDisabled 关闭内置账号开关后：内置账号登录 401
// （端点保留——LDAP fallback 共用该路径；未配置 LDAP 时也统一 401 防枚举）。
func TestPasswordLoginDisabled(t *testing.T) {
	t.Setenv("AM_PASSWORD_LOGIN", "0")
	rt := newTestRuntime(t)
	req := httptest.NewRequest(http.MethodPost, "/auth/password-login",
		strings.NewReader(`{"username":"sysadmin","password":"sysadmin123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("disabled builtin login: want 401, got %d", rec.Code)
	}
}
