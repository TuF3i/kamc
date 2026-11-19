package oidc

import (
	"context"
	"testing"
	"time"
)

// TestMemoryStateStoreBounded 未完成登录的 state 必须有界：
// 超过上限淘汰、时间前进后清理过期项，避免匿名请求耗尽内存。
func TestMemoryStateStoreBounded(t *testing.T) {
	now := time.Now()
	store := newMemoryStateStore()
	store.now = func() time.Time { return now }
	ctx := context.Background()

	for i := 0; i < maxMemoryStates+100; i++ {
		if err := store.Save(ctx, "state-"+itoa(i), now.Add(stateTTL)); err != nil {
			t.Fatalf("save: %v", err)
		}
	}
	if len(store.states) > maxMemoryStates {
		t.Fatalf("state store unbounded: %d", len(store.states))
	}

	// 全部过期后，再保存一次应触发清理，只剩新 state
	now = now.Add(stateTTL + time.Minute)
	if err := store.Save(ctx, "fresh", now.Add(stateTTL)); err != nil {
		t.Fatalf("save fresh: %v", err)
	}
	if len(store.states) != 1 {
		t.Fatalf("expired states should be swept, got %d", len(store.states))
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
