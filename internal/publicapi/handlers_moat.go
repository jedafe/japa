package publicapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// requireAPIKey writes a 401 and returns false when no key context exists.
func requireAPIKey(w http.ResponseWriter, r *http.Request) bool {
	if APIKeyFromContext(r.Context()) == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return false
	}
	return true
}

func parseUUIDQuery(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if v := r.URL.Query().Get(param); v != "" {
		if err := id.Scan(v); err != nil {
			return pgtype.UUID{}, err
		}
	}
	return id, nil
}

// ListVisaRoutes handles GET /public/visa-routes.
// Params: dest, profession, education, experience_years, page_size, offset.
func (h *Handlers) ListVisaRoutes(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	dest, err := parseUUIDQuery(r, "dest")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dest space ID")
		return
	}
	years, _ := strconv.ParseInt(r.URL.Query().Get("experience_years"), 10, 32)
	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)
	page := offset/limit + 1

	routes, total, err := h.Service.ListVisaRoutes(r.Context(), dest,
		r.URL.Query().Get("profession"), r.URL.Query().Get("education"),
		int32(years), page, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": routes, "total": total, "limit": limit, "offset": offset,
	})
}

// GetVisaRoute handles GET /public/visa-routes/{id}.
func (h *Handlers) GetVisaRoute(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid visa route ID")
		return
	}
	route, err := h.Service.GetVisaRoute(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": route})
}

// CompareVisaRoutes handles GET /public/visa-routes/compare?ids=a,b,c.
func (h *Handlers) CompareVisaRoutes(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	raw := r.URL.Query().Get("ids")
	if raw == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ids", "ids query param is required")
		return
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 5 {
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_ids", "At most 5 route IDs may be compared")
		return
	}
	ids := make([]pgtype.UUID, 0, len(parts))
	for _, p := range parts {
		var id pgtype.UUID
		if err := id.Scan(strings.TrimSpace(p)); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid route ID in ids list")
			return
		}
		ids = append(ids, id)
	}
	routes, err := h.Service.CompareVisaRoutes(r.Context(), ids)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": routes})
}

// GetVisaTimelineStats handles GET /public/visa-timelines/stats.
// Params: origin, dest, visa, consulate, months.
func (h *Handlers) GetVisaTimelineStats(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	origin, err := parseUUIDQuery(r, "origin")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid origin space ID")
		return
	}
	dest, err := parseUUIDQuery(r, "dest")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dest space ID")
		return
	}
	months := 0
	if v := r.URL.Query().Get("months"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 60 {
			months = n
		}
	}
	stats, err := h.Service.GetVisaTimelineStats(r.Context(), origin, dest,
		r.URL.Query().Get("visa"), r.URL.Query().Get("consulate"), months)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
}

// GetVisaTimelineTrend handles GET /public/visa-timelines/stats/trend.
func (h *Handlers) GetVisaTimelineTrend(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	dest, err := parseUUIDQuery(r, "dest")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dest space ID")
		return
	}
	months := 12
	if v := r.URL.Query().Get("months"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 60 {
			months = n
		}
	}
	trend, err := h.Service.GetVisaTimelineTrend(r.Context(), dest,
		r.URL.Query().Get("visa"), r.URL.Query().Get("consulate"), months)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": trend})
}

// ListCredentialProfessions handles GET /public/credentials/professions.
func (h *Handlers) ListCredentialProfessions(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	profs, err := h.Service.ListCredentialProfessions(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profs})
}

// ListCredentialPathways handles GET /public/credentials.
// Params: dest, issuing, profession, page_size, offset.
func (h *Handlers) ListCredentialPathways(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	dest, err := parseUUIDQuery(r, "dest")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dest space ID")
		return
	}
	issuing, err := parseUUIDQuery(r, "issuing")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid issuing space ID")
		return
	}
	profession, err := parseUUIDQuery(r, "profession")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profession ID")
		return
	}
	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)
	page := offset/limit + 1

	pathways, total, err := h.Service.ListCredentialPathways(r.Context(), dest, issuing, profession, page, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": pathways, "total": total, "limit": limit, "offset": offset,
	})
}

// GetCredentialPathway handles GET /public/credentials/{id}.
func (h *Handlers) GetCredentialPathway(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid pathway ID")
		return
	}
	p, err := h.Service.GetCredentialPathway(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": p})
}

// ListLanguageTests handles GET /public/language-tests.
func (h *Handlers) ListLanguageTests(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	tests, err := h.Service.ListLanguageTests(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tests})
}

// GetLanguageTest handles GET /public/language-tests/{slug}.
func (h *Handlers) GetLanguageTest(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}
	test, err := h.Service.GetLanguageTest(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": test})
}

// ListLanguageTestRequirements handles GET /public/language-tests/requirements.
// Params: dest (required), visa.
func (h *Handlers) ListLanguageTestRequirements(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r) {
		return
	}
	dest, err := parseUUIDQuery(r, "dest")
	if err != nil || !dest.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "A valid dest space ID is required")
		return
	}
	reqs, err := h.Service.ListLanguageTestRequirements(r.Context(), dest, r.URL.Query().Get("visa"))
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": reqs})
}
