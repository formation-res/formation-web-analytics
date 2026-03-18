package elastic

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/formation-res/formation-web-analytics/internal/config"
	"github.com/formation-res/formation-web-analytics/internal/events"
	"github.com/formation-res/formation-web-analytics/internal/metrics"
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
		if point, ok := geoPointDocument(event); ok {
			document["geo_location"] = point
		}
		addIfNotEmpty(document, "forwarded_for", event.ForwardedFor)
		addIfNotEmpty(document, "user_agent", event.UserAgent)
		addIfNotEmpty(document, "browser_family", event.BrowserFamily)
		addIfNotEmpty(document, "browser_version", event.BrowserVersion)
		addIfNotEmpty(document, "browser_major", event.BrowserMajor)
		addIfNotEmpty(document, "browser_engine", event.BrowserEngine)
		addIfNotEmpty(document, "os_family", event.OSFamily)
		addIfNotEmpty(document, "os_version", event.OSVersion)
		addIfNotEmpty(document, "device_family", event.DeviceFamily)
		addIfNotEmpty(document, "device_brand", event.DeviceBrand)
		addIfNotEmpty(document, "device_model", event.DeviceModel)
		addIfNotEmpty(document, "device_type", event.DeviceType)
		addIfNotEmpty(document, "accept_language", event.AcceptLanguage)
		addIfNotEmpty(document, "accept_language_primary_tag", event.AcceptLanguagePrimaryTag)
		addIfNotEmpty(document, "accept_language_primary_base", event.AcceptLanguagePrimaryBase)
		addIfNotEmpty(document, "accept_language_primary_region", event.AcceptLanguagePrimaryRegion)
		if len(event.AcceptLanguageTags) > 0 {
			document["accept_language_tags"] = event.AcceptLanguageTags
		}
		addIfNotEmpty(document, "timezone", event.Timezone)
		addIfNotEmpty(document, "timezone_area", event.TimezoneArea)
		addIfNotEmpty(document, "timezone_location", event.TimezoneLocation)
		if event.TimezoneOffsetMinutes != nil {
			document["timezone_offset_minutes"] = *event.TimezoneOffsetMinutes
		}
		addIfNotEmpty(document, "origin", event.Origin)
		addIfNotEmpty(document, "referer_header", event.RefererHeader)
		addIfNotEmpty(document, "scheme", event.Scheme)
		addIfNotEmpty(document, "remote_addr", event.RemoteAddr)
		if len(event.SuspicionReasons) > 0 {
			document["suspicion_reasons"] = event.SuspicionReasons
		}
		promotePayloadFields(document, event.Payload)
		if err := encoder.Encode(document); err != nil {
			return nil, err
		}
	}
	return []byte(b.String()), nil
}

func promotePayloadFields(document map[string]any, payload map[string]any) {
	if len(payload) == 0 {
		return
	}

	promoteStringField(document, payload, "conversation_id")
	promoteIntegerField(document, payload, "turn_index")
	promoteStringField(document, payload, "locale")
	promoteStringField(document, payload, "consent_state")
	promoteStringField(document, payload, "conversation_stage")
	promoteStringField(document, payload, "actor")
	promoteStringField(document, payload, "message_role")
	promoteStringField(document, payload, "message_format")
	promoteStringField(document, payload, "response_kind")
	promoteStringField(document, payload, "source")
	promoteStringField(document, payload, "action_type")
	promoteBooleanField(document, payload, "has_read_more")
	promoteBooleanField(document, payload, "has_suggestions")
	promoteStringField(document, payload, "initiated_by")
	promoteStringField(document, payload, "navigation_source")
	promoteStringField(document, payload, "destination_title")
	promoteStringField(document, payload, "destination_href")
}

func addIfNotEmpty(document map[string]any, key, value string) {
	if value != "" {
		document[key] = value
	}
}

func promoteStringField(document, payload map[string]any, key string) {
	value, ok := payload[key].(string)
	if !ok || value == "" {
		return
	}
	document[key] = value
}

func promoteBooleanField(document, payload map[string]any, key string) {
	value, ok := payload[key].(bool)
	if !ok {
		return
	}
	document[key] = value
}

func promoteIntegerField(document, payload map[string]any, key string) {
	value, ok := payload[key]
	if !ok {
		return
	}
	switch typed := value.(type) {
	case int:
		document[key] = typed
	case int32:
		document[key] = typed
	case int64:
		document[key] = typed
	case float64:
		if typed == float64(int64(typed)) {
			document[key] = int64(typed)
		}
	}
}

func geoPointDocument(event events.Event) (map[string]float64, bool) {
	if event.GeoLocation == nil {
		return nil, false
	}
	lat := event.GeoLocation.Latitude
	lon := event.GeoLocation.Longitude
	if math.IsNaN(lat) || math.IsInf(lat, 0) || math.IsNaN(lon) || math.IsInf(lon, 0) {
		return nil, false
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, false
	}
	return map[string]float64{"lat": lat, "lon": lon}, true
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
