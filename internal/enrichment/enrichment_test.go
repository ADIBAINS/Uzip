package enrichment

import (
	"testing"

	"github.com/adilbains/uzip/internal/model"
)

func TestApplyClassifiesAndRemovesIP(t *testing.T) {
	event := model.ClickEvent{
		UserAgent: "Mozilla/5.0 (iPhone) AppleWebKit Safari/605.1",
		Referrer:  "https://news.example.com/article",
		IPAddress: "192.0.2.1",
	}
	(&Enricher{}).Apply(&event)
	if event.Device != "Mobile" || event.Browser != "Safari" || event.OS != "iOS" {
		t.Fatalf("unexpected classification: %+v", event)
	}
	if event.IPAddress != "" || event.Referrer != "news.example.com" || event.IsBot {
		t.Fatalf("privacy or referrer classification failed: %+v", event)
	}
}

func TestApplyDetectsBots(t *testing.T) {
	event := model.ClickEvent{UserAgent: "Googlebot/2.1", IPAddress: "192.0.2.1"}
	(&Enricher{}).Apply(&event)
	if !event.IsBot {
		t.Fatal("expected bot classification")
	}
}
