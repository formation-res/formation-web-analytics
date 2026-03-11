package elastic

import (
	"strings"
	"testing"
	"time"

	"github.com/jillesvangurp/formation-web-analytics/internal/events"
)

func TestBuildBulkPayload(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	payload, err := BuildBulkPayload("analytics-events", []events.Event{{
		Type:       "page_view",
		SiteID:     "site",
		Timestamp:  now.Format(time.RFC3339Nano),
		ReceivedAt: now,
		Payload: map[string]any{
			"utm_source": "google",
		},
	}})
	if err != nil {
		t.Fatalf("expected payload generation to succeed: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "\"_index\":\"analytics-events\"") {
		t.Fatalf("expected data stream in payload: %s", text)
	}
	if !strings.Contains(text, "\"type\":\"page_view\"") {
		t.Fatalf("expected event type in payload")
	}
}

func TestBackoffBounds(t *testing.T) {
	delay := Backoff(time.Second, 5*time.Second, 10)
	if delay < 5*time.Second || delay >= 5*time.Second+1250*time.Millisecond {
		t.Fatalf("unexpected backoff: %s", delay)
	}
}
