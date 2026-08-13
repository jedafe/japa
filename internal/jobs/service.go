// Package jobs — service.go holds the business logic for the Jobs Board.
// It reads jobs.* system_settings via the hot-reloadable config.Settings and
// enqueues Asynq tasks fire-and-forget (Rule 8) for application status
// notifications.
//
// Ref: japaconnect-engineering/sections/63-jobs-board.md
//      japaconnect-engineering/adr/ADR-081-jobs-board.md
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
)

// Application status constants — match the CHECK constraint on
// job_applications.status.
const (
	StatusApplied    = "applied"
	StatusReviewing  = "reviewing"
	StatusShortlisted = "shortlisted"
	StatusRejected   = "rejected"
	StatusOffered    = "offered"
	StatusWithdrawn  = "withdrawn"
)

// validStatuses is the set of statuses accepted by the CHECK constraint.
var validStatuses = map[string]bool{
	StatusApplied:     true,
	StatusReviewing:   true,
	StatusShortlisted: true,
	StatusRejected:    true,
	StatusOffered:     true,
	StatusWithdrawn:   true,
}

// IsValidStatus returns true if the given status is one of the allowed values.
func IsValidStatus(status string) bool {
	return validStatuses[status]
}

// Task type constants for Asynq (Section 11 catalog).
const (
	TaskNotifyApplicationStatus = "jobs:notify_application_status"
	TaskExpireListings          = "jobs:expire_listings"
)

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// ErrDuplicateApplication is returned when a user has already applied to a job.
var ErrDuplicateApplication = errors.New("jobs: already applied to this job")

// JobFilters holds the server-side filter parameters for the job finder.
type JobFilters struct {
	Country         string
	JobType         string
	LocationType    string // remote, onsite, hybrid
	VisaSponsorship *bool
	Q               string // text search on title
	Sort            string // newest, deadline_asc, salary_desc
	Page            int
	Limit           int
}

// JobResult is a single job listing in the finder results.
type JobResult struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Slug         string            `json:"slug"`
	Summary      string            `json:"summary"`
	FieldValues  map[string]string `json:"field_values"`
	PublishedAt  string            `json:"published_at"`
}

// JobDetail is a single job listing with all its custom field values.
type JobDetail struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Slug        string            `json:"slug"`
	Summary     string            `json:"summary"`
	Body        string            `json:"body"`
	FieldValues map[string]string `json:"field_values"`
	PublishedAt string            `json:"published_at"`
}

// AdminJobListing is a job listing row in the admin list view. It extends
// JobResult with the content status, featured flag, org/author IDs, and
// created_at — the fields an admin needs to moderate listings.
type AdminJobListing struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	Slug           string            `json:"slug"`
	Summary        string            `json:"summary"`
	Status         string            `json:"status"`
	IsFeatured     bool              `json:"is_featured"`
	OrganizationID string            `json:"organization_id,omitempty"`
	AuthorID       string            `json:"author_id,omitempty"`
	FieldValues    map[string]string `json:"field_values"`
	PublishedAt    string            `json:"published_at"`
	CreatedAt      string            `json:"created_at"`
}

// ApplicationResult is a single application record in API responses.
type ApplicationResult struct {
	ID              string  `json:"id"`
	JobID           string  `json:"job_id"`
	JobTitle        string  `json:"job_title,omitempty"`
	JobSlug         string  `json:"job_slug,omitempty"`
	ApplicantID     string  `json:"applicant_id,omitempty"`
	ApplicantUsername string `json:"applicant_username,omitempty"`
	ApplicantEmail  string  `json:"applicant_email,omitempty"`
	Status          string  `json:"status"`
	EmployerNotes   *string `json:"employer_notes,omitempty"`
	AppliedAt       string  `json:"applied_at"`
	StatusUpdatedAt string  `json:"status_updated_at,omitempty"`
}

// Service holds the business logic for the Jobs Board.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
}

// NewService creates a Jobs service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetEnqueuer injects the Asynq task enqueuer for notification tasks.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// IsEnabled reads the jobs.enabled system_setting (default true).
func (s *Service) IsEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "jobs.enabled", true)
}

// FeaturedCredits reads the jobs.featured_credits system_setting (default 1000).
func (s *Service) FeaturedCredits(ctx context.Context, tenantID pgtype.UUID) int {
	if s.settings == nil {
		return 1000
	}
	return s.settings.GetIntOr(ctx, tenantID, "jobs.featured_credits", 1000)
}

// FindJobs queries published job_listing contents with server-side filtering
// on custom_field_values. It uses a direct pgx query against the database pool
// because the dynamic WHERE clauses on custom_field_values cannot be expressed
// as static sqlc queries.
//
// Ref: japaconnect-engineering/sections/63-jobs-board.md
func (s *Service) FindJobs(ctx context.Context, tenantID pgtype.UUID, filters JobFilters) ([]JobResult, int64, error) {
	if filters.Limit <= 0 {
		filters.Limit = 20
	}
	if filters.Page <= 0 {
		filters.Page = 1
	}
	offset := (filters.Page - 1) * filters.Limit

	var (
		clauses []string
		args    []any
		argIdx  = 1
	)

	clauses = append(clauses, "c.tenant_id = $1", "c.content_type = 'job_listing'", "c.status = 'published'")
	args = append(args, tenantID)
	argIdx = 2

	if filters.Q != "" {
		clauses = append(clauses, fmt.Sprintf("c.title ILIKE $%d", argIdx))
		args = append(args, "%"+filters.Q+"%")
		argIdx++
	}

	// custom_field_values filters are applied via EXISTS subqueries so each
	// filter is independent.
	if filters.Country != "" {
		clauses = append(clauses, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				WHERE cfv.content_id = c.id
				  AND cfv.field_key = 'country'
				  AND cfv.value_text = $%d
			)`, argIdx))
		args = append(args, filters.Country)
		argIdx++
	}

	if filters.JobType != "" {
		clauses = append(clauses, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				WHERE cfv.content_id = c.id
				  AND cfv.field_key = 'job_type'
				  AND cfv.value_text = $%d
			)`, argIdx))
		args = append(args, filters.JobType)
		argIdx++
	}

	if filters.LocationType != "" {
		clauses = append(clauses, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				WHERE cfv.content_id = c.id
				  AND cfv.field_key = 'location_type'
				  AND cfv.value_text = $%d
			)`, argIdx))
		args = append(args, filters.LocationType)
		argIdx++
	}

	if filters.VisaSponsorship != nil {
		clauses = append(clauses, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				WHERE cfv.content_id = c.id
				  AND cfv.field_key = 'visa_sponsorship'
				  AND cfv.value_bool = $%d
			)`, argIdx))
		args = append(args, *filters.VisaSponsorship)
		argIdx++
	}

	// Sort order — default to newest.
	orderClause := "c.published_at DESC NULLS LAST"
	switch filters.Sort {
	case "deadline_asc":
		// Join to the application_deadline custom field value for sorting.
		orderClause = "deadline_val ASC NULLS LAST"
	case "salary_desc":
		// Join to the salary_max custom field value for sorting.
		orderClause = "salary_val DESC NULLS LAST"
	case "newest":
		orderClause = "c.published_at DESC NULLS LAST"
	default:
		orderClause = "c.published_at DESC NULLS LAST"
	}

	// Optional LEFT JOINs for sort columns.
	deadlineJoin := ""
	if filters.Sort == "deadline_asc" {
		deadlineJoin = fmt.Sprintf(`
			LEFT JOIN LATERAL (
				SELECT cfv.value_text AS deadline_val
				FROM custom_field_values cfv
				WHERE cfv.content_id = c.id AND cfv.field_key = 'application_deadline'
				LIMIT 1
			) dl ON true`)
	}
	salaryJoin := ""
	if filters.Sort == "salary_desc" {
		salaryJoin = fmt.Sprintf(`
			LEFT JOIN LATERAL (
				SELECT cfv.value_number AS salary_val
				FROM custom_field_values cfv
				WHERE cfv.content_id = c.id AND cfv.field_key = 'salary_max'
				LIMIT 1
			) sl ON true`)
	}

	whereClause := strings.Join(clauses, " AND ")

	// Count query (no limit/offset).
	countSQL := fmt.Sprintf(`
		SELECT COUNT(*) FROM contents c
		WHERE %s`, whereClause)

	// Data query with field value extraction via JSON aggregation.
	dataSQL := fmt.Sprintf(`
		SELECT
			c.id, c.title, c.slug,
			COALESCE(
				(SELECT json_object_agg(cfv.field_key,
					CASE
						WHEN cfv.value_text IS NOT NULL THEN cfv.value_text
						WHEN cfv.value_number IS NOT NULL THEN cfv.value_number::text
						WHEN cfv.value_bool IS NOT NULL THEN cfv.value_bool::text
						ELSE ''
					END
				) FROM custom_field_values cfv WHERE cfv.content_id = c.id),
				'{}'::json
			) AS field_values,
			c.published_at
		FROM contents c
		%s
		%s
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		deadlineJoin, salaryJoin, whereClause, orderClause, argIdx, argIdx+1)

	args = append(args, filters.Limit, offset)

	pool := s.store.pool.Reader

	// Execute count query.
	var total int64
	if err := pool.QueryRow(ctx, countSQL, args[:len(args)-2]...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("jobs: find: count: %w", err)
	}

	if total == 0 {
		return []JobResult{}, 0, nil
	}

	// Execute data query.
	rows, err := pool.Query(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("jobs: find: query: %w", err)
	}
	defer rows.Close()

	results := make([]JobResult, 0)
	for rows.Next() {
		var (
			id          pgtype.UUID
			title       string
			slug        string
			fieldValues []byte
			publishedAt pgtype.Timestamptz
		)
		if err := rows.Scan(&id, &title, &slug, &fieldValues, &publishedAt); err != nil {
			return nil, 0, fmt.Errorf("jobs: find: scan: %w", err)
		}

		fvMap := make(map[string]string)
		if len(fieldValues) > 0 {
			_ = json.Unmarshal(fieldValues, &fvMap)
		}

		publishedStr := ""
		if publishedAt.Valid {
			publishedStr = publishedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
		}

		results = append(results, JobResult{
			ID:          id.String(),
			Title:       title,
			Slug:        slug,
			Summary:     "",
			FieldValues: fvMap,
			PublishedAt: publishedStr,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("jobs: find: rows: %w", err)
	}

	return results, total, nil
}

// GetJobDetail returns a single job listing with all its custom field values.
func (s *Service) GetJobDetail(ctx context.Context, tenantID pgtype.UUID, slug string) (JobDetail, error) {
	query := `
		SELECT
			c.id, c.title, c.slug, c.body,
			COALESCE(
				(SELECT json_object_agg(cfv.field_key,
					CASE
						WHEN cfv.value_text IS NOT NULL THEN cfv.value_text
						WHEN cfv.value_number IS NOT NULL THEN cfv.value_number::text
						WHEN cfv.value_bool IS NOT NULL THEN cfv.value_bool::text
						ELSE ''
					END
				) FROM custom_field_values cfv WHERE cfv.content_id = c.id),
				'{}'::json
			) AS field_values,
			c.published_at
		FROM contents c
		WHERE c.tenant_id = $1
		  AND c.content_type = 'job_listing'
		  AND c.status = 'published'
		  AND c.slug = $2
		LIMIT 1`

	var (
		id          pgtype.UUID
		title       string
		slugOut     string
		body        *string
		fieldValues []byte
		publishedAt pgtype.Timestamptz
	)

	err := s.store.pool.Reader.QueryRow(ctx, query, tenantID, slug).Scan(
		&id, &title, &slugOut, &body, &fieldValues, &publishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JobDetail{}, fmt.Errorf("jobs: job not found")
		}
		return JobDetail{}, fmt.Errorf("jobs: get job detail: %w", err)
	}

	fvMap := make(map[string]string)
	if len(fieldValues) > 0 {
		_ = json.Unmarshal(fieldValues, &fvMap)
	}

	bodyStr := ""
	if body != nil {
		bodyStr = *body
	}

	publishedStr := ""
	if publishedAt.Valid {
		publishedStr = publishedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}

	return JobDetail{
		ID:          id.String(),
		Title:       title,
		Slug:        slugOut,
		Summary:     "",
		Body:        bodyStr,
		FieldValues: fvMap,
		PublishedAt: publishedStr,
	}, nil
}

// ListOrgJobs returns published job_listing contents for a specific org,
// with pagination.
func (s *Service) ListOrgJobs(ctx context.Context, tenantID, orgID pgtype.UUID, page, limit int) ([]JobResult, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	countSQL := `
		SELECT COUNT(*) FROM contents c
		WHERE c.tenant_id = $1 AND c.organization_id = $2
		  AND c.content_type = 'job_listing' AND c.status = 'published'`

	dataSQL := `
		SELECT
			c.id, c.title, c.slug,
			COALESCE(
				(SELECT json_object_agg(cfv.field_key,
					CASE
						WHEN cfv.value_text IS NOT NULL THEN cfv.value_text
						WHEN cfv.value_number IS NOT NULL THEN cfv.value_number::text
						WHEN cfv.value_bool IS NOT NULL THEN cfv.value_bool::text
						ELSE ''
					END
				) FROM custom_field_values cfv WHERE cfv.content_id = c.id),
				'{}'::json
			) AS field_values,
			c.published_at
		FROM contents c
		WHERE c.tenant_id = $1 AND c.organization_id = $2
		  AND c.content_type = 'job_listing' AND c.status = 'published'
		ORDER BY c.published_at DESC NULLS LAST
		LIMIT $3 OFFSET $4`

	pool := s.store.pool.Reader

	var total int64
	if err := pool.QueryRow(ctx, countSQL, tenantID, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("jobs: list org jobs: count: %w", err)
	}

	if total == 0 {
		return []JobResult{}, 0, nil
	}

	rows, err := pool.Query(ctx, dataSQL, tenantID, orgID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("jobs: list org jobs: query: %w", err)
	}
	defer rows.Close()

	results := make([]JobResult, 0)
	for rows.Next() {
		var (
			id          pgtype.UUID
			title       string
			slug        string
			fieldValues []byte
			publishedAt pgtype.Timestamptz
		)
		if err := rows.Scan(&id, &title, &slug, &fieldValues, &publishedAt); err != nil {
			return nil, 0, fmt.Errorf("jobs: list org jobs: scan: %w", err)
		}

		fvMap := make(map[string]string)
		if len(fieldValues) > 0 {
			_ = json.Unmarshal(fieldValues, &fvMap)
		}

		publishedStr := ""
		if publishedAt.Valid {
			publishedStr = publishedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
		}

		results = append(results, JobResult{
			ID:          id.String(),
			Title:       title,
			Slug:        slug,
			Summary:     "",
			FieldValues: fvMap,
			PublishedAt: publishedStr,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("jobs: list org jobs: rows: %w", err)
	}

	return results, total, nil
}

// CreateJobListing creates a job_listing content row with the given field
// values and returns the created listing detail.
func (s *Service) CreateJobListing(ctx context.Context, tenantID, orgID, authorID pgtype.UUID, title, slug, summary, body string, fieldValues map[string]string) (JobDetail, error) {
	if slug == "" {
		slug = generateJobSlug(title)
	}

	// Insert the content row.
	var contentID pgtype.UUID
	err := s.store.pool.Writer.QueryRow(ctx, `
		INSERT INTO contents (tenant_id, organization_id, author_id, content_type, title, slug, body, status, published_at)
		VALUES ($1, $2, $3, 'job_listing', $4, $5, NULLIF($6, ''), 'published', now())
		RETURNING id`,
		tenantID, orgID, authorID, title, slug, body).Scan(&contentID)
	if err != nil {
		return JobDetail{}, fmt.Errorf("jobs: create listing: insert content: %w", err)
	}

	// Insert custom field values.
	for fieldKey, value := range fieldValues {
		if value == "" {
			continue
		}
		if _, err := s.store.pool.Writer.Exec(ctx, `
			INSERT INTO custom_field_values (content_id, field_key, value_text)
			VALUES ($1, $2, $3)
			ON CONFLICT (content_id, field_key) DO UPDATE SET value_text = $3`,
			contentID, fieldKey, value); err != nil {
			slog.Warn("jobs: create listing: field value insert failed", "field_key", fieldKey, "error", err)
		}
	}

	return JobDetail{
		ID:          contentID.String(),
		Title:       title,
		Slug:        slug,
		Summary:     summary,
		Body:        body,
		FieldValues: fieldValues,
		PublishedAt: "",
	}, nil
}

// generateJobSlug creates a URL-safe slug from a title.
func generateJobSlug(title string) string {
	slug := strings.ToLower(strings.TrimSpace(title))
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "/", "-")
	// Remove non-alphanumeric characters except hyphens.
	var b strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if result == "" {
		return "job-listing"
	}
	return result
}

// ApplyToJob creates a job_applications record. Checks for duplicate — a user
// can only apply once per job. submissionID is optional (nil if applying with
// form data submitted separately).
func (s *Service) ApplyToJob(ctx context.Context, tenantID, jobID, applicantID pgtype.UUID, submissionID *uuid.UUID) (gen.JobApplication, error) {
	var subID pgtype.UUID
	if submissionID != nil {
		var b [16]byte = *submissionID
		subID = pgtype.UUID{Bytes: b, Valid: true}
	}

	app, err := s.store.CreateJobApplication(ctx, tenantID, jobID, applicantID, subID)
	if err != nil {
		// ON CONFLICT DO NOTHING with sqlc :one returns pgx.ErrNoRows when
		// the conflict triggers (no row returned). Detect that and re-query
		// to return the existing application with ErrDuplicateApplication.
		if errors.Is(err, pgx.ErrNoRows) {
			existing, qerr := s.findExistingApplication(ctx, tenantID, jobID, applicantID)
			if qerr != nil {
				return gen.JobApplication{}, ErrDuplicateApplication
			}
			return existing, ErrDuplicateApplication
		}
		return gen.JobApplication{}, err
	}
	return app, nil
}

// findExistingApplication looks up an existing application for the user+job.
func (s *Service) findExistingApplication(ctx context.Context, tenantID, jobID, applicantID pgtype.UUID) (gen.JobApplication, error) {
	query := `
		SELECT id, tenant_id, job_id, applicant_id, submission_id, status, employer_notes, applied_at, status_updated_at
		FROM job_applications
		WHERE tenant_id = $1 AND job_id = $2 AND applicant_id = $3
		LIMIT 1`
	var app gen.JobApplication
	err := s.store.pool.Reader.QueryRow(ctx, query, tenantID, jobID, applicantID).Scan(
		&app.ID, &app.TenantID, &app.JobID, &app.ApplicantID, &app.SubmissionID,
		&app.Status, &app.EmployerNotes, &app.AppliedAt, &app.StatusUpdatedAt,
	)
	if err != nil {
		return gen.JobApplication{}, err
	}
	return app, nil
}

// UpdateApplicationStatus updates the status of an application and enqueues a
// jobs:notify_application_status task fire-and-forget (Rule 8).
func (s *Service) UpdateApplicationStatus(ctx context.Context, tenantID, appID pgtype.UUID, status string, employerNotes *string) (gen.JobApplication, error) {
	if !IsValidStatus(status) {
		return gen.JobApplication{}, fmt.Errorf("jobs: invalid status %q", status)
	}

	app, err := s.store.UpdateJobApplicationStatus(ctx, tenantID, appID, status, employerNotes)
	if err != nil {
		return gen.JobApplication{}, fmt.Errorf("jobs: update application status: %w", err)
	}

	// Enqueue notification task fire-and-forget (Rule 8).
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"application_id": app.ID.String(),
			"applicant_id":   app.ApplicantID.String(),
			"job_id":         app.JobID.String(),
			"tenant_id":      tenantID.String(),
			"status":         status,
		})
		if err != nil {
			slog.Warn("jobs: marshal notify payload failed", "error", err)
		} else if err := s.enqueuer.Enqueue(TaskNotifyApplicationStatus, payload); err != nil {
			slog.Warn("jobs: enqueue notify task failed", "error", err)
		}
	}

	return app, nil
}

// GetApplicationForNotification fetches a job application by ID for the
// notification adapter. Read-only — does not mutate application state.
// Used by the jobs:notify_application_status Asynq task handler (§63,
// ADR-081).
func (s *Service) GetApplicationForNotification(ctx context.Context, tenantID, applicationID pgtype.UUID) (gen.JobApplication, error) {
	return s.store.GetJobApplicationByID(ctx, tenantID, applicationID)
}

// ListUserApplications returns an applicant's application history with
// pagination.
func (s *Service) ListUserApplications(ctx context.Context, tenantID, userID pgtype.UUID, page, limit int) ([]ApplicationResult, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	total, err := s.store.CountUserJobApplications(ctx, tenantID, userID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.store.ListUserJobApplications(ctx, tenantID, userID, offset, int32(limit))
	if err != nil {
		return nil, 0, err
	}

	results := make([]ApplicationResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, applicationResultFromUserRow(row))
	}
	return results, total, nil
}

// ListJobApplications returns applications for a specific job (employer view)
// with pagination and optional status filter.
func (s *Service) ListJobApplications(ctx context.Context, tenantID, jobID pgtype.UUID, status string, page, limit int) ([]ApplicationResult, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	total, err := s.store.CountJobApplicationsByJob(ctx, tenantID, jobID, status)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.store.ListJobApplicationsByJob(ctx, tenantID, jobID, status, offset, int32(limit))
	if err != nil {
		return nil, 0, err
	}

	results := make([]ApplicationResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, applicationResultFromJobRow(row))
	}
	return results, total, nil
}

// AdminListApplications returns all applications across all jobs for a tenant,
// with pagination and optional status filter.
func (s *Service) AdminListApplications(ctx context.Context, tenantID pgtype.UUID, status string, page, limit int) ([]ApplicationResult, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	total, err := s.store.AdminCountJobApplications(ctx, tenantID, status)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.store.AdminListJobApplications(ctx, tenantID, status, offset, int32(limit))
	if err != nil {
		return nil, 0, err
	}

	results := make([]ApplicationResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, applicationResultFromAdminRow(row))
	}
	return results, total, nil
}

// AdminListJobs returns all job_listing contents for a tenant, with
// pagination and optional status filter. This is the admin "all listings"
// view per Section 63: GET /admin/jobs — all listings (admin).
func (s *Service) AdminListJobs(ctx context.Context, tenantID pgtype.UUID, status string, page, limit int) ([]AdminJobListing, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	// Build the WHERE clause — always tenant + content_type, optionally status.
	whereClause := "c.tenant_id = $1 AND c.content_type = 'job_listing'"
	args := []any{tenantID}
	argIdx := 2
	if status != "" {
		whereClause += fmt.Sprintf(" AND c.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	// Count query.
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM contents c WHERE %s`, whereClause)
	var total int64
	if err := s.store.pool.Reader.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("jobs: admin list jobs: count: %w", err)
	}
	if total == 0 {
		return []AdminJobListing{}, 0, nil
	}

	// Data query — extract field values via JSON aggregation, joining
	// custom_field_values → custom_fields to resolve the field key. The
	// is_featured flag is read from the same join.
	dataSQL := fmt.Sprintf(`
		SELECT
			c.id, c.title, c.slug, c.status,
			COALESCE(
				(SELECT cfv.value_bool
				 FROM custom_field_values cfv
				 JOIN custom_fields cf ON cf.id = cfv.field_id
				 WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				   AND cf.key = 'is_featured'
				 LIMIT 1),
				false
			) AS is_featured,
			c.organization_id, c.author_id,
			COALESCE(
				(SELECT json_object_agg(cf.key,
					CASE
						WHEN cfv.value_text IS NOT NULL THEN cfv.value_text
						WHEN cfv.value_number IS NOT NULL THEN cfv.value_number::text
						WHEN cfv.value_bool IS NOT NULL THEN cfv.value_bool::text
						ELSE ''
					END
				) FROM custom_field_values cfv
				  JOIN custom_fields cf ON cf.id = cfv.field_id
				  WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id),
				'{}'::json
			) AS field_values,
			c.published_at, c.created_at
		FROM contents c
		WHERE %s
		ORDER BY c.created_at DESC
		LIMIT $%d OFFSET $%d`,
		whereClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.store.pool.Reader.Query(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("jobs: admin list jobs: query: %w", err)
	}
	defer rows.Close()

	results := make([]AdminJobListing, 0)
	for rows.Next() {
		var (
			id            pgtype.UUID
			title         string
			slug          string
			contentStatus string
			isFeatured    bool
			orgID         pgtype.UUID
			authorID      pgtype.UUID
			fieldValues   []byte
			publishedAt   pgtype.Timestamptz
			createdAt     pgtype.Timestamptz
		)
		if err := rows.Scan(&id, &title, &slug, &contentStatus,
			&isFeatured, &orgID, &authorID, &fieldValues,
			&publishedAt, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("jobs: admin list jobs: scan: %w", err)
		}

		fvMap := make(map[string]string)
		if len(fieldValues) > 0 {
			_ = json.Unmarshal(fieldValues, &fvMap)
		}

		publishedStr := ""
		if publishedAt.Valid {
			publishedStr = publishedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		createdStr := ""
		if createdAt.Valid {
			createdStr = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
		}

		listing := AdminJobListing{
			ID:          id.String(),
			Title:       title,
			Slug:        slug,
			Summary:     "",
			Status:      contentStatus,
			IsFeatured:  isFeatured,
			FieldValues: fvMap,
			PublishedAt: publishedStr,
			CreatedAt:   createdStr,
		}
		if orgID.Valid {
			listing.OrganizationID = orgID.String()
		}
		if authorID.Valid {
			listing.AuthorID = authorID.String()
		}
		results = append(results, listing)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("jobs: admin list jobs: rows: %w", err)
	}

	return results, total, nil
}

// AdminJobStatusDistribution returns the count of job_listing contents grouped
// by content status (published, archived, draft) for a tenant.
func (s *Service) AdminJobStatusDistribution(ctx context.Context, tenantID pgtype.UUID) (map[string]int64, error) {
	query := `
		SELECT status, COUNT(*) AS count
		FROM contents
		WHERE tenant_id = $1 AND content_type = 'job_listing'
		GROUP BY status
		ORDER BY count DESC`
	rows, err := s.store.pool.Reader.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("jobs: admin job status distribution: %w", err)
	}
	defer rows.Close()

	dist := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("jobs: admin job status distribution: scan: %w", err)
		}
		dist[status] = count
	}
	return dist, rows.Err()
}

// AdminApplicationStatusDistribution returns the count of applications grouped
// by status for a tenant.
func (s *Service) AdminApplicationStatusDistribution(ctx context.Context, tenantID pgtype.UUID) (map[string]int64, error) {
	rows, err := s.store.AdminApplicationStatusDistribution(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	dist := make(map[string]int64, len(rows))
	for _, row := range rows {
		dist[row.Status] = row.Count
	}
	return dist, nil
}

// ExpireListingsPastDeadline archives published job_listing content whose
// application_deadline is in the past. Called by the jobs:expire_listings task.
func (s *Service) ExpireListingsPastDeadline(ctx context.Context, tenantID pgtype.UUID) (int64, error) {
	return s.store.ExpireListingsPastDeadline(ctx, tenantID)
}

// ToggleFeature toggles the is_featured custom field value for a job listing.
// Used by the admin feature route. Tenant-scoped and content-type-checked for
// security (prevents cross-tenant toggle and toggling non-job content).
func (s *Service) ToggleFeature(ctx context.Context, tenantID, jobID pgtype.UUID) (bool, error) {
	// Read the current value — verify tenant + content_type = job_listing.
	// Join custom_field_values → custom_fields to resolve the field key.
	var current bool
	err := s.store.pool.Reader.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT cfv.value_bool
			 FROM custom_field_values cfv
			 JOIN custom_fields cf ON cf.id = cfv.field_id
			 JOIN contents c ON c.id = cfv.entity_id
			 WHERE cfv.entity_type = 'content' AND cfv.entity_id = $1
			   AND c.tenant_id = $2
			   AND c.content_type = 'job_listing'
			   AND cf.key = 'is_featured'
			 LIMIT 1),
			false
		)`, jobID, tenantID).Scan(&current)
	if err != nil {
		return false, fmt.Errorf("jobs: toggle feature: read: %w", err)
	}

	newVal := !current

	// Resolve the custom_fields.id for 'is_featured' in this tenant, then
	// upsert the custom_field_values row.
	var fieldID pgtype.UUID
	err = s.store.pool.Writer.QueryRow(ctx, `
		SELECT id FROM custom_fields
		WHERE tenant_id = $2 AND key = 'is_featured'
		LIMIT 1`, jobID, tenantID).Scan(&fieldID)
	if err != nil {
		return false, fmt.Errorf("jobs: toggle feature: resolve field id: %w", err)
	}

	_, err = s.store.pool.Writer.Exec(ctx, `
		INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_bool)
		SELECT $1, 'content', $2, $3
		WHERE EXISTS (
			SELECT 1 FROM contents c
			WHERE c.id = $2 AND c.tenant_id = $4 AND c.content_type = 'job_listing'
		)
		ON CONFLICT (field_id, entity_type, entity_id)
		DO UPDATE SET value_bool = $3`,
		fieldID, jobID, newVal, tenantID)
	if err != nil {
		return false, fmt.Errorf("jobs: toggle feature: upsert: %w", err)
	}

	return newVal, nil
}

// AdminUpdateJob updates a job listing's core fields (title, summary, body,
// status). The jobID must be a content row with content_type='job_listing'
// in the given tenant. Field values (custom fields) can be updated via the
// fieldValues map.
func (s *Service) AdminUpdateJob(ctx context.Context, tenantID, jobID pgtype.UUID, title, summary, body, status *string, fieldValues map[string]string) error {
	// Build the SET clause dynamically based on which fields are provided.
	setParts := []string{"updated_at = now()"}
	args := []interface{}{tenantID, jobID}
	argIdx := 3
	if title != nil {
		setParts = append(setParts, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *title)
		argIdx++
	}
	if summary != nil {
		setParts = append(setParts, fmt.Sprintf("summary = $%d", argIdx))
		args = append(args, *summary)
		argIdx++
	}
	if body != nil {
		setParts = append(setParts, fmt.Sprintf("body = $%d", argIdx))
		args = append(args, *body)
		argIdx++
	}
	if status != nil {
		setParts = append(setParts, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *status)
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE contents SET %s
		WHERE id = $2 AND tenant_id = $1 AND content_type = 'job_listing'`,
		strings.Join(setParts, ", "))

	_, err := s.store.pool.Writer.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("jobs: admin update: update content: %w", err)
	}

	// Update custom field values — resolve field_id from custom_fields by key.
	for fieldKey, value := range fieldValues {
		if _, err := s.store.pool.Writer.Exec(ctx, `
			INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_text)
			SELECT cf.id, 'content', $1, $3
			FROM custom_fields cf
			WHERE cf.tenant_id = $4 AND cf.key = $2
			  AND EXISTS (
				SELECT 1 FROM contents c
				WHERE c.id = $1 AND c.tenant_id = $4 AND c.content_type = 'job_listing'
			  )
			ON CONFLICT (field_id, entity_type, entity_id)
			DO UPDATE SET value_text = $3`,
			jobID, fieldKey, value, tenantID); err != nil {
			slog.Warn("jobs: admin update: field value upsert failed", "field_key", fieldKey, "error", err)
		}
	}

	return nil
}

// AdminDeleteJob soft-deletes a job listing by setting deleted_at = now().
// The jobID must be a content row with content_type='job_listing' in the
// given tenant.
func (s *Service) AdminDeleteJob(ctx context.Context, tenantID, jobID pgtype.UUID) error {
	_, err := s.store.pool.Writer.Exec(ctx, `
		UPDATE contents SET deleted_at = now(), updated_at = now()
		WHERE id = $2 AND tenant_id = $1 AND content_type = 'job_listing'`,
		tenantID, jobID)
	if err != nil {
		return fmt.Errorf("jobs: admin delete: %w", err)
	}
	return nil
}

// applicationResultFromUserRow converts a ListUserJobApplicationsRow to an
// ApplicationResult.
func applicationResultFromUserRow(row gen.ListUserJobApplicationsRow) ApplicationResult {
	r := ApplicationResult{
		ID:        row.ID.String(),
		JobID:     row.JobID.String(),
		JobTitle:  row.JobTitle,
		JobSlug:   row.JobSlug,
		Status:    row.Status,
		EmployerNotes: row.EmployerNotes,
	}
	if row.AppliedAt.Valid {
		r.AppliedAt = row.AppliedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	if row.StatusUpdatedAt.Valid {
		r.StatusUpdatedAt = row.StatusUpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return r
}

// applicationResultFromJobRow converts a ListJobApplicationsByJobRow to an
// ApplicationResult.
func applicationResultFromJobRow(row gen.ListJobApplicationsByJobRow) ApplicationResult {
	r := ApplicationResult{
		ID:               row.ID.String(),
		JobID:            row.JobID.String(),
		ApplicantID:      row.ApplicantID.String(),
		ApplicantUsername: row.ApplicantUsername,
		ApplicantEmail:   row.ApplicantEmail,
		Status:           row.Status,
		EmployerNotes:    row.EmployerNotes,
	}
	if row.AppliedAt.Valid {
		r.AppliedAt = row.AppliedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	if row.StatusUpdatedAt.Valid {
		r.StatusUpdatedAt = row.StatusUpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return r
}

// applicationResultFromAdminRow converts an AdminListJobApplicationsRow to an
// ApplicationResult.
func applicationResultFromAdminRow(row gen.AdminListJobApplicationsRow) ApplicationResult {
	r := ApplicationResult{
		ID:               row.ID.String(),
		JobID:            row.JobID.String(),
		ApplicantID:      row.ApplicantID.String(),
		ApplicantUsername: row.ApplicantUsername,
		JobTitle:         row.JobTitle,
		Status:           row.Status,
		EmployerNotes:    row.EmployerNotes,
	}
	if row.AppliedAt.Valid {
		r.AppliedAt = row.AppliedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	if row.StatusUpdatedAt.Valid {
		r.StatusUpdatedAt = row.StatusUpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return r
}
