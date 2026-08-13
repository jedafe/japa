package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/tenant"
)

// radarItem is the v1 radar feed item shape. In v1 the radar surfaces
// promoted scholarships only; future domains (jobs, visa alerts) will be
// unioned in via the same envelope once their projection tables exist.
type radarItem struct {
	ID                pgtype.UUID    `json:"id"`
	Type              string         `json:"type"`
	Title             string         `json:"title"`
	Description       *string        `json:"description,omitempty"`
	Country           string         `json:"country"`
	University        *string        `json:"university,omitempty"`
	DegreeLevel       *string        `json:"degree_level,omitempty"`
	Deadline          pgtype.Date    `json:"deadline"`
	ApplicationURL    *string        `json:"application_url,omitempty"`
	ConfidenceOverall float32        `json:"confidence_overall"`
	TrustLevel        int16          `json:"trust_level"`
	FreshnessScore    int32          `json:"freshness_score"`
	SourceURL         string         `json:"source_url"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
}

func scholarshipToRadarItem(s gen.Scholarship) radarItem {
	return radarItem{
		ID:                s.ID,
		Type:              "scholarship",
		Title:             s.Title,
		Description:       s.Description,
		Country:           s.Country,
		University:        s.University,
		DegreeLevel:       s.DegreeLevel,
		Deadline:          s.Deadline,
		ApplicationURL:    s.ApplicationUrl,
		ConfidenceOverall: s.ConfidenceOverall,
		TrustLevel:        s.TrustLevel,
		FreshnessScore:    s.FreshnessScore,
		SourceURL:         s.SourceUrl,
		CreatedAt:         s.CreatedAt,
	}
}

// encodeCursor encodes a keyset cursor (created_at + id) as a base64 string
// for the next_cursor field in the spec envelope. The cursor is opaque to
// the client; the handler decodes it to resume pagination.
func encodeCursor(createdAt pgtype.Timestamptz, id pgtype.UUID) string {
	if !createdAt.Valid || !id.Valid {
		return ""
	}
	raw := fmt.Sprintf("%d|%s", createdAt.Time.UnixNano(), id.String())
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// parseCursor decodes a base64 keyset cursor. Returns the created_at
// timestamp and id; ok=false if the cursor is empty or malformed.
func parseCursor(cursor string) (ts pgtype.Timestamptz, id pgtype.UUID, ok bool) {
	if cursor == "" {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	parts := splitOnce(raw, '|')
	if len(parts) != 2 {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	nano, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	if err := id.Scan(parts[1]); err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	ts = pgtype.Timestamptz{Time: timeFromNano(nano), Valid: true}
	return ts, id, true
}

// splitOnce splits s at the first occurrence of sep.
func splitOnce(s []byte, sep byte) []string {
	for i, b := range s {
		if b == sep {
			return []string{string(s[:i]), string(s[i+1:])}
		}
	}
	return []string{string(s)}
}

// timeFromNano converts a UnixNano value to a time.Time.
func timeFromNano(nano int64) time.Time {
	return time.Unix(0, nano).UTC()
}

// countByType aggregates radar items by their Type field.
func countByType(items []radarItem) map[string]int {
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Type]++
	}
	return counts
}

// countByCountry aggregates radar items by their Country field.
func countByCountry(items []radarItem) map[string]int {
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Country]++
	}
	return counts
}

// HandleRadarFeed returns the geo-scoped radar feed. In v1 this is the set
// of promoted scholarships filtered by the caller's space_id query param
// (or country when space_id is absent). Anonymous callers get the global
// feed (no space filter).
//
// GET /api/v1/intel/radar
func (h *Handlers) HandleRadarFeed(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, _ := optionalUser(w, r)
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	page, limit := parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	// BUG-006 fix: the spec §731-733 requires keyset pagination via ?cursor=.
	// The cursor encodes (created_at, id). When present, decode it and use a
	// keyset query (WHERE (created_at, id) < (cursor_ts, cursor_id)); when
	// absent, fall back to offset pagination for the first page. The
	// next_cursor is still returned so clients can resume.
	cursorStr := q.Get("cursor")
	cursorTS, cursorID, hasCursor := parseCursor(cursorStr)

	spaceIDStr := q.Get("space_id")
	country := q.Get("country")

	var rows []gen.Scholarship
	var err error

	switch {
	case hasCursor:
		// Keyset path: resume after the cursor item, ignoring space/country
		// filters (the cursor already encodes the prior page's scope). This
		// keeps the global feed paginating consistently.
		rows, err = h.intelStores.ListScholarshipsKeyset(r.Context(), tenantID, gen.ListScholarshipsKeysetParams{
			Column2: cursorTS,
			ID:      cursorID,
			Limit:   clampInt32(limit),
		})
	case spaceIDStr != "":
		var spaceID pgtype.UUID
		if e := spaceID.Scan(spaceIDStr); e != nil {
			writeError(w, http.StatusBadRequest, "invalid_uuid", "Invalid space_id")
			return
		}
		rows, err = h.intelStores.ListScholarshipsBySpace(r.Context(), tenantID, gen.ListScholarshipsBySpaceParams{
			CountrySpaceID: spaceID,
			Limit:          clampInt32(limit),
			Offset:         offset,
		})
	case country != "":
		rows, err = h.intelStores.ListScholarshipsByCountry(r.Context(), tenantID, gen.ListScholarshipsByCountryParams{
			Country: country,
			Limit:   clampInt32(limit),
			Offset:  offset,
		})
	default:
		rows, err = h.intelStores.ListScholarships(r.Context(), tenantID, gen.ListScholarshipsParams{
			Limit:  clampInt32(limit),
			Offset: offset,
		})
	}
	if err != nil {
		slog.Error("intel: radar feed query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "radar_feed_failed", "Failed to load radar feed")
		return
	}

	items := make([]radarItem, 0, len(rows))
	for _, s := range rows {
		items = append(items, scholarshipToRadarItem(s))
	}

	// Keyset pagination: if the result set equals the limit, there may be
	// more items. Encode the last item's (created_at, id) as the cursor.
	hasMore := len(items) >= limit
	nextCursor := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	// Resolve the caller's followed space IDs so the UI can badge items
	// whose country space the user follows. Best-effort: a failure
	// degrades to an empty slice.
	followedSpaces := h.resolveFollowedSpaceIDs(r.Context(), tenantID, userID)

	// Spec envelope: {items, next_cursor, has_more, counts_by_type,
	// counts_by_country, followed_space_ids} (BUG-03 fix, §3.3).
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":              items,
		"next_cursor":        nextCursor,
		"has_more":           hasMore,
		"counts_by_type":     countByType(items),
		"counts_by_country":  countByCountry(items),
		"followed_space_ids": followedSpaces,
	})
}

// HandleRadarCounts returns per-country scholarship counts for the dashboard
// widget. The widget renders a world map heat-map; this endpoint supplies the
// counts that drive the colour intensity per country.
//
// GET /api/v1/intel/radar/counts
func (h *Handlers) HandleRadarCounts(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}
	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = defaultIntelTenantID
	}

	rows, err := h.intelStores.CountScholarshipsByCountry(r.Context(), tenantID)
	if err != nil {
		slog.Error("intel: radar counts query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "counts_failed", "Failed to load radar counts")
		return
	}
	if rows == nil {
		rows = []gen.CountScholarshipsByCountryRow{}
	}

	total := int64(0)
	byCountry := make(map[string]int64, len(rows))
	for _, r := range rows {
		total += r.Count
		byCountry[r.Country] = r.Count
	}

	// Spec envelope: {total_new, by_type, by_country} (BUG-04 fix, §3.3).
	// In v1 all radar items are scholarships, so by_type has a single key.
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_new":  total,
		"by_type":    map[string]int64{"scholarship": total},
		"by_country": byCountry,
	})
}

// HandleRadarFollowed returns the radar feed scoped to the caller's followed
// country spaces. It resolves the user's followed "space" entities via the
// follows service, then fetches scholarships for ALL followed spaces in a
// single query (BUG-008b fix: the prior per-space loop was an N+1 — one
// query per followed country).
//
// BUG-008a fix: the response now uses the radar envelope
// {items, next_cursor, has_more, counts_by_type, counts_by_country,
// followed_space_ids} per spec §739 (same envelope as /radar), not the
// generic {data, meta} envelope.
//
// GET /api/v1/intel/radar/followed
func (h *Handlers) HandleRadarFollowed(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil || h.followsSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	_, limit := parsePagination(r)

	followed, err := h.followsSvc.GetFollowingByType(r.Context(), tenantID, userID, userID, follows.EntityTypeSpace, 100, 0)
	if err != nil {
		slog.Error("intel: radar followed spaces lookup failed", "user_id", userID.String(), "error", err)
		writeError(w, http.StatusInternalServerError, "followed_spaces_failed", "Failed to load followed spaces")
		return
	}

	// Collect followed space IDs and run a single union query.
	spaceIDs := make([]pgtype.UUID, 0, len(followed))
	followedSpaceStrs := make([]string, 0, len(followed))
	for _, f := range followed {
		if f.EntityID.Valid {
			spaceIDs = append(spaceIDs, f.EntityID)
			followedSpaceStrs = append(followedSpaceStrs, f.EntityID.String())
		}
	}

	var rows []gen.Scholarship
	if len(spaceIDs) > 0 {
		rows, err = h.intelStores.ListScholarshipsBySpaces(r.Context(), tenantID, gen.ListScholarshipsBySpacesParams{
			Column2: spaceIDs,
			Limit:   clampInt32(limit),
		})
		if err != nil {
			slog.Error("intel: radar followed union query failed", "error", err)
			writeError(w, http.StatusInternalServerError, "followed_feed_failed", "Failed to load followed feed")
			return
		}
	}

	items := make([]radarItem, 0, len(rows))
	for _, s := range rows {
		items = append(items, scholarshipToRadarItem(s))
	}

	hasMore := len(items) >= limit
	nextCursor := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	// Radar envelope (spec §739), NOT the generic {data, meta} envelope.
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":              items,
		"next_cursor":        nextCursor,
		"has_more":           hasMore,
		"counts_by_type":     countByType(items),
		"counts_by_country":  countByCountry(items),
		"followed_space_ids": followedSpaceStrs,
	})
}

// HandleRadarFollow follows a country space. The body carries the space_id;
// the handler delegates to the follows service with entity_type "space" so
// the radar reuses the polymorphic follows table.
//
// POST /api/v1/intel/radar/follow
func (h *Handlers) HandleRadarFollow(w http.ResponseWriter, r *http.Request) {
	if h.followsSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var body struct {
		SpaceID string `json:"space_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if body.SpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_param", "space_id is required")
		return
	}

	var spaceID pgtype.UUID
	if err := spaceID.Scan(body.SpaceID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_uuid", "Invalid space_id")
		return
	}

	followed, err := h.followsSvc.ToggleFollow(r.Context(), tenantID, userID, follows.EntityTypeSpace, spaceID)
	if err != nil {
		slog.Error("intel: radar follow failed", "space_id", body.SpaceID, "error", err)
		writeError(w, http.StatusBadRequest, "follow_failed", "Failed to follow space")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"space_id":    body.SpaceID,
			"following":   followed,
		},
	})
}

// HandleRadarUnfollow unfollows a country space. Per the spec the space_id
// is carried in the request body ({ space_id }). A query-string fallback
// (?space_id=) is kept for backward compatibility with bodyless DELETE
// clients (BUG-15 fix, §3.3).
//
// DELETE /api/v1/intel/radar/follow
func (h *Handlers) HandleRadarUnfollow(w http.ResponseWriter, r *http.Request) {
	if h.followsSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// Read space_id from body per spec; fall back to query string.
	spaceIDStr := r.URL.Query().Get("space_id")
	if spaceIDStr == "" && r.Body != nil {
		var body struct {
			SpaceID string `json:"space_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.SpaceID != "" {
			spaceIDStr = body.SpaceID
		}
	}
	if spaceIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing_param", "space_id is required")
		return
	}
	var spaceID pgtype.UUID
	if err := spaceID.Scan(spaceIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_uuid", "Invalid space_id")
		return
	}

	followed, err := h.followsSvc.ToggleFollow(r.Context(), tenantID, userID, follows.EntityTypeSpace, spaceID)
	if err != nil {
		slog.Error("intel: radar unfollow failed", "space_id", spaceIDStr, "error", err)
		writeError(w, http.StatusBadRequest, "unfollow_failed", "Failed to unfollow space")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"space_id":  spaceIDStr,
			"following": followed,
		},
	})
}

// HandleRadarItemDetail returns a single radar item (scholarship) by its
// id. The route parameter is the scholarship projection row id.
//
// GET /api/v1/intel/radar/feed/{item_id}
func (h *Handlers) HandleRadarItemDetail(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}
	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = defaultIntelTenantID
	}

	itemID, ok := scanUUIDParam(w, r, "item_id")
	if !ok {
		return
	}

	s, err := h.intelStores.GetScholarship(r.Context(), tenantID, itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}
		slog.Error("intel: radar item detail lookup failed", "item_id", itemID.String(), "error", err)
		writeError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load radar item")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": scholarshipToRadarItem(s),
	})
}

// HandleRadarSaved lists the caller's saved radar opportunities. In v1 these
// are saved scholarships; the handler delegates to the scholarships service
// so the radar reuses the existing saved-scholarship tracker.
//
// GET /api/v1/intel/radar/saved
func (h *Handlers) HandleRadarSaved(w http.ResponseWriter, r *http.Request) {
	if h.scholarSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	page, limit := parsePagination(r)

	rows, total, err := h.scholarSvc.ListSavedScholarships(r.Context(), tenantID, userID, status, page, limit)
	if err != nil {
		slog.Error("intel: radar saved list failed", "user_id", userID.String(), "error", err)
		writeError(w, http.StatusInternalServerError, "saved_list_failed", "Failed to load saved opportunities")
		return
	}
	if rows == nil {
		rows = []scholarships.SavedScholarshipResponse{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": rows,
		"meta": map[string]interface{}{
			"total":       total,
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
		},
	})
}

// HandleRadarSave saves a radar opportunity to the caller's tracker. The body
// carries the item_id (scholarship projection row id); the handler resolves
// the backing guide_id and delegates to the scholarships service.
//
// POST /api/v1/intel/radar/save
func (h *Handlers) HandleRadarSave(w http.ResponseWriter, r *http.Request) {
	if h.scholarSvc == nil || h.intelStores == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var body struct {
		ItemID string `json:"item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if body.ItemID == "" {
		writeError(w, http.StatusBadRequest, "missing_param", "item_id is required")
		return
	}

	var itemID pgtype.UUID
	if err := itemID.Scan(body.ItemID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_uuid", "Invalid item_id")
		return
	}

	// Resolve the scholarship projection row to get its guide_id — the
	// saved-scholarship tracker keys on guide_id, not scholarship.id.
	s, err := h.intelStores.GetScholarship(r.Context(), tenantID, itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}
		slog.Error("intel: radar save lookup failed", "item_id", itemID.String(), "error", err)
		writeError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load radar item")
		return
	}

	saved, err := h.scholarSvc.SaveScholarship(r.Context(), tenantID, userID, s.GuideID)
	if err != nil {
		if errors.Is(err, scholarships.ErrGuideNotFound) {
			writeError(w, http.StatusNotFound, "guide_not_found", "Guide not found for this opportunity")
			return
		}
		slog.Error("intel: radar save failed", "item_id", itemID.String(), "error", err)
		writeError(w, http.StatusBadRequest, "save_failed", "Failed to save opportunity")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"id":                 pgUUIDString(saved.ID),
			"guide_id":           pgUUIDString(saved.GuideID),
			"status":             saved.Status,
			"remind_before_days": saved.RemindBeforeDays,
		},
	})
}

// HandleRadarUnsave removes a radar opportunity from the caller's tracker.
// The item_id path parameter is the scholarship projection row id; the
// handler resolves the guide_id and delegates to the scholarships service.
//
// DELETE /api/v1/intel/radar/save/{item_id}
func (h *Handlers) HandleRadarUnsave(w http.ResponseWriter, r *http.Request) {
	if h.scholarSvc == nil || h.intelStores == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	itemID, ok := scanUUIDParam(w, r, "item_id")
	if !ok {
		return
	}

	s, err := h.intelStores.GetScholarship(r.Context(), tenantID, itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}
		slog.Error("intel: radar unsave lookup failed", "item_id", itemID.String(), "error", err)
		writeError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load radar item")
		return
	}

	if err := h.scholarSvc.DeleteSavedScholarship(r.Context(), tenantID, userID, s.GuideID); err != nil {
		slog.Error("intel: radar unsave failed", "item_id", itemID.String(), "error", err)
		writeError(w, http.StatusBadRequest, "unsave_failed", "Failed to unsave opportunity")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{"deleted": true},
	})
}

// resolveFollowedSpaceIDs returns the set of space IDs the user follows, as
// a slice of strings. It is best-effort: any error returns an empty slice.
func (h *Handlers) resolveFollowedSpaceIDs(ctx context.Context, tenantID, userID pgtype.UUID) []string {
	if h.followsSvc == nil || !userID.Valid {
		return []string{}
	}
	followed, err := h.followsSvc.GetFollowingByType(ctx, tenantID, userID, userID, follows.EntityTypeSpace, 100, 0)
	if err != nil {
		return []string{}
	}
	out := make([]string, 0, len(followed))
	for _, f := range followed {
		if f.EntityID.Valid {
			out = append(out, f.EntityID.String())
		}
	}
	return out
}

// pgUUIDString returns the string form of a pgtype.UUID, or "" when invalid.
func pgUUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}
