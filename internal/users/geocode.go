package users

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
)

// nominatimMaxResponseSize caps the reverse-geocode response body to prevent
// a malicious or misbehaving Nominatim endpoint from exhausting server memory.
const nominatimMaxResponseSize = 1 << 20 // 1 MiB

// nominatimDefaultURL is the fallback reverse-geocoding endpoint when the
// system_settings value is not set (Rule 3: configurable, not hardcoded).
const nominatimDefaultURL = "https://nominatim.openstreetmap.org/reverse"

// nominatimDefaultTimeoutMs is the fallback request timeout in milliseconds
// when the system_settings value is not set (Rule 3: configurable).
const nominatimDefaultTimeoutMs = 5000

// geoSettingsReader is the subset of config.Settings the geocoder needs.
type geoSettingsReader interface {
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// reverseGeocode calls the Nominatim reverse-geocoding API to resolve
// latitude/longitude coordinates to a city name and ISO country code.
// Returns empty strings and an error when the API is unreachable or
// returns no results. The caller is responsible for matching the city
// name to an existing seeded Space (ADR-064a).
//
// The endpoint URL and timeout are configurable via system_settings
// (geo_personalization.nominatim_url, geo_personalization.nominatim_timeout_ms)
// per Rule 3 — no hardcoded URLs or limits.
//
// Ref: japaconnect-engineering/adr/ADR-064a-geo-awareness-extension.md.
func reverseGeocode(ctx context.Context, settings geoSettingsReader, tenantID pgtype.UUID, lat, lng float64) (city, state, countryCode string, err error) {
	// Validate coordinate ranges before making any external call.
	if lat < -90 || lat > 90 {
		return "", "", "", fmt.Errorf("latitude %.6f out of range [-90, 90]", lat)
	}

	if lng < -180 || lng > 180 {
		return "", "", "", fmt.Errorf("longitude %.6f out of range [-180, 180]", lng)
	}

	url := nominatimDefaultURL
	timeoutMs := nominatimDefaultTimeoutMs

	if settings != nil {
		url = settings.GetStringOr(ctx, pgtype.UUID{}, "geo_personalization.nominatim_url", nominatimDefaultURL)
		timeoutMs = settings.GetIntOr(ctx, pgtype.UUID{}, "geo_personalization.nominatim_timeout_ms", nominatimDefaultTimeoutMs)
	}

	fullURL := fmt.Sprintf("%s?format=json&lat=%f&lon=%f&zoom=10&addressdetails=1", url, lat, lng)

	// #nosec G704 // pending allowlist validation
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("building nominatim request: %w", err)
	}
	// Nominatim usage policy requires a descriptive User-Agent.
	req.Header.Set("User-Agent", "JapaConnect/1.0 (geo-aware personalization)")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}

	// #nosec G704 // pending allowlist validation
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("nominatim request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Limit the error body read to prevent log flooding from large responses.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", "", fmt.Errorf("nominatim returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Address struct {
			City        string `json:"city"`
			Town        string `json:"town"`
			Village     string `json:"village"`
			State       string `json:"state"`
			County      string `json:"county"`
			Country     string `json:"country"`
			CountryCode string `json:"country_code"`
		} `json:"address"`
	}
	// Limit the response body to prevent memory exhaustion from a
	// misbehaving or malicious endpoint.
	if err := json.NewDecoder(io.LimitReader(resp.Body, nominatimMaxResponseSize)).Decode(&result); err != nil {
		return "", "", "", fmt.Errorf("decoding nominatim response: %w", err)
	}

	// Nominatim uses city/town/village depending on population — take the first non-empty.
	city = result.Address.City
	if city == "" {
		city = result.Address.Town
	}

	if city == "" {
		city = result.Address.Village
	}
	// Nominatim uses state or county depending on the region — take the first non-empty.
	state = result.Address.State
	if state == "" {
		state = result.Address.County
	}

	countryCode = strings.ToUpper(result.Address.CountryCode)

	return city, state, countryCode, nil
}

// Ensure config.Settings satisfies geoSettingsReader at compile time.
var _ geoSettingsReader = (*config.Settings)(nil)
