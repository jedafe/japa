package geoip

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

// SpacesService is the subset of spaces.Service the geo hierarchy handlers
// need — countries/states/cities are seeded as spaces (catalog: "from seeded
// country spaces"), not a separate table.
type SpacesService interface {
	ListSpacesByLevel(ctx context.Context, tenantID pgtype.UUID, level string) ([]gen.ListSpacesByLevelRow, error)
	ListStatesByCountryCode(ctx context.Context, tenantID pgtype.UUID, countryCode string) ([]gen.ListStatesByCountryGeoCodeRow, error)
	ListCitiesByState(ctx context.Context, tenantID, stateID pgtype.UUID) ([]gen.ListCitiesByStateIDRow, error)
	GetSpaceByGeoCode(ctx context.Context, tenantID pgtype.UUID, countryCode string) (gen.GetSpaceByGeoCodeRow, error)
	FindCitySpaceByName(ctx context.Context, tenantID pgtype.UUID, cityName string) (gen.FindCitySpaceByNameRow, error)
}

// SuggestedSpace is the public projection of a seeded geo Space returned in
// the /geo/me `suggested_spaces` array. It is a stable contract type,
// independent of sqlc-generated row structs, so regenerating internal/db
// never changes the API shape.
type SuggestedSpace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Level       string `json:"level"`
	GeoCode     string `json:"geo_code,omitempty"`
	MemberCount int32  `json:"member_count"`
	PostCount   int32  `json:"post_count"`
}

// Handlers provides HTTP endpoints for geolocation.
type Handlers struct {
	reader *Reader
	spaces SpacesService
}

// NewHandlers creates new geo handlers.
func NewHandlers(reader *Reader, spacesService SpacesService) *Handlers {
	return &Handlers{reader: reader, spaces: spacesService}
}

// HandleGeoMe returns geo data for the current request IP.
// The geoip middleware populates the context on every request,
// so this endpoint simply reads it back and enriches it with
// suggested_spaces (deterministic country/city Space matches) and is_vpn.
//
// GET /api/v1/geo/me
func (h *Handlers) HandleGeoMe(w http.ResponseWriter, r *http.Request) {
	geo := FromContext(r.Context())
	if !geo.IsValid {
		writeError(w, http.StatusNotFound, "geo_not_available", "Geolocation data not available for this request")
		return
	}

	resp := map[string]any{
		"country_code":     geo.CountryCode,
		"country_name":     geo.CountryName,
		"state":            geo.Subdivision,
		"city":             geo.City,
		"time_zone":        geo.TimeZone,
		"latitude":         geo.Latitude,
		"longitude":        geo.Longitude,
		"suggested_spaces": h.suggestedSpaces(r, geo),
		// GeoLite2-City carries no anonymous-IP data; is_vpn is always false
		// until a GeoLite2-Anonymous-IP integration is enabled via the
		// geoip.anonymous_ip_enabled system setting (Section 37).
		"is_vpn": false,
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleGeoLookup resolves an arbitrary IP address (admin only).
// Returns the geo data for the given IP query parameter.
//
// GET /api/v1/geo/lookup?ip=1.2.3.4
func (h *Handlers) HandleGeoLookup(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		writeError(w, http.StatusBadRequest, "missing_ip", "ip query parameter is required")
		return
	}

	if !h.reader.IsLoaded() {
		writeError(w, http.StatusServiceUnavailable, "geoip_not_loaded", "GeoIP database not loaded")
		return
	}

	geo := h.reader.Lookup(ip)
	if !geo.IsValid {
		writeError(w, http.StatusBadRequest, "lookup_failed", "could not resolve IP address")
		return
	}

	resp := map[string]any{
		"country_code": geo.CountryCode,
		"country_name": geo.CountryName,
		"state":        geo.Subdivision,
		"city":         geo.City,
		"time_zone":    geo.TimeZone,
		"latitude":     geo.Latitude,
		"longitude":    geo.Longitude,
		// See HandleGeoMe: GeoLite2-City has no anonymous-IP data.
		"is_vpn": false,
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleGeoDetect resolves the current request IP to a locale (catalog: GET
// /geo/detect → {country, state, city}), a narrower projection of the same
// GeoContext HandleGeoMe returns, without the "suggested spaces" payload.
//
// GET /api/v1/geo/detect
func (h *Handlers) HandleGeoDetect(w http.ResponseWriter, r *http.Request) {
	geo := FromContext(r.Context())
	if !geo.IsValid {
		writeError(w, http.StatusNotFound, "geo_not_available", "Geolocation data not available for this request")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"country": geo.CountryName,
			"state":   geo.Subdivision,
			"city":    geo.City,
		},
	})
}

// HandleListCountries returns every seeded country space.
//
// GET /api/v1/geo/countries
func (h *Handlers) HandleListCountries(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	countries, err := h.spaces.ListSpacesByLevel(r.Context(), t.ID, "country")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list countries")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": countries})
}

// HandleListStates returns the state/province spaces for a country, looked
// up by its ISO geo_code.
//
// GET /api/v1/geo/countries/{code}/states
func (h *Handlers) HandleListStates(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	code := chi.URLParam(r, "code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing_code", "country code is required")
		return
	}

	states, err := h.spaces.ListStatesByCountryCode(r.Context(), t.ID, code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list states")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": states})
}

// HandleListCities returns the city spaces for a state.
//
// GET /api/v1/geo/states/{id}/cities
func (h *Handlers) HandleListCities(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	stateIDStr := chi.URLParam(r, "id")

	var stateID pgtype.UUID
	if err := stateID.Scan(stateIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid state id format")
		return
	}

	cities, err := h.spaces.ListCitiesByState(r.Context(), t.ID, stateID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list cities")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": cities})
}

// suggestedSpaces runs the deterministic /geo/me suggested_spaces algorithm
// (Section 37): country Space by geo_code, then city Space by name. Returns
// an empty (never nil) slice on any miss; errors are logged and treated as
// "no match" so a lookup failure never breaks /geo/me.
func (h *Handlers) suggestedSpaces(r *http.Request, geo GeoContext) []SuggestedSpace {
	out := make([]SuggestedSpace, 0, 2)

	if h.spaces == nil {
		return out
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		return out
	}

	ctx := r.Context()

	// 1. Country space by geo_code.
	if geo.CountryCode != "" {
		country, err := h.spaces.GetSpaceByGeoCode(ctx, t.ID, geo.CountryCode)
		if err == nil {
			out = append(out, SuggestedSpace{
				ID:          country.ID.String(),
				Name:        country.Name,
				Slug:        country.Slug,
				Level:       country.Level,
				GeoCode:     derefStr(country.GeoCode),
				MemberCount: country.MemberCount,
				PostCount:   country.PostCount,
			})
		} else {
			slog.Debug("geoip: suggested_spaces country miss",
				"country_code", geo.CountryCode, "error", err)
		}
	}

	// 2. City space by name (case-insensitive, tenant-scoped).
	if geo.City != "" {
		city, err := h.spaces.FindCitySpaceByName(ctx, t.ID, geo.City)
		if err == nil {
			out = append(out, SuggestedSpace{
				ID:          city.ID.String(),
				Name:        city.Name,
				Slug:        city.Slug,
				Level:       city.Level,
				GeoCode:     derefStr(city.GeoCode),
				MemberCount: city.MemberCount,
				PostCount:   city.PostCount,
			})
		} else {
			slog.Debug("geoip: suggested_spaces city miss",
				"city", geo.City, "error", err)
		}
	}

	return out
}

// derefStr returns the string value of a *string, or "" when nil.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}

// ---------------------------------------------------------------------------
// JSON response helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			slog.Warn("geoip: json encode error", "error", err)
		}
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
