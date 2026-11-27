package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/adilbains/uzip/internal/model"
	"github.com/adilbains/uzip/internal/store"
)

type Worker struct {
	db       *store.Postgres
	endpoint string
	token    string
	novu     bool
	client   *http.Client
	logger   *slog.Logger
}

func NewWorker(db *store.Postgres, endpoint, token, format string, logger *slog.Logger) (*Worker, error) {
	switch format {
	case "", "webhook":
	case "novu":
		if token == "" {
			return nil, errors.New("Novu delivery requires a notification webhook token")
		}
		if endpoint == "" {
			endpoint = "https://api.novu.co/v1/events/trigger"
		}
	default:
		return nil, errors.New("notification webhook format must be webhook or novu")
	}
	if endpoint != "" {
		target, err := url.Parse(endpoint)
		if err != nil || target.Hostname() == "" || target.User != nil ||
			(target.Scheme != "https" && !(target.Scheme == "http" && (target.Hostname() == "localhost" || target.Hostname() == "127.0.0.1"))) {
			return nil, errors.New("notification webhook must be an HTTPS URL or a loopback HTTP URL")
		}
	}
	return &Worker{db: db, endpoint: endpoint, token: token, novu: format == "novu", logger: logger, client: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (w *Worker) Run(ctx context.Context) {
	if w.endpoint != "" {
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					w.deliverBatch(ctx)
				}
			}
		}()
		defer func() { <-done }()
	}
	expiry := time.NewTicker(30 * time.Second)
	defer expiry.Stop()
	w.queueExpiry(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-expiry.C:
			w.queueExpiry(ctx)
		}
	}
}

func (w *Worker) queueExpiry(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := w.db.QueueExpiryNotifications(queryCtx); err != nil && ctx.Err() == nil {
		w.logger.Error("queue expiry notifications", "error", err)
	}
}

func (w *Worker) deliverBatch(ctx context.Context) {
	for range 25 {
		if ctx.Err() != nil {
			return
		}
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		delivery, err := w.db.ClaimNotification(queryCtx)
		cancel()
		if errors.Is(err, store.ErrNotFound) {
			return
		}
		if err != nil {
			w.logger.Error("claim notification", "error", err)
			return
		}
		var delay time.Duration
		if err := w.send(ctx, delivery); err != nil {
			delay = time.Minute * time.Duration(1<<min(delivery.Attempt-1, 6))
			w.logger.Warn("notification delivery failed", "id", delivery.ID, "attempt", delivery.Attempt, "error", err)
		}
		queryCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
		err = w.db.FinishNotification(queryCtx, delivery, delay)
		cancel()
		if err != nil {
			w.logger.Error("finish notification", "id", delivery.ID, "error", err)
			return
		}
	}
}

func (w *Worker) send(ctx context.Context, delivery store.NotificationDelivery) error {
	var payload any = delivery
	authScheme := "Bearer"
	if w.novu {
		authScheme = "ApiKey"
		payload = novuTrigger{
			Name:    "uzip-link-" + strings.ReplaceAll(delivery.Kind, "_", "-"),
			To:      novuSubscriber{ID: delivery.RecipientID, Email: delivery.RecipientEmail},
			Payload: delivery.Notification, TransactionID: delivery.ID,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, w.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", delivery.ID)
	if w.token != "" {
		request.Header.Set("Authorization", authScheme+" "+w.token)
	}
	response, err := w.client.Do(request)
	if err != nil {
		// A transport error can include the provider URL and its query string.
		return errors.New("provider request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider returned HTTP %d", response.StatusCode)
	}
	if w.novu {
		var receipt struct {
			novuReceipt
			Data *novuReceipt `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&receipt); err != nil {
			return errors.New("Novu returned an invalid receipt")
		}
		result := receipt.novuReceipt
		if receipt.Data != nil {
			result = *receipt.Data
		}
		if !result.Acknowledged || result.Status != "processed" {
			return errors.New("Novu did not accept the workflow trigger")
		}
	}
	return nil
}

type novuSubscriber struct {
	ID    string `json:"subscriberId"`
	Email string `json:"email,omitempty"`
}

type novuTrigger struct {
	Name          string             `json:"name"`
	To            novuSubscriber     `json:"to"`
	Payload       model.Notification `json:"payload"`
	TransactionID string             `json:"transactionId"`
}

type novuReceipt struct {
	Acknowledged bool   `json:"acknowledged"`
	Status       string `json:"status"`
}
