package handler

import "testing"

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
