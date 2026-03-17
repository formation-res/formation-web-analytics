package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type DropPolicy string

const (
	DropPolicyReject     DropPolicy = "reject"
	DropPolicyDropNewest DropPolicy = "drop_newest"
)

type Config struct {
	ListenAddr            string
	MetricsListenAddr     string
	AllowedDomains        []string
	AllowedDomainSet      map[string]struct{}
	SiteOriginSet         map[string]map[string]struct{}
	ElasticsearchURL      string
	ElasticsearchAPIKey   string
	ElasticsearchUsername string
	ElasticsearchPassword string
	DataStream            string
	GeoIPDBPath           string
	FlushInterval         time.Duration
	MaxBatchSize          int
	MaxQueueSize          int
	MaxPayloadBytes       int64
	MaxEventsPerRequest   int
	MaxFieldLength        int
	MaxPayloadEntries     int
	MaxPayloadDepth       int
	MaxRetries            int
	RetryMinBackoff       time.Duration
	RetryMaxBackoff       time.Duration
	TrustProxyHeaders     bool
	CaptureClientIP       bool
	StoreIPMetadata       bool
	SanitizeURLs          bool
	RequireOrigin         bool
	RequireURLHostMatch   bool
	BlockedUserAgents     []string
	SuspectUserAgents     []string
	RateLimitPerMinute    int
	DropPolicy            DropPolicy
	LogLevel              string
	RetentionDays         int
	RequireElasticReady   bool
	MetricsEnabled        bool
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	CollectorVersion      string
}

func Load(version string) (Config, error) {
	cfg := Config{
		ListenAddr:          envOrDefault("LISTEN_ADDR", ":8080"),
		MetricsListenAddr:   envOrDefault("METRICS_LISTEN_ADDR", ":9090"),
		ElasticsearchURL:    strings.TrimSpace(os.Getenv("ELASTICSEARCH_URL")),
		ElasticsearchAPIKey: strings.TrimSpace(os.Getenv("ELASTICSEARCH_API_KEY")),
		ElasticsearchUsername: strings.TrimSpace(
			os.Getenv("ELASTICSEARCH_USERNAME"),
		),
		ElasticsearchPassword: strings.TrimSpace(
			os.Getenv("ELASTICSEARCH_PASSWORD"),
		),
		DataStream:       envOrDefault("ELASTICSEARCH_DATA_STREAM", "web-analytics"),
		GeoIPDBPath:      strings.TrimSpace(os.Getenv("GEOIP_DB_PATH")),
		LogLevel:         envOrDefault("LOG_LEVEL", "info"),
		CollectorVersion: version,
	}

	var err error
	if cfg.AllowedDomains, cfg.AllowedDomainSet, err = parseDomains(os.Getenv("ALLOWED_DOMAINS")); err != nil {
		return Config{}, err
	}
	if cfg.SiteOriginSet, err = parseSiteOrigins(os.Getenv("SITE_ORIGIN_MAP")); err != nil {
		return Config{}, err
	}

	if len(cfg.AllowedDomains) == 0 {
		return Config{}, errors.New("ALLOWED_DOMAINS is required")
	}
	if cfg.ElasticsearchURL == "" {
		return Config{}, errors.New("ELASTICSEARCH_URL is required")
	}
	if cfg.GeoIPDBPath == "" {
		return Config{}, errors.New("GEOIP_DB_PATH is required")
	}
	if cfg.ElasticsearchAPIKey == "" && (cfg.ElasticsearchUsername == "" || cfg.ElasticsearchPassword == "") {
		return Config{}, errors.New("ELASTICSEARCH_API_KEY or username/password is required")
	}

	if cfg.FlushInterval, err = duration("FLUSH_INTERVAL", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.MaxBatchSize, err = intValue("MAX_BATCH_SIZE", 500); err != nil {
		return Config{}, err
	}
	if cfg.MaxQueueSize, err = intValue("MAX_QUEUE_SIZE", 10000); err != nil {
		return Config{}, err
	}
	if cfg.MaxPayloadBytes, err = int64Value("MAX_PAYLOAD_BYTES", 1048576); err != nil {
		return Config{}, err
	}
	if cfg.MaxEventsPerRequest, err = intValue("MAX_EVENTS_PER_REQUEST", 100); err != nil {
		return Config{}, err
	}
	if cfg.MaxFieldLength, err = intValue("MAX_FIELD_LENGTH", 2048); err != nil {
		return Config{}, err
	}
	if cfg.MaxPayloadEntries, err = intValue("MAX_PAYLOAD_ENTRIES", 128); err != nil {
		return Config{}, err
	}
	if cfg.MaxPayloadDepth, err = intValue("MAX_PAYLOAD_DEPTH", 4); err != nil {
		return Config{}, err
	}
	if cfg.MaxRetries, err = intValue("MAX_RETRIES", 10); err != nil {
		return Config{}, err
	}
	if cfg.RetryMinBackoff, err = duration("RETRY_MIN_BACKOFF", time.Second); err != nil {
		return Config{}, err
	}
	if cfg.RetryMaxBackoff, err = duration("RETRY_MAX_BACKOFF", 60*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.RetentionDays, err = intValue("RETENTION_DAYS", 90); err != nil {
		return Config{}, err
	}
	if cfg.TrustProxyHeaders, err = boolValue("TRUST_PROXY_HEADERS", true); err != nil {
		return Config{}, err
	}
	if cfg.CaptureClientIP, err = boolValue("CAPTURE_CLIENT_IP", false); err != nil {
		return Config{}, err
	}
	if cfg.StoreIPMetadata, err = boolValue("STORE_IP_METADATA", false); err != nil {
		return Config{}, err
	}
	if cfg.SanitizeURLs, err = boolValue("SANITIZE_URLS", true); err != nil {
		return Config{}, err
	}
	if cfg.RequireOrigin, err = boolValue("REQUIRE_ORIGIN", true); err != nil {
		return Config{}, err
	}
	if cfg.RequireURLHostMatch, err = boolValue("REQUIRE_URL_HOST_MATCH", true); err != nil {
		return Config{}, err
	}
	if cfg.RequireElasticReady, err = boolValue("REQUIRE_ELASTIC_READY", false); err != nil {
		return Config{}, err
	}
	if cfg.MetricsEnabled, err = boolValue("METRICS_ENABLED", false); err != nil {
		return Config{}, err
	}
	if cfg.ReadTimeout, err = duration("READ_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.WriteTimeout, err = duration("WRITE_TIMEOUT", 15*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.IdleTimeout, err = duration("IDLE_TIMEOUT", 60*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitPerMinute, err = intValue("RATE_LIMIT_PER_MINUTE", 300); err != nil {
		return Config{}, err
	}
	cfg.BlockedUserAgents = parseList(envOrDefault("BLOCKED_USER_AGENTS", "bot,crawler,spider,curl,wget,python-requests,go-http-client"))
	cfg.SuspectUserAgents = parseList(envOrDefault("SUSPECT_USER_AGENTS", "headless,playwright,puppeteer,selenium,phantomjs"))

	cfg.DropPolicy = DropPolicy(envOrDefault("DROP_POLICY", string(DropPolicyReject)))
	if cfg.DropPolicy != DropPolicyReject && cfg.DropPolicy != DropPolicyDropNewest {
		return Config{}, fmt.Errorf("invalid DROP_POLICY %q", cfg.DropPolicy)
	}

	if cfg.MaxBatchSize <= 0 || cfg.MaxQueueSize <= 0 || cfg.MaxPayloadBytes <= 0 || cfg.MaxEventsPerRequest <= 0 || cfg.MaxFieldLength <= 0 || cfg.MaxPayloadEntries <= 0 || cfg.MaxPayloadDepth <= 0 || cfg.MaxRetries < 0 || cfg.RateLimitPerMinute < 0 {
		return Config{}, errors.New("numeric config values must be positive")
	}

	return cfg, nil
}

func parseDomains(raw string) ([]string, map[string]struct{}, error) {
	set := map[string]struct{}{}
	var domains []string
	addDomain := func(part string) error {
		domain := normalizeDomain(part)
		if domain == "" {
			return nil
		}
		if strings.Contains(domain, "/") {
			return fmt.Errorf("invalid domain %q", part)
		}
		if _, exists := set[domain]; exists {
			return nil
		}
		set[domain] = struct{}{}
		domains = append(domains, domain)
		return nil
	}
	for _, part := range strings.Split(raw, ",") {
		if err := addDomain(part); err != nil {
			return nil, nil, err
		}
	}
	return domains, set, nil
}

func NormalizeDomain(v string) string {
	return normalizeDomain(v)
}

func parseSiteOrigins(raw string) (map[string]map[string]struct{}, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]map[string]struct{}{}, nil
	}
	result := make(map[string]map[string]struct{})
	for _, mapping := range strings.Split(raw, ";") {
		mapping = strings.TrimSpace(mapping)
		if mapping == "" {
			continue
		}
		parts := strings.SplitN(mapping, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid site origin mapping %q", mapping)
		}
		siteID := strings.TrimSpace(parts[0])
		if siteID == "" {
			return nil, fmt.Errorf("invalid site origin mapping %q", mapping)
		}
		origins, _, err := parseDomains(strings.ReplaceAll(parts[1], "|", ","))
		if err != nil {
			return nil, fmt.Errorf("invalid site origins for %q: %w", siteID, err)
		}
		if len(origins) == 0 {
			return nil, fmt.Errorf("site %q has no allowed origins", siteID)
		}
		allowed := make(map[string]struct{}, len(origins))
		for _, origin := range origins {
			allowed[origin] = struct{}{}
		}
		result[siteID] = allowed
	}
	return result, nil
}

func parseList(raw string) []string {
	var items []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		items = append(items, part)
	}
	return items
}

func normalizeDomain(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimPrefix(v, "http://")
	v = strings.TrimPrefix(v, "https://")
	if idx := strings.Index(v, "/"); idx >= 0 {
		v = v[:idx]
	}
	if idx := strings.Index(v, ":"); idx >= 0 {
		v = v[:idx]
	}
	return strings.TrimSpace(v)
}

func envOrDefault(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}

func duration(key string, defaultValue time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return value, nil
}

func intValue(key string, defaultValue int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return value, nil
}

func int64Value(key string, defaultValue int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return value, nil
}

func boolValue(key string, defaultValue bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return value, nil
}
