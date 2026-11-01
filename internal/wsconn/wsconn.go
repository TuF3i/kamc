// Package wsconn 把 WebSocket 连接适配为 net.Conn（字节流语义）。
//
// yamux 等多路复用协议要求 io.ReadWriteCloser，而 WebSocket 是消息帧协议；
// 本包用 binary frame 承载字节流：帧内数据按序拼接，读侧保留未消费完的残留字节。
//
// 两个关键约束（对应设计文档 §6.3 / BUG-4）：
//   - gorilla/websocket 同一时刻至多允许一个并发写者 → 写侧以互斥锁串行化
//     （yamux 内部虽有 sendLoop，但控制帧与并发流写必须统一加锁）；
//   - 读侧必须缓存"一帧中未被调用方缓冲区装下"的残留字节，直接截断会静默丢数据
//     （与 BUG-3 的 bufio 预读丢字节同源，务必保留）。
//
// 心跳不在本包内：WS ping/pong 由调用方按设计文档参数驱动（平台 ping 20s）。
package wsconn

import (
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Conn WebSocket → net.Conn 适配器，零值不可用，经 New 构造。
type Conn struct {
	conn *websocket.Conn

	// wmu 串行化写：gorilla 单写者约束（数据帧与 Close 控制帧共用）
	wmu sync.Mutex

	// rmu 保护残留缓冲；单读方（yamux 单读协程）下竞争面极小，但仍显式加锁
	rmu  sync.Mutex
	rbuf []byte

	closeOnce sync.Once // gorilla 底层 Close 不幂等，在此收敛为幂等
	closeErr  error
}

// New 包装一条已升级/已拨号的 WebSocket 连接。
func New(c *websocket.Conn) *Conn {
	return &Conn{conn: c}
}

// Read 按 net.Conn 语义读字节：先消费残留，再取下一条 binary frame。
// 非 binary 的消息（ping/pong 由库内部处理、text 帧不属于数据面）跳过不入流。
func (c *Conn) Read(b []byte) (int, error) {
	c.rmu.Lock()
	defer c.rmu.Unlock()
	for {
		if len(c.rbuf) > 0 {
			n := copy(b, c.rbuf)
			c.rbuf = c.rbuf[n:]
			return n, nil
		}
		mt, data, err := c.conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		c.rbuf = data
	}
}

// Write 将整包 b 作为一条 binary frame 写出（gorilla 无 partial write 概念）。
func (c *Conn) Write(b []byte) (int, error) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := c.conn.WriteMessage(websocket.BinaryMessage, b); err != nil {
		return 0, err
	}
	return len(b), nil
}

// Ping 发送 WS ping 控制帧（心跳用）。必须经本方法而非外部 WriteControl：
// 控制帧与数据帧共用 wmu 串行化，绕过锁会破坏 gorilla 单写者约束。
func (c *Conn) Ping() error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.conn.WriteControl(
		websocket.PingMessage,
		nil,
		time.Now().Add(10*time.Second),
	)
}

// Close 先发 Close 控制帧（1s 超时，尽力而为），再关闭底层连接。
// 幂等：gorilla 底层 Close 重复调用会报 use of closed connection，用 Once 收敛。
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		c.wmu.Lock()
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		)
		c.closeErr = c.conn.Close()
		c.wmu.Unlock()
	})
	return c.closeErr
}

// LocalAddr 底层 TCP 地址
func (c *Conn) LocalAddr() net.Addr { return c.conn.LocalAddr() }

// RemoteAddr 底层 TCP 地址
func (c *Conn) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }

// SetDeadline 同时设置读写超时
func (c *Conn) SetDeadline(t time.Time) error {
	if err := c.conn.SetReadDeadline(t); err != nil {
		return err
	}
	return c.conn.SetWriteDeadline(t)
}

// SetReadDeadline 设置读超时
func (c *Conn) SetReadDeadline(t time.Time) error { return c.conn.SetReadDeadline(t) }

// SetWriteDeadline 设置写超时
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.conn.SetWriteDeadline(t) }

// 编译期保证满足 net.Conn（yamux 的 io.ReadWriteCloser 要求随之满足）
var _ net.Conn = (*Conn)(nil)
