package elastic

import (
	"strings"
	"testing"
	"time"

	"github.com/formation-res/formation-web-analytics/internal/events"
)

func TestBuildBulkPayload(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	payload, err := BuildBulkPayload("web-analytics", []events.Event{{
		Type:           "page_view",
		SiteID:         "site",
		Timestamp:      now.Format(time.RFC3339Nano),
		ReceivedAt:     now,
		TrafficQuality: "suspect",
		IsSuspect:      true,
		SuspicionReasons: []string{
			"user_agent:headless",
		},
		Payload: map[string]any{
			"utm_source": "google",
		},
	}})
	if err != nil {
		t.Fatalf("expected payload generation to succeed: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "\"_index\":\"web-analytics\"") {
		t.Fatalf("expected data stream in payload: %s", text)
	}
	if !strings.Contains(text, "\"type\":\"page_view\"") {
		t.Fatalf("expected event type in payload")
	}
	if !strings.Contains(text, "\"traffic_quality\":\"suspect\"") || !strings.Contains(text, "\"is_suspect\":true") {
		t.Fatalf("expected traffic quality fields in payload: %s", text)
	}
	if strings.Contains(text, "\"forwarded_for\"") || strings.Contains(text, "\"remote_addr\"") {
		t.Fatalf("expected empty IP metadata fields to be omitted: %s", text)
	}
}

func TestBackoffBounds(t *testing.T) {
	delay := Backoff(time.Second, 5*time.Second, 10)
	if delay < 5*time.Second || delay >= 5*time.Second+1250*time.Millisecond {
		t.Fatalf("unexpected backoff: %s", delay)
	}
}
