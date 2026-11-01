// Package redis Redis 连接的统一构建入口。
//
// 返回 UniversalClient：同一入口兼容单机 / 哨兵 / 集群三种拓扑
// （设计文档 §7.6：Redis 仅承载隧道注册表、casbin watcher 与限流，
// 不在鉴权关键路径，故障时平台自愈或降级）。
package redis

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// DefaultAddr 未配置任何 Redis 地址时的本地开发兜底值
const DefaultAddr = "127.0.0.1:6379"

// Option 连接选项
type Option func(*options)

type options struct {
	addrs    []string
	password string
	db       int
}

// WithAddrs 指定地址列表（默认读 REDIS_ADDRESSES / REDIS_ADDR 环境变量）
func WithAddrs(addrs ...string) Option {
	return func(o *options) { o.addrs = addrs }
}

// WithAuth 指定密码与逻辑库
func WithAuth(password string, db int) Option {
	return func(o *options) { o.password, o.db = password, db }
}

// NewRedisClient 构建 Redis 客户端并做一次 Ping 验证（3s 超时，失败即返回错误）。
func NewRedisClient(opts ...Option) (goredis.UniversalClient, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	addrs := o.addrs
	if len(addrs) == 0 {
		addrs = redisAddrsFromEnv()
	}
	if len(addrs) == 0 {
		addrs = []string{DefaultAddr}
	}
	if o.password == "" {
		o.password = os.Getenv("REDIS_PASSWORD")
	}
	if o.db == 0 {
		o.db, _ = strconv.Atoi(os.Getenv("REDIS_DB"))
	}

	cli := goredis.NewUniversalClient(&goredis.UniversalOptions{
		Addrs:        addrs,
		Password:     o.password,
		DB:           o.db,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolSize:     20,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := cli.Ping(ctx).Err(); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("redis -> ping %v: %w", addrs, err)
	}

	log.Printf("[redis] connected (addrs=%v db=%d)", addrs, o.db)
	return cli, nil
}

// redisAddrsFromEnv 环境变量收集地址：REDIS_ADDRESSES（逗号分隔，优先）或 REDIS_ADDR（单个）
func redisAddrsFromEnv() []string {
	var addrs []string
	for _, env := range []string{"REDIS_ADDRESSES", "REDIS_ADDR"} {
		v := strings.TrimSpace(os.Getenv(env))
		if v == "" {
			continue
		}
		for _, a := range strings.Split(v, ",") {
			if a = strings.TrimSpace(a); a != "" {
				addrs = append(addrs, a)
			}
		}
		if len(addrs) > 0 {
			return addrs
		}
	}
	return nil
}
