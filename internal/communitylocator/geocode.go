// Package communitylocator — geocode.go provides forward geocoding
// (address → lat/lng) via the Nominatim API. This follows the same pattern
// as internal/users/geocode.go (reverse geocoding) — configurable endpoint
// URL and timeout via system_settings, Nominatim User-Agent policy, and
// response size limits for safety.
//
// Per §101/ADR-109: "Geocoding via Nominatim on submission (free, no API
// key). Coordinates stored as NUMERIC(9,6) lat/lng."
//
// Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
//
//	japaconnect-engineering/adr/ADR-109-community-locator.md
package communitylocator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/config"
)

// nominatimForwardMaxResponseSize caps the forward-geocode response body.
const nominatimForwardMaxResponseSize = 1 << 20 // 1 MiB

// nominatimForwardDefaultURL is the fallback forward-geocoding endpoint.
const nominatimForwardDefaultURL = "https://nominatim.openstreetmap.org/search"

// nominatimForwardDefaultTimeoutMs is the fallback request timeout.
const nominatimForwardDefaultTimeoutMs = 5000

// forwardGeocodeResult holds the lat/lng pair returned by forward geocoding.
type forwardGeocodeResult struct {
	Latitude  float64
	Longitude float64
}

// geoSettingsReader is the subset of config.Settings the geocoder needs.
// This mirrors the interface in internal/users/geocode.go.
type geoSettingsReader interface {
	GetStringOr(ctx context.Context, key, fallback string) string
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// forwardGeocode resolves a free-text address to latitude/longitude
// coordinates via the Nominatim forward-geocoding (search) API. Returns
// the first result. If no results are found, returns an error — the
// caller should proceed without coordinates (the submission is still
// valid without a pin on the map).
//
// The endpoint URL and timeout are configurable via system_settings
// (geo_personalization.nominatim_url, geo_personalization.nominatim_timeout_ms)
// per Rule 3 — no hardcoded URLs or limits.
func forwardGeocode(ctx context.Context, settings geoSettingsReader, address string) (forwardGeocodeResult, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return forwardGeocodeResult{}, errors.New("empty address")
	}

	url := nominatimForwardDefaultURL
	timeoutMs := nominatimForwardDefaultTimeoutMs

	if settings != nil {
		// Reuse the same settings keys as the reverse geocoder — the
		// Nominatim base URL is the same host, just a different path.
		url = settings.GetStringOr(ctx, "geo_personalization.nominatim_url", nominatimForwardDefaultURL)
		timeoutMs = settings.GetIntOr(ctx, "geo_personalization.nominatim_timeout_ms", nominatimForwardDefaultTimeoutMs)
		// The nominatim_url setting points at /reverse for the users
		// package. Replace the path with /search for forward geocoding.
		url = strings.Replace(url, "/reverse", "/search", 1)
		if !strings.HasSuffix(url, "/search") && !strings.Contains(url, "/search") {
			// If the URL doesn't end with /reverse or /search, append /search.
			url = strings.TrimRight(url, "/") + "/search"
		}
	}

	fullURL := fmt.Sprintf("%s?format=json&q=%s&limit=1&addressdetails=0", url, encodeURIComponent(address))

	// #nosec G704 // pending allowlist validation
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return forwardGeocodeResult{}, fmt.Errorf("building nominatim forward request: %w", err)
	}
	// Nominatim usage policy requires a descriptive User-Agent.
	req.Header.Set("User-Agent", "JapaConnect/1.0 (community locator)")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}

	// #nosec G704 // pending allowlist validation
	resp, err := client.Do(req)
	if err != nil {
		return forwardGeocodeResult{}, fmt.Errorf("nominatim forward request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return forwardGeocodeResult{}, fmt.Errorf("nominatim forward returned %d: %s", resp.StatusCode, string(body))
	}

	var results []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, nominatimForwardMaxResponseSize)).Decode(&results); err != nil {
		return forwardGeocodeResult{}, fmt.Errorf("decoding nominatim forward response: %w", err)
	}

	if len(results) == 0 {
		return forwardGeocodeResult{}, errors.New("no geocoding results for address")
	}

	var lat, lng float64
	if _, err := fmt.Sscanf(results[0].Lat, "%f", &lat); err != nil {
		return forwardGeocodeResult{}, fmt.Errorf("parsing latitude: %w", err)
	}

	if _, err := fmt.Sscanf(results[0].Lon, "%f", &lng); err != nil {
		return forwardGeocodeResult{}, fmt.Errorf("parsing longitude: %w", err)
	}

	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return forwardGeocodeResult{}, fmt.Errorf("geocoded coordinates out of range: lat=%f lng=%f", lat, lng)
	}

	return forwardGeocodeResult{Latitude: lat, Longitude: lng}, nil
}

// encodeURIComponent encodes a string for use in a URL query parameter.
// This is a minimal implementation that percent-encodes characters not
// safe in a query value, matching JavaScript's encodeURIComponent.
func encodeURIComponent(s string) string {
	const hex = "0123456789ABCDEF"

	var b strings.Builder

	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '!' || r == '~' || r == '*' || r == '\'' || r == '(' || r == ')' {
			b.WriteRune(r)
		} else if r <= 0x7F {
			b.WriteByte('%')
			b.WriteByte(hex[r>>4])
			b.WriteByte(hex[r&0xF])
		} else {
			// Multi-byte UTF-8 — encode each byte.
			for _, byt := range []byte(string(r)) {
				b.WriteByte('%')
				b.WriteByte(hex[byt>>4])
				b.WriteByte(hex[byt&0xF])
			}
		}
	}

	return b.String()
}

// Ensure config.Settings satisfies geoSettingsReader at compile time.
var _ geoSettingsReader = (*config.Settings)(nil)
