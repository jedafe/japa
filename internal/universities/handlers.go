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
	"encoding/csv"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
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
	ListSpacesByTypePaginated(ctx context.Context, tenantID, spaceTypeID pgtype.UUID, search, geoCode, sort string, limit, offset int32) ([]gen.ListSpacesByTypePaginatedRow, error)
	CountSpacesByType(ctx context.Context, tenantID, spaceTypeID pgtype.UUID, search, geoCode string) (int64, error)
	GetSpaceBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetSpaceBySlugRow, error)
	GetSpace(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetSpaceByIDRow, error)
	CreateSpace(ctx context.Context, params gen.CreateSpaceParams) (gen.CreateSpaceRow, error)
	UpdateSpace(ctx context.Context, params gen.UpdateSpaceParams) (gen.UpdateSpaceRow, error)
	UpdateSpaceSlug(ctx context.Context, id, tenantID pgtype.UUID, slug string) (gen.UpdateSpaceSlugRow, error)
	DeleteSpace(ctx context.Context, id, tenantID pgtype.UUID) error
}

// CustomFieldsService is the subset of customfields.Service the university
// handlers need to attach university-specific field values.
type CustomFieldsService interface {
	ListFieldValuesByEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) ([]gen.CustomFieldValue, error)
	// ListFieldValuesWithDefsByEntity returns field values joined with their
	// definitions (label, key, field_type, options), filtered to
	// show_in_frontend = true — the display-ready shape used by the public
	// /universities/{slug} About facts grid (Section 39).
	ListFieldValuesWithDefsByEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) ([]gen.ListCustomFieldValuesWithDefsByEntityRow, error)
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
		"version":     "1",
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
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
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

// universityField is a display-ready custom field value: the human-readable
// label, the machine key, the resolved display value (typed value rendered to
// a string, with select/multiselect option values mapped to their labels),
// and the field_type so the frontend can render booleans/dates/URLs specially.
// Used by the /universities/{slug} About facts grid (Section 39 sidebar:
// "key custom field values (ranking, tuition, programs, etc.)").
type universityField struct {
	Label     string `json:"label"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	FieldType string `json:"field_type"`
}

// universityDetail pairs a space with its resolved university-specific field
// values so a single response carries both. LogoURL is the imgproxy-signed URL
// for the university logo (resolved from logo_media_id); empty when no logo
// is set or the media service is unavailable. DisplayFields is the
// show_in_frontend-filtered, definition-joined set rendered on the public
// About tab; Fields is the raw value set retained for the compare page and
// admin tooling.
type universityDetail struct {
	gen.GetSpaceBySlugRow
	Fields        []gen.CustomFieldValue `json:"fields"`
	DisplayFields []universityField      `json:"display_fields"`
	LogoURL       string                 `json:"logo_url"`
}

// ListUniversities handles GET /api/v1/universities — every space of the
// University space-type. Honors the Section 39 catalogued query params:
// ?country= (ISO 3166-1 alpha-2), ?q= (name/slug ILIKE search), ?page= (1-based),
// ?limit= (page size, default 25, max 100). Returns {data, meta} with pagination
// metadata. Server-side filtered + paginated so the ~9,000-row seeded catalog
// never loads in full on a single SSR render (Section 39 line 149, ADR-039).
// [public]
func (h *Handlers) ListUniversities(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve university space type")
		return
	}

	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	limit := parseIntDefault(r.URL.Query().Get("limit"), 25)
	if limit < 1 || limit > 100 {
		limit = 25
	}
	if page < 1 {
		page = 1
	}
	offset := int32((page - 1) * limit)
	limit32 := int32(limit)

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	country := strings.TrimSpace(r.URL.Query().Get("country"))
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))

	universities, err := h.spaces.ListSpacesByTypePaginated(r.Context(), tenantID, spaceType.ID, search, country, sort, limit32, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list universities")
		return
	}

	total, err := h.spaces.CountSpacesByType(r.Context(), tenantID, spaceType.ID, search, country)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count universities")
		return
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": universities,
		"meta": map[string]any{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

// ListUniversitiesAdmin handles GET /api/v1/admin/universities — server-side
// paginated list of university spaces with search and country filter.
// Supports ?page=&limit=&q=&country= query params. Returns {data, meta} with
// pagination metadata for the admin DataTable. [admin: admin.settings]
func (h *Handlers) ListUniversitiesAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve university space type")
		return
	}

	// Parse pagination params.
	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	limit := parseIntDefault(r.URL.Query().Get("limit"), 25)
	if limit < 1 || limit > 200 {
		limit = 25
	}
	if page < 1 {
		page = 1
	}
	offset := int32((page - 1) * limit)
	limit32 := int32(limit)

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	country := strings.TrimSpace(r.URL.Query().Get("country"))
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))

	universities, err := h.spaces.ListSpacesByTypePaginated(r.Context(), tenantID, spaceType.ID, search, country, sort, limit32, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list universities")
		return
	}

	total, err := h.spaces.CountSpacesByType(r.Context(), tenantID, spaceType.ID, search, country)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count universities")
		return
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": universities,
		"meta": map[string]any{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
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
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "University slug is required")
		return
	}

	space, err := h.spaces.GetSpaceBySlug(r.Context(), slug, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}

	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve university space type")
		return
	}

	if space.SpaceTypeID != spaceType.ID {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}

	fields, err := h.customFields.ListFieldValuesByEntity(r.Context(), tenantID, customFieldEntityTypeSpace, space.ID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load university fields")
		return
	}

	// Display-ready fields (Section 39 About facts grid): values joined with
	// their definitions and filtered to show_in_frontend = true, with
	// select/multiselect option values resolved to labels. Best-effort — a
	// failure degrades to an empty slice (the About tab still renders the
	// wiki extract). Fire-and-forget resolution keeps the public endpoint fast.
	displayFields := make([]universityField, 0)
	if rows, derr := h.customFields.ListFieldValuesWithDefsByEntity(r.Context(), tenantID, customFieldEntityTypeSpace, space.ID); derr == nil {
		for _, row := range rows {
			if dv, ok := resolveDisplayValue(row); ok {
				displayFields = append(displayFields, universityField{
					Label:     row.FieldLabel,
					Key:       row.FieldKey,
					Value:     dv,
					FieldType: row.FieldType,
				})
			}
		}
	} else {
		slog.Warn("universities: load display fields", "space_id", space.ID.String(), "error", derr)
	}

	// Resolve the logo media ID to an imgproxy-signed URL (Section 34 / ADR-039).
	// Best-effort: a missing or unreachable media service degrades to an empty
	// logo_url, and the frontend falls back to the GraduationCap icon.
	logoURL := h.resolveLogoURL(r.Context(), tenantID, space.LogoMediaID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": universityDetail{
		GetSpaceBySlugRow: space,
		Fields:            fields,
		DisplayFields:     displayFields,
		LogoURL:           logoURL,
	}})
}

// resolveDisplayValue picks the populated typed-value column from a
// custom_field_values row and renders it to a display string. For select and
// multiselect fields, the stored value_text is an option key (or a JSON array
// of keys) that is mapped to its human-readable label via the field's options
// JSONB column ([{value, label}, ...]). Returns ok=false when no value column
// is populated (the field is skipped — never rendered as an empty row).
func resolveDisplayValue(row gen.ListCustomFieldValuesWithDefsByEntityRow) (string, bool) {
	switch row.FieldType {
	case "boolean":
		if row.ValueBool != nil {
			if *row.ValueBool {
				return "Yes", true
			}
			return "No", true
		}
		return "", false
	case "number", "currency", "percentage", "rating":
		// pgtype.Numeric holds the value in a big.Int + exponent; render via
		// the Float64 accessor. Integer-valued numerics (rankings, years,
		// student population) print without a trailing decimal part.
		if row.ValueNumber.Valid {
			if f, err := row.ValueNumber.Float64Value(); err == nil && f.Valid {
				return strconv.FormatFloat(f.Float64, 'f', -1, 64), true
			}
		}
		return "", false
	case "date", "datetime":
		if row.ValueDate.Valid {
			return row.ValueDate.Time.Format("2006-01-02"), true
		}
		return "", false
	case "select":
		if row.ValueText == nil || *row.ValueText == "" {
			return "", false
		}
		if label, ok := lookupOptionLabel(row.FieldOptions, *row.ValueText); ok {
			return label, true
		}
		return *row.ValueText, true
	case "multiselect":
		// value_json holds a JSON array of selected option keys; fall back to
		// value_text if the legacy single-string form was stored.
		keys := decodeMultiselectKeys(row.ValueJson, row.ValueText)
		if len(keys) == 0 {
			return "", false
		}
		labels := make([]string, 0, len(keys))
		for _, k := range keys {
			if label, ok := lookupOptionLabel(row.FieldOptions, k); ok {
				labels = append(labels, label)
			} else {
				labels = append(labels, k)
			}
		}
		return strings.Join(labels, ", "), true
	default:
		// text, textarea, url, email, phone, color, etc. — render value_text.
		if row.ValueText != nil && *row.ValueText != "" {
			return *row.ValueText, true
		}
		return "", false
	}
}

// fieldOption mirrors the {value, label} shape stored in custom_fields.options
// for select/multiselect fields (see internal/customfields.FieldOption).
type fieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// lookupOptionLabel finds the human-readable label for a given option value
// in the field's JSONB options column. Returns the value itself if the
// options blob is missing or the value is not found (graceful degradation).
func lookupOptionLabel(optionsJSON []byte, value string) (string, bool) {
	if len(optionsJSON) == 0 {
		return value, false
	}
	var opts []fieldOption
	if err := json.Unmarshal(optionsJSON, &opts); err != nil {
		return value, false
	}
	for _, o := range opts {
		if o.Value == value {
			return o.Label, true
		}
	}
	return value, false
}

// decodeMultiselectKeys extracts the selected option keys from a multiselect
// value. Modern storage uses value_json (a JSON array of strings); the legacy
// form stored a comma-separated string in value_text. Returns nil when no
// keys are present.
func decodeMultiselectKeys(valueJSON []byte, valueText *string) []string {
	if len(valueJSON) > 0 {
		var keys []string
		if err := json.Unmarshal(valueJSON, &keys); err == nil && len(keys) > 0 {
			return keys
		}
	}
	if valueText != nil && *valueText != "" {
		// Legacy comma-separated form.
		parts := strings.Split(*valueText, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		return out
	}
	return nil
}

// parseIntDefault parses a string as an int, returning the default on error
// or empty string.
func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid body")
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}

	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load university space type")
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
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create university")
		return
	}

	var actorID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		actorID = u.UserID
	}
	h.enqueueFetchLogo(r.Context(), tenantID, space.ID, actorID, input.LogoURL)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": space})
}

// UpdateUniversity handles PATCH /admin/universities/:id.
func (h *Handlers) UpdateUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	existing, err := h.spaces.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}

	var input struct {
		Name     *string `json:"name"`
		Slug     *string `json:"slug"`
		Verified *bool   `json:"verified"`
		LogoURL  *string `json:"logo_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid body")
		return
	}

	name := existing.Name
	if input.Name != nil && strings.TrimSpace(*input.Name) != "" {
		name = strings.TrimSpace(*input.Name)
	}

	// U-2 fix: verified=false → "pending" (consistent with CreateUniversity),
	// not "suspended" (which is a moderation/punishment status).
	status := existing.Status
	if input.Verified != nil {
		if *input.Verified {
			status = "active"
		} else {
			status = "pending"
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
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update university")
		return
	}

	// U-4 fix: actually update the slug when a new one is supplied. UpdateSpace
	// does not touch slug (no slug column in its SET clause), so a separate
	// UpdateSpaceSlug call is required. A collision on UNIQUE(tenant_id, slug)
	// surfaces as a 409.
	if input.Slug != nil {
		newSlug := strings.TrimSpace(*input.Slug)
		if newSlug != "" && newSlug != existing.Slug {
			updated, err := h.spaces.UpdateSpaceSlug(r.Context(), id, tenantID, newSlug)
			if err != nil {
				errpkg.WriteError(w, http.StatusConflict, "slug_in_use", "Slug is already in use")
				return
			}
			space.Slug = updated.Slug
			space.UpdatedAt = updated.UpdatedAt
		}
	}

	if input.LogoURL != nil {
		var actorID pgtype.UUID
		if u := auth.FromContext(r.Context()); u != nil {
			actorID = u.UserID
		}
		h.enqueueFetchLogo(r.Context(), tenantID, space.ID, actorID, *input.LogoURL)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": space})
}

// DeleteUniversity handles DELETE /admin/universities/:id.
func (h *Handlers) DeleteUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	if err := h.spaces.DeleteSpace(r.Context(), id, tenantID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete university")
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	existing, err := h.spaces.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "University not found")
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
		errpkg.WriteError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify university")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": space})
}

// UnverifyUniversity handles DELETE /admin/universities/:id/verify — reverses
// a prior verify by setting status back to "pending" (consistent with
// CreateUniversity's unverified status). Ref: Section 39, audit finding U-1.
func (h *Handlers) UnverifyUniversity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	existing, err := h.spaces.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "University not found")
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
		Status:         "pending",
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "unverify_failed", "Failed to unverify university")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": space})
}

// allowedUniversityStatuses is the set of status values the admin may set on a
// university space. The spaces.status column has no CHECK constraint, so this
// allowlist prevents orphan statuses no other code path recognizes (audit U-3).
var allowedUniversityStatuses = map[string]bool{
	"active":    true,
	"pending":   true,
	"suspended": true,
}

// UpdateUniversityStatus handles PATCH /admin/universities/:id/status — sets
// the space status to one of {active, pending, suspended}. Ref: Section 39,
// audit finding U-1.
func (h *Handlers) UpdateUniversityStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid university ID")
		return
	}

	var input struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid body")
		return
	}
	input.Status = strings.TrimSpace(input.Status)
	if !allowedUniversityStatuses[input.Status] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status",
			"Status must be one of: active, pending, suspended")
		return
	}

	existing, err := h.spaces.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "University not found")
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
		Status:         input.Status,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update status")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": space})
}

// BulkImportUniversities handles POST /admin/universities/bulk-import —
// accepts a CSV upload (columns: name, slug, logo_url) and creates a
// university space per row. Idempotent per row: a duplicate slug is skipped
// (not an error) so re-importing the same file is safe. Returns a summary of
// created / skipped / failed counts. Ref: Section 39, audit finding U-1.
func (h *Handlers) BulkImportUniversities(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// 10 MB upload cap — matches media.max_upload_mb default for a CSV.
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_upload", "Failed to parse upload (max 10 MB)")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_file", "CSV file is required (field name: file)")
		return
	}
	defer file.Close()

	spaceType, err := h.spaces.GetSpaceTypeBySlug(r.Context(), universitySpaceTypeSlug, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve university space type")
		return
	}

	var actorID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		actorID = u.UserID
	}

	created, skipped, failed := 0, 0, 0
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // tolerate ragged rows
	header, err := reader.Read()
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_csv", "CSV is empty or missing a header row")
		return
	}
	colIndex := make(map[string]int, len(header))
	for i, h := range header {
		colIndex[strings.ToLower(strings.TrimSpace(h))] = i
	}
	nameCol, nameOK := colIndex["name"]
	slugCol, slugOK := colIndex["slug"]
	logoCol, logoOK := colIndex["logo_url"]

	if !nameOK {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_csv", "CSV must have a 'name' column")
		return
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			failed++
			continue
		}
		name := strings.TrimSpace(row[nameCol])
		if name == "" {
			failed++
			continue
		}
		slug := ""
		if slugOK && slugCol < len(row) {
			slug = strings.TrimSpace(row[slugCol])
		}
		if slug == "" {
			slug = slugify(name)
		}
		logoURL := ""
		if logoOK && logoCol < len(row) {
			logoURL = strings.TrimSpace(row[logoCol])
		}

		space, err := h.spaces.CreateSpace(r.Context(), gen.CreateSpaceParams{
			TenantID:       tenantID,
			SpaceTypeID:    spaceType.ID,
			Name:           name,
			Slug:           slug,
			Level:          "custom",
			Visibility:     "public",
			PostingPerm:    "members",
			ReplyingPerm:   "members",
			CommentingPerm: "members",
			JoiningPerm:    "open",
			Status:         "pending",
		})
		if err != nil {
			// Duplicate slug (UNIQUE violation) → skip, not a failure.
			skipped++
			continue
		}
		if logoURL != "" {
			h.enqueueFetchLogo(r.Context(), tenantID, space.ID, actorID, logoURL)
		}
		created++
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"created": created,
			"skipped": skipped,
			"failed":  failed,
		},
	})
}

// CompareUniversities handles GET /universities/{slug}/compare — redirects to
// the comparison picker at /compare/universities with this university
// pre-selected as Item 1 (Section 39 line 152: "comparison matrix link
// (redirects to /comparisons/universities)"). Ref: audit finding U-5.
func (h *Handlers) CompareUniversities(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "University slug is required")
		return
	}
	// Verify the university exists so a bad slug 404s rather than redirecting
	// to a picker with a dead pre-selection.
	if _, err := h.spaces.GetSpaceBySlug(r.Context(), slug, tenantID); err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "University not found")
		return
	}
	target := "/compare/universities?item1_slug=" + url.QueryEscape(slug)
	http.Redirect(w, r, target, http.StatusSeeOther)
}
