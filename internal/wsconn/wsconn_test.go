package wsconn

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newEchoServer 启动一个把 binary 消息原样回发的 WebSocket 测试服务。
func newEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			if err := c.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dialEcho(t *testing.T, srv *httptest.Server) *Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	wc, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = wc.Close() })
	return New(wc)
}

// TestReadResidualBuffered 验证读侧残留缓冲：一帧 11 字节，调用方每次只读 4 字节，
// 分 3 次读完且内容无损（直接截断会丢第 5 字节之后的残留——BUG-3 同源问题）。
func TestReadResidualBuffered(t *testing.T) {
	c := dialEcho(t, newEchoServer(t))

	const msg = "hello world"
	if _, err := c.Write([]byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}

	var got []byte
	buf := make([]byte, 4)
	for len(got) < len(msg) {
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := c.Read(buf)
		if err != nil {
			t.Fatalf("read after %d bytes: %v", len(got), err)
		}
		got = append(got, buf[:n]...)
	}
	if string(got) != msg {
		t.Fatalf("roundtrip mismatch: got %q want %q", got, msg)
	}
}

// TestWriteIsMessageFramed 验证一次 Write 对应一条完整 binary frame。
func TestWriteIsMessageFramed(t *testing.T) {
	c := dialEcho(t, newEchoServer(t))

	if _, err := c.Write([]byte("abc")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := c.Write([]byte("defg")); err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, want := range []string{"abc", "defg"} {
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		mt, data, err := c.conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if mt != websocket.BinaryMessage || string(data) != want {
			t.Fatalf("frame mismatch: mt=%d data=%q want %q", mt, data, want)
		}
	}
}

// TestCloseIdempotent 验证 Close 可重复调用（幂等约定）。
func TestCloseIdempotent(t *testing.T) {
	c := dialEcho(t, newEchoServer(t))
	if err := c.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
