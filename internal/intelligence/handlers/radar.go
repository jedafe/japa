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
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/tenant"
)

// radarItem is the v1 radar feed item shape. The radar surfaces promoted
// entities from all intelligence domains (scholarships, jobs, visa routes,
// housing guides, remittance rates, cost-of-living items) unioned into a
// single feed. Domain-specific fields are surfaced via the common columns
// (title, description, country, source_url, etc.); the Type field
// identifies the domain.
type radarItem struct {
	ID                pgtype.UUID        `json:"id"`
	Type              string             `json:"type"`
	Title             string             `json:"title"`
	Description       *string            `json:"description,omitempty"`
	Country           string             `json:"country"`
	University        *string            `json:"university,omitempty"`
	DegreeLevel       *string            `json:"degree_level,omitempty"`
	Deadline          pgtype.Date        `json:"deadline"`
	ApplicationURL    *string            `json:"application_url,omitempty"`
	ConfidenceOverall float32            `json:"confidence_overall"`
	TrustLevel        int16              `json:"trust_level"`
	FreshnessScore    int32              `json:"freshness_score"`
	SourceURL         string             `json:"source_url"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
	// Saved is only populated in the detail handler (HandleRadarItemDetail)
	// so the save button can seed its initial state. It is nil/omitted in
	// list responses to avoid an N+1 saved-status check per item.
	Saved *bool `json:"saved,omitempty"`
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

// jobToRadarItem converts an intel_jobs row to a radarItem.
func jobToRadarItem(j gen.IntelJob) radarItem {
	return radarItem{
		ID:                j.ID,
		Type:              "job",
		Title:             j.Title,
		Description:       j.Description,
		Country:           j.Country,
		Deadline:          j.Deadline,
		ApplicationURL:    j.ApplicationUrl,
		ConfidenceOverall: j.ConfidenceOverall,
		TrustLevel:        j.TrustLevel,
		FreshnessScore:    j.FreshnessScore,
		SourceURL:         j.SourceUrl,
		CreatedAt:         j.CreatedAt,
	}
}

// visaRouteToRadarItem converts a visa_routes row (with intel columns) to
// a radarItem. The country is derived from the destination space; when
// the space is not loaded, the country field is empty and the UI falls
// back to the destination_space_id.
func visaRouteToRadarItem(v gen.VisaRoute) radarItem {
	govURL := v.GovSourceUrl
	return radarItem{
		ID:                v.ID,
		Type:              "visa",
		Title:             v.Name,
		Description:       v.EligibilitySummary,
		ApplicationURL:    &govURL,
		ConfidenceOverall: v.ConfidenceOverall,
		TrustLevel:        0,
		FreshnessScore:    v.FreshnessScore,
		SourceURL:         v.GovSourceUrl,
		CreatedAt:         v.CreatedAt,
	}
}

// housingGuideToRadarItem converts a housing_guides row (with intel
// columns) to a radarItem.
func housingGuideToRadarItem(h gen.HousingGuide) radarItem {
	title := ""
	if h.AccommodationType != nil {
		title = *h.AccommodationType
	}
	return radarItem{
		ID:                h.ID,
		Type:              "housing",
		Title:             title,
		Description:       h.Tips,
		ConfidenceOverall: h.ConfidenceOverall,
		TrustLevel:        0,
		FreshnessScore:    h.FreshnessScore,
		SourceURL:         "",
		CreatedAt:         h.CreatedAt,
	}
}

// remittanceRateToRadarItem converts a remittance_rates row (with intel
// columns) to a radarItem. The title is composed from the currency pair.
func remittanceRateToRadarItem(r gen.RemittanceRate) radarItem {
	title := r.SendCurrency + " → " + r.ReceiveCurrency
	sourceURL := ""
	if r.SourceUrl != nil {
		sourceURL = *r.SourceUrl
	}
	return radarItem{
		ID:                r.ID,
		Type:              "remittance",
		Title:             title,
		ConfidenceOverall: r.ConfidenceOverall,
		TrustLevel:        0,
		FreshnessScore:    r.FreshnessScore,
		SourceURL:         sourceURL,
		CreatedAt:         r.FetchedAt,
	}
}

// comparisonItemToRadarItem converts a comparison_items row (with intel
// columns) to a radarItem for the cost_of_living domain.
func comparisonItemToRadarItem(c gen.ComparisonItem) radarItem {
	return radarItem{
		ID:                c.ID,
		Type:              "cost_of_living",
		Title:             c.Title,
		ConfidenceOverall: c.ConfidenceOverall,
		TrustLevel:        0,
		FreshnessScore:    c.FreshnessScore,
		SourceURL:         "",
		CreatedAt:         c.CreatedAt,
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

// HandleRadarFeed returns the geo-scoped radar feed. The feed unions
// promoted entities from all intelligence domains: scholarships, jobs,
// visa routes, housing guides, remittance rates, and cost-of-living
// items. Items are merged and sorted by created_at desc.
//
// GET /api/v1/intel/radar
func (h *Handlers) HandleRadarFeed(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, _ := optionalUser(w, r)
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	// BUG-006 fix: the spec §731-733 requires keyset pagination via ?cursor=.
	cursorStr := q.Get("cursor")
	cursorTS, cursorID, hasCursor := parseCursor(cursorStr)

	spaceIDStr := q.Get("space_id")
	country := q.Get("country")
	domainFilter := q.Get("type") // optional domain filter (e.g. "job", "visa")

	// Fetch scholarships (existing logic).
	var schRows []gen.Scholarship
	var err error
	switch {
	case hasCursor:
		schRows, err = h.intelStores.ListScholarshipsKeyset(r.Context(), tenantID, gen.ListScholarshipsKeysetParams{
			Column2: cursorTS,
			ID:      cursorID,
			Limit:   clampInt32(limit),
		})
	case spaceIDStr != "":
		var spaceID pgtype.UUID
		if e := spaceID.Scan(spaceIDStr); e != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid space_id")
			return
		}
		schRows, err = h.intelStores.ListScholarshipsBySpace(r.Context(), tenantID, gen.ListScholarshipsBySpaceParams{
			CountrySpaceID: spaceID,
			Limit:          clampInt32(limit),
			Offset:         offset,
		})
	case country != "":
		schRows, err = h.intelStores.ListScholarshipsByCountry(r.Context(), tenantID, gen.ListScholarshipsByCountryParams{
			Country: country,
			Limit:   clampInt32(limit),
			Offset:  offset,
		})
	default:
		schRows, err = h.intelStores.ListScholarships(r.Context(), tenantID, gen.ListScholarshipsParams{
			Limit:  clampInt32(limit),
			Offset: offset,
		})
	}
	if err != nil {
		slog.Error("intel: radar feed scholarships query failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "radar_feed_failed", "Failed to load radar feed")
		return
	}

	items := make([]radarItem, 0, len(schRows))
	if domainFilter == "" || domainFilter == "scholarship" {
		for _, s := range schRows {
			items = append(items, scholarshipToRadarItem(s))
		}
	}

	// Union in the other domain tables. Each domain is fetched with its
	// own keyset query and converted to radarItems. When a domain filter
	// is active, only that domain is fetched. Errors from individual
	// domains are logged but do not fail the whole feed (graceful
	// degradation — a domain with no data or a transient error should
	// not blank the entire radar).
	if domainFilter == "" || domainFilter == "job" {
		jobRows, jerr := h.intelStores.ListIntelJobsKeyset(r.Context(), tenantID, gen.ListIntelJobsKeysetParams{
			Column2: cursorTS,
			ID:      cursorID,
			Limit:   clampInt32(limit),
		})
		if jerr != nil {
			slog.Warn("intel: radar feed jobs query failed", "error", jerr)
		} else {
			for _, j := range jobRows {
				items = append(items, jobToRadarItem(j))
			}
		}
	}
	if domainFilter == "" || domainFilter == "visa" {
		vRows, verr := h.intelStores.ListVisaRoutesIntelKeyset(r.Context(), tenantID, gen.ListVisaRoutesIntelKeysetParams{
			Column2: cursorTS,
			ID:      cursorID,
			Limit:   clampInt32(limit),
		})
		if verr != nil {
			slog.Warn("intel: radar feed visa routes query failed", "error", verr)
		} else {
			for _, v := range vRows {
				items = append(items, visaRouteToRadarItem(v))
			}
		}
	}
	if domainFilter == "" || domainFilter == "housing" {
		hRows, herr := h.intelStores.ListHousingGuidesIntelKeyset(r.Context(), tenantID, gen.ListHousingGuidesIntelKeysetParams{
			Column2: cursorTS,
			ID:      cursorID,
			Limit:   clampInt32(limit),
		})
		if herr != nil {
			slog.Warn("intel: radar feed housing guides query failed", "error", herr)
		} else {
			for _, hg := range hRows {
				items = append(items, housingGuideToRadarItem(hg))
			}
		}
	}
	if domainFilter == "" || domainFilter == "remittance" {
		rRows, rerr := h.intelStores.ListRemittanceRatesIntelKeyset(r.Context(), tenantID, gen.ListRemittanceRatesIntelKeysetParams{
			Column2: cursorTS,
			ID:      cursorID,
			Limit:   clampInt32(limit),
		})
		if rerr != nil {
			slog.Warn("intel: radar feed remittance rates query failed", "error", rerr)
		} else {
			for _, rr := range rRows {
				items = append(items, remittanceRateToRadarItem(rr))
			}
		}
	}
	if domainFilter == "" || domainFilter == "cost_of_living" {
		cRows, cerr := h.intelStores.ListComparisonItemsIntelKeyset(r.Context(), tenantID, gen.ListComparisonItemsIntelKeysetParams{
			Column2: cursorTS,
			ID:      cursorID,
			Limit:   clampInt32(limit),
		})
		if cerr != nil {
			slog.Warn("intel: radar feed cost-of-living query failed", "error", cerr)
		} else {
			for _, c := range cRows {
				items = append(items, comparisonItemToRadarItem(c))
			}
		}
	}

	// Sort the merged items by created_at desc (stable sort to preserve
	// domain insertion order for same-timestamp items).
	sortRadarItems(items)

	// Trim to the page limit.
	if len(items) > int(limit) {
		items = items[:limit]
	}

	hasMore := len(items) >= int(limit)
	nextCursor := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	followedSpaces := h.resolveFollowedSpaceIDs(r.Context(), tenantID, userID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"items":              items,
		"next_cursor":        nextCursor,
		"has_more":           hasMore,
		"counts_by_type":     countByType(items),
		"counts_by_country":  countByCountry(items),
		"followed_space_ids": followedSpaces,
	})
}

// sortRadarItems sorts radar items by created_at descending (newest first).
// Items with invalid created_at are placed last.
func sortRadarItems(items []radarItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0; j-- {
			if !items[j].CreatedAt.Valid {
				break
			}
			if !items[j-1].CreatedAt.Valid || items[j].CreatedAt.Time.After(items[j-1].CreatedAt.Time) {
				items[j], items[j-1] = items[j-1], items[j]
			} else {
				break
			}
		}
	}
}

// HandleRadarCounts returns per-country scholarship counts for the dashboard
// widget. The widget renders a world map heat-map; this endpoint supplies the
// counts that drive the colour intensity per country.
//
// GET /api/v1/intel/radar/counts
func (h *Handlers) HandleRadarCounts(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
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
		errpkg.WriteError(w, http.StatusInternalServerError, "counts_failed", "Failed to load radar counts")
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

	// Spec envelope: {data: {total_new, by_type, by_country}} (INTEL-019 fix,
	// §3.3 / api-response-format.md). In v1 all radar items are scholarships,
	// so by_type has a single key.
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"total_new":  total,
			"by_type":    map[string]int64{"scholarship": total},
			"by_country": byCountry,
		},
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
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
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

	_, limit := h.parsePagination(r)

	// INTEL-016 fix: max followed spaces from system_settings (intel.radar.max_followed_spaces).
	maxFollowed := 100
	if h.settings != nil {
		maxFollowed = h.settings.GetIntOr(r.Context(), h.defaultTenant, "intel.radar.max_followed_spaces", 100)
	}

	followed, err := h.followsSvc.GetFollowingByType(r.Context(), tenantID, userID, userID, follows.EntityTypeSpace, int32(maxFollowed), 0)
	if err != nil {
		slog.Error("intel: radar followed spaces lookup failed", "user_id", userID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "followed_spaces_failed", "Failed to load followed spaces")
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
			errpkg.WriteError(w, http.StatusInternalServerError, "followed_feed_failed", "Failed to load followed feed")
			return
		}
	}

	items := make([]radarItem, 0, len(rows))
	for _, s := range rows {
		items = append(items, scholarshipToRadarItem(s))
	}

	// Union in jobs from followed spaces (graceful degradation on error).
	if len(spaceIDs) > 0 {
		jobRows, jerr := h.intelStores.ListIntelJobsBySpaces(r.Context(), tenantID, gen.ListIntelJobsBySpacesParams{
			Column2: spaceIDs,
			Limit:   clampInt32(limit),
		})
		if jerr != nil {
			slog.Warn("intel: radar followed jobs query failed", "error", jerr)
		} else {
			for _, j := range jobRows {
				items = append(items, jobToRadarItem(j))
			}
		}
	}

	// Sort the merged items by created_at desc.
	sortRadarItems(items)
	if len(items) > int(limit) {
		items = items[:limit]
	}

	hasMore := len(items) >= limit
	nextCursor := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	// Radar envelope (spec §739), NOT the generic {data, meta} envelope.
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if body.SpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "space_id is required")
		return
	}

	var spaceID pgtype.UUID
	if err := spaceID.Scan(body.SpaceID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid space_id")
		return
	}

	followed, err := h.followsSvc.ToggleFollow(r.Context(), tenantID, userID, follows.EntityTypeSpace, spaceID)
	if err != nil {
		slog.Error("intel: radar follow failed", "space_id", body.SpaceID, "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "follow_failed", "Failed to follow space")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"space_id":  body.SpaceID,
			"following": followed,
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
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
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
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "space_id is required")
		return
	}
	var spaceID pgtype.UUID
	if err := spaceID.Scan(spaceIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid space_id")
		return
	}

	followed, err := h.followsSvc.ToggleFollow(r.Context(), tenantID, userID, follows.EntityTypeSpace, spaceID)
	if err != nil {
		slog.Error("intel: radar unfollow failed", "space_id", spaceIDStr, "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "unfollow_failed", "Failed to unfollow space")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
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
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}
		slog.Error("intel: radar item detail lookup failed", "item_id", itemID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load radar item")
		return
	}

	item := scholarshipToRadarItem(s)

	// Populate the Saved flag so the detail page's save button can seed
	// its initial state. Only checked for authenticated users; guests get
	// nil (omitted) and the button defaults to unsaved.
	if h.scholarSvc != nil {
		if uid, ok := optionalUser(w, r); ok {
			saved, serr := h.scholarSvc.IsSavedScholarship(r.Context(), tenantID, uid, s.GuideID)
			if serr != nil {
				slog.Warn("intel: radar item saved-status check failed", "item_id", itemID.String(), "error", serr)
			} else {
				item.Saved = &saved
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": item,
	})
}

// HandleRadarSaved lists the caller's saved radar opportunities. In v1 these
// are saved scholarships; the handler delegates to the scholarships service
// so the radar reuses the existing saved-scholarship tracker.
//
// GET /api/v1/intel/radar/saved
func (h *Handlers) HandleRadarSaved(w http.ResponseWriter, r *http.Request) {
	if h.scholarSvc == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
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
	page, limit := h.parsePagination(r)

	rows, total, err := h.scholarSvc.ListSavedScholarships(r.Context(), tenantID, userID, status, page, limit)
	if err != nil {
		slog.Error("intel: radar saved list failed", "user_id", userID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "saved_list_failed", "Failed to load saved opportunities")
		return
	}
	if rows == nil {
		rows = []scholarships.SavedScholarshipResponse{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if body.ItemID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "item_id is required")
		return
	}

	var itemID pgtype.UUID
	if err := itemID.Scan(body.ItemID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid item_id")
		return
	}

	// Resolve the scholarship projection row to get its guide_id — the
	// saved-scholarship tracker keys on guide_id, not scholarship.id.
	s, err := h.intelStores.GetScholarship(r.Context(), tenantID, itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}
		slog.Error("intel: radar save lookup failed", "item_id", itemID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load radar item")
		return
	}

	saved, err := h.scholarSvc.SaveScholarship(r.Context(), tenantID, userID, s.GuideID)
	if err != nil {
		if errors.Is(err, scholarships.ErrGuideNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Guide not found for this opportunity")
			return
		}
		slog.Error("intel: radar save failed", "item_id", itemID.String(), "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "save_failed", "Failed to save opportunity")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]interface{}{
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
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
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
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}
		slog.Error("intel: radar unsave lookup failed", "item_id", itemID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load radar item")
		return
	}

	if err := h.scholarSvc.DeleteSavedScholarship(r.Context(), tenantID, userID, s.GuideID); err != nil {
		slog.Error("intel: radar unsave failed", "item_id", itemID.String(), "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "unsave_failed", "Failed to unsave opportunity")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
