package oidc

// StateStore 的 Redis 实现（多副本共享，§4.1）：
// Save = SET EX；Take = GETDEL（原子一次性消费）。

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RedisStateStore StateStore 的 Redis 实现
type RedisStateStore struct {
	cli goredis.UniversalClient
}

// NewRedisStateStore 构造（key 前缀 am:oidc:state:）
func NewRedisStateStore(cli goredis.UniversalClient) *RedisStateStore {
	return &RedisStateStore{cli: cli}
}

func stateKey(state string) string { return "am:oidc:state:" + state }

// Save SET + TTL
func (r *RedisStateStore) Save(ctx context.Context, state string, exp time.Time) error {
	ttl := time.Until(exp)
	if ttl <= 0 {
		// 保存已过期 state = 确保不存在
		if err := r.cli.Del(ctx, stateKey(state)).Err(); err != nil {
			return fmt.Errorf("oidc -> save state (del): %w", err)
		}
		return nil
	}
	if err := r.cli.Set(ctx, stateKey(state), "1", ttl).Err(); err != nil {
		return fmt.Errorf("oidc -> save state: %w", err)
	}
	return nil
}

// Take GETDEL：原子取出并删除（一次性消费；并发回调只有一个能取到）
func (r *RedisStateStore) Take(ctx context.Context, state string) (bool, error) {
	v, err := r.cli.GetDel(ctx, stateKey(state)).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return false, nil
		}
		return false, fmt.Errorf("oidc -> take state: %w", err)
	}
	return v == "1", nil
}
