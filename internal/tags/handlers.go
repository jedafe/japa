package tags

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/worker"
)

const (
	defaultPageSize int32 = 20
	maxPageSize     int32 = 100
)

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
	service     *Service
	content     *content.Service
	follows     *follows.Service
	permCache   PermCacheInvalidator
	asynqClient *asynq.Client // nil disables search index enqueue (test-only)
	fieldDefs   TagFieldDefLister
}

// TagFieldDefLister supplies custom field definitions for the tag entity
// class so GET /tags/{slug} can emit field labels to guests — the
// /custom-field-groups endpoint is auth-gated. Satisfied by
// customfields.Service. The guest can_view lister narrows to guest-visible
// fields so the defs never advertise admin-only fields (CF9).
type TagFieldDefLister interface {
	ListFieldGroupsByEntityClass(ctx context.Context, entityClass string) ([]gen.CustomFieldGroup, error)
	ListFieldsByGroup(ctx context.Context, groupID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error)
	ListVisibleFieldIDsForGroupSlug(ctx context.Context, groupSlug string) ([]pgtype.UUID, error)
}

// NewHandlers creates Handlers with the given services.
func NewHandlers(service *Service, contentService *content.Service, followsService *follows.Service) *Handlers {
	return &Handlers{service: service, content: contentService, follows: followsService}
}

// SetAsynqClient injects the Asynq client so tag CRUD can enqueue
// search:index_entity tasks (§12 stale-index fix, entity_type
// "directory_tag"). nil disables enqueue (test-only).
func (h *Handlers) SetAsynqClient(c *asynq.Client) { h.asynqClient = c }

// SetFieldDefLister injects the custom-fields service so tag responses can
// embed field definitions for the "tag" entity class.
func (h *Handlers) SetFieldDefLister(l TagFieldDefLister) { h.fieldDefs = l }

// enqueueSearchIndex enqueues a Meilisearch index sync for a directory tag
// (entity_type "directory_tag" — the /tags surface). operation is "index"
// (create/update) or "delete". Fire-and-forget per Rule 8.
func (h *Handlers) enqueueSearchIndex(operation string, tagID pgtype.UUID) {
	if h.asynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"entity_id":   tagID.String(),
		"entity_type": "directory_tag",

		"operation": operation,
	})
	if err != nil {
		slog.Warn("tags: failed to marshal search index payload", "error", err)
		return
	}

	if _, err := h.asynqClient.Enqueue(
		asynq.NewTask(worker.TypeSearchIndexSync, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeSearchIndexSync)),
	); err != nil {
		slog.Warn("tags: failed to enqueue search index task", "entity_id", tagID.String(), "error", err)
	}
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

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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
func (h *Handlers) resolveTag(w http.ResponseWriter, r *http.Request) (gen.GetTagBySlugRow, bool) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Tag slug is required")
		return gen.GetTagBySlugRow{}, false
	}

	tag, err := h.service.GetTagBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Tag not found")
		} else {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load tag")
		}

		return gen.GetTagBySlugRow{}, false
	}

	return tag, true
}

// ListTags handles GET /api/v1/tags — public list with ?q, ?sort, ?page, ?page_size.
// Also handles GET /api/v1/admin/tags — admin list with pagination.
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
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
		Q:               q,
		MinContentCount: minContentCount,
		Sort:            sortParam,
		PageOffset:      offset,
		PageLimit:       limit,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list tags")
		return
	}

	count, err := h.service.AdminCountTags(r.Context(), gen.AdminCountTagsParams{
		Q:               q,
		MinContentCount: minContentCount,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count tags")
		return
	}

	totalPages := int32(1)
	if limit > 0 {
		// #nosec G115 // value bounded by caller
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        enriched,
		"total":       count,
		"page":        offset / limit, // 0-based page index (offset is always a multiple of limit from parsePageOffset)
		"total_pages": totalPages,
	})
}

// GetTag handles GET /api/v1/tags/{slug}. [public].
func (h *Handlers) GetTag(w http.ResponseWriter, r *http.Request) {
	tag, ok := h.resolveTag(w, r)
	if !ok {
		return
	}

	row := structToMap(tag)
	row["logo_url"] = resolveLogoURL(tag.LogoStoragePath, tag.LogoStorageBucket)
	row["cover_url"] = resolveLogoURL(tag.CoverStoragePath, tag.CoverStorageBucket)
	// Embed the tag entity-class field definitions so guests can render
	// labeled field_values — /custom-field-groups is auth-gated. Best-effort:
	// nil lister or an error degrades to an empty array.
	row["field_defs"] = h.tagFieldDefs(r.Context())
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": row})
}

// tagFieldDef is the field-definition shape emitted in GET /tags/{slug}.
type tagFieldDef struct {
	Key            string  `json:"key"`
	Label          string  `json:"label"`
	FieldType      string  `json:"field_type"`
	Placeholder    *string `json:"placeholder,omitempty"`
	DefaultValue   *string `json:"default_value,omitempty"`
	IsRequired     bool    `json:"is_required"`
	ShowInFrontend bool    `json:"show_in_frontend"`
}

// tagFieldDefs loads the custom field definitions assigned to the "tag"
// entity class (CF9): only fields the guest group can_view AND flagged
// show_in_frontend are emitted — guests must never see admin-only defs.
// Deduplicated by key (multiple groups may define the same key — the page's
// keyed each block would break). Returns an empty slice on failure or when
// no lister is wired.
func (h *Handlers) tagFieldDefs(ctx context.Context) []tagFieldDef {
	defs := make([]tagFieldDef, 0)
	if h.fieldDefs == nil {
		return defs
	}

	// CF9: the guest group's can_view grants bound the def list. A lookup
	// error fails closed to an empty set (no defs leak on outage).
	guestVisible := map[string]bool{}

	if ids, err := h.fieldDefs.ListVisibleFieldIDsForGroupSlug(ctx, "guest"); err != nil {
		slog.Warn("tags: failed to load guest-visible field ids", "error", err)
	} else {
		for _, id := range ids {
			guestVisible[id.String()] = true
		}
	}

	groups, err := h.fieldDefs.ListFieldGroupsByEntityClass(ctx, "tag")
	if err != nil {
		slog.Warn("tags: failed to load tag field groups", "error", err)
		return defs
	}

	seen := make(map[string]bool)

	for _, g := range groups {
		fields, err := h.fieldDefs.ListFieldsByGroup(ctx, g.ID)
		if err != nil {
			slog.Warn("tags: failed to load tag field group fields", "group_id", g.ID, "error", err)
			continue
		}

		for _, f := range fields {
			if f.Key == "" || seen[f.Key] {
				continue
			}
			// CF9: guest can_view + show_in_frontend both required. When the
			// guest set is empty (outage or no grants) nothing is emitted.
			if !guestVisible[f.ID.String()] || !f.ShowInFrontend {
				continue
			}

			seen[f.Key] = true

			defs = append(defs, tagFieldDef{
				Key:            f.Key,
				Label:          f.Label,
				FieldType:      f.FieldType,
				Placeholder:    f.Placeholder,
				DefaultValue:   f.DefaultValue,
				IsRequired:     f.IsRequired,
				ShowInFrontend: f.ShowInFrontend,
			})
		}
	}

	return defs
}

// GetTagContent handles GET /api/v1/tags/{slug}/content — published content
// tagged with this tag, newest first. Supports ?cursor= (RFC3339 timestamp)
// and ?cursor_id= (UUID) for composite cursor pagination, plus ?page_size=.
// The composite (created_at, id) cursor avoids skipping items that share the
// same created_at timestamp. [public].
func (h *Handlers) GetTagContent(w http.ResponseWriter, r *http.Request) {
	tag, ok := h.resolveTag(w, r)
	if !ok {
		return
	}

	var cursor pgtype.Timestamptz

	var cursorID pgtype.UUID

	if c := r.URL.Query().Get("cursor"); c != "" {
		t, err := time.Parse(time.RFC3339, c)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_cursor", "cursor must be an RFC3339 timestamp")
			return
		}

		cursor = pgtype.Timestamptz{Time: t, Valid: true}

		// cursor_id is required when cursor is set — it's the id of the
		// last item on the previous page, used as the tiebreaker.
		if cid := r.URL.Query().Get("cursor_id"); cid != "" {
			if err := cursorID.Scan(cid); err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_cursor_id", "cursor_id must be a UUID")
				return
			}
		}
	}

	limit := parsePageSize(r)

	filter := visibility.VisibilityFilterFromRequest(r)

	items, err := h.content.ListContentByTag(r.Context(), tag.ID, cursor, cursorID, limit, filter)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list tagged content")
		return
	}

	// Resolve template literals ({{...}}) in item titles/bodies — same
	// best-effort resolution the content list handler applies.
	rc := literals.NewRequestContext(r, "")
	if resErr := h.content.ResolveContentFields(r.Context(), items, rc); resErr != nil {
		slog.Warn("tags: failed to resolve item literals", "error", resErr)
	}

	// Enrich each item with featured_image_url resolved from the media JOIN
	// so the frontend card can render the cover image without a second
	// round-trip to /media/{id}.
	result := make([]map[string]any, 0, len(items))

	for _, c := range items {
		// Forum-style bodies are TipTap JSON — LEFT(body, 200) would leak raw
		// JSON into the card excerpt, so derive plain text instead.
		excerpt := c.Excerpt
		if excerpt != "" && (excerpt[0] == '[' || excerpt[0] == '{') {
			if text, ok := content.ExtractTipTapText([]byte(c.Body)); ok && text != "" {
				excerpt = text
				if len([]rune(excerpt)) > 200 {
					excerpt = string([]rune(excerpt)[:200]) + "…"
				}
			}
		}
		// Stored excerpts can carry raw HTML (e.g. imported feed summaries) —
		// strip tags so cards render plain text, not markup.
		if strings.ContainsRune(excerpt, '<') {
			excerpt = content.MakeExcerpt(excerpt, 200)
		}

		row := map[string]any{
			"id":                 c.ID,
			"author_id":          c.AuthorID,
			"space_id":           c.SpaceID,
			"content_type":       c.ContentType,
			"title":              c.Title,
			"slug":               c.Slug,
			"body":               c.Body,
			"body_html":          c.BodyHtml,
			"field_values":       jsonField(c.FieldValues),
			"author_name":        c.AuthorName,
			"author_avatar_url":  c.AuthorAvatarUrl,
			"space_name":         c.SpaceName,
			"space_slug":         c.SpaceSlug,
			"excerpt":            excerpt,
			"tags":               splitTagNames(c.Tags),
			"content_hash":       c.ContentHash,
			"geo_space_id":       c.GeoSpaceID,
			"header_image_id":    c.HeaderImageID,
			"featured_image_id":  c.FeaturedImageID,
			"canonical_url":      c.CanonicalUrl,
			"meta_title":         c.MetaTitle,
			"meta_description":   c.MetaDescription,
			"og_image_url":       c.OgImageUrl,
			"seo_indexable":      c.SeoIndexable,
			"status":             c.Status,
			"visibility":         c.Visibility,
			"is_locked":          c.IsLocked,
			"is_pinned":          c.IsPinned,
			"read_time_minutes":  c.ReadTimeMinutes,
			"view_count":         c.ViewCount,
			"comment_count":      c.CommentCount,
			"share_count":        c.ShareCount,
			"reshare_count":      c.ReshareCount,
			"publish_at":         c.PublishAt,
			"published_at":       c.PublishedAt,
			"created_at":         c.CreatedAt,
			"updated_at":         c.UpdatedAt,
			"featured_image_url": "",
		}
		if c.CoverStorageBucket != nil && c.CoverStoragePath != nil &&
			*c.CoverStorageBucket != "" && *c.CoverStoragePath != "" {
			row["featured_image_url"] = media.MediaURL(*c.CoverStorageBucket, *c.CoverStoragePath, media.ImageProxyOpts{})
		}

		result = append(result, row)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ToggleTagFollow handles POST/DELETE /api/v1/tags/{slug}/follow.
func (h *Handlers) ToggleTagFollow(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tag, ok := h.resolveTag(w, r)
	if !ok {
		return
	}

	// POST = ensure-follow, DELETE = ensure-unfollow (method semantics
	// per api-routes-catalog); a raw toggle inverts state on repeats.
	var following bool

	var err error
	if r.Method == http.MethodDelete {
		following, err = h.follows.EnsureUnfollow(r.Context(), user.UserID, follows.EntityTypeTag, tag.ID)
	} else {
		following, err = h.follows.EnsureFollow(r.Context(), user.UserID, follows.EntityTypeTag, tag.ID)
	}
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to toggle tag follow")
		return
	}

	fc, _ := h.follows.CountFollowers(r.Context(), follows.EntityTypeTag, tag.ID)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"following": following, "follower_count": fc}})
}

// GetTagFollowers handles GET /api/v1/tags/{slug}/followers.
func (h *Handlers) GetTagFollowers(w http.ResponseWriter, r *http.Request) {
	tag, ok := h.resolveTag(w, r)
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

	followers, err := h.follows.GetFollowers(r.Context(), callerID, follows.EntityTypeTag, tag.ID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get tag followers")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": followers})
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
		// Support page-based offset: page=N → offset = (N-1) * page_size.
		// The JEOS route catalog documents `page=1` as the first page
		// (1-based), so page=1 maps to offset 0.
		if ps := r.URL.Query().Get("page"); ps != "" {
			if p, err := strconv.ParseInt(ps, 10, 32); err == nil && p >= 1 {
				return int32(p-1) * parsePageSize(r) // #nosec
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
	// field_values is a jsonb column sqlc types as []byte; the JSON round-trip
	// above emits it as a base64 string instead of the stored object. Decode
	// it back so API consumers receive the actual JSON object.
	if s, ok := m["field_values"].(string); ok {
		if raw, derr := base64.StdEncoding.DecodeString(s); derr == nil {
			var fv any
			if json.Unmarshal(raw, &fv) == nil {
				m["field_values"] = fv
			}
		}
	}

	return m
}

// jsonField decodes a jsonb []byte column into its JSON value so the API
// emits an object rather than a base64 string.
func jsonField(b []byte) any {
	if len(b) == 0 {
		return nil
	}

	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}

	return v
}

// splitTagNames converts the comma-separated string_agg tag list into a
// string slice for the feed card contract (same shape as the feed handler).
func splitTagNames(v any) []string {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}

	return strings.Split(s, ",")
}

// CreateTag handles POST /api/v1/admin/tags.
func (h *Handlers) CreateTag(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string          `json:"name"`
		Slug         string          `json:"slug"`
		Description  *string         `json:"description"`
		Rules        *string         `json:"rules"`
		CoverMediaID string          `json:"cover_media_id"`
		Color        *string         `json:"color"`
		IsSystem     bool            `json:"is_system"`
		LogoMediaID  string          `json:"logo_media_id"`
		FieldValues  json.RawMessage `json:"field_values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	// Always slugify: use the provided slug if non-empty, else derive from
	// name. Check the raw input, not the slugified result — Slugify("")
	// returns "untitled", not "", so a blank slug would never fall through.
	slug := content.Slugify(input.Slug)
	if strings.TrimSpace(input.Slug) == "" {
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
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_color", "color must be a valid hex color (e.g. #3b82f6)")
			return
		}
	}

	var logoMediaID pgtype.UUID
	if input.LogoMediaID != "" {
		if err := logoMediaID.Scan(input.LogoMediaID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_logo_media_id", "logo_media_id must be a valid UUID")
			return
		}
	}

	var coverMediaID pgtype.UUID
	if input.CoverMediaID != "" {
		if err := coverMediaID.Scan(input.CoverMediaID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_cover_media_id", "cover_media_id must be a valid UUID")
			return
		}
	}

	tag, err := h.service.CreateTag(r.Context(), CreateTagInput{
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
			errpkg.WriteError(w, http.StatusConflict, "duplicate_slug", "A tag with this slug already exists")
			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create tag")

		return
	}

	// Index the new directory tag for /search?type=tag (§12 stale-index fix).
	h.enqueueSearchIndex("index", tag.ID)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": structToMap(tag)})
}

// UpdateTag handles PATCH /api/v1/admin/tags/{id}.
func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	// Always slugify: use the provided slug if non-empty, else derive from
	// name. Check the raw input, not the slugified result — Slugify("")
	// returns "untitled", not "", so a blank slug would never fall through.
	slug := content.Slugify(input.Slug)
	if strings.TrimSpace(input.Slug) == "" {
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
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_color", "color must be a valid hex color (e.g. #3b82f6)")
			return
		}
	}

	var logoMediaID pgtype.UUID
	if input.LogoMediaID != nil && *input.LogoMediaID != "" {
		if err := logoMediaID.Scan(*input.LogoMediaID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_logo_media_id", "logo_media_id must be a valid UUID")
			return
		}
	}

	var coverMediaID pgtype.UUID
	if input.CoverMediaID != nil && *input.CoverMediaID != "" {
		if err := coverMediaID.Scan(*input.CoverMediaID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_cover_media_id", "cover_media_id must be a valid UUID")
			return
		}
	}

	// Preserve the existing is_system and field_values when the client omits
	// them — a PATCH that leaves a key out must not clear it (field_values is
	// otherwise overwritten with {} and is_system with false). The row is
	// fetched once when either guarded field is absent.
	var existing gen.GetTagByIDRow

	if input.IsSystem == nil || len(input.FieldValues) == 0 || string(input.FieldValues) == "null" {
		e, err := h.service.GetTagByID(r.Context(), id)
		if err != nil {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Tag not found")
			return
		}

		existing = e
	}

	var isSystem bool
	if input.IsSystem != nil {
		isSystem = *input.IsSystem
	} else {
		isSystem = existing.IsSystem
	}

	fieldValues := input.FieldValues
	if len(fieldValues) == 0 || string(fieldValues) == "null" {
		fieldValues = existing.FieldValues
	}

	tag, err := h.service.UpdateTag(r.Context(), id, UpdateTagInput{
		Name:         content.SanitizeText(input.Name),
		Slug:         slug,
		Description:  input.Description,
		Rules:        input.Rules,
		CoverMediaID: coverMediaID,
		Color:        input.Color,
		LogoMediaID:  logoMediaID,
		IsSystem:     isSystem,
		FieldValues:  fieldValues,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Tag not found")
			return
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			errpkg.WriteError(w, http.StatusConflict, "duplicate_slug", "A tag with this slug already exists")
			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update tag")

		return
	}

	// Re-index the directory tag — name/description/content_count may have
	// changed and the tags index would otherwise go stale (§12).
	h.enqueueSearchIndex("index", tag.ID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": structToMap(tag)})
}

// DeleteTag handles DELETE /api/v1/admin/tags/{id}.
// Spec: "delete tag (only if no content tagged)" and system tags cannot be deleted.
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	// Guard 1: Check if tag is a system tag (cannot be deleted).
	tag, err := h.service.GetTagByID(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Tag not found")
		return
	}

	if tag.IsSystem {
		errpkg.WriteError(w, http.StatusForbidden, "system_tag_protected", "System tags cannot be deleted")
		return
	}

	// Guard 2: Check if any content is tagged with this tag.
	contentCount, err := h.service.CountContentByTag(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to check tag usage")
		return
	}

	if contentCount > 0 {
		errpkg.WriteError(w, http.StatusConflict, "tag_in_use", "Tag is in use by content and cannot be deleted")
		return
	}

	if err := h.service.DeleteTag(r.Context(), id); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete tag")
		return
	}

	// BUG-009 (follows audit): flush stale jc:is_following keys for the
	// deleted tag's followers.
	if h.follows != nil {
		h.follows.InvalidateEntityFollowers(r.Context(), follows.EntityTypeTag, id)
	}

	// Drop the directory tag from the tags index.
	h.enqueueSearchIndex("delete", id)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
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

// GetTagModerators handles GET /api/v1/tags/{slug}/moderators — public list
// of the tag's assigned moderators (Section 08). The frontend uses it to
// decide whether the tag hub renders its Moderators tab (shown only when at
// least one moderator is assigned) and to populate that tab's list.
func (h *Handlers) GetTagModerators(w http.ResponseWriter, r *http.Request) {
	tag, ok := h.resolveTag(w, r)
	if !ok {
		return
	}

	rows, err := h.service.ListTagModerators(r.Context(), tag.ID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list tag moderators")
		return
	}

	moderators := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		moderators = append(moderators, map[string]any{
			"user_id":      row.UserID,
			"username":     row.Username,
			"display_name": row.DisplayName,
			"avatar_url":   row.AvatarUrl,
			"added_at":     row.CreatedAt,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": moderators})
}

// ListTagModerators handles GET /api/v1/admin/tags/:id/moderators.
func (h *Handlers) ListTagModerators(w http.ResponseWriter, r *http.Request) {
	tagID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	rows, err := h.service.ListTagModerators(r.Context(), tagID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list tag moderators")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": moderators})
}

// AddTagModerator handles POST /api/v1/admin/tags/:id/moderators (Section 49).
func (h *Handlers) AddTagModerator(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tagID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	var input struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var userID pgtype.UUID

	switch {
	case input.UserID != "":
		if err := userID.Scan(input.UserID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
			return
		}
	case input.Username != "":
		userID, err = h.service.GetUserIDByUsername(r.Context(), input.Username)
		if err != nil {
			errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "user_id or username is required")
		return
	}

	row, err := h.service.AddTagModerator(r.Context(), AddTagModeratorInput{
		TagID:  tagID,
		UserID: userID,

		AssignedBy:       "site_admin",
		AssignedByUserID: user.UserID,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Tag or user not found")
			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to add tag moderator")

		return
	}

	// Rule 6: invalidate the TARGET user's permission cache so the newly
	// assigned tag moderator's permissions are visible immediately.
	h.invalidateTargetPermCache(r.Context(), userID)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"tag_id":   row.TagID,
		"user_id":  row.UserID,
		"added_at": row.CreatedAt,
	}})
}

// RemoveTagModerator handles DELETE /api/v1/admin/tags/:id/moderators/:userId (Section 49).
func (h *Handlers) RemoveTagModerator(w http.ResponseWriter, r *http.Request) {
	tagID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "userId")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
		return
	}

	if err := h.service.RemoveTagModerator(r.Context(), tagID, userID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove tag moderator")
		return
	}

	// Rule 6: invalidate the removed moderator's permission cache so their
	// tag moderation permissions are revoked immediately.
	h.invalidateTargetPermCache(r.Context(), userID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"removed": true}})
}
