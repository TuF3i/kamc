package oidc

import (
	"context"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func goredisNewUniversalClient(t *testing.T, addr string) goredis.UniversalClient {
	t.Helper()
	cli := goredis.NewUniversalClient(&goredis.UniversalOptions{Addrs: []string{addr}})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := cli.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect test redis %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

// 测试断言 StateStore 语义（内存与 Redis 实现共用同一套用例）

func assertStateStore(t *testing.T, store StateStore) {
	t.Helper()
	ctx := context.Background()

	// 未保存的 state → false
	if ok, err := store.Take(ctx, "no-such-state"); ok || err != nil {
		t.Fatalf("unknown state: ok=%v err=%v", ok, err)
	}

	// 保存 → 取到 → 二次取不到（一次性）
	if err := store.Save(ctx, "s1", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("save: %v", err)
	}
	if ok, err := store.Take(ctx, "s1"); err != nil || !ok {
		t.Fatalf("take saved: ok=%v err=%v", ok, err)
	}
	if ok, _ := store.Take(ctx, "s1"); ok {
		t.Fatal("state must be single-use")
	}

	// 已过期 → 取不到
	if err := store.Save(ctx, "s2", time.Now().Add(-time.Second)); err != nil {
		t.Fatalf("save expired: %v", err)
	}
	if ok, _ := store.Take(ctx, "s2"); ok {
		t.Fatal("expired state must not be taken")
	}
}

// TestMemoryStateStore 进程内实现语义
func TestMemoryStateStore(t *testing.T) {
	assertStateStore(t, newMemoryStateStore())
}

// TestRedisStateStore Redis 实现语义（TEST_REDIS_ADDR 门控）
func TestRedisStateStore(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set, skip redis state store test")
	}
	cli := goredisNewUniversalClient(t, addr)
	assertStateStore(t, NewRedisStateStore(cli))
}
