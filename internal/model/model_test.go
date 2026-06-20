package model

import (
	"testing"
	"time"
)

func TestLinkActive(t *testing.T) {
	t.Parallel()
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	if !(Link{}).Active(now) {
		t.Fatal("plain link should be active")
	}
	if (Link{ExpiresAt: &past}).Active(now) {
		t.Fatal("expired link should be inactive")
	}
	if !(Link{ExpiresAt: &future}).Active(now) {
		t.Fatal("unexpired link should be active")
	}
	if (Link{ArchivedAt: &past}).Active(now) {
		t.Fatal("archived link should be inactive")
	}
}
