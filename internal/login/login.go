// Package login k8s-am-login 的核心逻辑：OIDC Device Flow（RFC 8628）获取
// ID Token → 平台 /auth/token-exchange 兑换平台 JWT → 输出 client-go
// exec credential plugin 要求的 ExecCredential JSON（§4.4）。
//
// 凭证缓存：平台 JWT 按过期时间缓存于用户配置目录（~/.config/kamc/），
// 剩余有效期 ≥ 5min 直接复用；过期后重新走设备流程。
package login

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// credentialTTLBuffer 缓存复用的剩余有效期阈值
const credentialTTLBuffer = 5 * time.Minute

// pollDefaults 设备流程轮询参数（RFC 8628：slow_down 递增 5s）
const (
	defaultInterval  = 5 * time.Second
	slowDownStep     = 5 * time.Second
	defaultExpiresIn = 5 * time.Minute
)

// ErrTimedOut 设备授权超时/被拒绝
var ErrTimedOut = errors.New("device authorization timed out")

// Options 运行参数（命令行映射）
type Options struct {
	Server       string // 平台地址（token-exchange 调用目标）
	ClusterID    string // 集群 ID（缓存键组成；平台 JWT 本身跨集群有效）
	Issuer       string // OIDC issuer（Authentik application URL）
	ClientID     string // OIDC client id
	DeviceAuthUR string // 设备授权端点显式覆盖（默认经 discovery，再退 issuer+device/authorization）
	NoCache      bool
	Insecure     bool // 跳过 TLS 校验（开发环境自签证书）
	Now          func() time.Time
	Stderr       io.Writer // 提示信息（stdout 只输出 ExecCredential）
	PollTimeout  time.Duration
	PollInterval time.Duration // 测试注入（默认取 device 授权响应或 5s）
}

// discoveryDocument OIDC 发现文档（只取设备流相关字段）
type discoveryDocument struct {
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
}

type deviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expires_in"`
}

type tokenResponse struct {
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

type exchangeResponse struct {
	Status int `json:"status"`
	Data   struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	} `json:"data"`
}

// ExecCredential client.authentication.k8s.io/v1 输出形态
type ExecCredential struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Status     struct {
		Token               string `json:"token"`
		ExpirationTimestamp string `json:"expirationTimestamp,omitempty"`
	} `json:"status"`
}

// Run 执行完整流程，返回可写向 stdout 的 ExecCredential JSON。
func Run(ctx context.Context, o Options) ([]byte, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.PollTimeout == 0 {
		o.PollTimeout = defaultExpiresIn
	}
	if o.Server == "" || o.Issuer == "" || o.ClientID == "" {
		return nil, errors.New("login: --server/--oidc-issuer/--oidc-client-id are required")
	}

	key := cacheKey(o.Server, o.ClusterID)
	if !o.NoCache {
		if cred, ok := loadCachedCredential(key, o.Now()); ok {
			return cred, nil
		}
	}

	client := o.httpClient()

	// 1. 设备授权
	doc, err := discover(ctx, client, o)
	if err != nil {
		return nil, err
	}
	da, err := deviceAuthorize(ctx, client, doc.DeviceAuthorizationEndpoint, o.ClientID)
	if err != nil {
		return nil, err
	}
	promptDeviceFlow(o.Stderr, da)

	// 2. 轮询换取 ID Token
	idToken, err := pollForToken(ctx, client, doc.TokenEndpoint, da, o)
	if err != nil {
		return nil, err
	}

	// 3. 兑换平台 JWT
	cred, err := exchangeForCredential(ctx, client, o, idToken)
	if err != nil {
		return nil, err
	}

	if !o.NoCache {
		saveCachedCredential(key, cred, o.Now())
	}
	return renderCredential(cred), nil
}

// ---- 发现与设备授权 ----

func discover(ctx context.Context, client *http.Client, o Options) (*discoveryDocument, error) {
	docURL := strings.TrimRight(o.Issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, nil)
	if err != nil {
		return nil, fmt.Errorf("login -> new request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login -> discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login -> discovery %s: status %d", docURL, resp.StatusCode)
	}
	var doc discoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("login -> discovery decode: %w", err)
	}
	// 兜底：部分 Provider（如 Authentik）discovery 不含设备授权端点
	if doc.DeviceAuthorizationEndpoint == "" {
		doc.DeviceAuthorizationEndpoint = strings.TrimRight(o.Issuer, "/") + "/device/authorization"
	}
	return &doc, nil
}

func deviceAuthorize(ctx context.Context, client *http.Client, endpoint, clientID string) (*deviceAuthResponse, error) {
	form := url.Values{"client_id": {clientID}, "scope": {"openid profile email groups"}}
	resp, err := client.PostForm(endpoint, form)
	if err != nil {
		return nil, fmt.Errorf("login -> device authorize: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("login -> device authorize: status %d body %s", resp.StatusCode, body)
	}
	var da deviceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&da); err != nil {
		return nil, fmt.Errorf("login -> device authorize decode: %w", err)
	}
	if da.DeviceCode == "" {
		return nil, errors.New("login -> device authorize: missing device_code")
	}
	return &da, nil
}

func promptDeviceFlow(stderr io.Writer, da *deviceAuthResponse) {
	uri := da.VerificationURIComplete
	if uri == "" {
		uri = da.VerificationURI
	}
	fmt.Fprintf(stderr, "请在浏览器打开: %s\n", uri)
	if da.VerificationURIComplete == "" && da.UserCode != "" {
		fmt.Fprintf(stderr, "并输入设备码: %s\n", da.UserCode)
	}
}

// pollForToken 轮询 token 端点直至授权完成（authorization_pending 重试、
// slow_down 递增间隔、expired_token/access_denied 终止）
func pollForToken(ctx context.Context, client *http.Client, endpoint string, da *deviceAuthResponse, o Options) (string, error) {
	interval := time.Duration(da.Interval) * time.Second
	if interval <= 0 {
		interval = defaultInterval
	}
	if o.PollInterval > 0 {
		interval = o.PollInterval // 测试/高级覆盖
	}
	deadline := time.Duration(da.ExpiresIn) * time.Second
	if deadline <= 0 {
		deadline = defaultExpiresIn
	}
	pollCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	for {
		select {
		case <-pollCtx.Done():
			return "", fmt.Errorf("%w: %v", ErrTimedOut, pollCtx.Err())
		case <-time.After(interval):
		}

		form := url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {da.DeviceCode},
			"client_id":   {o.ClientID},
		}
		resp, err := client.PostForm(endpoint, form)
		if err != nil {
			return "", fmt.Errorf("login -> token poll: %w", err)
		}
		var tr tokenResponse
		err = json.NewDecoder(resp.Body).Decode(&tr)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("login -> token poll decode: %w", err)
		}

		switch {
		case resp.StatusCode == http.StatusOK && tr.IDToken != "":
			return tr.IDToken, nil
		case tr.Error == "authorization_pending":
			// 继续等待
		case tr.Error == "slow_down":
			interval += slowDownStep
		case tr.Error != "":
			return "", fmt.Errorf("login -> device flow: %s", tr.Error)
		default:
			return "", fmt.Errorf("login -> token poll: status %d without id_token", resp.StatusCode)
		}
	}
}

// exchangeForCredential 平台兑换 → ExecCredential
func exchangeForCredential(ctx context.Context, client *http.Client, o Options, idToken string) (*ExecCredential, error) {
	body, _ := json.Marshal(map[string]string{"id_token": idToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(o.Server, "/")+"/auth/token-exchange", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("login -> exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login -> exchange: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login -> exchange: status %d", resp.StatusCode)
	}
	var er exchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, fmt.Errorf("login -> exchange decode: %w", err)
	}
	if er.Status != 0 || er.Data.Token == "" {
		return nil, fmt.Errorf("login -> exchange: status %d", er.Status)
	}

	cred := &ExecCredential{}
	cred.APIVersion = "client.authentication.k8s.io/v1"
	cred.Kind = "ExecCredential"
	cred.Status.Token = er.Data.Token
	if er.Data.ExpiresAt != "" {
		if exp, err := time.Parse(time.RFC3339, er.Data.ExpiresAt); err == nil {
			cred.Status.ExpirationTimestamp = exp.UTC().Format(time.RFC3339)
		}
	}
	return cred, nil
}

// renderCredential 序列化为 stdout 专用 JSON（单行）
func renderCredential(cred *ExecCredential) []byte {
	b, _ := json.Marshal(cred)
	return b
}

// ---- 缓存 ----

type cacheEntry struct {
	Key                 string `json:"key"`
	Token               string `json:"token"`
	ExpirationTimestamp string `json:"expiration_timestamp"`
}

func cacheKey(server, clusterID string) string {
	sum := sha256.Sum256([]byte(server + "|" + clusterID))
	return hex.EncodeToString(sum[:])
}

func cacheFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kamc", "login-cache.json"), nil
}

// loadCachedCredential 命中且剩余有效期 ≥ 5min 则复用
func loadCachedCredential(key string, now time.Time) ([]byte, bool) {
	path, err := cacheFilePath()
	if err != nil {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entries []cacheEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, false
	}
	for _, e := range entries {
		if e.Key != key || e.Token == "" {
			continue
		}
		exp, err := time.Parse(time.RFC3339, e.ExpirationTimestamp)
		if err != nil || now.Add(credentialTTLBuffer).After(exp) {
			continue
		}
		cred := &ExecCredential{}
		cred.APIVersion = "client.authentication.k8s.io/v1"
		cred.Kind = "ExecCredential"
		cred.Status.Token = e.Token
		cred.Status.ExpirationTimestamp = e.ExpirationTimestamp
		return renderCredential(cred), true
	}
	return nil, false
}

// saveCachedCredential 追加/覆盖对应键（其余键保留）
func saveCachedCredential(key string, cred *ExecCredential, now time.Time) {
	path, err := cacheFilePath()
	if err != nil {
		return
	}
	var entries []cacheEntry
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &entries)
	}
	kept := entries[:0]
	for _, e := range entries {
		if e.Key != key && now.Add(-30*24*time.Hour).Before(parseTimeOrZero(e.ExpirationTimestamp)) {
			kept = append(kept, e)
		}
	}
	kept = append(kept, cacheEntry{Key: key, Token: cred.Status.Token, ExpirationTimestamp: cred.Status.ExpirationTimestamp})
	if b, err := json.Marshal(kept); err == nil {
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		_ = os.WriteFile(path, b, 0o600)
	}
}

func parseTimeOrZero(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ---- HTTP 客户端 ----

func (o Options) httpClient() *http.Client {
	if !o.Insecure {
		return http.DefaultClient
	}
	return &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // 开发环境显式开关（--insecure）
	}}
}
