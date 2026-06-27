package docs

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for documentation HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "duplicate"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// ---------------------------------------------------------------------------
// Public Handlers (read-only, RBAC-gated by audience)
// ---------------------------------------------------------------------------

// ListDocPages handles GET /api/v1/docs/{audience}.
// Returns published pages for the given audience section.
func (h *Handlers) ListDocPages(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	audience := chi.URLParam(r, "audience")
	if !ValidAudiences[audience] {
		writeError(w, http.StatusBadRequest, "invalid_audience", "Invalid audience section")
		return
	}
	pages, err := h.Service.ListDocPagesByAudience(r.Context(), tenantID, audience)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": pages})
}

// GetDocPage handles GET /api/v1/docs/{audience}/{slug}.
// Returns a single published page.
func (h *Handlers) GetDocPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	audience := chi.URLParam(r, "audience")
	slug := chi.URLParam(r, "slug")
	if !ValidAudiences[audience] {
		writeError(w, http.StatusBadRequest, "invalid_audience", "Invalid audience section")
		return
	}
	page, err := h.Service.GetDocPageBySlug(r.Context(), tenantID, audience, slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": page})
}

// ---------------------------------------------------------------------------
// Admin Handlers (write operations)
// ---------------------------------------------------------------------------

// ListAllDocPages handles GET /api/v1/admin/docs/{audience}.
// Returns all pages (any status) for admin listing.
func (h *Handlers) ListAllDocPages(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	audience := chi.URLParam(r, "audience")
	if !ValidAudiences[audience] {
		writeError(w, http.StatusBadRequest, "invalid_audience", "Invalid audience section")
		return
	}
	pages, err := h.Service.ListAllDocPagesByAudience(r.Context(), tenantID, audience)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": pages})
}

// CreateDocPage handles POST /api/v1/admin/docs.
func (h *Handlers) CreateDocPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var req CreateDocPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if req.Title == "" || req.Slug == "" || req.Audience == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "title, slug, and audience are required")
		return
	}
	if err := h.Service.CreateDocPage(r.Context(), tenantID, req); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

// UpdateDocPage handles PATCH /api/v1/admin/docs/{id}.
func (h *Handlers) UpdateDocPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}
	var req UpdateDocPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "title is required")
		return
	}
	if err := h.Service.UpdateDocPage(r.Context(), tenantID, id, req); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteDocPage handles DELETE /api/v1/admin/docs/{id}.
func (h *Handlers) DeleteDocPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}
	if err := h.Service.DeleteDocPage(r.Context(), tenantID, id); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
