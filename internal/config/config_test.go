package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFileEnvPrecedence 文件提供默认值，环境变量覆盖，未配置键取默认。
func TestFileEnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kamc.yaml")
	content := []byte(`
server:
  listen_addr: ":9090"
  external_url: "https://file.example.com"
auth:
  password_login:
    enabled: false
redis:
  addresses: "redis-a:6379,redis-b:6379"
audit:
  retention: "48h"
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	l := New()
	l.SetDefault("server.listen_addr", ":8080")
	l.SetDefault("audit.retention", 720*time.Hour)
	l.Bind("server.external_url", "TEST_CFG_EXTERNAL_URL")
	l.Bind("auth.password_login.enabled", "TEST_CFG_PASSWORD_LOGIN")
	if err := l.LoadFile(path, "kamc"); err != nil {
		t.Fatalf("load: %v", err)
	}

	if got := l.String("server.listen_addr"); got != ":9090" {
		t.Fatalf("file value: %q", got)
	}
	t.Setenv("TEST_CFG_EXTERNAL_URL", "https://env.example.com")
	if got := l.String("server.external_url"); got != "https://env.example.com" {
		t.Fatalf("env should override file: %q", got)
	}
	if l.Bool("auth.password_login.enabled", true) {
		t.Fatal("file bool false not applied")
	}
	if got := l.Duration("audit.retention", time.Hour); got != 48*time.Hour {
		t.Fatalf("duration: %v", got)
	}
	if got := l.StringList("redis.addresses"); len(got) != 2 || got[1] != "redis-b:6379" {
		t.Fatalf("list: %v", got)
	}
	if got := l.FileUsed(); got != path {
		t.Fatalf("file used: %q", got)
	}
}

// TestBoolEnvAndZeroDuration 环境变量的 0/1 与 AM_AUDIT_RETENTION=0 语义。
func TestBoolEnvAndZeroDuration(t *testing.T) {
	t.Setenv("TEST_CFG_PASSWORD_LOGIN", "0")
	t.Setenv("TEST_CFG_RETENTION", "0")
	l := New()
	l.SetDefault("auth.password_login.enabled", true)
	l.SetDefault("audit.retention", 720*time.Hour)
	l.Bind("auth.password_login.enabled", "TEST_CFG_PASSWORD_LOGIN")
	l.Bind("audit.retention", "TEST_CFG_RETENTION")
	if err := l.LoadFile("", "kamc-not-exist"); err != nil {
		t.Fatalf("load missing file should be ignored: %v", err)
	}
	if l.Bool("auth.password_login.enabled", true) {
		t.Fatal("env 0 should disable")
	}
	if got := l.Duration("audit.retention", 720*time.Hour); got != 0 {
		t.Fatalf("retention 0 should disable: %v", got)
	}
}

// TestExplicitFileMissing 显式指定的文件不存在必须报错（避免静默用默认值）。
func TestExplicitFileMissing(t *testing.T) {
	l := New()
	if err := l.LoadFile(filepath.Join(t.TempDir(), "nope.yaml"), "kamc"); err == nil {
		t.Fatal("explicit missing file should error")
	}
}
