package notifications

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adilbains/uzip/internal/model"
	"github.com/adilbains/uzip/internal/store"
)

func TestNovuTrigger(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, receipt string
		accepted      bool
	}{
		{"accepted", `{"data":{"acknowledged":true,"status":"processed"}}`, true},
		{"inactive", `{"acknowledged":true,"status":"trigger_not_active"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := store.NotificationDelivery{
				Notification: model.Notification{ID: "event-1", Kind: "click_milestone", Milestone: 100},
				RecipientID:  "user-1", RecipientEmail: "verified@example.com",
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "ApiKey test-token" ||
					r.Header.Get("Idempotency-Key") != entry.ID {
					t.Errorf("unexpected provider request")
				}
				var trigger novuTrigger
				err := json.NewDecoder(r.Body).Decode(&trigger)
				if err != nil || trigger.Name != "uzip-link-click-milestone" || trigger.To.ID != entry.RecipientID ||
					trigger.To.Email != entry.RecipientEmail || trigger.TransactionID != entry.ID || trigger.Payload.Milestone != 100 {
					t.Errorf("unexpected provider payload: %+v, error=%v", trigger, err)
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(test.receipt))
			}))
			defer server.Close()
			worker, err := NewWorker(nil, server.URL, "test-token", "novu", slog.Default())
			if err != nil {
				t.Fatal(err)
			}
			if err := worker.send(t.Context(), entry); (err == nil) != test.accepted {
				t.Fatalf("delivery accepted=%t, error=%v", err == nil, err)
			}
		})
	}
}
