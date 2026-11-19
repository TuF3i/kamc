package platform_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// TestGroupChangeRevokesToken 权限相关变更必须立即吊销旧 JWT：
// 移出组/删除组后，旧 Token 调用 /auth/me 应 401（不等 1h 过期）。
func TestGroupChangeRevokesToken(t *testing.T) {
	rt := newTestRuntime(t)
	admin := adminToken(t, rt)

	suffix := time.Now().UnixNano()
	group := fmt.Sprintf("e2e-revoke-%d", suffix)
	user := fmt.Sprintf("e2e-bob-%d", suffix)

	resp, ar := call(t, rt, http.MethodPost, "/api/v1/groups",
		map[string]any{"name": group, "desc": "e2e", "is_admin": false}, admin)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("create group: http=%d body=%+v", resp.StatusCode, ar)
	}
	resp, ar = call(t, rt, http.MethodPost, "/api/v1/users",
		map[string]any{"username": user, "nickname": "e2e", "groups": []string{group}}, admin)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("create user: http=%d body=%+v", resp.StatusCode, ar)
	}

	// 移出组：旧 Token 立即失效
	userToken := devToken(t, rt, user, []string{group})
	if _, me := call(t, rt, http.MethodGet, "/auth/me", nil, userToken); me.Status != errcode.CodeSuccess {
		t.Fatalf("token before removal: %+v", me)
	}
	resp, ar = call(t, rt, http.MethodDelete, "/api/v1/groups/"+group+"/members/"+user, nil, admin)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("remove member: http=%d body=%+v", resp.StatusCode, ar)
	}
	if resp := callRaw(t, rt, http.MethodGet, "/auth/me", userToken); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token after removal: want 401, got %d", resp.StatusCode)
	}

	// 加回组 → 新 Token；删除组后旧 Token 也必须立即失效
	resp, ar = call(t, rt, http.MethodPost, "/api/v1/groups/"+group+"/members",
		map[string]any{"username": user}, admin)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("add member: http=%d body=%+v", resp.StatusCode, ar)
	}
	freshToken := devToken(t, rt, user, []string{group})
	if _, me := call(t, rt, http.MethodGet, "/auth/me", nil, freshToken); me.Status != errcode.CodeSuccess {
		t.Fatalf("fresh token before delete group: %+v", me)
	}
	resp, ar = call(t, rt, http.MethodDelete, "/api/v1/groups/"+group, nil, admin)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("delete group: http=%d body=%+v", resp.StatusCode, ar)
	}
	if resp := callRaw(t, rt, http.MethodGet, "/auth/me", freshToken); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token after delete group: want 401, got %d", resp.StatusCode)
	}
}
