// Package registry 隧道归属注册表：记录"每条 Agent 隧道当前由哪个平台副本持有"，
// 供跨 Pod 数据面转发查询 owner（设计文档 §7.2）。
//
// 本包同时定义消费方接口 Registry（service 层依赖它而非 Redis 实现，
// 测试注入 fake）与 Redis 实现（TTL 心跳：键过期即视为持有方死亡，自愈）。
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// KeyPrefix 隧道注册键前缀，键形如 am:tunnel:{clusterID}:{agentID}
const KeyPrefix = "am:tunnel:"

// Owner 一条隧道的归属记录
type Owner struct {
	ClusterID string    `json:"cluster_id"`
	AgentID   string    `json:"agent_id"`
	Pod       string    `json:"pod"` // 持有该隧道的平台副本
	Since     time.Time `json:"since"`
}

// Registry 隧道归属注册表接口（实现须保证：不再续期的记录在 TTL 后自动消失）
type Registry interface {
	// Report 上报/续期一条隧道归属（每次调用都重置 TTL）
	Report(ctx context.Context, clusterID, agentID, pod string) error
	// Deregister 注销一条隧道归属（持有方主动下线时调用，避免等待 TTL）
	Deregister(ctx context.Context, clusterID, agentID string) error
	// Lookup 查询某集群当前全部隧道归属（供跨 Pod 转发择优）
	Lookup(ctx context.Context, clusterID string) ([]Owner, error)
}

// RedisRegistry Registry 的 Redis 实现
type RedisRegistry struct {
	cli goredis.UniversalClient
	ttl time.Duration
}

// NewRedisRegistry 构造 Redis 注册表；ttl 即心跳失联判定阈值（建议 3×心跳间隔）。
func NewRedisRegistry(cli goredis.UniversalClient, ttl time.Duration) *RedisRegistry {
	return &RedisRegistry{cli: cli, ttl: ttl}
}

func tunnelKey(clusterID, agentID string) string {
	return fmt.Sprintf("%s%s:%s", KeyPrefix, clusterID, agentID)
}

// Report 上报归属：SET + TTL（每次覆盖 Since，体现最近一次心跳）
func (r *RedisRegistry) Report(ctx context.Context, clusterID, agentID, pod string) error {
	b, err := json.Marshal(Owner{ClusterID: clusterID, AgentID: agentID, Pod: pod, Since: time.Now()})
	if err != nil {
		return fmt.Errorf("registry -> marshal owner: %w", err)
	}
	if err := r.cli.Set(ctx, tunnelKey(clusterID, agentID), b, r.ttl).Err(); err != nil {
		return fmt.Errorf("registry -> report %s/%s: %w", clusterID, agentID, err)
	}
	return nil
}

// Deregister 注销归属
func (r *RedisRegistry) Deregister(ctx context.Context, clusterID, agentID string) error {
	if err := r.cli.Del(ctx, tunnelKey(clusterID, agentID)).Err(); err != nil {
		return fmt.Errorf("registry -> deregister %s/%s: %w", clusterID, agentID, err)
	}
	return nil
}

// Lookup 扫描某集群的全部归属记录（集群规模 ≤ 数百，SCAN 足够）
func (r *RedisRegistry) Lookup(ctx context.Context, clusterID string) ([]Owner, error) {
	pattern := fmt.Sprintf("%s%s:*", KeyPrefix, clusterID)

	var owners []Owner
	iter := r.cli.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		raw, err := r.cli.Get(ctx, iter.Val()).Bytes()
		if err != nil {
			// 键在 SCAN 与 GET 之间过期属正常竞态，跳过即可
			if !errors.Is(err, goredis.Nil) {
				return nil, fmt.Errorf("registry -> lookup get %s: %w", iter.Val(), err)
			}
			continue
		}
		var o Owner
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, fmt.Errorf("registry -> lookup unmarshal %s: %w", iter.Val(), err)
		}
		owners = append(owners, o)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("registry -> lookup scan: %w", err)
	}
	return owners, nil
}
