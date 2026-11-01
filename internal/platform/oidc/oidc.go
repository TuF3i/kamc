// Package oidc 对接 Authentik（或任意标准 OIDC Provider）的登录客户端：
// 授权码流程（/auth/login 发起 → /auth/callback 兑换）+ ID Token 校验，
// 换取平台 JWT 的用户身份来源（设计文档 §4.1）。
//
// state 防 CSRF：进程内 map + 10min TTL、一次性消费。多副本部署需外置
// （Redis，路线图）；单副本语义正确。
package oidc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// stateTTL 授权 state 有效期
const stateTTL = 10 * time.Minute

// ErrNotConfigured 未配置 OIDC（OIDC_ISSUER 为空）
var ErrNotConfigured = errors.New("oidc not configured")

// ErrState state 无效/过期（一次性消费失败）
var ErrState = errors.New("invalid state")

// Config OIDC 客户端配置
type Config struct {
	Issuer       string // Provider 地址（如 https://authentik.example.com/application/o/kamc/）
	ClientID     string
	ClientSecret string
	RedirectURL  string // 平台回调地址（https://.../auth/callback）
}

// User 兑换得到的用户身份
type User struct {
	Sub    string
	Email  string
	Groups []string
}

// Client OIDC 登录客户端（provider 懒发现，首次使用时请求 discovery）
type Client struct {
	cfg Config

	mu       sync.Mutex
	provider *oidc.Provider
	oauth    *oauth2.Config
	states   map[string]time.Time
}

// New 构造客户端（配置不全返回 ErrNotConfigured）
func New(cfg Config) (*Client, error) {
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.RedirectURL == "" {
		return nil, ErrNotConfigured
	}
	return &Client{cfg: cfg, states: make(map[string]time.Time)}, nil
}

// LoginURL 发起登录：返回授权跳转地址与一次性 state（回调须原样带回）
func (c *Client) LoginURL(ctx context.Context) (string, string, error) {
	oauth, err := c.ensure(ctx)
	if err != nil {
		return "", "", err
	}

	state := randomState()
	c.mu.Lock()
	c.sweepLocked()
	c.states[state] = time.Now().Add(stateTTL)
	c.mu.Unlock()

	return oauth.AuthCodeURL(state), state, nil
}

// Exchange 回调兑换：校验一次性 state → code 换 Token → 校验 ID Token → 提取身份。
// 任何失败统一返回错误（state 与 code 错误不区分，防注入探测）。
func (c *Client) Exchange(ctx context.Context, code, state string) (*User, error) {
	oauth, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	exp, ok := c.states[state]
	if ok {
		delete(c.states, state) // 一次性
	}
	c.mu.Unlock()
	if !ok || time.Now().After(exp) {
		return nil, ErrState
	}

	tok, err := oauth.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oidc -> exchange code: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return nil, errors.New("oidc -> token response missing id_token")
	}

	user, err := c.verifyIDToken(ctx, rawID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// VerifyIDToken 校验外部 OIDC Provider 签发的 ID Token（exec plugin 的
// /auth/token-exchange 兑换用，§4.4）：签名/iss/aud/exp 全检后提取身份。
func (c *Client) VerifyIDToken(ctx context.Context, rawIDToken string) (*User, error) {
	if _, err := c.ensure(ctx); err != nil {
		return nil, err
	}
	return c.verifyIDToken(ctx, rawIDToken)
}

// verifyIDToken 校验签名/iss/aud/exp 并提取 sub/email/groups（须先 ensure）
func (c *Client) verifyIDToken(ctx context.Context, rawID string) (*User, error) {
	c.mu.Lock()
	verifier := c.provider.Verifier(&oidc.Config{ClientID: c.cfg.ClientID})
	c.mu.Unlock()

	idToken, err := verifier.Verify(ctx, rawID)
	if err != nil {
		return nil, fmt.Errorf("oidc -> verify id_token: %w", err)
	}
	var raw struct {
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}
	if err := idToken.Claims(&raw); err != nil {
		return nil, fmt.Errorf("oidc -> id_token claims: %w", err)
	}
	return &User{Sub: idToken.Subject, Email: raw.Email, Groups: raw.Groups}, nil
}

// ensure 懒发现 provider（并发安全；失败不缓存，下次重试）
func (c *Client) ensure(ctx context.Context) (*oauth2.Config, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.provider != nil {
		return c.oauth, nil
	}
	provider, err := oidc.NewProvider(ctx, c.cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc -> discover %s: %w", c.cfg.Issuer, err)
	}
	c.provider = provider
	c.oauth = &oauth2.Config{
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  c.cfg.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}
	return c.oauth, nil
}

// sweepLocked 清理过期 state（调用方持锁）
func (c *Client) sweepLocked() {
	now := time.Now()
	for s, exp := range c.states {
		if now.After(exp) {
			delete(c.states, s)
		}
	}
}

func randomState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("oidc: rand: %v", err))
	}
	return hex.EncodeToString(b)
}
