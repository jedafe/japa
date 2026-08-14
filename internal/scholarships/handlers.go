// Package scholarships implements the Scholarship Finder surface
// (Section 73, ADR-073).
//
// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
package scholarships

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the scholarship finder HTTP routes.
type Handlers struct {
	svc *Service
}

// NewHandlers creates scholarship finder handlers.
func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

// HandleFindScholarships searches published scholarship guides.
// GET /scholarships
func (h *Handlers) HandleFindScholarships(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// Global killswitch (scholarship_finder.enabled, ADR-073 §73).
	if !h.svc.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "scholarship_finder_disabled", "Scholarship Finder is not available")
		return
	}

	q := r.URL.Query()
	filters := ScholarshipFilters{
		Country:      q.Get("country"),
		Level:        q.Get("level"),
		Q:            q.Get("q"),
		Sort:         q.Get("sort"),
		DeadlineFrom: q.Get("deadline_from"),
		DeadlineTo:   q.Get("deadline_to"),
	}
	if v := q.Get("min_amount"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			filters.MinAmount = f
		}
	}
	if v := q.Get("max_amount"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			filters.MaxAmount = f
		}
	}
	if v := q.Get("renewable"); v != "" {
		b, _ := strconv.ParseBool(v)
		filters.Renewable = &b
	}
	// Offset pagination per conventions/api-response-format.md
	// (?page_size=N&page_offset=N). Back-compat: also accept `page` + `limit`.
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize, _ = strconv.Atoi(q.Get("limit"))
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	if pageOffset < 0 {
		pageOffset, _ = strconv.Atoi(q.Get("page"))
		pageOffset = (pageOffset - 1) * pageSize
		if pageOffset < 0 {
			pageOffset = 0
		}
	}
	filters.Page = (pageOffset / pageSize) + 1
	filters.Limit = pageSize

	results, total, err := h.svc.FindScholarships(r.Context(), tenantID, filters)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "search_failed", err.Error())
		return
	}
	if results == nil {
		results = []ScholarshipResult{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": results,
		"meta": map[string]interface{}{
			"total":       total,
			"page_size":   pageSize,
			"page_offset": pageOffset,
		},
	})
}

// HandleScholarshipDetail returns a single scholarship guide.
// GET /scholarships/{id}
func (h *Handlers) HandleScholarshipDetail(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// Global killswitch (scholarship_finder.enabled, ADR-073 §73).
	if !h.svc.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "scholarship_finder_disabled", "Scholarship Finder is not available")
		return
	}

	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	result, err := h.svc.GetScholarshipDetail(r.Context(), tenantID, guideID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Scholarship not found")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": result})
}

// HandleSaveScholarship saves a scholarship to the user's tracker.
// POST /me/scholarships/{id}/save
func (h *Handlers) HandleSaveScholarship(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// Global killswitch (scholarship_finder.enabled, ADR-073 §73).
	if !h.svc.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "scholarship_finder_disabled", "Scholarship Finder is not available")
		return
	}

	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	saved, err := h.svc.SaveScholarship(r.Context(), tenantID, userID, guideID)
	if err != nil {
		if errors.Is(err, ErrGuideNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", err.Error())
			return
		}
		errpkg.WriteError(w, http.StatusBadRequest, "save_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"id":                 pgtypeUUIDString(saved.ID),
			"guide_id":           pgtypeUUIDString(saved.GuideID),
			"status":             saved.Status,
			"remind_before_days": saved.RemindBeforeDays,
		},
	})
}

// HandleUpdateSavedScholarship updates a saved scholarship's status/notes.
// PATCH /me/scholarships/{id}
func (h *Handlers) HandleUpdateSavedScholarship(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Status           string  `json:"status"`
		Notes            *string `json:"notes"`
		RemindBeforeDays *int32  `json:"remind_before_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	saved, err := h.svc.UpdateSavedScholarship(r.Context(), tenantID, userID, guideID, body.Status, body.Notes, body.RemindBeforeDays)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "update_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": saved})
}

// HandleDeleteSavedScholarship removes a scholarship from the tracker.
// DELETE /me/scholarships/{id}
func (h *Handlers) HandleDeleteSavedScholarship(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.DeleteSavedScholarship(r.Context(), tenantID, userID, guideID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "delete_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": map[string]interface{}{"deleted": true}})
}

// HandleListSavedScholarships lists the user's saved scholarships.
// GET /me/scholarships
func (h *Handlers) HandleListSavedScholarships(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
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

	rows, total, err := h.svc.ListSavedScholarships(r.Context(), tenantID, userID, status, page, limit)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	if rows == nil {
		rows = []SavedScholarshipResponse{}
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

// HandleAdminStatusDistribution returns admin aggregate stats
// (total_scholarships, upcoming_deadlines, reminders_sent, unique_users,
// and the per-status distribution).
// GET /admin/scholarships/stats
func (h *Handlers) HandleAdminStatusDistribution(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	stats, err := h.svc.AdminStats(r.Context(), tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": stats})
}

// HandleAdminTopSaved returns admin top-saved report.
// GET /admin/scholarships/top-saved
func (h *Handlers) HandleAdminTopSaved(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)
	rows, err := h.svc.AdminTopSaved(r.Context(), tenantID, page, limit)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "report_failed", err.Error())
		return
	}
	if rows == nil {
		rows = []gen.AdminScholarshipTopSavedRow{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": rows})
}

// HandleAdminUpcomingDeadlines returns admin upcoming deadlines report
// mapped to the AdminDeadlineItem shape (id, title, deadline, days_until,
// status) that the admin panel's DeadlineItem contract expects.
// GET /admin/scholarships/deadlines
func (h *Handlers) HandleAdminUpcomingDeadlines(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)
	rows, err := h.svc.AdminDeadlines(r.Context(), tenantID, page, limit)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "report_failed", err.Error())
		return
	}
	if rows == nil {
		rows = []AdminDeadlineItem{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": rows})
}

// HandleAdminReminderLog returns admin reminder delivery history.
// GET /admin/scholarships/reminder-log
func (h *Handlers) HandleAdminReminderLog(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)
	rows, err := h.svc.AdminReminderLog(r.Context(), tenantID, page, limit)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "report_failed", err.Error())
		return
	}
	if rows == nil {
		rows = []gen.AdminScholarshipReminderLogRow{}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": rows})
}

// --- shared helpers ---

type tenantCtxKey struct{}
type userCtxKey struct{}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	id := tenant.IDFromContext(r.Context())
	if !id.Valid {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return id, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil || !u.UserID.Valid {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}
	return u.UserID, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", param+" is required")
		return pgtype.UUID{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid "+param)
		return pgtype.UUID{}, false
	}
	return id, true
}

// parsePagination reads the offset-pagination convention params
// (`page_size` + `page_offset`) per conventions/api-response-format.md, with
// back-compat for the legacy `limit` + `page` (1-based) pair. Returns a
// 1-based page and a clamped page size so callers can compute
// `offset = (page-1) * limit` unchanged.
func parsePagination(r *http.Request) (int, int) {
	q := r.URL.Query()
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize, _ = strconv.Atoi(q.Get("limit"))
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	if pageOffset < 0 {
		// Legacy back-compat: derive offset from 1-based `page`.
		page, _ := strconv.Atoi(q.Get("page"))
		if page <= 0 {
			page = 1
		}
		pageOffset = (page - 1) * pageSize
	}
	page := (pageOffset / pageSize) + 1
	return page, pageSize
}

// --- Admin CRUD handlers ---

// HandleAdminListScholarships returns a paginated, filtered list of all
// scholarships for the admin management page. Supports text search via the
// `q` query param (title/university/country) and status filter via
// `verification_status`.
//
// GET /admin/scholarships
// Requires: scholarship.manage
func (h *Handlers) HandleAdminListScholarships(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	page, limit := parsePagination(r)
	offset := (page - 1) * limit

	search := q.Get("q")
	status := q.Get("verification_status")

	rows, err := h.svc.store.AdminListScholarships(r.Context(), tenantID, gen.AdminListScholarshipsParams{
		Column2: search,
		Column3: status,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list scholarships")
		return
	}

	total, err := h.svc.store.AdminCountScholarships(r.Context(), tenantID, gen.AdminCountScholarshipsParams{
		Column2: search,
		Column3: status,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to count scholarships")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": rows,
		"meta": map[string]interface{}{
			"page_size":   limit,
			"page_offset": offset,
			"count":       total,
		},
	})
}

// HandleAdminCreateScholarship creates a new scholarship record. A guide
// content row (content_type='guide', guide_category='scholarship') is
// inserted first because scholarships.guide_id is NOT NULL and references
// contents(id). The scholarship projection row is then inserted with the
// guide_id pointing to the new content row.
//
// POST /admin/scholarships
// Requires: scholarship.manage
func (h *Handlers) HandleAdminCreateScholarship(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var body struct {
		Title             string          `json:"title"`
		Description       *string         `json:"description"`
		Country           string          `json:"country"`
		University        *string         `json:"university"`
		DegreeLevel       *string         `json:"degree_level"`
		FieldsOfStudy     []string        `json:"fields_of_study"`
		EligibleCountries []string        `json:"eligible_countries"`
		Funding           json.RawMessage `json:"funding"`
		Deadline          *string         `json:"deadline"`
		ApplicationUrl    *string         `json:"application_url"`
		RequiredDocuments []string        `json:"required_documents"`
		LanguageReqs      json.RawMessage `json:"language_requirements"`
		SourceUrl         string          `json:"source_url"`
		SourceType        string          `json:"source_type"`
		TrustLevel        int16           `json:"trust_level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if body.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "title is required")
		return
	}
	if body.Country == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "country is required")
		return
	}

	// Build optional fields
	var deadline pgtype.Date
	if body.Deadline != nil && *body.Deadline != "" {
		if err := deadline.Scan(*body.Deadline); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_deadline", "Invalid deadline date format")
			return
		}
	}

	// Build the guide body from the description or title.
	guideBody := ""
	if body.Description != nil && *body.Description != "" {
		guideBody = *body.Description
	} else {
		guideBody = body.Title
	}
	slug := content.Slugify(body.Title)
	hash := content.ComputeContentHash(guideBody)
	canonicalURL := body.SourceUrl
	guideCategory := "scholarship"
	now := time.Now().UTC()

	// 1. Insert the guide content row.
	contentParams := gen.CreateContentParams{
		TenantID:        tenantID,
		AuthorID:        userID,
		ContentType:     "guide",
		Title:           body.Title,
		Slug:            slug,
		Body:            guideBody,
		BodyHtml:        "<p>" + guideBody + "</p>",
		FieldValues:     []byte("{}"),
		ContentHash:     &hash,
		CanonicalUrl:    &canonicalURL,
		Status:          "published",
		Visibility:      "public",
		SeoIndexable:    true,
		ReadTimeMinutes: content.ComputeReadTime(guideBody),
		GuideCategory:   &guideCategory,
		PublishAt:       pgtype.Timestamptz{Time: now, Valid: true},
		PublishedAt:     pgtype.Timestamptz{Time: now, Valid: true},
	}

	contentRow, err := h.svc.store.queries.CreateContent(r.Context(), contentParams)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			errpkg.WriteError(w, http.StatusConflict, "duplicate_content", "A scholarship with this content already exists")
			return
		}
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create scholarship guide content")
		return
	}

	// 2. Insert the scholarship projection row with guide_id.
	// Initialize NOT NULL array/jsonb fields to empty defaults when not
	// provided — sqlc sends nil for unset slices/byte slices which would
	// violate the NOT NULL constraint even though the column has a DEFAULT.
	fieldsOfStudy := body.FieldsOfStudy
	if fieldsOfStudy == nil {
		fieldsOfStudy = []string{}
	}
	eligibleCountries := body.EligibleCountries
	if eligibleCountries == nil {
		eligibleCountries = []string{}
	}
	requiredDocuments := body.RequiredDocuments
	if requiredDocuments == nil {
		requiredDocuments = []string{}
	}
	funding := body.Funding
	if len(funding) == 0 {
		funding = json.RawMessage("{}")
	}
	languageReqs := body.LanguageReqs
	if len(languageReqs) == 0 {
		languageReqs = json.RawMessage("{}")
	}

	arg := gen.CreateScholarshipParams{
		TenantID:             tenantID,
		GuideID:              contentRow.ID,
		Title:                body.Title,
		Description:          body.Description,
		Country:              body.Country,
		University:           body.University,
		DegreeLevel:          body.DegreeLevel,
		FieldsOfStudy:        fieldsOfStudy,
		EligibleCountries:    eligibleCountries,
		Funding:              funding,
		Deadline:             deadline,
		ApplicationUrl:       body.ApplicationUrl,
		RequiredDocuments:    requiredDocuments,
		LanguageRequirements: languageReqs,
		SourceUrl:            body.SourceUrl,
		SourceType:           body.SourceType,
		TrustLevel:           body.TrustLevel,
		ConfidenceOverall:    1.0,
		VerificationStatus:   "verified",
		FreshnessScore:       100,
		OverrideSource:       "editorial",
		EditorialOverrides:   []byte("{}"),
	}

	sch, err := h.svc.store.AdminCreateScholarship(r.Context(), tenantID, arg)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create scholarship")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]interface{}{"data": sch})
}

// HandleAdminUpdateScholarship updates an existing scholarship record.
//
// PATCH /admin/scholarships/{id}
// Requires: scholarship.manage
func (h *Handlers) HandleAdminUpdateScholarship(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Load existing record to merge with updates.
	existing, err := h.svc.store.AdminGetScholarship(r.Context(), tenantID, id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Scholarship not found")
		return
	}

	var body struct {
		Title             *string         `json:"title"`
		Description       *string         `json:"description"`
		Country           *string         `json:"country"`
		University        *string         `json:"university"`
		DegreeLevel       *string         `json:"degree_level"`
		FieldsOfStudy     []string        `json:"fields_of_study"`
		EligibleCountries []string        `json:"eligible_countries"`
		Funding           json.RawMessage `json:"funding"`
		Deadline          *string         `json:"deadline"`
		ApplicationUrl    *string         `json:"application_url"`
		RequiredDocuments []string        `json:"required_documents"`
		LanguageReqs      json.RawMessage `json:"language_requirements"`
		SourceUrl         *string         `json:"source_url"`
		SourceType        *string         `json:"source_type"`
		TrustLevel        *int16          `json:"trust_level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// Merge: use provided values or fall back to existing.
	arg := gen.UpdateScholarshipParams{
		TenantID:             tenantID,
		ID:                   id,
		Title:                existing.Title,
		Description:          existing.Description,
		Country:              existing.Country,
		CountrySpaceID:       existing.CountrySpaceID,
		University:           existing.University,
		DegreeLevel:          existing.DegreeLevel,
		FieldsOfStudy:        existing.FieldsOfStudy,
		EligibleCountries:    existing.EligibleCountries,
		Funding:              existing.Funding,
		Deadline:             existing.Deadline,
		ApplicationUrl:       existing.ApplicationUrl,
		RequiredDocuments:    existing.RequiredDocuments,
		MinimumGpa:           existing.MinimumGpa,
		LanguageRequirements: existing.LanguageRequirements,
		ConfidenceOverall:    existing.ConfidenceOverall,
		VerificationStatus:   existing.VerificationStatus,
		FreshnessScore:       existing.FreshnessScore,
		OverrideSource:       "editorial",
		EditorialOverrides:   existing.EditorialOverrides,
		SourceUrl:            existing.SourceUrl,
		SourceType:           existing.SourceType,
		TrustLevel:           existing.TrustLevel,
	}

	if body.Title != nil {
		arg.Title = *body.Title
	}
	if body.Description != nil {
		arg.Description = body.Description
	}
	if body.Country != nil {
		arg.Country = *body.Country
	}
	if body.University != nil {
		arg.University = body.University
	}
	if body.DegreeLevel != nil {
		arg.DegreeLevel = body.DegreeLevel
	}
	if body.FieldsOfStudy != nil {
		arg.FieldsOfStudy = body.FieldsOfStudy
	}
	if body.EligibleCountries != nil {
		arg.EligibleCountries = body.EligibleCountries
	}
	if body.Funding != nil {
		arg.Funding = body.Funding
	}
	if body.Deadline != nil && *body.Deadline != "" {
		var d pgtype.Date
		if err := d.Scan(*body.Deadline); err == nil {
			arg.Deadline = d
		}
	}
	if body.ApplicationUrl != nil {
		arg.ApplicationUrl = body.ApplicationUrl
	}
	if body.RequiredDocuments != nil {
		arg.RequiredDocuments = body.RequiredDocuments
	}
	if body.LanguageReqs != nil {
		arg.LanguageRequirements = body.LanguageReqs
	}
	if body.SourceUrl != nil && *body.SourceUrl != "" {
		arg.SourceUrl = *body.SourceUrl
	}
	if body.SourceType != nil && *body.SourceType != "" {
		arg.SourceType = *body.SourceType
	}
	if body.TrustLevel != nil {
		arg.TrustLevel = *body.TrustLevel
	}

	if err := h.svc.store.AdminUpdateScholarship(r.Context(), tenantID, arg); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update scholarship")
		return
	}

	// Re-fetch so the admin panel receives the updated row.
	updated, err := h.svc.store.AdminGetScholarship(r.Context(), tenantID, id)
	if err != nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": "ok"})
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": updated})
}

// HandleAdminDeleteScholarship deletes a scholarship record.
//
// DELETE /admin/scholarships/{id}
// Requires: scholarship.manage
func (h *Handlers) HandleAdminDeleteScholarship(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.store.AdminDeleteScholarship(r.Context(), tenantID, id); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete scholarship")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": "ok"})
}
