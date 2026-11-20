package platform_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestAvatarUploadE2E 头像全链路：上传 → 返回签名 URL → 匿名读取 → 错误 token 拒绝。
func TestAvatarUploadE2E(t *testing.T) {
	t.Setenv("AM_AVATAR_DIR", t.TempDir())
	rt := newTestRuntime(t)
	token := adminToken(t, rt)

	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profile/avatar", bytes.NewReader(png))
	req.Header.Set("Content-Type", "image/png")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: http=%d body=%s", rec.Code, rec.Body.String())
	}
	var uploaded struct {
		Status int `json:"status"`
		Data   struct {
			AvatarURL string `json:"avatar_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &uploaded); err != nil || uploaded.Status != 0 {
		t.Fatalf("upload response: %v %s", err, rec.Body.String())
	}
	if uploaded.Data.AvatarURL == "" {
		t.Fatal("empty avatar_url")
	}

	// 匿名读取：签名 token 即凭证
	getReq := httptest.NewRequest(http.MethodGet, uploaded.Data.AvatarURL, nil)
	getRec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get avatar: http=%d", getRec.Code)
	}
	got, _ := io.ReadAll(getRec.Body)
	if !bytes.Equal(got, png) {
		t.Fatalf("avatar bytes mismatch: %d", len(got))
	}
	if ct := getRec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content type: %q", ct)
	}

	// 篡改 token：401
	u, err := url.Parse(uploaded.Data.AvatarURL)
	if err != nil {
		t.Fatalf("parse avatar url: %v", err)
	}
	q := u.Query()
	q.Set("token", "deadbeef")
	u.RawQuery = q.Encode()
	badReq := httptest.NewRequest(http.MethodGet, u.String(), nil)
	badRec := httptest.NewRecorder()
	rt.Engine().ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: want 401, got %d", badRec.Code)
	}
}
