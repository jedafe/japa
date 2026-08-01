// Package spaces — HTTP handlers for the Geo-First Space System.
//
// Every handler is a method on Handlers so dependencies (Service and
// UnreadCounter) are injected via the constructor, never read from globals.
// Responses follow the JapaConnect API envelope: success bodies are wrapped
// in {"data": ...} and errors in {"error":{"code":"...","message":"..."}}.
// Permission enforcement is handled by middleware wired at the router level;
// these handlers focus on parsing, validation, and Service delegation.
package spaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// ContentService is the subset of content.Service the space content/pinned
// handlers need.
type ContentService interface {
	ListContentsBySpace(ctx context.Context, tenantID, spaceID pgtype.UUID, cursor pgtype.Timestamptz, limit int32) ([]gen.ListContentsBySpaceRow, error)
	SearchContentsBySpace(ctx context.Context, tenantID, spaceID pgtype.UUID, query string, limit, offset int32) ([]gen.SearchContentsBySpaceRow, error)
	PinContent(ctx context.Context, tenantID, spaceID, contentID, pinnedBy pgtype.UUID, position int32) (gen.PinContentToSpaceRow, error)
	UnpinContent(ctx context.Context, spaceID, contentID pgtype.UUID) error
	ListPinnedContent(ctx context.Context, spaceID pgtype.UUID) ([]gen.ListPinnedContentRow, error)
}

// PermCacheInvalidator flushes a user's permission cache (jc:perm:{user_id})
// after a permission-affecting mutation. Implemented by rbac.RBAC.
// Ref: AGENTS.md Rule 6, sections/03-rbac.md (Permission Cache Invalidation).
type PermCacheInvalidator interface {
	InvalidateUserCache(ctx context.Context, userID pgtype.UUID)
}

// Handlers holds dependencies for space HTTP handlers. The Service wraps the
// reader/writer database pools; the UnreadCounter tracks per-user unread
// post counts via Valkey (nil is safe — it degrades to no-op).
type Handlers struct {
	Service *Service
	Unread  *UnreadCounter
	content ContentService
	follows FollowToggleService
	permCache PermCacheInvalidator
}

// FollowToggleService is the subset of the follows.Service used by the space
// handler to toggle follow status. Routing space follow through the generic
// follows service ensures auto-join for open spaces (BL-010 / Section 31)
// and notification dispatch are applied consistently (H3).
type FollowToggleService interface {
	ToggleFollow(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error)
}

// SetFollowToggleService injects the generic follows service for space follow.
func (h *Handlers) SetFollowToggleService(s FollowToggleService) {
	h.follows = s
}

// SetPermCacheInvalidator injects the RBAC resolver so handlers can flush
// the TARGET user's permission cache after moderator assign/remove (Rule 6).
// The middleware-level RequirePermissionWithInvalidation only flushes the
// CALLER's cache; the target user's cache must be flushed here.
func (h *Handlers) SetPermCacheInvalidator(p PermCacheInvalidator) {
	h.permCache = p
}

// invalidateTargetPermCache flushes the target user's permission cache after
// a moderator assignment/removal. It is a no-op when no invalidator is wired
// (graceful degradation). Errors are logged but never fail the request — the
// mutation has already succeeded and the cache will expire via TTL.
func (h *Handlers) invalidateTargetPermCache(ctx context.Context, targetUserID pgtype.UUID) {
	if h.permCache == nil {
		return
	}
	h.permCache.InvalidateUserCache(ctx, targetUserID)
}

// NewHandlers creates Handlers with the given Service and UnreadCounter.
func NewHandlers(service *Service, unread *UnreadCounter, contentService ContentService) *Handlers {
	return &Handlers{Service: service, Unread: unread, content: contentService}
}

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}

	return id, nil
}

// requireTenant resolves the tenant from the request context and writes a 503
// error when none is available. The boolean return is false when the caller
// should abort (the error has already been written).
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP
// response. It centralizes the not-found, duplicate, and fallback mapping so
// every handler stays consistent.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// spaceFromContext writes a 404 error and returns false when no resolved space
// is present in the request context. Callers should abort when the boolean is
// false.
func spaceFromContext(w http.ResponseWriter, r *http.Request) (*gen.Space, bool) {
	space := FromContext(r.Context())
	if space == nil {
		writeError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return nil, false
	}

	return space, true
}

// ---------------------------------------------------------------------------
// Space Type Handlers
// ---------------------------------------------------------------------------

// ListSpaceTypes handles GET /api/v1/space-types.
func (h *Handlers) ListSpaceTypes(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	types, err := h.Service.ListSpaceTypes(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": types})
}

// GetSpaceType handles GET /api/v1/space-types/{id}.
func (h *Handlers) GetSpaceType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	st, err := h.Service.GetSpaceType(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": st})
}

// CreateSpaceType handles POST /api/v1/space-types. API-created types are
// never system types.
func (h *Handlers) CreateSpaceType(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string  `json:"name"`
		Slug         string  `json:"slug"`
		Description  *string `json:"description"`
		Icon         *string `json:"icon"`
		FieldGroupID string  `json:"field_group_id"`
		IsUniversity bool    `json:"is_university"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Name and slug are required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var fieldGroupID pgtype.UUID
	if req.FieldGroupID != "" {
		if err := fieldGroupID.Scan(req.FieldGroupID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid field_group_id format")
			return
		}
	}

	st, err := h.Service.CreateSpaceType(r.Context(), gen.CreateSpaceTypeParams{
		TenantID:     tenantID,
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		Icon:         req.Icon,
		FieldGroupID: fieldGroupID,
		IsUniversity: req.IsUniversity,
		IsSystem:     false,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": st})
}

// UpdateSpaceType handles PATCH /api/v1/space-types/{id}. Slug is immutable
// and therefore not accepted in the request body.
func (h *Handlers) UpdateSpaceType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Name         string  `json:"name"`
		Description  *string `json:"description"`
		Icon         *string `json:"icon"`
		FieldGroupID string  `json:"field_group_id"`
		IsUniversity bool    `json:"is_university"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var fieldGroupID pgtype.UUID
	if req.FieldGroupID != "" {
		if err := fieldGroupID.Scan(req.FieldGroupID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid field_group_id format")
			return
		}
	}

	st, err := h.Service.UpdateSpaceType(r.Context(), gen.UpdateSpaceTypeParams{
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		FieldGroupID: fieldGroupID,
		IsUniversity: req.IsUniversity,
		ID:           id,
		TenantID:     tenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": st})
}

// DeleteSpaceType handles DELETE /api/v1/space-types/{id}.
func (h *Handlers) DeleteSpaceType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteSpaceType(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Space Handlers
// ---------------------------------------------------------------------------

// ListSpaces handles GET /api/v1/spaces. Optional query params filter the
// result set: q (search by name/slug), level (by hierarchy level), type (by
// space_type_id), parent (by parent_id). page_size/page_offset paginate the
// result. With no filters, all visible spaces for the tenant are returned.
func (h *Handlers) ListSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Resolve the optional viewer user so the visibility filter can include
	// members/private spaces the caller belongs to (BUG-007, Section 05 line 272).
	// On OptionalAuth routes the user may be nil → zero UUID → public spaces only.
	var viewerUserID pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil {
		viewerUserID = user.UserID
	}

	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("q"))

	var spaces []gen.Space
	var err error

	switch {
	case q.Get("level") != "":
		spaces, err = h.Service.ListSpacesByLevel(r.Context(), tenantID, q.Get("level"))
	case q.Get("type") != "":
		var spaceTypeID pgtype.UUID
		if scanErr := spaceTypeID.Scan(q.Get("type")); scanErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid type filter format")
			return
		}
		spaces, err = h.Service.ListSpacesByType(r.Context(), tenantID, spaceTypeID)
	case q.Get("parent") != "":
		var parentID pgtype.UUID
		if scanErr := parentID.Scan(q.Get("parent")); scanErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid parent filter format")
			return
		}
		spaces, err = h.Service.ListChildSpaces(r.Context(), tenantID, parentID)
	default:
		spaces, err = h.Service.ListSpaces(r.Context(), tenantID, viewerUserID)
	}

	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Apply search filter (by name or slug) if provided.
	if search != "" {
		filtered := make([]gen.Space, 0, len(spaces))
		for _, s := range spaces {
			if strings.Contains(strings.ToLower(s.Name), strings.ToLower(search)) ||
				strings.Contains(strings.ToLower(s.Slug), strings.ToLower(search)) {
				filtered = append(filtered, s)
			}
		}
		spaces = filtered
	}

	total := int32(len(spaces))

	// Apply pagination.
	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)
	if offset >= total {
		spaces = []gen.Space{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		spaces = spaces[offset:end]
	}

	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": spaces,
		"meta": map[string]any{
			"total":       total,
			"page":        offset / limit,
			"total_pages": totalPages,
		},
	})
}

// AdminListSpaces handles GET /api/v1/admin/spaces — paginated spaces for the
// admin panel with owner and space_type name resolved via JOINs.
func (h *Handlers) AdminListSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	spaces, err := h.Service.AdminListSpaces(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	total, err := h.Service.AdminListSpacesCount(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	totalPages := int32(1)
	if limit > 0 {
		totalPages = int32((total + int64(limit) - 1) / int64(limit))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":       spaces,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// ListMySpaces handles GET /api/v1/me/spaces — every space the caller belongs to.
func (h *Handlers) ListMySpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Optional q parameter for autocomplete search — filtered at the SQL
	// level by name/slug (case-insensitive ILIKE). Used by the /create
	// composer's space selector.
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	spaces, err := h.Service.ListSpacesByMember(r.Context(), tenantID, user.UserID, q)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// ListByMemberUser handles GET /users/{id}/spaces — the public profile Spaces
// tab. Public-safe: only public+active spaces of the {id} user. Non-active
// users are already 404'd by the requireActiveProfileUser gate on the route.
func (h *Handlers) ListByMemberUser(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	spaces, err := h.Service.ListPublicByMemberUser(r.Context(), tenantID, userID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": toSpaceCards(spaces)})
}

// SpaceCard is the lean public shape for the profile Spaces tab — only the
// fields the profile card renders. It deliberately omits the internal columns
// the raw gen.Space row carries (tenant_id, parent_id, space_type_id,
// wiki_content, the *_perm governance fields, geo_code, level, status, and
// counts the card doesn't show), so a public endpoint never echoes them.
type SpaceCard struct {
	ID            pgtype.UUID        `json:"id"`
	Name          string             `json:"name"`
	Slug          string             `json:"slug"`
	Description   *string            `json:"description"`
	LogoMediaID   pgtype.UUID        `json:"logo_media_id"`
	HeaderImageID pgtype.UUID        `json:"header_image_id"`
	MemberCount   int32              `json:"member_count"`
	Visibility    string             `json:"visibility"`
	Role          string             `json:"role"`
	CreatedAt     pgtype.Timestamptz `json:"created_at"`
}

func toSpaceCards(spaces []gen.ListPublicSpacesByMemberUserRow) []SpaceCard {
	cards := make([]SpaceCard, len(spaces))
	for i := range spaces {
		s := &spaces[i]
		cards[i] = SpaceCard{
			ID:            s.ID,
			Name:          s.Name,
			Slug:          s.Slug,
			Description:   s.Description,
			LogoMediaID:   s.LogoMediaID,
			HeaderImageID: s.HeaderImageID,
			MemberCount:   s.MemberCount,
			Visibility:    s.Visibility,
			Role:          s.MemberRole,
			CreatedAt:     s.CreatedAt,
		}
	}
	return cards
}

// ListMyModeratedSpaces handles GET /api/v1/me/spaces/moderated — every space
// the caller moderates.
func (h *Handlers) ListMyModeratedSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	spaces, err := h.Service.ListSpacesByModerator(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// ListMyOwnedSpaces handles GET /api/v1/me/spaces/owned — every space the
// caller owns (space_members.role = 'owner'). Distinct from /me/spaces which
// returns every space the caller belongs to as a member.
func (h *Handlers) ListMyOwnedSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	spaces, err := h.Service.ListSpacesByOwner(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// GetSpace handles GET /api/v1/spaces/{slug}. It returns the space together
// with its rules so a single request can render a complete space view.
func (h *Handlers) GetSpace(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Space slug is required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	space, err := h.Service.GetSpaceBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	rules, err := h.Service.ListSpaceRules(r.Context(), space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if rules == nil {
		rules = []gen.SpaceRule{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"space": space, "rules": rules},
	})
}

// AdminGetSpace handles GET /api/v1/admin/spaces/{id} — returns a single space
// by UUID (not slug) for the admin panel.
func (h *Handlers) AdminGetSpace(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	space, err := h.Service.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	rules, err := h.Service.ListSpaceRules(r.Context(), space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if rules == nil {
		rules = []gen.SpaceRule{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"space": space, "rules": rules},
	})
}

// CreateSpace handles POST /api/v1/spaces. When preset_name is provided the
// five permission axes are loaded from the named preset; otherwise the
// default axes are applied.
func (h *Handlers) CreateSpace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string   `json:"name"`
		Slug           string   `json:"slug"`
		SpaceTypeID    string   `json:"space_type_id"`
		Level          string   `json:"level"`
		ParentID       string   `json:"parent_id"`
		ParentSpaceIDs []string `json:"parent_space_ids"`
		Description    *string  `json:"description"`
		GeoCode        *string  `json:"geo_code"`
		PresetName     string   `json:"preset_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" ||
		strings.TrimSpace(req.SpaceTypeID) == "" || strings.TrimSpace(req.Level) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Name, slug, space_type_id, and level are required")
		return
	}

	if !IsValidLevel(req.Level) {
		writeError(w, http.StatusBadRequest, "invalid_level", "Level must be country, state, city, or custom")
		return
	}

	// BUG-002: custom spaces must be linked to at least one primary space
	// (country/state/city) via space_parent_spaces (Section 05 line 14, 190).
	// Primary spaces (country/state/city) are themselves parents and need no
	// parent linkage.
	if req.Level == LevelCustom && len(req.ParentSpaceIDs) == 0 {
		writeError(w, http.StatusBadRequest, "missing_parent",
			"Custom spaces must belong to at least one primary space (country, state, or city)")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// §26 / Section 03: enforce max_spaces_creatable quota from the caller's
	// user group. -1 = unlimited, 0 = none, N = capped at N spaces.
	maxSpaces, err := h.Service.GetUserMaxSpacesCreatable(r.Context(), tenantID, user.UserID)
	if err != nil {
		slog.Warn("create space: failed to load max_spaces_creatable quota",
			"user_id", user.UserID.String(), "error", err)
		// Fail open — don't block creation on a quota lookup failure.
	} else if maxSpaces == 0 {
		writeError(w, http.StatusForbidden, "quota_exceeded",
			"Your account is not allowed to create spaces")
		return
	} else if maxSpaces > 0 {
		currentCount, err := h.Service.CountSpacesByOwner(r.Context(), tenantID, user.UserID)
		if err != nil {
			slog.Warn("create space: failed to count owned spaces",
				"user_id", user.UserID.String(), "error", err)
		} else if currentCount >= maxSpaces {
			writeError(w, http.StatusForbidden, "quota_exceeded",
				"You have reached the maximum number of spaces you can create")
			return
		}
	}

	var spaceTypeID pgtype.UUID
	if err := spaceTypeID.Scan(req.SpaceTypeID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid space_type_id format")
		return
	}

	var parentID pgtype.UUID
	if req.ParentID != "" {
		if err := parentID.Scan(req.ParentID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid parent_id format")
			return
		}
	}

	// BUG-002: validate each parent_space_id is a primary space (level IN
	// country/state/city) within the tenant. Collect the parsed UUIDs for
	// linkage after the space is created.
	parentSpaceUUIDs := make([]pgtype.UUID, 0, len(req.ParentSpaceIDs))
	for _, psidStr := range req.ParentSpaceIDs {
		var psid pgtype.UUID
		if err := psid.Scan(psidStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid parent_space_id format")
			return
		}
		parent, err := h.Service.GetSpace(r.Context(), psid, tenantID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if parent.Level != LevelCountry && parent.Level != LevelState && parent.Level != LevelCity {
			writeError(w, http.StatusBadRequest, "invalid_parent",
				"Parent spaces must be primary (country, state, or city)")
			return
		}
		parentSpaceUUIDs = append(parentSpaceUUIDs, psid)
	}

	// Resolve permission axes: from the named preset, or the defaults.
	axes := DefaultPermAxes()

	if req.PresetName != "" {
		preset, err := h.Service.GetPermissionPreset(r.Context(), req.PresetName, tenantID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_preset", "Preset not found")
			return
		}

		axes = SpacePermAxes{
			Visibility: preset.Visibility,
			Posting:    preset.PostingPerm,
			Replying:   preset.ReplyingPerm,
			Commenting: preset.CommentingPerm,
			Joining:    preset.JoiningPerm,
		}
	}

	space, err := h.Service.CreateSpace(r.Context(), gen.CreateSpaceParams{
		TenantID:       tenantID,
		ParentID:       parentID,
		SpaceTypeID:    spaceTypeID,
		Name:           req.Name,
		Slug:           req.Slug,
		Description:    req.Description,
		GeoCode:        req.GeoCode,
		Level:          req.Level,
		Visibility:     axes.Visibility,
		PostingPerm:    axes.Posting,
		ReplyingPerm:   axes.Replying,
		CommentingPerm: axes.Commenting,
		JoiningPerm:    axes.Joining,
		Status:         StatusActive,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-001: add the creator as the space owner (role='owner') so they can
	// exercise the space_owner moderator-assignment path (Section 05 lines
	// 198-224). Without this the creator could never assign moderators.
	if err := h.Service.AddOwner(r.Context(), space.ID, user.UserID, tenantID); err != nil {
		slog.Warn("create space: failed to add creator as owner",
			"space_id", space.ID.String(), "user_id", user.UserID.String(), "error", err)
		// Non-fatal: the space exists; the owner row can be repaired later.
	} else if err := h.Service.IncrementSpaceCount(r.Context(), space.ID, tenantID, 1); err != nil {
		slog.Warn("create space: failed to increment member_count",
			"space_id", space.ID.String(), "error", err)
	}

	// BUG-002: link the new custom space to its primary parent spaces via
	// space_parent_spaces (Section 05 lines 46-53, 190). Best-effort —
	// logs failures but does not fail the request since the space row is
	// already committed; the parent links can be repaired via POST
	// /spaces/{slug}/parents.
	for _, psid := range parentSpaceUUIDs {
		if err := h.Service.AddParentSpace(r.Context(), space.ID, psid); err != nil {
			slog.Warn("create space: failed to link parent space",
				"space_id", space.ID.String(), "parent_id", psid.String(), "error", err)
		}
	}

	// Enqueue async auto-population (Wikipedia summary, flag image, etc.).
	h.Service.EnqueueAutoPopulate(r.Context(), space.ID, tenantID)
	// Enqueue Meilisearch index sync so the new space is searchable
	// immediately (BL-004).
	h.Service.EnqueueSearchIndex(r.Context(), space.ID, tenantID, "index")

	writeJSON(w, http.StatusCreated, map[string]any{"data": space})
}

// UpdateSpace handles PATCH /api/v1/spaces/{id}. It loads the existing space
// first so unspecified fields keep their current values (true PATCH
// semantics), then validates any provided permission axes before saving.
func (h *Handlers) UpdateSpace(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	existing, err := h.Service.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var req struct {
		Name           string  `json:"name"`
		Description    *string `json:"description"`
		WikiContent    string  `json:"wiki_content"`
		GeoCode        *string `json:"geo_code"`
		Visibility     string  `json:"visibility"`
		PostingPerm    string  `json:"posting_perm"`
		ReplyingPerm   string  `json:"replying_perm"`
		CommentingPerm string  `json:"commenting_perm"`
		JoiningPerm    string  `json:"joining_perm"`
		Status         string  `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	// Validate each provided permission axis individually.
	if req.Visibility != "" && !validVisibility[req.Visibility] {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid visibility value")
		return
	}

	if req.PostingPerm != "" && !validPostingPerm[req.PostingPerm] {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid posting_perm value")
		return
	}

	if req.ReplyingPerm != "" && !validPostingPerm[req.ReplyingPerm] {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid replying_perm value")
		return
	}

	if req.CommentingPerm != "" && !validPostingPerm[req.CommentingPerm] {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid commenting_perm value")
		return
	}

	if req.JoiningPerm != "" && !validJoiningPerm[req.JoiningPerm] {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid joining_perm value")
		return
	}

	// Overlay provided fields onto the existing space, keeping current values
	// for anything the request left blank.
	name := existing.Name
	if strings.TrimSpace(req.Name) != "" {
		name = req.Name
	}

	description := existing.Description
	if req.Description != nil {
		description = req.Description
	}

	wikiContent := existing.WikiContent
	if req.WikiContent != "" {
		wikiContent = []byte(req.WikiContent)
	}

	geoCode := existing.GeoCode
	if req.GeoCode != nil {
		geoCode = req.GeoCode
	}

	visibility := existing.Visibility
	if req.Visibility != "" {
		visibility = req.Visibility
	}

	postingPerm := existing.PostingPerm
	if req.PostingPerm != "" {
		postingPerm = req.PostingPerm
	}

	replyingPerm := existing.ReplyingPerm
	if req.ReplyingPerm != "" {
		replyingPerm = req.ReplyingPerm
	}

	commentingPerm := existing.CommentingPerm
	if req.CommentingPerm != "" {
		commentingPerm = req.CommentingPerm
	}

	joiningPerm := existing.JoiningPerm
	if req.JoiningPerm != "" {
		joiningPerm = req.JoiningPerm
	}

	status := existing.Status
	if req.Status != "" {
		status = req.Status
	}

	space, err := h.Service.UpdateSpace(r.Context(), gen.UpdateSpaceParams{
		Name:           name,
		Description:    description,
		WikiContent:    wikiContent,
		GeoCode:        geoCode,
		Visibility:     visibility,
		PostingPerm:    postingPerm,
		ReplyingPerm:   replyingPerm,
		CommentingPerm: commentingPerm,
		JoiningPerm:    joiningPerm,
		Status:         status,
		ID:             id,
		TenantID:       tenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Re-sync the Meilisearch index so search reflects the update (BL-004).
	h.Service.EnqueueSearchIndex(r.Context(), space.ID, tenantID, "index")

	writeJSON(w, http.StatusOK, map[string]any{"data": space})
}

// DeleteSpace handles DELETE /api/v1/spaces/{id}.
func (h *Handlers) DeleteSpace(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteSpace(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Remove the space from the Meilisearch index (BL-004).
	h.Service.EnqueueSearchIndex(r.Context(), id, tenantID, "delete")

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Space Rule Handlers
// ---------------------------------------------------------------------------

// ListSpaceRules handles GET /api/v1/spaces/{slug}/rules. The space is
// resolved by the SpaceResolver middleware and read from the request context.
func (h *Handlers) ListSpaceRules(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	rules, err := h.Service.ListSpaceRules(r.Context(), space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if rules == nil {
		rules = []gen.SpaceRule{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": rules})
}

// CreateSpaceRule handles POST /api/v1/spaces/{slug}/rules.
func (h *Handlers) CreateSpaceRule(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Position int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Title and body are required")
		return
	}

	rule, err := h.Service.CreateSpaceRule(r.Context(), gen.CreateSpaceRuleParams{
		SpaceID:  space.ID,
		Title:    req.Title,
		Body:     req.Body,
		Position: req.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": rule})
}

// UpdateSpaceRule handles PATCH /api/v1/spaces/{slug}/rules/{ruleId}.
func (h *Handlers) UpdateSpaceRule(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	ruleID, err := parseUUIDParam(r, "ruleId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID format")
		return
	}

	var req struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Position int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Title and body are required")
		return
	}

	rule, err := h.Service.UpdateSpaceRule(r.Context(), gen.UpdateSpaceRuleParams{
		Title:    req.Title,
		Body:     req.Body,
		Position: req.Position,
		ID:       ruleID,
		SpaceID:  space.ID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": rule})
}

// DeleteSpaceRule handles DELETE /api/v1/spaces/{slug}/rules/{ruleId}.
func (h *Handlers) DeleteSpaceRule(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	ruleID, err := parseUUIDParam(r, "ruleId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID format")
		return
	}

	if err := h.Service.DeleteSpaceRule(r.Context(), ruleID, space.ID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Space Membership Handlers
// ---------------------------------------------------------------------------

// ToggleSpaceMembership handles POST /api/v1/spaces/{slug}/join.
// Toggles membership: joins if not a member, leaves if already a member.
func (h *Handlers) ToggleSpaceMembership(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	isMember, err := h.Service.IsMember(r.Context(), space.ID, user.UserID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if isMember {
		if err := h.Service.Leave(r.Context(), space.ID, user.UserID, tenantID); err != nil {
			handleServiceError(w, err)
			return
		}

		if err := h.Service.IncrementSpaceCount(r.Context(), space.ID, tenantID, -1); err != nil {
			handleServiceError(w, err)
			return
		}
	} else {
		// BUG-004: honor the space's joining_perm axis (Section 05 lines 358-360).
		// - 'open': immediate membership (existing behavior).
		// - 'request': insert a pending join request (status='pending') for
		//   admin approval; do NOT increment member_count until approved.
		// - 'invite': only invited users may join; reject otherwise.
		switch space.JoiningPerm {
		case JoiningRequest:
			if err := h.Service.JoinPending(r.Context(), space.ID, user.UserID, tenantID); err != nil {
				handleServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{
				"data": map[string]any{
					"joined":       false,
					"pending":      true,
					"member_count": 0, // unchanged — pending requests don't count
				},
			})
			return
		case JoiningInvite:
			writeError(w, http.StatusForbidden, "invite_only",
				"This space is invite-only; you cannot join without an invitation")
			return
		default:
			// JoiningOpen — immediate membership.
			if err := h.Service.Join(r.Context(), space.ID, user.UserID, tenantID); err != nil {
				handleServiceError(w, err)
				return
			}

			if err := h.Service.IncrementSpaceCount(r.Context(), space.ID, tenantID, 1); err != nil {
				handleServiceError(w, err)
				return
			}
		}
	}

	memberCount, err := h.Service.CountMembers(r.Context(), space.ID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"joined":       !isMember,
			"member_count": memberCount,
		},
	})
}

// ---------------------------------------------------------------------------
// Permission Preset Handlers
// ---------------------------------------------------------------------------

// ListPermissionPresets handles GET /api/v1/space-permission-presets.
func (h *Handlers) ListPermissionPresets(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	presets, err := h.Service.ListPermissionPresets(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": presets})
}

// UpdatePermissionPreset handles PUT /api/v1/space-permission-presets/{name}.
// The preset name is taken from the URL path; the five axis values come from
// the request body.
func (h *Handlers) UpdatePermissionPreset(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Preset name is required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	existing, err := h.Service.GetPermissionPreset(r.Context(), name, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var req struct {
		Visibility     string `json:"visibility"`
		PostingPerm    string `json:"posting_perm"`
		ReplyingPerm   string `json:"replying_perm"`
		CommentingPerm string `json:"commenting_perm"`
		JoiningPerm    string `json:"joining_perm"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	axes := SpacePermAxes{
		Visibility: req.Visibility,
		Posting:    req.PostingPerm,
		Replying:   req.ReplyingPerm,
		Commenting: req.CommentingPerm,
		Joining:    req.JoiningPerm,
	}
	if !ValidPermissionAxes(axes) {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "One or more permission axes are invalid")
		return
	}

	preset, err := h.Service.UpdatePermissionPreset(r.Context(), gen.UpdatePermissionPresetParams{
		Visibility:     req.Visibility,
		PostingPerm:    req.PostingPerm,
		ReplyingPerm:   req.ReplyingPerm,
		CommentingPerm: req.CommentingPerm,
		JoiningPerm:    req.JoiningPerm,
		ID:             existing.ID,
		TenantID:       tenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": preset})
}

// ---------------------------------------------------------------------------
// Space Moderator Handlers
// ---------------------------------------------------------------------------

// AssignModerator handles POST /api/v1/spaces/{slug}/moderators.
// Adds a user as a moderator of the space.
func (h *Handlers) AssignModerator(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req struct {
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.UserID) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "user_id is required")
		return
	}

	var targetUserID pgtype.UUID
	if err := targetUserID.Scan(req.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user_id format")
		return
	}

	// Determine who is assigning: site_admin or space_owner.
	// The route is behind spaces.moderator.assign permission. If the caller
	// is the space owner, use the space_owner path and require the target to
	// be an existing member (Section 03). Otherwise treat as site_admin.
	assignedBy := "site_admin"
	assignedByID := user.UserID

	isOwner, err := h.Service.IsOwner(r.Context(), space.ID, user.UserID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if isOwner {
		assignedBy = "space_owner"
		// Section 03: space_owner path requires the target to be an existing
		// member of the space before promoting to moderator.
		isMember, err := h.Service.IsMember(r.Context(), space.ID, targetUserID, tenantID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if !isMember {
			writeError(w, http.StatusBadRequest, "not_a_member",
				"Target user must be a member of the space before being assigned as a moderator")
			return
		}
	}

	if err := h.Service.AssignModerator(r.Context(), gen.AssignSpaceModeratorParams{
		TenantID:         tenantID,
		SpaceID:          space.ID,
		UserID:           targetUserID,
		AssignedBy:       assignedBy,
		AssignedByUserID: assignedByID,
	}); err != nil {
		handleServiceError(w, err)
		return
	}

	// Rule 6: invalidate the TARGET user's permission cache so the newly
	// assigned moderator's spaces.moderate.assigned node is visible
	// immediately (the middleware only flushed the CALLER's cache).
	h.invalidateTargetPermCache(r.Context(), targetUserID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"space_id":    space.ID,
			"user_id":     targetUserID,
			"assigned_by": assignedBy,
		},
	})
}

// RemoveModerator handles DELETE /api/v1/spaces/{slug}/moderators/{userId}.
// Removes a user from the space moderator list.
func (h *Handlers) RemoveModerator(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	userID, err := parseUUIDParam(r, "userId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID format")
		return
	}

	if err := h.Service.RemoveModerator(r.Context(), space.ID, userID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Rule 6: invalidate the removed moderator's permission cache so their
	// spaces.moderate.assigned node is revoked immediately.
	h.invalidateTargetPermCache(r.Context(), userID)

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "removed"}})
}

// ListModerators handles GET /api/v1/spaces/{slug}/moderators.
// Returns all moderators assigned to the space.
func (h *Handlers) ListModerators(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	mods, err := h.Service.ListModerators(r.Context(), space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": mods})
}

// AdminListModerators handles GET /api/v1/admin/spaces/{id}/moderators.
func (h *Handlers) AdminListModerators(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid space ID")
		return
	}

	_, err = h.Service.GetSpaceByID(r.Context(), tenantID, spaceID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	mods, err := h.Service.ListModerators(r.Context(), spaceID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": mods})
}

// AdminAssignModerator handles POST /api/v1/admin/spaces/{id}/moderators.
// Admin-scoped variant of the space moderator assignment that resolves the
// space by ID (not slug) and accepts either a user_id or username.
func (h *Handlers) AdminAssignModerator(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid space ID")
		return
	}

	space, err := h.Service.GetSpaceByID(r.Context(), tenantID, spaceID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var req struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var targetUserID pgtype.UUID
	switch {
	case req.UserID != "":
		if err := targetUserID.Scan(req.UserID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user_id format")
			return
		}
	case req.Username != "":
		targetUserID, err = h.Service.GetUserIDByUsername(r.Context(), tenantID, req.Username)
		if err != nil {
			handleServiceError(w, err)
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "missing_fields", "user_id or username is required")
		return
	}

	if err := h.Service.AssignModerator(r.Context(), gen.AssignSpaceModeratorParams{
		TenantID:         tenantID,
		SpaceID:          space.ID,
		UserID:           targetUserID,
		AssignedBy:       "site_admin",
		AssignedByUserID: user.UserID,
	}); err != nil {
		handleServiceError(w, err)
		return
	}

	// Rule 6: invalidate the TARGET user's permission cache.
	h.invalidateTargetPermCache(r.Context(), targetUserID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"space_id": space.ID.String(),
			"user_id":  targetUserID.String(),
		},
	})
}

// AdminRemoveModerator handles DELETE /api/v1/admin/spaces/{id}/moderators/{userId}.
func (h *Handlers) AdminRemoveModerator(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if user := auth.FromContext(r.Context()); user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid space ID")
		return
	}

	space, err := h.Service.GetSpaceByID(r.Context(), tenantID, spaceID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var targetUserID pgtype.UUID
	switch {
	case chi.URLParam(r, "moderator_id") != "":
		if err := targetUserID.Scan(chi.URLParam(r, "moderator_id")); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid moderator_id format")
			return
		}
	case chi.URLParam(r, "userId") != "":
		if err := targetUserID.Scan(chi.URLParam(r, "userId")); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid userId format")
			return
		}
	case chi.URLParam(r, "username") != "":
		targetUserID, err = h.Service.GetUserIDByUsername(r.Context(), tenantID, chi.URLParam(r, "username"))
		if err != nil {
			handleServiceError(w, err)
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "missing_fields", "userId or username is required")
		return
	}

	if err := h.Service.RemoveModerator(r.Context(), space.ID, targetUserID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Rule 6: invalidate the removed moderator's permission cache.
	h.invalidateTargetPermCache(r.Context(), targetUserID)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"space_id": space.ID.String(),
			"user_id":  targetUserID.String(),
		},
	})
}

// ---------------------------------------------------------------------------
// Space Parent Spaces (Many-to-Many Hierarchy) Handlers
// ---------------------------------------------------------------------------

// AddParentSpace handles POST /api/v1/spaces/{slug}/parents.
// Links the space to a parent space via the junction table.
func (h *Handlers) AddParentSpace(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		ParentSpaceID string `json:"parent_space_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.ParentSpaceID) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "parent_space_id is required")
		return
	}

	var parentID pgtype.UUID
	if err := parentID.Scan(req.ParentSpaceID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid parent_space_id format")
		return
	}
	// Verify the parent space exists and belongs to the same tenant.
	if _, err := h.Service.GetSpace(r.Context(), parentID, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	if err := h.Service.AddParentSpace(r.Context(), space.ID, parentID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"space_id":        space.ID,
			"parent_space_id": parentID,
		},
	})
}

// RemoveParentSpace handles DELETE /api/v1/spaces/{slug}/parents/{parentId}.
// Unlinks the space from the given parent.
func (h *Handlers) RemoveParentSpace(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	parentID, err := parseUUIDParam(r, "parentId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid parent ID format")
		return
	}

	if err := h.Service.RemoveParentSpace(r.Context(), space.ID, parentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Parent relationship not found")
			return
		}

		handleServiceError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "removed"}})
}

// ListParentSpaces handles GET /api/v1/spaces/{slug}/parents.
// Returns all parent spaces linked via the junction table.
func (h *Handlers) ListParentSpaces(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	parents, err := h.Service.GetParentSpaces(r.Context(), space.ID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": parents})
}

// ---------------------------------------------------------------------------
// Space Members Listing Handler
// ---------------------------------------------------------------------------

// ListMembers handles GET /api/v1/spaces/{slug}/members.
// Returns paginated members of the space with user profile data.
func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	members, err := h.Service.ListMembers(r.Context(), space.ID, tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": members})
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return 20
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 20
	}

	if n > 100 {
		return 100
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// =====================
// Follow / Unfollow
// =====================

// ToggleFollow handles POST/DELETE /api/v1/spaces/{slug}/follow.
// Toggles the authenticated user's follow status for the space.
func (h *Handlers) ToggleFollow(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// H3: route through the generic follows service when available, so
	// auto-join for open spaces (BL-010) and notification dispatch are
	// applied consistently. Fall back to the space-specific path only when
	// the generic service is not wired (backward compatibility).
	var followed bool
	var err error
	if h.follows != nil {
		followed, err = h.follows.ToggleFollow(r.Context(), tenantID, user.UserID, "space", space.ID)
	} else {
		followed, err = h.Service.ToggleFollowSpace(r.Context(), tenantID, user.UserID, space.ID)
	}
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Return the actual post-toggle state. The `followed` bool is true when
	// the user is now following, false when now unfollowed. Hardcoding false
	// on DELETE would misreport the state when the user wasn't following
	// before (the toggle follows them but the response says false).
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"following": followed}})
}

// =====================
// Mute / Unmute
// =====================

// MuteSpace handles POST /api/v1/spaces/{slug}/mute.
func (h *Handlers) MuteSpace(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if err := h.Service.MuteSpace(r.Context(), tenantID, user.UserID, space.ID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"muted": true}})
}

// UnmuteSpace handles DELETE /api/v1/spaces/{slug}/mute.
func (h *Handlers) UnmuteSpace(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if err := h.Service.UnmuteSpace(r.Context(), tenantID, user.UserID, space.ID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"muted": false}})
}

// GetMuteStatus handles GET /api/v1/spaces/{slug}/mute-status.
func (h *Handlers) GetMuteStatus(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"muted": false}})
		return
	}

	muted, err := h.Service.IsSpaceMuted(r.Context(), tenantID, user.UserID, space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"muted": muted}})
}

// ListSpaceContent handles GET /api/v1/spaces/{slug}/content — content
// belonging to this space, newest first. Supports ?cursor= and ?page_size=.
// When ?q= is supplied, results are filtered by title (case-insensitive
// substring) and offset-paginated via ?page_offset= (space search tab).
func (h *Handlers) ListSpaceContent(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)

	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		offset := parsePageSizeOffset(r)
		items, err := h.content.SearchContentsBySpace(r.Context(), tenantID, space.ID, q, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to search space content")
			return
		}
		if items == nil {
			items = []gen.SearchContentsBySpaceRow{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items})
		return
	}

	var cursor pgtype.Timestamptz
	if cs := r.URL.Query().Get("cursor"); cs != "" {
		if err := cursor.Scan(cs); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid cursor timestamp")
			return
		}
	}

	items, err := h.content.ListContentsBySpace(r.Context(), tenantID, space.ID, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list space content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ListPinned handles GET /api/v1/spaces/{slug}/pinned.
func (h *Handlers) ListPinned(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	pins, err := h.content.ListPinnedContent(r.Context(), space.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list pinned content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": pins})
}

// AddPinned handles POST /api/v1/spaces/{slug}/pinned — pins a content item
// to the space (moderator action, gated by spaces.moderate at the router).
func (h *Handlers) AddPinned(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var input struct {
		ContentID string `json:"content_id"`
		Position  int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ContentID == "" {
		writeError(w, http.StatusBadRequest, "missing_content_id", "content_id is required")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	pin, err := h.content.PinContent(r.Context(), tenantID, space.ID, contentID, user.UserID, input.Position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to pin content")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": pin})
}

// RemovePinned handles DELETE /api/v1/spaces/{slug}/pinned/{content_id}.
func (h *Handlers) RemovePinned(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(chi.URLParam(r, "content_id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	if err := h.content.UnpinContent(r.Context(), space.ID, contentID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unpin content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"unpinned": true}})
}

// Leave handles DELETE /api/v1/spaces/{slug}/leave — dedicated leave action,
// split from the join/leave toggle in ToggleSpaceMembership.
func (h *Handlers) Leave(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.Leave(r.Context(), space.ID, user.UserID, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	if err := h.Service.IncrementSpaceCount(r.Context(), space.ID, tenantID, -1); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"left": true}})
}

// ─── Admin Space Extras (P5.6 — Task 12) ─────────────────────────────────

// ListJoinRequests handles GET /admin/spaces/:id/join-requests.
func (h *Handlers) ListJoinRequests(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.read.ListSpaceJoinRequests(r.Context(), gen.ListSpaceJoinRequestsParams{
		SpaceID:  spaceID,
		TenantID: tenantID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list join requests")
		return
	}
	requests := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		requests = append(requests, map[string]any{
			"user_id": r.UserID, "username": r.Username, "display_name": r.DisplayName, "avatar_url": r.AvatarUrl, "joined_at": r.JoinedAt, "status": r.Status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": requests})
}

// UpdateJoinRequest handles PATCH /admin/spaces/:id/join-requests/:requestId.
func (h *Handlers) UpdateJoinRequest(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	requestID, err := parseUUIDParam(r, "requestId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_id", "Invalid request ID")
		return
	}
	var input struct {
		Status string `json:"status"` // "approved" or "rejected"
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	err = h.Service.write.UpdateSpaceJoinRequest(r.Context(), gen.UpdateSpaceJoinRequestParams{
		SpaceID:  spaceID,
		UserID:   requestID,
		Status:   input.Status,
		TenantID: tenantID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update join request")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": requestID.String(), "status": input.Status})
}

// UpdateSpaceStatus handles PATCH /admin/spaces/:id/status.
func (h *Handlers) UpdateSpaceStatus(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	err = h.Service.write.UpdateSpaceStatus(r.Context(), gen.UpdateSpaceStatusParams{
		ID:       spaceID,
		TenantID: tenantID,
		Status:   input.Status,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update space status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": spaceID.String(), "status": input.Status})
}

// GetSpaceAnalytics handles GET /admin/spaces/:id/analytics.
func (h *Handlers) GetSpaceAnalytics(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// BUG-005: accept both UUID and slug. The catalog route is
	// /spaces/{id}/analytics, but the SSR space page passes the slug.
	// Try UUID first; if parsing fails, fall back to slug lookup.
	param := chi.URLParam(r, "id")
	var spaceID pgtype.UUID
	if err := spaceID.Scan(param); err != nil {
		// Not a UUID — try slug lookup.
		space, err := h.Service.GetSpaceBySlug(r.Context(), param, tenantID)
		if err != nil {
			writeError(w, http.StatusNotFound, "space_not_found", "Space not found")
			return
		}
		spaceID = space.ID
	}

	// [auth, space-admin] per JEOS §215 — only the space owner or an assigned
	// moderator may view the analytics dashboard. Guests are blocked by
	// RequireAuth on the route; non-admin members are blocked here.
	canManage, err := h.Service.CanManageSpace(r.Context(), spaceID, user.UserID, tenantID)
	if err != nil {
		slog.Error("space stats: checking manage permission", "error", err, "space_id", spaceID.String())
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to authorize analytics")
		return
	}
	if !canManage {
		writeError(w, http.StatusForbidden, "analytics_forbidden", "Only space admins can view analytics")
		return
	}

	// BUG-020: single round-trip aggregate instead of 3 sequential queries.
	summary, err := h.Service.GetSpaceAnalyticsSummary(r.Context(), spaceID, tenantID)
	if err != nil {
		slog.Error("space stats: loading analytics summary", "error", err, "space_id", spaceID.String())
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load space stats")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"member_count":  summary.MemberCount,
		"content_count": summary.ContentCount,
		"view_count":    summary.ViewCount,
	}})
}

// GetSpaceAnalyticsBySlug handles GET /spaces/{slug}/analytics — the
// slug-based variant for the main app's analytics tab (BUG-005). The space is
// resolved by the SpaceResolver middleware and read from the request context.
// [auth, space-admin] per JEOS §215 — only the space owner or an assigned
// moderator may view the analytics dashboard.
func (h *Handlers) GetSpaceAnalyticsBySlug(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Only the space owner or an assigned moderator may view analytics.
	canManage, err := h.Service.CanManageSpace(r.Context(), space.ID, user.UserID, tenantID)
	if err != nil {
		slog.Error("space stats: checking manage permission", "error", err, "space_id", space.ID.String())
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to authorize analytics")
		return
	}
	if !canManage {
		writeError(w, http.StatusForbidden, "analytics_forbidden", "Only space admins can view analytics")
		return
	}

	// BUG-020: single round-trip aggregate instead of 3 sequential queries.
	summary, err := h.Service.GetSpaceAnalyticsSummary(r.Context(), space.ID, tenantID)
	if err != nil {
		slog.Error("space stats: loading analytics summary", "error", err, "space_id", space.ID.String())
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load space stats")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"member_count":  summary.MemberCount,
		"content_count": summary.ContentCount,
		"view_count":    summary.ViewCount,
	}})
}

// ReorderPinned handles POST /admin/spaces/:id/pinned/reorder.
func (h *Handlers) ReorderPinned(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var input struct {
		Order []string `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	contentIDs := make([]pgtype.UUID, 0, len(input.Order))
	for _, contentIDStr := range input.Order {
		var contentID pgtype.UUID
		if scanErr := contentID.Scan(contentIDStr); scanErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content ID: "+contentIDStr)
			return
		}
		contentIDs = append(contentIDs, contentID)
	}

	if err := h.Service.ReorderPinned(r.Context(), spaceID, contentIDs); err != nil {
		slog.Error("reordering pinned content", "error", err, "space_id", spaceID.String())
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reorder pinned content")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})
}

// SuggestParentSpaces handles GET /spaces/suggest-parents.
func (h *Handlers) SuggestParentSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	rows, err := h.Service.read.SuggestParentSpaces(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to suggest parents")
		return
	}
	spaces := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		spaces = append(spaces, map[string]any{"id": r.ID, "name": r.Name, "slug": r.Slug})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// AdminListJoinRequests handles GET /api/v1/admin/join-requests — all
// pending join requests across every space in the admin's tenant. Each row is
// given a composite id (space_id::user_id) so the panel can target a specific
// request for approval/rejection.
func (h *Handlers) AdminListJoinRequests(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	rows, err := h.Service.read.ListAllSpaceJoinRequests(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list join requests")
		return
	}

	requests := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		compositeID := row.SpaceID.String() + "::" + row.UserID.String()
		requests = append(requests, map[string]any{
			"id":           compositeID,
			"space_id":     row.SpaceID,
			"user_id":      row.UserID,
			"username":     row.Username,
			"display_name": row.DisplayName,
			"avatar_url":   row.AvatarUrl,
			"joined_at":    row.JoinedAt,
			"status":       row.Status,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": requests})
}

// AdminResolveJoinRequest is the shared approve/reject implementation behind
// POST /api/v1/admin/join-requests/{id}/approve and /reject. The composite id
// encodes space_id::user_id, which is split and applied via the existing
// UpdateSpaceJoinRequest store call.
func (h *Handlers) AdminResolveJoinRequest(w http.ResponseWriter, r *http.Request, status string) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	composite := chi.URLParam(r, "id")
	parts := strings.Split(composite, "::")
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid composite request id")
		return
	}

	var spaceID, userID pgtype.UUID
	if err := spaceID.Scan(parts[0]); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid space id")
		return
	}
	if err := userID.Scan(parts[1]); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user id")
		return
	}

	if err := h.Service.write.UpdateSpaceJoinRequest(r.Context(), gen.UpdateSpaceJoinRequestParams{
		SpaceID:  spaceID,
		UserID:   userID,
		Status:   status,
		TenantID: tenantID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update join request")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": composite, "status": status})
}

// AdminApproveJoinRequest handles POST /api/v1/admin/join-requests/{id}/approve.
func (h *Handlers) AdminApproveJoinRequest(w http.ResponseWriter, r *http.Request) {
	h.AdminResolveJoinRequest(w, r, "approved")
}

// AdminRejectJoinRequest handles POST /api/v1/admin/join-requests/{id}/reject.
func (h *Handlers) AdminRejectJoinRequest(w http.ResponseWriter, r *http.Request) {
	h.AdminResolveJoinRequest(w, r, "rejected")
}
