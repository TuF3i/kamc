package auth

// Token 吊销（§4.2）：用户级 epoch + jti 黑名单。
//
// 用户级踢出采用**单调 epoch**（无时钟歧义）：每用户维护递增计数，
// Token 签发时携带当前 epoch；Verify 发现 token.epoch < 用户当前 epoch 即吊销。
// 同一秒内"踢出→重新签发"也不误杀（新 Token 天然携带新 epoch）。
// Redis 侧 HINCRBY 原子递增，天然跨副本一致。
//
// 校验路径只查本地缓存（§7.1 鉴权零外部依赖不变量）；RevocationStore
// 按惰性周期同步到本进程，写入方即时更新本地并写穿。

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// ErrRevoked token 已被吊销（区别于 ErrInvalid/ErrExpired，供日志与审计区分）
var ErrRevoked = errors.New("token revoked")

// RevocationStore 吊销状态的跨副本存储（Redis 实现；nil = 单副本进程内语义）
type RevocationStore interface {
	// BumpUserEpoch 原子递增用户 epoch，返回新值
	BumpUserEpoch(ctx context.Context, sub string) (int64, error)
	// LoadUserEpochs 全量拉取用户 epoch
	LoadUserEpochs(ctx context.Context) (map[string]int64, error)
	// SaveToken 吊销单个 jti（过期后由实现自行清理）
	SaveToken(ctx context.Context, jti string, exp time.Time) error
	// LoadTokens 全量拉取未过期的已吊销 jti
	LoadTokens(ctx context.Context) (map[string]struct{}, error)
}

// RedisRevocations RevocationStore 的 Redis 实现：
//   - 用户 epoch：HINCRBY am:revoke:epochs <sub> 1
//   - jti 集合：ZSET am:revoke:tokens（score=过期时刻，读取时顺带清理）
type RedisRevocations struct {
	cli goredis.UniversalClient
}

// NewRedisRevocations 构造 Redis 吊销存储
func NewRedisRevocations(cli goredis.UniversalClient) *RedisRevocations {
	return &RedisRevocations{cli: cli}
}

const (
	keyRevokeEpochs = "am:revoke:epochs"
	keyRevokedToks  = "am:revoke:tokens"
)

func (r *RedisRevocations) BumpUserEpoch(ctx context.Context, sub string) (int64, error) {
	v, err := r.cli.HIncrBy(ctx, keyRevokeEpochs, sub, 1).Result()
	if err != nil {
		return 0, fmt.Errorf("auth -> bump user epoch: %w", err)
	}
	return v, nil
}

func (r *RedisRevocations) LoadUserEpochs(ctx context.Context) (map[string]int64, error) {
	vals, err := r.cli.HGetAll(ctx, keyRevokeEpochs).Result()
	if err != nil {
		return nil, fmt.Errorf("auth -> load user epochs: %w", err)
	}
	out := make(map[string]int64, len(vals))
	for sub, v := range vals {
		var n int64
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			out[sub] = n
		}
	}
	return out, nil
}

func (r *RedisRevocations) SaveToken(ctx context.Context, jti string, exp time.Time) error {
	if err := r.cli.ZAdd(ctx, keyRevokedToks, goredis.Z{Score: float64(exp.Unix()), Member: jti}).Err(); err != nil {
		return fmt.Errorf("auth -> revoke token: %w", err)
	}
	return nil
}

func (r *RedisRevocations) LoadTokens(ctx context.Context) (map[string]struct{}, error) {
	now := time.Now().Unix()
	if err := r.cli.ZRemRangeByScore(ctx, keyRevokedToks, "-inf", fmt.Sprintf("(%d", now)).Err(); err != nil {
		return nil, fmt.Errorf("auth -> cleanup revoked tokens: %w", err)
	}
	vals, err := r.cli.ZRange(ctx, keyRevokedToks, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("auth -> load revoked tokens: %w", err)
	}
	out := make(map[string]struct{}, len(vals))
	for _, jti := range vals {
		out[jti] = struct{}{}
	}
	return out, nil
}

// ---- 本地状态与同步 ----

// WithRevocations 启用吊销（store 可为 nil：仅进程内生效，多副本不同步）
func WithRevocations(store RevocationStore) Option {
	return func(o *options) { o.revocations = store }
}

// WithRevocationSyncInterval 跨副本同步周期（默认 5s；一致性窗口即此值）
func WithRevocationSyncInterval(d time.Duration) Option {
	return func(o *options) { o.revocationSyncInterval = d }
}

// RevokeUser 踢出用户：epoch 递增，旧 Token 立即失效（本地即时 + 写穿）
func (s *Service) RevokeUser(ctx context.Context, sub string) error {
	if sub == "" {
		return errors.New("auth -> revoke user: empty sub")
	}
	var epoch int64
	if s.store != nil {
		v, err := s.store.BumpUserEpoch(ctx, sub)
		if err != nil {
			return fmt.Errorf("auth -> bump user epoch: %w", err)
		}
		epoch = v
	} else {
		s.revMu.Lock()
		epoch = s.userEpochs[sub] + 1 // 无存储：本地递增（单副本语义）
		s.revMu.Unlock()
	}

	s.revMu.Lock()
	if s.userEpochs[sub] < epoch {
		s.userEpochs[sub] = epoch
	}
	s.revMu.Unlock()
	return nil
}

// RevokeToken 吊销单个 Token（jti；登出用）。ttl 为剩余有效期，
// 过期后存储端自行清理。
func (s *Service) RevokeToken(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" {
		return errors.New("auth -> revoke token: empty jti")
	}
	s.revMu.Lock()
	s.revokedTokens[jti] = struct{}{}
	s.revMu.Unlock()

	if s.store == nil {
		return nil
	}
	if err := s.store.SaveToken(ctx, jti, s.now().Add(ttl)); err != nil {
		return fmt.Errorf("auth -> save revoked token: %w", err)
	}
	return nil
}

// maybeSyncRevocations 惰性周期同步（Verify 热路径只做一次时间判断）。
// 到期时同步执行：一次 Redis 全量拉取（3s 超时），换跨副本吊销的确定性——
// 异步执行会让触发请求先于同步完成返回，吊销生效时间不可预期。
func (s *Service) maybeSyncRevocations() {
	if s.store == nil {
		return
	}
	s.revMu.Lock()
	needs := s.now().Sub(s.lastRevSync) >= s.revSyncInterval
	if needs {
		s.lastRevSync = s.now() // 先推进，失败则下个周期重试
	}
	s.revMu.Unlock()
	if needs {
		s.syncRevocations() // 锁外执行（syncRevocations 内部自行加锁）
	}
}

func (s *Service) syncRevocations() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	epochs, err := s.store.LoadUserEpochs(ctx)
	if err != nil {
		return // 保留旧状态，下个周期重试
	}
	tokens, err := s.store.LoadTokens(ctx)
	if err != nil {
		return
	}
	s.revMu.Lock()
	for sub, e := range epochs {
		if e > s.userEpochs[sub] {
			s.userEpochs[sub] = e
		}
	}
	s.revokedTokens = tokens
	s.revMu.Unlock()
}

// checkRevoked 校验本地吊销状态（Verify 内调用；只查内存）
func (s *Service) checkRevoked(claims *Claims) error {
	s.revMu.RLock()
	defer s.revMu.RUnlock()
	if _, ok := s.revokedTokens[claims.ID]; ok {
		return ErrRevoked
	}
	// 无 epoch 声明的旧 Token 按 0 处理：任何踢出都会使其失效
	if claims.Epoch < s.userEpochs[claims.Subject] {
		return ErrRevoked
	}
	return nil
}
