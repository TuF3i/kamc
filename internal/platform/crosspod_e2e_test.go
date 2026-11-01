package platform_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TuF3i/kamc/infrastructure/postgres"
	"github.com/TuF3i/kamc/internal/platform"
	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/tunnel"
	"github.com/TuF3i/kamc/internal/tunnel/service"
)

// TestCrossPodForwardingE2E 跨 Pod 转发全链路（§7.2.4，HA 核心）：
// Agent 隧道落在 Pod-A，用户请求落在 Pod-B → B 查注册表发现 owner=A →
// peer RELAY 拼接到 A 的隧道 → 上游。同时验证故障语义（owner 注销后 503）。
func TestCrossPodForwardingE2E(t *testing.T) {
	dsn := getenvOrSkip(t)

	// 假 apiserver：捕获身份头，回 PodList
	var gotImpersonateUser string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotImpersonateUser = r.Header.Get("Impersonate-User")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"PodList","apiVersion":"v1","metadata":{"resourceVersion":"1"},"items":[]}`))
	}))
	defer upstream.Close()

	db, err := postgres.NewPostgresClient(postgres.WithDSN(dsn))
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	t.Setenv("AM_DEV_ISSUE_TOKEN", "1")

	// 两副本共享的内存注册表（替代 Redis）
	reg := newSharedRegistry()

	// ── Pod-A：持有隧道 + peer 服务端 ──
	trtA, err := tunnel.CreateRuntime(
		tunnel.WithRegistry(reg),
		tunnel.WithPodID("pod-a"),
		tunnel.WithHeartbeat(time.Hour, time.Hour),
		tunnel.WithPeers(service.PeerOptions{Token: "e2e-peer", ListenAddr: "127.0.0.1:0"}),
	)
	if err != nil {
		t.Fatalf("tunnel A: %v", err)
	}
	t.Cleanup(func() { _ = trtA.ReleaseRuntime() })
	rtA, err := platform.CreateRuntime(platform.WithDB(db), platform.WithTunnels(trtA))
	if err != nil {
		t.Fatalf("platform A: %v", err)
	}
	t.Cleanup(func() { _ = rtA.ReleaseRuntime() })
	srvA := httptest.NewServer(rtA.Engine())
	defer srvA.Close()

	// 种子（经 A；此时 B 尚未创建，其 Casbin 缓存将直接加载到种子数据）
	clusterID := fmt.Sprintf("e2e-crosspod-%d", time.Now().UnixNano())
	tokA := adminToken(t, rtA)
	if _, ar := call(t, rtA, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": clusterID, "name": "crosspod"}, tokA); ar.Status != errcode.CodeSuccess {
		t.Fatalf("create cluster: %+v", ar)
	}
	if _, ar := call(t, rtA, http.MethodPost, "/api/v1/policies",
		map[string]string{"sub": "role:cluster-admin", "dom": clusterID, "obj": "*", "act": ".*"}, tokA); ar.Status != errcode.CodeSuccess {
		t.Fatalf("seed policy: %+v", ar)
	}
	if _, ar := call(t, rtA, http.MethodPost, "/api/v1/bindings",
		map[string]string{"user": "admin-test", "role": "role:cluster-admin", "dom": clusterID}, tokA); ar.Status != errcode.CodeSuccess {
		t.Fatalf("seed binding: %+v", ar)
	}

	// ── Pod-B：无本地隧道，peer 指向 A（Casbin 此时加载种子策略）──
	trtB, err := tunnel.CreateRuntime(
		tunnel.WithRegistry(reg),
		tunnel.WithPodID("pod-b"),
		tunnel.WithHeartbeat(time.Hour, time.Hour),
		tunnel.WithPeers(service.PeerOptions{
			Token:     "e2e-peer",
			Addresses: map[string]string{"pod-a": trtA.PeerAddr().String()},
		}),
	)
	if err != nil {
		t.Fatalf("tunnel B: %v", err)
	}
	t.Cleanup(func() { _ = trtB.ReleaseRuntime() })
	rtB, err := platform.CreateRuntime(platform.WithDB(db), platform.WithTunnels(trtB))
	if err != nil {
		t.Fatalf("platform B: %v", err)
	}
	t.Cleanup(func() { _ = rtB.ReleaseRuntime() })
	srvB := httptest.NewServer(rtB.Engine())
	defer srvB.Close()

	// Agent 只接 A
	_, ar := call(t, rtA, http.MethodPost, "/api/v1/clusters/"+clusterID+"/tokens",
		map[string]string{"kind": "longterm"}, tokA)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("issue token: %+v", ar)
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(ar.Data, &issued); err != nil {
		t.Fatalf("decode issued: %v", err)
	}
	agentToken := issued.Token
	startAgent(t, clusterID, "ws://"+srvA.Listener.Addr().String()+"/tunnel", agentToken, upstream.URL)
	waitForClusterStatus(t, rtA, tokA, clusterID, "online", 5*time.Second)

	// B 自己签发的同身份 JWT（各副本密钥独立）
	tokB := devToken(t, rtB, "admin-test", []string{"am:admin"})

	// ── 关键断言：请求落 B，隧道在 A → RELAY 转发成功 ──
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/proxy/"+clusterID+"/api/v1/namespaces/dev/pods", nil)
	req.Header.Set("Authorization", "Bearer "+tokB)
	rtB.Engine().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("crosspod proxy: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PodList") {
		t.Fatalf("response should come from upstream: %s", rec.Body.String())
	}
	if gotImpersonateUser != "admin-test" {
		t.Fatalf("Impersonate-User: %q (中继必须字节透明)", gotImpersonateUser)
	}

	// 对照：A 本地直连同样可用
	recA := httptest.NewRecorder()
	reqA, _ := http.NewRequest(http.MethodGet, "/proxy/"+clusterID+"/api/v1/namespaces/dev/pods", nil)
	reqA.Header.Set("Authorization", "Bearer "+tokA)
	rtA.Engine().ServeHTTP(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Fatalf("direct proxy on A: %d", recA.Code)
	}

	// ── 故障语义 ──
	// 1) 注册表注销 owner 后，跨副本新拨号失败（既有空闲连接按 transport
	//    池语义继续可用，属正常行为而非路由失效）
	//    （Agent ID 与 testhelpers.startAgent 的命名一致）
	if err := reg.Deregister(context.Background(), clusterID, "e2e-agent-"+clusterID); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if _, err := trtB.Service.Open(clusterID); err == nil {
		t.Fatal("cross-pod dial after owner eviction must fail")
	}
	// A 本地拨号不受注册表影响
	if _, err := trtA.Service.Open(clusterID); err != nil {
		t.Fatalf("A local dial after eviction: %v", err)
	}

	// 2) 有授权但无隧道的集群 → HTTP 503（池 ErrorHandler 映射；无授权是 403，
	//    Casbin 先行——fail-closed 语义由鉴权层测试覆盖）
	if _, ar := call(t, rtA, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": clusterID + "-empty", "name": "no agent"}, tokA); ar.Status != errcode.CodeSuccess {
		t.Fatalf("create empty cluster: %+v", ar)
	}
	if _, ar := call(t, rtA, http.MethodPost, "/api/v1/policies",
		map[string]string{"sub": "role:cluster-admin", "dom": clusterID + "-empty", "obj": "*", "act": ".*"}, tokA); ar.Status != errcode.CodeSuccess {
		t.Fatalf("seed empty policy: %+v", ar)
	}
	if _, ar := call(t, rtA, http.MethodPost, "/api/v1/bindings",
		map[string]string{"user": "admin-test", "role": "role:cluster-admin", "dom": clusterID + "-empty"}, tokA); ar.Status != errcode.CodeSuccess {
		t.Fatalf("seed empty binding: %+v", ar)
	}
	// B 的 Casbin 缓存不含新集群策略，经 peer 触发不了——直接走 A 验证（A 亦无隧道）
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/proxy/"+clusterID+"-empty/api/v1/pods", nil)
	req.Header.Set("Authorization", "Bearer "+tokA)
	rtA.Engine().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("cluster without agent: want 503, got %d", rec.Code)
	}
}
