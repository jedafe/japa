package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/scholarships"
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
	offset := int32((page - 1) * limit)

	spaceIDStr := q.Get("space_id")
	country := q.Get("country")

	var rows []gen.Scholarship
	var err error

	switch {
	case spaceIDStr != "":
		var spaceID pgtype.UUID
		if e := spaceID.Scan(spaceIDStr); e != nil {
			writeError(w, http.StatusBadRequest, "invalid_uuid", "Invalid space_id")
			return
		}
		rows, err = h.intelStores.ListScholarshipsBySpace(r.Context(), gen.ListScholarshipsBySpaceParams{
			CountrySpaceID: spaceID,
			Limit:          int32(limit),
			Offset:         offset,
		})
	case country != "":
		rows, err = h.intelStores.ListScholarshipsByCountry(r.Context(), gen.ListScholarshipsByCountryParams{
			Country: country,
			Limit:   int32(limit),
			Offset:  offset,
		})
	default:
		rows, err = h.intelStores.ListScholarships(r.Context(), gen.ListScholarshipsParams{
			Limit:  int32(limit),
			Offset: offset,
		})
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "radar_feed_failed", err.Error())
		return
	}

	items := make([]radarItem, 0, len(rows))
	for _, s := range rows {
		items = append(items, scholarshipToRadarItem(s))
	}

	// Resolve the caller's followed space IDs so the UI can badge items
	// whose country space the user follows. Best-effort: a failure
	// degrades to an empty slice.
	followedSpaces := h.resolveFollowedSpaceIDs(r.Context(), tenantID, userID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": items,
		"meta": map[string]interface{}{
			"page_size":          limit,
			"page_offset":        (page - 1) * limit,
			"count":              len(items),
			"followed_space_ids": followedSpaces,
		},
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

	rows, err := h.intelStores.CountScholarshipsByCountry(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "counts_failed", err.Error())
		return
	}
	if rows == nil {
		rows = []gen.CountScholarshipsByCountryRow{}
	}

	total := int64(0)
	for _, r := range rows {
		total += r.Count
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": rows,
		"meta": map[string]interface{}{
			"total_countries": len(rows),
			"total_scholarships": total,
		},
	})
}

// HandleRadarFollowed returns the radar feed scoped to the caller's followed
// country spaces. It resolves the user's followed "space" entities via the
// follows service, then unions the scholarships for each followed space.
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

	page, limit := parsePagination(r)
	offset := int32((page - 1) * limit)

	followed, err := h.followsSvc.GetFollowingByType(r.Context(), tenantID, userID, userID, follows.EntityTypeSpace, 100, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "followed_spaces_failed", err.Error())
		return
	}

	// Collect scholarships across all followed spaces. In v1 we fetch per
	// space and merge; a dedicated union query can replace this once the
	// feed grows beyond a handful of followed spaces.
	var allItems []radarItem
	for _, f := range followed {
		rows, err := h.intelStores.ListScholarshipsBySpace(r.Context(), gen.ListScholarshipsBySpaceParams{
			CountrySpaceID: f.EntityID,
			Limit:          int32(limit),
			Offset:         offset,
		})
		if err != nil {
			continue // best-effort per space
		}
		for _, s := range rows {
			allItems = append(allItems, scholarshipToRadarItem(s))
		}
	}

	// Clamp to the requested page size after merging.
	if len(allItems) > limit {
		allItems = allItems[:limit]
	}
	if allItems == nil {
		allItems = []radarItem{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": allItems,
		"meta": map[string]interface{}{
			"page_size":      limit,
			"page_offset":    (page - 1) * limit,
			"count":          len(allItems),
			"followed_count": len(followed),
		},
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
		writeError(w, http.StatusBadRequest, "follow_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"space_id":    body.SpaceID,
			"following":   followed,
		},
	})
}

// HandleRadarUnfollow unfollows a country space. The space_id is read from
// the query string so DELETE can stay bodyless.
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

	spaceIDStr := r.URL.Query().Get("space_id")
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
		writeError(w, http.StatusBadRequest, "unfollow_failed", err.Error())
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

	itemID, ok := scanUUIDParam(w, r, "item_id")
	if !ok {
		return
	}

	s, err := h.intelStores.GetScholarship(r.Context(), itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup_failed", err.Error())
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
		writeError(w, http.StatusInternalServerError, "saved_list_failed", err.Error())
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
	s, err := h.intelStores.GetScholarship(r.Context(), itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup_failed", err.Error())
		return
	}

	saved, err := h.scholarSvc.SaveScholarship(r.Context(), tenantID, userID, s.GuideID)
	if err != nil {
		if errors.Is(err, scholarships.ErrGuideNotFound) {
			writeError(w, http.StatusNotFound, "guide_not_found", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "save_failed", err.Error())
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

	s, err := h.intelStores.GetScholarship(r.Context(), itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup_failed", err.Error())
		return
	}

	if err := h.scholarSvc.DeleteSavedScholarship(r.Context(), tenantID, userID, s.GuideID); err != nil {
		writeError(w, http.StatusBadRequest, "unsave_failed", err.Error())
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
