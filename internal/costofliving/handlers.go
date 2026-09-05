// Package costofliving — handlers.go provides the HTTP handlers for the Cost
// of Living Calculator routes.
//
// Routes (Section 72, ADR-072):
//
//	GET /api/v1/calculator/cost-of-living/cities  — list cities (public)
//	GET /api/v1/calculator/cost-of-living/compare — ?city_ids=uuid,uuid → matrix
//	GET /api/v1/admin/calculator/cost-of-living/stats — admin stats
//
// Ref: japaconnect-engineering/sections/72-cost-of-living.md
//
//	japaconnect-engineering/adr/ADR-072-cost-of-living-calculator.md
package costofliving

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the cost-of-living calculator endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates cost-of-living handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as rag/handlers.go)
// ---------------------------------------------------------------------------

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}

	return id, true
}

// parseCityIDs parses the comma-separated city_ids query parameter into a
// slice of pgtype.UUID. Returns the parsed IDs and whether parsing succeeded.
func parseCityIDs(w http.ResponseWriter, r *http.Request) ([]pgtype.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("city_ids"))
	if raw == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_city_ids", "city_ids query parameter is required")
		return nil, false
	}

	parts := strings.Split(raw, ",")
	ids := make([]pgtype.UUID, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		var id pgtype.UUID
		if err := id.Scan(p); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_city_id", "Invalid city ID: "+p)
			return nil, false
		}

		ids = append(ids, id)
	}

	if len(ids) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_city_ids", "At least one city ID is required")
		return nil, false
	}

	return ids, true
}

// parsePageLimit extracts page and limit query params with safe defaults.
// Limit is capped at 200 to prevent excessive result sets.
func parsePageLimit(r *http.Request) (int, int) {
	page := 1
	limit := 20

	const maxLimit = 200

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}

	return page, limit
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// HandleListCities handles GET /api/v1/calculator/cost-of-living/cities —
// paginated, searchable list of cities with cost_of_living data. Public
// route (col_calculator.use — all users including anonymous).
// Query params: search, country_code, page, limit.
func (h *Handlers) HandleListCities(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "calculator_unavailable", "Cost of living service not configured")
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))
	countryCode := strings.TrimSpace(r.URL.Query().Get("country_code"))
	page, limit := parsePageLimit(r)

	resp, err := h.service.ListCities(r.Context(), tenantID, search, countryCode, page, limit)
	if err != nil {
		if errors.Is(err, ErrCalculatorDisabled) {
			errpkg.WriteError(w, http.StatusNotFound, "col_calculator_disabled", "Cost of Living Calculator is not available")
			return
		}

		slog.Error("costofliving: list cities failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_cities_failed", "Failed to list cities")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleCompare handles GET /api/v1/calculator/cost-of-living/compare —
// returns the comparison matrix with raw values + ratios vs the baseline.
// Public route. Query param: city_ids (comma-separated UUIDs).
func (h *Handlers) HandleCompare(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "calculator_unavailable", "Cost of living service not configured")
		return
	}

	cityIDs, ok := parseCityIDs(w, r)
	if !ok {
		return
	}

	matrix, err := h.service.CompareCities(r.Context(), tenantID, cityIDs)
	if err != nil {
		if errors.Is(err, ErrCalculatorDisabled) {
			errpkg.WriteError(w, http.StatusNotFound, "col_calculator_disabled", "Cost of Living Calculator is not available")
			return
		}

		slog.Error("costofliving: compare failed", "error", err)
		// Distinguish the max_cities violation for a clearer client error.
		if strings.Contains(err.Error(), "too many cities") {
			errpkg.WriteError(w, http.StatusBadRequest, "too_many_cities", err.Error())
			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "compare_failed", "Failed to compare cities")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": matrix})
}

// HandleAdminStats handles GET /api/v1/admin/calculator/cost-of-living/stats —
// data completeness + staleness report per city. Gated by
// col_calculator.manage (checked by RBAC middleware at route registration).
func (h *Handlers) HandleAdminStats(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "calculator_unavailable", "Cost of living service not configured")
		return
	}

	page, limit := parsePageLimit(r)

	resp, err := h.service.AdminStats(r.Context(), tenantID, page, limit)
	if err != nil {
		slog.Error("costofliving: admin stats failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", "Failed to get admin stats")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}
