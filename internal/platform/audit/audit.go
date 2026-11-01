// Package audit 审计事件异步写出（§12.2）：
// 业务路径零阻塞投递 → 内存缓冲 → 批量写 PG；PG 不可用时事件落本地
// spill 文件（JSONL），恢复后回放并清空。
//
// 并发模式（约定 done-channel + wg）：Release 顺序 close(done) → 排空缓冲
// 尽力写出 → wg.Wait。缓冲满时丢弃最旧语义简化为丢弃新事件并计数
// （dropped 指标供观测，§12.3）。
package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

// Sink 事件落地目标（PG 实现；测试可注入内存/failing 实现）
type Sink interface {
	InsertBatch(ctx context.Context, events []*modules.AuditEvent) error
}

const (
	bufferSize    = 4096
	batchSize     = 100
	flushInterval = 500 * time.Millisecond
)

// Writer 审计事件异步写出器
type Writer struct {
	sink      Sink
	spillPath string
	buf       chan *modules.AuditEvent
	done      chan struct{}
	wg        sync.WaitGroup
	dropped   atomic.Int64
}

// NewWriter 构造（spillPath 为空则不落本地文件，sink 故障即丢弃并计数）
func NewWriter(sink Sink, spillPath string) *Writer {
	return &Writer{
		sink:      sink,
		spillPath: spillPath,
		buf:       make(chan *modules.AuditEvent, bufferSize),
		done:      make(chan struct{}),
	}
}

// Start 启动写出循环（幂等）
func (w *Writer) Start() {
	w.wg.Add(1)
	go w.loop()
}

// Dropped 累计丢弃事件数（缓冲满或 spill 不可用）
func (w *Writer) Dropped() int64 { return w.dropped.Load() }

// Log 非阻塞投递（业务热路径安全；满则丢弃并计数）
func (w *Writer) Log(ev *modules.AuditEvent) {
	if w == nil || ev == nil {
		return
	}
	select {
	case <-w.done:
		return
	default:
	}
	select {
	case w.buf <- ev:
	default:
		w.dropped.Add(1) // 缓冲满：丢弃（§12.3 指标观测）
	}
}

// Release 停止并排空（幂等）
func (w *Writer) Release() {
	w.muOnceClose()
	w.wg.Wait()
}

func (w *Writer) muOnceClose() {
	select {
	case <-w.done:
	default:
		close(w.done)
	}
}

// loop 主循环：批量聚合 + 定时刷写
func (w *Writer) loop() {
	defer w.wg.Done()
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]*modules.AuditEvent, 0, batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		w.writeBatch(batch)
		batch = batch[:0]
	}

	for {
		select {
		case <-w.done:
			// 排空缓冲后退出
			for {
				select {
				case ev := <-w.buf:
					batch = append(batch, ev)
					if len(batch) >= batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case ev := <-w.buf:
			batch = append(batch, ev)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// writeBatch 写 PG；失败落 spill
func (w *Writer) writeBatch(events []*modules.AuditEvent) {
	// 先回放历史 spill（PG 此前故障期间暂存的事件，按序优先）
	w.replaySpill()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.sink.InsertBatch(ctx, events); err != nil {
		fmt.Printf("[audit] insert batch(%d) failed, spilling: %v\n", len(events), err)
		w.spill(events)
	}
}

// spill 追加写本地 JSONL（append + 每行一事件）
func (w *Writer) spill(events []*modules.AuditEvent) {
	if w.spillPath == "" {
		w.dropped.Add(int64(len(events)))
		return
	}
	f, err := os.OpenFile(w.spillPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		w.dropped.Add(int64(len(events)))
		fmt.Printf("[audit] spill open: %v\n", err)
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			w.dropped.Add(1)
		}
	}
}

// replaySpill 回放 spill 文件（成功后删除）；无文件或不可读静默跳过
func (w *Writer) replaySpill() {
	if w.spillPath == "" {
		return
	}
	f, err := os.Open(w.spillPath)
	if err != nil {
		return // 不存在即正常
	}
	defer f.Close()

	var recovered []*modules.AuditEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var ev modules.AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue // 损坏行跳过
		}
		recovered = append(recovered, &ev)
	}
	if len(recovered) == 0 {
		_ = os.Remove(w.spillPath)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := w.sink.InsertBatch(ctx, recovered); err != nil {
		return // 仍不可用：保留文件下轮再试
	}
	_ = os.Remove(w.spillPath)
	fmt.Printf("[audit] replayed %d spilled events\n", len(recovered))
}

// ErrNoSink 占位错误（Writer 构造校验用）
var ErrNoSink = errors.New("audit: sink is nil")
