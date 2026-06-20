package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adilbains/uzip/internal/analytics"
	"github.com/adilbains/uzip/internal/config"
	"github.com/adilbains/uzip/internal/enrichment"
	"github.com/adilbains/uzip/internal/events"
	"github.com/adilbains/uzip/internal/httpx"
	"github.com/adilbains/uzip/internal/model"
	"github.com/adilbains/uzip/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load("8082")
	shutdownTracing, err := observability.Setup(context.Background(), "event-processor")
	if err != nil {
		logger.Error("tracing", "error", err)
		os.Exit(1)
	}
	defer shutdownTracing(context.Background())
	store, err := analytics.Open(cfg.ClickHouseAddr, cfg.ClickHouseDB, cfg.ClickHouseUser, cfg.ClickHousePass, cfg.ClickHouseSecure)
	if err != nil {
		logger.Error("clickhouse", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	enricher, err := enrichment.Open(cfg.GeoIPDatabase)
	if err != nil {
		logger.Error("geoip", "error", err)
		os.Exit(1)
	}
	defer enricher.Close()
	consumer := events.NewConsumer(events.Config{Brokers: cfg.KafkaBrokers, Topic: cfg.KafkaTopic, Group: cfg.KafkaGroup, IAM: cfg.KafkaIAM, Region: cfg.AWSRegion})
	defer consumer.Close()
	dlq := events.NewPublisher(events.Config{Brokers: cfg.KafkaBrokers, Topic: cfg.KafkaDLQTopic, IAM: cfg.KafkaIAM, Region: cfg.AWSRegion})
	defer dlq.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	health := http.NewServeMux()
	metrics := &httpx.Metrics{}
	health.HandleFunc("GET /live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	health.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		if err := store.Ping(r.Context()); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	health.Handle("GET /metrics", metrics.Handler())
	go func() {
		if err := http.ListenAndServe(cfg.Addr(), health); err != nil {
			logger.Error("health server", "error", err)
			stop()
		}
	}()
	logger.Info("event processor started", "topic", cfg.KafkaTopic, "group", cfg.KafkaGroup)
	for {
		message, err := consumer.Fetch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error("fetch", "error", err)
			time.Sleep(time.Second)
			continue
		}
		var event model.ClickEvent
		if err := json.Unmarshal(message.Value, &event); err != nil {
			logger.Error("invalid event", "error", err, "offset", message.Offset)
			_ = dlq.PublishBytes(ctx, message.Key, message.Value)
			_ = consumer.Commit(ctx, message)
			continue
		}
		enricher.Apply(&event)
		carrier := propagation.MapCarrier{"traceparent": event.TraceParent}
		eventContext := otel.GetTextMapPropagator().Extract(ctx, carrier)
		eventContext, span := otel.Tracer("event-processor").Start(eventContext, "process click")
		var insertErr error
		for attempt := 1; attempt <= 3; attempt++ {
			insertErr = store.Insert(eventContext, event)
			if insertErr == nil {
				break
			}
			logger.Error("insert failed", "error", insertErr, "event_id", event.EventID, "attempt", attempt)
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		if insertErr != nil {
			if err := dlq.Publish(eventContext, event); err != nil {
				logger.Error("dlq publish failed", "error", err, "event_id", event.EventID)
				span.RecordError(err)
				span.End()
				continue
			}
		}
		if err := consumer.Commit(eventContext, message); err != nil {
			logger.Error("commit failed", "error", err)
		}
		span.End()
	}
}
