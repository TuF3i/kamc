// 限流中间件（§7）：按用户对数据面 /proxy 的滑动窗口限流，
// 防止单用户打爆隧道。ZSET 实现（score=请求时刻），窗口滑出即过期。
//
// 语义：限流器故障 fail-open（可用性优先——限流是保护措施，不能因它拒服务）；
// 无身份请求不计数（认证中间件先行）。
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RateLimiter 限流判定接口（滑动窗口；Redis / 内存双实现）
type RateLimiter interface {
	// Allow 判定 key 在当前窗口内是否放行（并记录本次请求）
	Allow(ctx context.Context, key string) (bool, error)
}

// RedisRateLimiter 滑动窗口限流（ZSET：score=请求时刻纳秒）
type RedisRateLimiter struct {
	cli    goredis.UniversalClient
	window time.Duration
	limit  int
}

// NewRedisRateLimiter 构造（window 窗口宽度；limit 窗口内最大请求数）
func NewRedisRateLimiter(cli goredis.UniversalClient, window time.Duration, limit int) *RedisRateLimiter {
	return &RedisRateLimiter{cli: cli, window: window, limit: limit}
}

func (l *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now()
	nowNano := now.UnixNano()
	windowStart := fmt.Sprintf("%d", now.Add(-l.window).UnixNano())

	member := fmt.Sprintf("%d-%s", nowNano, nonce())
	rkey := "am:rl:" + key

	pipe := l.cli.Pipeline()
	pipe.ZRemRangeByScore(ctx, rkey, "-inf", windowStart)
	pipe.ZAdd(ctx, rkey, goredis.Z{Score: float64(nowNano), Member: member})
	cardCmd := pipe.ZCard(ctx, rkey)
	pipe.Expire(ctx, rkey, l.window+time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("ratelimit -> redis: %w", err)
	}
	card, err := cardCmd.Result()
	if err != nil {
		return false, fmt.Errorf("ratelimit -> card: %w", err)
	}
	return int(card) <= l.limit, nil
}

// MemoryRateLimiter 进程内滑动窗口（无 Redis 时使用；单副本语义）
type MemoryRateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	hits   map[string][]time.Time
	now    func() time.Time
}

// NewMemoryRateLimiter 构造
func NewMemoryRateLimiter(window time.Duration, limit int) *MemoryRateLimiter {
	return &MemoryRateLimiter{
		window: window,
		limit:  limit,
		hits:   make(map[string][]time.Time),
		now:    time.Now,
	}
}

// SetNow 覆盖时钟（测试注入）
func (l *MemoryRateLimiter) SetNow(now func() time.Time) { l.now = now }

func (l *MemoryRateLimiter) Allow(_ context.Context, key string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-l.window)

	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false, nil
	}
	l.hits[key] = append(kept, now)
	return true, nil
}

func nonce() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RateLimit 中间件：超限返回 429（Retry-After = 窗口秒数）。
// limiter 为 nil 或 limit <= 0 时不启用（直通）。
func RateLimit(limiter RateLimiter, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil || limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			claims, ok := ClaimsFromCtx(r.Context())
			if !ok {
				next.ServeHTTP(w, r) // 无身份请求由认证中间件负责
				return
			}
			allowed, err := limiter.Allow(r.Context(), "user:"+claims.Subject)
			if err != nil {
				// fail-open：限流器故障不拦业务，仅记录（保护措施不能变成单点）
				log.Printf("[platform] ratelimit check failed: %v", err)
				next.ServeHTTP(w, r)
				return
			}
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
