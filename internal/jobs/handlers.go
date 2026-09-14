// Package jobs — handlers.go provides the HTTP handlers for the Jobs Board
// routes (finder, detail, apply, applications, admin).
//
// Routes (Section 63, ADR-081):
//
//	GET   /api/v1/jobs                                    — finder
//	GET   /api/v1/jobs/:slug                              — job detail
//	POST  /api/v1/jobs/:id/apply                          — submit application
//	GET   /api/v1/me/jobs/applications                    — applicant history
//	GET   /api/v1/org/:org_id/jobs                        — org's listings
//	POST  /api/v1/org/:org_id/jobs                        — create listing
//	PATCH /api/v1/org/:org_id/jobs/:id                    — edit listing (auth, jobs.post)
//	GET   /api/v1/org/:org_id/jobs/:id/applications       — employer list
//	PATCH /api/v1/org/:org_id/jobs/:id/applications/:app_id — update status
//	GET   /api/v1/admin/jobs                              — admin list
//	PATCH /api/v1/admin/jobs/:id/feature                  — toggle featured
//
// Ref: japaconnect-engineering/sections/63-jobs-board.md
//
//	japaconnect-engineering/adr/ADR-081-jobs-board.md
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// createJobListingRequest is the JSON body for POST /org/{org_id}/jobs.
// Summary is intentionally absent — the contents table has no summary column
// (JOBS-004/015); a display summary is derived from body at read time.
type createJobListingRequest struct {
	Title       string            `json:"title"`
	Slug        string            `json:"slug"`
	Body        string            `json:"body"`
	FieldValues map[string]string `json:"field_values"`
	PublishedAt *string           `json:"published_at,omitempty"`
}

// updateJobListingRequest is the JSON body for PATCH /org/{org_id}/jobs/{id}.
type updateJobListingRequest struct {
	Title       string            `json:"title"`
	Slug        string            `json:"slug"`
	Body        string            `json:"body"`
	FieldValues map[string]string `json:"field_values"`
	PublishedAt *string           `json:"published_at,omitempty"`
}

// listingUpdater is the subset of Service methods used by the public job
// update handler.
type listingUpdater interface {
	IsEnabled(ctx context.Context) bool
	UpdateJobListing(ctx context.Context, orgID, jobID, callerID pgtype.UUID, input UpdateJobListingInput) (JobDetail, error)
}

// Handlers exposes the Jobs Board HTTP endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	updater  listingUpdater
}

// NewHandlers creates Jobs handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings, updater: service}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as rag/handlers.go)
// ---------------------------------------------------------------------------

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
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
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service != nil && !h.service.IsEnabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "jobs_disabled", "Jobs board is not available")
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

	results, total, err := h.service.FindJobs(r.Context(), filters)
	if err != nil {
		slog.Error("jobs: find failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "jobs_find_failed", "Failed to search jobs")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Job slug is required")
		return
	}

	detail, err := h.service.GetJobDetail(r.Context(), slug)
	if err != nil {
		slog.Error("jobs: detail failed", "error", err, "slug", slug)
		errpkg.WriteError(w, http.StatusNotFound, "job_not_found", "Job not found")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
	_, ok = requireTenant(w, r)
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

	app, err := h.service.ApplyToJob(r.Context(), jobID, user.UserID, req.SubmissionID)
	if err != nil {
		if errors.Is(err, ErrDuplicateApplication) {
			errpkg.WriteError(w, http.StatusConflict, "already_applied", "You have already applied to this job")
			return
		}

		slog.Error("jobs: apply failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "apply_failed", "Failed to submit application")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
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
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)

	results, total, err := h.service.ListUserApplications(r.Context(), user.UserID, page, limit)
	if err != nil {
		slog.Error("jobs: my applications failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "applications_list_failed", "Failed to list applications")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": results,
		"meta": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleListOrgJobs handles GET /api/v1/org/:org_id/jobs — lists the org's
// published job listings. Auth required (org admin / jobs.post). The caller
// must be a member of the org with an editor-or-above role (JOBS-012).
func (h *Handlers) HandleListOrgJobs(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	// JOBS-012: jobs.post is granted to the default member role, so verify
	// the caller actually belongs to this org with an editor-or-above role.
	member, err := h.service.IsOrgMember(r.Context(), user.UserID, orgID, OrgJobEditorRoles)
	if err != nil {
		slog.Error("jobs: org membership check failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "org_check_failed", "Failed to verify org membership")

		return
	}

	if !member {
		errpkg.WriteError(w, http.StatusForbidden, "not_org_member", "You are not a member of this organization")
		return
	}

	page, limit := parsePagination(r)

	results, total, err := h.service.ListOrgJobs(r.Context(), orgID, page, limit)
	if err != nil {
		slog.Error("jobs: list org jobs failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "org_jobs_list_failed", "Failed to list org jobs")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
// jobs.post). The caller must be an editor-or-above member of the org
// (JOBS-012).
func (h *Handlers) HandleCreateJobListing(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	// JOBS-012: verify org membership (editor-or-above) before creating.
	member, err := h.service.IsOrgMember(r.Context(), user.UserID, orgID, OrgJobEditorRoles)
	if err != nil {
		slog.Error("jobs: org membership check failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "org_check_failed", "Failed to verify org membership")

		return
	}

	if !member {
		errpkg.WriteError(w, http.StatusForbidden, "not_org_member", "You are not a member of this organization")
		return
	}

	var req createJobListingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Job title is required")
		return
	}

	listing, err := h.service.CreateJobListing(r.Context(), orgID, user.UserID, req.Title, req.Slug, "", req.Body, req.FieldValues)
	if err != nil {
		slog.Error("jobs: create listing failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_listing_failed", "Failed to create job listing")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": listing,
	})
}

// HandleUpdateJob handles PATCH /api/v1/org/:org_id/jobs/:id — updates a job
// listing's title, slug, body, field values and published_at. Auth required
// (jobs.post). The caller must be an editor-or-above member of the org
// (JOBS-012).
func (h *Handlers) HandleUpdateJob(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	if h.updater == nil || !h.updater.IsEnabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "jobs_disabled", "Jobs board is not available")
		return
	}

	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	jobID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var req updateJobListingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	listing, err := h.updater.UpdateJobListing(r.Context(), orgID, jobID, user.UserID, UpdateJobListingInput(req))
	if err != nil {
		if errors.Is(err, ErrNotOrgMember) {
			errpkg.WriteError(w, http.StatusForbidden, "not_org_member", "You are not a member of this organization")
			return
		}

		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "job_not_found", "Job not found")
			return
		}

		slog.Error("jobs: update listing failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update job listing")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": listing})
}

// HandleListJobApplications handles GET /api/v1/org/:org_id/jobs/:id/applications
// — employer view of applications for a job. Auth required (org admin). The
// caller must be an admin of the org that owns the job (JOBS-012).
func (h *Handlers) HandleListJobApplications(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	jobID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// JOBS-012: verify the caller is an admin of the org and the job
	// actually belongs to that org (prevents reaching applications via a
	// different org's ID).
	if !requireOrgJobAdmin(w, r, h, user.UserID, orgID, jobID) {
		return
	}

	page, limit := parsePagination(r)
	status := r.URL.Query().Get("status")

	results, total, err := h.service.ListJobApplications(r.Context(), jobID, status, page, limit)
	if err != nil {
		slog.Error("jobs: list job applications failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "applications_list_failed", "Failed to list applications")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
// status. Auth required (org admin). The caller must be an admin of the org
// that owns the job (JOBS-012).
func (h *Handlers) HandleUpdateApplicationStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	jobID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	appID, ok := scanUUIDParam(w, r, "app_id")
	if !ok {
		return
	}

	// JOBS-012: verify org admin + job ownership before mutating.
	if !requireOrgJobAdmin(w, r, h, user.UserID, orgID, jobID) {
		return
	}

	var req updateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if !IsValidStatus(req.Status) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Invalid status value")
		return
	}

	app, err := h.service.UpdateApplicationStatus(r.Context(), appID, req.Status, req.EmployerNotes)
	if err != nil {
		slog.Error("jobs: update status failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update application status")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":                app.ID.String(),
			"status":            app.Status,
			"employer_notes":    app.EmployerNotes,
			"status_updated_at": formatTimestamptz(app.StatusUpdatedAt),
		},
	})
}

// requireOrgJobAdmin verifies the user is an admin of the org and that the
// job belongs to that org. It writes the appropriate error response and
// returns false on failure. Used by the application list/update handlers
// (JOBS-012).
func requireOrgJobAdmin(w http.ResponseWriter, r *http.Request, h *Handlers, userID, orgID, jobID pgtype.UUID) bool {
	member, err := h.service.IsOrgMember(r.Context(), userID, orgID, OrgJobAdminRoles)
	if err != nil {
		slog.Error("jobs: org admin check failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "org_check_failed", "Failed to verify org membership")

		return false
	}

	if !member {
		errpkg.WriteError(w, http.StatusForbidden, "not_org_admin", "You are not an admin of this organization")
		return false
	}

	owns, err := h.service.JobBelongsToOrg(r.Context(), jobID, orgID)
	if err != nil {
		slog.Error("jobs: job ownership check failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "org_check_failed", "Failed to verify job ownership")

		return false
	}

	if !owns {
		errpkg.WriteError(w, http.StatusNotFound, "job_not_found", "Job not found in this organization")
		return false
	}

	return true
}

// HandleAdminListJobs handles GET /api/v1/admin/jobs — admin view of all job
// listings (Section 63: "all listings (admin)"). Admin only.
func (h *Handlers) HandleAdminListJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)
	status := r.URL.Query().Get("status")

	results, total, err := h.service.AdminListJobs(r.Context(), status, page, limit)
	if err != nil {
		slog.Error("jobs: admin list jobs failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list job listings")

		return
	}

	// Include listing status distribution for the admin dashboard.
	dist, _ := h.service.AdminJobStatusDistribution(r.Context())

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	jobID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	featured, err := h.service.ToggleFeature(r.Context(), jobID)
	if err != nil {
		slog.Error("jobs: toggle feature failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "feature_toggle_failed", "Failed to toggle featured status")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":          jobID.String(),
			"is_featured": featured,
		},
	})
}

// HandleAdminUpdateJob updates a job listing's content (title, summary, body,
// status) and custom field values. Admin only.
//
// PATCH /admin/jobs/{id}
// Requires: jobs.manage.
func (h *Handlers) HandleAdminUpdateJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	jobID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Title       *string           `json:"title"`
		Summary     *string           `json:"summary"`
		Body        *string           `json:"body"`
		Status      *string           `json:"status"`
		FieldValues map[string]string `json:"field_values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if err := h.service.AdminUpdateJob(r.Context(), jobID, body.Title, body.Summary, body.Body, body.Status, body.FieldValues); err != nil {
		slog.Error("jobs: admin update failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update job listing")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": "ok"})
}

// HandleAdminDeleteJob soft-deletes a job listing. Admin only.
//
// DELETE /admin/jobs/{id}
// Requires: jobs.manage.
func (h *Handlers) HandleAdminDeleteJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	jobID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.AdminDeleteJob(r.Context(), jobID); err != nil {
		slog.Error("jobs: admin delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete job listing")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": "ok"})
}

// formatTimestamptz converts a pgtype.Timestamptz to an RFC 3339 UTC string.
func formatTimestamptz(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}

	return ts.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
}
