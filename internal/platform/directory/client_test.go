package directory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientListsOrgsAndUsers(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/core/groups/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"pk": "g1", "name": "dev-team"}},
			})
		case "/api/v3/core/users/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"pk": "u1", "uid": "uid-alice", "username": "alice",
					"email": "alice@example.com", "name": "Alice",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{
		baseURL:      srv.URL,
		token:        "test-token",
		userSubField: "uid",
		httpClient:   srv.Client(),
		cacheTTL:     time.Minute,
		cache:        make(map[string]cacheEntry),
	}

	orgs, err := c.ListOrgs(context.Background(), "dev")
	if err != nil {
		t.Fatalf("list orgs: %v", err)
	}
	if len(orgs) != 1 || orgs[0].Value != "dev-team" || orgs[0].Label != "dev-team" {
		t.Fatalf("orgs: %+v", orgs)
	}
	users, err := c.ListUsers(context.Background(), "alice")
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 || users[0].Value != "uid-alice" || users[0].Label != "alice (alice@example.com)" {
		t.Fatalf("users: %+v", users)
	}

	// 同一查询命中缓存，不再打 Authentik。
	if _, err := c.ListUsers(context.Background(), "alice"); err != nil {
		t.Fatalf("cached users: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 upstream calls, got %d", calls)
	}
}

func TestNewFromEnvDisabled(t *testing.T) {
	t.Setenv("AUTHENTIK_API_URL", "")
	t.Setenv("OIDC_ISSUER", "")
	t.Setenv("AUTHENTIK_API_TOKEN", "")
	if c := NewFromEnv(); c != nil {
		t.Fatal("client should be nil when API URL/token are absent")
	}
}
