// Package jobs — handlers.go provides the HTTP handlers for the Jobs Board
// routes (finder, detail, apply, applications, admin).
//
// Routes (Section 63, ADR-081):
//
//	GET   /api/v1/jobs                                    — finder
//	GET   /api/v1/jobs/:slug                              — job detail
//	POST  /api/v1/jobs/:id/apply                          — submit application
//	GET   /api/v1/me/jobs/applications                    — applicant history
//	GET   /api/v1/org/:org_id/jobs/:id/applications       — employer list
//	PATCH /api/v1/org/:org_id/jobs/:id/applications/:app_id — update status
//	GET   /api/v1/admin/jobs                              — admin list
//	PATCH /api/v1/admin/jobs/:id/feature                  — toggle featured
//
// Ref: japaconnect-engineering/sections/63-jobs-board.md
//      japaconnect-engineering/adr/ADR-081-jobs-board.md
package jobs

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/tenant"
)

// createJobListingRequest is the JSON body for POST /org/{org_id}/jobs.
type createJobListingRequest struct {
	Title           string            `json:"title"`
	Slug            string            `json:"slug"`
	Summary         string            `json:"summary"`
	Body            string            `json:"body"`
	FieldValues     map[string]string `json:"field_values"`
	PublishedAt     *string           `json:"published_at,omitempty"`
}

// Handlers exposes the Jobs Board HTTP endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates Jobs handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as rag/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("jobs: encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

// parsePagination extracts page and limit from query params with defaults.
func parsePagination(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return page, limit
}

// parseBoolParam parses a boolean query param (e.g. visa_sponsorship=true).
func parseBoolParam(r *http.Request, key string) *bool {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil
	}
	return &b
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// HandleFindJobs handles GET /api/v1/jobs — public job finder with server-side
// filtering on custom_field_values.
func (h *Handlers) HandleFindJobs(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service != nil && !h.service.IsEnabled(r.Context(), tenantID) {
		writeError(w, http.StatusForbidden, "jobs_disabled", "Jobs board is disabled")
		return
	}

	page, limit := parsePagination(r)
	q := r.URL.Query()

	filters := JobFilters{
		Country:         q.Get("country"),
		JobType:         q.Get("job_type"),
		LocationType:    q.Get("remote"),
		VisaSponsorship: parseBoolParam(r, "visa_sponsorship"),
		Q:               q.Get("q"),
		Sort:            q.Get("sort"),
		Page:            page,
		Limit:           limit,
	}

	// The "remote" query param is a shorthand for location_type=remote.
	if filters.LocationType == "" && q.Get("location_type") != "" {
		filters.LocationType = q.Get("location_type")
	}
	if filters.LocationType == "true" || filters.LocationType == "1" {
		filters.LocationType = "remote"
	}

	results, total, err := h.service.FindJobs(r.Context(), tenantID, filters)
	if err != nil {
		slog.Error("jobs: find failed", "error", err)
		writeError(w, http.StatusInternalServerError, "jobs_find_failed", "Failed to search jobs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": results,
		"meta": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleJobDetail handles GET /api/v1/jobs/:slug — public job detail.
func (h *Handlers) HandleJobDetail(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Job slug is required")
		return
	}

	detail, err := h.service.GetJobDetail(r.Context(), tenantID, slug)
	if err != nil {
		slog.Error("jobs: detail failed", "error", err, "slug", slug)
		writeError(w, http.StatusNotFound, "job_not_found", "Job not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": detail,
	})
}

// applyRequest is the JSON body for POST /jobs/:id/apply.
type applyRequest struct {
	SubmissionID *uuid.UUID `json:"submission_id"`
}

// HandleApply handles POST /api/v1/jobs/:id/apply — submit an on-platform
// application. Auth required.
func (h *Handlers) HandleApply(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	jobID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var req applyRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	app, err := h.service.ApplyToJob(r.Context(), tenantID, jobID, user.UserID, req.SubmissionID)
	if err != nil {
		if err == ErrDuplicateApplication {
			writeError(w, http.StatusConflict, "already_applied", "You have already applied to this job")
			return
		}
		slog.Error("jobs: apply failed", "error", err)
		writeError(w, http.StatusInternalServerError, "apply_failed", "Failed to submit application")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"id":         app.ID.String(),
			"job_id":     app.JobID.String(),
			"status":     app.Status,
			"applied_at": formatTimestamptz(app.AppliedAt),
		},
	})
}

// HandleMyApplications handles GET /api/v1/me/jobs/applications — the
// authenticated user's application history. Auth required.
func (h *Handlers) HandleMyApplications(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)

	results, total, err := h.service.ListUserApplications(r.Context(), tenantID, user.UserID, page, limit)
	if err != nil {
		slog.Error("jobs: my applications failed", "error", err)
		writeError(w, http.StatusInternalServerError, "applications_list_failed", "Failed to list applications")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": results,
		"meta": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleListOrgJobs handles GET /api/v1/org/:org_id/jobs — lists the org's
// published job listings. Auth required (org admin / jobs.post).
func (h *Handlers) HandleListOrgJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	page, limit := parsePagination(r)

	results, total, err := h.service.ListOrgJobs(r.Context(), tenantID, orgID, page, limit)
	if err != nil {
		slog.Error("jobs: list org jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "org_jobs_list_failed", "Failed to list org jobs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": results,
		"meta": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleCreateJobListing handles POST /api/v1/org/:org_id/jobs — creates a
// job_listing content with custom field values. Auth required (org admin /
// jobs.post).
func (h *Handlers) HandleCreateJobListing(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	var req createJobListingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Job title is required")
		return
	}

	listing, err := h.service.CreateJobListing(r.Context(), tenantID, orgID, user.UserID, req.Title, req.Slug, req.Summary, req.Body, req.FieldValues)
	if err != nil {
		slog.Error("jobs: create listing failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_listing_failed", "Failed to create job listing")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": listing,
	})
}

// HandleListJobApplications handles GET /api/v1/org/:org_id/jobs/:id/applications
// — employer view of applications for a job. Auth required (org admin).
func (h *Handlers) HandleListJobApplications(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	jobID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	page, limit := parsePagination(r)
	status := r.URL.Query().Get("status")

	results, total, err := h.service.ListJobApplications(r.Context(), tenantID, jobID, status, page, limit)
	if err != nil {
		slog.Error("jobs: list job applications failed", "error", err)
		writeError(w, http.StatusInternalServerError, "applications_list_failed", "Failed to list applications")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": results,
		"meta": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// updateStatusRequest is the JSON body for PATCH application status.
type updateStatusRequest struct {
	Status        string  `json:"status"`
	EmployerNotes *string `json:"employer_notes"`
}

// HandleUpdateApplicationStatus handles PATCH
// /api/v1/org/:org_id/jobs/:id/applications/:app_id — update application
// status. Auth required (org admin).
func (h *Handlers) HandleUpdateApplicationStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	appID, ok := scanUUIDParam(w, r, "app_id")
	if !ok {
		return
	}

	var req updateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if !IsValidStatus(req.Status) {
		writeError(w, http.StatusBadRequest, "invalid_status", "Invalid status value")
		return
	}

	app, err := h.service.UpdateApplicationStatus(r.Context(), tenantID, appID, req.Status, req.EmployerNotes)
	if err != nil {
		slog.Error("jobs: update status failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update application status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":                app.ID.String(),
			"status":            app.Status,
			"employer_notes":    app.EmployerNotes,
			"status_updated_at": formatTimestamptz(app.StatusUpdatedAt),
		},
	})
}

// HandleAdminListJobs handles GET /api/v1/admin/jobs — admin view of all job
// listings (Section 63: "all listings (admin)"). Admin only.
func (h *Handlers) HandleAdminListJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)
	status := r.URL.Query().Get("status")

	results, total, err := h.service.AdminListJobs(r.Context(), tenantID, status, page, limit)
	if err != nil {
		slog.Error("jobs: admin list jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list job listings")
		return
	}

	// Include listing status distribution for the admin dashboard.
	dist, _ := h.service.AdminJobStatusDistribution(r.Context(), tenantID)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": results,
		"meta": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
		"status_distribution": dist,
	})
}

// HandleAdminToggleFeature handles PATCH /api/v1/admin/jobs/:id/feature —
// toggles the is_featured custom field value. Admin only.
func (h *Handlers) HandleAdminToggleFeature(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	jobID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	featured, err := h.service.ToggleFeature(r.Context(), tenantID, jobID)
	if err != nil {
		slog.Error("jobs: toggle feature failed", "error", err)
		writeError(w, http.StatusInternalServerError, "feature_toggle_failed", "Failed to toggle featured status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":         jobID.String(),
			"is_featured": featured,
		},
	})
}

// formatTimestamptz converts a pgtype.Timestamptz to an RFC 3339 UTC string.
func formatTimestamptz(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
}
