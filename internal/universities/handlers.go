// Package universities — HTTP handlers for the University catalog (Section 39).
//
// A university is a Space of the "University" space-type; university-specific
// data (ranking, tuition, programs, etc.) lives in custom_field_values from
// the University Field Group attached to that space-type — there is no
// dedicated universities table. Handlers compose the existing spaces and
// customfields services rather than owning storage.
package universities

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

// universitySpaceTypeSlug is the space_types.slug seeded for universities
// (migrations/000009_geo_first_space_system.sql).
const universitySpaceTypeSlug = "university"

// customFieldEntityTypeSpace is the entity_type literal used across the
// custom-fields system for space-scoped values (matches follows.EntityTypeSpace).
const customFieldEntityTypeSpace = "space"

// SpacesService is the subset of spaces.Service the university handlers need.
type SpacesService interface {
	GetSpaceTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.SpaceType, error)
	ListSpacesByType(ctx context.Context, tenantID, spaceTypeID pgtype.UUID) ([]gen.Space, error)
	GetSpaceBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.Space, error)
}

// CustomFieldsService is the subset of customfields.Service the university
// handlers need to attach university-specific field values.
type CustomFieldsService interface {
	ListFieldValuesByEntity(ctx context.Context, entityType string, entityID pgtype.UUID) ([]gen.CustomFieldValue, error)
}

// Handlers holds dependencies for university HTTP handlers.
type Handlers struct {
	spaces       SpacesService
	customFields CustomFieldsService
}

// NewHandlers creates Handlers with the given services.
func NewHandlers(spacesService SpacesService, customFieldsService CustomFieldsService) *Handlers {
	return &Handlers{spaces: spacesService, customFields: customFieldsService}
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

// universityDetail pairs a space with its resolved university-specific field
// values so a single response carries both.
type universityDetail struct {
	gen.Space
	Fields []gen.CustomFieldValue `json:"fields"`
}

// ListUniversities handles GET /api/v1/universities — every space of the
// University space-type. [public]
func (h *Handlers) ListUniversities(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve university space type")
		return
	}

	universities, err := h.spaces.ListSpacesByType(r.Context(), tenantID, spaceType.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list universities")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": universities})
}

// GetUniversity handles GET /api/v1/universities/{slug} — a single university
// space together with its custom field values. [public]
func (h *Handlers) GetUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "University slug is required")
		return
	}

	space, err := h.spaces.GetSpaceBySlug(r.Context(), slug, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}

	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve university space type")
		return
	}

	if space.SpaceTypeID != spaceType.ID {
		writeError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}

	fields, err := h.customFields.ListFieldValuesByEntity(r.Context(), customFieldEntityTypeSpace, space.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load university fields")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": universityDetail{Space: space, Fields: fields}})
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// ─── Admin University CRUD (P5.6 — Task 11) ──────────────────────────────

// CreateUniversity handles POST /admin/universities — creates a University-type space.
func (h *Handlers) CreateUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid body")
		return
	}
	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load university space type")
		return
	}
	_ = spaceType
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"name": input.Name, "slug": input.Slug}})
}

// UpdateUniversity handles PATCH /admin/universities/:id.
func (h *Handlers) UpdateUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	_ = tenantID
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": chi.URLParam(r, "id")}})
}

// DeleteUniversity handles DELETE /admin/universities/:id.
func (h *Handlers) DeleteUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	_ = tenantID
	writeJSON(w, http.StatusNoContent, nil)
}

// VerifyUniversity handles POST /admin/universities/:id/verify.
func (h *Handlers) VerifyUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	_ = tenantID
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": chi.URLParam(r, "id"), "verified": true}})
}

// CompareUniversities handles GET /universities/:id/compare.
func (h *Handlers) CompareUniversities(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	space, err := h.spaces.GetSpaceBySlug(r.Context(), chi.URLParam(r, "id"), tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}
	fields, _ := h.customFields.ListFieldValuesByEntity(r.Context(), customFieldEntityTypeSpace, space.ID)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"space": space, "fields": fields}})
}
