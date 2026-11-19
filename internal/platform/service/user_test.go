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

// TestVerifyBuiltinPassword 内置管理员环境变量密码核对（恒定时间比较）。
func TestVerifyBuiltinPassword(t *testing.T) {
	cases := []struct {
		name              string
		username, envUser string
		envPass, password string
		want              bool
	}{
		{"完全匹配", "sysadmin", "sysadmin", "sysadmin123", "sysadmin123", true},
		{"用户名不符", "alice", "sysadmin", "sysadmin123", "sysadmin123", false},
		{"密码不符", "sysadmin", "sysadmin", "sysadmin123", "wrong", false},
		{"空密码", "sysadmin", "sysadmin", "sysadmin123", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifyBuiltinPassword(tc.username, tc.envUser, tc.envPass, tc.password); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
