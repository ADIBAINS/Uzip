package events

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adilbains/uzip/internal/model"
	"github.com/aws/aws-msk-iam-sasl-signer-go/signer"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
)

type Publisher struct{ writer *kafka.Writer }

type Config struct {
	Brokers string
	Topic   string
	Group   string
	IAM     bool
	Region  string
}

func NewPublisher(config Config) *Publisher {
	transport := &kafka.Transport{}
	if config.IAM {
		transport.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
		transport.SASL = oauthMechanism{region: config.Region}
	}
	return &Publisher{writer: &kafka.Writer{
		Addr: kafka.TCP(strings.Split(config.Brokers, ",")...), Topic: config.Topic, Transport: transport,
		Balancer: &kafka.Hash{}, BatchTimeout: 10 * time.Millisecond, RequiredAcks: kafka.RequireOne,
		Async: true,
	}}
}

func (p *Publisher) Publish(ctx context.Context, event model.ClickEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.PublishBytes(ctx, []byte(event.TenantID), data)
}

func (p *Publisher) PublishBytes(ctx context.Context, key, data []byte) error {
	return p.writer.WriteMessages(ctx, kafka.Message{Key: key, Value: data, Time: time.Now().UTC()})
}

func (p *Publisher) Close() error { return p.writer.Close() }

type Consumer struct{ reader *kafka.Reader }

func NewConsumer(config Config) *Consumer {
	dialer := dialer(config)
	return &Consumer{reader: kafka.NewReader(kafka.ReaderConfig{
		Brokers: strings.Split(config.Brokers, ","), Topic: config.Topic, GroupID: config.Group, Dialer: dialer,
		MinBytes: 1, MaxBytes: 10e6, CommitInterval: 0,
	})}
}

func (c *Consumer) Fetch(ctx context.Context) (kafka.Message, error) {
	return c.reader.FetchMessage(ctx)
}
func (c *Consumer) Commit(ctx context.Context, message kafka.Message) error {
	return c.reader.CommitMessages(ctx, message)
}
func (c *Consumer) Close() error { return c.reader.Close() }

func EnsureTopics(ctx context.Context, config Config, topics ...string) error {
	dialer := dialer(config)
	broker := strings.Split(config.Brokers, ",")[0]
	connection, err := dialer.DialContext(ctx, "tcp", broker)
	if err != nil {
		return fmt.Errorf("connect to kafka: %w", err)
	}
	defer connection.Close()
	controller, err := connection.Controller()
	if err != nil {
		return fmt.Errorf("find kafka controller: %w", err)
	}
	controllerAddress := fmt.Sprintf("%s:%d", controller.Host, controller.Port)
	controllerConnection, err := dialer.DialContext(ctx, "tcp", controllerAddress)
	if err != nil {
		return fmt.Errorf("connect to kafka controller: %w", err)
	}
	defer controllerConnection.Close()
	configs := make([]kafka.TopicConfig, 0, len(topics))
	for _, topic := range topics {
		configs = append(configs, kafka.TopicConfig{Topic: topic, NumPartitions: 12, ReplicationFactor: -1})
	}
	if err := controllerConnection.CreateTopics(configs...); err != nil && !strings.Contains(err.Error(), "Topic with this name already exists") {
		return fmt.Errorf("create kafka topics: %w", err)
	}
	return nil
}

func dialer(config Config) *kafka.Dialer {
	dialer := &kafka.Dialer{Timeout: 10 * time.Second}
	if config.IAM {
		dialer.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
		dialer.SASLMechanism = oauthMechanism{region: config.Region}
	}
	return dialer
}

type oauthMechanism struct{ region string }
type oauthSession struct{}

func (oauthMechanism) Name() string { return "OAUTHBEARER" }

func (m oauthMechanism) Start(ctx context.Context) (sasl.StateMachine, []byte, error) {
	token, _, err := signer.GenerateAuthToken(ctx, m.region)
	if err != nil {
		return nil, nil, fmt.Errorf("generate MSK IAM token: %w", err)
	}
	initial := []byte("n,,\x01auth=Bearer " + token + "\x01\x01")
	return oauthSession{}, initial, nil
}

func (oauthSession) Next(_ context.Context, challenge []byte) (bool, []byte, error) {
	if len(challenge) != 0 {
		return false, nil, fmt.Errorf("MSK rejected OAuth token: %s", challenge)
	}
	return true, nil, nil
}
