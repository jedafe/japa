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
	ListSpacesByLevel(ctx context.Context, tenantID pgtype.UUID, level string) ([]gen.Space, error)
	ListStatesByCountryCode(ctx context.Context, tenantID pgtype.UUID, countryCode string) ([]gen.Space, error)
	ListCitiesByState(ctx context.Context, tenantID, stateID pgtype.UUID) ([]gen.Space, error)
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
// so this endpoint simply reads it back.
//
// GET /api/v1/geo/me
func (h *Handlers) HandleGeoMe(w http.ResponseWriter, r *http.Request) {
	geo := FromContext(r.Context())
	if !geo.IsValid {
		writeJSON(w, http.StatusOK, map[string]any{"data": nil})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": geo})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": geo})
}

// HandleGeoDetect resolves the current request IP to a locale (catalog: GET
// /geo/detect → {country, state, city}), a narrower projection of the same
// GeoContext HandleGeoMe returns, without the "suggested spaces" payload.
//
// GET /api/v1/geo/detect
func (h *Handlers) HandleGeoDetect(w http.ResponseWriter, r *http.Request) {
	geo := FromContext(r.Context())
	if !geo.IsValid {
		writeJSON(w, http.StatusOK, map[string]any{"data": nil})
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
