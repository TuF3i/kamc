package handler

import (
	"net/url"
	"strings"
)

// auditSensitiveQuery 审计中不可落库的 query 参数（大小写不敏感）。
// 重点覆盖 WebSocket access_token、OIDC id_token、常见密码/密钥字段。
var auditSensitiveQuery = map[string]bool{
	"access_token":  true,
	"token":         true,
	"id_token":      true,
	"refresh_token": true,
	"password":      true,
	"passwd":        true,
	"secret":        true,
	"client_secret": true,
	"api_key":       true,
	"apikey":        true,
	"authorization": true,
}

// auditSensitiveSubstrings 兜底匹配：未列入白名单但含这些子串的参数也脱敏
// （厂商自定义鉴权参数：x-amz-security-token、signature、key 等）。
var auditSensitiveSubstrings = []string{
	"token", "secret", "pass", "key", "sig", "auth", "credential",
}

func isSensitiveQueryKey(key string) bool {
	k := strings.ToLower(key)
	if auditSensitiveQuery[k] {
		return true
	}
	for _, sub := range auditSensitiveSubstrings {
		if strings.Contains(k, sub) {
			return true
		}
	}
	return false
}

// redactAuditURI 对 URI 的 query 做密钥脱敏，保留路径与其余参数。
// 用于审计 path 与请求报文起始行，避免 access_token 等凭证进入 DB/syslog。
// 解析失败时 fail-closed：丢弃整个 query（畸形编码不能成为绕过脱敏的通道）。
func redactAuditURI(raw string) string {
	if raw == "" {
		return raw
	}
	path, query, ok := strings.Cut(raw, "?")
	if !ok || query == "" {
		return raw
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return path + "?<redacted>"
	}
	changed := false
	for key := range values {
		if isSensitiveQueryKey(key) {
			values.Set(key, "***")
			changed = true
		}
	}
	if !changed {
		return raw
	}
	return path + "?" + values.Encode()
}
