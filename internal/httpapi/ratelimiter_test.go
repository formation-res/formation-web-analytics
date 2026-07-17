package httpapi

import (
	"testing"
	"time"
)

func TestRateLimiterBoundsClientCardinality(t *testing.T) {
	limiter := newRateLimiter(10, 2)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	if !limiter.Allow("client-a", now) || !limiter.Allow("client-b", now) {
		t.Fatal("expected clients within the cap to be allowed")
	}
	if limiter.Allow("client-c", now) {
		t.Fatal("expected a new client beyond the cap to be rejected")
	}
	if !limiter.Allow("client-a", now) {
		t.Fatal("expected an existing client to continue within its rate limit")
	}
}

func TestRateLimiterPrunesOnceAtWindowChange(t *testing.T) {
	limiter := newRateLimiter(10, 1)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	if !limiter.Allow("old-client", now) {
		t.Fatal("expected initial client to be allowed")
	}
	if !limiter.Allow("new-client", now.Add(time.Minute)) {
		t.Fatal("expected previous-window state to be pruned")
	}
	if len(limiter.clients) != 1 {
		t.Fatalf("expected one active client, got %d", len(limiter.clients))
	}
}
