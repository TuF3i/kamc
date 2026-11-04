package platform_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TuF3i/kamc/infrastructure/postgres"
	"github.com/TuF3i/kamc/internal/agent"
	"github.com/TuF3i/kamc/internal/platform"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/tunnel"
	"github.com/TuF3i/kamc/internal/tunnel/registry"
)

// noopRegistry e2e 用的空注册表（跨 Pod 路由不在本测试范围）
type noopRegistry struct{}

func (noopRegistry) Report(_ context.Context, _, _, _ string) error  { return nil }
func (noopRegistry) Deregister(_ context.Context, _, _ string) error { return nil }
func (noopRegistry) Lookup(_ context.Context, _ string) ([]registry.Owner, error) {
	return nil, nil
}

// TestTunnelEndToEnd 全链路：签发 Token → Agent 拨号 /tunnel → 隧道接入（集群转
// online）→ /proxy 请求穿隧道到上游（伪造 Impersonate 被剥离、Authorization 不透传）→
// Agent 断开（集群转 offline）。
// 依赖真实数据库（Token 校验落库），TEST_PG_DSN 门控。
func TestTunnelEndToEnd(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set, skip tunnel e2e")
	}
	db, err := postgres.NewPostgresClient(postgres.WithDSN(dsn))
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	// 假 kube-apiserver：回显路径与收到的头
	var gotImpersonateUser, gotImpersonateGroups, gotAuth, gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotImpersonateUser = r.Header.Get("Impersonate-User")
		gotImpersonateGroups = strings.Join(r.Header.Values("Impersonate-Group"), ",")
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"PodList","via":"fake-apiserver"}`))
	}))
	defer upstream.Close()

	// 隧道模块（fake 注册表，无需 Redis）
	trt, err := tunnel.CreateRuntime(
		tunnel.WithRegistry(noopRegistry{}),
		tunnel.WithPodID("e2e-pod"),
		tunnel.WithHeartbeat(time.Hour, time.Hour), // e2e 不测心跳
	)
	if err != nil {
		t.Fatalf("tunnel runtime: %v", err)
	}
	t.Cleanup(func() { _ = trt.ReleaseRuntime() })

	// 平台（含 /tunnel 与 /proxy 路由；代理池走未鉴权开关语义之外的直配）
	t.Setenv("AM_DEV_ISSUE_TOKEN", "1")
	rt, err := platform.CreateRuntime(platform.WithDB(db), platform.WithTunnels(trt))
	if err != nil {
		t.Fatalf("platform runtime: %v", err)
	}
	t.Cleanup(func() { _ = rt.ReleaseRuntime() })

	// 真实监听：Agent 经 WebSocket 拨号
	srv := httptest.NewServer(rt.Engine())
	defer srv.Close()

	// 注册集群 + 签发 Token
	clusterID := fmt.Sprintf("e2e-tunnel-%d", time.Now().UnixNano())
	tok := adminToken(t, rt)
	resp, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": clusterID, "name": "e2e 隧道集群"}, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("create cluster: http=%d body=%+v", resp.StatusCode, ar)
	}
	resp, ar = call(t, rt, http.MethodPost, "/api/v1/clusters/"+clusterID+"/tokens",
		map[string]string{"kind": "longterm"}, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("issue token: %+v", ar)
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(ar.Data, &issued); err != nil {
		t.Fatalf("decode issued: %v", err)
	}

	// 策略种子：管理员直接用户策略（Casbin 数据面鉴权）
	seedUserPolicy(t, rt, tok, "admin-test", clusterID)

	// Agent：真实拨号（默认 dialFn）+ 显式注入上游
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(issued.Token+"\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	upstreamURL, _ := url.Parse(upstream.URL)
	art, err := agent.CreateRuntime(
		agent.WithConfig(agent.Config{
			PlatformURL:  "ws://" + srv.Listener.Addr().String() + "/tunnel",
			ClusterID:    clusterID,
			AgentID:      "e2e-agent-1",
			TokenFile:    tokenFile,
			PingInterval: 100 * time.Millisecond, // 快速驱动 ping/pong 路径
			BackoffBase:  10 * time.Millisecond,
			BackoffMax:   50 * time.Millisecond,
		}),
		agent.WithUpstream(upstreamURL, http.DefaultTransport),
	)
	if err != nil {
		t.Fatalf("agent runtime: %v", err)
	}
	t.Cleanup(func() { _ = art.ReleaseRuntime() })

	// 集群应转 online
	waitForClusterStatus(t, rt, tok, clusterID, "online", 5*time.Second)

	// 无策略用户 → 403（Casbin 第一层拒绝，fail-closed）
	viewerTok := devToken(t, rt, "viewer-user", nil)
	if resp := callRaw(t, rt, http.MethodGet, "/proxy/"+clusterID+"/api/v1/namespaces/dev/pods", viewerTok); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer proxy: want 403, got %d", resp.StatusCode)
	}

	// 经 /proxy 穿隧道请求（带伪造 Impersonate，断言被剥离且不再注入）
	req := httptest.NewRequest(http.MethodGet, "/proxy/"+clusterID+"/api/v1/namespaces/dev/pods", nil)
	req.Header.Set("Impersonate-User", "mallory")
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("proxy request: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "fake-apiserver") {
		t.Fatalf("response should come from upstream: %s", rec.Body.String())
	}
	if gotPath != "/api/v1/namespaces/dev/pods" {
		t.Fatalf("upstream path: got %q (FIX-6 strip)", gotPath)
	}
	// 集群侧不再切分权限：mallory 被剥离，且平台不再注入身份头
	if gotImpersonateUser != "" {
		t.Fatalf("Impersonate-User should be stripped, got %q", gotImpersonateUser)
	}
	if gotImpersonateGroups != "" {
		t.Fatalf("Impersonate-Group should be stripped, got %q", gotImpersonateGroups)
	}
	if gotAuth != "" {
		t.Fatalf("user Authorization leaked: %q", gotAuth)
	}

	// Agent 断开 → 集群转 offline
	_ = art.ReleaseRuntime()
	waitForClusterStatus(t, rt, tok, clusterID, "offline", 5*time.Second)
}

func waitForClusterStatus(t *testing.T, rt *platform.Runtime, tok, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, ar := call(t, rt, http.MethodGet, "/api/v1/clusters/"+id, nil, tok)
		if ar.Status == errcode.CodeSuccess && strings.Contains(string(ar.Data), `"status":"`+want+`"`) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting cluster %s status=%s", id, want)
}
