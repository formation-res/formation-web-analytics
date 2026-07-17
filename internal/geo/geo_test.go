package geo

import (
	"math"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func TestResolverLookup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.mmdb")
	writeTestDB(t, path, "EX", "Exampleland")

	resolver, err := New(path)
	if err != nil {
		t.Fatalf("failed to open resolver: %v", err)
	}
	defer resolver.Close()

	result, ok := resolver.Lookup("1.2.3.4")
	if !ok {
		t.Fatal("expected lookup to succeed")
	}
	if result.CountryISOCode != "EX" {
		t.Fatalf("unexpected country code: %#v", result)
	}
	if result.Point == nil || result.Point.Latitude != 12.34 || result.Point.Longitude != 56.78 {
		t.Fatalf("unexpected geo point: %#v", result.Point)
	}
}

func TestResolverReloadsReplacedDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.mmdb")
	writeTestDB(t, path, "EX", "Exampleland")
	resolver, err := New(path)
	if err != nil {
		t.Fatalf("failed to open resolver: %v", err)
	}
	defer resolver.Close()

	replacement := filepath.Join(dir, "replacement.mmdb")
	writeTestDB(t, replacement, "NW", "Newland")
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(replacement, future, future); err != nil {
		t.Fatalf("failed to adjust replacement timestamp: %v", err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatalf("failed to replace database: %v", err)
	}
	reloaded, err := resolver.ReloadIfChanged()
	if err != nil {
		t.Fatalf("failed to reload database: %v", err)
	}
	if !reloaded {
		t.Fatal("expected database replacement to be detected")
	}
	result, ok := resolver.Lookup("1.2.3.4")
	if !ok || result.CountryISOCode != "NW" {
		t.Fatalf("expected lookup from replacement database, got %#v", result)
	}
}

func writeTestDB(t *testing.T, path, countryCode, countryName string) {
	t.Helper()
	writer, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            "Formation-Analytics-Test-GeoIP",
		Languages:               []string{"en"},
		IncludeReservedNetworks: true,
	})
	if err != nil {
		t.Fatalf("failed to create mmdb writer: %v", err)
	}
	_, network, err := net.ParseCIDR("1.2.3.0/24")
	if err != nil {
		t.Fatalf("failed to parse cidr: %v", err)
	}
	record := mmdbtype.Map{
		"country": mmdbtype.Map{
			"iso_code": mmdbtype.String(countryCode),
			"names": mmdbtype.Map{
				"en": mmdbtype.String(countryName),
			},
		},
		"city": mmdbtype.Map{
			"names": mmdbtype.Map{
				"en": mmdbtype.String("Example City"),
			},
		},
		"location": mmdbtype.Map{
			"latitude":  mmdbtype.Float64(12.34),
			"longitude": mmdbtype.Float64(56.78),
		},
	}
	if err := writer.Insert(network, record); err != nil {
		t.Fatalf("failed to insert record: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create db file: %v", err)
	}
	if _, err := writer.WriteTo(file); err != nil {
		_ = file.Close()
		t.Fatalf("failed to write db: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed to close db: %v", err)
	}
}

func TestValidPointRejectsMalformedCoordinates(t *testing.T) {
	lat := math.NaN()
	lon := 12.0
	if point, ok := validPoint(&lat, &lon); ok || point != nil {
		t.Fatalf("expected NaN latitude to be rejected")
	}

	lat = 91
	lon = 12
	if point, ok := validPoint(&lat, &lon); ok || point != nil {
		t.Fatalf("expected out-of-range latitude to be rejected")
	}

	lat = 52.3
	if point, ok := validPoint(&lat, nil); ok || point != nil {
		t.Fatalf("expected missing longitude to be rejected")
	}
}
