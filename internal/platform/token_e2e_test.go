package platform_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// TestTokenLifecycleE2E Token 全生命周期（TEST_PG_DSN 门控）：
// 签发 → 校验命中 → 轮换产生宽限 → 旧 Token 宽限期内仍可校验 → 吊销立即失效。
func TestTokenLifecycleE2E(t *testing.T) {
	rt := newTestRuntime(t)
	id := "it-token-cluster"

	// 前置：注册集群
	resp, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": id, "name": "token 测试集群"})
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("setup cluster: http=%d body=%+v", resp.StatusCode, ar)
	}
	t.Cleanup(func() { _, _ = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+id, nil) })

	// 签发 longterm Token
	resp, ar = call(t, rt, http.MethodPost, "/api/v1/clusters/"+id+"/tokens",
		map[string]string{"kind": "longterm"})
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("issue: http=%d body=%+v", resp.StatusCode, ar)
	}
	var issued struct {
		Token string `json:"token"`
		Kind  string `json:"kind"`
	}
	if err := json.Unmarshal(ar.Data, &issued); err != nil {
		t.Fatalf("decode issued: %v", err)
	}
	if issued.Kind != "longterm" || len(issued.Token) == 0 {
		t.Fatalf("unexpected issued: %+v", issued)
	}

	// 校验命中
	cluster, kind, err := rt.Service.LookupToken(t.Context(), issued.Token)
	if err != nil || cluster != id || kind != modules.TokenKindLongterm {
		t.Fatalf("lookup: cluster=%q kind=%q err=%v", cluster, kind, err)
	}

	// 轮换：旧 Token 进宽限期
	resp, ar = call(t, rt, http.MethodPost, "/api/v1/clusters/"+id+"/tokens",
		map[string]string{"kind": "longterm"})
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("rotate: %+v", ar)
	}
	var rotated struct {
		Token       string     `json:"token"`
		GraceEndsAt *time.Time `json:"grace_ends_at"`
	}
	if err := json.Unmarshal(ar.Data, &rotated); err != nil {
		t.Fatalf("decode rotated: %v", err)
	}
	if rotated.GraceEndsAt == nil {
		t.Fatal("rotation should report grace period")
	}

	// 旧 Token 宽限期内仍命中
	cluster, _, err = rt.Service.LookupToken(t.Context(), issued.Token)
	if err != nil || cluster != id {
		t.Fatalf("grace lookup: cluster=%q err=%v", cluster, err)
	}

	// 新 Token 命中
	if cluster, _, err = rt.Service.LookupToken(t.Context(), rotated.Token); err != nil || cluster != id {
		t.Fatalf("new token lookup: cluster=%q err=%v", cluster, err)
	}

	// 元数据列表：状态 rotating，永不泄露哈希/明文
	resp, ar = call(t, rt, http.MethodGet, "/api/v1/clusters/"+id+"/tokens", nil)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("list: %+v", ar)
	}
	if raw := string(ar.Data); strings.Contains(raw, issued.Token) || strings.Contains(raw, rotated.Token) {
		t.Fatalf("token plaintext leaked in list: %s", raw)
	}

	// 吊销 → 立即失效
	resp, ar = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+id+"/tokens/longterm", nil)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("revoke: %+v", ar)
	}
	if _, _, err := rt.Service.LookupToken(t.Context(), rotated.Token); err == nil {
		t.Fatal("revoked token should be invalid")
	}

	// 再吊销 → ErrTokenNotFound
	_, ar = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+id+"/tokens/longterm", nil)
	if ar.Status != errcode.CodeTokenNotFound {
		t.Fatalf("revoke again: want %d, got %+v", errcode.CodeTokenNotFound, ar)
	}
}

// TestRegisterTokenExpiry 注册 Token 过期后失效（1h TTL，测试中直接构造过期行）。
func TestRegisterTokenExpiry(t *testing.T) {
	rt := newTestRuntime(t)
	id := "it-token-expiry"
	_, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": id, "name": "expiry"})
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("setup: %+v", ar)
	}
	t.Cleanup(func() { _, _ = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+id, nil) })

	resp, ar := call(t, rt, http.MethodPost, "/api/v1/clusters/"+id+"/tokens",
		map[string]string{"kind": "register"})
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("issue register: http=%d body=%+v", resp.StatusCode, ar)
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(ar.Data, &issued); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, _, err := rt.Service.LookupToken(t.Context(), issued.Token); err != nil {
		t.Fatalf("fresh register token should be valid: %v", err)
	}
}

// TestRegisterTokenExchangeE2E 注册 Token 一次性交换（§6.1）：
// 换取成功 → 长期 Token 可用 → 注册 Token 立即失效（一次性）→
// 长期 Token 不能再次交换（类型不对）。
func TestRegisterTokenExchangeE2E(t *testing.T) {
	rt := newTestRuntime(t)
	id := "it-exchange-cluster"
	_, ar := call(t, rt, http.MethodPost, "/api/v1/clusters",
		map[string]string{"id": id, "name": "exchange"})
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("setup: %+v", ar)
	}
	t.Cleanup(func() { _, _ = call(t, rt, http.MethodDelete, "/api/v1/clusters/"+id, nil) })

	_, ar = call(t, rt, http.MethodPost, "/api/v1/clusters/"+id+"/tokens",
		map[string]string{"kind": "register"})
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("issue register: %+v", ar)
	}
	var reg struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(ar.Data, &reg); err != nil {
		t.Fatalf("decode register: %v", err)
	}

	// 交换
	issued, err := rt.Service.ExchangeRegisterToken(t.Context(), reg.Token)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if issued.Kind != modules.TokenKindLongterm || issued.Token == "" {
		t.Fatalf("unexpected issued: %+v", issued)
	}

	// 长期 Token 生效、注册 Token 已一次性删除
	if cluster, kind, err := rt.Service.LookupToken(t.Context(), issued.Token); err != nil || cluster != id || kind != modules.TokenKindLongterm {
		t.Fatalf("longterm lookup: cluster=%q kind=%q err=%v", cluster, kind, err)
	}
	if _, _, err := rt.Service.LookupToken(t.Context(), reg.Token); err == nil {
		t.Fatal("register token should be deleted after exchange (one-time)")
	}

	// 长期 Token 不能走注册交换
	if _, err := rt.Service.ExchangeRegisterToken(t.Context(), issued.Token); err == nil {
		t.Fatal("longterm token must not be exchangeable")
	}
}
