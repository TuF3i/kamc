package audit

// Syslog 导出（集群操作日志专用）：RFC5424 over UDP/TCP，JSON 作为 MSG。
//
// 环境变量（装配层传入）：
//   - AM_SYSLOG_ADDR        udp://host:514、tcp://host:514 或 tls://host:6514（空 = 不导出）
//   - AM_SYSLOG_TAG         应用名，默认 kamc-cluster-audit
//   - AM_SYSLOG_CA_FILE     tls 模式可选 CA（默认系统根证书）
//   - AM_SYSLOG_INSECURE    tls 模式跳过证书校验（仅开发，默认 false）

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

const (
	syslogFacility = 16 // local0
	syslogSeverity = 6  // info
)

// Syslog 远程日志导出器（懒连接；发送失败计数并丢弃，不阻塞审计主循环）
type Syslog struct {
	network string // udp / tcp / tls
	addr    string
	tag     string
	host    string
	tlsConf *tls.Config
	failed  atomic.Int64

	mu   sync.Mutex
	conn net.Conn
}

// NewSyslogFromEnv 按 AM_SYSLOG_ADDR 装配（未配置返回 nil）。
func NewSyslogFromEnv() *Syslog {
	raw := strings.TrimSpace(os.Getenv("AM_SYSLOG_ADDR"))
	if raw == "" {
		return nil
	}
	network, addr := "udp", raw
	if n, a, ok := strings.Cut(raw, "://"); ok {
		network = strings.ToLower(n)
		addr = a
	}
	if network != "udp" && network != "tcp" && network != "tls" {
		return nil
	}
	tag := os.Getenv("AM_SYSLOG_TAG")
	if tag == "" {
		tag = "kamc-cluster-audit"
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "-"
	}
	cfg := &Syslog{network: network, addr: addr, tag: tag, host: host}
	if network == "tls" {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if caFile := strings.TrimSpace(os.Getenv("AM_SYSLOG_CA_FILE")); caFile != "" {
			pem, err := os.ReadFile(caFile)
			if err != nil {
				return nil
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil
			}
			tlsCfg.RootCAs = pool
		}
		if os.Getenv("AM_SYSLOG_INSECURE") == "1" {
			tlsCfg.InsecureSkipVerify = true
		}
		cfg.tlsConf = tlsCfg
	}
	return cfg
}

// Failed 累计发送失败次数
func (s *Syslog) Failed() int64 { return s.failed.Load() }

// Send 批量发送（每事件一条 RFC5424 消息，JSON 为 MSG）
func (s *Syslog) Send(events []*modules.AuditEvent) {
	if s == nil || len(events) == 0 {
		return
	}
	for _, ev := range events {
		if err := s.sendOne(ev); err != nil {
			s.failed.Add(1)
			s.dropConn()
			return // 连接问题：本批剩余不重试，计数即可（DB 仍有完整记录）
		}
	}
}

func (s *Syslog) sendOne(ev *modules.AuditEvent) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	pri := syslogFacility*8 + syslogSeverity
	msg := fmt.Sprintf("<%d>1 %s %s %s %d - - %s",
		pri, ev.Timestamp.UTC().Format(time.RFC3339Nano), s.host, s.tag, os.Getpid(),
		strings.ReplaceAll(string(payload), "\n", " "))

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		var conn net.Conn
		var err error
		if s.network == "tls" {
			dialer := &net.Dialer{Timeout: 2 * time.Second}
			conn, err = tls.DialWithDialer(dialer, "tcp", s.addr, s.tlsConf)
		} else {
			conn, err = net.DialTimeout(s.network, s.addr, 2*time.Second)
		}
		if err != nil {
			return err
		}
		s.conn = conn
	}
	_ = s.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = s.conn.Write([]byte(msg))
	return err
}

func (s *Syslog) dropConn() {
	s.mu.Lock()
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
	s.mu.Unlock()
}

// Close 释放连接（幂等）
func (s *Syslog) Close() {
	if s == nil {
		return
	}
	s.dropConn()
}

// Summary 便于启动日志输出
func (s *Syslog) Summary() string {
	if s == nil {
		return "disabled"
	}
	return s.network + "://" + s.addr + " (tag=" + s.tag + ", pri=" + strconv.Itoa(syslogFacility*8+syslogSeverity) + ")"
}
