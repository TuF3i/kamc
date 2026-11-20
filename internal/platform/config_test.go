package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadConfigFileAndEnv 配置文件提供结构，环境变量覆盖同键（部署契约不变）。
func TestLoadConfigFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kamc.yaml")
	content := []byte(`
server:
  listen_addr: ":9999"
  env: production
  external_url: https://file.example.com
database:
  url: postgres://app:file@db:5432/kamc
jwt:
  issuer: https://file-issuer.example.com
  audience: kamc
auth:
  admin_group: file-admins
  password_login:
    enabled: false
  dev_token: false
oidc:
  issuer: https://sso.example.com/application/o/kamc/
  client_id: kamc
  client_secret: file-secret
  redirect_url: https://file.example.com/auth/callback
redis:
  addresses:
    - redis-a:6379
    - redis-b:6379
  password: redis-pass
  db: 3
tunnel:
  peer:
    addr: ":19090"
    token: peer-token
    addresses: "pod-a:9090,pod-b:9090"
audit:
  spill_file: /tmp/spill.jsonl
  retention: 48h
  cluster_log:
    file: /tmp/cluster-audit.jsonl
    max_mb: 50
    backups: 3
avatar:
  dir: /tmp/avatar
  secret: avatar-file-secret
rate_limit:
  proxy: 100
  login: 5
geoip:
  db: /tmp/geo.mmdb
agent:
  image_repo: reg.example.com/kamc/agent
web:
  dir: /tmp/web
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AM_ADMIN_GROUP", "env-admins") // 环境变量覆盖文件
	t.Setenv("REDIS_DB", "5")
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.ListenAddr != ":9999" || cfg.Env != "production" {
		t.Fatalf("server: %+v", cfg)
	}
	if cfg.AdminGroup != "env-admins" {
		t.Fatalf("env should override file: %q", cfg.AdminGroup)
	}
	if cfg.PasswordLoginEnabled {
		t.Fatal("file password_login.enabled=false not applied")
	}
	if cfg.AuditRetention != 48*time.Hour {
		t.Fatalf("retention: %v", cfg.AuditRetention)
	}
	if len(cfg.Tunnel.RedisAddrs) != 2 || cfg.Tunnel.RedisDB != 5 {
		t.Fatalf("redis: %+v", cfg.Tunnel)
	}
	if !cfg.Tunnel.PeerEnabled || cfg.Tunnel.Peer.Token != "peer-token" {
		t.Fatalf("peer: %+v", cfg.Tunnel)
	}
	if cfg.ClusterLog.File != "/tmp/cluster-audit.jsonl" || cfg.ClusterLog.MaxMB != 50 || cfg.ClusterLog.Backups != 3 {
		t.Fatalf("cluster log: %+v", cfg.ClusterLog)
	}
	if cfg.ProxyRateLimit != 100 || cfg.LoginRateLimit != 5 {
		t.Fatalf("rate limits: %d/%d", cfg.ProxyRateLimit, cfg.LoginRateLimit)
	}
	if cfg.LDAP != nil {
		t.Fatal("ldap should be nil when not configured")
	}
}

// TestLoadConfigDefaults 无配置文件时保持历史环境变量语义与默认值。
func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("AM_PASSWORD_LOGIN", "0")
	t.Setenv("PEER_ADDRESSES", "")
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "none.yaml"))
	if err == nil {
		t.Fatal("explicit missing config must fail")
	}
	cfg, err = LoadConfig("")
	if err != nil {
		t.Fatalf("default load: %v", err)
	}
	if cfg.ListenAddr != ":8080" || cfg.JWTIssuer != "https://kamc.local" {
		t.Fatalf("defaults: %+v", cfg)
	}
	if cfg.PasswordLoginEnabled {
		t.Fatal("AM_PASSWORD_LOGIN=0 should disable")
	}
	if cfg.LoginRateLimit != 30 || cfg.AuditRetention != 720*time.Hour {
		t.Fatalf("default limits: %+v", cfg)
	}
}
