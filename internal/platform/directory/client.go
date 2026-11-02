// Package directory 对接 Authentik 的 core users/groups API，为策略编辑器提供
// 组织（Authentik group）与用户（Authentik user）下拉选项。
//
// 本包只读，不缓存敏感信息；缓存仅用于降低下拉搜索的 API 压力。
package directory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Option 下拉选项：Value 用于策略主体，Label 用于展示。
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type cacheEntry struct {
	options []Option
	expires time.Time
}

// Client Authentik 目录客户端。
type Client struct {
	baseURL      string
	token        string
	userSubField string
	httpClient   *http.Client
	cacheTTL     time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewFromEnv 从环境变量构造客户端；未配置 API URL/Token 时返回 nil（功能降级）。
func NewFromEnv() *Client {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("AUTHENTIK_API_URL")), "/")
	if base == "" {
		if issuer := strings.TrimSpace(os.Getenv("OIDC_ISSUER")); issuer != "" {
			if u, err := url.Parse(issuer); err == nil && u.Scheme != "" && u.Host != "" {
				base = u.Scheme + "://" + u.Host
			}
		}
	}
	token := strings.TrimSpace(os.Getenv("AUTHENTIK_API_TOKEN"))
	if base == "" || token == "" {
		return nil
	}
	subField := strings.ToLower(strings.TrimSpace(os.Getenv("AUTHENTIK_USER_SUB_FIELD")))
	switch subField {
	case "uid", "username", "pk":
	default:
		subField = "uid"
	}
	ttl := parseDuration(os.Getenv("AUTHENTIK_CACHE_TTL"), 60*time.Second)
	timeout := parseDuration(os.Getenv("AUTHENTIK_HTTP_TIMEOUT"), 10*time.Second)
	return &Client{
		baseURL:      base,
		token:        token,
		userSubField: subField,
		httpClient:   &http.Client{Timeout: timeout},
		cacheTTL:     ttl,
		cache:        make(map[string]cacheEntry),
	}
}

func parseDuration(raw string, def time.Duration) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

// ListOrgs 列出/搜索 Authentik groups（组织）。
func (c *Client) ListOrgs(ctx context.Context, query string) ([]Option, error) {
	return c.list(ctx, "org", "/api/v3/core/groups/", query, func(results []map[string]any) []Option {
		options := make([]Option, 0, len(results))
		for _, item := range results {
			name := stringField(item, "name")
			if name == "" {
				continue
			}
			options = append(options, Option{Value: name, Label: name})
		}
		return options
	})
}

// ListUsers 列出/搜索 Authentik users（用户）。
func (c *Client) ListUsers(ctx context.Context, query string) ([]Option, error) {
	return c.list(ctx, "user", "/api/v3/core/users/", query, func(results []map[string]any) []Option {
		options := make([]Option, 0, len(results))
		for _, item := range results {
			username := stringField(item, "username")
			email := stringField(item, "email")
			name := stringField(item, "name")
			value := stringField(item, c.userSubField)
			if value == "" {
				value = username
			}
			if value == "" {
				value = stringField(item, "pk")
			}
			if value == "" {
				continue
			}
			label := username
			if label == "" {
				label = name
			}
			if label == "" {
				label = value
			}
			if email != "" {
				label += " (" + email + ")"
			}
			options = append(options, Option{Value: value, Label: label})
		}
		return options
	})
}

func (c *Client) list(
	ctx context.Context,
	kind, path, query string,
	convert func([]map[string]any) []Option,
) ([]Option, error) {
	cacheKey := kind + ":" + query
	c.mu.Lock()
	if entry, ok := c.cache[cacheKey]; ok && time.Now().Before(entry.expires) {
		c.mu.Unlock()
		return entry.options, nil
	}
	c.mu.Unlock()

	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("directory -> parse url: %w", err)
	}
	q := u.Query()
	q.Set("page_size", "100")
	if strings.TrimSpace(query) != "" {
		q.Set("search", strings.TrimSpace(query))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("directory -> request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("directory -> call authentik: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("directory -> authentik status %d", resp.StatusCode)
	}

	var body struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("directory -> decode response: %w", err)
	}
	options := convert(body.Results)

	c.mu.Lock()
	c.cache[cacheKey] = cacheEntry{options: options, expires: time.Now().Add(c.cacheTTL)}
	c.mu.Unlock()
	return options, nil
}

func stringField(item map[string]any, key string) string {
	if v, ok := item[key].(string); ok {
		return v
	}
	return ""
}
