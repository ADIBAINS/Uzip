package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
