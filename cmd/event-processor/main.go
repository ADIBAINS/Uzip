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
	"github.com/adilbains/uzip/internal/store"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
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
	clickHouse, err := analytics.Open(cfg.ClickHouseAddr, cfg.ClickHouseDB, cfg.ClickHouseUser, cfg.ClickHousePass, cfg.ClickHouseSecure)
	if err != nil {
		logger.Error("clickhouse", "error", err)
		os.Exit(1)
	}
	defer clickHouse.Close()
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
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	db, err := store.OpenPostgres(dbCtx, cfg.PostgresURL)
	cancel()
	if err != nil {
		logger.Error("notification postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	notificationConsumer := events.NewConsumer(events.Config{Brokers: cfg.KafkaBrokers, Topic: cfg.KafkaTopic,
		Group: cfg.KafkaGroup + "-notifications", IAM: cfg.KafkaIAM, Region: cfg.AWSRegion})
	defer notificationConsumer.Close()
	notificationDone := make(chan struct{})
	go func() {
		defer close(notificationDone)
		consumeNotificationClicks(ctx, notificationConsumer, db, logger)
	}()
	defer func() {
		stop()
		<-notificationDone
	}()
	health := http.NewServeMux()
	metrics := &httpx.Metrics{}
	health.HandleFunc("GET /live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	health.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		if err := clickHouse.Ping(r.Context()); err != nil {
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
		carrier := propagation.MapCarrier{}
		var event model.ClickEvent
		decodeErr := json.Unmarshal(message.Value, &event)
		if decodeErr == nil {
			carrier.Set("traceparent", event.TraceParent)
		}
		eventContext := otel.GetTextMapPropagator().Extract(ctx, carrier)
		eventContext, span := otel.Tracer("event-processor").Start(eventContext, "process click")
		sendToDLQ := decodeErr != nil
		if sendToDLQ {
			logger.Error("invalid event", "error", decodeErr, "offset", message.Offset)
			span.RecordError(decodeErr)
		} else {
			enricher.Apply(&event)
			for attempt := 1; attempt <= 3; attempt++ {
				insertErr := clickHouse.Insert(eventContext, event)
				sendToDLQ = insertErr != nil
				if !sendToDLQ {
					break
				}
				logger.Error("insert failed", "error", insertErr, "event_id", event.EventID, "attempt", attempt)
				span.RecordError(insertErr)
				if attempt < 3 && !waitForRetry(ctx, time.Duration(attempt)*time.Second) {
					span.End()
					return
				}
			}
		}
		if sendToDLQ && !retryKafka(eventContext, logger, "dlq publish failed", func() error {
			if decodeErr != nil {
				return dlq.PublishBytes(eventContext, message.Key, message.Value)
			}
			return dlq.Publish(eventContext, event)
		}) {
			span.End()
			return
		}
		// A later commit on this partition would also advance past this message.
		committed := retryKafka(eventContext, logger, "commit failed", func() error {
			return consumer.Commit(eventContext, message)
		})
		span.End()
		if !committed {
			return
		}
	}
}

func consumeNotificationClicks(ctx context.Context, consumer *events.Consumer, db *store.Postgres, logger *slog.Logger) {
	for ctx.Err() == nil {
		message, err := consumer.Fetch(ctx)
		if err != nil {
			if ctx.Err() == nil {
				logger.Error("fetch notification click", "error", err)
				waitForRetry(ctx, time.Second)
			}
			continue
		}
		var event model.ClickEvent
		if err := json.Unmarshal(message.Value, &event); err == nil && event.SchemaVersion == 1 &&
			uuid.Validate(event.EventID) == nil && uuid.Validate(event.TenantID) == nil && uuid.Validate(event.LinkID) == nil &&
			!enrichment.IsBot(event.UserAgent) {
			if !retryKafka(ctx, logger, "record notification click", func() error {
				queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				return db.RecordNotificationClick(queryCtx, event)
			}) {
				return
			}
		}
		if !retryKafka(ctx, logger, "commit notification click", func() error {
			return consumer.Commit(ctx, message)
		}) {
			return
		}
	}
}

func retryKafka(ctx context.Context, logger *slog.Logger, failure string, operation func() error) bool {
	for ctx.Err() == nil {
		err := operation()
		if err == nil {
			return true
		}
		logger.Error(failure, "error", err)
		oteltrace.SpanFromContext(ctx).RecordError(err)
		if !waitForRetry(ctx, time.Second) {
			break
		}
	}
	return false
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
