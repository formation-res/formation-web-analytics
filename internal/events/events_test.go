package events

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jillesvangurp/formation-web-analytics/internal/config"
)

func TestDecodeBatchAcceptsSingleEvent(t *testing.T) {
	body := []byte(`{"type":"page_view","site_id":"marketing"}`)
	events, err := DecodeBatch(body)
	if err != nil {
		t.Fatalf("expected single event to decode: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestDecodeBatchRejectsUnknownFields(t *testing.T) {
	body := []byte(`{"type":"page_view","site_id":"marketing","bogus":true}`)
	if _, err := DecodeBatch(body); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func TestDecodeBatchRejectsTrailingContent(t *testing.T) {
	body := []byte(`{"type":"page_view","site_id":"marketing"}{"extra":true}`)
	if _, err := DecodeBatch(body); err == nil {
		t.Fatal("expected trailing content to be rejected")
	}
}

func TestEnrichUsesForwardedHeaders(t *testing.T) {
	cfg := config.Config{
		TrustProxyHeaders: true,
		CaptureClientIP:   true,
		StoreIPMetadata:   false,
		SanitizeURLs:      true,
		CollectorVersion:  "test",
	}
	req := httptest.NewRequest("POST", "http://collector/collect", nil)
	req.Host = "collector.internal"
	req.RemoteAddr = "10.0.0.2:1234"
	req.Header.Set("X-Forwarded-Host", "www.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("User-Agent", "test-agent")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Origin", "https://www.example.com")
	req.Header.Set("Referer", "https://www.example.com/page")

	event := Event{Type: "page_view", SiteID: "site"}
	domain, clientIP := Enrich(req, cfg, &event, time.Unix(0, 0))

	if domain != "www.example.com" {
		t.Fatalf("unexpected domain: %s", domain)
	}
	if event.ClientIP != "1.2.3.4" {
		t.Fatalf("expected client ip from proxy header")
	}
	if clientIP != "1.2.3.4" {
		t.Fatalf("expected resolved client ip")
	}
	if event.Scheme != "https" {
		t.Fatalf("expected https scheme")
	}
	if event.ForwardedFor != "" || event.RemoteAddr != "" {
		t.Fatalf("expected IP metadata not to be stored by default")
	}
	if event.RefererHeader != "https://www.example.com/page" {
		t.Fatalf("expected referer header to remain sanitized without query, got %s", event.RefererHeader)
	}
}

func TestEnrichSanitizesTrackedURLs(t *testing.T) {
	cfg := config.Config{
		SanitizeURLs:     true,
		CollectorVersion: "test",
	}
	req := httptest.NewRequest("POST", "http://collector/collect", nil)
	req.Header.Set("Origin", "https://www.example.com")
	req.Header.Set("Referer", "https://www.example.com/source?token=secret#frag")

	event := Event{
		Type:     "page_view",
		SiteID:   "site",
		URL:      "https://www.example.com/pricing?email=test@example.com#hero",
		Referrer: "https://search.example.com/?q=private",
		Path:     "/pricing?email=test@example.com#hero",
	}
	Enrich(req, cfg, &event, time.Unix(0, 0))

	if event.URL != "https://www.example.com/pricing" {
		t.Fatalf("expected sanitized URL, got %s", event.URL)
	}
	if event.Referrer != "https://search.example.com/" {
		t.Fatalf("expected sanitized referrer, got %s", event.Referrer)
	}
	if event.Path != "/pricing" {
		t.Fatalf("expected sanitized path, got %s", event.Path)
	}
	if event.RefererHeader != "https://www.example.com/source" {
		t.Fatalf("expected sanitized referer header, got %s", event.RefererHeader)
	}
}

func TestValidateRejectsInvalidPayload(t *testing.T) {
	cfg := config.Config{
		MaxFieldLength:    32,
		MaxPayloadEntries: 4,
		MaxPayloadDepth:   2,
	}
	event := Event{
		Type:    "page_view",
		SiteID:  "site",
		Payload: map[string]any{"one": map[string]any{"two": map[string]any{"three": "x"}}},
	}
	if err := event.Validate(cfg); err == nil {
		t.Fatal("expected payload depth validation error")
	}
}
