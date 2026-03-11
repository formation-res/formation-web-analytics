package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jillesvangurp/formation-web-analytics/internal/config"
)

type Event struct {
	Type             string            `json:"type"`
	SiteID           string            `json:"site_id"`
	Timestamp        string            `json:"timestamp,omitempty"`
	SessionID        string            `json:"session_id,omitempty"`
	AnonymousID      string            `json:"anonymous_id,omitempty"`
	UserID           *string           `json:"user_id,omitempty"`
	Path             string            `json:"path,omitempty"`
	URL              string            `json:"url,omitempty"`
	Referrer         string            `json:"referrer,omitempty"`
	Title            string            `json:"title,omitempty"`
	Payload          map[string]any    `json:"payload,omitempty"`
	ReceivedAt       time.Time         `json:"received_at,omitempty"`
	RequestHost      string            `json:"request_host,omitempty"`
	RequestDomain    string            `json:"request_domain,omitempty"`
	ClientIP         string            `json:"client_ip,omitempty"`
	GeoCountryISO    string            `json:"geo_country_iso_code,omitempty"`
	GeoCountryName   string            `json:"geo_country_name,omitempty"`
	GeoCityName      string            `json:"geo_city_name,omitempty"`
	ForwardedFor     string            `json:"forwarded_for,omitempty"`
	UserAgent        string            `json:"user_agent,omitempty"`
	AcceptLanguage   string            `json:"accept_language,omitempty"`
	Origin           string            `json:"origin,omitempty"`
	RefererHeader    string            `json:"referer_header,omitempty"`
	Scheme           string            `json:"scheme,omitempty"`
	RemoteAddr       string            `json:"remote_addr,omitempty"`
	CollectorVersion string            `json:"collector_version,omitempty"`
	ExtraHeaders     map[string]string `json:"-"`
}

type BatchRequest struct {
	Events []Event `json:"events"`
}

var identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,128}$`)
var ErrEmptyBatch = errors.New("empty batch")

func (e *Event) Normalize(now time.Time) {
	e.ReceivedAt = now.UTC()
	if strings.TrimSpace(e.Timestamp) == "" {
		e.Timestamp = e.ReceivedAt.Format(time.RFC3339Nano)
	} else if parsed, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
		e.Timestamp = parsed.UTC().Format(time.RFC3339Nano)
	} else {
		e.Timestamp = e.ReceivedAt.Format(time.RFC3339Nano)
	}
	if e.Payload == nil {
		e.Payload = map[string]any{}
	}
}

func (e *Event) Validate(cfg config.Config) error {
	if !identifierPattern.MatchString(strings.TrimSpace(e.Type)) {
		return errors.New("invalid type")
	}
	if !identifierPattern.MatchString(strings.TrimSpace(e.SiteID)) {
		return errors.New("invalid site_id")
	}
	if tooLong(e.SessionID, cfg.MaxFieldLength) || tooLong(e.AnonymousID, cfg.MaxFieldLength) {
		return errors.New("invalid identifiers")
	}
	if e.UserID != nil && tooLong(*e.UserID, cfg.MaxFieldLength) {
		return errors.New("invalid user_id")
	}
	if tooLong(e.Path, cfg.MaxFieldLength) || tooLong(e.Title, cfg.MaxFieldLength) {
		return errors.New("invalid path or title")
	}
	if err := validateURLField(e.URL, cfg.MaxFieldLength); err != nil {
		return err
	}
	if err := validateURLField(e.Referrer, cfg.MaxFieldLength); err != nil {
		return err
	}
	if err := validatePayload(e.Payload, cfg.MaxPayloadEntries, cfg.MaxPayloadDepth, cfg.MaxFieldLength); err != nil {
		return err
	}
	return nil
}

func (e Event) TimestampValue() time.Time {
	if t, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
		return t.UTC()
	}
	return e.ReceivedAt.UTC()
}

func Enrich(r *http.Request, cfg config.Config, event *Event, now time.Time) (string, string) {
	event.Normalize(now)
	event.RequestHost = effectiveHost(r, cfg.TrustProxyHeaders)
	event.RequestDomain = config.NormalizeDomain(event.RequestHost)
	resolvedClientIP := clientIP(r, cfg.TrustProxyHeaders)
	event.ForwardedFor = strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	event.UserAgent = strings.TrimSpace(r.Header.Get("User-Agent"))
	event.AcceptLanguage = strings.TrimSpace(r.Header.Get("Accept-Language"))
	event.Origin = strings.TrimSpace(r.Header.Get("Origin"))
	event.RefererHeader = strings.TrimSpace(r.Header.Get("Referer"))
	event.Scheme = scheme(r, cfg.TrustProxyHeaders)
	event.RemoteAddr = r.RemoteAddr
	event.CollectorVersion = cfg.CollectorVersion
	if cfg.CaptureClientIP {
		event.ClientIP = resolvedClientIP
	}
	return event.RequestDomain, resolvedClientIP
}

func DecodeBatch(body []byte) ([]Event, error) {
	if batch, ok, err := decodeBatchRequest(body); err != nil {
		return nil, err
	} else if ok {
		if len(batch.Events) == 0 {
			return nil, ErrEmptyBatch
		}
		return batch.Events, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var single Event
	if err := decoder.Decode(&single); err != nil {
		return nil, err
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, err
	}
	return []Event{single}, nil
}

func AllowedDomain(cfg config.Config, domain string) bool {
	_, ok := cfg.AllowedDomainSet[config.NormalizeDomain(domain)]
	return ok
}

func effectiveHost(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); host != "" {
			return host
		}
	}
	if r.Host != "" {
		return r.Host
	}
	return r.URL.Host
}

func scheme(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
			return forwarded
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func clientIP(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			return strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func tooLong(value string, max int) bool {
	return len(value) > max
}

func validateURLField(raw string, max int) error {
	if raw == "" {
		return nil
	}
	if len(raw) > max {
		return errors.New("url field too long")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid url")
	}
	if parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("invalid url scheme")
	}
	return nil
}

func validatePayload(payload map[string]any, maxEntries, maxDepth, maxFieldLength int) error {
	if payload == nil {
		return nil
	}
	count, err := validatePayloadValue(payload, 1, maxDepth, maxFieldLength)
	if err != nil {
		return err
	}
	if count > maxEntries {
		return errors.New("payload has too many entries")
	}
	return nil
}

func validatePayloadValue(value any, depth, maxDepth, maxFieldLength int) (int, error) {
	if depth > maxDepth {
		return 0, errors.New("payload exceeds max depth")
	}
	switch typed := value.(type) {
	case nil, bool, float64:
		return 1, nil
	case string:
		if len(typed) > maxFieldLength {
			return 0, errors.New("payload string too long")
		}
		return 1, nil
	case map[string]any:
		count := 0
		for key, nested := range typed {
			if key == "" || len(key) > maxFieldLength {
				return 0, errors.New("invalid payload key")
			}
			nestedCount, err := validatePayloadValue(nested, depth+1, maxDepth, maxFieldLength)
			if err != nil {
				return 0, err
			}
			count += 1 + nestedCount
		}
		return count, nil
	case []any:
		count := 0
		for _, nested := range typed {
			nestedCount, err := validatePayloadValue(nested, depth+1, maxDepth, maxFieldLength)
			if err != nil {
				return 0, err
			}
			count += nestedCount
		}
		return count, nil
	default:
		return 0, errors.New("payload contains unsupported type")
	}
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("unexpected trailing content")
}

func decodeBatchRequest(body []byte) (BatchRequest, bool, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return BatchRequest{}, false, nil
	}
	rawEvents, ok := probe["events"]
	if !ok {
		return BatchRequest{}, false, nil
	}
	if len(probe) != 1 {
		return BatchRequest{}, false, errors.New("unexpected fields in batch request")
	}
	decoder := json.NewDecoder(bytes.NewReader(rawEvents))
	decoder.DisallowUnknownFields()
	var events []Event
	if err := decoder.Decode(&events); err != nil {
		return BatchRequest{}, true, err
	}
	if err := ensureEOF(decoder); err != nil {
		return BatchRequest{}, true, err
	}
	return BatchRequest{Events: events}, true, nil
}
