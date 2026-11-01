package audit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// memSink 内存收集（可注入失败）
type memSink struct {
	mu     sync.Mutex
	events []*modules.AuditEvent
	fail   bool
}

func (m *memSink) InsertBatch(_ context.Context, events []*modules.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("sink down")
	}
	m.events = append(m.events, events...)
	return nil
}

func (m *memSink) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func newEvent(user string) *modules.AuditEvent {
	return &modules.AuditEvent{Timestamp: time.Now(), User: user, Cluster: "c1", Allowed: true, Reason: "allowed"}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", msg)
}

// TestWriterBatchFlush 事件聚合批量写出。
func TestWriterBatchFlush(t *testing.T) {
	sink := &memSink{}
	w := NewWriter(sink, filepath.Join(t.TempDir(), "spill.jsonl"))
	w.Start()
	defer w.Release()

	for i := 0; i < 5; i++ {
		w.Log(newEvent("alice"))
	}
	waitFor(t, 2*time.Second, func() bool { return sink.count() == 5 }, "all events flushed")
}

// TestWriterSpillAndReplay sink 故障 → spill 文件；恢复后新批次触发回放并清空。
func TestWriterSpillAndReplay(t *testing.T) {
	spill := filepath.Join(t.TempDir(), "spill.jsonl")
	sink := &memSink{fail: true}
	w := NewWriter(sink, spill)
	w.Start()

	for i := 0; i < 3; i++ {
		w.Log(newEvent("bob"))
	}
	waitFor(t, 2*time.Second, func() bool { _, err := os.Stat(spill); return err == nil }, "spill file created")

	// sink 恢复 → 下一批写出前回放
	sink.mu.Lock()
	sink.fail = false
	sink.mu.Unlock()
	w.Log(newEvent("carol"))
	waitFor(t, 3*time.Second, func() bool { return sink.count() >= 4 }, "spilled events replayed")
	if _, err := os.Stat(spill); !os.IsNotExist(err) {
		t.Fatal("spill file should be removed after replay")
	}
	if sink.count() != 4 {
		t.Fatalf("events: %d", sink.count())
	}
}

// TestWriterReleaseDrains Release 排空缓冲。
func TestWriterReleaseDrains(t *testing.T) {
	sink := &memSink{}
	w := NewWriter(sink, filepath.Join(t.TempDir(), "spill.jsonl"))
	w.Start()
	for i := 0; i < 10; i++ {
		w.Log(newEvent("dave"))
	}
	w.Release()
	if sink.count() != 10 {
		t.Fatalf("release should drain buffer: %d", sink.count())
	}
}

// TestWriterSpillFormat spill 行为合法 JSON（回放兼容）。
func TestWriterSpillFormat(t *testing.T) {
	spill := filepath.Join(t.TempDir(), "spill.jsonl")
	sink := &memSink{fail: true}
	w := NewWriter(sink, spill)
	w.Start()
	w.Log(newEvent("eve"))
	waitFor(t, 2*time.Second, func() bool { _, err := os.Stat(spill); return err == nil }, "spill created")
	w.Release()

	raw, _ := os.ReadFile(spill)
	var ev modules.AuditEvent
	if err := json.Unmarshal(raw[:len(raw)-1], &ev); err != nil {
		t.Fatalf("spill line should be valid json: %v", err)
	}
}
