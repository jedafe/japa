package geoip

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// SpacesService is the subset of spaces.Service the geo hierarchy handlers
// need — countries/states/cities are seeded as spaces (catalog: "from seeded
// country spaces"), not a separate table.
type SpacesService interface {
	ListSpacesByLevel(ctx context.Context, tenantID, viewerUserID pgtype.UUID, level string) ([]gen.ListSpacesByLevelRow, error)
	ListStatesByCountryCode(ctx context.Context, tenantID pgtype.UUID, countryCode string) ([]gen.ListStatesByCountryGeoCodeRow, error)
	ListCitiesByState(ctx context.Context, tenantID, stateID pgtype.UUID) ([]gen.ListCitiesByStateIDRow, error)
	GetSpaceByGeoCode(ctx context.Context, tenantID pgtype.UUID, countryCode string) (gen.GetSpaceByGeoCodeRow, error)
	FindCitySpaceByName(ctx context.Context, tenantID pgtype.UUID, cityName string) (gen.FindCitySpaceByNameRow, error)
	CreateCity(ctx context.Context, tenantID, stateID pgtype.UUID, name string) (gen.CreateSpaceRow, error)
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

// GeoSpace is the public projection of a seeded geo Space returned by the
// hierarchy endpoints (/geo/countries, /geo/countries/{code}/states,
// /geo/states/{id}/cities). It is a stable contract type, independent of
// sqlc-generated row structs, so regenerating internal/db never changes the
// API shape and internal fields (, perms, wiki_content, etc.) are
// never leaked to unauthenticated callers.
type GeoSpace struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Slug          string  `json:"slug"`
	Level         string  `json:"level"`
	GeoCode       string  `json:"geo_code,omitempty"`
	Description   string  `json:"description,omitempty"`
	CurrencyCode  *string `json:"currency_code,omitempty"`
	MemberCount   int32   `json:"member_count"`
	PostCount     int32   `json:"post_count"`
	FollowerCount int32   `json:"follower_count"`
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
// // GET /api/v1/geo/me.
func (h *Handlers) HandleGeoMe(w http.ResponseWriter, r *http.Request) {
	geo := FromContext(r.Context())
	if !geo.IsValid {
		errpkg.WriteError(w, http.StatusNotFound, "geo_not_available", "Geolocation data not available for this request")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleGeoLookup resolves an arbitrary IP address (admin only).
// Returns the geo data for the given IP query parameter.
// // GET /api/v1/geo/lookup?ip=1.2.3.4.
func (h *Handlers) HandleGeoLookup(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ip", "ip query parameter is required")
		return
	}

	// Validate IP format before touching the database. This separates the
	// two distinct error conditions per Section 37: (a) the param is not a
	// valid IP address format → 400 invalid_ip; (b) the IP is valid format
	// but resolves to a reserved range / has no country data → 404
	// geo_not_available. Conflating them (the previous behaviour) returned
	// 400 lookup_failed for a valid-format IP like 10.0.0.1, which violated
	// the IsValid semantics in Section 37.
	if net.ParseIP(ip) == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_ip", "ip query parameter is not a valid IP address")
		return
	}

	if !h.reader.IsLoaded() {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "geoip_not_loaded", "GeoIP database not loaded")
		return
	}

	geo := h.reader.Lookup(ip)
	if !geo.IsValid {
		errpkg.WriteError(w, http.StatusNotFound, "geo_not_available", "No geolocation data for this IP address")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleGeoDetect resolves the current request IP to a locale (catalog: GET
// /geo/detect → {country, state, city}), a narrower projection of the same
// GeoContext HandleGeoMe returns, without the "suggested spaces" payload.
// // GET /api/v1/geo/detect.
func (h *Handlers) HandleGeoDetect(w http.ResponseWriter, r *http.Request) {
	geo := FromContext(r.Context())
	if !geo.IsValid {
		errpkg.WriteError(w, http.StatusNotFound, "geo_not_available", "Geolocation data not available for this request")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"country": geo.CountryName,
			"state":   geo.Subdivision,
			"city":    geo.City,
		},
	})
}

// HandleListCountries returns every seeded country space.
// // GET /api/v1/geo/countries.
func (h *Handlers) HandleListCountries(w http.ResponseWriter, r *http.Request) {
	t := &struct{ ID pgtype.UUID }{}
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	countries, err := h.spaces.ListSpacesByLevel(r.Context(), t.ID, pgtype.UUID{}, "country")
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list countries")
		return
	}

	out := make([]GeoSpace, 0, len(countries))
	for _, c := range countries {
		out = append(out, projectGeoSpace(c.ID, c.Name, c.Slug, c.Level, c.GeoCode, c.Description, c.CurrencyCode, c.MemberCount, c.PostCount, c.FollowerCount))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// HandleListStates returns the state/province spaces for a country, looked
// up by its ISO geo_code.
// // GET /api/v1/geo/countries/{code}/states.
func (h *Handlers) HandleListStates(w http.ResponseWriter, r *http.Request) {
	t := &struct{ ID pgtype.UUID }{}
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	code := chi.URLParam(r, "code")
	if code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_code", "country code is required")
		return
	}

	states, err := h.spaces.ListStatesByCountryCode(r.Context(), t.ID, code)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list states")
		return
	}

	out := make([]GeoSpace, 0, len(states))
	for _, s := range states {
		out = append(out, projectGeoSpace(s.ID, s.Name, s.Slug, s.Level, s.GeoCode, s.Description, s.CurrencyCode, s.MemberCount, s.PostCount, s.FollowerCount))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// HandleListCities returns the city spaces for a state.
// // GET /api/v1/geo/states/{id}/cities.
func (h *Handlers) HandleListCities(w http.ResponseWriter, r *http.Request) {
	t := &struct{ ID pgtype.UUID }{}
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	stateIDStr := chi.URLParam(r, "id")

	var stateID pgtype.UUID
	if err := stateID.Scan(stateIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid state id format")
		return
	}

	cities, err := h.spaces.ListCitiesByState(r.Context(), t.ID, stateID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list cities")
		return
	}

	out := make([]GeoSpace, 0, len(cities))
	for _, c := range cities {
		out = append(out, projectGeoSpace(c.ID, c.Name, c.Slug, c.Level, c.GeoCode, c.Description, c.CurrencyCode, c.MemberCount, c.PostCount, c.FollowerCount))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// CreateCity creates a new city-level geo space under the given state.
// It is intentionally quota-free because adding a missing city is part of
// completing a user's location, not creating an owned community.
// // POST /api/v1/geo/states/{id}/cities.
func (h *Handlers) CreateCity(w http.ResponseWriter, r *http.Request) {
	t := &struct{ ID pgtype.UUID }{}
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	stateIDStr := chi.URLParam(r, "id")

	var stateID pgtype.UUID

	if err := stateID.Scan(stateIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid state id format")
		return
	}

	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "name is required")
		return
	}

	city, err := h.spaces.CreateCity(r.Context(), t.ID, stateID, name)
	if err != nil {
		slog.Warn("create city failed", "user_id", user.UserID.String(), "state_id", stateIDStr, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Could not create city")

		return
	}

	out := projectGeoSpace(
		city.ID, city.Name, city.Slug, city.Level,
		city.GeoCode, city.Description, city.CurrencyCode,
		city.MemberCount, city.PostCount, city.FollowerCount,
	)
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": out})
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

	t := &struct{ ID pgtype.UUID }{}
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

// projectGeoSpace builds a GeoSpace projection from the common fields shared
// by all geo hierarchy sqlc row structs. It is the single chokepoint that
// keeps internal columns (, space_type_id, wiki_content, perms,
// logo_media_id, header_image_id, deleted_at, etc.) out of the public API
// response. currency_code is intentionally exposed so geo-aware surfaces can
// default currency to the selected primary space (Rule 16, ADR-065).
func projectGeoSpace(
	id pgtype.UUID,
	name, slug, level string,
	geoCode, description *string,
	currencyCode *string,
	memberCount, postCount, followerCount int32,
) GeoSpace {
	g := GeoSpace{
		ID:            id.String(),
		Name:          name,
		Slug:          slug,
		Level:         level,
		GeoCode:       derefStr(geoCode),
		Description:   derefStr(description),
		CurrencyCode:  currencyCode,
		MemberCount:   memberCount,
		PostCount:     postCount,
		FollowerCount: followerCount,
	}
	if g.CurrencyCode != nil && *g.CurrencyCode == "" {
		g.CurrencyCode = nil
	}

	return g
}

// ---------------------------------------------------------------------------
// JSON response helpers
// ---------------------------------------------------------------------------
