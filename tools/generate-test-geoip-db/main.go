package main

import (
	"log"
	"net"
	"os"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("usage: %s /path/to/output.mmdb", os.Args[0])
	}

	writer, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            "Formation-Analytics-Test-GeoIP",
		Languages:               []string{"en"},
		IncludeReservedNetworks: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	entries := []struct {
		cidr    string
		country string
		code    string
		city    string
	}{
		{cidr: "127.0.0.0/8", country: "Loopbackland", code: "LB", city: "Loopback City"},
		{cidr: "1.2.3.0/24", country: "Exampleland", code: "EX", city: "Example City"},
	}

	for _, entry := range entries {
		_, network, err := net.ParseCIDR(entry.cidr)
		if err != nil {
			log.Fatal(err)
		}
		record := mmdbtype.Map{
			"country": mmdbtype.Map{
				"iso_code": mmdbtype.String(entry.code),
				"names": mmdbtype.Map{
					"en": mmdbtype.String(entry.country),
				},
			},
			"city": mmdbtype.Map{
				"names": mmdbtype.Map{
					"en": mmdbtype.String(entry.city),
				},
			},
		}
		if err := writer.Insert(network, record); err != nil {
			log.Fatal(err)
		}
	}

	output, err := os.Create(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer output.Close()

	if _, err := writer.WriteTo(output); err != nil {
		log.Fatal(err)
	}
}
