package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// scholarshipDetail is the full detail shape returned by the scholarship
// detail endpoint. It includes the promotion metadata (confidence, trust
// level, freshness, verification status) that the radar UI surfaces.
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
	Funding              []byte             `json:"funding,omitempty"`
	Deadline             pgtype.Date        `json:"deadline"`
	ApplicationURL       *string            `json:"application_url,omitempty"`
	RequiredDocuments    []string           `json:"required_documents,omitempty"`
	MinimumGpa           pgtype.Numeric     `json:"minimum_gpa"`
	LanguageRequirements []byte             `json:"language_requirements,omitempty"`
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
		Funding:              s.Funding,
		Deadline:             s.Deadline,
		ApplicationURL:       s.ApplicationUrl,
		RequiredDocuments:    s.RequiredDocuments,
		MinimumGpa:           s.MinimumGpa,
		LanguageRequirements: s.LanguageRequirements,
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
// deadline range, and fields_of_study filters with offset pagination.
//
// GET /api/v1/intel/scholarships
func (h *Handlers) HandleSearchScholarships(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}

	q := r.URL.Query()
	page, limit := parsePagination(r)
	offset := clampInt32((page - 1) * limit)

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
	if v := q.Get("deadline_from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			params.Column4 = pgtype.Date{Time: t, Valid: true}
		}
	}
	if v := q.Get("deadline_to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			params.Column5 = pgtype.Date{Time: t, Valid: true}
		}
	}
	if v := q.Get("field_of_study"); v != "" {
		params.Column6 = v
	}

	rows, err := h.intelStores.SearchScholarships(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", err.Error())
		return
	}

	items := make([]radarItem, 0, len(rows))
	for _, s := range rows {
		items = append(items, scholarshipToRadarItem(s))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": items,
		"meta": map[string]interface{}{
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
			"count":       len(items),
		},
	})
}

// HandleScholarshipDetail returns a single scholarship projection row by id.
//
// GET /api/v1/intel/scholarships/{id}
func (h *Handlers) HandleScholarshipDetail(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}

	scholarshipID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	s, err := h.intelStores.GetScholarship(r.Context(), scholarshipID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Scholarship not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": scholarshipToDetail(s),
	})
}
