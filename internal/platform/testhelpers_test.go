package platform_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TuF3i/kamc/internal/tunnel/registry"

	"github.com/TuF3i/kamc/infrastructure/postgres"
	"github.com/TuF3i/kamc/internal/agent"
	"github.com/TuF3i/kamc/internal/platform"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/tunnel"
)

// 本文件是隧道类 e2e 的共享脚手架：平台+隧道+假监听的装配、
// 集群与策略种子、Agent 接入。（tunnel/exec 两个 e2e 复用。）

// sharedRegistry 多副本共享的内存注册表（跨 Pod e2e 用，替代 Redis）
type sharedRegistry struct {
	mu     sync.Mutex
	owners map[string][]registry.Owner
}

func newSharedRegistry() *sharedRegistry {
	return &sharedRegistry{owners: make(map[string][]registry.Owner)}
}

func (r *sharedRegistry) Report(_ context.Context, clusterID, agentID, pod string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.owners[clusterID]
	for i, o := range list {
		if o.AgentID == agentID {
			list[i].Pod, list[i].Since = pod, time.Now()
			return nil
		}
	}
	r.owners[clusterID] = append(list, registry.Owner{ClusterID: clusterID, AgentID: agentID, Pod: pod, Since: time.Now()})
	return nil
}

func (r *sharedRegistry) Deregister(_ context.Context, clusterID, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.owners[clusterID]
	for i, o := range list {
		if o.AgentID == agentID {
			r.owners[clusterID] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *sharedRegistry) Lookup(_ context.Context, clusterID string) ([]registry.Owner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]registry.Owner(nil), r.owners[clusterID]...), nil
}

// getenvOrSkip 数据库门控：未设置 TEST_PG_DSN 则跳过（数据库不进普通单测）
func getenvOrSkip(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set, skip database e2e")
	}
	return dsn
}

// setupTunnelPlatform 装配 平台(含隧道模块) + 真实 HTTP 监听。
// 返回的句柄由调用方 Release（顺序：agent → platform → tunnel）。
func setupTunnelPlatform(t *testing.T, _ string) (*tunnel.Runtime, *platform.Runtime, *httptest.Server) {
	t.Helper()
	dsn := getenvOrSkip(t)
	t.Setenv("AM_DEV_ISSUE_TOKEN", "1")

	db, err := postgres.NewPostgresClient(postgres.WithDSN(dsn))
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	trt, err := tunnel.CreateRuntime(
		tunnel.WithRegistry(noopRegistry{}),
		tunnel.WithPodID("e2e-pod"),
		tunnel.WithHeartbeat(time.Hour, time.Hour), // e2e 不测心跳
	)
	if err != nil {
		t.Fatalf("tunnel runtime: %v", err)
	}

	rt, err := platform.CreateRuntime(platform.WithDB(db), platform.WithTunnels(trt))
	if err != nil {
		t.Fatalf("platform runtime: %v", err)
	}

	return trt, rt, httptest.NewServer(rt.Engine())
}

// seedUserPolicy 给指定用户主体创建一条 cluster 全资源/全动作策略。
func seedUserPolicy(t *testing.T, rt *platform.Runtime, token, subject, clusterID string) {
	t.Helper()
	_, ar := call(t, rt, http.MethodPost, "/api/v1/policies", map[string]any{
		"subject_type": "user",
		"subject":      subject,
		"cluster":      clusterID,
		"rules": []map[string]any{
			{"namespace": "", "resource": "*", "namespaced": false, "verbs": []string{"*"}},
		},
	}, token)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("seed policy for %s: %+v", subject, ar)
	}
}

// seedClusterAndPolicies 建集群 + 签发长期 Token + 管理员直接策略种子。
// 返回 (管理员 JWT, 长期 Token 明文供 Agent 接入)。
func seedClusterAndPolicies(t *testing.T, rt *platform.Runtime, _ *httptest.Server, clusterID string) (string, string) {
	t.Helper()
	tok := adminToken(t, rt)

	resp, ar := call(t, rt, "POST", "/api/v1/clusters",
		map[string]string{"id": clusterID, "name": "e2e"}, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("create cluster: http=%d body=%+v", resp.StatusCode, ar)
	}
	resp, ar = call(t, rt, "POST", "/api/v1/clusters/"+clusterID+"/tokens",
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

	seedUserPolicy(t, rt, tok, "admin-test", clusterID)
	return tok, issued.Token
}

// startAgent 启动接入指定平台的 Agent（真实拨号 + 显式上游）。
// platformURL 为完整地址（ws:// 或 wss://…/tunnel）；tls 面向自签证书时跳过校验。
func startAgent(t *testing.T, clusterID, platformURL, token, upstreamURL string) {
	t.Helper()
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	upstream, err := url.Parse(upstreamURL)
	if err != nil {
		t.Fatalf("parse upstream: %v", err)
	}
	art, err := agent.CreateRuntime(
		agent.WithConfig(agent.Config{
			PlatformURL:  platformURL,
			ClusterID:    clusterID,
			AgentID:      "e2e-agent-" + clusterID,
			TokenFile:    tokenFile,
			PingInterval: 100 * time.Millisecond,
			BackoffBase:  10 * time.Millisecond,
			BackoffMax:   50 * time.Millisecond,
			InsecureTLS:  strings.HasPrefix(platformURL, "wss://"),
		}),
		agent.WithUpstream(upstream, http.DefaultTransport),
	)
	if err != nil {
		t.Fatalf("agent runtime: %v", err)
	}
	t.Cleanup(func() { _ = art.ReleaseRuntime() })
}
