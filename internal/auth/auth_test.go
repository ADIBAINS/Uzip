package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDevelopmentAuthentication(t *testing.T) {
	t.Parallel()
	verifier := NewVerifier("", "", true)
	handler := verifier.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := Principal(r.Context())
		if !ok || principal.TenantID != "tenant-1" || principal.Role != "admin" {
			t.Fatalf("unexpected principal: %+v", principal)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-User-ID", "user-1")
	request.Header.Set("X-Tenant-ID", "tenant-1")
	request.Header.Set("X-Role", "admin")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestDevelopmentAuthenticationRequiresTenant(t *testing.T) {
	t.Parallel()
	handler := NewVerifier("", "", true).Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-User-ID", "user-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestRateLimitUsesUserAndRoutePattern(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	handleLink := func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "" {
			t.Error("route parameters were not set")
		}
		w.WriteHeader(http.StatusNoContent)
	}
	mux.HandleFunc("GET /links/{id}", handleLink)
	mux.HandleFunc("DELETE /links/{id}", handleLink)
	seen := map[string]bool{}
	handler := NewVerifier("", "", true).Middleware(RateLimit(mux, func(_ context.Context, key string, _ int, _ time.Duration) (time.Duration, error) {
		if seen[key] {
			return 200 * time.Millisecond, nil
		}
		seen[key] = true
		return 0, nil
	}))
	for _, test := range []struct {
		method, path, user string
		status             int
	}{
		{"GET", "/links/one", "alice", http.StatusNoContent},
		{"GET", "/links/two", "alice", http.StatusTooManyRequests},
		{"DELETE", "/links/two", "alice", http.StatusNoContent},
		{"GET", "/links/two", "bob", http.StatusNoContent},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		request.Header.Set("X-User-ID", test.user)
		request.Header.Set("X-Tenant-ID", "tenant")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("%s %s for %s = %d, want %d", test.method, test.path, test.user, response.Code, test.status)
		}
		if test.status == http.StatusTooManyRequests && response.Header().Get("Retry-After") != "1" {
			t.Errorf("Retry-After = %q, want 1", response.Header().Get("Retry-After"))
		}
	}
}
