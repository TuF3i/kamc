package audit

// 集群审计日志文件出口（替代远程 syslog）：JSON Lines，供 Promtail 采集到 Loki。
//
// 配置（装配层经 Viper 装载）：
//   - audit.cluster_log.file    文件路径（空 = 不写文件）
//   - audit.cluster_log.max_mb  单文件大小上限（MB，默认 100）
//   - audit.cluster_log.backups 保留的轮转文件数（默认 5）
//
// 写入按批进行；超过上限时按时间戳轮转，Promtail 可用 *.jsonl* 通配读取。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// ClusterLogConfig 集群审计文件出口配置。
type ClusterLogConfig struct {
	File    string // 文件路径；空 = 不启用
	MaxMB   int    // 单文件上限（MB）
	Backups int    // 保留的轮转文件数
}

// ClusterLog JSON Lines 文件出口（并发安全，按大小轮转）。
type ClusterLog struct {
	path     string
	maxBytes int64
	backups  int

	mu   sync.Mutex
	file *os.File
	size int64
}

// NewClusterLog 打开/创建日志文件；File 为空返回 nil（不启用）。
func NewClusterLog(cfg ClusterLogConfig) (*ClusterLog, error) {
	path := strings.TrimSpace(cfg.File)
	if path == "" {
		return nil, nil
	}
	maxMB := cfg.MaxMB
	if maxMB <= 0 {
		maxMB = 100
	}
	backups := cfg.Backups
	if backups < 0 {
		backups = 0
	}
	if backups == 0 {
		backups = 5
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("audit -> create cluster log dir: %w", err)
	}
	l := &ClusterLog{
		path:     path,
		maxBytes: int64(maxMB) << 20,
		backups:  backups,
	}
	if err := l.openLocked(); err != nil {
		return nil, err
	}
	return l, nil
}

// Send 写入一批事件（每行一个 JSON 对象）。nil 接收者安全。
func (l *ClusterLog) Send(events []*modules.AuditEvent) error {
	if l == nil || len(events) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, ev := range events {
		payload, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("audit -> marshal cluster event: %w", err)
		}
		line := append(payload, '\n')
		if l.size+int64(len(line)) > l.maxBytes && l.size > 0 {
			if err := l.rotateLocked(); err != nil {
				return err
			}
		}
		n, err := l.file.Write(line)
		l.size += int64(n)
		if err != nil {
			return fmt.Errorf("audit -> write cluster log: %w", err)
		}
	}
	return l.file.Sync()
}

// Close 关闭文件（幂等）。
func (l *ClusterLog) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// Summary 启动日志摘要。
func (l *ClusterLog) Summary() string {
	if l == nil {
		return "disabled"
	}
	return fmt.Sprintf("%s (max %dMB x %d)", l.path, l.maxBytes>>20, l.backups)
}

func (l *ClusterLog) openLocked() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("audit -> open cluster log %s: %w", l.path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("audit -> stat cluster log: %w", err)
	}
	l.file = f
	l.size = info.Size()
	return nil
}

func (l *ClusterLog) rotateLocked() error {
	if l.file != nil {
		if err := l.file.Close(); err != nil {
			return fmt.Errorf("audit -> close cluster log: %w", err)
		}
		l.file = nil
	}
	rotated := fmt.Sprintf("%s.%s", l.path, time.Now().Format("20060102-150405"))
	if _, err := os.Stat(rotated); err == nil {
		rotated = fmt.Sprintf("%s.%d", rotated, time.Now().UnixNano())
	}
	if err := os.Rename(l.path, rotated); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("audit -> rotate cluster log: %w", err)
	}
	l.pruneLocked()
	return l.openLocked()
}

// pruneLocked 只保留最近 backups 个轮转文件（按修改时间）。
func (l *ClusterLog) pruneLocked() {
	if l.backups <= 0 {
		return
	}
	matches, err := filepath.Glob(l.path + ".*")
	if err != nil || len(matches) <= l.backups {
		return
	}
	sort.Slice(matches, func(i, j int) bool {
		si, _ := os.Stat(matches[i])
		sj, _ := os.Stat(matches[j])
		if si == nil || sj == nil {
			return matches[i] > matches[j]
		}
		return si.ModTime().After(sj.ModTime())
	})
	for _, path := range matches[l.backups:] {
		_ = os.Remove(path)
	}
}
