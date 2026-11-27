//go:build integration

package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/adilbains/uzip/internal/model"
	"github.com/google/uuid"
)

func integrationCache(t *testing.T) *Cache {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	cache, err := OpenCache(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	return cache
}

func TestArchiveWinsAgainstRedirectCacheFill(t *testing.T) {
	cache := integrationCache(t)
	ctx := context.Background()
	link := model.Link{Alias: uuid.NewString(), Destination: "https://example.com", Redirect: 302}
	if err := cache.CacheResolvedLink(ctx, link); err != nil {
		t.Fatal(err)
	}
	archived := link
	now := time.Now()
	archived.ArchivedAt = &now
	if err := cache.CacheArchivedLink(ctx, archived); err != nil {
		t.Fatal(err)
	}
	if err := cache.CacheResolvedLink(ctx, link); err != nil {
		t.Fatal(err)
	}
	cached, found, err := cache.GetLink(ctx, link.Alias)
	if err != nil || !found || cached.Active(now) {
		t.Fatalf("archive was overwritten: found=%t link=%+v err=%v", found, cached, err)
	}
}

func TestRateLimitRefillsBeforeWindowEnds(t *testing.T) {
	cache := integrationCache(t)
	ctx := context.Background()
	key := uuid.NewString()
	for range 2 {
		if delay, err := cache.RateLimit(ctx, key, 2, time.Second); err != nil || delay != 0 {
			t.Fatalf("filled bucket denied: delay=%v err=%v", delay, err)
		}
	}
	delay, err := cache.RateLimit(ctx, key, 2, time.Second)
	if err != nil || delay <= 0 || delay > 500*time.Millisecond {
		t.Fatalf("empty bucket retry: delay=%v err=%v", delay, err)
	}
	time.Sleep(delay + 5*time.Millisecond)
	if delay, err := cache.RateLimit(ctx, key, 2, time.Second); err != nil || delay != 0 {
		t.Fatalf("refilled bucket denied: delay=%v err=%v", delay, err)
	}
}
