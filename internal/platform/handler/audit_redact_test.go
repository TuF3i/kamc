package handler

import (
	"testing"

	"github.com/TuF3i/kamc/internal/platform/modules"
)

func TestRedactAuditURI(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/proxy/c/api/v1/nodes", "/proxy/c/api/v1/nodes"},
		{"/proxy/c/exec?command=sh&command=-c&command=id&access_token=secret", "/proxy/c/exec?access_token=%2A%2A%2A&command=sh&command=-c&command=id"},
		{"/auth/callback?code=x&id_token=abc", "/auth/callback?code=x&id_token=%2A%2A%2A"},
		{"/x?token=a&Token=b", "/x?Token=%2A%2A%2A&token=%2A%2A%2A"},
	}
	for _, tc := range cases {
		if got := redactAuditURI(tc.in); got != tc.want {
			t.Fatalf("redact %q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

// TestRedactAuditURIFailClosed 畸形 query 不得原样返回：整段 query 丢弃，
// 防止攻击者用非法编码把 access_token 写进审计库/日志平台。
func TestRedactAuditURIFailClosed(t *testing.T) {
	got := redactAuditURI("/proxy/c/api/v1/pods?x=%zz&access_token=secret")
	if got != "/proxy/c/api/v1/pods?<redacted>" {
		t.Fatalf("malformed query: got %q", got)
	}
}

// TestCapAuditEvent 超长字段按列宽截断（rune 安全），避免整批审计写入失败。
func TestCapAuditEvent(t *testing.T) {
	long := make([]rune, 3000)
	for i := range long {
		long[i] = '中'
	}
	ev := capAuditEvent(&modules.AuditEvent{
		Path:   string(long),
		Object: string(long),
		User:   string(long),
		Groups: string(long),
	})
	if len([]rune(ev.Path)) != 1024 || len([]rune(ev.Object)) != 1024 {
		t.Fatalf("path/object not capped: %d/%d", len([]rune(ev.Path)), len([]rune(ev.Object)))
	}
	if len([]rune(ev.User)) != 128 || len([]rune(ev.Groups)) != 1024 {
		t.Fatalf("user/groups not capped: %d/%d", len([]rune(ev.User)), len([]rune(ev.Groups)))
	}
}
