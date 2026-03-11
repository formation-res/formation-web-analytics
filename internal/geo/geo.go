package geo

import (
	"fmt"
	"net/netip"

	"github.com/oschwald/maxminddb-golang/v2"
)

type Result struct {
	CountryISOCode string
	CountryName    string
	CityName       string
}

type Resolver interface {
	Lookup(ip string) (Result, bool)
	Close() error
}

type maxMindResolver struct {
	reader *maxminddb.Reader
}

type cityRecord struct {
	Country struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
}

func New(path string) (Resolver, error) {
	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open geoip db: %w", err)
	}
	return &maxMindResolver{reader: reader}, nil
}

func (r *maxMindResolver) Lookup(rawIP string) (Result, bool) {
	ip, err := netip.ParseAddr(rawIP)
	if err != nil {
		return Result{}, false
	}
	var record cityRecord
	if err := r.reader.Lookup(ip).Decode(&record); err != nil {
		return Result{}, false
	}
	result := Result{
		CountryISOCode: record.Country.ISOCode,
		CountryName:    record.Country.Names["en"],
		CityName:       record.City.Names["en"],
	}
	if result.CountryISOCode == "" && result.CountryName == "" && result.CityName == "" {
		return Result{}, false
	}
	return result, true
}

func (r *maxMindResolver) Close() error {
	return r.reader.Close()
}
