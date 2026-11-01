package platform_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/TuF3i/kamc/internal/platform/errcode"
)

// fakeIDP 最小 OIDC Provider：discovery + jwks + token 三端点，
// 签发 RS256 ID Token（sub/email/groups 可由用例注入）。
type fakeIDP struct {
	srv *httptest.Server
	key *rsa.PrivateKey

	User struct {
		Sub    string
		Email  string
		Groups []string
	}
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("idp key: %v", err)
	}
	idp := &fakeIDP{key: key}
	idp.User.Sub = "alice-from-idp"
	idp.User.Email = "alice@example.com"
	idp.User.Groups = []string{"dev-team", "am:admin"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.srv.URL,
			"authorization_endpoint":                idp.srv.URL + "/authorize",
			"token_endpoint":                        idp.srv.URL + "/token",
			"jwks_uri":                              idp.srv.URL + "/jwks.json",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		pub := &key.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA", "kid": "idp-key", "use": "sig", "alg": "RS256",
				"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			}},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json") // oauth2 按此选择 JSON 解析分支
		idToken, err := idp.MintIDToken()
		if err != nil {
			http.Error(w, "sign: "+err.Error(), 500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-xxx", "id_token": idToken, "token_type": "Bearer",
		})
	})

	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

// MintIDToken 铸一枚 ID Token（token 端点与 token-exchange 测试共用）
func (idp *fakeIDP) MintIDToken() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":    idp.srv.URL,
		"sub":    idp.User.Sub,
		"aud":    "kamc-test",
		"exp":    now.Add(time.Hour).Unix(),
		"iat":    now.Unix(),
		"email":  idp.User.Email,
		"groups": idp.User.Groups,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "idp-key"
	return tok.SignedString(idp.key)
}

// TestOIDCLoginFlowE2E 完整授权码流程：/auth/login 302（带 state）→
// /auth/callback 兑换 → 平台 JWT → /auth/me 回显 claims；state 错误 401。
func TestOIDCLoginFlowE2E(t *testing.T) {
	idp := newFakeIDP(t)
	t.Setenv("OIDC_ISSUER", idp.srv.URL)
	t.Setenv("OIDC_CLIENT_ID", "kamc-test")
	t.Setenv("OIDC_CLIENT_SECRET", "s3cret")
	t.Setenv("OIDC_REDIRECT_URL", "http://localhost:8080/auth/callback")
	rt := newTestRuntime(t)

	// 1. 发起登录 → 302 + state
	resp := callRaw(t, rt, http.MethodGet, "/auth/login")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login: want 302, got %d", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	if !strings.HasPrefix(loc.Path, "/authorize") {
		t.Fatalf("authorize endpoint mismatch: %s", loc)
	}
	state := loc.Query().Get("state")
	if state == "" {
		t.Fatal("state should be present in authorize URL")
	}

	// 2. 错误 state → 401
	if bad := callRaw(t, rt, http.MethodGet, "/auth/callback?code=good-code&state=wrong"); bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad state: want 401, got %d", bad.StatusCode)
	}

	// 3. 正确兑换 → 平台 JWT
	resp2, ar := call(t, rt, http.MethodGet, "/auth/callback?code=good-code&state="+state, nil)
	if resp2.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("callback: http=%d body=%+v", resp2.StatusCode, ar)
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(ar.Data, &issued); err != nil || issued.Token == "" {
		t.Fatalf("decode issued: %v data=%s", err, ar.Data)
	}

	// 4. 平台 JWT 可用：/auth/me 回显 claims（sub/email/groups 来自 ID Token）
	_, ar = call(t, rt, http.MethodGet, "/auth/me", nil, issued.Token)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("me: %+v", ar)
	}
	var claims map[string]any
	if err := json.Unmarshal(ar.Data, &claims); err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	if claims["sub"] != idp.User.Sub {
		t.Fatalf("sub: got %v want %s", claims["sub"], idp.User.Sub)
	}
	if claims["email"] != idp.User.Email {
		t.Fatalf("email: got %v", claims["email"])
	}
	if groups, _ := claims["groups"].([]any); len(groups) != 2 {
		t.Fatalf("groups: %v", claims["groups"])
	}

	// 5. 该身份可访问管理面（含 am:admin 组）
	if meResp := callRaw(t, rt, http.MethodGet, "/api/v1/clusters", issued.Token); meResp.StatusCode != http.StatusOK {
		t.Fatalf("admin access with idp-admin identity: %d", meResp.StatusCode)
	}
}

// TestTokenExchangeE2E exec plugin 的兑换端点：ID Token 验签 → 平台 JWT →
// /auth/me 验身份；篡改/异端签发一律 401。
func TestTokenExchangeE2E(t *testing.T) {
	idp := newFakeIDP(t)
	t.Setenv("OIDC_ISSUER", idp.srv.URL)
	t.Setenv("OIDC_CLIENT_ID", "kamc-test")
	t.Setenv("OIDC_CLIENT_SECRET", "s3cret")
	t.Setenv("OIDC_REDIRECT_URL", "http://localhost:8080/auth/callback")
	rt := newTestRuntime(t)

	idToken, err := idp.MintIDToken()
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	resp, ar := call(t, rt, http.MethodPost, "/auth/token-exchange",
		map[string]string{"id_token": idToken})
	if resp.StatusCode != http.StatusOK || ar.Status != errcode.CodeSuccess {
		t.Fatalf("exchange: http=%d body=%+v", resp.StatusCode, ar)
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(ar.Data, &issued); err != nil || issued.Token == "" {
		t.Fatalf("decode issued: %v data=%s", err, ar.Data)
	}

	// 兑换所得 JWT 身份正确
	_, ar = call(t, rt, http.MethodGet, "/auth/me", nil, issued.Token)
	if ar.Status != errcode.CodeSuccess {
		t.Fatalf("me: %+v", ar)
	}
	if raw := string(ar.Data); !strings.Contains(raw, idp.User.Sub) {
		t.Fatalf("me claims: %s", raw)
	}

	// 篡改 → 401
	if bad := callRaw(t, rt, http.MethodPost, "/auth/token-exchange",
		"bad-token"); bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tampered: want 401, got %d", bad.StatusCode)
	}
}
