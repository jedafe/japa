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
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/httpx"
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
	GetSpace(ctx context.Context, id, tenantID pgtype.UUID) (gen.Space, error)
	CreateSpace(ctx context.Context, params gen.CreateSpaceParams) (gen.Space, error)
	UpdateSpace(ctx context.Context, params gen.UpdateSpaceParams) (gen.Space, error)
	DeleteSpace(ctx context.Context, id, tenantID pgtype.UUID) error
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
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
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
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve university space type")
		return
	}

	universities, err := h.spaces.ListSpacesByType(r.Context(), tenantID, spaceType.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list universities")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": universities})
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
		httpx.WriteError(w, http.StatusBadRequest, "missing_slug", "University slug is required")
		return
	}

	space, err := h.spaces.GetSpaceBySlug(r.Context(), slug, tenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}

	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve university space type")
		return
	}

	if space.SpaceTypeID != spaceType.ID {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}

	fields, err := h.customFields.ListFieldValuesByEntity(r.Context(), customFieldEntityTypeSpace, space.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load university fields")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": universityDetail{Space: space, Fields: fields}})
}

// ─── Admin University CRUD (P5.6 — Task 11) ──────────────────────────────

var (
	invalidSlugChars = regexp.MustCompile(`[^a-z0-9-]+`)
	multipleHyphens  = regexp.MustCompile(`-{2,}`)
	trimHyphens      = regexp.MustCompile(`^-+|-+$`)
)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = invalidSlugChars.ReplaceAllString(s, "-")
	s = multipleHyphens.ReplaceAllString(s, "-")
	s = trimHyphens.ReplaceAllString(s, "")
	if s == "" {
		return "untitled"
	}
	return s
}

// CreateUniversity handles POST /admin/universities — creates a University-type space.
func (h *Handlers) CreateUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid body")
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}

	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load university space type")
		return
	}

	slug := strings.TrimSpace(input.Slug)
	if slug == "" {
		slug = slugify(input.Name)
	}

	status := "active"
	if !input.Verified {
		status = "pending"
	}

	space, err := h.spaces.CreateSpace(r.Context(), gen.CreateSpaceParams{
		TenantID:       tenantID,
		SpaceTypeID:    spaceType.ID,
		Name:           strings.TrimSpace(input.Name),
		Slug:           slug,
		Level:          "custom",
		Visibility:     "public",
		PostingPerm:    "members",
		ReplyingPerm:   "members",
		CommentingPerm: "members",
		JoiningPerm:    "open",
		Status:         status,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create university")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": space})
}

// UpdateUniversity handles PATCH /admin/universities/:id.
func (h *Handlers) UpdateUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	existing, err := h.spaces.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}

	var input struct {
		Name     *string `json:"name"`
		Slug     *string `json:"slug"`
		Verified *bool   `json:"verified"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid body")
		return
	}

	name := existing.Name
	if input.Name != nil && strings.TrimSpace(*input.Name) != "" {
		name = strings.TrimSpace(*input.Name)
	}

	status := existing.Status
	if input.Verified != nil {
		if *input.Verified {
			status = "active"
		} else {
			status = "suspended"
		}
	}

	space, err := h.spaces.UpdateSpace(r.Context(), gen.UpdateSpaceParams{
		ID:             id,
		TenantID:       tenantID,
		Name:           name,
		Description:    existing.Description,
		WikiContent:    existing.WikiContent,
		GeoCode:        existing.GeoCode,
		Visibility:     existing.Visibility,
		PostingPerm:    existing.PostingPerm,
		ReplyingPerm:   existing.ReplyingPerm,
		CommentingPerm: existing.CommentingPerm,
		JoiningPerm:    existing.JoiningPerm,
		Status:         status,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update university")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": space})
}

// DeleteUniversity handles DELETE /admin/universities/:id.
func (h *Handlers) DeleteUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	if err := h.spaces.DeleteSpace(r.Context(), id, tenantID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete university")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// VerifyUniversity handles POST /admin/universities/:id/verify.
func (h *Handlers) VerifyUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	existing, err := h.spaces.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}

	space, err := h.spaces.UpdateSpace(r.Context(), gen.UpdateSpaceParams{
		ID:             id,
		TenantID:       tenantID,
		Name:           existing.Name,
		Description:    existing.Description,
		WikiContent:    existing.WikiContent,
		GeoCode:        existing.GeoCode,
		Visibility:     existing.Visibility,
		PostingPerm:    existing.PostingPerm,
		ReplyingPerm:   existing.ReplyingPerm,
		CommentingPerm: existing.CommentingPerm,
		JoiningPerm:    existing.JoiningPerm,
		Status:         "active",
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify university")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": space})
}

// CompareUniversities handles GET /universities/:id/compare.
func (h *Handlers) CompareUniversities(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	space, err := h.spaces.GetSpaceBySlug(r.Context(), chi.URLParam(r, "id"), tenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}
	fields, _ := h.customFields.ListFieldValuesByEntity(r.Context(), customFieldEntityTypeSpace, space.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"space": space, "fields": fields}})
}
