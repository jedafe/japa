package geoip

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Handlers provides HTTP endpoints for geolocation.
type Handlers struct {
	reader *Reader
}

// NewHandlers creates new geo handlers.
func NewHandlers(reader *Reader) *Handlers {
	return &Handlers{reader: reader}
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
