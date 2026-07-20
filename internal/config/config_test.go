package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadParsesDomainsAndDefaults(t *testing.T) {
	t.Setenv("ALLOWED_DOMAINS", "Example.com, www.example.com")
	t.Setenv("ELASTICSEARCH_URL", "http://localhost:9200")
	t.Setenv("ELASTICSEARCH_API_KEY", "test")
	t.Setenv("GEOIP_DB_PATH", "/tmp/GeoLite2-City.mmdb")

	cfg, err := Load("test")
	if err != nil {
		t.Fatalf("expected config to load: %v", err)
	}
	if len(cfg.AllowedDomains) != 2 {
		t.Fatalf("expected 2 configured domains, got %d", len(cfg.AllowedDomains))
	}
	if _, ok := cfg.AllowedDomainSet["example.com"]; !ok {
		t.Fatal("expected normalized example.com domain")
	}
	if _, ok := cfg.AllowedDomainSet["www.example.com"]; !ok {
		t.Fatal("expected normalized www.example.com domain")
	}
	if cfg.FlushInterval != 5*time.Second {
		t.Fatalf("expected default flush interval")
	}
	if cfg.MaxEventsPerRequest != 100 {
		t.Fatalf("expected default max events per request")
	}
	if cfg.MetricsEnabled {
		t.Fatal("expected metrics to be disabled by default")
	}
	if cfg.MetricsListenAddr != ":9090" {
		t.Fatalf("expected default metrics listen addr, got %s", cfg.MetricsListenAddr)
	}
	if cfg.ReadTimeout != 10*time.Second || cfg.WriteTimeout != 15*time.Second || cfg.IdleTimeout != 60*time.Second {
		t.Fatal("expected default HTTP timeouts to be loaded")
	}
	if !cfg.RequireOrigin || !cfg.RequireURLHostMatch {
		t.Fatal("expected origin and URL host checks to be enabled by default")
	}
	if cfg.StoreIPMetadata {
		t.Fatal("expected IP metadata storage to be disabled by default")
	}
	if !cfg.SanitizeURLs {
		t.Fatal("expected URL sanitization to be enabled by default")
	}
	if cfg.RateLimitPerMinute != 300 {
		t.Fatalf("expected default rate limit, got %d", cfg.RateLimitPerMinute)
	}
	if cfg.RateLimitMaxClients != 100000 {
		t.Fatalf("expected default rate-limit client cap, got %d", cfg.RateLimitMaxClients)
	}
	if len(cfg.BlockedUserAgents) == 0 {
		t.Fatal("expected default flagged user agents")
	}
	if len(cfg.SuspectUserAgents) == 0 {
		t.Fatal("expected default suspect user agents")
	}
}

func TestLoadRejectsUnsafeDurations(t *testing.T) {
	tests := map[string]string{
		"zero flush interval":     "FLUSH_INTERVAL=0s",
		"zero retry minimum":      "RETRY_MIN_BACKOFF=0s",
		"zero retry maximum":      "RETRY_MAX_BACKOFF=0s",
		"zero read timeout":       "READ_TIMEOUT=0s",
		"zero write timeout":      "WRITE_TIMEOUT=0s",
		"zero idle timeout":       "IDLE_TIMEOUT=0s",
		"reversed retry backoffs": "RETRY_MIN_BACKOFF=2s,RETRY_MAX_BACKOFF=1s",
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ALLOWED_DOMAINS", "example.com")
			t.Setenv("ELASTICSEARCH_URL", "http://localhost:9200")
			t.Setenv("ELASTICSEARCH_API_KEY", "test")
			t.Setenv("GEOIP_DB_PATH", "/tmp/GeoLite2-City.mmdb")
			for _, assignment := range strings.Split(values, ",") {
				key, value, ok := strings.Cut(assignment, "=")
				if !ok {
					t.Fatalf("invalid test assignment %q", assignment)
				}
				t.Setenv(key, value)
			}
			if _, err := Load("test"); err == nil {
				t.Fatal("expected unsafe duration to be rejected")
			}
		})
	}
}

func TestLoadRejectsInvalidDropPolicy(t *testing.T) {
	t.Setenv("ALLOWED_DOMAINS", "example.com")
	t.Setenv("ELASTICSEARCH_URL", "http://localhost:9200")
	t.Setenv("ELASTICSEARCH_API_KEY", "test")
	t.Setenv("GEOIP_DB_PATH", "/tmp/GeoLite2-City.mmdb")
	t.Setenv("DROP_POLICY", "invalid")

	if _, err := Load("test"); err == nil {
		t.Fatal("expected invalid drop policy error")
	}
}

func TestNormalizeDomain(t *testing.T) {
	if got := NormalizeDomain("HTTPS://Example.com:443/path"); got != "example.com" {
		t.Fatalf("unexpected normalized domain: %s", got)
	}
}

func TestLoadParsesSiteOriginMap(t *testing.T) {
	t.Setenv("ALLOWED_DOMAINS", "example.com")
	t.Setenv("ELASTICSEARCH_URL", "http://localhost:9200")
	t.Setenv("ELASTICSEARCH_API_KEY", "test")
	t.Setenv("GEOIP_DB_PATH", "/tmp/GeoLite2-City.mmdb")
	t.Setenv("SITE_ORIGIN_MAP", "marketing:example.com|www.example.com;docs:docs.example.com")

	cfg, err := Load("test")
	if err != nil {
		t.Fatalf("expected config to load: %v", err)
	}
	if _, ok := cfg.SiteOriginSet["marketing"]["example.com"]; !ok {
		t.Fatal("expected marketing origin example.com")
	}
	if _, ok := cfg.SiteOriginSet["marketing"]["www.example.com"]; !ok {
		t.Fatal("expected marketing origin www.example.com")
	}
	if _, ok := cfg.SiteOriginSet["docs"]["docs.example.com"]; !ok {
		t.Fatal("expected docs origin docs.example.com")
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
