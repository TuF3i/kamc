package authz

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/TuF3i/kamc/infrastructure/postgres"
	"github.com/TuF3i/kamc/internal/platform/dao"
)

// 跨副本同步测试依赖真实中间件：TEST_REDIS_ADDR（Docker Redis，必需）、
// TEST_PG_DSN（跨 enforcer 场景）。

func newTestRedisClient(t *testing.T, addr string) goredis.UniversalClient {
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

func newTestDao(t *testing.T, dsn string) *dao.Dao {
	t.Helper()
	db, err := postgres.NewPostgresClient(postgres.WithDSN(dsn))
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	d := dao.NewDao(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.SetupPolicy(ctx); err != nil {
		t.Fatalf("setup policy table: %v", err)
	}
	return d
}

// randomSuffix 轻量随机串（频道/策略行隔离）
func randomSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// TestRedisWatcherCallback 订阅/发布回环：Notify 触发已注册回调。
func TestRedisWatcherCallback(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set, skip watcher test")
	}
	cli := newTestRedisClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w, err := newRedisWatcher(ctx, cli, "am:casbin:test-"+randomSuffix())
	if err != nil {
		t.Fatalf("watcher: %v", err)
	}
	defer w.Close()

	got := make(chan string, 8)
	w.SetUpdateCallback(func(payload string) { got <- payload })

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		w.Notify() // 订阅确认后仍可能偶发竞态，重试至收到
		select {
		case payload := <-got:
			if payload != "update" {
				t.Fatalf("payload: %q", payload)
			}
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatal("callback not invoked")
}

// TestWatcherCrossReplica 双 enforcer（模拟两副本，同一 DB + 同一 Redis）：
// A 加策略 → B 在同步窗口内可见；A 删除 → B 同步消失。
func TestWatcherCrossReplica(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set, skip cross-replica test")
	}
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set, skip cross-replica test")
	}
	redisCli := newTestRedisClient(t, addr)
	d := newTestDao(t, dsn)

	marker := randomSuffix()
	channel := "am:casbin:test-" + marker
	sub := "user:w-" + marker
	dom := "cluster-w-" + marker
	user := "w-" + marker

	enA, err := New(d, WithSyncChannel(redisCli, channel))
	if err != nil {
		t.Fatalf("enforcer A: %v", err)
	}
	defer enA.Release()
	enB, err := New(d, WithSyncChannel(redisCli, channel))
	if err != nil {
		t.Fatalf("enforcer B: %v", err)
	}
	defer enB.Release()

	// A 加直接用户策略 → 本地即时生效 → 广播后 B 可见
	if added, err := enA.AddPolicy(sub, dom, "dev/*", "get"); err != nil || !added {
		t.Fatalf("A add policy: added=%v err=%v", added, err)
	}
	if ok, _ := enA.Enforce(user, nil, dom, "dev/pods", "get"); !ok {
		t.Fatal("A should enforce immediately")
	}
	waitFor(t, 3*time.Second, func() bool {
		ok, _ := enB.Enforce(user, nil, dom, "dev/pods", "get")
		return ok
	}, "B should see A's policy after broadcast")

	// A 删除 → B 同步消失
	if _, err := enA.RemovePolicy(sub, dom, "dev/*", "get"); err != nil {
		t.Fatalf("A remove policy: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool {
		ok, _ := enB.Enforce(user, nil, dom, "dev/pods", "get")
		return !ok
	}, "B should drop the removed policy after broadcast")
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting: %s", msg)
}
