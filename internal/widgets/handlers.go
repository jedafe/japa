package widgets

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers holds dependencies for widget HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}


func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20

	if v := r.URL.Query().Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	} else if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	} else if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

func totalPages(total int64, limit int32) int64 {
	if limit <= 0 {
		return 1
	}

	return (total + int64(limit) - 1) / int64(limit)
}

func parseUUIDParam(r *http.Request, key string) (pgtype.UUID, error) {
	var u pgtype.UUID
	err := u.Scan(chi.URLParam(r, key))

	return u, err
}

// ListWidgets handles GET /api/v1/admin/widgets.
func (h *Handlers) ListWidgets(w http.ResponseWriter, r *http.Request) {

	page, limit := parsePageParams(r)

	widgets, total, err := h.Service.List(r.Context(), r.URL.Query().Get("q"), page, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        widgets,
		"total":       total,
		"total_pages": totalPages(total, limit),
		"page":        page,
		"page_size":   limit,
	})
}

// GetWidget handles GET /api/v1/admin/widgets/{id}.
func (h *Handlers) GetWidget(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	widget, err := h.Service.Get(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": widget})
}

// CreateWidget handles POST /api/v1/admin/widgets.
func (h *Handlers) CreateWidget(w http.ResponseWriter, r *http.Request) {

	var input CreateWidgetInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	widget, err := h.Service.Create(r.Context(), input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": widget})
}

// UpdateWidget handles PATCH /api/v1/admin/widgets/{id}.
func (h *Handlers) UpdateWidget(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input UpdateWidgetInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	widget, err := h.Service.Update(r.Context(), id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": widget})
}

// DeleteWidget handles DELETE /api/v1/admin/widgets/{id}.
func (h *Handlers) DeleteWidget(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.Delete(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// RenderWidget handles GET /api/v1/widgets/{id} — public endpoint that returns
// rendered widget data, Valkey-cached by the widget's cache_ttl_secs.
// Sets X-Cache: HIT or X-Cache: MISS response header.
func (h *Handlers) RenderWidget(w http.ResponseWriter, r *http.Request) {
	// Public endpoint — no platform required (widget is resolved
	// row itself). No auth required per Section 16.
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	rendered, cacheHit, err := h.Service.RenderWidget(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if cacheHit {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rendered})
}

// RequireAuth is a no-op middleware placeholder for documentation — the public
// render endpoint does not require auth. The admin CRUD routes use the
// auth.RequireAuth middleware wired at the router level.
var _ = auth.RequireAuth
