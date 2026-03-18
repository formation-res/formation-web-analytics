package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvalidEventIncludesValidationDetail(t *testing.T) {
	server := newTestServer(testConfig())

	body := `{"type":"chat_answer","site_id":"site","url":"https://example.com","timezone":"Mars/Olympus","payload":{"rendered_html":"` + strings.Repeat("x", 257) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(body))
	setBrowserHeaders(req)
	req.Header.Set("Origin", "https://example.com")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response["error"] != "invalid_event" {
		t.Fatalf("expected invalid_event, got %v", response["error"])
	}
	if response["detail"] != "invalid timezone" {
		t.Fatalf("expected invalid timezone detail, got %v", response["detail"])
	}
}

func TestOversizedPayloadIsTruncatedAndAccepted(t *testing.T) {
	server := newTestServer(testConfig())

	body := `{"type":"chat_answer","site_id":"site","url":"https://example.com","payload":{"rendered_html":"` + strings.Repeat("x", 257) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewBufferString(body))
	setBrowserHeaders(req)
	req.Header.Set("Origin", "https://example.com")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
}
