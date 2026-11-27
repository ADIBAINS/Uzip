package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/adilbains/uzip/internal/analytics"
	"github.com/adilbains/uzip/internal/auth"
	"github.com/adilbains/uzip/internal/config"
	"github.com/adilbains/uzip/internal/httpx"
	"github.com/adilbains/uzip/internal/model"
	"github.com/adilbains/uzip/internal/observability"
	"github.com/adilbains/uzip/internal/store"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type analyticsReader interface {
	Ping(context.Context) error
	Summary(context.Context, string, string, time.Time, time.Time) (model.AnalyticsSummary, error)
	TimeSeries(context.Context, string, string, string, time.Time, time.Time) ([]model.TimePoint, error)
	Breakdown(context.Context, string, string, string, time.Time, time.Time) ([]model.Breakdown, error)
}

type api struct{ store analyticsReader }

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load("8083")
	shutdownTracing, err := observability.Setup(context.Background(), "analytics-api")
	if err != nil {
		logger.Error("tracing", "error", err)
		os.Exit(1)
	}
	defer shutdownTracing(context.Background())
	if err := cfg.ValidateAuth(); err != nil {
		logger.Error("config", "error", err)
		os.Exit(1)
	}
	cache, err := store.OpenCache(cfg.RedisURL)
	if err != nil {
		logger.Error("redis", "error", err)
		os.Exit(1)
	}
	defer cache.Close()
	app := &api{}
	if cfg.RenderDemo {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		db, err := store.OpenPostgres(ctx, cfg.PostgresURL)
		cancel()
		if err != nil {
			logger.Error("postgres", "error", err)
			os.Exit(1)
		}
		defer db.Close()
		app.store = db
	} else {
		db, err := analytics.Open(cfg.ClickHouseAddr, cfg.ClickHouseDB, cfg.ClickHouseUser, cfg.ClickHousePass, cfg.ClickHouseSecure)
		if err != nil {
			logger.Error("clickhouse", "error", err)
			os.Exit(1)
		}
		defer db.Close()
		app.store = db
	}
	mux := http.NewServeMux()
	httpx.Health(mux, func() error { return app.store.Ping(context.Background()) })
	metrics := &httpx.Metrics{}
	mux.Handle("GET /metrics", metrics.Handler())
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/analytics/summary", app.summary)
	protected.HandleFunc("GET /api/v1/analytics/timeseries", app.timeseries)
	protected.HandleFunc("GET /api/v1/analytics/breakdown/{dimension}", app.breakdown)
	mux.Handle("/api/", auth.NewVerifier(cfg.CognitoIssuer, cfg.CognitoAudience, cfg.DevAuth).Middleware(auth.RateLimit(protected, cache.RateLimit)))
	root := otelhttp.NewHandler(httpx.CORS(cfg.AllowedOrigins, metrics.Middleware(httpx.Recover(logger, httpx.Log(logger, mux)))), "http.server")
	if err := httpx.Serve(cfg.Addr(), root, cfg.ShutdownTimeout, logger); err != nil {
		logger.Error("server", "error", err)
		os.Exit(1)
	}
}

func (a *api) summary(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.Principal(r.Context())
	from, to := rangeFromRequest(r)
	result, err := a.store.Summary(r.Context(), p.TenantID, r.URL.Query().Get("link_id"), from, to)
	if err != nil {
		httpx.Error(w, 500, "query_failed", "could not load analytics")
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": result, "from": from, "to": to})
}

func (a *api) timeseries(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.Principal(r.Context())
	from, to := rangeFromRequest(r)
	result, err := a.store.TimeSeries(r.Context(), p.TenantID, r.URL.Query().Get("link_id"), r.URL.Query().Get("interval"), from, to)
	if err != nil {
		httpx.Error(w, 500, "query_failed", "could not load analytics")
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": result, "from": from, "to": to})
}

func (a *api) breakdown(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.Principal(r.Context())
	from, to := rangeFromRequest(r)
	result, err := a.store.Breakdown(r.Context(), p.TenantID, r.URL.Query().Get("link_id"), r.PathValue("dimension"), from, to)
	if err != nil {
		httpx.Error(w, 422, "invalid_dimension", err.Error())
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": result, "from": from, "to": to})
}

func rangeFromRequest(r *http.Request) (time.Time, time.Time) {
	to := time.Now().UTC()
	from := to.Add(-30 * 24 * time.Hour)
	if parsed, err := time.Parse(time.RFC3339, r.URL.Query().Get("from")); err == nil {
		from = parsed
	}
	if parsed, err := time.Parse(time.RFC3339, r.URL.Query().Get("to")); err == nil {
		to = parsed
	}
	if from.After(to) {
		from = to.Add(-30 * 24 * time.Hour)
	}
	return from, to
}
