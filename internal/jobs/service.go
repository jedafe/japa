// Package jobs — service.go holds the business logic for the Jobs Board.
// It reads jobs.* system_settings via the hot-reloadable config.Settings and
// enqueues Asynq tasks fire-and-forget (Rule 8) for application status
// notifications.
//
// Ref: japaconnect-engineering/sections/63-jobs-board.md
//
//	japaconnect-engineering/adr/ADR-081-jobs-board.md
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
)

// Application status constants — match the CHECK constraint on
// job_applications.status.
const (
	StatusApplied     = "applied"
	StatusReviewing   = "reviewing"
	StatusShortlisted = "shortlisted"
	StatusRejected    = "rejected"
	StatusOffered     = "offered"
	StatusWithdrawn   = "withdrawn"
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

// ErrNotOrgMember is returned when a caller is not a member of the target
// organization with a job-editor-or-above role.
var ErrNotOrgMember = errors.New("jobs: not an organization member")

// ErrDuplicateContent is returned when a job listing's content_hash collides
// with an existing content row (Rule 12 → 409 duplicate_content).
var ErrDuplicateContent = errors.New("jobs: duplicate content")

// ErrInvalidFieldValue is returned when a field_values entry fails type
// validation — e.g. a url-typed field carrying a non-http(s) value. Handlers
// map it to 400 invalid_field_value (Rule 5).
var ErrInvalidFieldValue = errors.New("jobs: invalid field value")

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

// SourceAttribution is a lightweight source snapshot attached to job listings
// so the UI can link back to /sources/[slug].
type SourceAttribution struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	LogoURL    string `json:"logo_url"`
	SourceURL  string `json:"source_url"`
	TrustLevel int16  `json:"trust_level"`
}

// JobPoster is the entity that published the listing — the organization (or,
// failing that, the author user). Finder cards use it for the avatar/name
// fallback and the hover-card link target when no company_logo/company_name
// field overrides it.
type JobPoster struct {
	Kind       string `json:"kind"` // "organization" | "user"
	ID         string `json:"id"`
	Slug       string `json:"slug"` // org slug or username
	Name       string `json:"name"` // org name or display name
	LogoURL    string `json:"logo_url,omitempty"`
	Verified   bool   `json:"verified,omitempty"`
	WebsiteURL string `json:"website_url,omitempty"`
}

// JobResult is a single job listing in the finder results.
// Source is a pointer so listings without intel provenance serialize
// `"source": null` instead of an empty attribution object (JOBS-016).
type JobResult struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Slug        string             `json:"slug"`
	Summary     string             `json:"summary"`
	FieldValues map[string]string  `json:"field_values"`
	PublishedAt string             `json:"published_at"`
	Source      *SourceAttribution `json:"source"`
	Poster      *JobPoster         `json:"poster"`

	intelRecordID pgtype.UUID
}

// JobDetail is a single job listing with all its custom field values.
// Source is a pointer so listings without intel provenance serialize
// `"source": null` instead of an empty attribution object (JOBS-016).
type JobDetail struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	Slug           string             `json:"slug"`
	Summary        string             `json:"summary"`
	Body           string             `json:"body"`
	FieldValues    map[string]string  `json:"field_values"`
	PublishedAt    string             `json:"published_at"`
	OrganizationID string             `json:"organization_id,omitempty"`
	Source         *SourceAttribution `json:"source"`
	Poster         *JobPoster         `json:"poster"`
	// MyApplication is the authenticated viewer's own application to this
	// listing, or null when the viewer is a guest or has not applied. Lets
	// the detail page render an "Applied" state instead of the form.
	MyApplication *ViewerApplication `json:"my_application"`
}

// ViewerApplication is the caller's own job_applications row on a detail
// response — id, status, and applied_at only.
type ViewerApplication struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	AppliedAt string `json:"applied_at"`
}

// UpdateJobListingInput is the request body for the public PATCH
// /org/:org_id/jobs/:id endpoint.
type UpdateJobListingInput struct {
	Title       string
	Slug        string
	Body        string
	FieldValues map[string]string
	PublishedAt *string
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
	ID                string  `json:"id"`
	JobID             string  `json:"job_id"`
	JobTitle          string  `json:"job_title,omitempty"`
	JobSlug           string  `json:"job_slug,omitempty"`
	ApplicantID       string  `json:"applicant_id,omitempty"`
	ApplicantUsername string  `json:"applicant_username,omitempty"`
	ApplicantEmail    string  `json:"applicant_email,omitempty"`
	Status            string  `json:"status"`
	EmployerNotes     *string `json:"employer_notes,omitempty"`
	AppliedAt         string  `json:"applied_at"`
	StatusUpdatedAt   string  `json:"status_updated_at,omitempty"`
	// Submission carries the linked form_submissions.field_values bag
	// (e.g. resume_url, cover_letter) for the employer view; null when the
	// application has no linked submission.
	Submission map[string]any `json:"submission,omitempty"`
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
func (s *Service) IsEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "jobs.enabled", true)
}

// FeaturedCredits reads the jobs.featured_credits system_setting (default 1000).
func (s *Service) FeaturedCredits(ctx context.Context) int {
	if s.settings == nil {
		return 1000
	}

	return s.settings.GetIntOr(ctx, "jobs.featured_credits", 1000)
}

// IsOrgMember returns true if the user is a member of the org with one of the
// allowed roles. Used to authorize org-scoped job routes (JOBS-012): the
// router-level jobs.post permission is granted to the default member role, so
// without this check any member could manage any org's jobs.
func (s *Service) IsOrgMember(ctx context.Context, userID, orgID pgtype.UUID, allowedRoles []string) (bool, error) {
	if len(allowedRoles) == 0 {
		return false, nil
	}
	// Build the ANY array literal safely from the allowed roles.
	args := []any{userID, orgID}
	query := `
		SELECT EXISTS (
			SELECT 1 FROM organization_members
			WHERE user_id = $1 AND organization_id = $2
			  AND role = ANY($3)
		)`

	args = append(args, allowedRoles)

	var exists bool

	if err := s.store.pool.Reader.QueryRow(ctx, query, args...).Scan(&exists); err != nil {
		return false, fmt.Errorf("jobs: is org member: %w", err)
	}

	return exists, nil
}

// OrgRoles allowed for each org-scoped job action.
var (
	// OrgJobEditorRoles — create/list jobs for an org (owner/admin/editor).
	OrgJobEditorRoles = []string{"owner", "admin", "editor"}
	// OrgJobAdminRoles — view/update applications (owner/admin).
	OrgJobAdminRoles = []string{"owner", "admin"}
)

// JobBelongsToOrg returns true if the job (content) is a job_listing in the
// platform owned by orgID. Used to authorize application routes so a caller
// cannot reach a job's applications through a different org's ID (JOBS-012).
func (s *Service) JobBelongsToOrg(ctx context.Context, jobID, orgID pgtype.UUID) (bool, error) {
	var exists bool

	err := s.store.pool.Reader.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM contents
			WHERE id = $1 AND organization_id = $2
			  AND content_type = 'job_listing' AND deleted_at IS NULL
		)`, jobID, orgID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("jobs: job belongs to org: %w", err)
	}

	return exists, nil
}

// FindJobs queries published job_listing contents with server-side filtering
// on custom_field_values. It uses a direct pgx query against the database pool
// because the dynamic WHERE clauses on custom_field_values cannot be expressed
// as static sqlc queries.
//
// Ref: japaconnect-engineering/sections/63-jobs-board.md.
func (s *Service) FindJobs(ctx context.Context, filters JobFilters) ([]JobResult, int64, error) {
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

	// Base clauses — platform + content_type + published + not soft-deleted.
	// custom_field_values is EAV (field_id → custom_fields.id, entity_type,
	// entity_id); filters join through custom_fields to resolve the key.
	clauses = append(
		clauses,
		"c.content_type = 'job_listing'",
		"c.status = 'published'",
		"c.deleted_at IS NULL",
	)

	argIdx = 1

	if filters.Q != "" {
		clauses = append(clauses, fmt.Sprintf("c.title ILIKE $%d", argIdx))
		args = append(args, "%"+filters.Q+"%")
		argIdx++
	}

	// custom_field_values filters are applied via EXISTS subqueries so each
	// filter is independent. Each joins custom_fields to resolve the key and
	// scopes by platform to avoid cross-platform field-id collision.
	if filters.Country != "" {
		clauses = append(clauses, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				  AND cf.key = 'country'
				  AND cfv.value_text = $%d
			)`, argIdx))

		args = append(args, filters.Country)
		argIdx++
	}

	if filters.JobType != "" {
		clauses = append(clauses, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				  AND cf.key = 'job_type'
				  AND cfv.value_text = $%d
			)`, argIdx))

		args = append(args, filters.JobType)
		argIdx++
	}

	if filters.LocationType != "" {
		clauses = append(clauses, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				  AND cf.key = 'location_type'
				  AND cfv.value_text = $%d
			)`, argIdx))

		args = append(args, filters.LocationType)
		argIdx++
	}

	if filters.VisaSponsorship != nil {
		clauses = append(clauses, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				  AND cf.key = 'visa_sponsorship'
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

	// Optional LEFT JOINs for sort columns. Each joins custom_fields to
	// resolve the key (EAV schema).
	deadlineJoin := ""
	if filters.Sort == "deadline_asc" {
		deadlineJoin = `
			LEFT JOIN LATERAL (
				SELECT cfv.value_text AS deadline_val
				FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				  AND cf.key = 'application_deadline'
				LIMIT 1
			) dl ON true`
	}

	salaryJoin := ""
	if filters.Sort == "salary_desc" {
		salaryJoin = `
			LEFT JOIN LATERAL (
				SELECT cfv.value_number AS salary_val
				FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				  AND cf.key = 'salary_max'
				LIMIT 1
			) sl ON true`
	}

	whereClause := strings.Join(clauses, " AND ")

	// Count query (no limit/offset).
	countSQL := "\n\t\tSELECT COUNT(*) FROM contents c\n\t\tWHERE " + whereClause

	// Data query with field value extraction via JSON aggregation. The
	// aggregation joins custom_fields to resolve the key (EAV schema).
	// Org/user/media LEFT JOINs resolve the poster block (avatar/name/link
	// fallback for finder cards) in one round-trip.
	dataSQL := fmt.Sprintf(`
		SELECT
			c.id, c.title, c.slug, c.intel_record_id, LEFT(c.body, 300),
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
				  WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				),
				'{}'::json
			) AS field_values,
			c.published_at,
			c.organization_id, o.slug, o.name, o.verified, o.website_url,
			om.storage_bucket, om.storage_path,
			c.author_id, u.username, up.display_name, up.avatar_url
		FROM contents c
		LEFT JOIN organizations o ON o.id = c.organization_id
		LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
		LEFT JOIN users u ON u.id = c.author_id
		LEFT JOIN user_profiles up ON up.user_id = c.author_id
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
			id            pgtype.UUID
			title         string
			slug          string
			intelRecordID pgtype.UUID
			body          *string
			fieldValues   []byte
			publishedAt   pgtype.Timestamptz
			orgID         pgtype.UUID
			orgSlug       *string
			orgName       *string
			orgVerified   *bool
			orgWebsite    *string
			logoBucket    *string
			logoPath      *string
			authorID      pgtype.UUID
			username      *string
			displayName   *string
			avatarURL     *string
		)

		if err := rows.Scan(
			&id, &title, &slug, &intelRecordID, &body, &fieldValues, &publishedAt,
			&orgID, &orgSlug, &orgName, &orgVerified, &orgWebsite,
			&logoBucket, &logoPath,
			&authorID, &username, &displayName, &avatarURL,
		); err != nil {
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

		bodyStr := ""
		if body != nil {
			bodyStr = *body
		}

		results = append(results, JobResult{
			ID:            id.String(),
			Title:         title,
			Slug:          slug,
			Summary:       deriveSummary(bodyStr),
			FieldValues:   fvMap,
			PublishedAt:   publishedStr,
			Poster:        jobPosterFromRow(orgID, authorID, orgSlug, orgName, orgVerified, orgWebsite, logoBucket, logoPath, username, displayName, avatarURL),
			intelRecordID: intelRecordID,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("jobs: find: rows: %w", err)
	}

	recordIDs := make([]pgtype.UUID, 0, len(results))
	for _, r := range results {
		if r.intelRecordID.Valid {
			recordIDs = append(recordIDs, r.intelRecordID)
		}
	}

	if len(recordIDs) > 0 {
		if sources, err := s.store.MapSourcesByRecordIDs(ctx, recordIDs); err == nil {
			for i := range results {
				if src, ok := sources[results[i].intelRecordID.String()]; ok {
					attr := sourceAttributionFromRow(src)
					results[i].Source = &attr
				}
			}
		} else {
			slog.Warn("jobs: failed to load source attribution", "error", err)
		}
	}

	return results, total, nil
}

// jobPosterFromRow builds the poster block for a listing: the owning
// organization when set, else the author user. Returns nil when neither
// resolves (e.g. fully detached intel-ingested rows).
func jobPosterFromRow(
	orgID, authorID pgtype.UUID,
	orgSlug, orgName *string, orgVerified *bool, orgWebsite *string,
	logoBucket, logoPath *string,
	username, displayName, avatarURL *string,
) *JobPoster {
	if orgID.Valid && orgSlug != nil && orgName != nil {
		p := &JobPoster{
			Kind: "organization",
			ID:   orgID.String(),
			Slug: *orgSlug,
			Name: *orgName,
		}
		if orgVerified != nil {
			p.Verified = *orgVerified
		}
		if orgWebsite != nil {
			p.WebsiteURL = *orgWebsite
		}
		if logoBucket != nil && logoPath != nil && *logoBucket != "" && *logoPath != "" {
			p.LogoURL = media.MediaURL(*logoBucket, *logoPath, media.ImageProxyOpts{})
		}
		return p
	}
	if authorID.Valid && username != nil {
		name := *username
		if displayName != nil && *displayName != "" {
			name = *displayName
		}
		p := &JobPoster{
			Kind: "user",
			ID:   authorID.String(),
			Slug: *username,
			Name: name,
		}
		if avatarURL != nil {
			p.LogoURL = *avatarURL
		}
		return p
	}
	return nil
}

// GetJobDetail returns a single job listing with all its custom field values.
func (s *Service) GetJobDetail(ctx context.Context, slug string) (JobDetail, error) {
	query := `
		SELECT
			c.id, c.title, c.slug, c.body, c.organization_id, c.intel_record_id,
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
				  WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				),
				'{}'::json
			) AS field_values,
			c.published_at,
			c.author_id,
			o.slug, o.name, o.verified, o.website_url,
			om.storage_bucket, om.storage_path,
			u.username, up.display_name, up.avatar_url
		FROM contents c
		LEFT JOIN organizations o ON o.id = c.organization_id
		LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
		LEFT JOIN users u ON u.id = c.author_id
		LEFT JOIN user_profiles up ON up.user_id = c.author_id
		WHERE c.content_type = 'job_listing'
		  AND c.status = 'published'
		  AND c.deleted_at IS NULL
		  AND c.slug = $1
		LIMIT 1`

	var (
		id            pgtype.UUID
		title         string
		slugOut       string
		body          *string
		intelRecordID pgtype.UUID
		fieldValues   []byte
		publishedAt   pgtype.Timestamptz
		orgID         pgtype.UUID
		authorID      pgtype.UUID
		orgSlug       *string
		orgName       *string
		orgVerified   *bool
		orgWebsite    *string
		logoBucket    *string
		logoPath      *string
		username      *string
		displayName   *string
		avatarURL     *string
	)

	err := s.store.pool.Reader.QueryRow(ctx, query, slug).Scan(
		&id, &title, &slugOut, &body, &orgID, &intelRecordID, &fieldValues, &publishedAt,
		&authorID,
		&orgSlug, &orgName, &orgVerified, &orgWebsite,
		&logoBucket, &logoPath,
		&username, &displayName, &avatarURL,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JobDetail{}, errors.New("jobs: job not found")
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

	detail := JobDetail{
		ID:          id.String(),
		Title:       title,
		Slug:        slugOut,
		Summary:     deriveSummary(bodyStr),
		Body:        bodyStr,
		FieldValues: fvMap,
		PublishedAt: publishedStr,
	}
	if orgID.Valid {
		detail.OrganizationID = orgID.String()
	}
	detail.Poster = jobPosterFromRow(orgID, authorID, orgSlug, orgName, orgVerified, orgWebsite, logoBucket, logoPath, username, displayName, avatarURL)

	if intelRecordID.Valid {
		if sources, err := s.store.MapSourcesByRecordIDs(ctx, []pgtype.UUID{intelRecordID}); err == nil {
			if src, ok := sources[intelRecordID.String()]; ok {
				attr := sourceAttributionFromRow(src)
				detail.Source = &attr
			}
		}
	}

	return detail, nil
}

// ListOrgJobs returns published job_listing contents for a specific org,
// with pagination.
func (s *Service) ListOrgJobs(ctx context.Context, orgID pgtype.UUID, page, limit int) ([]JobResult, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	countSQL := `
		SELECT COUNT(*) FROM contents c
		WHERE c.organization_id = $1
		  AND c.content_type = 'job_listing' AND c.status = 'published'
		  AND c.deleted_at IS NULL`

	dataSQL := `
		SELECT
			c.id, c.title, c.slug,
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
				  WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				),
				'{}'::json
			) AS field_values,
			c.published_at
		FROM contents c
		WHERE c.organization_id = $1
		  AND c.content_type = 'job_listing' AND c.status = 'published'
		  AND c.deleted_at IS NULL
		ORDER BY c.published_at DESC NULLS LAST
		LIMIT $2 OFFSET $3`

	pool := s.store.pool.Reader

	var total int64
	if err := pool.QueryRow(ctx, countSQL, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("jobs: list org jobs: count: %w", err)
	}

	if total == 0 {
		return []JobResult{}, 0, nil
	}

	rows, err := pool.Query(ctx, dataSQL, orgID, limit, offset)
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
func (s *Service) CreateJobListing(ctx context.Context, orgID, authorID pgtype.UUID, title, slug, summary, body string, fieldValues map[string]string) (JobDetail, error) {
	if err := validateJobFieldValues(fieldValues); err != nil {
		return JobDetail{}, err
	}

	if slug == "" {
		slug = generateJobSlug(title)
	}

	// Rule 12: compute content_hash before every contents insert so the
	// unique index idx_contents_content_hash rejects duplicates.
	// Skip hashing for empty bodies — NULL content_hash doesn't collide
	// (the index has WHERE content_hash IS NOT NULL).
	var contentHash *string
	if strings.TrimSpace(body) != "" {
		h := content.ComputeContentHash(body)
		contentHash = &h
	}

	// body is NOT NULL DEFAULT '' — pass the value directly (empty string is
	// valid; NULLIF would turn "" into NULL and violate the NOT NULL
	// constraint, JOBS-006).
	var contentID pgtype.UUID

	err := s.store.pool.Writer.QueryRow(ctx, `
		INSERT INTO contents (organization_id, author_id, content_type, title, slug, body, content_hash, status, published_at)
		VALUES ($1, $2, 'job_listing', $3, $4, $5, $6, 'published', now())
		RETURNING id`,
		orgID, authorID, title, slug, body, contentHash).Scan(&contentID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return JobDetail{}, ErrDuplicateContent
		}
		return JobDetail{}, fmt.Errorf("jobs: create listing: insert content: %w", err)
	}

	// Insert custom field values via the typed upsert helper (EAV schema,
	// correct value_* column per field type — JOBS-002/007).
	for fieldKey, value := range fieldValues {
		s.upsertJobFieldValue(ctx, contentID, fieldKey, value)
	}

	return JobDetail{
		ID:          contentID.String(),
		Title:       title,
		Slug:        slug,
		Summary:     deriveSummary(body),
		Body:        body,
		FieldValues: fieldValues,
		PublishedAt: "",
	}, nil
}

// GetJobByID returns a single job_listing content row by id, scoped to
// platform and organization. Used by the public update flow to verify ownership
// and to re-fetch the updated row.
func (s *Service) GetJobByID(ctx context.Context, orgID, jobID pgtype.UUID) (JobDetail, error) {
	query := `
		SELECT
			c.id, c.title, c.slug, c.body,
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
				  WHERE cfv.entity_type = 'content' AND cfv.entity_id = c.id
				),
				'{}'::json
			) AS field_values,
			c.published_at
		FROM contents c
		WHERE c.organization_id = $1
		  AND c.id = $2
		  AND c.content_type = 'job_listing'
		  AND c.deleted_at IS NULL
		LIMIT 1`

	var (
		id          pgtype.UUID
		title       string
		slugOut     string
		body        *string
		fieldValues []byte
		publishedAt pgtype.Timestamptz
	)

	err := s.store.pool.Writer.QueryRow(ctx, query, orgID, jobID).Scan(
		&id, &title, &slugOut, &body, &fieldValues, &publishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JobDetail{}, pgx.ErrNoRows
		}

		return JobDetail{}, fmt.Errorf("jobs: get job by id: %w", err)
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
		ID:             id.String(),
		Title:          title,
		Slug:           slugOut,
		Summary:        deriveSummary(bodyStr),
		Body:           bodyStr,
		FieldValues:    fvMap,
		PublishedAt:    publishedStr,
		OrganizationID: orgID.String(),
	}, nil
}

// UpdateJobListing updates an org-owned job listing. The caller must be an
// editor-or-above member of the organization. Immutable fields (platform,
// organization, author, content_type, status) are not changed.
func (s *Service) UpdateJobListing(ctx context.Context, orgID, jobID, callerID pgtype.UUID, input UpdateJobListingInput) (JobDetail, error) {
	member, err := s.IsOrgMember(ctx, callerID, orgID, OrgJobEditorRoles)
	if err != nil {
		return JobDetail{}, fmt.Errorf("jobs: update listing: org check: %w", err)
	}

	if !member {
		return JobDetail{}, ErrNotOrgMember
	}

	existing, err := s.GetJobByID(ctx, orgID, jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JobDetail{}, pgx.ErrNoRows
		}

		return JobDetail{}, fmt.Errorf("jobs: update listing: %w", err)
	}

	if input.Title == "" && input.Body == "" && input.Slug == "" && input.PublishedAt == nil && len(input.FieldValues) == 0 {
		// Nothing to change; return the existing listing as-is.
		return existing, nil
	}

	if err := validateJobFieldValues(input.FieldValues); err != nil {
		return JobDetail{}, err
	}

	// Fill in any omitted fields with the existing values.
	title := input.Title
	if title == "" {
		title = existing.Title
	}

	slug := input.Slug
	if slug == "" {
		slug = existing.Slug
	}

	body := input.Body
	if body == "" {
		body = existing.Body
	}

	var publishedAt pgtype.Timestamptz

	if input.PublishedAt != nil && *input.PublishedAt != "" {
		t, err := time.Parse(time.RFC3339, *input.PublishedAt)
		if err != nil {
			return JobDetail{}, fmt.Errorf("jobs: update listing: invalid published_at: %w", err)
		}

		publishedAt = pgtype.Timestamptz{Time: t, Valid: true}
	} else {
		// Preserve the existing published_at timestamp.
		if existing.PublishedAt != "" {
			t, err := time.Parse(time.RFC3339, existing.PublishedAt)
			if err == nil {
				publishedAt = pgtype.Timestamptz{Time: t, Valid: true}
			}
		}
	}

	// Rule 12 — recompute content_hash on update; NULL for empty bodies so
	// the partial unique index doesn't collide (see CreateJobListing).
	var contentHash *string
	if strings.TrimSpace(body) != "" {
		h := content.ComputeContentHash(body)
		contentHash = &h
	}

	setParts := []string{"updated_at = now()", "title = $2", "slug = $3", "body = $4", "content_hash = $5"}
	args := []any{jobID, title, slug, body, contentHash}
	argIdx := 6

	if publishedAt.Valid {
		setParts = append(setParts, fmt.Sprintf("published_at = $%d", argIdx))
		args = append(args, publishedAt)
		argIdx++
	}

	// Add the organization predicate last so the UPDATE can only touch rows
	// that belong to the caller's org.
	orgIdx := argIdx

	args = append(args, orgID)
	argIdx++

	query := fmt.Sprintf(`
		UPDATE contents SET %s
		WHERE id = $1 AND organization_id = $%d
		  AND content_type = 'job_listing' AND deleted_at IS NULL`,
		strings.Join(setParts, ", "), orgIdx)

	cmd, err := s.store.pool.Writer.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return JobDetail{}, ErrDuplicateContent
		}
		return JobDetail{}, fmt.Errorf("jobs: update listing: update content: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return JobDetail{}, pgx.ErrNoRows
	}

	// Upsert any custom field values provided.
	for fieldKey, value := range input.FieldValues {
		s.upsertJobFieldValue(ctx, jobID, fieldKey, value)
	}

	// Re-fetch from the writer to avoid read-replica lag.
	return s.GetJobByID(ctx, orgID, jobID)
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

// deriveSummary returns a short plain-text summary from a body. The contents
// table has no summary column (migration 000430 confirms this); the codebase
// convention is to derive a description from LEFT(body, N). Ref: §63.
func deriveSummary(body string) string {
	const max = 200

	r := []rune(strings.TrimSpace(body))
	if len(r) <= max {
		return string(r)
	}

	return string(r[:max]) + "…"
}

func sourceAttributionFromRow(src gen.ListSourcesByRecordIDsRow) SourceAttribution {
	a := SourceAttribution{
		ID:         src.SourceID.String(),
		Name:       src.Name,
		SourceURL:  src.SourceUrl,
		TrustLevel: src.TrustLevel,
	}
	if src.Slug != nil {
		a.Slug = *src.Slug
	}
	if src.LogoUrl != nil {
		a.LogoURL = *src.LogoUrl
	}
	return a
}

// jobFieldType describes how a job_listing custom field value is stored in
// custom_field_values. The EAV table has separate value_text / value_number /
// value_bool columns; writing every value to value_text breaks boolean and
// numeric filters/sorts (JOBS-007).
type jobFieldType string

const (
	fieldTypeText    jobFieldType = "text"
	fieldTypeNumber  jobFieldType = "number"
	fieldTypeBoolean jobFieldType = "boolean"
	// fieldTypeURL stores to value_text like text, but the value must be an
	// absolute http(s) URL — finder cards and detail pages open these values
	// as links, so javascript:/data:/scheme-relative junk must never persist.
	fieldTypeURL jobFieldType = "url"
)

// jobFieldTypes maps the 20 seeded job_listing_fields keys to their storage
// column. Must match migrations 000475 + 000734 + 000738 field_type values.
var jobFieldTypes = map[string]jobFieldType{
	"company_name":          fieldTypeText,
	"company_logo":          fieldTypeURL,
	"company_website":       fieldTypeURL,
	"job_type":              fieldTypeText,
	"job_role":              fieldTypeText,
	"job_function":          fieldTypeText,
	"location_type":         fieldTypeText,
	"country":               fieldTypeText,
	"city":                  fieldTypeText,
	"salary_min":            fieldTypeNumber,
	"salary_max":            fieldTypeNumber,
	"salary_currency":       fieldTypeText,
	"salary_period":         fieldTypeText,
	"experience_level":      fieldTypeText,
	"skills_required":       fieldTypeText,
	"application_url":       fieldTypeURL,
	"application_deadline":  fieldTypeText, // ISO date stored as value_text
	"visa_sponsorship":      fieldTypeBoolean,
	"relocation_assistance": fieldTypeBoolean,
	"is_featured":           fieldTypeBoolean,
}

// sanitizeJobFieldURL validates a url-typed field value. Only absolute http(s)
// URLs with a host are accepted — anything else (javascript:, data:, protocol
// -relative //host, bare strings) is rejected so cards can safely open the
// stored value in a new tab.
func sanitizeJobFieldURL(raw string) (string, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", true // empty = field not provided / cleared, not an error
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	return v, true
}

// sanitizeJobMediaURL validates company_logo. The media service returns
// uploaded assets as a same-origin path (/api/v1/media/serve/{bucket}/{key})
// when no CDN base URL is configured, so site-root-relative paths are
// accepted here in addition to absolute http(s) URLs — a leading "/" can
// never carry a javascript:/data: scheme. External-link fields
// (application_url, company_website) stay on the strict check.
func sanitizeJobMediaURL(raw string) (string, bool) {
	v := strings.TrimSpace(raw)
	if v != "" && strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "//") {
		return v, true
	}
	return sanitizeJobFieldURL(v)
}

// jobFieldURLSanitizer picks the validator for a url-typed field key.
func jobFieldURLSanitizer(key string) func(string) (string, bool) {
	if key == "company_logo" {
		return sanitizeJobMediaURL
	}
	return sanitizeJobFieldURL
}

// validateJobFieldValues checks all url-typed values before any write so a
// bad URL fails the whole request with a 400 instead of partially writing.
// Runs before the contents insert/update in every write path.
func validateJobFieldValues(fieldValues map[string]string) error {
	for key, value := range fieldValues {
		if jobFieldTypes[key] != fieldTypeURL {
			continue
		}
		if _, ok := jobFieldURLSanitizer(key)(value); !ok {
			return fmt.Errorf("%w: %s must be an absolute http(s) URL", ErrInvalidFieldValue, key)
		}
	}
	return nil
}

// upsertJobFieldValue upserts a single custom_field_values row with the
// correct typed column for the field key. It resolves field_id from
// custom_fields by key + platform, and verifies the target content is a
// job_listing in the platform (prevents cross-platform / non-job writes).
// Empty values are skipped. Unknown keys are skipped with a warning.
func (s *Service) upsertJobFieldValue(ctx context.Context, contentID pgtype.UUID, key, value string) {
	if value == "" {
		return
	}

	ft, ok := jobFieldTypes[key]
	if !ok {
		slog.Warn("jobs: unknown field key, skipping", "field_key", key)
		return
	}

	// url fields are pre-validated by validateJobFieldValues in every write
	// path; sanitize again here so no caller can bypass the scheme check.
	if ft == fieldTypeURL {
		v, ok := jobFieldURLSanitizer(key)(value)
		if !ok {
			slog.Warn("jobs: invalid url field value, skipping", "field_key", key)
			return
		}
		value = v
	}

	switch ft {
	case fieldTypeBoolean:
		b := strings.EqualFold(value, "true") || value == "1"
		if _, err := s.store.pool.Writer.Exec(ctx, `
			INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_bool)
			SELECT cf.id, 'content', $1, $3
			FROM custom_fields cf
			WHERE cf.key = $2
			  AND EXISTS (
				SELECT 1 FROM contents c
				WHERE c.id = $1 AND c.content_type = 'job_listing'
			  )
			ON CONFLICT (field_id, entity_type, entity_id)
			DO UPDATE SET value_bool = $3, updated_at = now()`,
			contentID, key, b); err != nil {
			slog.Warn("jobs: field value upsert failed", "field_key", key, "error", err)
		}
	case fieldTypeNumber:
		if _, err := s.store.pool.Writer.Exec(ctx, `
			INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_number)
			SELECT cf.id, 'content', $1, $3::numeric
			FROM custom_fields cf
			WHERE cf.key = $2
			  AND EXISTS (
				SELECT 1 FROM contents c
				WHERE c.id = $1 AND c.content_type = 'job_listing'
			  )
			ON CONFLICT (field_id, entity_type, entity_id)
			DO UPDATE SET value_number = $3::numeric, updated_at = now()`,
			contentID, key, value); err != nil {
			slog.Warn("jobs: field value upsert failed", "field_key", key, "error", err)
		}
	default:
		if _, err := s.store.pool.Writer.Exec(ctx, `
			INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_text)
			SELECT cf.id, 'content', $1, $3
			FROM custom_fields cf
			WHERE cf.key = $2
			  AND EXISTS (
				SELECT 1 FROM contents c
				WHERE c.id = $1 AND c.content_type = 'job_listing'
			  )
			ON CONFLICT (field_id, entity_type, entity_id)
			DO UPDATE SET value_text = $3, updated_at = now()`,
			contentID, key, value); err != nil {
			slog.Warn("jobs: field value upsert failed", "field_key", key, "error", err)
		}
	}
}

// ApplyToJob creates a job_applications record. Checks for duplicate — a user
// can only apply once per job. submissionID is optional (nil if applying with
// form data submitted separately).
func (s *Service) ApplyToJob(ctx context.Context, jobID, applicantID pgtype.UUID, submissionID *uuid.UUID) (gen.JobApplication, error) {
	var subID pgtype.UUID

	if submissionID != nil {
		var b [16]byte = *submissionID
		subID = pgtype.UUID{Bytes: b, Valid: true}
	}

	app, err := s.store.CreateJobApplication(ctx, jobID, applicantID, subID)
	if err != nil {
		// ON CONFLICT DO NOTHING with sqlc :one returns pgx.ErrNoRows when
		// the conflict triggers (no row returned). Detect that and re-query
		// to return the existing application with ErrDuplicateApplication.
		if errors.Is(err, pgx.ErrNoRows) {
			existing, qerr := s.findExistingApplication(ctx, jobID, applicantID)
			if qerr != nil {
				return gen.JobApplication{}, ErrDuplicateApplication
			}

			return existing, ErrDuplicateApplication
		}

		return gen.JobApplication{}, err
	}

	return app, nil
}

// ViewerApplicationForJob returns the viewer's own application for a job,
// or nil when none exists. Used by HandleJobDetail to render the applied
// state for returning applicants (JOBS apply flow).
func (s *Service) ViewerApplicationForJob(ctx context.Context, jobID, applicantID pgtype.UUID) *ViewerApplication {
	app, err := s.findExistingApplication(ctx, jobID, applicantID)
	if err != nil {
		return nil
	}
	v := &ViewerApplication{ID: app.ID.String(), Status: app.Status}
	if app.AppliedAt.Valid {
		v.AppliedAt = app.AppliedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return v
}

// findExistingApplication looks up an existing application for the user+job.
func (s *Service) findExistingApplication(ctx context.Context, jobID, applicantID pgtype.UUID) (gen.JobApplication, error) {
	query := `
		SELECT id, job_id, applicant_id, submission_id, status, employer_notes, applied_at, status_updated_at
		FROM job_applications
		WHERE job_id = $1 AND applicant_id = $2
		LIMIT 1`

	var app gen.JobApplication

	err := s.store.pool.Reader.QueryRow(ctx, query, jobID, applicantID).Scan(
		&app.ID, &app.JobID, &app.ApplicantID, &app.SubmissionID,
		&app.Status, &app.EmployerNotes, &app.AppliedAt, &app.StatusUpdatedAt,
	)
	if err != nil {
		return gen.JobApplication{}, err
	}

	return app, nil
}

// UpdateApplicationStatus updates the status of an application and enqueues a
// jobs:notify_application_status task fire-and-forget (Rule 8).
func (s *Service) UpdateApplicationStatus(ctx context.Context, appID pgtype.UUID, status string, employerNotes *string) (gen.JobApplication, error) {
	if !IsValidStatus(status) {
		return gen.JobApplication{}, fmt.Errorf("jobs: invalid status %q", status)
	}

	app, err := s.store.UpdateJobApplicationStatus(ctx, appID, status, employerNotes)
	if err != nil {
		return gen.JobApplication{}, fmt.Errorf("jobs: update application status: %w", err)
	}

	// Enqueue notification task fire-and-forget (Rule 8).
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"application_id": app.ID.String(),
			"applicant_id":   app.ApplicantID.String(),
			"job_id":         app.JobID.String(),

			"status": status,
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
func (s *Service) GetApplicationForNotification(ctx context.Context, applicationID pgtype.UUID) (gen.JobApplication, error) {
	return s.store.GetJobApplicationByID(ctx, applicationID)
}

// ListUserApplications returns an applicant's application history with
// pagination.
func (s *Service) ListUserApplications(ctx context.Context, userID pgtype.UUID, page, limit int) ([]ApplicationResult, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	// #nosec G115 // value bounded by caller
	offset := int32((page - 1) * limit)

	total, err := s.store.CountUserJobApplications(ctx, userID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.store.ListUserJobApplications(ctx, userID, offset, int32(limit)) //nolint:gosec // G115: bounded
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
func (s *Service) ListJobApplications(ctx context.Context, jobID pgtype.UUID, status string, page, limit int) ([]ApplicationResult, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	// #nosec G115 // value bounded by caller
	offset := int32((page - 1) * limit)

	total, err := s.store.CountJobApplicationsByJob(ctx, jobID, status)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.store.ListJobApplicationsByJob(ctx, jobID, status, offset, int32(limit)) //nolint:gosec // G115: bounded
	if err != nil {
		return nil, 0, err
	}

	results := make([]ApplicationResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, applicationResultFromJobRow(row))
	}

	return results, total, nil
}

// AdminListApplications returns all applications across all jobs for a platform,
// with pagination and optional status filter.
func (s *Service) AdminListApplications(ctx context.Context, status string, page, limit int) ([]ApplicationResult, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	// #nosec G115 // value bounded by caller
	offset := int32((page - 1) * limit)

	total, err := s.store.AdminCountJobApplications(ctx, status)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.store.AdminListJobApplications(ctx, status, offset, int32(limit)) //nolint:gosec // G115: bounded
	if err != nil {
		return nil, 0, err
	}

	results := make([]ApplicationResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, applicationResultFromAdminRow(row))
	}

	return results, total, nil
}

// AdminListJobs returns all job_listing contents for a platform, with
// pagination and optional status/q filters. This is the admin "all listings"
// view per Section 63: GET /admin/jobs — all listings (admin).
func (s *Service) AdminListJobs(ctx context.Context, status, q string, page, limit int) ([]AdminJobListing, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	// Build the WHERE clause — always platform + content_type + not
	// soft-deleted, optionally status. Admin can see archived listings via
	// the status filter; soft-deleted rows are excluded entirely.
	whereClause := "c.content_type = 'job_listing' AND c.deleted_at IS NULL"
	args := []any{}

	argIdx := 1
	if status != "" {
		whereClause += fmt.Sprintf(" AND c.status = $%d", argIdx)

		args = append(args, status)
		argIdx++
	}

	// Admin DataTable search box sends ?q= — filter on title (JOBS-017).
	if q != "" {
		whereClause += fmt.Sprintf(" AND c.title ILIKE $%d", argIdx)

		args = append(args, "%"+q+"%")
		argIdx++
	}

	// Count query.
	countSQL := "SELECT COUNT(*) FROM contents c WHERE " + whereClause

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
// by content status (published, archived, draft) for a platform.
func (s *Service) AdminJobStatusDistribution(ctx context.Context) (map[string]int64, error) {
	query := `
		SELECT status, COUNT(*) AS count
		FROM contents
		WHERE content_type = 'job_listing'
		GROUP BY status
		ORDER BY count DESC`

	rows, err := s.store.pool.Reader.Query(ctx, query)
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
// by status for a platform.
func (s *Service) AdminApplicationStatusDistribution(ctx context.Context) (map[string]int64, error) {
	rows, err := s.store.AdminApplicationStatusDistribution(ctx)
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
func (s *Service) ExpireListingsPastDeadline(ctx context.Context) (int64, error) {
	return s.store.ExpireListingsPastDeadline(ctx)
}

// ToggleFeature toggles the is_featured custom field value for a job listing.
// Used by the admin feature route. Platform-scoped and content-type-checked for
// security (prevents cross-platform toggle and toggling non-job content).
func (s *Service) ToggleFeature(ctx context.Context, jobID pgtype.UUID) (bool, error) {
	// Read the current value — verify platform + content_type = job_listing.
	// Join custom_field_values → custom_fields to resolve the field key.
	var current bool

	err := s.store.pool.Reader.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT cfv.value_bool
			 FROM custom_field_values cfv
			 JOIN custom_fields cf ON cf.id = cfv.field_id
			 JOIN contents c ON c.id = cfv.entity_id
			 WHERE cfv.entity_type = 'content' AND cfv.entity_id = $1
			   
			   AND c.content_type = 'job_listing'
			   AND cf.key = 'is_featured'
			 LIMIT 1),
			false
		)`, jobID).Scan(&current)
	if err != nil {
		return false, fmt.Errorf("jobs: toggle feature: read: %w", err)
	}

	newVal := !current

	// Resolve the custom_fields.id for 'is_featured' in this platform, then
	// upsert the custom_field_values row.
	var fieldID pgtype.UUID

	err = s.store.pool.Writer.QueryRow(ctx, `
		SELECT id FROM custom_fields
		WHERE key = 'is_featured'
		LIMIT 1`).Scan(&fieldID)
	if err != nil {
		return false, fmt.Errorf("jobs: toggle feature: resolve field id: %w", err)
	}

	_, err = s.store.pool.Writer.Exec(ctx, `
		INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_bool)
		SELECT $1, 'content', $2, $3
		WHERE EXISTS (
			SELECT 1 FROM contents c
			WHERE c.id = $2 AND c.content_type = 'job_listing'
		)
		ON CONFLICT (field_id, entity_type, entity_id)
		DO UPDATE SET value_bool = $3`,
		fieldID, jobID, newVal)
	if err != nil {
		return false, fmt.Errorf("jobs: toggle feature: upsert: %w", err)
	}

	return newVal, nil
}

// AdminUpdateJob updates a job listing's core fields (title, body, status)
// and custom field values. The jobID must be a content row with
// content_type='job_listing' in the given platform. The contents table has no
// summary column (JOBS-004); summary is derived from body at read time.
func (s *Service) AdminUpdateJob(ctx context.Context, jobID pgtype.UUID, title, summary, body, status *string, fieldValues map[string]string) error {
	if err := validateJobFieldValues(fieldValues); err != nil {
		return err
	}

	// Build the SET clause dynamically based on which fields are provided.
	// summary is intentionally ignored — no such column (JOBS-004).
	setParts := []string{"updated_at = now()"}
	args := []interface{}{jobID}

	argIdx := 2
	if title != nil {
		setParts = append(setParts, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *title)
		argIdx++
	}

	if body != nil {
		setParts = append(setParts, fmt.Sprintf("body = $%d", argIdx))
		args = append(args, *body)
		argIdx++
		// Rule 12: recompute content_hash when body changes; NULL for empty
		// bodies — the unique index is partial (WHERE content_hash IS NOT NULL).
		setParts = append(setParts, fmt.Sprintf("content_hash = $%d", argIdx))
		if strings.TrimSpace(*body) == "" {
			args = append(args, nil)
		} else {
			args = append(args, content.ComputeContentHash(*body))
		}
		argIdx++
	}

	if status != nil {
		setParts = append(setParts, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *status)
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE contents SET %s
		WHERE id = $1 AND content_type = 'job_listing'`,
		strings.Join(setParts, ", "))

	_, err := s.store.pool.Writer.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateContent
		}
		return fmt.Errorf("jobs: admin update: update content: %w", err)
	}

	// Update custom field values via the typed upsert helper (correct
	// value_* column per field type — JOBS-007).
	for fieldKey, value := range fieldValues {
		s.upsertJobFieldValue(ctx, jobID, fieldKey, value)
	}

	return nil
}

// AdminDeleteJob soft-deletes a job listing by setting deleted_at = now().
// The jobID must be a content row with content_type='job_listing' in the
// given platform.
func (s *Service) AdminDeleteJob(ctx context.Context, jobID pgtype.UUID) error {
	_, err := s.store.pool.Writer.Exec(ctx, `
		UPDATE contents SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND content_type = 'job_listing'`,
		jobID)
	if err != nil {
		return fmt.Errorf("jobs: admin delete: %w", err)
	}

	return nil
}

// applicationResultFromUserRow converts a ListUserJobApplicationsRow to an
// ApplicationResult.
func applicationResultFromUserRow(row gen.ListUserJobApplicationsRow) ApplicationResult {
	r := ApplicationResult{
		ID:            row.ID.String(),
		JobID:         row.JobID.String(),
		JobTitle:      row.JobTitle,
		JobSlug:       row.JobSlug,
		Status:        row.Status,
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

// decodeSubmissionFields unmarshals a form_submissions.field_values JSONB
// blob into a map for the API response. Returns nil for absent or
// unparseable values so the field serializes as omitted.
func decodeSubmissionFields(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// applicationResultFromJobRow converts a ListJobApplicationsByJobRow to an
// ApplicationResult.
func applicationResultFromJobRow(row gen.ListJobApplicationsByJobRow) ApplicationResult {
	r := ApplicationResult{
		ID:                row.ID.String(),
		JobID:             row.JobID.String(),
		ApplicantID:       row.ApplicantID.String(),
		ApplicantUsername: row.ApplicantUsername,
		ApplicantEmail:    row.ApplicantEmail,
		Status:            row.Status,
		EmployerNotes:     row.EmployerNotes,
		Submission:        decodeSubmissionFields(row.SubmissionFields),
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
		ID:                row.ID.String(),
		JobID:             row.JobID.String(),
		ApplicantID:       row.ApplicantID.String(),
		ApplicantUsername: row.ApplicantUsername,
		JobTitle:          row.JobTitle,
		Status:            row.Status,
		EmployerNotes:     row.EmployerNotes,
	}
	if row.AppliedAt.Valid {
		r.AppliedAt = row.AppliedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}

	if row.StatusUpdatedAt.Valid {
		r.StatusUpdatedAt = row.StatusUpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
	}

	return r
}
