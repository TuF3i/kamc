package oidc

import "testing"

// TestIdentityFromClaims 覆盖 SSO 身份映射：用户名与展示名都不应回落到
// Authentik 的 sub 哈希（除非 IdP 完全没给 preferred_username/email）。
func TestIdentityFromClaims(t *testing.T) {
	cases := []struct {
		name               string
		sub, email         string
		fullName           string
		preferred, picture string
		wantUser           string
		wantNick           string
	}{
		{
			name:  "authentik 常见形态：preferred_username + name",
			sub:   "d14f4deb417119edad313ed5aea282f54ddd6183109d8cd2977ae1d166e161cc",
			email: "sre@redrock.team", fullName: "SRE 管理员", preferred: "akadmin",
			wantUser: "akadmin", wantNick: "SRE 管理员",
		},
		{
			name: "无 name 时展示名回落到用户名",
			sub:  "hash", email: "alice@example.com", preferred: "alice",
			wantUser: "alice", wantNick: "alice",
		},
		{
			name: "无 preferred 时用 email",
			sub:  "hash", email: "bob@example.com",
			wantUser: "bob@example.com", wantNick: "bob@example.com",
		},
		{
			name:     "什么都没有才用 sub",
			sub:      "hash-only",
			wantUser: "hash-only", wantNick: "hash-only",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := identityFromClaims(tc.sub, tc.email, tc.fullName, tc.preferred, tc.picture, []string{"am:admin"})
			if u.Username != tc.wantUser {
				t.Fatalf("username: got %q want %q", u.Username, tc.wantUser)
			}
			if u.Nickname != tc.wantNick {
				t.Fatalf("nickname: got %q want %q", u.Nickname, tc.wantNick)
			}
			if u.Sub != tc.sub {
				t.Fatalf("sub: got %q want %q", u.Sub, tc.sub)
			}
		})
	}
}
