package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/scholarships"
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
	SpaceID           pgtype.UUID        `json:"space_id,omitempty"`
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

	// Internal fields used for source-blocking (Task 2 v2). Not exposed in the
	// JSON envelope.
	IntelRecordID pgtype.UUID `json:"-"`
	SourceID      string      `json:"-"`
}

func scholarshipToRadarItem(s gen.Scholarship) radarItem {
	return radarItem{
		ID:                s.ID,
		IntelRecordID:     s.IntelRecordID,
		Type:              "scholarship",
		Title:             s.Title,
		Description:       s.Description,
		Country:           s.Country,
		SpaceID:           s.CountrySpaceID,
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
		IntelRecordID:     j.IntelRecordID,
		Type:              "job",
		Title:             j.Title,
		Description:       j.Description,
		Country:           j.Country,
		SpaceID:           j.CountrySpaceID,
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
		IntelRecordID:     v.IntelRecordID,
		Type:              "visa",
		Title:             v.Name,
		SpaceID:           v.DestinationSpaceID,
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
		ID:            h.ID,
		IntelRecordID: h.IntelRecordID,
		Type:          "housing",
		Title:         title,
		SpaceID: func() pgtype.UUID {
			if h.DestinationSpaceID.Valid {
				return h.DestinationSpaceID
			}
			return h.CitySpaceID
		}(),
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
		IntelRecordID:     r.IntelRecordID,
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
		IntelRecordID:     c.IntelRecordID,
		Type:              "cost_of_living",
		Title:             c.Title,
		SpaceID:           c.GeoSpaceID,
		Description:       c.Description,
		ConfidenceOverall: c.ConfidenceOverall,
		TrustLevel:        0,
		FreshnessScore:    c.FreshnessScore,
		SourceURL:         "",
		CreatedAt:         c.CreatedAt,
	}
}

// ---------------------------------------------------------------------------
// Radar pagination
// ---------------------------------------------------------------------------

// radarDomainOrder defines the deterministic merge order for
// same-timestamp items across domains. Domains not in the map sort last.
var radarDomainOrder = map[string]int{
	"scholarship":    0,
	"job":            1,
	"visa":           2,
	"housing":        3,
	"remittance":     4,
	"cost_of_living": 5,
}

func radarTypeRank(t string) int {
	if r, ok := radarDomainOrder[t]; ok {
		return r
	}

	return len(radarDomainOrder)
}

// radarCursorCheckpoint is one domain's resume position in the merged
// keyset stream: the (created_at, id) of the last item of that domain
// that was emitted.
type radarCursorCheckpoint struct {
	TS int64  `json:"t"` // created_at, UnixNano
	ID string `json:"i"` // row id
}

// encodeRadarCursor encodes the per-domain checkpoints as a base64 JSON
// object for the next_cursor field. A single (created_at,id) cursor cannot
// resume a merged multi-domain stream correctly — applying one domain's id
// to another table's keyset clause randomly duplicates or drops
// same-timestamp rows. Keeping a checkpoint per domain makes each domain
// resume from its own last emitted item, so nothing is re-emitted or lost.
func encodeRadarCursor(checkpoints map[string]radarCursorCheckpoint) string {
	if len(checkpoints) == 0 {
		return ""
	}

	raw, err := json.Marshal(checkpoints)
	if err != nil {
		return ""
	}

	return base64.RawURLEncoding.EncodeToString(raw)
}

// parseSinceParam parses the ?since= filter (catalog INTEL): accepts an
// RFC3339 timestamp or a Unix epoch (seconds); anything else yields an
// invalid timestamp, which the queries treat as "no filter".
func parseSinceParam(raw string) pgtype.Timestamptz {
	if raw == "" {
		return pgtype.Timestamptz{}
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
	}
	if secs, err := strconv.ParseInt(raw, 10, 64); err == nil && secs > 0 {
		return pgtype.Timestamptz{Time: time.Unix(secs, 0).UTC(), Valid: true}
	}
	return pgtype.Timestamptz{}
}

// parseRadarCursor decodes the keyset cursor. Returns nil when absent or
// malformed (→ offset pagination). Two wire formats are supported: the v2
// per-domain checkpoint map (base64 JSON), and the legacy v1
// "nano|uuid" single cursor, which is applied to every domain — matching
// the pre-fix resume semantics for in-flight cursors.
func parseRadarCursor(raw string) map[string]radarCursorCheckpoint {
	if raw == "" {
		return nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}

	if len(decoded) > 0 && decoded[0] == '{' {
		var m map[string]radarCursorCheckpoint
		if err := json.Unmarshal(decoded, &m); err == nil && len(m) > 0 {
			return m
		}

		return nil
	}

	tsStr, idStr, found := strings.Cut(string(decoded), "|")
	if !found {
		return nil
	}

	nano, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return nil
	}

	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil || !id.Valid {
		return nil
	}

	cp := radarCursorCheckpoint{TS: nano, ID: id.String()}

	return map[string]radarCursorCheckpoint{
		"scholarship": cp, "job": cp, "visa": cp,
		"housing": cp, "remittance": cp, "cost_of_living": cp,
	}
}

// timeFromNano converts a UnixNano value to a time.Time.
func timeFromNano(nano int64) time.Time {
	return time.Unix(0, nano).UTC()
}

// normalizeRadarItemType maps the spec item_type vocabulary
// (api-routes-catalog: scholarship|job|visa_alert|fx_update|announcement)
// and the internal domain names onto the emitted item.type values. An
// unmapped value is returned lower-cased so unrecognized types yield an
// empty (not erroring) filtered feed.
func normalizeRadarItemType(v string) string {
	switch v {
	case "", "all":
		return ""
	case "scholarship":
		return "scholarship"
	case "job", "jobs":
		return "job"
	case "visa", "visa_alert":
		return "visa"
	case "remittance", "fx_update", "fx":
		return "remittance"
	case "housing":
		return "housing"
	case "cost_of_living", "col":
		return "cost_of_living"
	case "announcement":
		// Spec'd item_type with no backing domain yet — filters to an
		// empty feed rather than returning an error for a catalog value.
		return "announcement"
	default:
		return v
	}
}

// resolveRadarCountryNames replaces each item's raw extraction text
// ("USA", "Japan") with the canonical space name ("United States",
// "Japan") when a geo space is linked — and fills Country for items whose
// domain has no country text column (visa_routes, housing_guides). One
// batch query per page. Best-effort: failures leave raw text in place.
func (h *Handlers) resolveRadarCountryNames(ctx context.Context, items []radarItem) {
	if h.intelStores == nil {
		return
	}

	seen := map[pgtype.UUID]bool{}
	var ids []pgtype.UUID
	for _, it := range items {
		if it.SpaceID.Valid && !seen[it.SpaceID] {
			seen[it.SpaceID] = true
			ids = append(ids, it.SpaceID)
		}
	}
	if len(ids) == 0 {
		return
	}

	rows, err := h.intelStores.GetSpacesByIDs(ctx, ids)
	if err != nil {
		slog.Warn("intel: radar country-name resolution failed", "error", err)
		return
	}
	names := make(map[pgtype.UUID]string, len(rows))
	for _, r := range rows {
		names[r.ID] = r.Name
	}
	for i := range items {
		if n, ok := names[items[i].SpaceID]; ok && n != "" {
			items[i].Country = n
		}
	}
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

// filterBlockedRadarItems removes radar items whose source has been blocked by
// the caller. It batch-loads source attribution from the intel_record links.
func (h *Handlers) filterBlockedRadarItems(ctx context.Context, userID pgtype.UUID, items []radarItem) ([]radarItem, error) {
	if !userID.Valid || h.intelSources == nil || h.intelStores == nil {
		return items, nil
	}

	blocked, err := h.intelSources.ListBlockedSourceIDs(ctx, userID)
	if err != nil || len(blocked) == 0 {
		return items, err
	}

	blockedSet := make(map[string]struct{}, len(blocked))

	for _, id := range blocked {
		if id.Valid {
			blockedSet[id.String()] = struct{}{}
		}
	}

	recordIDs := make([]pgtype.UUID, 0, len(items))

	for _, it := range items {
		if it.IntelRecordID.Valid {
			recordIDs = append(recordIDs, it.IntelRecordID)
		}
	}

	if len(recordIDs) > 0 {
		sourceMap, err := h.intelStores.MapSourcesByRecordIDs(ctx, recordIDs)
		if err != nil {
			return nil, err
		}

		for i := range items {
			if attr, ok := sourceMap[items[i].IntelRecordID.String()]; ok {
				items[i].SourceID = attr.SourceID
			}
		}
	}

	filtered := make([]radarItem, 0, len(items))

	for _, it := range items {
		if _, isBlocked := blockedSet[it.SourceID]; isBlocked {
			continue
		}

		filtered = append(filtered, it)
	}

	return filtered, nil
}

// radarFetch is one domain's contribution to a merged radar page: the
// emitted item type and a list function that returns the domain's next
// rows after its (ts,id) keyset position, capped at limit. An invalid ts
// means "from the top".
type radarFetch struct {
	domain string
	list   func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, limit int32) ([]radarItem, error)
}

func convertRadarRows[T any](rows []T, convert func(T) radarItem) []radarItem {
	out := make([]radarItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, convert(row))
	}

	return out
}

// radarFeedDomains builds the per-domain fetch list for GET /intel/radar.
// The geo filters ($3 space / $4 country) are pushed into every domain's
// keyset query so filtered feeds stay filtered across pages — previously
// only scholarships were geo-filtered, and the filter was dropped entirely
// whenever a cursor was present. remittance_rates has no geo columns, so
// it is excluded when a geo filter is active rather than leaking
// unrelated corridors into a country-scoped feed.
func (h *Handlers) radarFeedDomains(domainFilter string, spaceID pgtype.UUID, country string, since pgtype.Timestamptz) []radarFetch {
	geoActive := spaceID.Valid || country != ""

	out := make([]radarFetch, 0, 6)
	add := func(domain string, list func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, limit int32) ([]radarItem, error)) {
		if domainFilter == "" || domainFilter == domain {
			out = append(out, radarFetch{domain: domain, list: list})
		}
	}

	add("scholarship", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
		rows, err := h.intelStores.ListScholarshipsKeyset(ctx, gen.ListScholarshipsKeysetParams{
			Column1: ts, ID: id, Column3: spaceID, Column4: country, Column5: since, Limit: lim,
		})
		if err != nil {
			return nil, err
		}

		return convertRadarRows(rows, scholarshipToRadarItem), nil
	})

	add("job", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
		rows, err := h.intelStores.ListIntelJobsKeyset(ctx, gen.ListIntelJobsKeysetParams{
			Column1: ts, ID: id, Column3: spaceID, Column4: country, Column5: since, Limit: lim,
		})
		if err != nil {
			return nil, err
		}

		return convertRadarRows(rows, jobToRadarItem), nil
	})

	add("visa", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
		rows, err := h.intelStores.ListVisaRoutesIntelKeyset(ctx, gen.ListVisaRoutesIntelKeysetParams{
			Column1: ts, ID: id, Column3: spaceID, Column4: country, Column5: since, Limit: lim,
		})
		if err != nil {
			return nil, err
		}

		return convertRadarRows(rows, visaRouteToRadarItem), nil
	})

	add("housing", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
		rows, err := h.intelStores.ListHousingGuidesIntelKeyset(ctx, gen.ListHousingGuidesIntelKeysetParams{
			Column1: ts, ID: id, Column3: spaceID, Column4: country, Column5: since, Limit: lim,
		})
		if err != nil {
			return nil, err
		}

		return convertRadarRows(rows, housingGuideToRadarItem), nil
	})

	if !geoActive {
		add("remittance", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
			rows, err := h.intelStores.ListRemittanceRatesIntelKeyset(ctx, gen.ListRemittanceRatesIntelKeysetParams{
				Column1: ts, ID: id, Column3: since, Limit: lim,
			})
			if err != nil {
				return nil, err
			}

			return convertRadarRows(rows, remittanceRateToRadarItem), nil
		})
	}

	add("cost_of_living", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
		rows, err := h.intelStores.ListComparisonItemsIntelKeyset(ctx, gen.ListComparisonItemsIntelKeysetParams{
			Column1: ts, ID: id, Column3: spaceID, Column4: country, Column5: since, Limit: lim,
		})
		if err != nil {
			return nil, err
		}

		return convertRadarRows(rows, comparisonItemToRadarItem), nil
	})

	return out
}

// radarFollowedDomains builds the per-domain fetch list for
// GET /intel/radar/followed — the space-keyed domains filtered to the
// caller's followed country spaces. remittance is excluded (no geo
// columns, and the followed feed is itself a geo filter). domainFilter
// applies the same ?item_type= narrowing as the public feed.
func (h *Handlers) radarFollowedDomains(spaceIDs []pgtype.UUID, since pgtype.Timestamptz, domainFilter string) []radarFetch {
	out := make([]radarFetch, 0, 5)
	add := func(domain string, list func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, limit int32) ([]radarItem, error)) {
		if domainFilter == "" || domainFilter == domain {
			out = append(out, radarFetch{domain: domain, list: list})
		}
	}

	add("scholarship", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
		rows, err := h.intelStores.ListScholarshipsBySpaces(ctx, gen.ListScholarshipsBySpacesParams{
			Column1: spaceIDs, Column2: ts, ID: id, Column4: since, Limit: lim,
		})
		if err != nil {
			return nil, err
		}

		return convertRadarRows(rows, scholarshipToRadarItem), nil
	})
	add("job", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
		rows, err := h.intelStores.ListIntelJobsBySpaces(ctx, gen.ListIntelJobsBySpacesParams{
			Column1: spaceIDs, Column2: ts, ID: id, Column4: since, Limit: lim,
		})
		if err != nil {
			return nil, err
		}

		return convertRadarRows(rows, jobToRadarItem), nil
	})
	add("visa", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
		rows, err := h.intelStores.ListVisaRoutesBySpaces(ctx, gen.ListVisaRoutesBySpacesParams{
			Column1: spaceIDs, Column2: ts, ID: id, Column4: since, Limit: lim,
		})
		if err != nil {
			return nil, err
		}

		return convertRadarRows(rows, visaRouteToRadarItem), nil
	})
	add("housing", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
		rows, err := h.intelStores.ListHousingGuidesBySpaces(ctx, gen.ListHousingGuidesBySpacesParams{
			Column1: spaceIDs, Column2: ts, ID: id, Column4: since, Limit: lim,
		})
		if err != nil {
			return nil, err
		}

		return convertRadarRows(rows, housingGuideToRadarItem), nil
	})
	add("cost_of_living", func(ctx context.Context, ts pgtype.Timestamptz, id pgtype.UUID, lim int32) ([]radarItem, error) {
		rows, err := h.intelStores.ListComparisonItemsBySpaces(ctx, gen.ListComparisonItemsBySpacesParams{
			Column1: spaceIDs, Column2: ts, ID: id, Column4: since, Limit: lim,
		})
		if err != nil {
			return nil, err
		}

		return convertRadarRows(rows, comparisonItemToRadarItem), nil
	})

	return out
}

// radarMergePage fetches every domain's next batch, merges, sorts,
// blocked-filters, and slices out the requested page. In keyset mode
// (checkpoints != nil) each domain resumes from its own checkpoint and
// fetches `limit` rows; in offset mode (checkpoints == nil) each domain
// fetches offset+limit rows from the top and the merged result is sliced
// [offset:offset+limit]. Individual domain query errors are logged and
// swallowed so one failing domain does not blank the whole feed.
//
// Returns the page items, the per-domain emitted checkpoints for the
// next_cursor (incoming checkpoints are preserved for domains that
// emitted nothing), and hasMore.
func (h *Handlers) radarMergePage(ctx context.Context, userID pgtype.UUID, domains []radarFetch, checkpoints map[string]radarCursorCheckpoint, offset, limit int) ([]radarItem, map[string]radarCursorCheckpoint, bool, error) {
	// Fetch one row beyond the page window so hasMore can distinguish
	// "exactly N rows remain" from "more than N remain" — a plain
	// LIMIT offset+limit returns the same row count in both cases and
	// previously reported has_more=false while pages still existed.
	fetchLimit := limit + 1
	if checkpoints == nil {
		fetchLimit = offset + limit + 1
	}

	// Fan the domain queries out concurrently — the feed unions up to six
	// tables and running them serially made page latency the sum of all
	// queries. The sqlc Queries sit on pgxpool, which is goroutine-safe.
	// Per-domain errors are logged and swallowed so one failing domain
	// does not blank the whole feed.
	results := make([][]radarItem, len(domains))

	var wg sync.WaitGroup

	for i, d := range domains {
		wg.Add(1)

		go func(i int, d radarFetch) {
			defer wg.Done()

			var ts pgtype.Timestamptz

			var id pgtype.UUID

			if cp, ok := checkpoints[d.domain]; ok {
				ts = pgtype.Timestamptz{Time: timeFromNano(cp.TS), Valid: true}
				_ = id.Scan(cp.ID)
			}

			rows, err := d.list(ctx, ts, id, clampInt32(fetchLimit))
			if err != nil {
				slog.Warn("intel: radar "+d.domain+" query failed", "error", err)
				return
			}

			results[i] = rows
		}(i, d)
	}

	wg.Wait()

	var merged []radarItem
	for _, rows := range results {
		merged = append(merged, rows...)
	}

	sortRadarItems(merged)

	filtered, err := h.filterBlockedRadarItems(ctx, userID, merged)
	if err != nil {
		return nil, nil, false, err
	}

	start := 0
	if checkpoints == nil {
		start = offset
	}

	if start > len(filtered) {
		start = len(filtered)
	}

	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	page := filtered[start:end]
	hasMore := len(filtered) > end

	next := make(map[string]radarCursorCheckpoint, len(domains))
	for k, v := range checkpoints {
		next[k] = v
	}

	for _, it := range page {
		if it.CreatedAt.Valid && it.ID.Valid {
			next[it.Type] = radarCursorCheckpoint{TS: it.CreatedAt.Time.UnixNano(), ID: it.ID.String()}
		}
	}

	if page == nil {
		page = []radarItem{}
	}

	return page, next, hasMore, nil
}

// HandleRadarFeed returns the geo-scoped radar feed. The feed unions
// promoted entities from all intelligence domains: scholarships, jobs,
// visa routes, housing guides, remittance rates, and cost-of-living
// items. Items are merged and sorted by created_at desc.
// // Pagination: ?cursor= (spec §731-733 keyset, per-domain checkpoints)
// // or ?page_offset=/​?page= (offset) — the main frontend paginates by
// // offset; both modes honor the geo + item_type filters.
// // GET /api/v1/intel/radar.
func (h *Handlers) HandleRadarFeed(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, _ := optionalUser(w, r)

	q := r.URL.Query()
	page, limit := h.parsePagination(r)
	offset := (page - 1) * limit

	checkpoints := parseRadarCursor(q.Get("cursor"))
	since := parseSinceParam(q.Get("since"))

	var spaceID pgtype.UUID
	if spaceIDStr := q.Get("space_id"); spaceIDStr != "" {
		if e := spaceID.Scan(spaceIDStr); e != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid space_id")
			return
		}
	}

	country := q.Get("country")

	// Spec §740 defines ?item_type=; ?type= is accepted as an alias for
	// clients built against the earlier implementation.
	domainFilter := normalizeRadarItemType(q.Get("item_type"))
	if domainFilter == "" {
		domainFilter = normalizeRadarItemType(q.Get("type"))
	}

	domains := h.radarFeedDomains(domainFilter, spaceID, country, since)

	items, next, hasMore, err := h.radarMergePage(r.Context(), userID, domains, checkpoints, offset, limit)
	if err != nil {
		slog.Error("intel: radar feed merge failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "radar_feed_failed", "Failed to load radar feed")

		return
	}

	followedSpaces := h.resolveFollowedSpaceIDs(r.Context(), userID)

	h.resolveRadarCountryNames(r.Context(), items)
	h.resolveLiterals(r, &items)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"items":              items,
		"next_cursor":        encodeRadarCursor(next),
		"has_more":           hasMore,
		"counts_by_type":     countByType(items),
		"counts_by_country":  countByCountry(items),
		"followed_space_ids": followedSpaces,
	})
}

// sortRadarItems sorts radar items by (created_at DESC, domain rank ASC,
// id DESC) — deterministic across the merged domains so keyset pages are
// reproducible. Items with invalid created_at are placed last.
func sortRadarItems(items []radarItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.CreatedAt.Valid != b.CreatedAt.Valid {
			return a.CreatedAt.Valid
		}

		if !a.CreatedAt.Time.Equal(b.CreatedAt.Time) {
			return a.CreatedAt.Time.After(b.CreatedAt.Time)
		}

		if ra, rb := radarTypeRank(a.Type), radarTypeRank(b.Type); ra != rb {
			return ra < rb
		}

		return a.ID.String() > b.ID.String()
	})
}

// HandleRadarCounts returns per-domain and per-country promoted-item
// counts for the dashboard widget. The widget renders a world map
// heat-map; this endpoint supplies the counts that drive the colour
// intensity per country.
// // GET /api/v1/intel/radar/counts.
func (h *Handlers) HandleRadarCounts(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	var spaceID pgtype.UUID
	if raw := r.URL.Query().Get("space_id"); raw != "" {
		if e := spaceID.Scan(raw); e != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid space_id")
			return
		}
	}

	typeRows, err := h.intelStores.CountIntelRadarByType(r.Context(), spaceID)
	if err != nil {
		slog.Error("intel: radar counts by type failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "counts_failed", "Failed to load radar counts")

		return
	}

	countryRows, err := h.intelStores.CountIntelRadarByCountry(r.Context(), spaceID)
	if err != nil {
		slog.Error("intel: radar counts by country failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "counts_failed", "Failed to load radar counts")

		return
	}

	total := int64(0)
	byType := make(map[string]int64, len(typeRows))

	for _, row := range typeRows {
		total += row.Count
		byType[row.ItemType] = row.Count
	}

	byCountry := make(map[string]int64, len(countryRows))

	for _, row := range countryRows {
		if row.Country == "" {
			continue
		}

		byCountry[row.Country] = row.Count
	}

	// Spec envelope: {data: {total_new, by_type, by_country}} (INTEL-019 fix,
	// §3.3 / api-response-format.md). by_type/by_country now cover every
	// radar domain, not just scholarships.
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"total_new":  total,
			"by_type":    byType,
			"by_country": byCountry,
		},
	})
}

// HandleRadarFollowed returns the radar feed scoped to the caller's followed
// country spaces. It resolves the user's followed "space" entities via the
// follows service, then unions the space-keyed domains (scholarships, jobs,
// visa routes, housing guides, comparison items) for ALL followed spaces in
// a single merged fetch (BUG-008b fix: the prior per-space loop was an N+1).
// // The radar envelope {items, next_cursor, has_more, counts_by_type,
// // counts_by_country, followed_space_ids} per spec §739 matches /radar.
// // Pagination honors both ?page_offset= (frontend contract) and ?cursor=
// // (spec keyset) — previously the page/cursor was ignored entirely, so
// // "Load more" re-fetched page 1.
// // GET /api/v1/intel/radar/followed.
func (h *Handlers) HandleRadarFollowed(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil || h.followsSvc == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	page, limit := h.parsePagination(r)
	offset := (page - 1) * limit
	checkpoints := parseRadarCursor(q.Get("cursor"))
	since := parseSinceParam(q.Get("since"))
	domainFilter := normalizeRadarItemType(q.Get("item_type"))
	if domainFilter == "" {
		domainFilter = normalizeRadarItemType(q.Get("type"))
	}

	spaceIDs, followedSpaceStrs, ok := h.resolveRadarFollowedSpaces(w, r, userID)
	if !ok {
		return
	}

	var items []radarItem

	next := map[string]radarCursorCheckpoint{}
	hasMore := false

	if len(spaceIDs) > 0 {
		var err error

		items, next, hasMore, err = h.radarMergePage(r.Context(), userID, h.radarFollowedDomains(spaceIDs, since, domainFilter), checkpoints, offset, limit)
		if err != nil {
			slog.Error("intel: radar followed merge failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "followed_feed_failed", "Failed to load followed feed")

			return
		}
	}

	if items == nil {
		items = []radarItem{}
	}

	// Radar envelope (spec §739), NOT the generic {data, meta} envelope.
	h.resolveRadarCountryNames(r.Context(), items)
	h.resolveLiterals(r, &items)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"items":              items,
		"next_cursor":        encodeRadarCursor(next),
		"has_more":           hasMore,
		"counts_by_type":     countByType(items),
		"counts_by_country":  countByCountry(items),
		"followed_space_ids": followedSpaceStrs,
	})
}

// resolveRadarFollowedSpaces resolves the user's followed space IDs for the
// radar feed, honoring the intel.radar.max_followed_spaces cap (INTEL-016).
// Returns ok=false (with an error response written) when the follows lookup
// fails.
func (h *Handlers) resolveRadarFollowedSpaces(w http.ResponseWriter, r *http.Request, userID pgtype.UUID) (spaceIDs []pgtype.UUID, spaceStrs []string, ok bool) {
	maxFollowed := 100
	if h.settings != nil {
		maxFollowed = h.settings.GetIntOr(r.Context(), "intel.radar.max_followed_spaces", 100)
	}

	if maxFollowed > math.MaxInt32 {
		maxFollowed = math.MaxInt32
	}
	// #nosec G115 -- maxFollowed is clamped to math.MaxInt32 above.
	followed, err := h.followsSvc.GetFollowingByType(r.Context(), userID, userID, follows.EntityTypeSpace, int32(maxFollowed), 0)
	if err != nil {
		slog.Error("intel: radar followed spaces lookup failed", "user_id", userID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "followed_spaces_failed", "Failed to load followed spaces")

		return nil, nil, false
	}

	spaceIDs = make([]pgtype.UUID, 0, len(followed))
	spaceStrs = make([]string, 0, len(followed))

	for _, f := range followed {
		if f.EntityID.Valid {
			spaceIDs = append(spaceIDs, f.EntityID)
			spaceStrs = append(spaceStrs, f.EntityID.String())
		}
	}

	return spaceIDs, spaceStrs, true
}

// HandleRadarFollow follows a country space. The body carries the space_id;
// the handler delegates to the follows service with entity_type "space" so
// the radar reuses the polymorphic follows table.
// // POST /api/v1/intel/radar/follow.
func (h *Handlers) HandleRadarFollow(w http.ResponseWriter, r *http.Request) {
	if h.followsSvc == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
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

	followed, err := h.followsSvc.EnsureFollow(r.Context(), userID, follows.EntityTypeSpace, spaceID)
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
// // DELETE /api/v1/intel/radar/follow.
func (h *Handlers) HandleRadarUnfollow(w http.ResponseWriter, r *http.Request) {
	if h.followsSvc == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
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

	followed, err := h.followsSvc.EnsureUnfollow(r.Context(), userID, follows.EntityTypeSpace, spaceID)
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

// resolveRadarItem loads a radar item by its projection row id across all
// radar domains. itemType (already normalized) short-circuits the probe to
// one table when supplied; otherwise each domain table is tried in order
// until the row is found. Returns the item, the domain's guide_id when the
// row has one (scholarships/jobs link to contents for the saved tracker),
// and found=false when no domain table has the id.
var errRadarItemNotFound = errors.New("radar item not found")

func (h *Handlers) resolveRadarItem(ctx context.Context, itemID pgtype.UUID, itemType string) (radarItem, pgtype.UUID, error) {
	if itemType == "" || itemType == "scholarship" {
		s, err := h.intelStores.GetScholarship(ctx, itemID)
		if err == nil {
			return scholarshipToRadarItem(s), s.GuideID, nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return radarItem{}, pgtype.UUID{}, err
		}
	}

	if itemType == "" || itemType == "job" {
		j, err := h.intelStores.GetIntelJob(ctx, itemID)
		if err == nil {
			return jobToRadarItem(j), j.GuideID, nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return radarItem{}, pgtype.UUID{}, err
		}
	}

	if itemType == "" || itemType == "visa" {
		v, err := h.intelStores.GetIntelVisaRouteByID(ctx, itemID)
		if err == nil {
			return visaRouteToRadarItem(v), v.GuideID, nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return radarItem{}, pgtype.UUID{}, err
		}
	}

	if itemType == "" || itemType == "housing" {
		g, err := h.intelStores.GetIntelHousingGuideByID(ctx, itemID)
		if err == nil {
			return housingGuideToRadarItem(g), g.GuideID, nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return radarItem{}, pgtype.UUID{}, err
		}
	}

	if itemType == "" || itemType == "remittance" {
		rt, err := h.intelStores.GetIntelRemittanceRateByID(ctx, itemID)
		if err == nil {
			return remittanceRateToRadarItem(rt), pgtype.UUID{}, nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return radarItem{}, pgtype.UUID{}, err
		}
	}

	if itemType == "" || itemType == "cost_of_living" {
		c, err := h.intelStores.GetIntelComparisonItemByID(ctx, itemID)
		if err == nil {
			return comparisonItemToRadarItem(c), pgtype.UUID{}, nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return radarItem{}, pgtype.UUID{}, err
		}
	}

	return radarItem{}, pgtype.UUID{}, errRadarItemNotFound
}

// HandleRadarItemDetail returns a single radar item by its projection row
// id, resolved across all radar domains — the feed unions six domain
// types, so the detail endpoint must accept all of them, not just
// scholarships.
// // GET /api/v1/intel/radar/feed/{item_id}?item_type={type}.
func (h *Handlers) HandleRadarItemDetail(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	itemID, ok := scanUUIDParam(w, r, "item_id")
	if !ok {
		return
	}

	q := r.URL.Query()
	itemType := normalizeRadarItemType(q.Get("item_type"))

	if itemType == "" {
		itemType = normalizeRadarItemType(q.Get("type"))
	}

	item, guideID, err := h.resolveRadarItem(r.Context(), itemID, itemType)
	if err != nil {
		if errors.Is(err, errRadarItemNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}

		slog.Error("intel: radar item detail lookup failed", "item_id", itemID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load radar item")

		return
	}

	// Populate the Saved flag so the detail page's save button can seed
	// its initial state. Only scholarships are savable (the tracker keys on
	// the guide's contents id); other types omit the flag. Only checked
	// for authenticated users; guests get nil (omitted).
	if item.Type == "scholarship" && h.scholarSvc != nil {
		if uid, ok := optionalUser(w, r); ok {
			saved, serr := h.scholarSvc.IsSavedScholarship(r.Context(), uid, guideID)
			if serr != nil {
				slog.Warn("intel: radar item saved-status check failed", "item_id", itemID.String(), "error", serr)
			} else {
				item.Saved = &saved
			}
		}
	}

	resolved := []radarItem{item}
	h.resolveRadarCountryNames(r.Context(), resolved)
	item = resolved[0]
	h.resolveLiterals(r, &item)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": item,
	})
}

// HandleRadarSaved lists the caller's saved radar opportunities. In v1 these
// are saved scholarships; the handler delegates to the scholarships service
// so the radar reuses the existing saved-scholarship tracker.
// // GET /api/v1/intel/radar/saved.
func (h *Handlers) HandleRadarSaved(w http.ResponseWriter, r *http.Request) {
	if h.scholarSvc == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	page, limit := h.parsePagination(r)

	rows, total, err := h.scholarSvc.ListSavedScholarships(r.Context(), userID, status, "", page, limit)
	if err != nil {
		slog.Error("intel: radar saved list failed", "user_id", userID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "saved_list_failed", "Failed to load saved opportunities")

		return
	}

	if rows == nil {
		rows = []scholarships.SavedScholarshipResponse{}
	}

	h.resolveLiterals(r, &rows)

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
// the backing guide_id and delegates to the scholarships service. The spec
// body also carries item_type — in v1 only scholarships are savable (spec
// §763 reserves saved_jobs for later), so a non-scholarship item_type gets a
// clear 422 rather than a misleading not_found.
// // POST /api/v1/intel/radar/save.
func (h *Handlers) HandleRadarSave(w http.ResponseWriter, r *http.Request) {
	if h.scholarSvc == nil || h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	var body struct {
		ItemID   string `json:"item_id"`
		ItemType string `json:"item_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if body.ItemID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "item_id is required")
		return
	}

	if t := normalizeRadarItemType(body.ItemType); t != "" && t != "scholarship" {
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "unsupported_item_type", "Only scholarship opportunities can be saved in v1")
		return
	}

	var itemID pgtype.UUID
	if err := itemID.Scan(body.ItemID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid item_id")
		return
	}

	// Resolve the scholarship projection row to get its guide_id — the
	// saved-scholarship tracker keys on guide_id, not scholarship.id.
	s, err := h.intelStores.GetScholarship(r.Context(), itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}

		slog.Error("intel: radar save lookup failed", "item_id", itemID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load radar item")

		return
	}

	saved, err := h.scholarSvc.SaveScholarship(r.Context(), userID, s.GuideID)
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
// // DELETE /api/v1/intel/radar/save/{item_id}.
func (h *Handlers) HandleRadarUnsave(w http.ResponseWriter, r *http.Request) {
	if h.scholarSvc == nil || h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
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
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Radar item not found")
			return
		}

		slog.Error("intel: radar unsave lookup failed", "item_id", itemID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load radar item")

		return
	}

	if err := h.scholarSvc.DeleteSavedScholarship(r.Context(), userID, s.GuideID); err != nil {
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
// Honors intel.radar.max_followed_spaces (INTEL-016) — the same cap as the
// followed-feed resolver, so the badge sidecar and the feed agree.
func (h *Handlers) resolveFollowedSpaceIDs(ctx context.Context, userID pgtype.UUID) []string {
	if h.followsSvc == nil || !userID.Valid {
		return []string{}
	}

	maxFollowed := 100
	if h.settings != nil {
		maxFollowed = h.settings.GetIntOr(ctx, "intel.radar.max_followed_spaces", 100)
	}
	if maxFollowed > math.MaxInt32 {
		maxFollowed = math.MaxInt32
	}

	// #nosec G115 -- maxFollowed is clamped to math.MaxInt32 above.
	followed, err := h.followsSvc.GetFollowingByType(ctx, userID, userID, follows.EntityTypeSpace, int32(maxFollowed), 0)
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
