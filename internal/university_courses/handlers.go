// Package university_courses — HTTP handlers for the institutional course
// catalog and per-university course offerings (Section 39).
//
// Two surfaces:
//   - Public:  GET /universities/{slug}/courses[/{offeringSlug}] — published
//     offerings rendered on the university page's Programs tab and the public
//     course detail page.
//   - Admin:   /admin/course-catalog and /admin/course-offerings CRUD, gated
//     by the universities.course_catalog.* / universities.course_offering.*
//     RBAC nodes seeded in migration 000730.
//
// These are distinct from the Japa University creator tracks under
// /admin/university/* (ADR-063) — this package manages the degree programs a
// real-world university offers.
package university_courses

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// SpacesResolver is the subset of spaces.Service the handlers need to resolve
// a university slug/ID to its space row.
type SpacesResolver interface {
	GetSpaceBySlug(ctx context.Context, slug string) (gen.GetSpaceBySlugRow, error)
	GetSpace(ctx context.Context, id pgtype.UUID) (gen.GetSpaceByIDRow, error)
}

// Handlers owns the public + admin university course HTTP endpoints.
type Handlers struct {
	svc    *Service
	spaces SpacesResolver
}

// NewHandlers creates the university_courses handlers. spaces may be nil in
// tests — the public list and offering-slug defaulting degrade to errors when
// a resolution is actually required.
func NewHandlers(svc *Service, spaces SpacesResolver) *Handlers {
	return &Handlers{svc: svc, spaces: spaces}
}

var (
	invalidSlugChars = regexp.MustCompile(`[^a-z0-9-]+`)
	multipleHyphens  = regexp.MustCompile(`-{2,}`)
	trimHyphens      = regexp.MustCompile(`^-+|-+$`)
)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = invalidSlugChars.ReplaceAllString(s, "-")
	s = multipleHyphens.ReplaceAllString(s, "-")
	s = trimHyphens.ReplaceAllString(s, "")

	return s
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}

	return n
}

// parseBoolPtr parses a tri-state query param ("true"/"false"/"") into *bool.
func parseBoolPtr(s string) *bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		v := true
		return &v
	case "false", "0", "no":
		v := false
		return &v
	default:
		return nil
	}
}

func scanUUID(s string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if strings.TrimSpace(s) == "" {
		return id, false
	}
	if err := id.Scan(s); err != nil {
		return pgtype.UUID{}, false
	}
	return id, true
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b, _ := id.MarshalJSON()
	return strings.Trim(string(b), `"`)
}

func numericString(n pgtype.Numeric) any {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil {
		return nil
	}
	return f.Float64
}

// offeringToJSON serializes a CourseOffering for API responses. Numeric
// columns are emitted as numbers (or null); IDs as strings.
func offeringToJSON(o CourseOffering) map[string]any {
	return map[string]any{
		"id":                uuidString(o.ID),
		"course_id":         uuidString(o.CourseID),
		"university_id":     uuidString(o.UniversityID),
		"slug":              o.Slug,
		"tuition":           numericString(o.Tuition),
		"currency":          o.Currency,
		"duration_years":    numericString(o.DurationYears),
		"study_mode":        o.StudyMode,
		"intake_months":     o.IntakeMonths,
		"entry_requirements": o.EntryRequirements,
		"accreditation":     o.Accreditation,
		"is_published":      o.IsPublished,
		"created_at":        o.CreatedAt.Time,
		"updated_at":        o.UpdatedAt.Time,
		"course": map[string]any{
			"title":                  o.CourseTitle,
			"slug":                   o.CourseSlug,
			"description":            o.CourseDescription,
			"degree_type":            o.DegreeType,
			"department":             o.Department,
			"field_of_study":         o.FieldOfStudy,
			"language":               o.Language,
			"default_duration_years": numericString(o.DefaultDurationYears),
		},
		"university": map[string]any{
			"name": o.UniversityName,
			"slug": o.UniversitySlug,
		},
	}
}

// catalogToJSON serializes a CourseCatalog for API responses.
func catalogToJSON(c CourseCatalog) map[string]any {
	return map[string]any{
		"id":                     uuidString(c.ID),
		"title":                  c.Title,
		"slug":                   c.Slug,
		"description":            c.Description,
		"degree_type":            c.DegreeType,
		"department":             c.Department,
		"field_of_study":         c.FieldOfStudy,
		"language":               c.Language,
		"default_duration_years": numericString(c.DefaultDurationYears),
		"is_published":           c.IsPublished,
		"offering_count":         c.OfferingCount,
		"created_at":             c.CreatedAt.Time,
		"updated_at":             c.UpdatedAt.Time,
	}
}

// ---------------------------------------------------------------------------
// Public routes
// ---------------------------------------------------------------------------

// ListUniversityCourses handles GET /api/v1/universities/{slug}/courses —
// published course offerings at a university, paginated. Honors
// ?q=&degree_type=&page=&limit= per Section 39. [public]
func (h *Handlers) ListUniversityCourses(w http.ResponseWriter, r *http.Request) {
	if h.spaces == nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "University lookup unavailable")
		return
	}

	slug := chi.URLParam(r, "slug")
	space, err := h.spaces.GetSpaceBySlug(r.Context(), slug)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	limit := parseIntDefault(r.URL.Query().Get("limit"), 25)
	if limit < 1 || limit > 100 {
		limit = 25
	}
	if page < 1 {
		page = 1
	}

	offset := int32((page - 1) * limit) // #nosec
	limit32 := int32(limit)             // #nosec

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	degreeType := strings.TrimSpace(r.URL.Query().Get("degree_type"))

	offerings, err := h.svc.SearchOfferings(r.Context(), space.ID, search, degreeType, limit32, offset)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	total, err := h.svc.CountOfferingsByUniversity(r.Context(), space.ID, search, degreeType)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(offerings))
	for _, o := range offerings {
		items = append(items, offeringToJSON(o))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": (int(total) + limit - 1) / limit,
		},
	})
}

// GetUniversityCourse handles
// GET /api/v1/universities/{slug}/courses/{offeringSlug} — a single published
// offering scoped to its university. The nested route is canonical: an
// offering requested under a different university slug returns 404. [public]
func (h *Handlers) GetUniversityCourse(w http.ResponseWriter, r *http.Request) {
	offering, err := h.svc.GetPublishedOfferingBySlugs(
		r.Context(),
		chi.URLParam(r, "slug"),
		chi.URLParam(r, "offeringSlug"),
	)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, offeringToJSON(offering))
}

// ---------------------------------------------------------------------------
// Admin: course catalog
// ---------------------------------------------------------------------------

// AdminListCatalog handles GET /api/v1/admin/course-catalog — server-side
// paginated catalog list with ?q=&degree_type=&is_published=&sort= filters.
// [admin: universities.course_catalog.list]
func (h *Handlers) AdminListCatalog(w http.ResponseWriter, r *http.Request) {
	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	limit := parseIntDefault(r.URL.Query().Get("limit"), 25)
	if limit < 1 || limit > 200 {
		limit = 25
	}
	if page < 1 {
		page = 1
	}

	f := CatalogListFilters{
		Search:      strings.TrimSpace(r.URL.Query().Get("q")),
		DegreeType:  strings.TrimSpace(r.URL.Query().Get("degree_type")),
		IsPublished: parseBoolPtr(r.URL.Query().Get("is_published")),
		Sort:        strings.TrimSpace(r.URL.Query().Get("sort")),
		Limit:       int32(limit), // #nosec
		Offset:      int32((page - 1) * limit), // #nosec
	}

	rows, err := h.svc.ListCatalogAdmin(r.Context(), f)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	total, err := h.svc.CountCatalogAdmin(r.Context(), f)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		items = append(items, catalogToJSON(c))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": (int(total) + limit - 1) / limit,
		},
	})
}

// AdminGetCatalog handles GET /api/v1/admin/course-catalog/{id}.
// [admin: universities.course_catalog.view]
func (h *Handlers) AdminGetCatalog(w http.ResponseWriter, r *http.Request) {
	id, ok := scanUUID(chi.URLParam(r, "id"))
	if !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid course catalog ID")
		return
	}

	c, err := h.svc.GetCatalogByID(r.Context(), id)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, catalogToJSON(c))
}

// catalogRequest is the create/update body for a catalog course.
type catalogRequest struct {
	Title                string   `json:"title"`
	Slug                 string   `json:"slug"`
	Description          *string  `json:"description"`
	DegreeType           *string  `json:"degree_type"`
	Department           *string  `json:"department"`
	FieldOfStudy         *string  `json:"field_of_study"`
	Language             *string  `json:"language"`
	DefaultDurationYears *float64 `json:"default_duration_years"`
	IsPublished          *bool    `json:"is_published"`
}

func numericFromFloat(f *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if f == nil {
		return n
	}
	if err := n.Scan(*f); err != nil {
		return pgtype.Numeric{}
	}
	return n
}

// AdminCreateCatalog handles POST /api/v1/admin/course-catalog.
// [admin: universities.course_catalog.create]
func (h *Handlers) AdminCreateCatalog(w http.ResponseWriter, r *http.Request) {
	var req catalogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid JSON body")
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "title is required")
		return
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(req.Title)
	}

	var lang any
	if req.Language != nil && strings.TrimSpace(*req.Language) != "" {
		lang = strings.TrimSpace(*req.Language)
	}

	c, err := h.svc.CreateCatalog(r.Context(), gen.CreateCourseCatalogParams{
		Title:                req.Title,
		Slug:                 slug,
		Description:          req.Description,
		DegreeType:           req.DegreeType,
		Department:           req.Department,
		FieldOfStudy:         req.FieldOfStudy,
		Language:             lang,
		DefaultDurationYears: numericFromFloat(req.DefaultDurationYears),
		IsPublished:          req.IsPublished,
	})
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, catalogToJSON(c))
}

// AdminUpdateCatalog handles PATCH /api/v1/admin/course-catalog/{id}.
// [admin: universities.course_catalog.update]
func (h *Handlers) AdminUpdateCatalog(w http.ResponseWriter, r *http.Request) {
	id, ok := scanUUID(chi.URLParam(r, "id"))
	if !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid course catalog ID")
		return
	}

	var req catalogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid JSON body")
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "title is required")
		return
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(req.Title)
	}

	lang := "English"
	if req.Language != nil && strings.TrimSpace(*req.Language) != "" {
		lang = strings.TrimSpace(*req.Language)
	}

	published := true
	if req.IsPublished != nil {
		published = *req.IsPublished
	}

	c, err := h.svc.UpdateCatalog(r.Context(), gen.UpdateCourseCatalogParams{
		ID:                   id,
		Title:                req.Title,
		Slug:                 slug,
		Description:          req.Description,
		DegreeType:           req.DegreeType,
		Department:           req.Department,
		FieldOfStudy:         req.FieldOfStudy,
		Language:             lang,
		DefaultDurationYears: numericFromFloat(req.DefaultDurationYears),
		IsPublished:          published,
	})
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, catalogToJSON(c))
}

// AdminDeleteCatalog handles DELETE /api/v1/admin/course-catalog/{id} —
// soft delete. Offerings referencing the catalog row stay admin-visible but
// drop out of public joins (Section 39). [admin: universities.course_catalog.delete]
func (h *Handlers) AdminDeleteCatalog(w http.ResponseWriter, r *http.Request) {
	id, ok := scanUUID(chi.URLParam(r, "id"))
	if !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid course catalog ID")
		return
	}

	if err := h.svc.SoftDeleteCatalog(r.Context(), id); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Admin: course offerings
// ---------------------------------------------------------------------------

// AdminListOfferings handles GET /api/v1/admin/course-offerings — server-side
// paginated list with ?q=&university_id=&course_id=&is_published=&sort=.
// [admin: universities.course_offering.list]
func (h *Handlers) AdminListOfferings(w http.ResponseWriter, r *http.Request) {
	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	limit := parseIntDefault(r.URL.Query().Get("limit"), 25)
	if limit < 1 || limit > 200 {
		limit = 25
	}
	if page < 1 {
		page = 1
	}

	f := OfferingListFilters{
		Search:      strings.TrimSpace(r.URL.Query().Get("q")),
		IsPublished: parseBoolPtr(r.URL.Query().Get("is_published")),
		Sort:        strings.TrimSpace(r.URL.Query().Get("sort")),
		Limit:       int32(limit), // #nosec
		Offset:      int32((page - 1) * limit), // #nosec
	}
	if id, ok := scanUUID(r.URL.Query().Get("university_id")); ok {
		f.UniversityID = id
	}
	if id, ok := scanUUID(r.URL.Query().Get("course_id")); ok {
		f.CourseID = id
	}

	rows, err := h.svc.ListOfferingsAdmin(r.Context(), f)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	total, err := h.svc.CountOfferingsAdmin(r.Context(), f)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(rows))
	for _, o := range rows {
		items = append(items, offeringToJSON(o))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": (int(total) + limit - 1) / limit,
		},
	})
}

// AdminGetOffering handles GET /api/v1/admin/course-offerings/{id}.
// [admin: universities.course_offering.view]
func (h *Handlers) AdminGetOffering(w http.ResponseWriter, r *http.Request) {
	id, ok := scanUUID(chi.URLParam(r, "id"))
	if !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid course offering ID")
		return
	}

	o, err := h.svc.GetOfferingByID(r.Context(), id)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, offeringToJSON(o))
}

// offeringRequest is the create/update body for a course offering.
type offeringRequest struct {
	CourseID          string    `json:"course_id"`
	UniversityID      string    `json:"university_id"`
	Slug              string    `json:"slug"`
	Tuition           *float64  `json:"tuition"`
	Currency          *string   `json:"currency"`
	DurationYears     *float64  `json:"duration_years"`
	StudyMode         *string   `json:"study_mode"`
	IntakeMonths      *[]string `json:"intake_months"`
	EntryRequirements *string   `json:"entry_requirements"`
	Accreditation     *string   `json:"accreditation"`
	IsPublished       *bool     `json:"is_published"`
}

// defaultOfferingSlug builds the conventional offering slug
// ({university_slug}-{course_slug}) when the admin leaves slug empty.
func (h *Handlers) defaultOfferingSlug(ctx context.Context, courseID, universityID pgtype.UUID) (string, error) {
	course, err := h.svc.GetCatalogByID(ctx, courseID)
	if err != nil {
		return "", err
	}

	if h.spaces == nil {
		return "", errpkg.ErrRequired
	}

	space, err := h.spaces.GetSpace(ctx, universityID)
	if err != nil {
		return "", err
	}

	return slugify(space.Slug + "-" + course.Slug), nil
}

// AdminCreateOffering handles POST /api/v1/admin/course-offerings.
// [admin: universities.course_offering.create]
func (h *Handlers) AdminCreateOffering(w http.ResponseWriter, r *http.Request) {
	var req offeringRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid JSON body")
		return
	}

	courseID, ok := scanUUID(req.CourseID)
	if !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "course_id is required and must be a UUID")
		return
	}

	universityID, ok := scanUUID(req.UniversityID)
	if !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "university_id is required and must be a UUID")
		return
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		var err error
		slug, err = h.defaultOfferingSlug(r.Context(), courseID, universityID)
		if err != nil {
			errpkg.HandleServiceError(w, err)
			return
		}
	}

	var intake []string
	if req.IntakeMonths != nil {
		intake = *req.IntakeMonths
	}

	id, err := h.svc.CreateOffering(r.Context(), gen.CreateCourseOfferingParams{
		CourseID:          courseID,
		UniversityID:      universityID,
		Slug:              slug,
		Tuition:           numericFromFloat(req.Tuition),
		Currency:          req.Currency,
		DurationYears:     numericFromFloat(req.DurationYears),
		StudyMode:         req.StudyMode,
		IntakeMonths:      intake,
		EntryRequirements: req.EntryRequirements,
		Accreditation:     req.Accreditation,
		IsPublished:       req.IsPublished,
	})
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	o, err := h.svc.GetOfferingByID(r.Context(), id)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, offeringToJSON(o))
}

// AdminUpdateOffering handles PATCH /api/v1/admin/course-offerings/{id}.
// [admin: universities.course_offering.update]
func (h *Handlers) AdminUpdateOffering(w http.ResponseWriter, r *http.Request) {
	id, ok := scanUUID(chi.URLParam(r, "id"))
	if !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid course offering ID")
		return
	}

	var req offeringRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid JSON body")
		return
	}

	// Full-row update — load the current row so omitted fields keep their
	// values rather than being zeroed.
	current, err := h.svc.GetOfferingByID(r.Context(), id)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	courseID := current.CourseID
	if v, ok := scanUUID(req.CourseID); ok {
		courseID = v
	}
	universityID := current.UniversityID
	if v, ok := scanUUID(req.UniversityID); ok {
		universityID = v
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = current.Slug
	}

	var intake []string
	if req.IntakeMonths != nil {
		intake = *req.IntakeMonths
	} else {
		intake = current.IntakeMonths
	}

	published := current.IsPublished
	if req.IsPublished != nil {
		published = *req.IsPublished
	}

	if _, err := h.svc.UpdateOffering(r.Context(), gen.UpdateCourseOfferingParams{
		ID:                id,
		CourseID:          courseID,
		UniversityID:      universityID,
		Slug:              slug,
		Tuition:           numericFromFloat(req.Tuition),
		Currency:          req.Currency,
		DurationYears:     numericFromFloat(req.DurationYears),
		StudyMode:         req.StudyMode,
		IntakeMonths:      intake,
		EntryRequirements: req.EntryRequirements,
		Accreditation:     req.Accreditation,
		IsPublished:       published,
	}); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	o, err := h.svc.GetOfferingByID(r.Context(), id)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, offeringToJSON(o))
}

// AdminDeleteOffering handles DELETE /api/v1/admin/course-offerings/{id} —
// soft delete. [admin: universities.course_offering.delete]
func (h *Handlers) AdminDeleteOffering(w http.ResponseWriter, r *http.Request) {
	id, ok := scanUUID(chi.URLParam(r, "id"))
	if !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid course offering ID")
		return
	}

	if err := h.svc.SoftDeleteOffering(r.Context(), id); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// bulkOfferingRequest is the POST /admin/course-offerings/bulk body.
type bulkOfferingRequest struct {
	Action string   `json:"action"`
	IDs    []string `json:"ids"`
}

// AdminBulkOfferings handles POST /api/v1/admin/course-offerings/bulk —
// {action: publish|unpublish|delete, ids: []}.
// [admin: universities.course_offering.update / .delete]
func (h *Handlers) AdminBulkOfferings(w http.ResponseWriter, r *http.Request) {
	var req bulkOfferingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "Invalid JSON body")
		return
	}

	if len(req.IDs) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "ids is required")
		return
	}

	ids := make([]pgtype.UUID, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id, ok := scanUUID(raw)
		if !ok {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "ids must be UUIDs")
			return
		}
		ids = append(ids, id)
	}

	var err error
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "publish":
		err = h.svc.BulkSetOfferingsPublished(r.Context(), ids, true)
	case "unpublish":
		err = h.svc.BulkSetOfferingsPublished(r.Context(), ids, false)
	case "delete":
		err = h.svc.BulkSoftDeleteOfferings(r.Context(), ids)
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "action must be publish, unpublish, or delete")
		return
	}
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "affected": len(ids)})
}
