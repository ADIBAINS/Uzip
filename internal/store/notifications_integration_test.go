//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/adilbains/uzip/internal/model"
	"github.com/google/uuid"
)

func TestNotificationLifecycle(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	admin, err := OpenPostgres(ctx, os.Getenv("TEST_DATABASE_ADMIN_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	tenantID := uuid.NewString()
	if _, err := admin.Pool.Exec(ctx, `INSERT INTO tenants(id,name,slug) VALUES ($1::uuid,'Notifications',$1::text)`, tenantID); err != nil {
		t.Fatal(err)
	}
	defer admin.Pool.Exec(context.Background(), `DELETE FROM tenants WHERE id=$1`, tenantID)

	principal := model.Principal{TenantID: tenantID, UserID: "creator", Email: "creator@example.com", EmailVerified: true}
	expiry := time.Now().Add(time.Hour)
	input := model.CreateLink{Alias: "notify-" + tenantID[:8], Destination: "https://example.com", Redirect: 302, ExpiresAt: &expiry}
	link, err := db.CreateLink(ctx, principal, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Alias += "-archived"
	archived, err := db.CreateLink(ctx, principal, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ArchiveLink(ctx, tenantID, archived.ID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := db.QueueExpiryNotifications(ctx); err != nil {
			t.Fatal(err)
		}
	}

	event := model.ClickEvent{TenantID: tenantID, LinkID: link.ID}
	for range 100 {
		event.EventID = uuid.NewString()
		for range 2 {
			if err := db.RecordNotificationClick(ctx, event); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.RecordCreationFailure(ctx, principal, link.Alias); err != nil {
		t.Fatal(err)
	}
	entries, err := db.ListNotifications(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, entry := range entries {
		kinds[entry.Kind]++
		if entry.Kind == "click_milestone" && entry.Milestone != 100 {
			t.Fatalf("incorrect milestone: %+v", entry)
		}
		if entry.Kind == "expiring" && (entry.LinkID == nil || *entry.LinkID != link.ID) {
			t.Fatalf("incorrect link expiration: %+v", entry)
		}
	}
	if len(entries) != 5 || kinds["created"] != 2 || kinds["creation_failed"] != 1 || kinds["expiring"] != 1 || kinds["click_milestone"] != 1 {
		t.Fatalf("notification counts: %v", kinds)
	}
	if other, err := db.ListNotifications(ctx, model.Principal{TenantID: tenantID, UserID: "another-user"}); err != nil || len(other) > 0 {
		t.Fatalf("notifications leaked to another member: %v, %v", other, err)
	}

	claimed := map[string]bool{}
	for range 5 {
		delivery, err := db.ClaimNotification(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if claimed[delivery.ID] || delivery.Attempt != 1 || delivery.RecipientEmail != principal.Email {
			t.Fatalf("invalid delivery: %+v", delivery)
		}
		claimed[delivery.ID] = true
		if err := db.FinishNotification(ctx, delivery, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ClaimNotification(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected an empty delivery queue, got %v", err)
	}
}
