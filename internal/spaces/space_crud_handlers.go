package spaces

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Space Handlers
// ---------------------------------------------------------------------------

// ListSpaces handles GET /api/v1/spaces. Optional query params filter the
// result set: q (search by name/slug), level (by hierarchy level), type (by
// space_type_id), parent (by parent_id), geo (by geo_code), sort
// (members|followers|posts|name|newest). page_size/page_offset paginate the
// result. With no filters, all visible spaces for the tenant are returned.
// SPACES-009/011: the default (no type/parent filter) branch uses SQL-level
// pagination, search, geo, and sort for performance and spec conformance.
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
	geoCode := strings.TrimSpace(q.Get("geo"))
	sort := strings.TrimSpace(q.Get("sort"))
	levelFilter := strings.TrimSpace(q.Get("level"))
	region := strings.TrimSpace(q.Get("region"))
	subregion := strings.TrimSpace(q.Get("subregion"))

	// The different list branches return distinct sqlc-generated Row types
	// (each structurally identical to gen.Space minus the Keywords column).
	// Use any to hold the heterogeneous slice, then filter/paginate via the
	// shared helpers below.
	var spaces any

	var err error

	var total int

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	switch {
	case q.Get("type") != "":
		var spaceTypeID pgtype.UUID
		if scanErr := spaceTypeID.Scan(q.Get("type")); scanErr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid type filter format")
			return
		}

		spaces, err = h.Service.ListSpacesByType(r.Context(), tenantID, viewerUserID, spaceTypeID)
	case q.Get("parent") != "":
		var parentID pgtype.UUID
		if scanErr := parentID.Scan(q.Get("parent")); scanErr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid parent filter format")
			return
		}

		if levelFilter != "" {
			spaces, err = h.Service.ListSpacesByParentAndLevel(r.Context(), tenantID, viewerUserID, parentID, levelFilter)
		} else {
			spaces, err = h.Service.ListChildSpaces(r.Context(), tenantID, viewerUserID, parentID)
		}
	default:
		// SPACES-009/011: use SQL-level pagination, search, geo, level, region,
		// subregion, and sort for the default branch. This avoids loading the
		// entire tenant's space set into memory and supports the spec's
		// ?geo=&sort=&region=&subregion= params.
		rows, perr := h.Service.ListSpacesPaginated(r.Context(), tenantID, viewerUserID, search, geoCode, levelFilter, region, subregion, sort, int32(limit), int32(offset))
		if perr != nil {
			handleServiceError(w, perr)
			return
		}

		count, cerr := h.Service.CountSpaces(r.Context(), tenantID, viewerUserID, search, geoCode, levelFilter, region, subregion)
		if cerr != nil {
			handleServiceError(w, cerr)
			return
		}

		spaces = rows
		total = int(count)
	}

	if err != nil {
		handleServiceError(w, err)
		return
	}

	// For the non-default branches (type/parent filter), apply search filter
	// and pagination in memory. The default branch already did this in SQL.
	if q.Get("type") != "" || q.Get("parent") != "" {
		if search != "" {
			spaces = filterSpacesByNameSlug(spaces, search)
		}

		total = int(spacesLen(spaces))
		spaces = paginateSpaces(spaces, offset, limit)
	}

	totalPages := total / int(limit)
	if total%int(limit) > 0 {
		totalPages++
	}

	// Resolve each space's logo_media_id to an imgproxy-signed logo_url so the
	// /spaces index can render space logos. Mirrors GetSpace's logo resolution.
	// Best-effort: spaces without a logo (or when the media resolver is not
	// wired) get an empty logo_url and the frontend renders a fallback icon.
	items := h.spacesWithDetails(r.Context(), tenantID, spaces)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{
			"total":       total,
			"page":        offset / limit,
			"total_pages": totalPages,
		},
	})
}

func ptrStringsToStrings(in []*string) []string {
	out := make([]string, 0, len(in))

	for _, s := range in {
		if s != nil {
			out = append(out, *s)
		}
	}

	return out
}

// ListDistinctRegions handles GET /api/v1/spaces/regions.
func (h *Handlers) ListDistinctRegions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	regions, err := h.Service.ListDistinctRegions(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ptrStringsToStrings(regions)})
}

// ListSubregionsByRegion handles GET /api/v1/spaces/subregions.
func (h *Handlers) ListSubregionsByRegion(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	region := r.URL.Query().Get("region")
	if region == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_region", "region query parameter is required")
		return
	}

	subregions, err := h.Service.ListDistinctSubregions(r.Context(), tenantID, region)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ptrStringsToStrings(subregions)})
}

// filterSpacesByNameSlug filters a slice of space Row types by name/slug
// (case-insensitive substring match). It handles the heterogeneous Row types
// returned by the different ListSpaces branches via a type switch.
func filterSpacesByNameSlug(spaces any, search string) any {
	lower := strings.ToLower(search)

	switch s := spaces.(type) {
	case []gen.ListSpacesByTenantRow:
		filtered := make([]gen.ListSpacesByTenantRow, 0, len(s))

		for _, sp := range s {
			if strings.Contains(strings.ToLower(sp.Name), lower) ||
				strings.Contains(strings.ToLower(sp.Slug), lower) {
				filtered = append(filtered, sp)
			}
		}

		return filtered
	case []gen.ListSpacesByLevelRow:
		filtered := make([]gen.ListSpacesByLevelRow, 0, len(s))

		for _, sp := range s {
			if strings.Contains(strings.ToLower(sp.Name), lower) ||
				strings.Contains(strings.ToLower(sp.Slug), lower) {
				filtered = append(filtered, sp)
			}
		}

		return filtered
	case []gen.ListSpacesByTypeRow:
		filtered := make([]gen.ListSpacesByTypeRow, 0, len(s))

		for _, sp := range s {
			if strings.Contains(strings.ToLower(sp.Name), lower) ||
				strings.Contains(strings.ToLower(sp.Slug), lower) {
				filtered = append(filtered, sp)
			}
		}

		return filtered
	case []gen.ListChildSpacesRow:
		filtered := make([]gen.ListChildSpacesRow, 0, len(s))

		for _, sp := range s {
			if strings.Contains(strings.ToLower(sp.Name), lower) ||
				strings.Contains(strings.ToLower(sp.Slug), lower) {
				filtered = append(filtered, sp)
			}
		}

		return filtered
	case []gen.ListSpacesPaginatedRow:
		filtered := make([]gen.ListSpacesPaginatedRow, 0, len(s))

		for _, sp := range s {
			if strings.Contains(strings.ToLower(sp.Name), lower) ||
				strings.Contains(strings.ToLower(sp.Slug), lower) {
				filtered = append(filtered, sp)
			}
		}

		return filtered
	case []gen.ListSpacesByParentAndLevelRow:
		filtered := make([]gen.ListSpacesByParentAndLevelRow, 0, len(s))

		for _, sp := range s {
			if strings.Contains(strings.ToLower(sp.Name), lower) ||
				strings.Contains(strings.ToLower(sp.Slug), lower) {
				filtered = append(filtered, sp)
			}
		}

		return filtered
	case []gen.ListSpacesByAncestorAndTypeRow:
		filtered := make([]gen.ListSpacesByAncestorAndTypeRow, 0, len(s))

		for _, sp := range s {
			if strings.Contains(strings.ToLower(sp.Name), lower) ||
				strings.Contains(strings.ToLower(sp.Slug), lower) {
				filtered = append(filtered, sp)
			}
		}

		return filtered
	default:
		return spaces
	}
}

// spacesLen returns the length of a slice of space Row types held in an any.
func spacesLen(spaces any) int32 {
	switch s := spaces.(type) {
	case []gen.ListSpacesByTenantRow:
		return int32(len(s)) // #nosec G115 -- len() of a DB result slice
	case []gen.ListSpacesByLevelRow:
		return int32(len(s)) // #nosec G115 -- len() of a DB result slice
	case []gen.ListSpacesByTypeRow:
		return int32(len(s)) // #nosec G115 -- len() of a DB result slice
	case []gen.ListChildSpacesRow:
		return int32(len(s)) // #nosec G115 -- len() of a DB result slice
	case []gen.ListSpacesPaginatedRow:
		return int32(len(s)) // #nosec G115 -- len() of a DB result slice
	case []gen.ListSpacesByParentAndLevelRow:
		return int32(len(s)) // #nosec G115 -- len() of a DB result slice
	case []gen.ListSpacesByAncestorAndTypeRow:
		return int32(len(s)) // #nosec G115 -- len() of a DB result slice
	default:
		return 0
	}
}

// paginateSpaces applies offset/limit pagination to a slice of space Row types
// held in an any.
func paginateSpaces(spaces any, offset, limit int32) any {
	switch s := spaces.(type) {
	case []gen.ListSpacesByTenantRow:
		return paginateSpaceSlice(s, offset, limit)
	case []gen.ListSpacesByLevelRow:
		return paginateSpaceSlice(s, offset, limit)
	case []gen.ListSpacesByTypeRow:
		return paginateSpaceSlice(s, offset, limit)
	case []gen.ListChildSpacesRow:
		return paginateSpaceSlice(s, offset, limit)
	case []gen.ListSpacesPaginatedRow:
		return paginateSpaceSlice(s, offset, limit)
	default:
		return spaces
	}
}

// paginateSpaceSlice is a generic helper that applies offset/limit pagination
// to any slice type.
func paginateSpaceSlice[T any](s []T, offset, limit int32) []T {
	total := int32(len(s)) // #nosec G115 -- len() of a DB result slice
	if offset >= total {
		return []T{}
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return s[offset:end]
}

// spacesWithDetails resolves logo URLs and live member/post/follower counts for
// a slice of space rows of any sqlc-generated type, returning the list as maps.
// It uses reflection to avoid a type switch for every row variant; all space
// row structs share the "ID" and "LogoMediaID" fields. Counts are computed from
// the authoritative tables so negative/stale denormalised columns do not leak
// to the /spaces index.
func (h *Handlers) spacesWithDetails(ctx context.Context, tenantID pgtype.UUID, spaces any) []map[string]any {
	if spaces == nil {
		return nil
	}

	v := reflect.ValueOf(spaces)
	if v.Kind() != reflect.Slice {
		return nil
	}

	n := v.Len()
	out := make([]map[string]any, 0, n)
	ids := make([]pgtype.UUID, 0, n)

	for i := range n {
		elem := v.Index(i)

		idField := elem.FieldByName("ID")
		if !idField.IsValid() {
			continue
		}

		id, ok := idField.Interface().(pgtype.UUID)
		if !ok {
			continue
		}

		ids = append(ids, id)

		m := rowToMap(elem.Interface())

		if logoField := elem.FieldByName("LogoMediaID"); logoField.IsValid() {
			if logoID, ok := logoField.Interface().(pgtype.UUID); ok {
				m["logo_url"] = h.resolveLogoURL(ctx, tenantID, logoID)
			}
		}

		out = append(out, m)
	}

	stats, err := h.Service.CountSpaceStats(ctx, tenantID, ids)
	if err != nil {
		slog.Warn("spaces: failed to load space stats", "error", err)

		stats = nil
	}

	for _, m := range out {
		id, _ := m["id"].(string)
		if st, ok := stats[id]; ok {
			m["member_count"] = st.MemberCount
			m["post_count"] = st.PostCount
			m["follower_count"] = st.FollowerCount
		} else {
			// Best-effort clamp of any negative cached values when stats could
			// not be loaded.
			if mc, ok := m["member_count"].(float64); ok && int(mc) < 0 {
				m["member_count"] = 0
			}
		}
	}

	return out
}

// rowToMap serializes a space row to a map[string]any via a JSON round-trip,
// preserving the struct's json tags (so the output shape matches direct
// serialization of the typed row).
func rowToMap[T any](row T) map[string]any {
	b, err := json.Marshal(row)
	if err != nil {
		return map[string]any{}
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{}
	}

	return m
}

// AdminListSpaces handles GET /api/v1/admin/spaces — paginated spaces for the
// admin panel with owner and space_type name resolved via JOINs. Supports
// q (name/slug search), status, and visibility filter params.
func (h *Handlers) AdminListSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)
	search := r.URL.Query().Get("q")
	status := r.URL.Query().Get("status")
	visibility := r.URL.Query().Get("visibility")

	spaces, err := h.Service.AdminListSpaces(r.Context(), tenantID, search, status, visibility, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	total, err := h.Service.AdminListSpacesCount(r.Context(), tenantID, search, status, visibility)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	totalPages := int32(1)
	if limit > 0 {
		totalPages = int32((total + int64(limit) - 1) / int64(limit)) // #nosec G115 -- bounded by total/limit division
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":       spaces,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// ListMySpaces handles GET /api/v1/me/spaces — every space the caller belongs
// to. Super admins see every active space in the tenant so they can post to
// any space without joining it first (Section 03 — admin override).
func (h *Handlers) ListMySpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Optional q parameter for autocomplete search — filtered at the SQL
	// level by name/slug (case-insensitive ILIKE). Used by the /create
	// composer's space selector.
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	level := strings.TrimSpace(r.URL.Query().Get("level"))

	// Optional space_type filter. Empty or invalid values fall back to the
	// zero UUID sentinel, which the query treats as "no type filter".
	var spaceTypeID pgtype.UUID
	if err := spaceTypeID.Scan(r.URL.Query().Get("space_type")); err != nil || !spaceTypeID.Valid {
		spaceTypeID = pgtype.UUID{Valid: true}
	}

	// Optional page_size/page_offset. When omitted, apply a high default so
	// existing callers (dashboard, composer) continue to receive the full set
	// they expect, while the authenticated shell can request a small page.
	limit := parseMeSpacesLimit(r)
	offset := parseMeSpacesOffset(r)

	// Super admins bypass the membership requirement — they can post to any
	// space without joining it first (Section 03 — admin override).
	isSuperAdmin, err := h.Service.IsSuperAdmin(r.Context(), tenantID, user.UserID)
	if err != nil {
		slog.Warn("check super admin group", "error", err, "user_id", user.UserID)
	}

	if isSuperAdmin {
		spaces, lerr := h.Service.ListAllSpacesForAdmin(r.Context(), tenantID, user.UserID, q, level, spaceTypeID, limit, offset)
		if lerr != nil {
			handleServiceError(w, lerr)
			return
		}

		if spaces == nil {
			spaces = []gen.ListAllSpacesForAdminRow{}
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": spaces})

		return
	}

	spaces, err := h.Service.ListSpacesByMember(r.Context(), tenantID, user.UserID, q, level, spaceTypeID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if spaces == nil {
		spaces = []gen.ListSpacesByMemberUserRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": spaces})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	spaces, err := h.Service.ListPublicByMemberUser(r.Context(), tenantID, userID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toSpaceCards(spaces)})
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
// the caller moderates. Supports optional ?q, ?page_size and ?page_offset.
func (h *Handlers) ListMyModeratedSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := parseMeSpacesLimit(r)
	offset := parseMeSpacesOffset(r)

	spaces, err := h.Service.ListSpacesByModerator(r.Context(), tenantID, user.UserID, q, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if spaces == nil {
		spaces = []gen.ListSpacesByModeratorUserRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// ListMyOwnedSpaces handles GET /api/v1/me/spaces/owned — every space the
// caller owns (space_members.role = 'owner'). Distinct from /me/spaces which
// returns every space the caller belongs to as a member. Supports optional ?q,
// ?page_size and ?page_offset.
func (h *Handlers) ListMyOwnedSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := parseMeSpacesLimit(r)
	offset := parseMeSpacesOffset(r)

	spaces, err := h.Service.ListSpacesByOwner(r.Context(), tenantID, user.UserID, q, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if spaces == nil {
		spaces = []gen.ListSpacesByOwnerUserRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// GetSpace handles GET /api/v1/spaces/{slug}. It returns the space together
// with its rules so a single request can render a complete space view.
func (h *Handlers) GetSpace(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space slug is required")
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

	// Include keywords in the space response for the settings page.
	keywords, err := h.Service.GetSpaceKeywords(r.Context(), space.ID, tenantID)
	if err != nil {
		slog.Warn("spaces: failed to fetch keywords", "space_id", space.ID.String(), "error", err)

		keywords = []string{}
	}

	// Resolve logo_media_id to an imgproxy-signed logo_url.
	logoURL := h.resolveLogoURL(r.Context(), tenantID, space.LogoMediaID)
	// Resolve header_image_id to an imgproxy-signed header_url.
	headerURL := h.resolveHeaderURL(r.Context(), tenantID, space.HeaderImageID)

	// Replace drifted/negative denormalised counts with live computed values.
	if stats, err := h.Service.CountSpaceStats(r.Context(), tenantID, []pgtype.UUID{space.ID}); err == nil {
		if st, ok := stats[space.ID.String()]; ok {
			space.MemberCount = st.MemberCount
			space.PostCount = st.PostCount
			space.FollowerCount = st.FollowerCount
		}
	} else {
		slog.Warn("spaces: failed to load space stats", "space_id", space.ID.String(), "error", err)
	}

	// Surface viewer-scoped management flags so the frontend can render the
	// moderator assignment UI only for the space owner / an assigned
	// moderator (Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
	// lines 198-224; sections/03-rbac.md spaces.moderator.assign). Guests and
	// non-members get false; the mutating routes still enforce the permission
	// server-side, so a stale/true client flag cannot grant access.
	isOwner := false
	isModerator := false

	if user := auth.FromContext(r.Context()); user != nil && user.UserID.Valid {
		isOwner, err = h.Service.IsOwner(r.Context(), space.ID, user.UserID, tenantID)
		if err != nil {
			slog.Warn("spaces: IsOwner check failed", "space_id", space.ID.String(), "error", err)

			isOwner = false
		}

		isModerator, err = h.Service.IsModerator(r.Context(), space.ID, user.UserID, tenantID)
		if err != nil {
			slog.Warn("spaces: IsModerator check failed", "space_id", space.ID.String(), "error", err)

			isModerator = false
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"space":        space,
			"rules":        rules,
			"keywords":     keywords,
			"logo_url":     logoURL,
			"header_url":   headerURL,
			"is_owner":     isOwner,
			"is_moderator": isModerator,
		},
	})
}

// AdminGetSpace handles GET /api/v1/admin/spaces/{id} — returns a single space
// by UUID (not slug) for the admin panel.
func (h *Handlers) AdminGetSpace(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" ||
		strings.TrimSpace(req.SpaceTypeID) == "" || strings.TrimSpace(req.Level) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name, slug, space_type_id, and level are required")
		return
	}

	if !IsValidLevel(req.Level) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_level", "Level must be country, state, city, custom, or global")
		return
	}

	// Custom spaces must be linked to at least one primary space (Section 05).
	// The platform first keyword-matches the space name against primary geo
	// spaces; if that fails, the creator must explicitly provide a parent_id
	// or parent_space_ids. Parent linkage is resolved before creation.

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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
		errpkg.WriteError(w, http.StatusForbidden, "quota_exceeded",
			"Your account is not allowed to create spaces")
		return
	} else if maxSpaces > 0 {
		currentCount, err := h.Service.CountSpacesByOwner(r.Context(), tenantID, user.UserID)
		if err != nil {
			slog.Warn("create space: failed to count owned spaces",
				"user_id", user.UserID.String(), "error", err)
		} else if currentCount >= maxSpaces {
			errpkg.WriteError(w, http.StatusForbidden, "quota_exceeded",
				"You have reached the maximum number of spaces you can create")
			return
		}
	}

	var spaceTypeID pgtype.UUID
	if err := spaceTypeID.Scan(req.SpaceTypeID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space_type_id format")
		return
	}

	var parentID pgtype.UUID
	if req.ParentID != "" {
		if err := parentID.Scan(req.ParentID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid parent_id format")
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
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid parent_space_id format")
			return
		}

		parent, err := h.Service.GetSpace(r.Context(), psid, tenantID)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if parent.Level != LevelCountry && parent.Level != LevelState && parent.Level != LevelCity {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent",
				"Parent spaces must be primary (country, state, or city)")
			return
		}

		parentSpaceUUIDs = append(parentSpaceUUIDs, psid)
	}

	// If a single parent_id was provided, ensure it is also linked in the
	// space_parent_spaces junction so the hierarchy is consistent.
	if parentID.Valid && !parentSpaceUUIDsContains(parentSpaceUUIDs, parentID) {
		parentSpaceUUIDs = append([]pgtype.UUID{parentID}, parentSpaceUUIDs...)
	}

	// Section 05: custom spaces must be linked to at least one primary space.
	// If the creator did not provide an explicit parent, keyword-match the name
	// against primary geo spaces before creating. Reject creation if no match.
	if req.Level == LevelCustom && !parentID.Valid && len(parentSpaceUUIDs) == 0 {
		matchedParentID, matchedParentIDs, err := h.Service.LinkSpaceToPrimarySpaces(r.Context(), pgtype.UUID{}, tenantID, req.Name)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if !matchedParentID.Valid {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_primary_parent",
				"Custom spaces must be linked to at least one primary space. Select a country, state, or city, or include one in the name.")
			return
		}

		parentID = matchedParentID
		parentSpaceUUIDs = matchedParentIDs
	}

	// Resolve permission axes: from the named preset, or the defaults.
	axes := DefaultPermAxes()

	if req.PresetName != "" {
		preset, err := h.Service.GetPermissionPreset(r.Context(), req.PresetName, tenantID)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_preset", "Preset not found")
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

	space, err := h.Service.CreateSpaceWithOwner(r.Context(), gen.CreateSpaceParams{
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
	}, user.UserID, parentSpaceUUIDs)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Parent linkage was resolved before CreateSpaceWithOwner; see the
	// Section 05 pre-check above.

	// Enqueue async auto-population (Wikipedia summary, flag image, etc.).
	h.Service.EnqueueAutoPopulate(r.Context(), space.ID, tenantID)
	// Enqueue Meilisearch index sync so the new space is searchable
	// immediately (BL-004).
	h.Service.EnqueueSearchIndex(r.Context(), space.ID, tenantID, "index")

	// RAG indexing — fire-and-forget (§69/ADR-069 BUG-001 fix: spaces are
	// part of the RAG corpus).
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}

	h.indexSpaceRAG(r.Context(), tenantID, space.ID, req.Name, desc, nil)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": space})
}

// UpdateSpace handles PATCH /api/v1/spaces/{id}. It loads the existing space
// first so unspecified fields keep their current values (true PATCH
// semantics), then validates any provided permission axes before saving.
func (h *Handlers) UpdateSpace(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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
		Name           string   `json:"name"`
		Description    *string  `json:"description"`
		WikiContent    string   `json:"wiki_content"`
		GeoCode        *string  `json:"geo_code"`
		Visibility     string   `json:"visibility"`
		PostingPerm    string   `json:"posting_perm"`
		ReplyingPerm   string   `json:"replying_perm"`
		CommentingPerm string   `json:"commenting_perm"`
		JoiningPerm    string   `json:"joining_perm"`
		Status         string   `json:"status"`
		Keywords       []string `json:"keywords"`
		LogoMediaID    string   `json:"logo_media_id"`
		HeaderImageID  *string  `json:"header_image_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	// Validate each provided permission axis individually.
	if req.Visibility != "" && !validVisibility[req.Visibility] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid visibility value")
		return
	}

	if req.PostingPerm != "" && !validPostingPerm[req.PostingPerm] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid posting_perm value")
		return
	}

	if req.ReplyingPerm != "" && !validPostingPerm[req.ReplyingPerm] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid replying_perm value")
		return
	}

	if req.CommentingPerm != "" && !validPostingPerm[req.CommentingPerm] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid commenting_perm value")
		return
	}

	if req.JoiningPerm != "" && !validJoiningPerm[req.JoiningPerm] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid joining_perm value")
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

	// Track the latest space row for the response. Each update query returns
	// a distinct sqlc-generated Row type (structurally identical to gen.Space
	// minus Keywords), so hold the response in any and track the ID separately.
	var result any = space

	spaceID := space.ID

	// Update keywords separately if provided (nil = no change, [] = clear,
	// ["a","b"] = set). Uses a dedicated query to avoid NULL on NOT NULL column.
	if req.Keywords != nil {
		if err := h.Service.UpdateSpaceKeywords(r.Context(), id, tenantID, req.Keywords); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "keywords_update_failed", "Failed to update keywords")
			return
		}
	}

	// Update logo if provided. The frontend uploads via /media/upload flow
	// and passes the resulting media_id here.
	if req.LogoMediaID != "" {
		var logoUUID pgtype.UUID
		if err := logoUUID.Scan(req.LogoMediaID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_logo_media_id", "Invalid logo_media_id format")
			return
		}

		logoSpace, lErr := h.Service.UpdateSpaceLogoMediaID(r.Context(), gen.UpdateSpaceLogoMediaIDParams{
			LogoMediaID: logoUUID,
			ID:          id,
			TenantID:    tenantID,
		})
		if lErr != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "logo_update_failed", "Failed to update logo")
			return
		}

		result = logoSpace
		spaceID = logoSpace.ID
	}

	// Update header image if provided. A nil pointer clears the field; a
	// non-empty string sets it to the given media UUID.
	if req.HeaderImageID != nil {
		var headerUUID pgtype.UUID
		if *req.HeaderImageID != "" {
			if err := headerUUID.Scan(*req.HeaderImageID); err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_header_image_id", "Invalid header_image_id format")
				return
			}
		}

		headerSpace, hErr := h.Service.UpdateSpaceHeaderImageID(r.Context(), gen.UpdateSpaceHeaderImageIDParams{
			HeaderImageID: headerUUID,
			ID:            id,
			TenantID:      tenantID,
		})
		if hErr != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "header_update_failed", "Failed to update header image")
			return
		}

		result = headerSpace
		spaceID = headerSpace.ID
	}

	// Re-sync the Meilisearch index so search reflects the update (BL-004).
	h.Service.EnqueueSearchIndex(r.Context(), spaceID, tenantID, "index")

	// RAG re-indexing — fire-and-forget (§69/ADR-069 BUG-001 fix).
	updDesc := ""
	if description != nil {
		updDesc = *description
	}

	h.indexSpaceRAG(r.Context(), tenantID, spaceID, name, updDesc, req.Keywords)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DeleteSpace handles DELETE /api/v1/spaces/{id}.
// SPACES-002: enforces spec rule "creator only; must have 0 members"
// (Section 05 line 328). The route already requires spaces.delete
// permission; this handler additionally checks ownership and empties the
// space of members before soft-deleting.
func (h *Handlers) DeleteSpace(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// SPACES-002: only the space owner (creator) may delete the space.
	isOwner, err := h.Service.IsOwner(r.Context(), id, user.UserID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !isOwner {
		errpkg.WriteError(w, http.StatusForbidden, "not_space_owner",
			"Only the space creator can delete this space")
		return
	}

	// SPACES-002: the space must have 0 approved members (excluding the
	// owner themselves, who is always a member). A space with other members
	// cannot be deleted because it would orphan their membership.
	memberCount, err := h.Service.CountMembers(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if memberCount > 1 {
		errpkg.WriteError(w, http.StatusConflict, "space_not_empty",
			"Cannot delete a space that still has members")
		return
	}

	if err := h.Service.DeleteSpace(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Remove the space from the Meilisearch index (BL-004).
	h.Service.EnqueueSearchIndex(r.Context(), id, tenantID, "delete")

	// RAG purge — fire-and-forget (§69/ADR-069 BUG-001 fix: deleted spaces
	// must not be retrievable in RAG).
	h.purgeSpaceRAG(r.Context(), tenantID, id)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// parentSpaceUUIDsContains reports whether the given list already contains
// the given parent ID, comparing the raw UUID bytes.
func parentSpaceUUIDsContains(list []pgtype.UUID, id pgtype.UUID) bool {
	if !id.Valid {
		return false
	}

	for _, item := range list {
		if item.Valid && item.Bytes == id.Bytes {
			return true
		}
	}

	return false
}
