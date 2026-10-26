// Package jobs implements the Jobs Board (Section 63, ADR-081).
//
// Job listings are contents rows with content_type='job_listing'. The
// job-specific attributes (company_name, job_type, country, etc.) live in
// custom_field_values under the job_listing_fields field group. On-platform
// applications are tracked in the job_applications table.
//
// Ref: japaconnect-engineering/sections/63-jobs-board.md
//
//	japaconnect-engineering/adr/ADR-081-jobs-board.md
package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for job_applications operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries      *gen.Queries
	writeQueries *gen.Queries
	pool         *db.DB
}

// NewStore creates a Jobs store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries:      gen.New(database.Reader),
		writeQueries: gen.New(database.Writer),
		pool:         database,
	}
}

// MapSourcesByRecordIDs returns a map of source attribution keyed by intel record ID.
func (s *Store) MapSourcesByRecordIDs(ctx context.Context, recordIDs []pgtype.UUID) (map[string]gen.ListSourcesByRecordIDsRow, error) {
	rows, err := s.queries.ListSourcesByRecordIDs(ctx, recordIDs)
	if err != nil {
		return nil, fmt.Errorf("jobs: map sources by record ids: %w", err)
	}

	m := make(map[string]gen.ListSourcesByRecordIDsRow, len(rows))
	for _, r := range rows {
		m[r.RecordID.String()] = r
	}

	return m, nil
}

// CreateJobApplication inserts a job_applications row. The ON CONFLICT clause
// in the generated query ensures a user can only apply once per job.
func (s *Store) CreateJobApplication(ctx context.Context, jobID, applicantID, submissionID pgtype.UUID) (gen.JobApplication, error) {
	app, err := s.writeQueries.CreateJobApplication(ctx, gen.CreateJobApplicationParams{
		JobID:        jobID,
		ApplicantID:  applicantID,
		SubmissionID: submissionID,
	})
	if err != nil {
		return gen.JobApplication{}, fmt.Errorf("jobs: create application: %w", err)
	}

	return app, nil
}

// GetJobApplicationByID returns a single application by ID.
func (s *Store) GetJobApplicationByID(ctx context.Context, appID pgtype.UUID) (gen.JobApplication, error) {
	app, err := s.queries.GetJobApplicationByID(ctx,

		appID,
	)
	if err != nil {
		return gen.JobApplication{}, fmt.Errorf("jobs: get application: %w", err)
	}

	return app, nil
}

// UpdateJobApplicationStatus updates the status and employer notes of an
// application.
func (s *Store) UpdateJobApplicationStatus(ctx context.Context, appID pgtype.UUID, status string, employerNotes *string) (gen.JobApplication, error) {
	app, err := s.writeQueries.UpdateJobApplicationStatus(ctx, gen.UpdateJobApplicationStatusParams{
		Status:        status,
		EmployerNotes: employerNotes,

		ID: appID,
	})
	if err != nil {
		return gen.JobApplication{}, fmt.Errorf("jobs: update application status: %w", err)
	}

	return app, nil
}

// ListJobApplicationsByJob returns applications for a specific job (employer
// view), optionally filtered by status.
func (s *Store) ListJobApplicationsByJob(ctx context.Context, jobID pgtype.UUID, status string, pageOffset, pageLimit int32) ([]gen.ListJobApplicationsByJobRow, error) {
	rows, err := s.queries.ListJobApplicationsByJob(ctx, gen.ListJobApplicationsByJobParams{
		JobID:      jobID,
		Status:     status,
		PageOffset: pageOffset,
		PageLimit:  pageLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: list job applications: %w", err)
	}

	return rows, nil
}

// CountJobApplicationsByJob returns the total number of applications for a
// job, optionally filtered by status.
func (s *Store) CountJobApplicationsByJob(ctx context.Context, jobID pgtype.UUID, status string) (int64, error) {
	return s.queries.CountJobApplicationsByJob(ctx, gen.CountJobApplicationsByJobParams{
		JobID:  jobID,
		Status: status,
	})
}

// ListUserJobApplications returns an applicant's application history.
func (s *Store) ListUserJobApplications(ctx context.Context, userID pgtype.UUID, pageOffset, pageLimit int32) ([]gen.ListUserJobApplicationsRow, error) {
	rows, err := s.queries.ListUserJobApplications(ctx, gen.ListUserJobApplicationsParams{
		UserID:     userID,
		PageOffset: pageOffset,
		PageLimit:  pageLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: list user applications: %w", err)
	}

	return rows, nil
}

// CountUserJobApplications returns the total number of applications submitted
// by a user.
func (s *Store) CountUserJobApplications(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return s.queries.CountUserJobApplications(ctx,

		userID,
	)
}

// AdminListJobApplications returns all applications across all jobs, optionally
// filtered by status.
func (s *Store) AdminListJobApplications(ctx context.Context, status string, pageOffset, pageLimit int32) ([]gen.AdminListJobApplicationsRow, error) {
	rows, err := s.queries.AdminListJobApplications(ctx, gen.AdminListJobApplicationsParams{
		Status:     status,
		PageOffset: pageOffset,
		PageLimit:  pageLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: admin list applications: %w", err)
	}

	return rows, nil
}

// AdminCountJobApplications returns the total number of applications for a
// platform, optionally filtered by status.
func (s *Store) AdminCountJobApplications(ctx context.Context, status string) (int64, error) {
	return s.queries.AdminCountJobApplications(ctx,

		status,
	)
}

// AdminApplicationStatusDistribution returns the count of applications grouped
// by status.
func (s *Store) AdminApplicationStatusDistribution(ctx context.Context) ([]gen.AdminJobApplicationStatusDistributionRow, error) {
	rows, err := s.queries.AdminJobApplicationStatusDistribution(ctx)
	if err != nil {
		return nil, fmt.Errorf("jobs: admin status distribution: %w", err)
	}

	return rows, nil
}

// ExpireListingsPastDeadline archives published job_listing content whose
// application_deadline custom field value is in the past. Returns the
// expired rows (id + slug) so the caller can purge each listing's detail
// page at the edge CDN (ADR-134).
func (s *Store) ExpireListingsPastDeadline(ctx context.Context) ([]gen.ExpireJobListingsPastDeadlineRow, error) {
	rows, err := s.writeQueries.ExpireJobListingsPastDeadline(ctx)
	if err != nil {
		return nil, fmt.Errorf("jobs: expire listings: %w", err)
	}

	return rows, nil
}
