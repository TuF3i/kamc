package platform_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	httpstream "k8s.io/apimachinery/pkg/util/httpstream"
	rcconstants "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	streaminghttpstream "k8s.io/streaming/pkg/httpstream"
	spdy "k8s.io/streaming/pkg/httpstream/spdy"
)

// execCapture 假 apiserver 收到的请求特征
type execCapture struct {
	ImpersonateUser string
	Authorization   string
	Path            string
}

// newFakeExecServer 假 kube-apiserver 的 exec 端点：SPDY 服务端，
// 把 STDIN 回环到 STDOUT（模式来自 apiserver 官方 streamtranslator 测试），
// 并捕获 Impersonation 头供断言。
func newFakeExecServer(t *testing.T, captured *execCapture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t.Logf("[fake-apiserver] %s %s impersonate=%q", req.Method, req.URL.Path, req.Header.Get("Impersonate-User"))
		captured.ImpersonateUser = req.Header.Get("Impersonate-User")
		captured.Authorization = req.Header.Get("Authorization")
		captured.Path = req.URL.Path

		if _, err := streaminghttpstream.Handshake(req, w, []string{rcconstants.StreamProtocolV4Name}); err != nil {
			http.Error(w, "handshake: "+err.Error(), http.StatusBadRequest)
			return
		}
		upgrader := spdy.NewResponseUpgrader()
		streamCh := make(chan streaminghttpstream.Stream, 8)
		conn := upgrader.UpgradeResponse(w, req, func(s streaminghttpstream.Stream, _ <-chan struct{}) error {
			streamCh <- s
			return nil
		})
		defer conn.Close()

		// 收齐 error/stdin/stdout 三条流；error 流保持打开（客户端据此判结束）
		var stdin, stdout streaminghttpstream.Stream
		got := 0
		for got < 3 {
			s := <-streamCh
			switch s.Headers().Get(v1.StreamType) {
			case v1.StreamTypeError:
				got++
			case v1.StreamTypeStdin:
				stdin = s
				got++
			case v1.StreamTypeStdout:
				stdout = s
				got++
			}
		}
		if _, err := io.Copy(stdout, stdin); err != nil {
			t.Logf("fake exec loopback: %v", err)
		}
		_ = stdout.Close()
	}))
}

// TestExecEndToEnd exec 全链路（§9）：client-go 回退执行器（WS 优先，等价
// kubectl 1.31+ 默认行为）→ 平台 StreamTranslator（WS→SPDY）→ 隧道 →
// Agent 101 直通 → 假 apiserver（SPDY）。断言数据回环、Impersonation 注入、
// 用户凭证不透传、无策略用户 403。
func TestExecEndToEnd(t *testing.T) {
	dsn := getenvOrSkip(t)
	_ = dsn

	captured := &execCapture{}
	upstream := newFakeExecServer(t, captured)
	defer upstream.Close()

	trt, rt, srv := setupTunnelPlatform(t, upstream.URL)
	defer func() { _ = trt.ReleaseRuntime() }()
	defer func() { _ = rt.ReleaseRuntime() }()

	clusterID := fmt.Sprintf("e2e-exec-%d", time.Now().UnixNano())
	adminTok, agentToken := seedClusterAndPolicies(t, rt, srv, clusterID)

	// Agent 接入（显式注入上游）
	startAgent(t, clusterID, "ws://"+srv.Listener.Addr().String()+"/tunnel", agentToken, upstream.URL)
	waitForClusterStatus(t, rt, adminTok, clusterID, "online", 5*time.Second)

	// client-go 回退执行器：WS 优先（kubectl 1.31+ 默认），失败回退 SPDY
	restCfg := &rest.Config{Host: srv.URL, BearerToken: adminTok}
	execURL := srv.URL + "/proxy/" + clusterID + "/api/v1/namespaces/dev/pods/pod-1/exec?stdin=1&stdout=1"
	execURLParsed, err := url.Parse(execURL)
	if err != nil {
		t.Fatalf("parse exec url: %v", err)
	}
	wsExec, err := remotecommand.NewWebSocketExecutor(restCfg, "POST", execURL)
	if err != nil {
		t.Fatalf("ws executor: %v", err)
	}
	spdyExec, err := remotecommand.NewSPDYExecutor(restCfg, "POST", execURLParsed)
	if err != nil {
		t.Fatalf("spdy executor: %v", err)
	}
	exec, err := remotecommand.NewFallbackExecutor(wsExec, spdyExec, func(err error) bool {
		return httpstream.IsUpgradeFailure(err)
	})
	if err != nil {
		t.Fatalf("fallback executor: %v", err)
	}

	payload := "hello-through-tunnel"
	var stdout bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  strings.NewReader(payload),
		Stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("exec stream: %v", err)
	}
	if stdout.String() != payload {
		t.Fatalf("stdout mismatch: got %q want %q", stdout.String(), payload)
	}
	if captured.Path != "/api/v1/namespaces/dev/pods/pod-1/exec" {
		t.Fatalf("upstream path: %q (FIX-6 strip)", captured.Path)
	}
	if captured.ImpersonateUser != "admin-test" {
		t.Fatalf("Impersonate-User: got %q, want admin-test (transport 层注入)", captured.ImpersonateUser)
	}
	if captured.Authorization != "" {
		t.Fatalf("Authorization leaked: %q", captured.Authorization)
	}

	// 无策略用户 exec → 403（Casbin 第一层）
	viewerTok := devToken(t, rt, "viewer-exec", nil)
	req, _ := http.NewRequest(http.MethodPost, execURL, nil)
	req.Header.Set("Authorization", "Bearer "+viewerTok)
	rec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer exec: want 403, got %d", rec.Code)
	}
}
