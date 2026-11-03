// Package ldap LDAP 登录客户端：bind 校验 + 用户属性搜索，
// 登录成功后交由 service.SyncUserOnLogin 同步平台用户。
//
// 环境变量：
//   - LDAP_URL           ldaps://host:636 或 ldap://host:389（必填启用）
//   - LDAP_BIND_DN       搜索用服务账号 DN（可空 = 匿名搜索）
//   - LDAP_BIND_PASSWORD 服务账号密码
//   - LDAP_BASE_DN       用户搜索基准 DN（必填）
//   - LDAP_USER_FILTER   用户过滤器，默认 (uid=%s)；%s 为登录名
//   - LDAP_USERNAME_ATTR 登录名属性，默认 uid
//   - LDAP_EMAIL_ATTR    邮箱属性，默认 mail
//   - LDAP_NICKNAME_ATTR 显示名属性，默认 cn
//   - LDAP_GROUP_FILTER  组过滤器，默认 (member=%s)（%s 为用户 DN）
//   - LDAP_GROUP_ATTR    组名属性，默认 cn
//   - LDAP_INSECURE_SKIP_VERIFY  1 = 跳过 TLS 证书校验（仅 ldaps/starttls 开发用）
package ldap

import (
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	goldap "github.com/go-ldap/ldap/v3"
)

// Config LDAP 连接与搜索配置
type Config struct {
	URL          string
	BindDN       string
	BindPassword string
	BaseDN       string
	UserFilter   string
	UsernameAttr string
	EmailAttr    string
	NicknameAttr string
	GroupFilter  string
	GroupAttr    string
	InsecureTLS  bool
}

// ErrNotConfigured 未配置 LDAP（装配层据此降级 /auth/ldap-login → 503）
var ErrNotConfigured = errors.New("ldap not configured")

// User LDAP 搜索到的登录用户（组已解析）
type User struct {
	Username string
	Email    string
	Nickname string
	DN       string
	Groups   []string
}

// FromEnv 从环境变量装配配置（LDAP_URL 为空 = 未配置，返回 nil）
func FromEnv() *Config {
	url := strings.TrimSpace(os.Getenv("LDAP_URL"))
	if url == "" {
		return nil
	}
	base := strings.TrimSpace(os.Getenv("LDAP_BASE_DN"))
	if base == "" {
		return nil
	}
	return &Config{
		URL:          url,
		BindDN:       os.Getenv("LDAP_BIND_DN"),
		BindPassword: os.Getenv("LDAP_BIND_PASSWORD"),
		BaseDN:       base,
		UserFilter:   envDefault("LDAP_USER_FILTER", "(uid=%s)"),
		UsernameAttr: envDefault("LDAP_USERNAME_ATTR", "uid"),
		EmailAttr:    envDefault("LDAP_EMAIL_ATTR", "mail"),
		NicknameAttr: envDefault("LDAP_NICKNAME_ATTR", "cn"),
		GroupFilter:  envDefault("LDAP_GROUP_FILTER", "(member=%s)"),
		GroupAttr:    envDefault("LDAP_GROUP_ATTR", "cn"),
		InsecureTLS:  os.Getenv("LDAP_INSECURE_SKIP_VERIFY") == "1",
	}
}

func envDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// dial 建立连接（ldaps=TLS / starttls 可选 / 明文仅开发）
func (c *Config) dial() (*goldap.Conn, error) {
	conn, err := goldap.DialURL(c.URL, goldap.DialWithTLSConfig(&tls.Config{InsecureSkipVerify: c.InsecureTLS}))
	if err != nil {
		return nil, fmt.Errorf("ldap -> dial %s: %w", c.URL, err)
	}
	conn.SetTimeout(10 * time.Second)
	if c.BindDN != "" {
		if err := conn.Bind(c.BindDN, c.BindPassword); err != nil {
			conn.Close()
			return nil, fmt.Errorf("ldap -> service bind: %w", err)
		}
	} else if err := conn.UnauthenticatedBind(""); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ldap -> anonymous bind: %w", err)
	}
	return conn, nil
}

// Authenticate 校验用户名密码并返回用户属性（失败一律 ErrAuth 防枚举）。
var ErrAuth = errors.New("ldap auth failed")

func (c *Config) Authenticate(username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrAuth
	}
	conn, err := c.dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// 过滤器转义，防 LDAP 注入
	filter := fmt.Sprintf(c.UserFilter, goldap.EscapeFilter(username))
	req := goldap.NewSearchRequest(c.BaseDN, goldap.ScopeWholeSubtree, goldap.NeverDerefAliases, 2, 5, false,
		filter, []string{c.UsernameAttr, c.EmailAttr, c.NicknameAttr}, nil)
	res, err := conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("ldap -> search user: %w", err)
	}
	if len(res.Entries) != 1 {
		return nil, ErrAuth // 0 个或多个命中都不放行
	}
	entry := res.Entries[0]

	// 用户密码 bind 校验（服务账号 bind 已还原为匿名/服务身份，直接二次 Bind 校验）
	if err := conn.Bind(entry.DN, password); err != nil {
		return nil, ErrAuth
	}
	// 重新以服务身份搜索组（bind 已切到用户身份，需要重新建连）
	if err := conn.Bind(c.BindDN, c.BindPassword); err != nil && c.BindDN != "" {
		return nil, fmt.Errorf("ldap -> rebind service: %w", err)
	}

	user := &User{
		Username: entry.GetAttributeValue(c.UsernameAttr),
		Email:    entry.GetAttributeValue(c.EmailAttr),
		Nickname: entry.GetAttributeValue(c.NicknameAttr),
		DN:       entry.DN,
	}
	if user.Username == "" {
		user.Username = username
	}

	// 组解析（失败不阻断登录，仅无组）
	greq := goldap.NewSearchRequest(c.BaseDN, goldap.ScopeWholeSubtree, goldap.NeverDerefAliases, 0, 30, false,
		fmt.Sprintf(c.GroupFilter, goldap.EscapeFilter(entry.DN)), []string{c.GroupAttr}, nil)
	if gres, err := conn.Search(greq); err == nil {
		for _, g := range gres.Entries {
			name := g.GetAttributeValue(c.GroupAttr)
			if name != "" {
				user.Groups = append(user.Groups, name)
			}
		}
	}
	return user, nil
}
