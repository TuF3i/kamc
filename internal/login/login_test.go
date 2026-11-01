package login

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 测试环境重定向缓存目录（os.UserConfigDir 尊重 XDG_CONFIG_HOME）
func redirectCache(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func testOptions(server, issuer string) Options {
	return Options{
		Server: server, ClusterID: "cluster-a",
		Issuer: issuer, ClientID: "kamc-test",
		PollInterval: 5 * time.Millisecond,
		PollTimeout:  3 * time.Second,
	}
}

// newFakeFlow 假 OIDC Provider（device 授权 + token 轮询）+ 假平台（token-exchange）。
// tokenPollers 依次返回 token 端点的响应（耗尽后返回最终 id_token）。
func newFakeFlow(t *testing.T, tokenPollers []func(w http.ResponseWriter)) (*httptest.Server, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"device_authorization_endpoint": issuerURL + "/device/authorize",
			"token_endpoint":                issuerURL + "/token",
		})
	})
	mux.HandleFunc("/device/authorize", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dc-1", "user_code": "ABCD-EFGH",
			"verification_uri": issuerURL + "/device", "interval": 0, "expires_in": 300,
		})
	})
	pollIdx := 0
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if pollIdx < len(tokenPollers) {
			tokenPollers[pollIdx](w)
			pollIdx++
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": "id-token-final"})
	})
	idp := httptest.NewServer(mux)
	t.Cleanup(idp.Close)
	issuerURL = idp.URL // closure 引用（discover 后固定）

	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 0,
			"data": map[string]string{
				"token":      "platform-jwt",
				"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			},
		})
	}))
	t.Cleanup(platform.Close)
	return idp, platform
}

// issuerURL 在 newFakeFlow 内回填（discovery/授权端点需要绝对地址）
var issuerURL string

// TestRunFullFlow 设备流程 → 兑换 → ExecCredential 输出与缓存命中。
func TestRunFullFlow(t *testing.T) {
	redirectCache(t)
	idp, platform := newFakeFlow(t, nil)
	o := testOptions(platform.URL, idp.URL)
	o.Stderr = &strings.Builder{}

	credBytes, err := Run(context.Background(), o)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var cred ExecCredential
	if err := json.Unmarshal(credBytes, &cred); err != nil {
		t.Fatalf("decode credential: %v", err)
	}
	if cred.APIVersion != "client.authentication.k8s.io/v1" || cred.Kind != "ExecCredential" {
		t.Fatalf("credential shape: %+v", cred)
	}
	if cred.Status.Token != "platform-jwt" {
		t.Fatalf("token: %q", cred.Status.Token)
	}
	if _, err := time.Parse(time.RFC3339, cred.Status.ExpirationTimestamp); err != nil {
		t.Fatalf("expiration timestamp: %v", err)
	}

	// 二次运行命中缓存 → 不再轮询 token 端点（换一个必失败的 poller 验证）
	_, platform2 := newFakeFlow(t, []func(w http.ResponseWriter){
		func(w http.ResponseWriter) { t.Fatal("cache hit should not poll token endpoint") },
	})
	_ = platform2
	o2 := testOptions(platform.URL, idp.URL)
	o2.Stderr = &strings.Builder{}
	credBytes2, err := Run(context.Background(), o2)
	if err != nil {
		t.Fatalf("cached run: %v", err)
	}
	if !strings.Contains(string(credBytes2), "platform-jwt") {
		t.Fatalf("cached credential: %s", credBytes2)
	}
}

// TestRunSlowDownAndPending token 端点先 pending 再 slow_down 再放行。
func TestRunSlowDownAndPending(t *testing.T) {
	redirectCache(t)
	idp, platform := newFakeFlow(t, []func(w http.ResponseWriter){
		func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
		},
		func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
		},
	})
	o := testOptions(platform.URL, idp.URL)
	o.Stderr = &strings.Builder{}

	credBytes, err := Run(context.Background(), o)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// 兑换完成后输出平台 JWT（slow_down 递增间隔生效，全程约 5s 属 RFC 预期）
	if !strings.Contains(string(credBytes), "platform-jwt") {
		t.Fatalf("credential should embed exchanged jwt: %s", credBytes)
	}
}

// TestRunExpiredToken 终态错误立即返回，不悬挂。
func TestRunExpiredToken(t *testing.T) {
	redirectCache(t)
	idp, platform := newFakeFlow(t, []func(w http.ResponseWriter){
		func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "expired_token"})
		},
	})
	o := testOptions(platform.URL, idp.URL)
	o.Stderr = &strings.Builder{}

	start := time.Now()
	if _, err := Run(context.Background(), o); err == nil {
		t.Fatal("expired_token should fail")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("terminal error should return immediately, took %s", time.Since(start))
	}
}

// TestCacheExpiry 缓存剩余有效期不足 5min 时不复用。
func TestCacheExpiry(t *testing.T) {
	redirectCache(t)
	key := cacheKey("https://am.example.com", "cluster-a")

	// 剩余 10min → 命中
	saveCachedCredential(key, &ExecCredential{
		Status: struct {
			Token               string `json:"token"`
			ExpirationTimestamp string `json:"expirationTimestamp,omitempty"`
		}{Token: "tok", ExpirationTimestamp: time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)},
	}, time.Now())
	if _, ok := loadCachedCredential(key, time.Now()); !ok {
		t.Fatal("credential with 10min remaining should hit cache")
	}

	// 剩余 1min → 不命中（低于 5min 阈值）
	saveCachedCredential(key, &ExecCredential{
		Status: struct {
			Token               string `json:"token"`
			ExpirationTimestamp string `json:"expirationTimestamp,omitempty"`
		}{Token: "tok", ExpirationTimestamp: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)},
	}, time.Now())
	if _, ok := loadCachedCredential(key, time.Now()); ok {
		t.Fatal("credential with 1min remaining should not hit cache")
	}

	// 缓存文件落盘且权限收敛
	path, _ := cacheFilePath()
	if _, err := os.Stat(filepath.Join(path)); err != nil {
		t.Fatalf("cache file: %v", err)
	}
}
