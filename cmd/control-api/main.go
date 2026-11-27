package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/adilbains/uzip/internal/auth"
	"github.com/adilbains/uzip/internal/config"
	"github.com/adilbains/uzip/internal/httpx"
	"github.com/adilbains/uzip/internal/model"
	"github.com/adilbains/uzip/internal/observability"
	"github.com/adilbains/uzip/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type api struct {
	db         *store.Postgres
	cache      *store.Cache
	baseURL    string
	renderDemo bool
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load("8080")
	shutdownTracing, err := observability.Setup(context.Background(), "control-api")
	if err != nil {
		logger.Error("tracing", "error", err)
		os.Exit(1)
	}
	defer shutdownTracing(context.Background())
	if err := cfg.ValidateAuth(); err != nil {
		logger.Error("config", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := store.OpenPostgres(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Error("postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	cache, err := store.OpenCache(cfg.RedisURL)
	if err != nil {
		logger.Error("redis", "error", err)
		os.Exit(1)
	}
	defer cache.Close()
	if cfg.DevAuth {
		if err := db.EnsureDevelopmentTenant(ctx); err != nil {
			logger.Error("seed development team", "error", err)
			os.Exit(1)
		}
	}

	handler := &api{db: db, cache: cache, baseURL: cfg.RedirectBaseURL, renderDemo: cfg.RenderDemo}
	mux := http.NewServeMux()
	httpx.Health(mux, func() error { return db.Ping(context.Background()) })
	metrics := &httpx.Metrics{}
	mux.Handle("GET /metrics", metrics.Handler())
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/links", handler.listLinks)
	protected.HandleFunc("POST /api/v1/links", handler.createLink)
	protected.HandleFunc("GET /api/v1/links/{id}", handler.getLink)
	protected.Handle("DELETE /api/v1/links/{id}", auth.RequireRole("owner", "admin")(http.HandlerFunc(handler.archiveLink)))
	protected.Handle("GET /api/v1/api-keys", auth.RequireRole("owner", "admin")(http.HandlerFunc(handler.listAPIKeys)))
	protected.Handle("POST /api/v1/api-keys", auth.RequireRole("owner", "admin")(http.HandlerFunc(handler.createAPIKey)))
	protected.Handle("DELETE /api/v1/api-keys/{id}", auth.RequireRole("owner", "admin")(http.HandlerFunc(handler.revokeAPIKey)))
	verifier := auth.NewVerifier(cfg.CognitoIssuer, cfg.CognitoAudience, cfg.DevAuth)
	mux.Handle("/api/", verifier.Middleware(rateLimit(cache, protected)))
	root := otelhttp.NewHandler(httpx.CORS(cfg.AllowedOrigins, metrics.Middleware(httpx.Recover(logger, httpx.Log(logger, mux)))), "http.server")
	if err := httpx.Serve(cfg.Addr(), root, cfg.ShutdownTimeout, logger); err != nil {
		logger.Error("server", "error", err)
		os.Exit(1)
	}
}

func (a *api) createAPIKey(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Principal(r.Context())
	var input struct {
		Name string `json:"name"`
	}
	if !httpx.Decode(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 80 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_name", "name must contain 1-80 characters")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		httpx.Error(w, 500, "key_generation_failed", "could not generate API key")
		return
	}
	secret := "uz_live_" + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(secret))
	key, err := a.db.CreateAPIKey(r.Context(), principal.TenantID, input.Name, secret[:12], digest[:])
	if err != nil {
		httpx.Error(w, 500, "create_failed", "could not create API key")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": key, "secret": secret})
}

func (a *api) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Principal(r.Context())
	keys, err := a.db.ListAPIKeys(r.Context(), principal.TenantID)
	if err != nil {
		httpx.Error(w, 500, "query_failed", "could not list API keys")
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": keys})
}

func (a *api) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Principal(r.Context())
	err := a.db.RevokeAPIKey(r.Context(), principal.TenantID, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, 404, "not_found", "API key not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "revoke_failed", "could not revoke API key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func rateLimit(cache *store.Cache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, _ := auth.Principal(r.Context())
		allowed, err := cache.Allow(r.Context(), "control:"+principal.TenantID, 300, time.Minute)
		if err == nil && !allowed {
			w.Header().Set("Retry-After", "60")
			httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "request limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *api) createLink(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Principal(r.Context())
	var input model.CreateLink
	if !httpx.Decode(w, r, &input) {
		return
	}
	if err := validateDestination(input.Destination); err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_destination", err.Error())
		return
	}
	if a.renderDemo {
		destination, _ := url.ParseRequestURI(input.Destination)
		host := strings.ToLower(destination.Hostname())
		if destination.User != nil || destination.Port() != "" ||
			(host != "example.com" && host != "example.org" && host != "example.net") {
			httpx.Error(w, http.StatusUnprocessableEntity, "demo_destination", "Try a URL on example.com, example.org, or example.net in the public demo.")
			return
		}
	}
	if input.Redirect == 0 {
		input.Redirect = http.StatusFound
	}
	if input.Redirect != http.StatusMovedPermanently && input.Redirect != http.StatusFound {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_redirect", "redirect_type must be 301 or 302")
		return
	}
	if input.Alias == "" {
		input.Alias = randomAlias(7)
	}
	input.Alias = strings.ToLower(strings.TrimSpace(input.Alias))
	if !validAlias(input.Alias) {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_alias", "alias must contain 3-64 lowercase letters, numbers, hyphens, or underscores")
		return
	}
	link, err := a.db.CreateLink(r.Context(), principal.TenantID, input)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httpx.Error(w, http.StatusConflict, "alias_taken", "alias already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "create_failed", "could not create link")
		return
	}
	_ = a.cache.SetLink(r.Context(), link)
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": link, "short_url": strings.TrimRight(a.baseURL, "/") + "/" + link.Alias})
}

func (a *api) listLinks(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Principal(r.Context())
	limit := clampInt(r.URL.Query().Get("limit"), 20, 1, 100)
	offset := clampInt(r.URL.Query().Get("offset"), 0, 0, 100000)
	links, err := a.db.ListLinks(r.Context(), principal.TenantID, limit, offset)
	if err != nil {
		httpx.Error(w, 500, "query_failed", "could not list links")
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": links, "pagination": map[string]int{"limit": limit, "offset": offset}})
}

func (a *api) getLink(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Principal(r.Context())
	link, err := a.db.GetLink(r.Context(), principal.TenantID, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, 404, "not_found", "link not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "query_failed", "could not retrieve link")
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": link})
}

func (a *api) archiveLink(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Principal(r.Context())
	link, err := a.db.GetLink(r.Context(), principal.TenantID, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, 404, "not_found", "link not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "query_failed", "could not retrieve link")
		return
	}
	if err := a.db.ArchiveLink(r.Context(), principal.TenantID, link.ID); err != nil {
		httpx.Error(w, 500, "archive_failed", "could not archive link")
		return
	}
	_ = a.cache.Delete(r.Context(), link.Alias)
	w.WriteHeader(http.StatusNoContent)
}

func validateDestination(raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return errors.New("destination must be an absolute HTTP(S) URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".local") {
		return errors.New("local destinations are not allowed")
	}
	return nil
}

func validAlias(alias string) bool {
	if alias == "live" || alias == "ready" || alias == "metrics" {
		return false
	}
	if len(alias) < 3 || len(alias) > 64 {
		return false
	}
	for _, ch := range alias {
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

func randomAlias(length int) string {
	const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	result := make([]byte, length)
	for i := range result {
		value, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		result[i] = alphabet[value.Int64()]
	}
	return strings.ToLower(string(result))
}

func clampInt(raw string, fallback, min, max int) int {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
