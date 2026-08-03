// Package assessment implements the Japa Readiness Assessment (Section 71,
// ADR-071). It is a scored relocation-readiness questionnaire built on the
// §06 Form Builder infrastructure. Admin configures questions, weights, and
// thresholds; scoring runs server-side in an Asynq task; output is a total
// score, dimension breakdown, threshold label, and a personalised roadmap.
//
// store.go is the data-access layer — it wraps the sqlc-generated Queries for
// assessment operations. No business logic lives here.
//
// Ref: japaconnect-engineering/sections/71-readiness-assessment.md
//      japaconnect-engineering/adr/ADR-071-japa-readiness-assessment.md
package assessment

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for assessment operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates an assessment store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// GetActiveAssessmentForm returns the active assessment form
// (form_type='assessment', status='active') for the tenant.
func (s *Store) GetActiveAssessmentForm(ctx context.Context, tenantID pgtype.UUID) (gen.Form, error) {
	form, err := s.queries.GetActiveAssessmentForm(ctx, tenantID)
	if err != nil {
		return gen.Form{}, fmt.Errorf("assessment: get active form: %w", err)
	}
	return form, nil
}

// GetAssessmentFormByID returns an assessment form by ID.
func (s *Store) GetAssessmentFormByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.Form, error) {
	form, err := s.queries.GetAssessmentFormByID(ctx, gen.GetAssessmentFormByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return gen.Form{}, fmt.Errorf("assessment: get form by id: %w", err)
	}
	return form, nil
}

// GetAssessmentFormFields returns the custom fields attached to an assessment
// form, ordered by position. The scoring engine uses each field's config
// JSONB to resolve per-answer scores.
func (s *Store) GetAssessmentFormFields(ctx context.Context, tenantID, formID pgtype.UUID) ([]gen.CustomField, error) {
	fields, err := s.queries.GetAssessmentFormFields(ctx, gen.GetAssessmentFormFieldsParams{
		TenantID: tenantID,
		FormID:   formID,
	})
	if err != nil {
		return nil, fmt.Errorf("assessment: get form fields: %w", err)
	}
	return fields, nil
}

// CreateAssessmentSubmission inserts a form_submissions row for an assessment.
// userID may be invalid (zero) for anonymous submissions.
func (s *Store) CreateAssessmentSubmission(ctx context.Context, formID, tenantID, userID pgtype.UUID, fieldValues []byte) (gen.FormSubmission, error) {
	sub, err := s.queries.CreateAssessmentSubmission(ctx, gen.CreateAssessmentSubmissionParams{
		FormID:      formID,
		TenantID:    tenantID,
		UserID:      userID,
		FieldValues: fieldValues,
	})
	if err != nil {
		return gen.FormSubmission{}, fmt.Errorf("assessment: create submission: %w", err)
	}
	return sub, nil
}

// CreateAssessmentResult stores a scored assessment result.
func (s *Store) CreateAssessmentResult(ctx context.Context, tenantID, userID, submissionID pgtype.UUID, totalScore int32, dimensionScores []byte, thresholdLabel string, roadmapContent []byte) (gen.AssessmentResult, error) {
	result, err := s.queries.CreateAssessmentResult(ctx, gen.CreateAssessmentResultParams{
		TenantID:        tenantID,
		UserID:          userID,
		SubmissionID:    submissionID,
		TotalScore:      totalScore,
		DimensionScores: dimensionScores,
		ThresholdLabel:  thresholdLabel,
		RoadmapContent:  roadmapContent,
	})
	if err != nil {
		return gen.AssessmentResult{}, fmt.Errorf("assessment: create result: %w", err)
	}
	return result, nil
}

// GetAssessmentResultByID returns a single assessment result by ID.
func (s *Store) GetAssessmentResultByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.AssessmentResult, error) {
	result, err := s.queries.GetAssessmentResultByID(ctx, gen.GetAssessmentResultByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return gen.AssessmentResult{}, fmt.Errorf("assessment: get result: %w", err)
	}
	return result, nil
}

// ListUserAssessmentResults returns a user's assessment results (history),
// paginated by offset/limit, newest first.
func (s *Store) ListUserAssessmentResults(ctx context.Context, tenantID, userID pgtype.UUID, pageOffset, pageLimit int32) ([]gen.AssessmentResult, error) {
	results, err := s.queries.ListUserAssessmentResults(ctx, gen.ListUserAssessmentResultsParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageOffset: pageOffset,
		PageLimit:  pageLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("assessment: list user results: %w", err)
	}
	return results, nil
}

// CountUserAssessmentResults returns the total count of a user's results.
func (s *Store) CountUserAssessmentResults(ctx context.Context, tenantID, userID pgtype.UUID) (int64, error) {
	count, err := s.queries.CountUserAssessmentResults(ctx, gen.CountUserAssessmentResultsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return 0, fmt.Errorf("assessment: count user results: %w", err)
	}
	return count, nil
}

// UpdateAssessmentFormScoring updates the scoring_config JSONB on an
// assessment form. Admin-only.
func (s *Store) UpdateAssessmentFormScoring(ctx context.Context, tenantID, id pgtype.UUID, scoringConfig []byte) (gen.Form, error) {
	form, err := s.queries.UpdateAssessmentFormScoring(ctx, gen.UpdateAssessmentFormScoringParams{
		ScoringConfig: scoringConfig,
		TenantID:      tenantID,
		ID:            id,
	})
	if err != nil {
		return gen.Form{}, fmt.Errorf("assessment: update form scoring: %w", err)
	}
	return form, nil
}

// AdminListAssessmentResultsRow is a single row from the admin results list.
type AdminListAssessmentResultsRow struct {
	ID               pgtype.UUID
	TenantID         pgtype.UUID
	UserID           pgtype.UUID
	SubmissionID     pgtype.UUID
	TotalScore       int32
	DimensionScores  []byte
	ThresholdLabel   string
	RoadmapContent   []byte
	CreatedAt        pgtype.Timestamptz
	Username         *string
	SubmissionValues []byte
}

// AdminListAssessmentResults returns all users' assessment results for the
// tenant, paginated and filterable by score range. Admin-only.
func (s *Store) AdminListAssessmentResults(ctx context.Context, tenantID pgtype.UUID, minScore, maxScore, pageOffset, pageLimit int32) ([]AdminListAssessmentResultsRow, error) {
	rows, err := s.queries.AdminListAssessmentResults(ctx, gen.AdminListAssessmentResultsParams{
		TenantID:   tenantID,
		MinScore:   minScore,
		MaxScore:   maxScore,
		PageOffset: pageOffset,
		PageLimit:  pageLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("assessment: admin list results: %w", err)
	}
	results := make([]AdminListAssessmentResultsRow, 0, len(rows))
	for _, r := range rows {
		results = append(results, AdminListAssessmentResultsRow{
			ID:               r.ID,
			TenantID:         r.TenantID,
			UserID:           r.UserID,
			SubmissionID:     r.SubmissionID,
			TotalScore:       r.TotalScore,
			DimensionScores:  r.DimensionScores,
			ThresholdLabel:   r.ThresholdLabel,
			RoadmapContent:   r.RoadmapContent,
			CreatedAt:        r.CreatedAt,
			Username:         r.Username,
			SubmissionValues: r.SubmissionValues,
		})
	}
	return results, nil
}

// AdminCountAssessmentResults returns the total count of assessment results
// for the tenant, optionally filtered by score range. Admin-only.
func (s *Store) AdminCountAssessmentResults(ctx context.Context, tenantID pgtype.UUID, minScore, maxScore int32) (int64, error) {
	count, err := s.queries.AdminCountAssessmentResults(ctx, gen.AdminCountAssessmentResultsParams{
		TenantID: tenantID,
		MinScore: minScore,
		MaxScore: maxScore,
	})
	if err != nil {
		return 0, fmt.Errorf("assessment: admin count results: %w", err)
	}
	return count, nil
}

// AdminScoreDistributionRow is a single row from the score distribution query.
type AdminScoreDistributionRow struct {
	ThresholdLabel string
	Count          int64
	AvgScore       int32
}

// AdminScoreDistribution returns the count and average score of assessment
// results grouped by threshold label. Admin-only.
func (s *Store) AdminScoreDistribution(ctx context.Context, tenantID pgtype.UUID) ([]AdminScoreDistributionRow, error) {
	rows, err := s.queries.AdminAssessmentScoreDistribution(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("assessment: admin score distribution: %w", err)
	}
	results := make([]AdminScoreDistributionRow, 0, len(rows))
	for _, r := range rows {
		results = append(results, AdminScoreDistributionRow{
			ThresholdLabel: r.ThresholdLabel,
			Count:          r.Count,
			AvgScore:       r.AvgScore,
		})
	}
	return results, nil
}

// AdminCompletionStatsRow holds the aggregate completion statistics.
type AdminCompletionStatsRow struct {
	TotalSubmissions int64
	TotalScored      int64
	AvgScore         int32
}

// AdminCompletionStats returns total submissions, total scored results, and
// the average score across all assessment submissions. Admin-only.
func (s *Store) AdminCompletionStats(ctx context.Context, tenantID pgtype.UUID) (AdminCompletionStatsRow, error) {
	row, err := s.queries.AdminAssessmentCompletionStats(ctx, tenantID)
	if err != nil {
		return AdminCompletionStatsRow{}, fmt.Errorf("assessment: admin completion stats: %w", err)
	}
	return AdminCompletionStatsRow{
		TotalSubmissions: row.TotalSubmissions,
		TotalScored:      row.TotalScored,
		AvgScore:         row.AvgScore,
	}, nil
}
