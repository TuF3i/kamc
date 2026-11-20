package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

func TestClusterLogWritesJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cluster.jsonl")
	l, err := NewClusterLog(ClusterLogConfig{File: path, MaxMB: 100, Backups: 2})
	if err != nil {
		t.Fatalf("NewClusterLog: %v", err)
	}
	defer l.Close()

	events := []*modules.AuditEvent{
		{Kind: modules.AuditKindCluster, Event: "k8s-api", User: "alice", Timestamp: time.Now()},
		{Kind: modules.AuditKindCluster, Event: "k8s-api", User: "bob", Timestamp: time.Now()},
	}
	if err := l.Send(events); err != nil {
		t.Fatalf("Send: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var count int
	for scanner.Scan() {
		var ev modules.AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			t.Fatalf("decode line: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("want 2 lines, got %d", count)
	}
}

func TestClusterLogRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cluster.jsonl")
	l := &ClusterLog{path: path, maxBytes: 300, backups: 2}
	if err := l.openLocked(); err != nil {
		t.Fatalf("open: %v", err)
	}
	defer l.Close()

	// 单条事件超过 maxBytes，触发多次轮转
	ev := &modules.AuditEvent{Kind: modules.AuditKindCluster, Event: "k8s-api", User: "alice", Timestamp: time.Now()}
	for i := 0; i < 10; i++ {
		if err := l.Send([]*modules.AuditEvent{ev}); err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}
	matches, err := filepath.Glob(path + ".*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected rotated files")
	}
	if len(matches) > l.backups {
		t.Fatalf("backups not pruned: %d > %d", len(matches), l.backups)
	}
}

func TestClusterLogDisabled(t *testing.T) {
	l, err := NewClusterLog(ClusterLogConfig{})
	if err != nil || l != nil {
		t.Fatalf("empty file should disable: %v %v", l, err)
	}
	if err := l.Send(nil); err != nil {
		t.Fatalf("nil receiver Send: %v", err)
	}
	l.Close()
}
