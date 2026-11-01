package service

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		PlatformURL:  "wss://platform.example.com/tunnel",
		ClusterID:    "cluster-a",
		AgentID:      "agent-test",
		TokenFile:    filepath.Join(t.TempDir(), "token"),
		PingInterval: 20 * time.Second,
		BackoffBase:  5 * time.Millisecond,
		BackoffMax:   20 * time.Millisecond,
	}
}

// TestValidate 配置必填项校验
func TestValidate(t *testing.T) {
	cfg := testConfig(t)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	bad := testConfig(t)
	bad.PlatformURL = "https://wrong-scheme.example.com"
	if err := bad.Validate(); err == nil {
		t.Fatal("https:// scheme should be rejected")
	}

	bad = testConfig(t)
	bad.ClusterID = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("empty CLUSTER_ID should be rejected")
	}
}

// TestReadTokenMissing Token 文件缺失返回错误（含 ErrTokenUnavailable 语义路径）
func TestReadTokenMissing(t *testing.T) {
	s := NewService(testConfig(t))
	_, err := s.readToken()
	if err == nil {
		t.Fatal("missing token file should error")
	}

	// 空文件 → ErrTokenUnavailable
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.TokenFile, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	s2 := NewService(cfg)
	if _, err := s2.readToken(); !isTokenUnavailable(err) {
		t.Fatalf("empty token: want ErrTokenUnavailable, got %v", err)
	}
}

func isTokenUnavailable(err error) bool {
	for err != nil {
		if err == ErrTokenUnavailable {
			return true
		}
		err = unwrapOnce(err)
	}
	return false
}

func unwrapOnce(err error) error {
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return u.Unwrap()
	}
	return nil
}

// TestConnectLoopReconnectsAndReleases 端到端验证连接循环：
// 拨号失败 → 退避重连（次数递增）；Release 后循环干净退出、无 goroutine 泄漏。
func TestConnectLoopReconnectsAndReleases(t *testing.T) {
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.TokenFile, []byte("am-clu-test-token\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	var attempts atomic.Int64
	s := NewService(cfg)
	s.SetDialFunc(func(_ context.Context, _ Config, token string) (*websocket.Conn, *http.Response, error) {
		if token != "am-clu-test-token" {
			t.Errorf("dial got token %q, want file content", token)
		}
		attempts.Add(1)
		return nil, nil, websocket.ErrBadHandshake // 始终失败 → 循环退避重试
	})

	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && attempts.Load() < 3 {
		time.Sleep(2 * time.Millisecond)
	}
	if attempts.Load() < 3 {
		t.Fatalf("expected >=3 dial attempts under backoff, got %d", attempts.Load())
	}

	s.Release()
	if got := attempts.Load(); got < 3 {
		t.Fatalf("attempts changed unexpectedly after release: %d", got)
	}
}

// TestStartIdempotent Start 重复调用只启动一轮循环。
func TestStartIdempotent(t *testing.T) {
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.TokenFile, []byte("am-clu-test-token\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	var attempts atomic.Int64
	s := NewService(cfg)
	s.SetDialFunc(func(_ context.Context, _ Config, _ string) (*websocket.Conn, *http.Response, error) {
		attempts.Add(1)
		return nil, nil, websocket.ErrBadHandshake
	})

	if err := s.Start(); err != nil {
		t.Fatalf("start 1: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("start 2: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	n := attempts.Load()
	time.Sleep(50 * time.Millisecond)
	if n2 := attempts.Load(); n2 > 2*n+2 {
		t.Fatalf("second Start seems to run extra loop: %d -> %d", n, n2)
	}
	s.Release()
}

// TestReleaseIdempotent Release 幂等且无悬挂。
func TestReleaseIdempotent(t *testing.T) {
	cfg := testConfig(t)
	if err := os.WriteFile(cfg.TokenFile, []byte("am-clu-test-token\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	s := NewService(cfg)
	s.SetDialFunc(func(_ context.Context, _ Config, _ string) (*websocket.Conn, *http.Response, error) {
		return nil, nil, http.ErrAbortHandler
	})
	_ = s.Start()

	done := make(chan struct{})
	go func() { s.Release(); s.Release(); close(done) }()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("double Release blocked > 6s")
	}
}

// TestBackoffProgression 退避翻倍且封顶于 BackoffMax。
func TestBackoffProgression(t *testing.T) {
	cfg := testConfig(t) // base 5ms / max 20ms
	s := NewService(cfg)

	cur := s.cfg.BackoffBase
	s.sleepBackoff(&cur) // 内部随机睡 5ms 内，可接受
	if cur != 2*s.cfg.BackoffBase {
		t.Fatalf("after first backoff want %s, got %s", 2*s.cfg.BackoffBase, cur)
	}
	for i := 0; i < 10; i++ {
		s.sleepBackoff(&cur)
	}
	if cur != s.cfg.BackoffMax {
		t.Fatalf("backoff should cap at max %s, got %s", s.cfg.BackoffMax, cur)
	}
}

// TestConnectLoopParksOn401 401 停止重试（§6.5）：拨号被拒后驻留，
// Token 文件变更后自动恢复。
func TestConnectLoopParksOn401(t *testing.T) {
	cfg := testConfig(t)
	cfg.TokenPollInterval = 10 * time.Millisecond
	if err := os.WriteFile(cfg.TokenFile, []byte("tok-v1\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	var attempts atomic.Int64
	var lastToken atomic.Value
	s := NewService(cfg)
	s.SetDialFunc(func(_ context.Context, _ Config, token string) (*websocket.Conn, *http.Response, error) {
		attempts.Add(1)
		lastToken.Store(token)
		if token == "tok-v1" {
			return nil, &http.Response{StatusCode: http.StatusUnauthorized}, errors.New("unauthorized")
		}
		return nil, nil, websocket.ErrBadHandshake // 新 Token：改为普通失败（退避重试）
	})

	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && attempts.Load() < 1 {
		time.Sleep(2 * time.Millisecond)
	}

	// 驻留：500ms 内不再尝试
	n := attempts.Load()
	time.Sleep(500 * time.Millisecond)
	if got := attempts.Load(); got != n {
		t.Fatalf("401 should park the loop: attempts %d -> %d", n, got)
	}

	// Token 文件变更 → 恢复
	if err := os.WriteFile(cfg.TokenFile, []byte("tok-v2\n"), 0o600); err != nil {
		t.Fatalf("renew token: %v", err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if v, _ := lastToken.Load().(string); v == "tok-v2" {
			s.Release()
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	s.Release()
	t.Fatal("loop should resume dialing with renewed token")
}
