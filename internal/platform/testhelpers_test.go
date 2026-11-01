package platform_test

import (
	"encoding/json"
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
)

// 本文件是隧道类 e2e 的共享脚手架：平台+隧道+假监听的装配、
// 集群与策略种子、Agent 接入。（tunnel/exec 两个 e2e 复用。）

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

// seedClusterAndPolicies 建集群 + 签发长期 Token + 管理员角色种子。
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

	_, ar = call(t, rt, "POST", "/api/v1/policies",
		map[string]string{"sub": "role:cluster-admin", "dom": clusterID, "obj": "*", "act": ".*"}, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("seed policy: %+v", ar)
	}
	_, ar = call(t, rt, "POST", "/api/v1/bindings",
		map[string]string{"user": "admin-test", "role": "role:cluster-admin", "dom": clusterID}, tok)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("seed binding: %+v", ar)
	}
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
