package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/adilbains/uzip/internal/config"
	"github.com/adilbains/uzip/internal/events"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load("8084")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	topics := []string{cfg.KafkaTopic, cfg.KafkaDLQTopic}
	if retry := strings.TrimSpace(os.Getenv("KAFKA_RETRY_TOPIC")); retry != "" {
		topics = append(topics, retry)
	}
	if err := events.EnsureTopics(ctx, events.Config{Brokers: cfg.KafkaBrokers, IAM: cfg.KafkaIAM, Region: cfg.AWSRegion}, topics...); err != nil {
		logger.Error("topic provisioning failed", "error", err)
		os.Exit(1)
	}
	logger.Info("kafka topics ready", "topics", topics)
}
