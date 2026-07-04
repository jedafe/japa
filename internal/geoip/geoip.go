// Package geoip provides IP geolocation for JapaConnect using the
// self-hosted MaxMind GeoLite2-City database.
//
// The geoip2-golang library reads the binary .mmdb file directly in-process
// (no network call, sub-millisecond lookup). The GeoIPMiddleware runs on
// every request and adds GeoContext to the request context so downstream
// handlers can access country, city, timezone, and coordinates.
//
// The database is updated weekly via an Asynq task. Updates use an atomic
// pointer swap so the server never needs to restart.
package geoip

import (
	"context"
	"net"
	"sync"
	"sync/atomic"

	"github.com/oschwald/geoip2-golang"
)

// contextKey is an unexported type for context key safety.
type contextKey struct{ name string }

// GeoContextKey is the context key for the resolved geo context.
var GeoContextKey = contextKey{"geo"}

// GeoContext holds the geolocation data extracted from an IP address.
type GeoContext struct {
	CountryCode string
	CountryName string
	City        string
	Subdivision string
	TimeZone    string
	Latitude    float64
	Longitude   float64
	IsValid     bool
}

// FromContext extracts the GeoContext from the request context.
// Returns a zero-value GeoContext (IsValid=false) if no geo data is set.
func FromContext(ctx context.Context) GeoContext {
	g, ok := ctx.Value(GeoContextKey).(GeoContext)
	if !ok {
		return GeoContext{IsValid: false}
	}

	return g
}

// Reader holds a pointer to the current geoip2.Reader. The pointer is
// stored atomically so database updates can swap it without locking.
type Reader struct {
	current atomic.Pointer[geoip2.Reader]
	mu      sync.Mutex
}

// NewReader opens a GeoLite2 database file and returns a Reader.
// If the file does not exist, returns a Reader with nil internal pointer
// (lookups will return empty GeoContext - the server still works, just
// without geo data).
func NewReader(dbPath string) (*Reader, error) {
	r := &Reader{}

	reader, err := geoip2.Open(dbPath)
	if err != nil {
		// Return a working Reader with nil internal - the server
		// should not crash if the geoip database is missing
		return r, nil //nolint:nilerr // intentional: degradable geo lookup
	}

	r.current.Store(reader)

	return r, nil
}

// Lookup resolves an IP address to a GeoContext.
// If no database is loaded, returns an invalid GeoContext.
func (r *Reader) Lookup(ipStr string) GeoContext {
	reader := r.current.Load()
	if reader == nil {
		return GeoContext{IsValid: false}
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return GeoContext{IsValid: false}
	}

	record, err := reader.City(ip)
	if err != nil {
		return GeoContext{IsValid: false}
	}

	gc := GeoContext{IsValid: true}
	if record.Country.IsoCode != "" {
		gc.CountryCode = record.Country.IsoCode
	}

	if name, ok := record.Country.Names["en"]; ok {
		gc.CountryName = name
	}

	if name, ok := record.City.Names["en"]; ok {
		gc.City = name
	}

	if len(record.Subdivisions) > 0 {
		if name, ok := record.Subdivisions[0].Names["en"]; ok {
			gc.Subdivision = name
		}
	}

	if record.Location.TimeZone != "" {
		gc.TimeZone = record.Location.TimeZone
	}

	if record.Location.Latitude != 0 || record.Location.Longitude != 0 {
		gc.Latitude = record.Location.Latitude
		gc.Longitude = record.Location.Longitude
	}

	return gc
}

// Swap atomically replaces the database reader. Call this after downloading
// a new .mmdb file. The old reader is closed and the new one takes effect
// immediately for all subsequent lookups - no server restart needed.
func (r *Reader) Swap(newReader *geoip2.Reader) {
	r.mu.Lock()
	defer r.mu.Unlock()

	old := r.current.Swap(newReader)
	if old != nil {
		_ = old.Close()
	}
}

// Close releases the database reader resources.
func (r *Reader) Close() {
	reader := r.current.Load()
	if reader != nil {
		_ = reader.Close()
	}
}

// IsLoaded returns true if a geoip database is currently loaded.
func (r *Reader) IsLoaded() bool {
	return r.current.Load() != nil
}
