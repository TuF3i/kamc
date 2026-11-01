package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// memoryRevocations 测试/单副本语义的内存存储（接口实现的同步路径验证）
type memoryRevocations struct {
	mu     sync.Mutex
	epochs map[string]int64
	tokens map[string]struct{}
}

func newMemoryRevocations() *memoryRevocations {
	return &memoryRevocations{
		epochs: make(map[string]int64),
		tokens: make(map[string]struct{}),
	}
}

func (m *memoryRevocations) BumpUserEpoch(_ context.Context, sub string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.epochs[sub]++
	return m.epochs[sub], nil
}

func (m *memoryRevocations) LoadUserEpochs(_ context.Context) (map[string]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]int64, len(m.epochs))
	for k, v := range m.epochs {
		out[k] = v
	}
	return out, nil
}

func (m *memoryRevocations) SaveToken(_ context.Context, jti string, exp time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[jti] = struct{}{}
	return nil
}

func (m *memoryRevocations) LoadTokens(_ context.Context) (map[string]struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]struct{}, len(m.tokens))
	for k := range m.tokens {
		out[k] = struct{}{}
	}
	return out, nil
}

// TestRevokeTokenLocal 本进程吊销 jti：Verify 立即 ErrRevoked。
func TestRevokeTokenLocal(t *testing.T) {
	s := newTestService(t, 1)
	token, _, err := s.Sign("alice", "", nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := s.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify before revoke: %v", err)
	}

	if err := s.RevokeToken(context.Background(), claims.ID, time.Hour); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := s.Verify(context.Background(), token); !errors.Is(err, ErrRevoked) {
		t.Fatalf("after revoke: want ErrRevoked, got %v", err)
	}
}

// TestRevokeUserEpoch 用户级踢出：旧 epoch Token 失效；同一秒内重新签发的
// 新 Token 携带新 epoch，不受影响（epoch 方案的核心价值）。
func TestRevokeUserEpoch(t *testing.T) {
	s, err := NewService(
		WithIssuer("iss"), WithAudience("aud"),
		WithKey("k", mustPEM(t)),
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	oldToken, _, _ := s.Sign("alice", "", nil)
	if _, err := s.Verify(context.Background(), oldToken); err != nil {
		t.Fatalf("verify before revoke: %v", err)
	}

	// 踢出（epoch 递增）
	if err := s.RevokeUser(context.Background(), "alice"); err != nil {
		t.Fatalf("revoke user: %v", err)
	}
	if _, err := s.Verify(context.Background(), oldToken); !errors.Is(err, ErrRevoked) {
		t.Fatalf("old token: want ErrRevoked, got %v", err)
	}

	// 立即重新签发（同一秒）→ 新 epoch → 可用
	newToken, _, _ := s.Sign("alice", "", nil)
	if _, err := s.Verify(context.Background(), newToken); err != nil {
		t.Fatalf("new token after revoke (same second): %v", err)
	}

	// 旧格式（无 epoch 声明）按 0 处理 → 任何踢出都使其失效
	if _, err := s.Verify(context.Background(), oldToken); !errors.Is(err, ErrRevoked) {
		t.Fatalf("legacy token: want ErrRevoked, got %v", err)
	}
}

// TestRevocationCrossReplicaSync 另一副本的吊销经存储同步到本进程
// （惰性刷新：syncInterval 过后的首次 Verify 触发）。
func TestRevocationCrossReplicaSync(t *testing.T) {
	store := newMemoryRevocations()
	now := time.Now()
	clock := now

	// 共享密钥（生产为 Secret 下发到全部副本）
	keyPEM := mustPEM(t)

	// "副本 1"：签发 token（无吊销存储）
	replica1, err := NewService(
		WithIssuer("iss"), WithAudience("aud"),
		WithNow(func() time.Time { return clock }),
		WithKey("k", keyPEM),
	)
	if err != nil {
		t.Fatalf("replica1: %v", err)
	}
	token, _, _ := replica1.Sign("bob", "", nil)
	claims, _ := replica1.Verify(context.Background(), token)

	// "副本 2"（同密钥 + 吊销存储）：同步周期 10ms
	replica2, err := NewService(
		WithIssuer("iss"), WithAudience("aud"),
		WithNow(func() time.Time { return clock }),
		WithKey("k", keyPEM),
		WithRevocations(store),
		WithRevocationSyncInterval(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("replica2: %v", err)
	}
	if _, err := replica2.Verify(context.Background(), token); err != nil {
		t.Fatalf("verify before remote revoke: %v", err)
	}

	// 副本 1 踢出 bob → 写入共享存储
	if _, err := store.BumpUserEpoch(context.Background(), "bob"); err != nil {
		t.Fatalf("remote bump: %v", err)
	}
	if err := store.SaveToken(context.Background(), claims.ID, clock.Add(time.Hour)); err != nil {
		t.Fatalf("remote save token: %v", err)
	}

	// 副本 2 的时钟越过同步周期 → 首次 Verify 拉取 → 吊销生效
	clock = now.Add(time.Minute)
	if _, err := replica2.Verify(context.Background(), token); !errors.Is(err, ErrRevoked) {
		t.Fatalf("after sync: want ErrRevoked, got %v", err)
	}
}

// TestRevocationDisabled 未启用吊销时（store nil）本地吊销仍可用（单副本语义）。
func TestRevocationDisabled(t *testing.T) {
	s := newTestService(t, 1)
	token, _, _ := s.Sign("alice", "", nil)
	claims, _ := s.Verify(context.Background(), token)
	if err := s.RevokeToken(context.Background(), claims.ID, time.Hour); err != nil {
		t.Fatalf("revoke without store: %v", err)
	}
	if _, err := s.Verify(context.Background(), token); !errors.Is(err, ErrRevoked) {
		t.Fatalf("local revoke should work without store: %v", err)
	}
}
