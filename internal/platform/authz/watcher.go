package authz

// 跨副本策略同步（§7.6）：策略增删后 PUBLISH 通知，各副本订阅回调全量重载。
//
// 自写 pub/sub watcher 而非引入 casbin-redis-watcher：Redis 客户端已在依赖树
// （go-redis UniversalClient），避免第三方 watcher 拖入另一套 Redis 依赖版本。
// 同步语义为最终一致（pub/sub 无持久化——副本离线期间的变更靠下一次
// 全量 LoadPolicy 或重启补齐，规模内可接受，见设计文档 §7.6）。

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// DefaultSyncChannel 策略同步频道
const DefaultSyncChannel = "am:casbin:changed"

// redisWatcher 策略变更广播器（发布 + 订阅回调）
type redisWatcher struct {
	cli     goredis.UniversalClient
	channel string

	mu       sync.Mutex
	sub      *goredis.PubSub
	callback func(string)
	done     chan struct{}
	wg       sync.WaitGroup
}

// newRedisWatcher 建立订阅并起分发循环。回调在订阅 goroutine 内串行执行。
func newRedisWatcher(ctx context.Context, cli goredis.UniversalClient, channel string) (*redisWatcher, error) {
	sub := cli.Subscribe(ctx, channel)
	// 订阅确认（避免"发布先于订阅生效"的启动竞态）
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, fmt.Errorf("authz -> watcher subscribe: %w", err)
	}

	w := &redisWatcher{cli: cli, channel: channel, sub: sub, done: make(chan struct{})}
	w.wg.Add(1)
	go w.loop()
	return w, nil
}

// SetUpdateCallback 注册变更回调（收到广播时执行；单回调语义）
func (w *redisWatcher) SetUpdateCallback(fn func(string)) {
	w.mu.Lock()
	w.callback = fn
	w.mu.Unlock()
}

// Notify 广播一次变更（发布失败仅记录——订阅方仍有重启/重载兜底）
func (w *redisWatcher) Notify() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := w.cli.Publish(ctx, w.channel, "update").Err(); err != nil {
		log.Printf("[authz] watcher publish: %v", err)
	}
}

func (w *redisWatcher) loop() {
	defer w.wg.Done()
	msgs := w.sub.Channel()
	for {
		select {
		case <-w.done:
			return
		case msg := <-msgs:
			w.mu.Lock()
			fn := w.callback
			w.mu.Unlock()
			if fn == nil {
				continue
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[authz] watcher callback panic: %v\n%s", r, debug.Stack())
					}
				}()
				fn(msg.Payload)
			}()
		}
	}
}

// Close 停止分发并退订（幂等）
func (w *redisWatcher) Close() {
	w.mu.Lock()
	if w.done != nil {
		select {
		case <-w.done:
		default:
			close(w.done)
		}
	}
	w.mu.Unlock()
	if w.sub != nil {
		_ = w.sub.Close()
	}
	w.wg.Wait()
}
