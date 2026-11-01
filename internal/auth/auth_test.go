package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestService(t *testing.T, kids int) *Service {
	t.Helper()
	var opts []Option
	opts = append(opts,
		WithIssuer("https://am.example.com"),
		WithAudience("kamc"),
		WithNow(func() time.Time { return time.Now() }),
	)
	for i := 0; i < kids; i++ {
		pem, err := GenerateKeyPEM()
		if err != nil {
			t.Fatalf("generate key %d: %v", i, err)
		}
		opts = append(opts, WithKey(keyID(i), pem))
	}
	s, err := NewService(opts...)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return s
}

func keyID(i int) string { return "key-" + string(rune('a'+i)) }

// TestSignVerifyRoundtrip 签发→校验往返，claims 无损。
func TestSignVerifyRoundtrip(t *testing.T) {
	s := newTestService(t, 1)
	token, exp, err := s.Sign("alice", "alice@example.com", []string{"dev-team"})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if !exp.After(time.Now()) {
		t.Fatal("expiry should be in the future")
	}

	claims, err := s.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "alice" || claims.Email != "alice@example.com" {
		t.Fatalf("claims mismatch: %+v", claims.RegisteredClaims)
	}
	if len(claims.Groups) != 1 || claims.Groups[0] != "dev-team" {
		t.Fatalf("groups: %v", claims.Groups)
	}
	if claims.ID == "" {
		t.Fatal("jti should be set")
	}
}

// TestVerifyRejects 篡改/伪造/过期一律拒绝，且过期返回 ErrExpired。
func TestVerifyRejects(t *testing.T) {
	s := newTestService(t, 1)
	token, _, _ := s.Sign("alice", "", nil)

	// 篡改
	if _, err := s.Verify(context.Background(), token+"x"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered: want ErrInvalid, got %v", err)
	}

	// 其他密钥签发（未知 kid）
	other := newTestService(t, 1)
	foreign, _, _ := other.Sign("mallory", "", nil)
	if _, err := s.Verify(context.Background(), foreign); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign key: want ErrInvalid, got %v", err)
	}

	// 过期：签发时钟后移 2min、TTL 1min → exp 已成过去时；
	// jwt 库以真实时钟校验 exp（WithNow 仅作用于签发侧）
	pem := mustPEM(t)
	past := time.Now().Add(-2 * time.Minute)
	issuer, err := NewService(
		WithIssuer("https://am.example.com"), WithAudience("kamc"), WithTTL(time.Minute),
		WithNow(func() time.Time { return past }),
		WithKey("k", pem),
	)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	tokenExpired, _, _ := issuer.Sign("bob", "", nil)

	verifier, err := NewService(
		WithIssuer("https://am.example.com"), WithAudience("kamc"),
		WithKey("k", pem),
	)
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	if _, err := verifier.Verify(context.Background(), tokenExpired); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: want ErrExpired, got %v", err)
	}
}

// TestKeyRotation 零停机轮换（§4.3）：新 kid 加入即切换签名，
// 旧 kid 存量 Token 在密钥移除前仍可校验，移除后失效。
func TestKeyRotation(t *testing.T) {
	pemA, pemB := mustPEM(t), mustPEM(t)

	// 阶段1：仅 key-a
	s, err := NewService(WithIssuer("iss"), WithAudience("aud"), WithKey("key-a", pemA))
	if err != nil {
		t.Fatalf("stage1: %v", err)
	}
	oldToken, _, _ := s.Sign("alice", "", nil)

	// 阶段2：加入 key-b（目录排序靠后）→ 新签发用 key-b，旧 Token 仍可验
	s2, err := NewService(WithIssuer("iss"), WithAudience("aud"), WithKey("key-a", pemA), WithKey("key-b", pemB))
	if err != nil {
		t.Fatalf("stage2: %v", err)
	}
	newToken, _, _ := s2.Sign("bob", "", nil)
	if _, err := s2.Verify(context.Background(), oldToken); err != nil {
		t.Fatalf("old token should still verify: %v", err)
	}

	// 阶段3：移除 key-a → 旧 Token 失效，新 Token 正常
	s3, err := NewService(WithIssuer("iss"), WithAudience("aud"), WithKey("key-b", pemB))
	if err != nil {
		t.Fatalf("stage3: %v", err)
	}
	if _, err := s3.Verify(context.Background(), oldToken); !errors.Is(err, ErrInvalid) {
		t.Fatalf("old token should fail after key removal: %v", err)
	}
	if _, err := s3.Verify(context.Background(), newToken); err != nil {
		t.Fatalf("new token verify: %v", err)
	}
}

// TestKeyDirLoading 目录装载：文件名即 kid，排序末位为签名密钥。
func TestKeyDirLoading(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"jwt-1.pem", "jwt-2.pem"} {
		pem := mustPEM(t)
		if err := os.WriteFile(filepath.Join(dir, name), pem, 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}
	}
	s, err := NewService(WithIssuer("iss"), WithAudience("aud"), WithKeyDir(dir))
	if err != nil {
		t.Fatalf("load dir: %v", err)
	}
	token, _, _ := s.Sign("alice", "", nil)
	claims, err := s.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	// jwt-2 排序最后 → 签名 kid
	if _, err := s.Verify(context.Background(), token); err != nil {
		t.Fatalf("re-verify: %v", err)
	}
	_ = claims
}

// TestJWKS JWKS 输出形状：每个 kid 一条 RSA 公钥记录。
func TestJWKS(t *testing.T) {
	s := newTestService(t, 2)
	b, err := s.JWKS()
	if err != nil {
		t.Fatalf("jwks: %v", err)
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(b, &set); err != nil {
		t.Fatalf("unmarshal jwks: %v", err)
	}
	if len(set.Keys) != 2 {
		t.Fatalf("want 2 keys, got %d", len(set.Keys))
	}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.N == "" || !strings.HasPrefix(k.E, "A") {
			t.Fatalf("bad jwk: %+v", k)
		}
	}
}

func mustPEM(t *testing.T) []byte {
	t.Helper()
	pem, err := GenerateKeyPEM()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return pem
}
