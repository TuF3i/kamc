// Package service Agent 连接服务：Token 读取与轮询、指数退避拨出、
// yamux 会话建立与流处理。
//
// 并发模型（约定 done-channel + wg）：
//   - connectLoop：拨出主循环，会话断开后按 full-jitter 退避重连；
//   - tokenWatch：轮询 Token 文件哈希，变更即断开重连（设计文档 §6.3 热更新）；
//   - acceptLoop：每条会话一个，逐流分发给 handleStream；
//   - Release 顺序：close(done) → 关连接/会话 → 带超时 wg.Wait（幂等）。
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"

	"github.com/TuF3i/kamc/internal/wsconn"
)

// ErrTokenUnavailable Token 文件缺失或内容为空（Agent 收到 401 后的
// "停止重试等管理员"流程属后续路线图，当前按可重试错误处理）
var ErrTokenUnavailable = errors.New("agent token unavailable")

// tokenPollInterval Token 文件轮询间隔（Secret 同步约 1min，30s 轮询足够，§6.3）
const tokenPollInterval = 30 * time.Second

// Config Agent 运行配置（环境变量装载，定义在本包避免反向依赖模块层）
type Config struct {
	PlatformURL  string        // 平台隧道入口 wss://...
	ClusterID    string        // 本集群 ID
	AgentID      string        // 本实例标识（双副本须不同）
	TokenFile    string        // Token 文件路径（Secret 挂载）
	PingInterval time.Duration // WS 心跳 ping 间隔
	BackoffBase  time.Duration // 重连退避基数
	BackoffMax   time.Duration // 重连退避上限
}

// Validate 必填项校验
func (c Config) Validate() error {
	switch {
	case c.PlatformURL == "":
		return errors.New("PLATFORM_URL is required")
	case !strings.HasPrefix(c.PlatformURL, "ws://") && !strings.HasPrefix(c.PlatformURL, "wss://"):
		return errors.New("PLATFORM_URL must start with ws:// or wss://")
	case c.ClusterID == "":
		return errors.New("CLUSTER_ID is required")
	case c.TokenFile == "":
		return errors.New("TOKEN_FILE is required")
	}
	return nil
}

// DialFunc 拨出函数签名（测试注入点；默认走真实 WebSocket 拨号）
type DialFunc func(ctx context.Context, cfg Config, token string) (*websocket.Conn, error)

// Service Agent 连接服务
type Service struct {
	cfg Config

	mu            sync.Mutex
	conn          *websocket.Conn
	session       *yamux.Session
	lastTokenHash string

	dialFn  DialFunc
	done    chan struct{}
	wg      sync.WaitGroup
	startMu sync.Mutex
	started bool      // Start 幂等标记
	rel     sync.Once // Release 幂等（close(done) 只执行一次）
}

// NewService 构造 Service（Start 由模块装配层显式调用）
func NewService(cfg Config) *Service {
	return &Service{cfg: cfg, dialFn: defaultDial, done: make(chan struct{})}
}

// SetDialFunc 替换拨出函数（测试注入；须在 Start 之前调用）
func (s *Service) SetDialFunc(fn DialFunc) {
	s.dialFn = fn
}

// defaultDial 真实拨出：Bearer Token + 集群/实例标识头；TLS 信任系统根证书，
// 平台证书为私有 CA 时经 TOKEN_CA_FILE 扩展（路线图：Token 认证入口）。
func defaultDial(ctx context.Context, cfg Config, token string) (*websocket.Conn, error) {
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	header := http.Header{
		"Authorization": []string{"Bearer " + token},
		"X-Cluster-ID":  []string{cfg.ClusterID},
		"X-Agent-ID":    []string{cfg.AgentID},
	}
	conn, _, err := d.DialContext(ctx, cfg.PlatformURL, header)
	if err != nil {
		return nil, fmt.Errorf("agent -> dial platform: %w", err)
	}
	return conn, nil
}

// Start 启动连接循环与 Token 轮询（幂等）。
// Token 尚不可用不阻塞启动：Secret 可能稍后挂载，连接循环按退避持续重试。
func (s *Service) Start() error {
	if _, err := s.readToken(); err != nil {
		log.Printf("[agent] token 不可用，将按退避重试: %v", err)
	}
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if s.started {
		return nil
	}
	s.started = true
	s.wg.Add(2)
	go s.connectLoop()
	go s.tokenWatch()
	log.Printf("[agent] started (cluster=%s agent=%s platform=%s)",
		s.cfg.ClusterID, s.cfg.AgentID, s.cfg.PlatformURL)
	return nil
}

// connectLoop 拨出主循环：成功建立会话后阻塞等待其关闭（连接错误/Token 轮询断开），
// 随后按退避重连；done 关闭时干净退出。
func (s *Service) connectLoop() {
	defer s.wg.Done()
	backoff := s.cfg.BackoffBase

	for {
		if s.isDone() {
			return
		}

		token, err := s.readToken()
		if err != nil {
			log.Printf("[agent] read token: %v", err)
			if !s.sleepBackoff(&backoff) {
				return
			}
			continue
		}

		// 拨号自带 HandshakeTimeout，不依赖 ctx 取消；done 在拨号返回后立即生效
		conn, err := s.dialFn(context.Background(), s.cfg, token)
		if err != nil {
			log.Printf("[agent] 连接平台失败，退避 %s: %v", backoff, err)
			if !s.sleepBackoff(&backoff) {
				return
			}
			continue
		}
		backoff = s.cfg.BackoffBase // 成功即重置退避

		sessionDone, err := s.setupSession(conn)
		if err != nil {
			log.Printf("[agent] 建立隧道会话失败: %v", err)
			_ = conn.Close()
			if !s.sleepBackoff(&backoff) {
				return
			}
			continue
		}

		select {
		case <-s.done:
			s.closeSession()
			return
		case <-sessionDone:
			log.Printf("[agent] 隧道断开，准备重连")
		}
	}
}

// setupSession 包装 WS → yamux 会话并启动 acceptLoop，返回会话结束信号。
// conn 的所有权随之转移：成功时由会话/acceptLoop 管理，失败时调用方负责关闭。
func (s *Service) setupSession(conn *websocket.Conn) (<-chan struct{}, error) {
	session, err := yamux.Client(wsconn.New(conn), nil)
	if err != nil {
		return nil, fmt.Errorf("agent -> yamux client: %w", err)
	}

	ch := make(chan struct{})
	s.mu.Lock()
	s.conn = conn
	s.session = session
	s.mu.Unlock()

	s.wg.Add(1)
	go s.acceptLoop(session, ch)
	return ch, nil
}

// acceptLoop 逐流分发；会话关闭（主动 Release/Token 轮询重连/网络断开）时自然退出
func (s *Service) acceptLoop(session *yamux.Session, doneCh chan<- struct{}) {
	defer s.wg.Done()
	defer close(doneCh)
	for {
		stream, err := session.Accept()
		if err != nil {
			// Release/重连路径的关闭也走这里，一次会话一条日志即可
			log.Printf("[agent] 隧道会话关闭: %v", err)
			return
		}
		go s.handleStream(stream)
	}
}

// handleStream 处理平台经隧道发来的单条连接（yamux v0.1.2 的 Accept 直接给 net.Conn）。
// TODO(路线图·数据面)：按设计文档 §6.2 实现——读取 HTTP 请求后经
// rest.TransportFor(saConfig) 反向代理到 kube-apiserver（含 101 升级双向拷贝）。
// 基础框架阶段直接关闭，避免悬挂半开流。
func (s *Service) handleStream(stream net.Conn) {
	_ = stream.Close()
}

// tokenWatch 轮询 Token 文件哈希；变更即断开当前会话，connectLoop 会用新 Token 重连。
func (s *Service) tokenWatch() {
	defer s.wg.Done()
	ticker := time.NewTicker(tokenPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			token, err := s.readToken()
			if err != nil {
				continue // 文件瞬时不可读：保持现有连接，下轮再查
			}
			sum := sha256.Sum256([]byte(token))
			hash := hex.EncodeToString(sum[:])

			s.mu.Lock()
			changed := s.lastTokenHash != "" && hash != s.lastTokenHash
			s.mu.Unlock()
			if changed {
				log.Printf("[agent] 检测到 Token 变更，断开重连")
				s.closeSession() // 仅断会话，connectLoop 循环存活
			}
		}
	}
}

// readToken 读取并清洗 Token（connectLoop 与 tokenWatch 两个消费方，纯读不加锁）
func (s *Service) readToken() (string, error) {
	b, err := os.ReadFile(s.cfg.TokenFile)
	if err != nil {
		return "", fmt.Errorf("agent -> read token file: %w", err)
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", fmt.Errorf("agent -> token empty: %w", ErrTokenUnavailable)
	}
	return token, nil
}

// closeSession 关闭当前会话与底层连接（幂等；各路径的清理收敛于此）
func (s *Service) closeSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil {
		_ = s.session.Close()
		s.session = nil
	}
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}

// sleepBackoff full-jitter 退避（rand(0, b)），done 关闭时返回 false 中断重连。
// 每次成功拨号后由调用方重置基数。
func (s *Service) sleepBackoff(cur *time.Duration) bool {
	j := randDuration(*cur)
	select {
	case <-s.done:
		return false
	case <-time.After(j):
	}
	if next := *cur * 2; next > s.cfg.BackoffMax {
		*cur = s.cfg.BackoffMax // 封顶：后续退避保持最大值
	} else {
		*cur = next
	}
	return true
}

func randDuration(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)+1))
	if err != nil {
		return max / 2
	}
	return time.Duration(n.Int64())
}

func (s *Service) isDone() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// Release 优雅断开：停循环 → 关会话/连接 → 带超时等待（幂等）
func (s *Service) Release() {
	s.rel.Do(func() { close(s.done) })
	s.closeSession()

	ch := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(ch)
	}()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		log.Printf("[agent] release timed out waiting for loops")
	}
	log.Printf("[agent] released")
}
