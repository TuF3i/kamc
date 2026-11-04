package service

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// roundTripFunc 便于测试 transport 注入的头
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestFileTokenSourceReload 投射 SA Token 轮换后必须使用新值（否则约 1h 后全 401）。
func TestFileTokenSourceReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("token-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := &fileTokenSource{path: path}
	if got := src.Token(); got != "token-1" {
		t.Fatalf("token: got %q", got)
	}

	if err := os.WriteFile(path, []byte("token-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 确保 mtime 严格前进（部分文件系统时间精度较粗）
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if got := src.Token(); got != "token-2" {
		t.Fatalf("rotated token: got %q", got)
	}

	// 读失败时保留上次有效值
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := src.Token(); got != "token-2" {
		t.Fatalf("token after remove: got %q", got)
	}
}

func TestBearerTransportUsesLatestToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := &fileTokenSource{path: path}
	var got string
	tr := &bearerTransport{
		base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			got = r.Header.Get("Authorization")
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		}),
		source: src,
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid", nil)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer old" {
		t.Fatalf("authorization: %q", got)
	}

	if err := os.WriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer new" {
		t.Fatalf("rotated authorization: %q", got)
	}
}
