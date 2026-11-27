package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/adilbains/uzip/internal/config"
	"github.com/adilbains/uzip/internal/enrichment"
	"github.com/adilbains/uzip/internal/events"
	"github.com/adilbains/uzip/internal/httpx"
	"github.com/adilbains/uzip/internal/model"
	"github.com/adilbains/uzip/internal/observability"
	"github.com/adilbains/uzip/internal/store"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type redirector struct {
	db      *store.Postgres
	cache   *store.Cache
	clicks  chan model.ClickEvent
	metrics *httpx.Metrics
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load("8081")
	shutdownTracing, err := observability.Setup(context.Background(), "redirect")
	if err != nil {
		logger.Error("tracing", "error", err)
		os.Exit(1)
	}
	defer shutdownTracing(context.Background())
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
	metrics := &httpx.Metrics{}
	app := &redirector{db: db, cache: cache, clicks: make(chan model.ClickEvent, 1024), metrics: metrics}
	var publish func(context.Context, model.ClickEvent) error
	if cfg.RenderDemo {
		enricher, err := enrichment.Open("")
		if err != nil {
			logger.Error("enrichment", "error", err)
			os.Exit(1)
		}
		defer enricher.Close()
		publish = func(ctx context.Context, event model.ClickEvent) error {
			enricher.Apply(&event)
			event.Referrer = ""
			return db.RecordDemoClick(ctx, event)
		}
	} else {
		publisher := events.NewPublisher(events.Config{
			Brokers: cfg.KafkaBrokers, Topic: cfg.KafkaTopic, IAM: cfg.KafkaIAM, Region: cfg.AWSRegion, Async: true,
			OnDeliveryError: func(count int, err error) {
				metrics.AddErrors(uint64(count))
				logger.Warn("click delivery failed", "count", count, "error", err)
			},
		})
		defer publisher.Close()
		publish = publisher.Publish
	}
	clickContext, stopClicks := context.WithCancel(context.Background())
	defer stopClicks()
	drained := make(chan struct{})
	go app.publishClicks(clickContext, drained, publish, logger)
	mux := http.NewServeMux()
	httpx.Health(mux, func() error { return db.Ping(context.Background()) })
	mux.Handle("GET /metrics", metrics.Handler())
	mux.HandleFunc("GET /{alias}", app.redirect)
	root := otelhttp.NewHandler(metrics.Middleware(httpx.Recover(logger, httpx.Log(logger, mux))), "redirect")
	if err := httpx.Serve(cfg.Addr(), root, cfg.ShutdownTimeout, logger); err != nil {
		logger.Error("server", "error", err)
		os.Exit(1)
	}
	close(app.clicks)
	timer := time.NewTimer(cfg.ShutdownTimeout)
	defer timer.Stop()
	select {
	case <-drained:
	case <-timer.C:
		stopClicks()
		<-drained
	}
}

func (a *redirector) redirect(w http.ResponseWriter, r *http.Request) {
	alias := strings.ToLower(r.PathValue("alias"))
	delay, rateErr := a.cache.RateLimit(r.Context(), "redirect:"+clientIP(r), 300, time.Minute)
	if rateErr == nil && httpx.RateLimited(w, delay) {
		return
	}
	link, found, err := a.cache.GetLink(r.Context(), alias)
	if err != nil {
		found = false
	}
	if !found {
		link, err = a.db.ResolveAlias(r.Context(), alias)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			httpx.Error(w, 503, "resolution_failed", "link resolution unavailable")
			return
		}
		_ = a.cache.CacheResolvedLink(r.Context(), link)
	}
	if !link.Active(time.Now()) {
		httpx.Error(w, http.StatusGone, "link_inactive", "link is expired or archived")
		return
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(r.Context(), carrier)
	event := model.ClickEvent{
		SchemaVersion: 1, EventID: uuid.NewString(), OccurredAt: time.Now().UTC(), TenantID: link.TenantID,
		LinkID: link.ID, Alias: link.Alias, VisitorID: visitorID(r), IPAddress: clientIP(r),
		UserAgent: r.UserAgent(), Referrer: r.Referer(), TraceParent: carrier.Get("traceparent"),
	}
	select {
	case a.clicks <- event:
	default:
		a.metrics.Error()
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, link.Destination, link.Redirect)
}

func (a *redirector) publishClicks(ctx context.Context, drained chan<- struct{}, publish func(context.Context, model.ClickEvent) error, logger *slog.Logger) {
	defer close(drained)
	for event := range a.clicks {
		if ctx.Err() != nil {
			return
		}
		eventContext, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
		err := publish(eventContext, event)
		cancel()
		if err != nil {
			a.metrics.Error()
			logger.Warn("could not queue click for delivery", "event_id", event.EventID, "error", err)
		}
	}
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func visitorID(r *http.Request) string {
	sum := sha256.Sum256([]byte(clientIP(r) + "|" + r.UserAgent() + "|" + time.Now().UTC().Format("2006-01-02")))
	return hex.EncodeToString(sum[:16])
}
