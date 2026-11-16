package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/adilbains/uzip/internal/httpx"
	"github.com/adilbains/uzip/internal/model"
)

type contextKey string

const principalKey contextKey = "principal"

type Verifier struct {
	Issuer   string
	Audience string
	DevMode  bool
	client   *http.Client
	mu       sync.RWMutex
	keys     map[string]*rsa.PublicKey
}

func NewVerifier(issuer, audience string, dev bool) *Verifier {
	return &Verifier{Issuer: strings.TrimRight(issuer, "/"), Audience: audience, DevMode: dev, client: &http.Client{Timeout: 5 * time.Second}, keys: map[string]*rsa.PublicKey{}}
}

func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var principal model.Principal
		var err error
		if v.DevMode && r.Header.Get("X-User-ID") != "" {
			principal = model.Principal{
				UserID: r.Header.Get("X-User-ID"), TenantID: r.Header.Get("X-Tenant-ID"),
				Role: defaultString(r.Header.Get("X-Role"), "owner"), Email: r.Header.Get("X-Email"),
			}
		} else {
			principal, err = v.verify(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		}
		if err != nil || principal.UserID == "" || principal.TenantID == "" {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "valid authentication and tenant context are required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, principal)))
	})
}

func Principal(ctx context.Context) (model.Principal, bool) {
	p, ok := ctx.Value(principalKey).(model.Principal)
	return p, ok
}

func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := Principal(r.Context())
			if !ok || !allowed[p.Role] {
				httpx.Error(w, http.StatusForbidden, "forbidden", "insufficient role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type jwtHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
}
type jwtClaims struct {
	Subject  string `json:"sub"`
	Issuer   string `json:"iss"`
	Audience any    `json:"aud"`
	ClientID string `json:"client_id"`
	Expires  int64  `json:"exp"`
	Email    string `json:"email"`
	TenantID string `json:"custom:tenant_id"`
	Role     string `json:"custom:role"`
	TokenUse string `json:"token_use"`
}

func (v *Verifier) verify(ctx context.Context, token string) (model.Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return model.Principal{}, errors.New("invalid token")
	}
	var header jwtHeader
	var claims jwtClaims
	if err := decodePart(parts[0], &header); err != nil {
		return model.Principal{}, err
	}
	if err := decodePart(parts[1], &claims); err != nil {
		return model.Principal{}, err
	}
	if header.Algorithm != "RS256" || claims.TokenUse != "id" || claims.Issuer != v.Issuer || claims.Expires <= time.Now().Unix() || !audienceMatches(claims.Audience, claims.ClientID, v.Audience) {
		return model.Principal{}, errors.New("invalid claims")
	}
	key, err := v.key(ctx, header.KeyID)
	if err != nil {
		return model.Principal{}, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return model.Principal{}, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return model.Principal{}, err
	}
	return model.Principal{UserID: claims.Subject, TenantID: claims.TenantID, Role: defaultString(claims.Role, "member"), Email: claims.Email}, nil
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key := v.keys[kid]
	v.mu.RUnlock()
	if key != nil {
		return key, nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, v.Issuer+"/.well-known/jwks.json", nil)
	response, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks returned %s", response.Status)
	}
	var raw struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil {
		return nil, err
	}
	parsed := map[string]*rsa.PublicKey{}
	for _, item := range raw.Keys {
		nBytes, err := base64.RawURLEncoding.DecodeString(item["n"])
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(item["e"])
		if err != nil {
			continue
		}
		var exponent int
		for _, b := range eBytes {
			exponent = exponent<<8 + int(b)
		}
		parsed[item["kid"]] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: exponent}
	}
	v.mu.Lock()
	v.keys = parsed
	key = parsed[kid]
	v.mu.Unlock()
	if key == nil {
		return nil, errors.New("signing key not found")
	}
	return key, nil
}

func decodePart(value string, dst any) error {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func audienceMatches(aud any, clientID, expected string) bool {
	if clientID == expected {
		return true
	}
	switch value := aud.(type) {
	case string:
		return value == expected
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok && text == expected {
				return true
			}
		}
	}
	return false
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
