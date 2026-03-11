package geo

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func TestResolverLookup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.mmdb")
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
			"iso_code": mmdbtype.String("EX"),
			"names": mmdbtype.Map{
				"en": mmdbtype.String("Exampleland"),
			},
		},
		"city": mmdbtype.Map{
			"names": mmdbtype.Map{
				"en": mmdbtype.String("Example City"),
			},
		},
	}
	if err := writer.Insert(network, record); err != nil {
		t.Fatalf("failed to insert record: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create db file: %v", err)
	}
	defer file.Close()
	if _, err := writer.WriteTo(file); err != nil {
		t.Fatalf("failed to write db: %v", err)
	}

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
}
