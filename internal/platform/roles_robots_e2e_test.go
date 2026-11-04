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

// TestRobotPolicyE2E Robot 作为 user 主体：创建后由管理员直接授权，
// 删除 Robot 时清理其策略并吊销 Token。
func TestRobotPolicyE2E(t *testing.T) {
	rt := newTestRuntime(t)
	tok := adminToken(t, rt)
	suffix := time.Now().UnixNano()

	clusterID := fmt.Sprintf("it-robot-cluster-%d", suffix)
	robotName := fmt.Sprintf("it-robot-%d", suffix)

	if _, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": clusterID, "name": "robot e2e"}, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("create cluster: %+v", ar)
	}
	t.Cleanup(func() {
		_, _ = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+clusterID, nil, tok)
	})

	// 签发 Robot
	_, ar := call(t, rt, http.MethodPost, "/api/v1/robots",
		map[string]string{"name": robotName, "cluster_id": clusterID}, tok)
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

	// 为机器人主体创建直接策略（独立主体：subject 为机器人 ID）
	_, ar = call(t, rt, http.MethodPost, "/api/v1/policies", map[string]any{
		"subject_type": "robot",
		"subject":      created.Robot.ID,
		"cluster":      clusterID,
		"rules": []map[string]any{
			{"namespace": "", "resource": "*", "namespaced": false, "verbs": []string{"*"}},
		},
	}, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("create robot policy: %+v", ar)
	}

	// Robot Token 身份
	_, me := call(t, rt, http.MethodGet, "/auth/me", nil, authInfo.Token)
	if me.Status != errcode.CodeSuccess {
		t.Fatalf("robot /auth/me: %+v", me)
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(me.Data, &claims); err != nil {
		t.Fatalf("decode robot claims: %v", err)
	}
	if claims.Sub != created.Robot.Sub {
		t.Fatalf("robot sub: want %s, got %s", created.Robot.Sub, claims.Sub)
	}

	// 重复名称 → 已存在
	_, ar = call(t, rt, http.MethodPost, "/api/v1/robots",
		map[string]string{"name": robotName, "cluster_id": clusterID}, tok)
	if ar.Status != errcode.CodeRobotExists {
		t.Fatalf("duplicate robot: want %d, got %+v", errcode.CodeRobotExists, ar)
	}

	// 删除 Robot → Token 401，策略被清理
	if _, ar = call(t, rt, http.MethodDelete, "/api/v1/robots/"+created.Robot.ID, nil, tok); ar.Status != errcode.CodeSuccess {
		t.Fatalf("delete robot: %+v", ar)
	}
	if resp := callRaw(t, rt, http.MethodGet, "/auth/me", authInfo.Token); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked robot token: want 401, got %d", resp.StatusCode)
	}
	_, ar = call(t, rt, http.MethodGet, "/api/v1/policies", nil, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("list policies: %+v", ar)
	}
	var policies []modules.PolicyDTO
	if err := json.Unmarshal(ar.Data, &policies); err != nil {
		t.Fatalf("decode policies: %v", err)
	}
	for _, policy := range policies {
		if policy.Sub == created.Robot.Sub {
			t.Fatalf("robot policy should be removed after delete: %+v", policy)
		}
	}
}
