// 本地 Mock OIDC Provider：仅用于验证 k8s-am-login 的 Device Flow 与
// 平台 /auth/token-exchange。实现 discovery、device、token、jwks 四个端点。
package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"
)

const issuer = "https://127.0.0.1:9100"

var key *rsa.PrivateKey

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func signJWT(claims map[string]any) string {
	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": "mock-1"}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	input := b64(hb) + "." + b64(cb)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		log.Fatal(err)
	}
	return input + "." + b64(sig)
}

func idToken() string {
	now := time.Now()
	return signJWT(map[string]any{
		"iss": issuer, "sub": "alice", "aud": "kamc",
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"email": "alice@example.com", "email_verified": true,
		"preferred_username": "alice", "groups": []string{"dev-team"},
	})
}

func main() {
	var err error
	key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}
	// 落一份证书/私钥方便排查（非必需）
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	_ = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/authorize",
			"token_endpoint":                        issuer + "/token",
			"device_authorization_endpoint":         issuer + "/device",
			"jwks_uri":                              issuer + "/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"grant_types_supported":                 []string{"authorization_code", "urn:ietf:params:oauth:grant-type:device_code"},
			"scopes_supported":                      []string{"openid", "profile", "email", "groups"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		e := big.NewInt(int64(key.E)).Bytes()
		writeJSON(w, map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": "mock-1", "use": "sig", "alg": "RS256",
			"n": b64(key.N.Bytes()), "e": b64(e),
		}}})
	})
	mux.HandleFunc("/device", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		log.Printf("[oidc] device request client_id=%s", r.Form.Get("client_id"))
		writeJSON(w, map[string]any{
			"device_code": "mock-device-code", "user_code": "KAMC-TEST",
			"verification_uri": issuer + "/activate", "expires_in": 600, "interval": 1,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		log.Printf("[oidc] token grant=%s", r.Form.Get("grant_type"))
		writeJSON(w, map[string]any{
			"access_token": "mock-access-token", "token_type": "Bearer",
			"expires_in": 3600, "id_token": idToken(),
		})
	})
	mux.HandleFunc("/activate", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "mock device activation: KAMC-TEST")
	})

	log.Printf("[oidc] mock provider on https://127.0.0.1:9100 (issuer=%s)", issuer)
	srv := &http.Server{Addr: "127.0.0.1:9100", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServeTLS("/tmp/kamc-tls/cert.pem", "/tmp/kamc-tls/key.pem"))
	_ = os.Stdout
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
