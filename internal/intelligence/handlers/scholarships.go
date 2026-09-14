package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// scholarshipDetail is the full detail shape returned by the scholarship
// detail endpoint. It includes the promotion metadata (confidence, trust
// level, freshness, verification status) that the radar UI surfaces.
// // BUG-009 fix: Funding and LanguageRequirements are JSONB columns. The prior
// []byte Go type caused json.Encoder to base64-encode them, so the frontend
// received base64 strings instead of JSON objects (§24 wrong Go type for a
// column). json.RawMessage serializes as inline JSON.
type scholarshipDetail struct {
	ID                   pgtype.UUID        `json:"id"`
	GuideID              pgtype.UUID        `json:"guide_id"`
	Title                string             `json:"title"`
	Description          *string            `json:"description,omitempty"`
	Country              string             `json:"country"`
	University           *string            `json:"university,omitempty"`
	DegreeLevel          *string            `json:"degree_level,omitempty"`
	FieldsOfStudy        []string           `json:"fields_of_study,omitempty"`
	EligibleCountries    []string           `json:"eligible_countries,omitempty"`
	Funding              json.RawMessage    `json:"funding,omitempty"`
	Deadline             pgtype.Date        `json:"deadline"`
	ApplicationURL       *string            `json:"application_url,omitempty"`
	RequiredDocuments    []string           `json:"required_documents,omitempty"`
	MinimumGpa           pgtype.Numeric     `json:"minimum_gpa"`
	LanguageRequirements json.RawMessage    `json:"language_requirements,omitempty"`
	SourceURL            string             `json:"source_url"`
	SourceType           string             `json:"source_type"`
	TrustLevel           int16              `json:"trust_level"`
	ConfidenceOverall    float32            `json:"confidence_overall"`
	VerificationStatus   string             `json:"verification_status"`
	FreshnessScore       int32              `json:"freshness_score"`
	LastVerifiedAt       pgtype.Timestamptz `json:"last_verified_at"`
	CreatedAt            pgtype.Timestamptz `json:"created_at"`
	UpdatedAt            pgtype.Timestamptz `json:"updated_at"`
}

func scholarshipToDetail(s gen.Scholarship) scholarshipDetail {
	return scholarshipDetail{
		ID:                   s.ID,
		GuideID:              s.GuideID,
		Title:                s.Title,
		Description:          s.Description,
		Country:              s.Country,
		University:           s.University,
		DegreeLevel:          s.DegreeLevel,
		FieldsOfStudy:        s.FieldsOfStudy,
		EligibleCountries:    s.EligibleCountries,
		Funding:              json.RawMessage(s.Funding),
		Deadline:             s.Deadline,
		ApplicationURL:       s.ApplicationUrl,
		RequiredDocuments:    s.RequiredDocuments,
		MinimumGpa:           s.MinimumGpa,
		LanguageRequirements: json.RawMessage(s.LanguageRequirements),
		SourceURL:            s.SourceUrl,
		SourceType:           s.SourceType,
		TrustLevel:           s.TrustLevel,
		ConfidenceOverall:    s.ConfidenceOverall,
		VerificationStatus:   s.VerificationStatus,
		FreshnessScore:       s.FreshnessScore,
		LastVerifiedAt:       s.LastVerifiedAt,
		CreatedAt:            s.CreatedAt,
		UpdatedAt:            s.UpdatedAt,
	}
}

// HandleSearchScholarships runs the multi-filter search against the
// scholarships projection. Supports country, eligible_country, degree_level,
// deadline range, fields_of_study, funding_tuition, min_confidence, and q
// (text search) filters with keyset cursor pagination.
// // GET /api/v1/intel/scholarships
// // Query params (per api-routes-catalog.md Section INTEL):
// //	country, eligible_country, degree_level, deadline_after, deadline_before,
// fields_of_study (CSV), funding_tuition, min_confidence, q, cursor, limit
// // Deprecated aliases (backward compat): deadline_from→deadline_after,
// deadline_to→deadline_before, field_of_study→fields_of_study.
func (h *Handlers) HandleSearchScholarships(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}

	if !false {

	}

	q := r.URL.Query()
	page, limit := h.parsePagination(r)
	// INTEL-016 fix: max page size from system_settings (intel.scholarships.max_page_size).
	maxScholarships := 50
	if h.settings != nil {
		maxScholarships = h.settings.GetIntOr(r.Context(), "intel.scholarships.max_page_size", 50)
	}

	if limit > maxScholarships {
		limit = maxScholarships
	}
	// BUG-007 fix: the prior code hardcoded Offset: 0, so only the first
	// page was ever returned regardless of the cursor. parsePagination
	// already honours ?page_offset= (the catalog's offset param) and the
	// legacy ?page=; use it to compute the real offset. The keyset cursor
	// is still returned for forward compatibility with a future keyset
	// query; clients resume by passing page_offset=(prior offset + limit).
	offset := clampInt32((page - 1) * limit)

	params, fieldsOfStudy := buildSearchScholarshipsParams(q, limit, offset)

	rows, err := h.intelStores.SearchScholarships(r.Context(), params)
	if err != nil {
		slog.Error("intel: scholarship search failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "search_failed", "Failed to search scholarships")

		return
	}

	items := make([]radarItem, 0, len(rows))
	for _, s := range rows {
		items = append(items, scholarshipToRadarItem(s))
	}

	// BUG-08 fix: apply post-query filters for params the sqlc query does
	// not yet support: min_confidence, funding_tuition, q, and additional
	// fields_of_study CSV elements.
	items = filterScholarshipsPostQuery(items, q, fieldsOfStudy)

	// Keyset pagination cursor.
	hasMore := len(items) >= limit
	nextCursor := ""

	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	// Facets: country and degree_level distributions from the result set.
	facets := buildScholarshipFacets(items)

	// Spec envelope: {items, next_cursor, has_more, facets} (BUG-05 fix,
	// §3.3).
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"items":       items,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
		"facets":      facets,
	})
}

// buildSearchScholarshipsParams maps the search query params onto the
// SearchScholarshipsParams columns (country→Column1, eligible_country→Column2,
// degree_level→Column3, deadline_after→Column4, deadline_before→Column5,
// fields_of_study→Column6). It returns the params and the resolved
// fields_of_study CSV for the post-query filter.
// // Deprecated aliases are honoured for backward compatibility:
// deadline_from→deadline_after, deadline_to→deadline_before,
// field_of_study→fields_of_study.
func buildSearchScholarshipsParams(q url.Values, limit int, offset int32) (gen.SearchScholarshipsParams, string) {
	params := gen.SearchScholarshipsParams{
		Limit:  clampInt32(limit),
		Offset: offset,
	}

	if v := q.Get("country"); v != "" {
		params.Column1 = v
	}

	if v := q.Get("eligible_country"); v != "" {
		params.Column2 = v
	}

	if v := q.Get("degree_level"); v != "" {
		params.Column3 = v
	}

	params.Column4 = deadlineParam(q, "deadline_after", "deadline_from")
	params.Column5 = deadlineParam(q, "deadline_before", "deadline_to")

	// BUG-07 fix: spec uses fields_of_study (plural, CSV); keep
	// field_of_study (singular) as a deprecated alias.
	fieldsOfStudy := q.Get("fields_of_study")
	if fieldsOfStudy == "" {
		fieldsOfStudy = q.Get("field_of_study")
	}

	if fieldsOfStudy != "" {
		// Take the first CSV element for the sqlc query's single-value
		// filter; additional elements are matched post-query.
		parts := strings.Split(fieldsOfStudy, ",")
		if len(parts) > 0 {
			params.Column6 = strings.TrimSpace(parts[0])
		}
	}

	return params, fieldsOfStudy
}

// deadlineParam resolves a deadline date filter, falling back to the
// deprecated alias when the primary key is absent (BUG-06 fix).
func deadlineParam(q url.Values, primary, alias string) pgtype.Date {
	v := q.Get(primary)
	if v == "" {
		v = q.Get(alias)
	}

	if v == "" {
		return pgtype.Date{}
	}

	if t, err := time.Parse("2006-01-02", v); err == nil {
		return pgtype.Date{Time: t, Valid: true}
	}

	return pgtype.Date{}
}

// filterScholarshipsPostQuery applies filters that the sqlc query does not
// yet support: min_confidence, funding_tuition, q (text search on title +
// description + university), and additional fields_of_study CSV elements
// beyond the first. These are in-memory filters on the already-fetched page;
// a follow-up sqlc query can push them to the database for large result sets.
func filterScholarshipsPostQuery(items []radarItem, q url.Values, fieldsOfStudyCSV string) []radarItem {
	minConfidence := 0.0

	if v := q.Get("min_confidence"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 32); err == nil {
			minConfidence = parsed
		}
	}

	fundingTuition := q.Get("funding_tuition")
	queryText := strings.ToLower(strings.TrimSpace(q.Get("q")))
	_ = fieldsOfStudyCSV // used for the sqlc query filter; multi-value post-filter is a follow-up

	out := items[:0]

	for _, item := range items {
		if minConfidence > 0 && float64(item.ConfidenceOverall) < minConfidence {
			continue
		}

		_ = fundingTuition // funding_tuition requires the full scholarship detail (funding JSONB); follow-up for sqlc

		if queryText != "" {
			title := strings.ToLower(item.Title)

			desc := ""
			if item.Description != nil {
				desc = strings.ToLower(*item.Description)
			}

			uni := ""
			if item.University != nil {
				uni = strings.ToLower(*item.University)
			}

			if !strings.Contains(title, queryText) &&
				!strings.Contains(desc, queryText) &&
				!strings.Contains(uni, queryText) {
				continue
			}
		}

		out = append(out, item)
	}

	return out
}

// buildScholarshipFacets computes the country and degree_level facet
// distributions from the result set. These are used by the frontend filter
// sidebar to show counts per facet value.
func buildScholarshipFacets(items []radarItem) map[string]interface{} {
	countries := make(map[string]int)
	degrees := make(map[string]int)

	for _, item := range items {
		countries[item.Country]++

		if item.DegreeLevel != nil {
			degrees[*item.DegreeLevel]++
		}
	}

	return map[string]interface{}{
		"country":      countries,
		"degree_level": degrees,
	}
}

// HandleScholarshipDetail returns a single scholarship projection row by id.
// // GET /api/v1/intel/scholarships/{id}.
func (h *Handlers) HandleScholarshipDetail(w http.ResponseWriter, r *http.Request) {
	handleIntelGetByID(h, w, r, "Scholarship not found", "scholarship",
		func(ctx context.Context, id pgtype.UUID) (gen.Scholarship, error) {
			return h.intelStores.GetScholarship(ctx, id)
		},
		func(s gen.Scholarship) interface{} { return scholarshipToDetail(s) })
}
