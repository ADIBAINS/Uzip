//go:build integration

package store

import (
	"context"
	"os"
	"testing"

	"github.com/adilbains/uzip/internal/model"
	"github.com/google/uuid"
)

func TestTenantRowLevelSecurity(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := OpenPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	adminURL := os.Getenv("TEST_DATABASE_ADMIN_URL")
	admin, err := OpenPostgres(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Pool.Exec(ctx, `INSERT INTO tenants(id,name,slug) VALUES ($1::uuid,'A',$1::text),($2::uuid,'B',$2::text)`, tenantA, tenantB); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Pool.Exec(ctx, `DELETE FROM tenants WHERE id IN ($1,$2)`, tenantA, tenantB) })

	link, err := db.CreateLink(ctx, model.Principal{TenantID: tenantA}, model.CreateLink{Alias: "rls-" + tenantA[:8], Destination: "https://example.com", Redirect: 302})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetLink(ctx, tenantB, link.ID); err != ErrNotFound {
		t.Fatalf("tenant B accessed tenant A link: %v", err)
	}
	if _, err := db.ArchiveLink(ctx, tenantB, link.ID); err != ErrNotFound {
		t.Fatalf("tenant B mutated tenant A link: %v", err)
	}
	for _, query := range []string{"RLS-", "EXAMPLE.COM"} {
		matches, err := db.ListLinks(ctx, tenantA, query, 20, 0)
		if err != nil || len(matches) != 1 || matches[0].ID != link.ID {
			t.Fatalf("tenant A search %q = %v, %v", query, matches, err)
		}
		matches, err = db.ListLinks(ctx, tenantB, query, 20, 0)
		if err != nil || len(matches) != 0 {
			t.Fatalf("tenant B search %q = %v, %v", query, matches, err)
		}
	}
	if matches, err := db.ListLinks(ctx, tenantA, "%", 20, 0); err != nil || len(matches) != 0 {
		t.Fatalf("literal percent search = %v, %v", matches, err)
	}
	archived, err := db.ArchiveLink(ctx, tenantA, link.ID)
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("archive = %v, %v", archived, err)
	}
	again, err := db.ArchiveLink(ctx, tenantA, link.ID)
	if err != nil || again.ArchivedAt == nil || !again.ArchivedAt.Equal(*archived.ArchivedAt) {
		t.Fatalf("repeated archive = %v, %v", again, err)
	}
}
