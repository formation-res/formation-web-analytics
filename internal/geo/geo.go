package geo

import (
	"fmt"
	"math"
	"net/netip"
	"os"
	"sync"

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
	ReloadIfChanged() (bool, error)
	Close() error
}

type maxMindResolver struct {
	mu       sync.RWMutex
	reader   *maxminddb.Reader
	path     string
	fileInfo os.FileInfo
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
	info, err := os.Stat(path)
	if err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("stat geoip db: %w", err)
	}
	return &maxMindResolver{reader: reader, path: path, fileInfo: info}, nil
}

func (r *maxMindResolver) Lookup(rawIP string) (Result, bool) {
	ip, err := netip.ParseAddr(rawIP)
	if err != nil {
		return Result{}, false
	}
	var record cityRecord
	r.mu.RLock()
	defer r.mu.RUnlock()
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

func (r *maxMindResolver) ReloadIfChanged() (bool, error) {
	info, err := os.Stat(r.path)
	if err != nil {
		return false, fmt.Errorf("stat geoip db: %w", err)
	}

	r.mu.RLock()
	unchanged := os.SameFile(info, r.fileInfo) && info.ModTime().Equal(r.fileInfo.ModTime()) && info.Size() == r.fileInfo.Size()
	r.mu.RUnlock()
	if unchanged {
		return false, nil
	}

	reader, err := maxminddb.Open(r.path)
	if err != nil {
		return false, fmt.Errorf("reload geoip db: %w", err)
	}

	r.mu.Lock()
	oldReader := r.reader
	r.reader = reader
	r.fileInfo = info
	r.mu.Unlock()
	if err := oldReader.Close(); err != nil {
		return true, fmt.Errorf("close previous geoip db: %w", err)
	}
	return true, nil
}

func (r *maxMindResolver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
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
