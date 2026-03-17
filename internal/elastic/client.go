package elastic

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/jillesvangurp/formation-web-analytics/internal/config"
	"github.com/jillesvangurp/formation-web-analytics/internal/events"
	"github.com/jillesvangurp/formation-web-analytics/internal/metrics"
)

type BulkSender interface {
	Send(context.Context, []events.Event) (BulkResult, error)
	Ping(context.Context) error
}

type Client struct {
	cfg        config.Config
	httpClient *http.Client
	metrics    *metrics.Registry
}

type BulkResult struct {
	Indexed   int
	Failed    int
	Retryable bool
}

type bulkResponse struct {
	Errors bool `json:"errors"`
	Items  []struct {
		Create struct {
			Status int `json:"status"`
			Error  *struct {
				Type string `json:"type"`
			} `json:"error,omitempty"`
		} `json:"create"`
	} `json:"items"`
}

func New(cfg config.Config, registry *metrics.Registry) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		metrics: registry,
	}
}

func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.cfg.ElasticsearchURL, "/"), nil)
	if err != nil {
		return err
	}
	c.applyAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("elasticsearch ping returned %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) Send(ctx context.Context, batch []events.Event) (BulkResult, error) {
	payload, err := BuildBulkPayload(c.cfg.DataStream, batch)
	if err != nil {
		return BulkResult{}, err
	}

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(payload); err != nil {
		return BulkResult{}, err
	}
	if err := zw.Close(); err != nil {
		return BulkResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.ElasticsearchURL, "/")+"/_bulk", &compressed)
	if err != nil {
		return BulkResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("Content-Encoding", "gzip")
	c.applyAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return BulkResult{Retryable: true}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout {
		return BulkResult{Retryable: true}, fmt.Errorf("bulk request returned %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return BulkResult{}, fmt.Errorf("bulk request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed bulkResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return BulkResult{}, err
	}
	result := BulkResult{}
	if !parsed.Errors {
		result.Indexed = len(batch)
		return result, nil
	}
	for _, item := range parsed.Items {
		status := item.Create.Status
		if status >= 200 && status < 300 {
			result.Indexed++
			continue
		}
		result.Failed++
		if status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout {
			result.Retryable = true
		}
	}
	return result, nil
}

func BuildBulkPayload(dataStream string, batch []events.Event) ([]byte, error) {
	var b strings.Builder
	encoder := json.NewEncoder(&b)
	for _, event := range batch {
		action := map[string]any{
			"create": map[string]string{
				"_index": dataStream,
			},
		}
		if err := encoder.Encode(action); err != nil {
			return nil, err
		}
		document := map[string]any{
			"@timestamp":        event.TimestampValue().Format(time.RFC3339Nano),
			"received_at":       event.ReceivedAt.Format(time.RFC3339Nano),
			"type":              event.Type,
			"site_id":           event.SiteID,
			"request_domain":    event.RequestDomain,
			"request_host":      event.RequestHost,
			"collector_version": event.CollectorVersion,
			"traffic_quality":   event.TrafficQuality,
			"is_suspect":        event.IsSuspect,
			"payload":           event.Payload,
		}
		addIfNotEmpty(document, "session_id", event.SessionID)
		addIfNotEmpty(document, "anonymous_id", event.AnonymousID)
		if event.UserID != nil && *event.UserID != "" {
			document["user_id"] = event.UserID
		}
		addIfNotEmpty(document, "path", event.Path)
		addIfNotEmpty(document, "url", event.URL)
		addIfNotEmpty(document, "referrer", event.Referrer)
		addIfNotEmpty(document, "title", event.Title)
		addIfNotEmpty(document, "client_ip", event.ClientIP)
		addIfNotEmpty(document, "geo_country_iso_code", event.GeoCountryISO)
		addIfNotEmpty(document, "geo_country_name", event.GeoCountryName)
		addIfNotEmpty(document, "geo_city_name", event.GeoCityName)
		addIfNotEmpty(document, "forwarded_for", event.ForwardedFor)
		addIfNotEmpty(document, "user_agent", event.UserAgent)
		addIfNotEmpty(document, "accept_language", event.AcceptLanguage)
		addIfNotEmpty(document, "origin", event.Origin)
		addIfNotEmpty(document, "referer_header", event.RefererHeader)
		addIfNotEmpty(document, "scheme", event.Scheme)
		addIfNotEmpty(document, "remote_addr", event.RemoteAddr)
		if len(event.SuspicionReasons) > 0 {
			document["suspicion_reasons"] = event.SuspicionReasons
		}
		if err := encoder.Encode(document); err != nil {
			return nil, err
		}
	}
	return []byte(b.String()), nil
}

func addIfNotEmpty(document map[string]any, key, value string) {
	if value != "" {
		document[key] = value
	}
}

func Backoff(minimum, maximum time.Duration, attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	backoff := minimum << attempt
	if backoff > maximum {
		backoff = maximum
	}
	jitter := time.Duration(rand.Int63n(int64(backoff / 4)))
	return backoff + jitter
}

func (c *Client) applyAuth(req *http.Request) {
	if c.cfg.ElasticsearchAPIKey != "" {
		req.Header.Set("Authorization", "ApiKey "+c.cfg.ElasticsearchAPIKey)
		return
	}
	req.SetBasicAuth(c.cfg.ElasticsearchUsername, c.cfg.ElasticsearchPassword)
}
