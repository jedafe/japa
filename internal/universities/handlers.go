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
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
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
	ListSpacesByType(ctx context.Context, tenantID, spaceTypeID pgtype.UUID) ([]gen.ListSpacesByTypeRow, error)
	GetSpaceBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetSpaceBySlugRow, error)
	GetSpace(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetSpaceByIDRow, error)
	CreateSpace(ctx context.Context, params gen.CreateSpaceParams) (gen.CreateSpaceRow, error)
	UpdateSpace(ctx context.Context, params gen.UpdateSpaceParams) (gen.UpdateSpaceRow, error)
	DeleteSpace(ctx context.Context, id, tenantID pgtype.UUID) error
}

// CustomFieldsService is the subset of customfields.Service the university
// handlers need to attach university-specific field values.
type CustomFieldsService interface {
	ListFieldValuesByEntity(ctx context.Context, entityType string, entityID pgtype.UUID) ([]gen.CustomFieldValue, error)
}

// MediaResolver resolves a media ID to an imgproxy-signed URL for the
// university logo. The media.Service satisfies this. Nil = logo_url omitted.
type MediaResolver interface {
	GetMedia(ctx context.Context, tenantID, mediaID pgtype.UUID) (gen.Medium, error)
	GetMediaURL(m *gen.Medium, opts media.ImageProxyOpts) string
}

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// The worker.Enqueuer satisfies this.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Handlers holds dependencies for university HTTP handlers.
type Handlers struct {
	spaces       SpacesService
	customFields CustomFieldsService
	media        MediaResolver // nil = logo_url omitted from responses
	enqueuer     TaskEnqueuer  // nil = logo_url fetch is skipped on create/update
}

// NewHandlers creates Handlers with the given services.
func NewHandlers(spacesService SpacesService, customFieldsService CustomFieldsService) *Handlers {
	return &Handlers{spaces: spacesService, customFields: customFieldsService}
}

// SetMediaResolver wires the media service used to resolve logo_media_id to a
// signed imgproxy URL (returned as logo_url in university responses). Optional.
func (h *Handlers) SetMediaResolver(m MediaResolver) { h.media = m }

// SetEnqueuer wires the Asynq enqueuer used to trigger university:fetch_logo
// when an admin sets/changes a university's logo_url. Optional — if not
// called, logo_url is accepted but not fetched (graceful degradation in
// dev/test).
func (h *Handlers) SetEnqueuer(e TaskEnqueuer) { h.enqueuer = e }

// enqueueFetchLogo enqueues university:fetch_logo (Section 39, low tier) to
// download logoURL, store it via the media service, and set the space's
// logo_media_id. uploadedBy is the acting admin's user ID (from
// auth.FromContext), attributed as the media uploader — media has no
// generic "system user" row, so the task skips gracefully if this is
// invalid. Fire-and-forget: enqueue failures are logged, never returned to
// the caller (Rule 8 — never block a request on fan-out).
func (h *Handlers) enqueueFetchLogo(ctx context.Context, tenantID, spaceID, uploadedBy pgtype.UUID, logoURL string) {
	if h.enqueuer == nil || strings.TrimSpace(logoURL) == "" {
		return
	}
	payload, err := json.Marshal(map[string]string{
		"tenant_id":   tenantID.String(),
		"space_id":    spaceID.String(),
		"logo_url":    logoURL,
		"uploaded_by": uploadedBy.String(),
	})
	if err != nil {
		slog.Warn("universities: marshal fetch_logo payload", "error", err)
		return
	}
	if err := h.enqueuer.Enqueue(worker.TypeUniversityFetchLogo, payload); err != nil {
		slog.Warn("universities: enqueue fetch_logo", "space_id", spaceID.String(), "error", err)
	}
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

// resolveLogoURL resolves a space's logo_media_id to an imgproxy-signed URL
// via the media service. Returns an empty string when no logo is set, the
// media resolver is not wired, or the media item cannot be loaded. Best-effort
// — a failure never blocks the university response.
func (h *Handlers) resolveLogoURL(ctx context.Context, tenantID pgtype.UUID, logoMediaID pgtype.UUID) string {
	if !logoMediaID.Valid || h.media == nil {
		return ""
	}
	m, err := h.media.GetMedia(ctx, tenantID, logoMediaID)
	if err != nil {
		return ""
	}
	return h.media.GetMediaURL(&m, media.ImageProxyOpts{Width: 128, Height: 128, Format: "webp"})
}

// universityDetail pairs a space with its resolved university-specific field
// values so a single response carries both. LogoURL is the imgproxy-signed URL
// for the university logo (resolved from logo_media_id); empty when no logo
// is set or the media service is unavailable.
type universityDetail struct {
	gen.GetSpaceBySlugRow
	Fields  []gen.CustomFieldValue `json:"fields"`
	LogoURL string                 `json:"logo_url"`
}

// ListUniversities handles GET /api/v1/universities — every space of the
// University space-type. Supports optional ?country= filtering by geo_code
// (ISO 3166-1 alpha-2). [public]
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

	// Optional country filter — applied in-memory since ListSpacesByType does
	// not accept a geo_code parameter. The catalog is small so this is fine.
	country := strings.TrimSpace(r.URL.Query().Get("country"))
	if country != "" {
		filtered := make([]gen.ListSpacesByTypeRow, 0, len(universities))
		for _, u := range universities {
			if u.GeoCode != nil && *u.GeoCode == country {
				filtered = append(filtered, u)
			}
		}
		universities = filtered
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

	// Resolve the logo media ID to an imgproxy-signed URL (Section 34 / ADR-039).
	// Best-effort: a missing or unreachable media service degrades to an empty
	// logo_url, and the frontend falls back to the GraduationCap icon.
	logoURL := h.resolveLogoURL(r.Context(), tenantID, space.LogoMediaID)

	writeJSON(w, http.StatusOK, map[string]any{"data": universityDetail{GetSpaceBySlugRow: space, Fields: fields, LogoURL: logoURL}})
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
		LogoURL  string `json:"logo_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid body")
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}

	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load university space type")
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
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create university")
		return
	}

	var actorID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		actorID = u.UserID
	}
	h.enqueueFetchLogo(r.Context(), tenantID, space.ID, actorID, input.LogoURL)

	writeJSON(w, http.StatusCreated, map[string]any{"data": space})
}

// UpdateUniversity handles PATCH /admin/universities/:id.
func (h *Handlers) UpdateUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	existing, err := h.spaces.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}

	var input struct {
		Name     *string `json:"name"`
		Slug     *string `json:"slug"`
		Verified *bool   `json:"verified"`
		LogoURL  *string `json:"logo_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid body")
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
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update university")
		return
	}

	if input.LogoURL != nil {
		var actorID pgtype.UUID
		if u := auth.FromContext(r.Context()); u != nil {
			actorID = u.UserID
		}
		h.enqueueFetchLogo(r.Context(), tenantID, space.ID, actorID, *input.LogoURL)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": space})
}

// DeleteUniversity handles DELETE /admin/universities/:id.
func (h *Handlers) DeleteUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	if err := h.spaces.DeleteSpace(r.Context(), id, tenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete university")
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	existing, err := h.spaces.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "University not found")
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
		writeError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify university")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": space})
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
