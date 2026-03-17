package config

import (
	"os"
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

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
