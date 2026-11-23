package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port             string
	BindHost         string
	RenderDemo       bool
	PostgresURL      string
	RedisURL         string
	KafkaBrokers     string
	KafkaTopic       string
	KafkaDLQTopic    string
	KafkaGroup       string
	KafkaIAM         bool
	AWSRegion        string
	ClickHouseAddr   string
	ClickHouseDB     string
	ClickHouseUser   string
	ClickHousePass   string
	ClickHouseSecure bool
	CognitoIssuer    string
	CognitoAudience  string
	DevAuth          bool
	RedirectBaseURL  string
	GeoIPDatabase    string
	AllowedOrigins   string
	ShutdownTimeout  time.Duration
}

func Load(defaultPort string) Config {
	return Config{
		Port:             env("PORT", defaultPort),
		BindHost:         os.Getenv("BIND_HOST"),
		RenderDemo:       envBool("UZIP_RENDER_DEMO", false),
		PostgresURL:      env("POSTGRES_URL", "postgres://uzip:uzip@localhost:5432/uzip?sslmode=disable"),
		RedisURL:         env("REDIS_URL", "redis://localhost:6379/0"),
		KafkaBrokers:     env("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopic:       env("KAFKA_TOPIC", "clicks.raw.v1"),
		KafkaDLQTopic:    env("KAFKA_DLQ_TOPIC", "clicks.dlq.v1"),
		KafkaGroup:       env("KAFKA_GROUP", "uzip-event-processor-v1"),
		KafkaIAM:         envBool("KAFKA_IAM", false),
		AWSRegion:        env("AWS_REGION", "us-east-1"),
		ClickHouseAddr:   env("CLICKHOUSE_ADDR", "localhost:9000"),
		ClickHouseDB:     env("CLICKHOUSE_DATABASE", "uzip"),
		ClickHouseUser:   env("CLICKHOUSE_USER", "default"),
		ClickHousePass:   os.Getenv("CLICKHOUSE_PASSWORD"),
		ClickHouseSecure: envBool("CLICKHOUSE_SECURE", false),
		CognitoIssuer:    os.Getenv("COGNITO_ISSUER"),
		CognitoAudience:  os.Getenv("COGNITO_AUDIENCE"),
		DevAuth:          envBool("DEV_AUTH", true),
		RedirectBaseURL:  env("REDIRECT_BASE_URL", "http://localhost:8081"),
		GeoIPDatabase:    os.Getenv("GEOIP_DATABASE"),
		AllowedOrigins:   env("ALLOWED_ORIGINS", "http://localhost:3000"),
		ShutdownTimeout:  10 * time.Second,
	}
}

func (c Config) Addr() string { return net.JoinHostPort(c.BindHost, c.Port) }

func (c Config) ValidateAuth() error {
	if !c.DevAuth && (c.CognitoIssuer == "" || c.CognitoAudience == "") {
		return fmt.Errorf("COGNITO_ISSUER and COGNITO_AUDIENCE are required when DEV_AUTH=false")
	}
	return nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
