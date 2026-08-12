package tags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20
const maxPageSize int32 = 100

// PermCacheInvalidator flushes a user's permission cache (jc:perm:{user_id})
// after a permission-affecting mutation. Implemented by rbac.RBAC.
// Ref: AGENTS.md Rule 6, sections/03-rbac.md (Permission Cache Invalidation).
type PermCacheInvalidator interface {
	InvalidateUserCache(ctx context.Context, userID pgtype.UUID)
}

// Handlers holds dependencies for tag HTTP handlers. Composes the existing
// content and follows services directly (no import cycle: neither imports
// this package) rather than through an adapter interface.
type Handlers struct {
	service *Service
	content *content.Service
	follows *follows.Service
	permCache PermCacheInvalidator
}

// NewHandlers creates Handlers with the given services.
func NewHandlers(service *Service, contentService *content.Service, followsService *follows.Service) *Handlers {
	return &Handlers{service: service, content: contentService, follows: followsService}
}

// SetPermCacheInvalidator injects the RBAC resolver so handlers can flush
// the TARGET user's permission cache after tag moderator assign/remove
// (Rule 6). The middleware-level RequirePermissionWithInvalidation only
// flushes the CALLER's cache; the target user's cache must be flushed here.
func (h *Handlers) SetPermCacheInvalidator(p PermCacheInvalidator) {
	h.permCache = p
}

// invalidateTargetPermCache flushes the target user's permission cache after
// a tag moderator assignment/removal. It is a no-op when no invalidator is
// wired (graceful degradation). Errors are logged but never fail the
// request — the mutation has already succeeded and the cache will expire
// via TTL.
func (h *Handlers) invalidateTargetPermCache(ctx context.Context, targetUserID pgtype.UUID) {
	if h.permCache == nil {
		return
	}
	h.permCache.InvalidateUserCache(ctx, targetUserID)
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

// parseUUIDParam reads a UUID URL param, writing a 400 and returning the zero
// value on failure.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}
	return id, nil
}

// resolveTag looks up a tag by slug, writing a 404 and returning ok=false if
// it doesn't exist. Shared by every handler below GetTag.
func (h *Handlers) resolveTag(w http.ResponseWriter, r *http.Request, tenantID pgtype.UUID) (gen.GetTagBySlugRow, bool) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Tag slug is required")
		return gen.GetTagBySlugRow{}, false
	}

	tag, err := h.service.GetTagBySlug(r.Context(), tenantID, slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Tag not found")
		return gen.GetTagBySlugRow{}, false
	}

	return tag, true
}

// ListTags handles GET /api/v1/tags — public list with ?q, ?sort, ?page, ?page_size.
// Also handles GET /api/v1/admin/tags — admin list with pagination.
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query().Get("q")
	sortParam := r.URL.Query().Get("sort")
	if sortParam == "" {
		sortParam = "newest"
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	// Check if this is the admin route (has page/limit params from DataTable)
	// vs the public route (uses page_size/page_offset).
	// Both use the same handler; admin route adds min_content_count filter.
	minContentCount := int32(0)
	if mcc := r.URL.Query().Get("min_content_count"); mcc != "" {
		if v, err := strconv.ParseInt(mcc, 10, 32); err == nil && v > 0 {
			minContentCount = int32(v)
		}
	}

	// Use admin-listing query when filter or sort is specified, otherwise
	// the admin route also benefits from pagination.
	tags, err := h.service.AdminListTags(r.Context(), gen.AdminListTagsParams{
		TenantID:        tenantID,
		Q:               q,
		MinContentCount: minContentCount,
		Sort:            sortParam,
		PageOffset:      offset,
		PageLimit:       limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tags")
		return
	}

	count, err := h.service.AdminCountTags(r.Context(), gen.AdminCountTagsParams{
		TenantID:        tenantID,
		Q:               q,
		MinContentCount: minContentCount,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count tags")
		return
	}

	totalPages := int32(1)
	if limit > 0 {
		totalPages = (int32(count) + limit - 1) / limit
	}

	// Enrich each tag with resolved logo_url and cover_url from the joined media columns.
	enriched := make([]map[string]any, 0, len(tags))
	for _, t := range tags {
		row := structToMap(t)
		row["logo_url"] = resolveLogoURL(t.LogoStoragePath, t.LogoStorageBucket)
		row["cover_url"] = resolveLogoURL(t.CoverStoragePath, t.CoverStorageBucket)
		enriched = append(enriched, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":        enriched,
		"total":       count,
		"page":        offset / limit,
		"total_pages": totalPages,
	})
}

// GetTag handles GET /api/v1/tags/{slug}. [public]
func (h *Handlers) GetTag(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tag, ok := h.resolveTag(w, r, tenantID)
	if !ok {
		return
	}

	row := structToMap(tag)
	row["logo_url"] = resolveLogoURL(tag.LogoStoragePath, tag.LogoStorageBucket)
	row["cover_url"] = resolveLogoURL(tag.CoverStoragePath, tag.CoverStorageBucket)
	writeJSON(w, http.StatusOK, map[string]any{"data": row})
}

// GetTagContent handles GET /api/v1/tags/{slug}/content — published content
// tagged with this tag, newest first. Supports ?cursor= (RFC3339 timestamp)
// and ?page_size= for pagination. [public]
func (h *Handlers) GetTagContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tag, ok := h.resolveTag(w, r, tenantID)
	if !ok {
		return
	}

	var cursor pgtype.Timestamptz
	if c := r.URL.Query().Get("cursor"); c != "" {
		t, err := time.Parse(time.RFC3339, c)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor must be an RFC3339 timestamp")
			return
		}

		cursor = pgtype.Timestamptz{Time: t, Valid: true}
	}

	limit := parsePageSize(r)

	items, err := h.content.ListContentByTag(r.Context(), tenantID, tag.ID, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tagged content")
		return
	}

	// Enrich each item with featured_image_url resolved from the media JOIN
	// so the frontend card can render the cover image without a second
	// round-trip to /media/{id}.
	result := make([]map[string]any, 0, len(items))
	for _, c := range items {
		row := map[string]any{
			"id":                  c.ID,
			"tenant_id":           c.TenantID,
			"author_id":           c.AuthorID,
			"space_id":            c.SpaceID,
			"content_type":        c.ContentType,
			"title":               c.Title,
			"slug":                c.Slug,
			"body":                c.Body,
			"body_html":           c.BodyHtml,
			"field_values":        c.FieldValues,
			"content_hash":        c.ContentHash,
			"geo_space_id":        c.GeoSpaceID,
			"header_image_id":     c.HeaderImageID,
			"featured_image_id":   c.FeaturedImageID,
			"canonical_url":       c.CanonicalUrl,
			"meta_title":          c.MetaTitle,
			"meta_description":    c.MetaDescription,
			"og_image_url":        c.OgImageUrl,
			"seo_indexable":       c.SeoIndexable,
			"status":              c.Status,
			"visibility":          c.Visibility,
			"is_locked":           c.IsLocked,
			"is_pinned":           c.IsPinned,
			"read_time_minutes":   c.ReadTimeMinutes,
			"view_count":          c.ViewCount,
			"comment_count":       c.CommentCount,
			"share_count":         c.ShareCount,
			"reshare_count":       c.ReshareCount,
			"publish_at":          c.PublishAt,
			"published_at":        c.PublishedAt,
			"created_at":          c.CreatedAt,
			"updated_at":          c.UpdatedAt,
			"featured_image_url":  "",
		}
		if c.CoverStorageBucket != nil && c.CoverStoragePath != nil &&
			*c.CoverStorageBucket != "" && *c.CoverStoragePath != "" {
			row["featured_image_url"] = media.MediaURL(*c.CoverStorageBucket, *c.CoverStoragePath, media.ImageProxyOpts{})
		}
		result = append(result, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ToggleTagFollow handles POST/DELETE /api/v1/tags/{slug}/follow.
func (h *Handlers) ToggleTagFollow(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tag, ok := h.resolveTag(w, r, tenantID)
	if !ok {
		return
	}

	following, err := h.follows.ToggleFollow(r.Context(), tenantID, user.UserID, follows.EntityTypeTag, tag.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to toggle tag follow")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"following": following}})
}

// GetTagFollowers handles GET /api/v1/tags/{slug}/followers.
func (h *Handlers) GetTagFollowers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tag, ok := h.resolveTag(w, r, tenantID)
	if !ok {
		return
	}

	// The caller's user ID is needed to resolve is_following per item
	// (Section 31). Best-effort: when absent, is_following defaults to false.
	var callerID pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil {
		callerID = user.UserID
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	followers, err := h.follows.GetFollowers(r.Context(), tenantID, callerID, follows.EntityTypeTag, tag.ID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get tag followers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": followers})
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		s = r.URL.Query().Get("limit")
	}
	if s == "" {
		return defaultPageSize
	}

	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v <= 0 || v > int64(maxPageSize) {
		return defaultPageSize
	}

	return int32(v)
}

// parsePageOffset extracts the page_offset query parameter.
func parsePageOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		// Support page-based offset: page=N → offset = N * page_size
		if ps := r.URL.Query().Get("page"); ps != "" {
			if p, err := strconv.ParseInt(ps, 10, 32); err == nil && p >= 0 {
				return int32(p) * parsePageSize(r)
			}
		}
		return 0
	}

	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v < 0 {
		return 0
	}

	return int32(v)
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

// resolveLogoURL constructs a serve URL from the media storage path/bucket
// returned by the LEFT JOIN in the tag queries. Returns empty string when
// the tag has no logo.
func resolveLogoURL(storagePath, storageBucket *string) string {
	if storagePath == nil || *storagePath == "" {
		return ""
	}
	bucket := "media"
	if storageBucket != nil && *storageBucket != "" {
		bucket = *storageBucket
	}
	return media.MediaURL(bucket, *storagePath, media.ImageProxyOpts{})
}

// structToMap converts a struct with json tags to a map[string]any via JSON
// round-trip. Used to enrich tag responses with computed fields like logo_url
// without exposing internal join columns (logo_storage_path/bucket).
func structToMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{}
	}
	// Remove internal join columns — the API exposes logo_url/cover_url, not raw storage paths.
	delete(m, "logo_storage_path")
	delete(m, "logo_storage_bucket")
	delete(m, "cover_storage_path")
	delete(m, "cover_storage_bucket")
	return m
}

// CreateTag handles POST /api/v1/admin/tags.
func (h *Handlers) CreateTag(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name         string  `json:"name"`
		Slug         string  `json:"slug"`
		Description  *string `json:"description"`
		Rules        *string `json:"rules"`
		CoverMediaID string  `json:"cover_media_id"`
		Color        *string `json:"color"`
		IsSystem     bool            `json:"is_system"`
		LogoMediaID  string          `json:"logo_media_id"`
		FieldValues  json.RawMessage `json:"field_values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	// Always slugify: use the provided slug if non-empty, else derive from name.
	// Either way, the result is normalized to a URL-safe slug.
	slug := content.Slugify(input.Slug)
	if slug == "" {
		slug = content.Slugify(input.Name)
	}

	if input.Description != nil {
		sanitized := content.SanitizeText(*input.Description)
		input.Description = &sanitized
	}

	if input.Rules != nil {
		sanitized := content.SanitizeText(*input.Rules)
		input.Rules = &sanitized
	}

	// Validate color format if provided (must be a valid hex color).
	if input.Color != nil && *input.Color != "" {
		if !isValidHexColor(*input.Color) {
			writeError(w, http.StatusBadRequest, "invalid_color", "color must be a valid hex color (e.g. #3b82f6)")
			return
		}
	}

	var logoMediaID pgtype.UUID
	if input.LogoMediaID != "" {
		if err := logoMediaID.Scan(input.LogoMediaID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_logo_media_id", "logo_media_id must be a valid UUID")
			return
		}
	}

	var coverMediaID pgtype.UUID
	if input.CoverMediaID != "" {
		if err := coverMediaID.Scan(input.CoverMediaID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cover_media_id", "cover_media_id must be a valid UUID")
			return
		}
	}

	tag, err := h.service.CreateTag(r.Context(), tenantID, CreateTagInput{
		Name:         content.SanitizeText(input.Name),
		Slug:         slug,
		Description:  input.Description,
		Rules:        input.Rules,
		CoverMediaID: coverMediaID,
		Color:        input.Color,
		IsSystem:     input.IsSystem,
		LogoMediaID:  logoMediaID,
		FieldValues:  input.FieldValues,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "duplicate_slug", "A tag with this slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create tag")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": tag})
}

// UpdateTag handles PATCH /api/v1/admin/tags/{id}.
func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Name         string          `json:"name"`
		Slug         string          `json:"slug"`
		Description  *string         `json:"description"`
		Rules        *string         `json:"rules"`
		CoverMediaID *string         `json:"cover_media_id"`
		Color        *string         `json:"color"`
		LogoMediaID  *string         `json:"logo_media_id"`
		IsSystem     *bool           `json:"is_system"`
		FieldValues  json.RawMessage `json:"field_values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	// Always slugify: use the provided slug if non-empty, else derive from name.
	// Either way, the result is normalized to a URL-safe slug.
	slug := content.Slugify(input.Slug)
	if slug == "" {
		slug = content.Slugify(input.Name)
	}

	if input.Description != nil {
		sanitized := content.SanitizeText(*input.Description)
		input.Description = &sanitized
	}

	if input.Rules != nil {
		sanitized := content.SanitizeText(*input.Rules)
		input.Rules = &sanitized
	}

	if input.Color != nil && *input.Color != "" {
		if !isValidHexColor(*input.Color) {
			writeError(w, http.StatusBadRequest, "invalid_color", "color must be a valid hex color (e.g. #3b82f6)")
			return
		}
	}

	var logoMediaID pgtype.UUID
	if input.LogoMediaID != nil && *input.LogoMediaID != "" {
		if err := logoMediaID.Scan(*input.LogoMediaID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_logo_media_id", "logo_media_id must be a valid UUID")
			return
		}
	}

	var coverMediaID pgtype.UUID
	if input.CoverMediaID != nil && *input.CoverMediaID != "" {
		if err := coverMediaID.Scan(*input.CoverMediaID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cover_media_id", "cover_media_id must be a valid UUID")
			return
		}
	}

	// Preserve the existing is_system value when the client omits the field.
	var isSystem bool
	if input.IsSystem != nil {
		isSystem = *input.IsSystem
	} else {
		existing, err := h.service.GetTagByID(r.Context(), id, tenantID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Tag not found")
			return
		}
		isSystem = existing.IsSystem
	}

	tag, err := h.service.UpdateTag(r.Context(), id, tenantID, UpdateTagInput{
		Name:         content.SanitizeText(input.Name),
		Slug:         slug,
		Description:  input.Description,
		Rules:        input.Rules,
		CoverMediaID: coverMediaID,
		Color:        input.Color,
		LogoMediaID:  logoMediaID,
		IsSystem:     isSystem,
		FieldValues:  input.FieldValues,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "duplicate_slug", "A tag with this slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update tag")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tag})
}

// DeleteTag handles DELETE /api/v1/admin/tags/{id}.
// Spec: "delete tag (only if no content tagged)" and system tags cannot be deleted.
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	// Guard 1: Check if tag is a system tag (cannot be deleted).
	tag, err := h.service.GetTagByID(r.Context(), id, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Tag not found")
		return
	}
	if tag.IsSystem {
		writeError(w, http.StatusForbidden, "system_tag_protected", "System tags cannot be deleted")
		return
	}

	// Guard 2: Check if any content is tagged with this tag.
	contentCount, err := h.service.CountContentByTag(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to check tag usage")
		return
	}
	if contentCount > 0 {
		writeError(w, http.StatusConflict, "tag_in_use", "Tag is in use by content and cannot be deleted")
		return
	}

	if err := h.service.DeleteTag(r.Context(), id, tenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete tag")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// isValidHexColor checks if a string is a valid CSS hex color (#RGB or #RRGGBB).
func isValidHexColor(s string) bool {
	if len(s) == 0 || s[0] != '#' {
		return false
	}
	hex := s[1:]
	if len(hex) != 3 && len(hex) != 6 {
		return false
	}
	for _, c := range hex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ListTagModerators handles GET /api/v1/admin/tags/:id/moderators.
func (h *Handlers) ListTagModerators(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tagID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	rows, err := h.service.ListTagModerators(r.Context(), tagID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tag moderators")
		return
	}

	moderators := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		moderators = append(moderators, map[string]any{
			"user_id":  row.UserID,
			"username": row.Username,
			"added_at": row.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": moderators})
}

// AddTagModerator handles POST /api/v1/admin/tags/:id/moderators (Section 49).
func (h *Handlers) AddTagModerator(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tagID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	var input struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var userID pgtype.UUID
	switch {
	case input.UserID != "":
		if err := userID.Scan(input.UserID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
			return
		}
	case input.Username != "":
		userID, err = h.service.GetUserIDByUsername(r.Context(), tenantID, input.Username)
		if err != nil {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "missing_fields", "user_id or username is required")
		return
	}

	row, err := h.service.AddTagModerator(r.Context(), AddTagModeratorInput{
		TagID:            tagID,
		UserID:           userID,
		TenantID:         tenantID,
		AssignedBy:       "site_admin",
		AssignedByUserID: user.UserID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to add tag moderator")
		return
	}

	// Rule 6: invalidate the TARGET user's permission cache so the newly
	// assigned tag moderator's permissions are visible immediately.
	h.invalidateTargetPermCache(r.Context(), userID)

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"tag_id":   row.TagID,
		"user_id":  row.UserID,
		"added_at": row.CreatedAt,
	}})
}

// RemoveTagModerator handles DELETE /api/v1/admin/tags/:id/moderators/:userId (Section 49).
func (h *Handlers) RemoveTagModerator(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tagID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "userId")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
		return
	}

	if err := h.service.RemoveTagModerator(r.Context(), tagID, userID, tenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove tag moderator")
		return
	}

	// Rule 6: invalidate the removed moderator's permission cache so their
	// tag moderation permissions are revoked immediately.
	h.invalidateTargetPermCache(r.Context(), userID)

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"removed": true}})
}
