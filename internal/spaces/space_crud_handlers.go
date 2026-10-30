package spaces

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/bots"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/rbac"
)

// ---------------------------------------------------------------------------
// Space Handlers
// ---------------------------------------------------------------------------

// spaceSlugRe validates member-supplied space slugs (BUG-007); uuidLikeRe
// and reservedSlugs reject values that would collide with UUID-lookup or
// static sibling routes under /spaces/.
var spaceSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)
var uuidLikeRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var reservedSlugs = map[string]bool{
	"search": true, "regions": true, "subregions": true,
	"suggest-parents": true, "me": true, "admin": true, "new": true,
}

// ListSpaces handles GET /api/v1/spaces. Optional query params filter the
// result set: q (search by name/slug), level (by hierarchy level), type (by
// space_type_id), parent (by parent_id), geo (by geo_code), sort
// (members|followers|posts|name|newest). page_size/page_offset paginate the
// result. With no filters, all visible spaces for the platform are returned.
// SPACES-009/011: the default (no type/parent filter) branch uses SQL-level
// pagination, search, geo, and sort for performance and spec conformance.
func (h *Handlers) ListSpaces(w http.ResponseWriter, r *http.Request) {
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
	// ?joined=1 restricts the list to spaces the viewer is an approved member
	// of — the /spaces "My spaces" chip (audit fix: previously client-side
	// page-scoped filtering against the capped /me/spaces list).
	joinedOnly := q.Get("joined") == "1" || q.Get("joined") == "true"
	// ?member_created=1 restricts the list to member-created spaces (Section 05
	// amendment: level='custom' AND creator_user_id IS NOT NULL). The /spaces
	// directory always sends it; internal geo pickers (?level=, ?geo=) omit it
	// so primary spaces still resolve.
	memberCreated := q.Get("member_created") == "1" || q.Get("member_created") == "true"

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	var spaces any

	var total int

	switch {
	case q.Get("type") != "":
		// SPACES-009: the type branch shares the fully-filtered paginated
		// query so geo/region/subregion/level/joined/sort compose with the
		// type filter (previously every filter except search was silently
		// dropped here — audit fix).
		var spaceTypeID pgtype.UUID
		if scanErr := spaceTypeID.Scan(q.Get("type")); scanErr != nil {
			// §05 amendment — ?type= also accepts a type slug. 'university' is
			// a retired type slug: alias it to the education type and pin the
			// level so the filter still resolves platform universities.
			typeSlug := strings.ToLower(strings.TrimSpace(q.Get("type")))
			if typeSlug == "university" {
				typeSlug = "education"
				levelFilter = "university"
			}
			st, typeErr := h.Service.GetSpaceTypeBySlug(r.Context(), typeSlug)
			if typeErr != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid type filter format")
				return
			}
			spaceTypeID = st.ID
		}

		rows, err := h.Service.ListSpacesPaginated(r.Context(), viewerUserID, search, geoCode, levelFilter, region, subregion, sort, spaceTypeID, joinedOnly, memberCreated, int32(limit), int32(offset))
		if err != nil {
			handleServiceError(w, err)
			return
		}

		count, err := h.Service.CountSpaces(r.Context(), viewerUserID, search, geoCode, levelFilter, region, subregion, spaceTypeID, joinedOnly, memberCreated)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		spaces = rows
		total = int(count)
	case q.Get("parent") != "":
		// SPACES-009: the parent branch uses the filtered junction query so
		// search/geo/level/region/subregion/joined/sort all apply (previously
		// only level was honoured — audit fix).
		var parentID pgtype.UUID
		if scanErr := parentID.Scan(q.Get("parent")); scanErr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid parent filter format")
			return
		}

		rows, err := h.Service.ListSpacesByParentPaginated(r.Context(), viewerUserID, parentID, search, geoCode, levelFilter, region, subregion, sort, joinedOnly, memberCreated, int32(limit), int32(offset))
		if err != nil {
			handleServiceError(w, err)
			return
		}

		count, err := h.Service.CountSpacesByParent(r.Context(), viewerUserID, parentID, search, geoCode, levelFilter, region, subregion, joinedOnly, memberCreated)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		spaces = rows
		total = int(count)
	default:
		// SPACES-009/011: SQL-level pagination, search, geo, level, region,
		// subregion, and sort for the default branch.
		rows, err := h.Service.ListSpacesPaginated(r.Context(), viewerUserID, search, geoCode, levelFilter, region, subregion, sort, pgtype.UUID{}, joinedOnly, memberCreated, int32(limit), int32(offset))
		if err != nil {
			handleServiceError(w, err)
			return
		}

		count, err := h.Service.CountSpaces(r.Context(), viewerUserID, search, geoCode, levelFilter, region, subregion, pgtype.UUID{}, joinedOnly, memberCreated)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		spaces = rows
		total = int(count)
	}

	totalPages := total / int(limit)
	if total%int(limit) > 0 {
		totalPages++
	}

	// Resolve each space's logo_media_id to an imgproxy-signed logo_url so the
	// /spaces index can render space logos. Mirrors GetSpace's logo resolution.
	// Best-effort: spaces without a logo (or when the media resolver is not
	// wired) get an empty logo_url and the frontend renders a fallback icon.
	items := h.spacesWithDetails(r.Context(), viewerUserID, spaces)

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
	regions, err := h.Service.ListDistinctRegions(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ptrStringsToStrings(regions)})
}

// ListSubregionsByRegion handles GET /api/v1/spaces/subregions.
func (h *Handlers) ListSubregionsByRegion(w http.ResponseWriter, r *http.Request) {
	region := r.URL.Query().Get("region")
	if region == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_region", "region query parameter is required")
		return
	}

	subregions, err := h.Service.ListDistinctSubregions(r.Context(), region)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ptrStringsToStrings(subregions)})
}

// spacesWithDetails resolves logo URLs, live member/post/follower counts, and
// viewer-scoped relation flags (is_member / is_pending / is_following) for a
// slice of space rows of any sqlc-generated type, returning the list as maps.
// It uses reflection to avoid a type switch for every row variant; all space
// row structs share the "ID" and "LogoMediaID" fields. Counts are computed from
// the authoritative tables so negative/stale denormalised columns do not leak
// to the /spaces index.
func (h *Handlers) spacesWithDetails(ctx context.Context, viewerUserID pgtype.UUID, spaces any) []map[string]any {
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
	typeIDs := make(map[pgtype.UUID]struct{})
	rowTypeIDs := make([]pgtype.UUID, 0, n)

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

		var spaceTypeID pgtype.UUID

		if stField := elem.FieldByName("SpaceTypeID"); stField.IsValid() {
			if stID, ok := stField.Interface().(pgtype.UUID); ok {
				spaceTypeID = stID

				if _, seen := typeIDs[stID]; !seen {
					typeIDs[stID] = struct{}{}
				}
			}
		}

		rowTypeIDs = append(rowTypeIDs, spaceTypeID)

		m := rowToMap(elem.Interface())

		if logoField := elem.FieldByName("LogoMediaID"); logoField.IsValid() {
			if logoID, ok := logoField.Interface().(pgtype.UUID); ok && logoID.Valid {
				m["logo_url"] = logoID
			}
		}

		out = append(out, m)
	}

	// BUG-029: resolve logos concurrently — a sequential GetMedia per card
	// was an N+1 against media/imgproxy on every directory page.
	var wg sync.WaitGroup
	for _, m := range out {
		logoID, ok := m["logo_url"].(pgtype.UUID)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(m map[string]any, id pgtype.UUID) {
			defer wg.Done()
			m["logo_url"] = h.resolveLogoURL(ctx, id)
		}(m, logoID)
	}
	wg.Wait()

	// §80 wiki-only space types: per-row wiki_only on space list rows so the
	// frontend can suppress community CTAs without a per-card type lookup.
	if len(typeIDs) > 0 {
		typeIDList := make([]pgtype.UUID, 0, len(typeIDs))
		for tid := range typeIDs {
			typeIDList = append(typeIDList, tid)
		}

		if types, err := h.Service.GetSpaceTypesByIDs(ctx, typeIDList); err == nil {
			wikiOnlyByType := make(map[string]bool, len(types))
			for _, st := range types {
				wikiOnlyByType[st.ID.String()] = st.WikiOnly
			}

			for i, m := range out {
				m["wiki_only"] = wikiOnlyByType[rowTypeIDs[i].String()]
			}
		} else {
			slog.Warn("spaces: failed to load space types for wiki_only", "error", err)
		}
	}

	stats, err := h.Service.CountSpaceStats(ctx, ids)
	if err != nil {
		slog.Warn("spaces: failed to load space stats", "error", err)

		stats = nil
	}

	// Batch viewer-scoped relation flags — one query per page of spaces so the
	// /spaces cards render the correct Join/Pending/Joined and Follow state
	// without relying on the capped /me/spaces list (audit fix). Guests get
	// all-false flags.
	relations, relErr := h.Service.ListViewerSpaceRelations(ctx, viewerUserID, ids)
	if relErr != nil {
		slog.Warn("spaces: failed to load viewer relations", "error", relErr)

		relations = nil
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

		if rel, ok := relations[id]; ok {
			m["is_member"] = rel.IsMember
			m["is_pending"] = rel.IsPending
			m["is_following"] = rel.IsFollowing
		} else {
			m["is_member"] = false
			m["is_pending"] = false
			m["is_following"] = false
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
// q (name/slug search), status, visibility, space_type_id, deleted, sort/dir
// filter params.
func (h *Handlers) AdminListSpaces(w http.ResponseWriter, r *http.Request) {
	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)
	search := r.URL.Query().Get("q")
	status := r.URL.Query().Get("status")
	visibility := r.URL.Query().Get("visibility")
	deleted := r.URL.Query().Get("deleted")
	sortKey := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("dir")

	// Optional space-type filter — a zero UUID skips the filter at the SQL level.
	var spaceTypeID pgtype.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("space_type_id")); raw != "" {
		if err := spaceTypeID.Scan(raw); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space_type_id filter format")
			return
		}
	}

	spaces, err := h.Service.AdminListSpaces(r.Context(), search, status, visibility, deleted, spaceTypeID, sortKey, sortDir, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	total, err := h.Service.AdminListSpacesCount(r.Context(), search, status, visibility, deleted, spaceTypeID)
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
// to. Super admins see every active space in the platform so they can post to
// any space without joining it first (Section 03 — admin override).
func (h *Handlers) ListMySpaces(w http.ResponseWriter, r *http.Request) {
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
	// ?member_only=1 opts out: it forces the membership-only listing so
	// consumers that mean "spaces I belong to" (e.g. the LeftNav My spaces
	// switcher) still get an accurate list for super admins instead of an
	// arbitrary first page of every space in the platform.
	memberOnly := r.URL.Query().Get("member_only") == "1"
	// §05 amendment — ?member_created=1 restricts to member-created spaces
	// (composers: primary geo/university spaces auto-attach via keywords).
	memberCreated := r.URL.Query().Get("member_created") == "1" || r.URL.Query().Get("member_created") == "true"

	isSuperAdmin, err := h.Service.IsSuperAdmin(r.Context(), user.UserID)
	if err != nil {
		slog.Warn("check super admin group", "error", err, "user_id", user.UserID)
	}

	if isSuperAdmin && !memberOnly {
		spaces, lerr := h.Service.ListAllSpacesForAdmin(r.Context(), user.UserID, q, level, spaceTypeID, memberCreated, limit, offset)
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

	spaces, err := h.Service.ListSpacesByMember(r.Context(), user.UserID, q, level, spaceTypeID, memberCreated, limit, offset)
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
	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	spaces, err := h.Service.ListPublicByMemberUser(r.Context(), userID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toSpaceCards(spaces)})
}

// SpaceCard is the lean public shape for the profile Spaces tab — only the
// fields the profile card renders. It deliberately omits the internal columns
// the raw gen.Space row carries (, parent_id, space_type_id,
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
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := parseMeSpacesLimit(r)
	offset := parseMeSpacesOffset(r)

	spaces, err := h.Service.ListSpacesByModerator(r.Context(), user.UserID, q, limit, offset)
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
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := parseMeSpacesLimit(r)
	offset := parseMeSpacesOffset(r)

	spaces, err := h.Service.ListSpacesByOwner(r.Context(), user.UserID, q, limit, offset)
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

	space, err := h.Service.GetSpaceBySlug(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-001: non-public spaces (visibility='members' or 'private') are
	// hidden from non-members — same rule the list queries enforce. The
	// 'private' and 'admin_only' presets produce visibility='members', so
	// gating only 'private' leaked every members-only space by slug.
	if space.Visibility != VisibilityPublic && !h.canViewRestrictedSpace(r, space.ID) {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return
	}

	// BUG-006: non-active spaces (suspended/archived/draft) are offline —
	// suspension is a moderation freeze: managers and platform moderation
	// only, not ordinary members (matches the SpaceResolver gate).
	if space.Status != StatusActive && !h.canManageRestrictedSpace(r, space.ID) {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
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
	keywords, err := h.Service.GetSpaceKeywords(r.Context(), space.ID)
	if err != nil {
		slog.Warn("spaces: failed to fetch keywords", "space_id", space.ID.String(), "error", err)

		keywords = []string{}
	}

	// Resolve logo_media_id to an imgproxy-signed logo_url.
	logoURL := h.resolveLogoURL(r.Context(), space.LogoMediaID)
	// Resolve header_image_id to an imgproxy-signed header_url.
	headerURL := h.resolveHeaderURL(r.Context(), space.HeaderImageID)

	// Replace drifted/negative denormalised counts with live computed values.
	if stats, err := h.Service.CountSpaceStats(r.Context(), []pgtype.UUID{space.ID}); err == nil {
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
	isMember := false
	isPending := false
	isFollowing := false
	unreadCount := 0

	if user := auth.FromContext(r.Context()); user != nil && user.UserID.Valid {
		isOwner, err = h.Service.IsOwner(r.Context(), space.ID, user.UserID)
		if err != nil {
			slog.Warn("spaces: IsOwner check failed", "space_id", space.ID.String(), "error", err)

			isOwner = false
		}

		isModerator, err = h.Service.IsModerator(r.Context(), space.ID, user.UserID)
		if err != nil {
			slog.Warn("spaces: IsModerator check failed", "space_id", space.ID.String(), "error", err)

			isModerator = false
		}

		// Viewer-scoped relation flags — is_member/is_pending/is_following so
		// the hub renders the correct Join/Pending/Joined and Follow state for
		// any viewer, including super-admins for whom /me/spaces lists every
		// space regardless of actual membership (audit fix).
		if rels, relErr := h.Service.ListViewerSpaceRelations(r.Context(), user.UserID, []pgtype.UUID{space.ID}); relErr != nil {
			slog.Warn("spaces: viewer relation check failed", "space_id", space.ID.String(), "error", relErr)
		} else if rel, ok := rels[space.ID.String()]; ok {
			isMember = rel.IsMember
			isPending = rel.IsPending
			isFollowing = rel.IsFollowing
		}

		// BUG-016: visiting the space resets its unread counter (§05); the
		// pre-reset value is returned so the client can show the badge.
		if isMember && h.Unread != nil {
			unreadCount, _ = h.Unread.Get(r.Context(), user.UserID, space.ID)
			_ = h.Unread.Reset(r.Context(), user.UserID, space.ID)
		}
	}

	// Resolve template literals in the space description (best-effort —
	// space descriptions are authored via the shared TipTap editor which
	// offers the {{ token picker).
	if err := h.content.ResolveContentFields(r.Context(), &space, literals.NewRequestContext(r, "")); err != nil {
		slog.Warn("spaces: failed to resolve space literals", "error", err)
	}

	// §58 — resolved country currency for display surfaces (custom-field
	// currency adornments, price displays) — the space's own code when it's
	// a country, else its country ancestor's. Best-effort; "" when none.
	countryCurrency := h.Service.ResolveCountryCurrencyCode(r.Context(), space.ID, space.GeoCode)

	// §43 quality gate — same rule the spaces sitemap applies: public spaces
	// below the member/post/forum/question thresholds are thin pages and get
	// noindexed by the frontend (sitemap exclusion alone doesn't stop indexing).
	seoIndexable := space.Visibility == VisibilityPublic
	if seoIndexable {
		if members, posts, forums, questions, serr := h.Service.SpaceIndexSignals(r.Context(), space.ID); serr == nil {
			cfg := bots.DefaultSitemapQualityConfig()
			if h.settings != nil {
				cfg.MinMembers = h.settings.GetIntOr(r.Context(), "seo.sitemap.space_min_members", cfg.MinMembers)
				cfg.MinPosts = h.settings.GetIntOr(r.Context(), "seo.sitemap.space_min_posts", cfg.MinPosts)
				cfg.MinForums = h.settings.GetIntOr(r.Context(), "seo.sitemap.space_min_forums", cfg.MinForums)
				cfg.MinQuestions = h.settings.GetIntOr(r.Context(), "seo.sitemap.space_min_questions", cfg.MinQuestions)
			}
			seoIndexable = bots.IsSpaceIndexable(members, posts, forums, questions,
				cfg.MinMembers, cfg.MinPosts, cfg.MinForums, cfg.MinQuestions)
		} else {
			slog.Warn("spaces: index signals failed", "space_id", space.ID.String(), "error", serr)
			seoIndexable = false
		}
	}

	// §80 wiki-only space types — top-level flag on the detail envelope so
	// /spaces/{slug} can redirect to /wiki/{slug} and suppress community UI.
	wikiOnly := h.Service.IsWikiOnlyType(r.Context(), space.SpaceTypeID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"space":                 space,
			"seo_indexable":         seoIndexable,
			"rules":                 rules,
			"keywords":              keywords,
			"logo_url":              logoURL,
			"header_url":            headerURL,
			"is_owner":              isOwner,
			"is_moderator":          isModerator,
			"is_member":             isMember,
			"unread_count":          unreadCount,
			"is_pending":            isPending,
			"is_following":          isFollowing,
			"country_currency_code": countryCurrency,
			"wiki_only":             wikiOnly,
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

	space, err := h.Service.GetSpace(r.Context(), id)
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
		"data": map[string]any{
			"space":     space,
			"rules":     rules,
			"wiki_only": h.Service.IsWikiOnlyType(r.Context(), space.SpaceTypeID),
		},
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_level", "Level must be country, state, city, custom, university, or global")
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// BUG-003: primary levels (geo, university) are platform-seeded — members
	// create custom spaces only. Non-custom levels require the spaces.admin node.
	isAdmin := h.permChecker != nil && h.permChecker.HasPermission(r.Context(), user.UserID, "spaces.admin")
	if req.Level != LevelCustom && !isAdmin {
		errpkg.WriteError(w, http.StatusForbidden, "invalid_level",
			"Only platform admins can create primary-level spaces")
		return
	}

	// BUG-007: validate the slug — unvalidated slugs could collide with
	// static routes (/spaces/search, /spaces/regions, ...) or a raw UUID.
	slug := strings.ToLower(strings.TrimSpace(req.Slug))
	if !spaceSlugRe.MatchString(slug) || uuidLikeRe.MatchString(slug) || reservedSlugs[slug] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_slug",
			"Slug must be 2-64 chars of lowercase letters, numbers, or hyphens, and not reserved")
		return
	}

	// §26 / Section 03: enforce max_spaces_creatable quota from the caller's
	// user group. -1 = unlimited, 0 = none, N = capped at N spaces.
	maxSpaces, err := h.Service.GetUserMaxSpacesCreatable(r.Context(), user.UserID)
	if err != nil {
		// BUG-012: fail closed — a quota lookup error must not bypass the cap.
		slog.Warn("create space: failed to load max_spaces_creatable quota",
			"user_id", user.UserID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify space quota")
		return
	} else if maxSpaces == 0 {
		errpkg.WriteError(w, http.StatusForbidden, "quota_exceeded",
			"Your account is not allowed to create spaces")
		return
	} else if maxSpaces > 0 {
		currentCount, err := h.Service.CountSpacesByOwner(r.Context(), user.UserID)
		if err != nil {
			slog.Warn("create space: failed to count owned spaces",
				"user_id", user.UserID.String(), "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify space quota")
			return
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

		// BUG-004: parent_id must resolve to a primary (country/state/city)
		// space — the same rule applied to parent_space_ids below.
		parent, err := h.Service.GetSpace(r.Context(), parentID)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent", "Parent space not found")
			return
		}

		if parent.Status != StatusActive {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent", "Parent space is not active")
			return
		}

		if parent.Level != LevelCountry && parent.Level != LevelState && parent.Level != LevelCity {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent",
				"Parent spaces must be primary (country, state, or city)")
			return
		}
	}

	// BUG-002: validate each parent_space_id is a primary space (level IN
	// country/state/city) within the platform. Collect the parsed UUIDs for
	// linkage after the space is created.
	parentSpaceUUIDs := make([]pgtype.UUID, 0, len(req.ParentSpaceIDs))

	for _, psidStr := range req.ParentSpaceIDs {
		var psid pgtype.UUID
		if err := psid.Scan(psidStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid parent_space_id format")
			return
		}

		parent, err := h.Service.GetSpace(r.Context(), psid)
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
		geoCode := ""
		if req.GeoCode != nil {
			geoCode = *req.GeoCode
		}

		matchedParentID, matchedParentIDs, err := h.Service.LinkSpaceToPrimarySpaces(r.Context(), pgtype.UUID{}, req.Name, geoCode)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if !matchedParentID.Valid {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_primary_parent",
				"Custom spaces must be linked to at least one primary space — include a country, state, or city in the space name.")
			return
		}

		parentID = matchedParentID
		parentSpaceUUIDs = matchedParentIDs
	}

	// Resolve permission axes: from the named preset, or the defaults.
	axes := DefaultPermAxes()

	if req.PresetName != "" {
		preset, err := h.Service.GetPermissionPreset(r.Context(), req.PresetName)
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
		ParentID:       parentID,
		SpaceTypeID:    spaceTypeID,
		Name:           req.Name,
		Slug:           slug,
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

	// BUG-017: auto-population (Wikipedia/flag enrichment) is only
	// meaningful for geo spaces and universities — skip it for ordinary
	// member-created custom spaces.
	if st, terr := h.Service.GetSpaceType(r.Context(), spaceTypeID); terr == nil &&
		(req.Level != LevelCustom || st.WikiOnly) {
		h.Service.EnqueueAutoPopulate(r.Context(), space.ID)
	}
	// Enqueue Meilisearch index sync so the new space is searchable
	// immediately (BL-004).
	h.Service.EnqueueSearchIndex(r.Context(), space.ID, "index")

	// RAG indexing — fire-and-forget (§69/ADR-069 BUG-001 fix: spaces are
	// part of the RAG corpus).
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}

	h.indexSpaceRAG(r.Context(), space.ID, axes.Visibility, req.Name, desc, nil)

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

	existing, err := h.Service.GetSpace(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// SPACES-017: spaces.edit is granted to member_role as "edit own spaces"
	// (migration 000175) — the handler must check management rights per space.
	// Only the space owner, an assigned moderator, or a platform admin
	// (spaces.admin — the /admin/spaces/{id} route's gate) may update.
	if !h.canManageOrAdmin(w, r, id) {
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

	// Monetization is mutually exclusive (§05): paid axes conflict with an
	// active tips configuration — owner must disable tips first.
	if (req.JoiningPerm == "paid" || req.Visibility == "paid") &&
		h.tipSettings != nil && h.tipSettings.SpaceTipsEnabled(r.Context(), id) {
		errpkg.WriteError(w, http.StatusConflict, "monetization_conflict",
			"Disable tips before enabling paid membership — a space is either tips-funded or paid")
		return
	}

	// BUG-005: status transitions are admin-only. Without this, a space
	// owner could re-activate a space an admin suspended (moderation
	// bypass), or write a value outside the CHECK constraint.
	if req.Status != "" {
		if req.Status != StatusActive && req.Status != StatusArchived &&
			req.Status != StatusDraft && req.Status != "suspended" {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Invalid status value")
			return
		}

		statusAdmin := false
		if resolver := rbac.FromContext(r.Context()); resolver != nil {
			if u := auth.FromContext(r.Context()); u != nil && u.UserID.Valid {
				statusAdmin = resolver.HasPermission(r.Context(), u.UserID, "spaces.admin")
			}
		}

		if !statusAdmin {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Only platform admins can change space status")
			return
		}
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
		if err := h.Service.UpdateSpaceKeywords(r.Context(), id, req.Keywords); err != nil {
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

		if err := h.ensureImageMedia(r.Context(), logoUUID, "logo_media_id"); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_logo_media_id", err.Error())
			return
		}

		logoSpace, lErr := h.Service.UpdateSpaceLogoMediaID(r.Context(), gen.UpdateSpaceLogoMediaIDParams{
			LogoMediaID: logoUUID,
			ID:          id,
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

			if err := h.ensureImageMedia(r.Context(), headerUUID, "header_image_id"); err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_header_image_id", err.Error())
				return
			}
		}

		headerSpace, hErr := h.Service.UpdateSpaceHeaderImageID(r.Context(), gen.UpdateSpaceHeaderImageIDParams{
			HeaderImageID: headerUUID,
			ID:            id,
		})
		if hErr != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "header_update_failed", "Failed to update header image")
			return
		}

		result = headerSpace
		spaceID = headerSpace.ID
	}

	// Re-sync the Meilisearch index so search reflects the update (BL-004).
	h.Service.EnqueueSearchIndex(r.Context(), spaceID, "index")

	// RAG re-indexing — fire-and-forget (§69/ADR-069 BUG-001 fix).
	updDesc := ""
	if description != nil {
		updDesc = *description
	}

	h.indexSpaceRAG(r.Context(), spaceID, visibility, name, updDesc, req.Keywords)

	// Scoped retrieval (§69 amendment): a visibility/status change must
	// refresh the space's embedding rows AND every thread row inside it —
	// threads inherit space visibility. Fire-and-forget per Rule 8.
	go func() { // #nosec G118 -- request ctx is cancelled on HTTP return
		if err := h.ragIndexer.ReconcileSpaceScope(context.Background(), spaceID, visibility); err != nil {
			slog.Warn("spaces: RAG scope reconcile failed", "space_id", spaceID.String(), "error", err)
		}
	}()

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

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// SPACES-002: only the space owner (creator) may delete the space —
	// or a platform admin via /admin/spaces/{id} (spaces.admin), since the
	// admin panel's delete action must work on non-owned spaces too.
	isOwner, err := h.Service.IsOwner(r.Context(), id, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	isAdmin := false
	if resolver := rbac.FromContext(r.Context()); resolver != nil {
		isAdmin = resolver.HasPermission(r.Context(), user.UserID, "spaces.admin")
	}

	// ?force=true permanently removes an already-soft-deleted space
	// (admin-only). The guarded query refuses while subspaces exist and
	// returns 409 not_deleted on a live row.
	if r.URL.Query().Get("force") == "true" {
		if !isAdmin {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden",
				"Permanent delete requires the spaces.admin permission")
			return
		}

		if err := h.Service.HardDeleteSpace(r.Context(), id); err != nil {
			handleServiceError(w, err)
			return
		}

		h.Service.EnqueueSearchIndex(r.Context(), id, "delete")
		h.purgeSpaceRAG(r.Context(), id)
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted", "force": "true"}})

		return
	}

	if !isOwner {
		if !isAdmin {
			errpkg.WriteError(w, http.StatusForbidden, "not_space_owner",
				"Only the space creator can delete this space")
			return
		}
	}

	// SPACES-002: the space must have 0 approved members (excluding the
	// owner themselves, who is always a member). A space with other members
	// cannot be deleted because it would orphan their membership.
	memberCount, err := h.Service.CountMembers(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-014: platform admins may delete regardless of member count —
	// otherwise a populated space could never be removed.
	if memberCount > 1 && !isAdmin {
		errpkg.WriteError(w, http.StatusConflict, "space_not_empty",
			"Cannot delete a space that still has members")
		return
	}

	// Scoped retrieval (§69 amendment): threads survive deletion via ON
	// DELETE SET NULL — orphan their embeddings fail-closed BEFORE the
	// delete commits, while they still carry the space_id needed to match.
	// A reconcile miss here leaks the space's stale visibility until the
	// next reindex sweep, so run synchronously but never block the delete.
	if h.ragIndexer != nil {
		if err := h.ragIndexer.ReconcileSpaceDelete(r.Context(), id); err != nil {
			slog.Warn("spaces: RAG orphan reconcile failed", "space_id", id.String(), "error", err)
		}
	}

	if err := h.Service.DeleteSpace(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	// Remove the space from the Meilisearch index (BL-004).
	h.Service.EnqueueSearchIndex(r.Context(), id, "delete")

	// RAG purge — fire-and-forget (§69/ADR-069 BUG-001 fix: deleted spaces
	// must not be retrievable in RAG).
	h.purgeSpaceRAG(r.Context(), id)

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
