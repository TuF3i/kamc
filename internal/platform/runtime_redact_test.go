package platform

import "testing"

// TestRedactDSN DSN 脱敏必须覆盖 URL userinfo、URL query 与 keyword 三种形态，
// 任何形态都不能把数据库密码回显到 /api/v1/platform/info。
func TestRedactDSN(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "url userinfo",
			in:   "postgres://app:S3cret@db.internal:5432/kamc?sslmode=disable",
			want: "postgres://app:***@db.internal:5432/kamc?sslmode=disable",
		},
		{
			name: "url query password",
			in:   "postgres://app@db.internal:5432/kamc?password=S3cret&sslmode=disable",
			want: "postgres://app@db.internal:5432/kamc?password=%2A%2A%2A&sslmode=disable",
		},
		{
			name: "keyword form",
			in:   "host=db user=app password=S3cret dbname=kamc sslmode=require",
			want: "host=db user=app password=*** dbname=kamc sslmode=require",
		},
		{name: "empty", in: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactDSN(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// TestValidateProductionGuards 生产护栏：dev-token / 缺省 DSN / 派生头像密钥
// 必须拒绝启动；开发环境不受影响。
func TestValidateProductionGuards(t *testing.T) {
	t.Setenv("AM_AVATAR_SECRET", "s3cret")
	t.Setenv("DATABASE_URL", "")

	if err := validateProductionGuards(false, false, true); err != nil {
		t.Fatalf("dev should allow: %v", err)
	}
	if err := validateProductionGuards(true, false, true); err == nil {
		t.Fatal("production + dev-token should be rejected")
	}
	if err := validateProductionGuards(true, false, false); err == nil {
		t.Fatal("production without DATABASE_URL should be rejected")
	}

	t.Setenv("DATABASE_URL", "host=db user=app password=secret dbname=kamc")
	if err := validateProductionGuards(true, false, false); err != nil {
		t.Fatalf("production with DSN: %v", err)
	}
	if err := validateProductionGuards(true, true, false); err != nil {
		t.Fatalf("injected DB should skip DSN check: %v", err)
	}

	t.Setenv("AM_AVATAR_SECRET", "")
	if err := validateProductionGuards(true, true, false); err == nil {
		t.Fatal("production without AM_AVATAR_SECRET should be rejected")
	}
}
