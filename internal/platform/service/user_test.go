package service

import "testing"

// TestMergeOIDCSub 覆盖用户 metadata 中的 SSO 稳定标识：写入不丢其它键，
// 本地/LDAP 登录（sub 为空）不覆盖已有值。
func TestMergeOIDCSub(t *testing.T) {
	cases := []struct {
		name, raw, sub, want string
	}{
		{"空 metadata 写入 sub", "{}", "abc", `{"oidc_sub":"abc"}`},
		{"保留已有键", `{"dept":"平台研发"}`, "abc", `{"dept":"平台研发","oidc_sub":"abc"}`},
		{"本地登录不改动", `{"oidc_sub":"abc"}`, "", `{"oidc_sub":"abc"}`},
		{"空 raw + 空 sub", "", "", "{}"},
		{"非法 raw 重置为仅 sub", "not-json", "abc", `{"oidc_sub":"abc"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mergeOIDCSub(tc.raw, tc.sub); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
