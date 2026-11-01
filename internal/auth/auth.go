// Package auth 平台 JWT 认证：RS256 密钥环（多 kid 并存、零停机轮换，§4.2/§4.3）、
// Token 签发与校验、JWKS 公钥集导出。
//
// 本包是独立组件（叶组件）：由 platform 装配使用，不 import 项目内其他包。
//
// 密钥环形态：目录下每个 *.pem 文件一把私钥（文件名去扩展名即 kid），
// 对应 Secret 挂载（JWT_KEYRING_DIR）；轮换流程见设计文档 §4.3——
// 新 kid 加入即签发切换，旧 kid 保留至存量 Token 过期后移除。
package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 错误 sentinel：调用方 errors.Is 判定（模式 A）
var (
	// ErrInvalid token 无效（签名/格式/签名者未知）
	ErrInvalid = errors.New("invalid token")
	// ErrExpired token 已过期
	ErrExpired = errors.New("token expired")
)

// Claims 平台 JWT 载荷（§4.2）
type Claims struct {
	jwt.RegisteredClaims
	Email  string   `json:"email,omitempty"`
	Groups []string `json:"groups,omitempty"`
	// Epoch 签发时的用户吊销纪元（RevokeUser 递增；token.epoch < 用户当前
	// epoch 即吊销。无此声明的旧 Token 按 0 处理）
	Epoch int64 `json:"am_ep,omitempty"`
}

// Option 服务选项
type Option func(*options)

type options struct {
	issuer                 string
	audience               string
	ttl                    time.Duration
	keys                   []keyEntry
	now                    func() time.Time
	revocations            RevocationStore
	revocationSyncInterval time.Duration
}

type keyEntry struct {
	kid  string
	key  *rsa.PrivateKey
	sign bool // 是否为当前签名密钥（仅一把为 true）
}

// WithIssuer 签发者（iss 校验值）
func WithIssuer(iss string) Option { return func(o *options) { o.issuer = iss } }

// WithAudience 受众（aud 校验值）
func WithAudience(aud string) Option { return func(o *options) { o.audience = aud } }

// WithTTL Token 有效期（默认 1h，§4.2）
func WithTTL(ttl time.Duration) Option { return func(o *options) { o.ttl = ttl } }

// WithKey 追加一把签名密钥（多把时最后加入者为当前签名密钥）
func WithKey(kid string, pemBytes []byte) Option {
	return func(o *options) { o.keys = append(o.keys, keyEntry{kid: kid, key: mustParsePrivateKey(pemBytes)}) }
}

// WithKeyDir 从目录装载密钥环（*.pem，文件名即 kid；文件名排序最后者为签名密钥，
// 对应轮换流程"新密钥命名排序靠后"）
func WithKeyDir(dir string) Option {
	return func(o *options) {
		entries, err := loadKeyDir(dir)
		if err != nil {
			panic(fmt.Sprintf("auth: load key dir %s: %v", dir, err)) // 装配期错误直接失败
		}
		o.keys = append(o.keys, entries...)
	}
}

// WithNow 覆盖时钟（测试注入）
func WithNow(now func() time.Time) Option { return func(o *options) { o.now = now } }

// Service JWT 签发与校验
type Service struct {
	mu       sync.RWMutex
	keys     map[string]*rsa.PrivateKey
	order    []string // kid 装载顺序
	signKid  string
	issuer   string
	audience string
	ttl      time.Duration
	now      func() time.Time

	// 吊销状态（本地缓存；Verify 只查内存，§7.1 鉴权零外部依赖）
	store           RevocationStore
	revSyncInterval time.Duration
	revMu           sync.RWMutex
	userEpochs      map[string]int64
	revokedTokens   map[string]struct{}
	lastRevSync     time.Time
}

// NewService 构造认证服务：至少一把密钥，末位为当前签名密钥。
func NewService(opts ...Option) (*Service, error) {
	o := &options{ttl: time.Hour, now: time.Now}
	for _, opt := range opts {
		opt(o)
	}
	if len(o.keys) == 0 {
		return nil, errors.New("auth -> at least one signing key is required")
	}

	s := &Service{
		keys:            make(map[string]*rsa.PrivateKey, len(o.keys)),
		issuer:          o.issuer,
		audience:        o.audience,
		ttl:             o.ttl,
		now:             o.now,
		store:           o.revocations,
		revSyncInterval: o.revocationSyncInterval,
	}
	if s.revSyncInterval <= 0 {
		s.revSyncInterval = 5 * time.Second
	}
	s.userEpochs = make(map[string]int64)
	s.revokedTokens = make(map[string]struct{})
	for _, e := range o.keys {
		if _, dup := s.keys[e.kid]; dup {
			return nil, fmt.Errorf("auth -> duplicate kid %q", e.kid)
		}
		s.keys[e.kid] = e.key
		s.order = append(s.order, e.kid)
	}
	s.signKid = s.order[len(s.order)-1]
	return s, nil
}

// GenerateKeyPEM 生成 RSA 私钥 PEM（轮换流程第一步 / 开发环境临时密钥）
func GenerateKeyPEM() ([]byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("auth -> generate rsa key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("auth -> marshal pkcs8: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// Sign 签发平台 JWT（kid 头指向当前签名密钥），返回 token 与过期时间。
func (s *Service) Sign(subject, email string, groups []string) (string, time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.keys[s.signKid]
	if !ok {
		return "", time.Time{}, fmt.Errorf("auth -> sign kid %q missing", s.signKid)
	}
	s.revMu.RLock()
	userEpoch := s.userEpochs[subject]
	s.revMu.RUnlock()
	now := s.now()
	exp := now.Add(s.ttl)
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{s.audience},
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        randomID(),
		},
		Email:  email,
		Groups: groups,
		Epoch:  userEpoch,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.signKid
	signed, err := token.SignedString(key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth -> sign: %w", err)
	}
	return signed, exp, nil
}

// Verify 校验平台 JWT：kid 定位密钥 → RS256 验签 → iss/aud/exp 校验 →
// 本地吊销状态检查（jti 黑名单 + 用户级踢出标记）。
// 过期返回 ErrExpired、吊销返回 ErrRevoked，其余无效情形一律 ErrInvalid（防细节泄露）。
func (s *Service) Verify(ctx context.Context, tokenStr string) (*Claims, error) {
	s.maybeSyncRevocations()

	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		key, ok := s.keys[kid]
		if !ok {
			return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalid, kid)
		}
		return &key.PublicKey, nil
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(s.issuer),
		jwt.WithAudience(s.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := s.checkRevoked(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// JWKS 导出全部公钥的 JWKS 集（GET /.well-known/jwks.json，§4.3）
func (s *Service) JWKS() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 输出顺序稳定（按装载顺序），便于 diff 与缓存
	type jwk struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	set := struct {
		Keys []jwk `json:"keys"`
	}{Keys: make([]jwk, 0, len(s.order))}
	kids := append([]string(nil), s.order...)
	sort.Strings(kids)
	for _, kid := range kids {
		pub := &s.keys[kid].PublicKey
		set.Keys = append(set.Keys, jwk{
			Kty: "RSA", Kid: kid, Use: "sig", Alg: "RS256",
			N: base64URL(pub.N.Bytes()),
			E: base64URL(bigEndianBytes(pub.E)),
		})
	}
	b, err := jsonMarshal(set)
	if err != nil {
		return nil, fmt.Errorf("auth -> marshal jwks: %w", err)
	}
	return b, nil
}

// loadKeyDir 读取密钥目录（*.pem；文件名去扩展名即 kid）
func loadKeyDir(dir string) ([]keyEntry, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.pem"))
	if err != nil {
		return nil, fmt.Errorf("auth -> glob key dir: %w", err)
	}
	sort.Strings(matches)
	entries := make([]keyEntry, 0, len(matches))
	for _, path := range matches {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("auth -> read key %s: %w", path, err)
		}
		kid := strings.TrimSuffix(filepath.Base(path), ".pem")
		entries = append(entries, keyEntry{kid: kid, key: mustParsePrivateKey(b)})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("auth -> no *.pem key in %s", dir)
	}
	return entries, nil
}

// mustParsePrivateKey 解析 PKCS8/PKCS1 RSA 私钥（装配期失败即 panic，
// 密钥环损坏属部署错误，不应带病运行）
func mustParsePrivateKey(b []byte) *rsa.PrivateKey {
	block, _ := pem.Decode(b)
	if block == nil {
		panic("auth: key pem decode failed")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			panic("auth: key is not RSA")
		}
		return rsaKey
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(fmt.Sprintf("auth: parse private key: %v", err))
	}
	return key
}
