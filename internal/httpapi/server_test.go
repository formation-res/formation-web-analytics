package httpapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/formation-res/formation-web-analytics/internal/batcher"
	"github.com/formation-res/formation-web-analytics/internal/config"
	"github.com/formation-res/formation-web-analytics/internal/elastic"
	"github.com/formation-res/formation-web-analytics/internal/events"
	"github.com/formation-res/formation-web-analytics/internal/geo"
	"github.com/formation-res/formation-web-analytics/internal/metrics"
	"github.com/formation-res/formation-web-analytics/internal/queue"
)

type noopSender struct{}
type stubGeoResolver struct{}

func (noopSender) Send(context.Context, []events.Event) (elastic.BulkResult, error) {
	return elastic.BulkResult{}, nil
}

func (noopSender) Ping(context.Context) error { return nil }

func (stubGeoResolver) Lookup(ip string) (geo.Result, bool) {
	if strings.HasPrefix(ip, "127.") {
		return geo.Result{
			CountryISOCode: "LB",
			CountryName:    "Loopbackland",
			CityName:       "Loopback City",
		}, true
	}
	return geo.Result{}, false
}

func (stubGeoResolver) Close() error { return nil }

func testConfig() config.Config {
	return config.Config{
		AllowedDomainSet:    map[string]struct{}{"example.com": {}},
		AllowedDomains:      []string{"example.com"},
		DropPolicy:          config.DropPolicyReject,
		MaxPayloadBytes:     1024,
		MaxEventsPerRequest: 10,
		MaxFieldLength:      256,
		MaxPayloadEntries:   16,
		MaxPayloadDepth:     4,
		RateLimitPerMinute:  300,
		BlockedUserAgents:   []string{"bot", "crawler", "spider", "curl", "wget", "python-requests", "go-http-client"},
		SuspectUserAgents:   []string{"headless", "playwright", "puppeteer", "selenium", "phantomjs"},
		RequireOrigin:       true,
		RequireURLHostMatch: true,
		SanitizeURLs:        true,
	}
}

func newTestServer(cfg config.Config) *Server {
	registry := metrics.New()
	q := queue.New(10)
	b := batcher.New(config.Config{FlushInterval: time.Second, MaxBatchSize: 10}, q, noopSender{}, registry, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	return New(cfg, q, b, noopSender{}, stubGeoResolver{}, registry, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func setBrowserHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 TestBrowser")
}

func TestRejectsDisallowedDomain(t *testing.T) {
	server := newTestServer(testConfig())

	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"type":"page_view","site_id":"site","url":"https://evil.example"}`))
	setBrowserHeaders(req)
	req.Header.Set("Origin", "https://evil.example")
	req.Host = "evil.example"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestAcceptsAllowedDomain(t *testing.T) {
	cfg := testConfig()
	cfg.CollectorVersion = "test"
	server := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"type":"page_view","site_id":"site","url":"https://example.com"}`))
	setBrowserHeaders(req)
	req.Header.Set("Origin", "https://example.com")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func TestRejectsWrongContentType(t *testing.T) {
	server := newTestServer(testConfig())

	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"type":"page_view","site_id":"site"}`))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("User-Agent", "Mozilla/5.0 TestBrowser")
	req.Header.Set("Origin", "https://example.com")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
}

func TestRejectsTooManyEvents(t *testing.T) {
	cfg := testConfig()
	cfg.MaxPayloadBytes = 4096
	cfg.MaxEventsPerRequest = 1
	server := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"events":[{"type":"page_view","site_id":"site"},{"type":"click","site_id":"site"}]}`))
	setBrowserHeaders(req)
	req.Header.Set("Origin", "https://example.com")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestMetricsIsNotServedOnMainHandler(t *testing.T) {
	server := newTestServer(testConfig())

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRejectsEmptyBatch(t *testing.T) {
	cfg := testConfig()
	cfg.MaxPayloadBytes = 4096
	server := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"events":[]}`))
	setBrowserHeaders(req)
	req.Header.Set("Origin", "https://example.com")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRejectsMissingOrigin(t *testing.T) {
	server := newTestServer(testConfig())

	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"type":"page_view","site_id":"site","url":"https://example.com"}`))
	setBrowserHeaders(req)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRejectsBlockedUserAgent(t *testing.T) {
	server := newTestServer(testConfig())

	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"type":"page_view","site_id":"site","url":"https://example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("User-Agent", "curl/8.7.1")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestAcceptsSuspectUserAgentAndMarksQuality(t *testing.T) {
	bulkReceived := make(chan []byte, 1)
	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatalf("expected gzip bulk request: %v", err)
		}
		defer reader.Close()
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		bulkReceived <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":false,"items":[{"create":{"status":201}}]}`))
	}))
	defer esServer.Close()

	cfg := testConfig()
	cfg.MaxBatchSize = 1
	cfg.MaxQueueSize = 10
	cfg.FlushInterval = 20 * time.Millisecond
	cfg.MaxRetries = 0
	cfg.RetryMinBackoff = time.Millisecond
	cfg.RetryMaxBackoff = time.Millisecond
	cfg.ElasticsearchURL = esServer.URL
	cfg.ElasticsearchAPIKey = "test"
	cfg.DataStream = "web-analytics"

	registry := metrics.New()
	q := queue.New(cfg.MaxQueueSize)
	sender := elastic.New(cfg, registry)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	b := batcher.New(cfg, q, sender, registry, logger)
	server := New(cfg, q, b, sender, stubGeoResolver{}, registry, logger)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go b.Run(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/collect", strings.NewReader(`{"type":"page_view","site_id":"site","url":"https://example.com"}`))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 HeadlessChrome")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	select {
	case payload := <-bulkReceived:
		if !strings.Contains(string(payload), "\"traffic_quality\":\"suspect\"") || !strings.Contains(string(payload), "\"suspicion_reasons\":[\"user_agent:headless\"]") {
			t.Fatalf("expected suspect quality in payload, got %s", string(payload))
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for bulk request")
	}
}

func TestRejectsOriginNotAllowedForSite(t *testing.T) {
	cfg := testConfig()
	cfg.SiteOriginSet = map[string]map[string]struct{}{
		"site-a": {"example.com": {}},
	}
	server := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"type":"page_view","site_id":"site-a","url":"https://example.com"}`))
	setBrowserHeaders(req)
	req.Header.Set("Origin", "https://evil.example")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRejectsURLHostMismatch(t *testing.T) {
	server := newTestServer(testConfig())

	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"type":"page_view","site_id":"site","url":"https://evil.example"}`))
	setBrowserHeaders(req)
	req.Header.Set("Origin", "https://example.com")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRejectsRateLimitedClient(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimitPerMinute = 1
	server := newTestServer(cfg)

	first := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"type":"page_view","site_id":"site","url":"https://example.com"}`))
	setBrowserHeaders(first)
	first.Header.Set("Origin", "https://example.com")
	first.RemoteAddr = "1.2.3.4:1234"
	first.Host = "example.com"
	firstRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusAccepted {
		t.Fatalf("expected first request to pass, got %d", firstRec.Code)
	}

	second := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(`{"type":"page_view","site_id":"site","url":"https://example.com"}`))
	setBrowserHeaders(second)
	second.Header.Set("Origin", "https://example.com")
	second.RemoteAddr = "1.2.3.4:9999"
	second.Host = "example.com"
	secondRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(secondRec, second)

	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", secondRec.Code)
	}
}

func TestEndToEndCollectFlushesToBulkEndpoint(t *testing.T) {
	bulkReceived := make(chan []byte, 1)
	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_bulk" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatalf("expected gzip bulk request: %v", err)
		}
		defer reader.Close()
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("failed to read bulk body: %v", err)
		}
		select {
		case bulkReceived <- body:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":false,"items":[{"create":{"status":201}}]}`))
	}))
	defer esServer.Close()

	cfg := testConfig()
	cfg.MaxPayloadBytes = 4096
	cfg.MaxBatchSize = 1
	cfg.MaxQueueSize = 10
	cfg.FlushInterval = 20 * time.Millisecond
	cfg.MaxRetries = 0
	cfg.RetryMinBackoff = time.Millisecond
	cfg.RetryMaxBackoff = time.Millisecond
	cfg.ElasticsearchURL = esServer.URL
	cfg.ElasticsearchAPIKey = "test"
	cfg.DataStream = "web-analytics"
	cfg.CollectorVersion = "test"

	registry := metrics.New()
	q := queue.New(cfg.MaxQueueSize)
	sender := elastic.New(cfg, registry)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	b := batcher.New(cfg, q, sender, registry, logger)
	server := New(cfg, q, b, sender, stubGeoResolver{}, registry, logger)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go b.Run(ctx)

	apiServer := httptest.NewServer(server.Handler())
	defer apiServer.Close()

	requestBody := `{"type":"page_view","site_id":"site","path":"/pricing","url":"https://example.com/pricing","payload":{"utm_source":"google"}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiServer.URL+"/collect", strings.NewReader(requestBody))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 TestBrowser")
	req.Host = "example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to post collect request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected accepted response, got %d", resp.StatusCode)
	}

	var payload []byte
	select {
	case payload = <-bulkReceived:
	case <-ctx.Done():
		t.Fatal("timed out waiting for bulk request")
	}

	lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two ndjson lines, got %d", len(lines))
	}
	var action map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &action); err != nil {
		t.Fatalf("failed to decode action line: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &document); err != nil {
		t.Fatalf("failed to decode document line: %v", err)
	}
	if _, ok := action["create"]; !ok {
		t.Fatalf("expected create action")
	}
	if document["request_domain"] != "example.com" {
		t.Fatalf("expected request_domain enrichment, got %v", document["request_domain"])
	}
	if document["geo_country_iso_code"] != "LB" {
		t.Fatalf("expected geolocation enrichment, got %v", document["geo_country_iso_code"])
	}
	payloadMap, ok := document["payload"].(map[string]any)
	if !ok || payloadMap["utm_source"] != "google" {
		t.Fatalf("expected payload to be preserved: %#v", document["payload"])
	}
}
