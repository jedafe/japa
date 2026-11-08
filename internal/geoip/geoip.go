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
	"sync/atomic"

	"github.com/oschwald/geoip2-golang"
)

// contextKey is an unexported type for context key safety.
type contextKey struct{ name string }

// GeoContextKey is the context key for the resolved geo context.
var GeoContextKey = contextKey{"geo"}

// GeoContext holds the geolocation data extracted from an IP address.
//
// JSON tags align to the Section 37 response shape (snake_case, "state" for
// the subdivision). IsValid is an internal sentinel and is never serialized —
// handlers project it into the standard error envelope instead.
type GeoContext struct {
	CountryCode string  `json:"country_code"`
	CountryName string  `json:"country_name"`
	City        string  `json:"city"`
	Subdivision string  `json:"state"`
	TimeZone    string  `json:"time_zone"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	// IsVPN is true when the anonymous-IP database flags the address
	// (anonymous VPN, Tor exit node, public/residential proxy, hosting
	// provider). Populated only when a second .mmdb is loaded via
	// LoadAnonymousDB; emitted as is_vpn by handlers only when the
	// geoip.anonymous_ip feature flag is on (Section 37).
	IsVPN   bool `json:"is_vpn"`
	IsValid bool `json:"-"`
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
// anon optionally holds a GeoLite2-Anonymous-IP reader (loaded via
// LoadAnonymousDB) — the same atomic-swap pattern so its weekly update
// needs no restart either.
type Reader struct {
	current atomic.Pointer[geoip2.Reader]
	anon    atomic.Pointer[geoip2.Reader]
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

// LoadAnonymousDB opens a GeoLite2-Anonymous-IP database file and installs
// it as the anonymous-IP reader. Unlike NewReader it returns the open error
// so the caller can log a misconfigured path — the Reader stays usable for
// City lookups either way.
func (r *Reader) LoadAnonymousDB(path string) error {
	reader, err := geoip2.Open(path)
	if err != nil {
		return err
	}

	r.SwapAnonymous(reader)

	return nil
}

// SwapAnonymous atomically replaces the anonymous-IP reader, closing the
// previous one. Mirrors Swap — used by the update task after downloading a
// fresh anonymous-IP database.
func (r *Reader) SwapAnonymous(newReader *geoip2.Reader) {
	old := r.anon.Swap(newReader)
	if old != nil {
		_ = old.Close()
	}
}

// AnonymousLoaded returns true when an anonymous-IP database is installed.
func (r *Reader) AnonymousLoaded() bool {
	return r.anon.Load() != nil
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

	// Per Section 37, IsValid is true only when the lookup produced a usable
	// country. Reserved IP ranges and lookup misses return a record with no
	// country; those must surface as IsValid=false so handlers return
	// 404 geo_not_available instead of a 200 with an all-empty payload.
	gc := GeoContext{}
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

	gc.IsValid = gc.CountryCode != ""

	// Anonymous-IP overlay (Section 37 is_vpn): a lookup miss or absent
	// record leaves IsVPN false — the City record stays authoritative for
	// IsValid either way.
	if anon := r.anon.Load(); anon != nil {
		if rec, aerr := anon.AnonymousIP(ip); aerr == nil && rec != nil {
			gc.IsVPN = rec.IsAnonymous || rec.IsAnonymousVPN || rec.IsTorExitNode ||
				rec.IsPublicProxy || rec.IsResidentialProxy || rec.IsHostingProvider
		}
	}

	return gc
}

// Swap atomically replaces the database reader. Call this after downloading
// a new .mmdb file. The old reader is closed and the new one takes effect
// immediately for all subsequent lookups - no server restart needed.
func (r *Reader) Swap(newReader *geoip2.Reader) {
	old := r.current.Swap(newReader)
	if old != nil {
		_ = old.Close()
	}
}

// Close releases the database reader resources. It atomically swaps the
// pointer to nil before closing the taken reader, so concurrent Lookup
// and IsLoaded calls see an unloaded state immediately (not a closed
// reader that would produce errors). This also prevents a double-close
// race with Swap: only one of Close or Swap can win the Swap(nil) /
// Swap(newReader) and take ownership of the old reader.
func (r *Reader) Close() {
	old := r.current.Swap(nil)
	if old != nil {
		_ = old.Close()
	}

	r.SwapAnonymous(nil)
}

// IsLoaded returns true if a geoip database is currently loaded.
func (r *Reader) IsLoaded() bool {
	return r.current.Load() != nil
}
