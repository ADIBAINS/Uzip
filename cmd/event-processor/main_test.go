package main

import (
	"testing"

	"github.com/adilbains/uzip/internal/enrichment"
	"github.com/adilbains/uzip/internal/model"
)

func TestEnrich(t *testing.T) {
	t.Parallel()
	event := model.ClickEvent{
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit Safari/605.1",
		Referrer:  "https://news.example.com/article?id=1",
		IPAddress: "192.0.2.10",
	}
	enricher, _ := enrichment.Open("")
	enricher.Apply(&event)
	if event.Device != "Mobile" || event.Browser != "Safari" || event.OS != "iOS" {
		t.Fatalf("unexpected enrichment: %+v", event)
	}
	if event.Referrer != "news.example.com" || event.IPAddress != "" {
		t.Fatalf("privacy/referrer normalization failed: %+v", event)
	}
}
