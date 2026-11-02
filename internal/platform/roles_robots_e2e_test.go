package platform_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// TestRolesAndRobotsE2E 角色管理与 Robot 长期 kubeconfig：
//  1. 创建受管角色
//  2. 签发 Robot（自动建立 role 绑定），返回 kubeconfig 可解析
//  3. Robot Token 可通过 /auth/me（sub=robot:...）
//  4. 角色仍被 Robot 绑定时不可删除
//  5. 吊销 Robot 后 Token 401、角色可删除
func TestRolesAndRobotsE2E(t *testing.T) {
	rt := newTestRuntime(t)
	tok := adminToken(t, rt)
	suffix := time.Now().UnixNano()

	clusterID := fmt.Sprintf("it-robot-cluster-%d", suffix)
	roleName := fmt.Sprintf("role:it-robot-%d", suffix)
	robotName := fmt.Sprintf("it-robot-%d", suffix)

	// 集群
	if _, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": clusterID, "name": "robot e2e"}, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("create cluster: %+v", ar)
	}
	t.Cleanup(func() {
		_, _ = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+clusterID, nil, tok)
	})

	// 角色
	if _, ar := call(t, rt, http.MethodPost, "/api/v1/roles",
		map[string]string{"name": roleName, "desc": "robot e2e"}, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("create role: %+v", ar)
	}

	// 签发 Robot
	_, ar := call(t, rt, http.MethodPost, "/api/v1/robots",
		map[string]string{"name": robotName, "cluster_id": clusterID, "role": roleName}, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("create robot: %+v", ar)
	}
	var created struct {
		Robot      modules.Robot `json:"robot"`
		Kubeconfig string        `json:"kubeconfig"`
	}
	if err := json.Unmarshal(ar.Data, &created); err != nil {
		t.Fatalf("decode create robot: %v", err)
	}
	if created.Robot.ID == "" || created.Robot.Sub == "" || created.Kubeconfig == "" {
		t.Fatalf("incomplete robot response: %+v", created.Robot)
	}

	cfg, err := clientcmd.Load([]byte(created.Kubeconfig))
	if err != nil {
		t.Fatalf("parse robot kubeconfig: %v", err)
	}
	authInfo := cfg.AuthInfos[created.Robot.Sub]
	if authInfo == nil || authInfo.Token == "" {
		t.Fatalf("robot kubeconfig missing token for %s", created.Robot.Sub)
	}
	if got := cfg.Clusters[clusterID]; got == nil || got.Server == "" {
		t.Fatalf("robot kubeconfig missing cluster server")
	}

	// Robot Token 身份
	_, ar = call(t, rt, http.MethodGet, "/auth/me", nil, authInfo.Token)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("robot /auth/me: %+v", ar)
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(ar.Data, &claims); err != nil {
		t.Fatalf("decode robot claims: %v", err)
	}
	if claims.Sub != created.Robot.Sub {
		t.Fatalf("robot sub: want %s, got %s", created.Robot.Sub, claims.Sub)
	}

	// 角色仍被 Robot 绑定 → 拒绝删除
	_, ar = call(t, rt, http.MethodDelete, "/api/v1/roles/"+roleName, nil, tok)
	if ar.Status != errcode.CodeRoleInUse {
		t.Fatalf("delete role in use: want %d, got %+v", errcode.CodeRoleInUse, ar)
	}

	// 重复名称 → 已存在
	_, ar = call(t, rt, http.MethodPost, "/api/v1/robots",
		map[string]string{"name": robotName, "cluster_id": clusterID, "role": roleName}, tok)
	if ar.Status != errcode.CodeRobotExists {
		t.Fatalf("duplicate robot: want %d, got %+v", errcode.CodeRobotExists, ar)
	}

	// 吊销 Robot → Token 401，绑定解除，角色可删
	if _, ar = call(t, rt, http.MethodDelete, "/api/v1/robots/"+created.Robot.ID, nil, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("delete robot: %+v", ar)
	}
	if resp := callRaw(t, rt, http.MethodGet, "/auth/me", authInfo.Token); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked robot token: want 401, got %d", resp.StatusCode)
	}
	if _, ar = call(t, rt, http.MethodDelete, "/api/v1/roles/"+roleName, nil, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("delete role after robot: %+v", ar)
	}
}
