package geo

import (
	"fmt"
	"math"
	"net/netip"

	"github.com/oschwald/maxminddb-golang/v2"
)

type Point struct {
	Latitude  float64
	Longitude float64
}

type Result struct {
	CountryISOCode string
	CountryName    string
	CityName       string
	Point          *Point
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
	Location struct {
		Latitude  *float64 `maxminddb:"latitude"`
		Longitude *float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
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
	if point, ok := validPoint(record.Location.Latitude, record.Location.Longitude); ok {
		result.Point = point
	}
	if result.CountryISOCode == "" && result.CountryName == "" && result.CityName == "" {
		return Result{}, false
	}
	return result, true
}

func (r *maxMindResolver) Close() error {
	return r.reader.Close()
}

func validPoint(lat, lon *float64) (*Point, bool) {
	if lat == nil || lon == nil {
		return nil, false
	}
	if math.IsNaN(*lat) || math.IsInf(*lat, 0) || math.IsNaN(*lon) || math.IsInf(*lon, 0) {
		return nil, false
	}
	if *lat < -90 || *lat > 90 || *lon < -180 || *lon > 180 {
		return nil, false
	}
	return &Point{Latitude: *lat, Longitude: *lon}, true
}
