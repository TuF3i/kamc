package platform

import (
	"time"

	"github.com/TuF3i/kamc/internal/config"
	"github.com/TuF3i/kamc/internal/platform/audit"
	ldapclient "github.com/TuF3i/kamc/internal/platform/ldapclient"
	"github.com/TuF3i/kamc/internal/tunnel"
)

// Config 平台运行配置（Viper 装载：默认值 → 配置文件 → 环境变量）。
type Config struct {
	ListenAddr  string
	Env         string // AM_ENV（production 触发安全护栏）
	ExternalURL string

	DatabaseURL string

	JWTIssuer     string
	JWTAudience   string
	JWTKeyringDir string

	AdminGroup            string
	PasswordLoginEnabled  bool
	PasswordLoginUser     string
	PasswordLoginPassword string
	DevToken              bool
	OIDCIssuer            string
	OIDCClientID          string
	OIDCClientSecret      string
	OIDCRedirectURL       string
	LDAP                  *ldapclient.Config
	AuditSpillFile        string
	AuditRetention        time.Duration
	ClusterLog            audit.ClusterLogConfig
	GeoIPDB               string
	AvatarBackend         string // auto | local | minio（auto：配置了 MinIO endpoint 即用对象存储）
	AvatarDir             string
	AvatarSecret          string
	AvatarMinIO           AvatarMinIOConfig
	ProxyRateLimit        int
	LoginRateLimit        int
	RobotTokenTTL         time.Duration
	KubeconfigCAFile      string
	AgentImageRepo        string
	WebDir                string
	Tunnel                tunnel.Config
}

// AvatarMinIOConfig 头像 MinIO / S3 兼容对象存储配置。
type AvatarMinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	Prefix    string
	UseSSL    bool
}

// LoadConfig 装载平台配置。path 为空时读 AM_CONFIG_FILE，再按 kamc 搜索默认路径。
func LoadConfig(paths ...string) (Config, error) {
	path := ""
	if len(paths) > 0 {
		path = paths[0]
	}
	l := config.New()

	// ---- 默认值 ----
	l.SetDefault("server.listen_addr", ":8080")
	l.SetDefault("server.env", "development")
	l.SetDefault("jwt.issuer", "https://kamc.local")
	l.SetDefault("jwt.audience", "kamc")
	l.SetDefault("auth.admin_group", "am:admin")
	l.SetDefault("auth.password_login.enabled", true)
	l.SetDefault("auth.password_login.user", "sysadmin")
	l.SetDefault("auth.password_login.password", "sysadmin123")
	l.SetDefault("auth.dev_token", false)
	l.SetDefault("ldap.user_filter", "(uid=%s)")
	l.SetDefault("ldap.username_attr", "uid")
	l.SetDefault("ldap.email_attr", "mail")
	l.SetDefault("ldap.nickname_attr", "cn")
	l.SetDefault("ldap.group_filter", "(member=%s)")
	l.SetDefault("ldap.group_attr", "cn")
	l.SetDefault("tunnel.peer.addr", ":9090")
	l.SetDefault("tunnel.ttl", 15*time.Second)
	l.SetDefault("tunnel.interval", 5*time.Second)
	l.SetDefault("audit.spill_file", "/tmp/kamc-audit-spill.jsonl")
	l.SetDefault("audit.retention", 720*time.Hour)
	l.SetDefault("audit.cluster_log.file", "data/cluster-audit.jsonl")
	l.SetDefault("audit.cluster_log.max_mb", 100)
	l.SetDefault("audit.cluster_log.backups", 5)
	l.SetDefault("avatar.backend", "auto")
	l.SetDefault("avatar.dir", "data/avatar")
	l.SetDefault("avatar.minio.bucket", "kamc-avatars")
	l.SetDefault("avatar.minio.prefix", "avatars/")
	l.SetDefault("avatar.minio.use_ssl", false)
	l.SetDefault("rate_limit.proxy", 0)
	l.SetDefault("rate_limit.login", 30)
	l.SetDefault("robot.token_ttl", 8760*time.Hour)
	l.SetDefault("agent.image_repo", "tuf3i/k8s-am-agent")
	l.SetDefault("web.dir", "data/web")

	// ---- 环境变量（保持既有部署契约）----
	l.Bind("server.listen_addr", "AM_LISTEN_ADDR")
	l.Bind("server.env", "AM_ENV")
	l.Bind("server.external_url", "AM_EXTERNAL_URL")
	l.Bind("database.url", "DATABASE_URL")
	l.Bind("redis.addresses", "REDIS_ADDRESSES")
	l.Bind("redis.addr", "REDIS_ADDR")
	l.Bind("redis.password", "REDIS_PASSWORD")
	l.Bind("redis.db", "REDIS_DB")
	l.Bind("jwt.issuer", "AM_JWT_ISSUER")
	l.Bind("jwt.audience", "AM_JWT_AUD")
	l.Bind("jwt.keyring_dir", "JWT_KEYRING_DIR")
	l.Bind("auth.admin_group", "AM_ADMIN_GROUP")
	l.Bind("auth.password_login.enabled", "AM_PASSWORD_LOGIN")
	l.Bind("auth.password_login.user", "AM_PASSWORD_LOGIN_USER")
	l.Bind("auth.password_login.password", "AM_PASSWORD_LOGIN_PASSWORD")
	l.Bind("auth.dev_token", "AM_DEV_ISSUE_TOKEN")
	l.Bind("oidc.issuer", "OIDC_ISSUER")
	l.Bind("oidc.client_id", "OIDC_CLIENT_ID")
	l.Bind("oidc.client_secret", "OIDC_CLIENT_SECRET")
	l.Bind("oidc.redirect_url", "OIDC_REDIRECT_URL")
	l.Bind("ldap.url", "LDAP_URL")
	l.Bind("ldap.bind_dn", "LDAP_BIND_DN")
	l.Bind("ldap.bind_password", "LDAP_BIND_PASSWORD")
	l.Bind("ldap.base_dn", "LDAP_BASE_DN")
	l.Bind("ldap.user_filter", "LDAP_USER_FILTER")
	l.Bind("ldap.username_attr", "LDAP_USERNAME_ATTR")
	l.Bind("ldap.email_attr", "LDAP_EMAIL_ATTR")
	l.Bind("ldap.nickname_attr", "LDAP_NICKNAME_ATTR")
	l.Bind("ldap.group_filter", "LDAP_GROUP_FILTER")
	l.Bind("ldap.group_attr", "LDAP_GROUP_ATTR")
	l.Bind("ldap.insecure_skip_verify", "LDAP_INSECURE_SKIP_VERIFY")
	l.Bind("tunnel.pod_id", "POD_ID")
	l.Bind("tunnel.peer.addr", "AM_PEER_ADDR")
	l.Bind("tunnel.peer.token", "PEER_TOKEN")
	l.Bind("tunnel.peer.addresses", "PEER_ADDRESSES")
	l.Bind("tunnel.ttl", "AM_TUNNEL_TTL")
	l.Bind("tunnel.interval", "AM_TUNNEL_INTERVAL")
	l.Bind("audit.spill_file", "AM_AUDIT_SPILL_FILE")
	l.Bind("audit.retention", "AM_AUDIT_RETENTION")
	l.Bind("audit.cluster_log.file", "AM_AUDIT_LOG_FILE")
	l.Bind("audit.cluster_log.max_mb", "AM_AUDIT_LOG_MAX_MB")
	l.Bind("audit.cluster_log.backups", "AM_AUDIT_LOG_BACKUPS")
	l.Bind("geoip.db", "AM_GEOIP_DB")
	l.Bind("avatar.backend", "AM_AVATAR_BACKEND")
	l.Bind("avatar.dir", "AM_AVATAR_DIR")
	l.Bind("avatar.secret", "AM_AVATAR_SECRET")
	l.Bind("avatar.minio.endpoint", "AM_AVATAR_MINIO_ENDPOINT")
	l.Bind("avatar.minio.access_key", "AM_AVATAR_MINIO_ACCESS_KEY")
	l.Bind("avatar.minio.secret_key", "AM_AVATAR_MINIO_SECRET_KEY")
	l.Bind("avatar.minio.bucket", "AM_AVATAR_MINIO_BUCKET")
	l.Bind("avatar.minio.region", "AM_AVATAR_MINIO_REGION")
	l.Bind("avatar.minio.prefix", "AM_AVATAR_MINIO_PREFIX")
	l.Bind("avatar.minio.use_ssl", "AM_AVATAR_MINIO_USE_SSL")
	l.Bind("rate_limit.proxy", "AM_RATE_LIMIT")
	l.Bind("rate_limit.login", "AM_LOGIN_RATE_LIMIT")
	l.Bind("robot.token_ttl", "AM_ROBOT_TOKEN_TTL")
	l.Bind("kubeconfig.ca_file", "AM_KUBECONFIG_CA_FILE")
	l.Bind("agent.image_repo", "AM_AGENT_IMAGE_REPO")
	l.Bind("web.dir", "AM_WEB_DIR")

	if err := l.LoadFile(path, "kamc"); err != nil {
		return Config{}, err
	}

	redisAddrs := l.StringList("redis.addresses")
	if len(redisAddrs) == 0 {
		if addr := l.String("redis.addr"); addr != "" {
			redisAddrs = []string{addr}
		}
	}
	peerCfg, peerEnabled := tunnel.PeerOptionsFromConfig(
		l.String("tunnel.peer.addresses"),
		l.String("tunnel.peer.token"),
		l.String("tunnel.peer.addr"),
	)

	cfg := Config{
		ListenAddr:            l.String("server.listen_addr"),
		Env:                   l.String("server.env"),
		ExternalURL:           l.String("server.external_url"),
		DatabaseURL:           l.String("database.url"),
		JWTIssuer:             l.String("jwt.issuer"),
		JWTAudience:           l.String("jwt.audience"),
		JWTKeyringDir:         l.String("jwt.keyring_dir"),
		AdminGroup:            l.String("auth.admin_group"),
		PasswordLoginEnabled:  l.Bool("auth.password_login.enabled", true),
		PasswordLoginUser:     l.String("auth.password_login.user"),
		PasswordLoginPassword: l.String("auth.password_login.password"),
		DevToken:              l.Bool("auth.dev_token", false),
		OIDCIssuer:            l.String("oidc.issuer"),
		OIDCClientID:          l.String("oidc.client_id"),
		OIDCClientSecret:      l.String("oidc.client_secret"),
		OIDCRedirectURL:       l.String("oidc.redirect_url"),
		AuditSpillFile:        l.String("audit.spill_file"),
		AuditRetention:        l.Duration("audit.retention", 720*time.Hour),
		ClusterLog: audit.ClusterLogConfig{
			File:    l.String("audit.cluster_log.file"),
			MaxMB:   l.Int("audit.cluster_log.max_mb", 100),
			Backups: l.Int("audit.cluster_log.backups", 5),
		},
		GeoIPDB:       l.String("geoip.db"),
		AvatarBackend: l.String("avatar.backend"),
		AvatarDir:     l.String("avatar.dir"),
		AvatarSecret:  l.String("avatar.secret"),
		AvatarMinIO: AvatarMinIOConfig{
			Endpoint:  l.String("avatar.minio.endpoint"),
			AccessKey: l.String("avatar.minio.access_key"),
			SecretKey: l.String("avatar.minio.secret_key"),
			Bucket:    l.String("avatar.minio.bucket"),
			Region:    l.String("avatar.minio.region"),
			Prefix:    l.String("avatar.minio.prefix"),
			UseSSL:    l.Bool("avatar.minio.use_ssl", false),
		},
		ProxyRateLimit:   l.Int("rate_limit.proxy", 0),
		LoginRateLimit:   l.Int("rate_limit.login", 30),
		RobotTokenTTL:    l.Duration("robot.token_ttl", 8760*time.Hour),
		KubeconfigCAFile: l.String("kubeconfig.ca_file"),
		AgentImageRepo:   l.String("agent.image_repo"),
		WebDir:           l.String("web.dir"),
		Tunnel: tunnel.Config{
			RedisAddrs:    redisAddrs,
			RedisPassword: l.String("redis.password"),
			RedisDB:       l.Int("redis.db", 0),
			PodID:         l.String("tunnel.pod_id"),
			TTL:           l.Duration("tunnel.ttl", 15*time.Second),
			Interval:      l.Duration("tunnel.interval", 5*time.Second),
			Peer:          peerCfg,
			PeerEnabled:   peerEnabled,
		},
	}

	if l.String("ldap.url") != "" && l.String("ldap.base_dn") != "" {
		cfg.LDAP = &ldapclient.Config{
			URL:          l.String("ldap.url"),
			BindDN:       l.String("ldap.bind_dn"),
			BindPassword: l.String("ldap.bind_password"),
			BaseDN:       l.String("ldap.base_dn"),
			UserFilter:   l.String("ldap.user_filter"),
			UsernameAttr: l.String("ldap.username_attr"),
			EmailAttr:    l.String("ldap.email_attr"),
			NicknameAttr: l.String("ldap.nickname_attr"),
			GroupFilter:  l.String("ldap.group_filter"),
			GroupAttr:    l.String("ldap.group_attr"),
			InsecureTLS:  l.Bool("ldap.insecure_skip_verify", false),
		}
	}
	return cfg, nil
}
