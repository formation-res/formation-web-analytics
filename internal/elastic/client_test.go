package elastic

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/formation-res/formation-web-analytics/internal/config"
	"github.com/formation-res/formation-web-analytics/internal/events"
	"github.com/formation-res/formation-web-analytics/internal/geo"
	"github.com/formation-res/formation-web-analytics/internal/metrics"
)

func TestBuildBulkPayload(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	payload, err := BuildBulkPayload("web-analytics", []events.Event{{
		Type:                        "page_view",
		SiteID:                      "site",
		Timestamp:                   now.Format(time.RFC3339Nano),
		ReceivedAt:                  now,
		TrafficQuality:              "suspect",
		IsSuspect:                   true,
		BrowserFamily:               "Chrome",
		BrowserVersion:              "136.0.0",
		BrowserMajor:                "136",
		BrowserEngine:               "Blink",
		OSFamily:                    "Mac OS X",
		OSVersion:                   "10.15.7",
		DeviceFamily:                "Mac",
		DeviceType:                  "desktop",
		AcceptLanguage:              "en-US,en;q=0.9",
		AcceptLanguagePrimaryTag:    "en-US",
		AcceptLanguagePrimaryBase:   "en",
		AcceptLanguagePrimaryRegion: "US",
		AcceptLanguageTags:          []string{"en-US", "en"},
		Timezone:                    "Europe/Berlin",
		TimezoneArea:                "Europe",
		TimezoneLocation:            "Berlin",
		GeoLocation: &geo.Point{
			Latitude:  52.3676,
			Longitude: 4.9041,
		},
		SuspicionReasons: []string{
			"user_agent:headless",
		},
		Payload: map[string]any{
			"utm_source":         "google",
			"conversation_id":    "conversation-123",
			"turn_index":         float64(4),
			"locale":             "en",
			"consent_state":      "accepted",
			"conversation_stage": "profile_collection",
			"actor":              "assistant",
			"message_role":       "assistant",
			"message_format":     "rendered_html",
			"response_kind":      "survey_prompt",
			"source":             "command",
			"has_read_more":      false,
			"has_suggestions":    true,
			"initiated_by":       "assistant",
			"navigation_source":  "assistant_payload",
			"destination_title":  "See Services",
			"destination_href":   "/services/",
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
	if !strings.Contains(text, "\"browser_family\":\"Chrome\"") || !strings.Contains(text, "\"os_family\":\"Mac OS X\"") || !strings.Contains(text, "\"device_family\":\"Mac\"") {
		t.Fatalf("expected parsed user agent fields in payload: %s", text)
	}
	if !strings.Contains(text, "\"browser_engine\":\"Blink\"") || !strings.Contains(text, "\"device_type\":\"desktop\"") {
		t.Fatalf("expected derived user agent fields in payload: %s", text)
	}
	if !strings.Contains(text, "\"accept_language_primary_tag\":\"en-US\"") || !strings.Contains(text, "\"accept_language_tags\":[\"en-US\",\"en\"]") {
		t.Fatalf("expected parsed accept-language fields in payload: %s", text)
	}
	if !strings.Contains(text, "\"timezone\":\"Europe/Berlin\"") || !strings.Contains(text, "\"timezone_area\":\"Europe\"") {
		t.Fatalf("expected timezone fields in payload: %s", text)
	}
	if !strings.Contains(text, "\"geo_location\":{\"lat\":52.3676,\"lon\":4.9041}") {
		t.Fatalf("expected geo point in payload: %s", text)
	}
	if !strings.Contains(text, "\"conversation_id\":\"conversation-123\"") || !strings.Contains(text, "\"turn_index\":4") {
		t.Fatalf("expected promoted conversation fields in payload: %s", text)
	}
	if !strings.Contains(text, "\"response_kind\":\"survey_prompt\"") || !strings.Contains(text, "\"has_suggestions\":true") {
		t.Fatalf("expected promoted chat fields in payload: %s", text)
	}
	if !strings.Contains(text, "\"destination_href\":\"/services/\"") || !strings.Contains(text, "\"navigation_source\":\"assistant_payload\"") {
		t.Fatalf("expected promoted navigation fields in payload: %s", text)
	}
	if strings.Contains(text, "\"forwarded_for\"") || strings.Contains(text, "\"remote_addr\"") {
		t.Fatalf("expected empty IP metadata fields to be omitted: %s", text)
	}
}

func TestBuildBulkPayloadOmitsMalformedGeoPoint(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	payload, err := BuildBulkPayload("web-analytics", []events.Event{{
		Type:       "page_view",
		SiteID:     "site",
		Timestamp:  now.Format(time.RFC3339Nano),
		ReceivedAt: now,
		GeoLocation: &geo.Point{
			Latitude:  200,
			Longitude: 4.9041,
		},
		Payload: map[string]any{},
	}})
	if err != nil {
		t.Fatalf("expected payload generation to succeed: %v", err)
	}
	if strings.Contains(string(payload), "\"geo_location\"") {
		t.Fatalf("expected malformed geo point to be omitted: %s", string(payload))
	}
}

func TestBackoffBounds(t *testing.T) {
	delay := Backoff(time.Second, 5*time.Second, 10)
	if delay < 5*time.Second || delay >= 5*time.Second+1250*time.Millisecond {
		t.Fatalf("unexpected backoff: %s", delay)
	}
}

func TestBuildBulkPayloadKeepsStableDocumentID(t *testing.T) {
	batch := []events.Event{{
		Type:       "page_view",
		SiteID:     "site",
		Timestamp:  time.Unix(0, 0).UTC().Format(time.RFC3339Nano),
		ReceivedAt: time.Unix(0, 0).UTC(),
	}}
	first, err := BuildBulkPayload("web-analytics", batch)
	if err != nil {
		t.Fatalf("failed to build first payload: %v", err)
	}
	if batch[0].DocumentID == "" {
		t.Fatal("expected document id to be assigned")
	}
	second, err := BuildBulkPayload("web-analytics", batch)
	if err != nil {
		t.Fatalf("failed to build retry payload: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("expected retries to preserve the same bulk payload and document id")
	}
}

func TestSendReturnsOnlyRetryableItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":true,"items":[{"create":{"status":201}},{"create":{"status":429}},{"create":{"status":400}}]}`))
	}))
	defer server.Close()

	client := New(config.Config{
		ElasticsearchURL:    server.URL,
		ElasticsearchAPIKey: "test",
		DataStream:          "web-analytics",
	}, metrics.New())
	batch := []events.Event{
		{DocumentID: "one", Type: "page_view", SiteID: "site"},
		{DocumentID: "two", Type: "click", SiteID: "site"},
		{DocumentID: "three", Type: "invalid", SiteID: "site"},
	}
	result, err := client.Send(t.Context(), batch)
	if err != nil {
		t.Fatalf("expected item failures to be reported in the result: %v", err)
	}
	if result.Indexed != 1 || result.Failed != 1 {
		t.Fatalf("unexpected result counts: %#v", result)
	}
	if len(result.Retry) != 1 || result.Retry[0].DocumentID != "two" {
		t.Fatalf("expected only the 429 item to be retried: %#v", result.Retry)
	}
}

func TestSendTreatsStableIDConflictAsIndexed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":true,"items":[{"create":{"status":409}}]}`))
	}))
	defer server.Close()

	client := New(config.Config{
		ElasticsearchURL:    server.URL,
		ElasticsearchAPIKey: "test",
		DataStream:          "web-analytics",
	}, metrics.New())
	result, err := client.Send(t.Context(), []events.Event{{DocumentID: "stable", Type: "page_view", SiteID: "site"}})
	if err != nil {
		t.Fatalf("expected idempotent conflict to succeed: %v", err)
	}
	if result.Indexed != 1 || result.Failed != 0 || len(result.Retry) != 0 {
		t.Fatalf("unexpected conflict result: %#v", result)
	}
}

func TestSendRetriesAmbiguousResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":`))
	}))
	defer server.Close()

	client := New(config.Config{
		ElasticsearchURL:    server.URL,
		ElasticsearchAPIKey: "test",
		DataStream:          "web-analytics",
	}, metrics.New())
	batch := []events.Event{{DocumentID: "stable", Type: "page_view", SiteID: "site"}}
	result, err := client.Send(t.Context(), batch)
	if err == nil {
		t.Fatal("expected malformed response to fail")
	}
	if len(result.Retry) != 1 || result.Retry[0].DocumentID != "stable" {
		t.Fatalf("expected ambiguous response to retry the original event: %#v", result.Retry)
	}
}

func TestBackoffHandlesTinyAndReversedBounds(t *testing.T) {
	if got := Backoff(time.Nanosecond, 0, 1000); got != time.Nanosecond {
		t.Fatalf("unexpected defensive backoff: %s", got)
	}
}
