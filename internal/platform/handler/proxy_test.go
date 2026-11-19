package handler

import (
	"net/http/httptest"
	"testing"
)

// TestStripCredentialHeadersAndQuery 覆盖数据面净化：平台 JWT/Cookie 与凭证
// query 不得随流式升级进入 Agent，业务参数必须保留。
func TestStripCredentialHeadersAndQuery(t *testing.T) {
	r := httptest.NewRequest("GET",
		"/proxy/cluster-a/api/v1/namespaces/dev/pods/pod-1/exec?access_token=secret&token=t2&stdin=1&stdout=1&command=ls", nil)
	r.Header.Set("Authorization", "Bearer platform-jwt")
	r.Header.Set("Proxy-Authorization", "Basic abc")
	r.Header.Set("Cookie", "session=abc")
	r.Header.Set("X-Stream-Protocol-Version", "v4.channel.k8s.io")

	stripCredentialHeaders(r)
	stripCredentialQuery(r.URL)

	if got := r.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization leaked: %q", got)
	}
	if got := r.Header.Get("Proxy-Authorization"); got != "" {
		t.Fatalf("Proxy-Authorization leaked: %q", got)
	}
	if got := r.Header.Get("Cookie"); got != "" {
		t.Fatalf("Cookie leaked: %q", got)
	}
	if got := r.Header.Get("X-Stream-Protocol-Version"); got != "v4.channel.k8s.io" {
		t.Fatalf("business header dropped: %q", got)
	}

	q := r.URL.Query()
	if q.Get("access_token") != "" || q.Get("token") != "" {
		t.Fatalf("credential query leaked: %v", q)
	}
	if q.Get("stdin") != "1" || q.Get("stdout") != "1" || q.Get("command") != "ls" {
		t.Fatalf("business query dropped: %v", q)
	}
}

// TestStripCredentialQueryNoop 无凭证参数时不应重写查询串（避免改变参数顺序）。
func TestStripCredentialQueryNoop(t *testing.T) {
	r := httptest.NewRequest("GET", "/proxy/c/api/v1/pods?limit=500&watch=1", nil)
	before := r.URL.RawQuery
	stripCredentialQuery(r.URL)
	if r.URL.RawQuery != before {
		t.Fatalf("query rewritten unexpectedly: %q -> %q", before, r.URL.RawQuery)
	}
}
