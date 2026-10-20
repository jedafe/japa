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
//
//	japaconnect-engineering/adr/ADR-071-japa-readiness-assessment.md
package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for assessment operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries      *gen.Queries
	writeQueries *gen.Queries
	pool         *db.DB
}

// NewStore creates an assessment store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries:      gen.New(database.Reader),
		writeQueries: gen.New(database.Writer),
		pool:         database,
	}
}

// GetActiveAssessmentForm returns the active assessment form
// (form_type='assessment', status='active') for the platform.
func (s *Store) GetActiveAssessmentForm(ctx context.Context) (gen.Form, error) {
	form, err := s.queries.GetActiveAssessmentForm(ctx)
	if err != nil {
		return gen.Form{}, fmt.Errorf("assessment: get active form: %w", err)
	}

	return form, nil
}

// GetAssessmentFormFields returns the custom fields attached to an assessment
// form, ordered by position. The scoring engine uses each field's config
// JSONB to resolve per-answer scores.
func (s *Store) GetAssessmentFormFields(ctx context.Context, formID pgtype.UUID) ([]gen.CustomField, error) {
	fields, err := s.queries.GetAssessmentFormFields(ctx,

		formID,
	)
	if err != nil {
		return nil, fmt.Errorf("assessment: get form fields: %w", err)
	}

	return fields, nil
}

// CreateAssessmentSubmission inserts a form_submissions row for an assessment.
// userID may be invalid (zero) for anonymous submissions.
func (s *Store) CreateAssessmentSubmission(ctx context.Context, formID, userID pgtype.UUID, fieldValues []byte) (gen.FormSubmission, error) {
	sub, err := s.writeQueries.CreateAssessmentSubmission(ctx, gen.CreateAssessmentSubmissionParams{
		FormID: formID,

		UserID:      userID,
		FieldValues: fieldValues,
	})
	if err != nil {
		return gen.FormSubmission{}, fmt.Errorf("assessment: create submission: %w", err)
	}

	return sub, nil
}

// CreateAssessmentResult stores a scored assessment result.
func (s *Store) CreateAssessmentResult(ctx context.Context, userID, submissionID pgtype.UUID, totalScore int32, dimensionScores []byte, thresholdLabel string, roadmapContent []byte) (gen.AssessmentResult, error) {
	result, err := s.writeQueries.CreateAssessmentResult(ctx, gen.CreateAssessmentResultParams{
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
func (s *Store) GetAssessmentResultByID(ctx context.Context, id pgtype.UUID) (gen.AssessmentResult, error) {
	result, err := s.queries.GetAssessmentResultByID(ctx,

		id,
	)
	if err != nil {
		return gen.AssessmentResult{}, fmt.Errorf("assessment: get result: %w", err)
	}

	return result, nil
}

// GetAssessmentResultBySubmission returns the assessment result linked to a
// submission, if the Asynq scoring task has written it yet. Used by the
// public polling endpoint after an async submit.
func (s *Store) GetAssessmentResultBySubmission(ctx context.Context, submissionID pgtype.UUID) (gen.AssessmentResult, error) {
	result, err := s.queries.GetAssessmentResultBySubmission(ctx,

		submissionID,
	)
	if err != nil {
		return gen.AssessmentResult{}, fmt.Errorf("assessment: get result by submission: %w", err)
	}

	return result, nil
}

// ListUserAssessmentResults returns a user's assessment results (history),
// paginated by offset/limit, newest first.
func (s *Store) ListUserAssessmentResults(ctx context.Context, userID pgtype.UUID, pageOffset, pageLimit int32) ([]gen.AssessmentResult, error) {
	results, err := s.queries.ListUserAssessmentResults(ctx, gen.ListUserAssessmentResultsParams{
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
func (s *Store) CountUserAssessmentResults(ctx context.Context, userID pgtype.UUID) (int64, error) {
	count, err := s.queries.CountUserAssessmentResults(ctx,

		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("assessment: count user results: %w", err)
	}

	return count, nil
}

// CountAssessmentResultsByUserAndForm returns how many results the user has
// for a specific assessment form. Used to award completion points only once
// per user per form (Section 28).
func (s *Store) CountAssessmentResultsByUserAndForm(ctx context.Context, userID, formID pgtype.UUID) (int64, error) {
	count, err := s.queries.CountAssessmentResultsByUserAndForm(ctx, gen.CountAssessmentResultsByUserAndFormParams{
		UserID: userID,
		FormID: formID,
	})
	if err != nil {
		return 0, fmt.Errorf("assessment: count user results by form: %w", err)
	}

	return count, nil
}

// UpdateAssessmentFormScoring updates the scoring_config JSONB on an
// assessment form. Admin-only.
func (s *Store) UpdateAssessmentFormScoring(ctx context.Context, id pgtype.UUID, scoringConfig []byte) (gen.Form, error) {
	form, err := s.writeQueries.UpdateAssessmentFormScoring(ctx, gen.UpdateAssessmentFormScoringParams{
		ScoringConfig: scoringConfig,

		ID: id,
	})
	if err != nil {
		return gen.Form{}, fmt.Errorf("assessment: update form scoring: %w", err)
	}

	return form, nil
}

// AdminListAssessmentResultsRow is a single row from the admin results list.
type AdminListAssessmentResultsRow struct {
	ID               pgtype.UUID
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

// adminAssessmentSortColumns maps sort keys to their qualified column names.
// Only these columns may appear in the ORDER BY clause (SQLi guard). Mirrors
// the digest store's allowedSortColumns pattern.
var adminAssessmentSortColumns = map[string]string{
	"total_score":     "ar.total_score",
	"threshold_label": "ar.threshold_label",
	"created_at":      "ar.created_at",
	"username":        "u.username",
}

// AdminListAssessmentResultsFilter holds the parameters for the admin results
// list query: score-range filter, pagination, and sort.
type AdminListAssessmentResultsFilter struct {
	MinScore int32
	MaxScore int32
	Offset   int32
	Limit    int32
	Sort     string // whitelisted key (total_score, threshold_label, created_at, username)
	Order    string // "asc" or "desc" (default desc)
}

// AdminListAssessmentResults returns all users' assessment results for the
// platform, paginated, filterable by score range, and sortable by a whitelisted
// column. Admin-only. Uses a hand-written query (not sqlc) so the ORDER BY
// clause can be built from a sanitized sort key + direction.
func (s *Store) AdminListAssessmentResults(ctx context.Context, f AdminListAssessmentResultsFilter) ([]AdminListAssessmentResultsRow, int64, error) {
	var conditions []string

	var args []any

	argIdx := 1

	if f.MinScore != 0 {
		conditions = append(conditions, fmt.Sprintf("ar.total_score >= $%d", argIdx))
		args = append(args, f.MinScore)
		argIdx++
	}

	if f.MaxScore != 0 {
		conditions = append(conditions, fmt.Sprintf("ar.total_score <= $%d", argIdx))
		args = append(args, f.MaxScore)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "TRUE"
	}

	// Build ORDER BY from whitelisted sort column + direction.
	sortCol := adminAssessmentSortColumns[f.Sort]
	if sortCol == "" {
		sortCol = "ar.created_at"
	}

	orderDir := "DESC"
	if strings.ToUpper(f.Order) == "ASC" {
		orderDir = "ASC"
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	listQ := fmt.Sprintf(`
SELECT
    ar.id, ar.user_id, ar.submission_id, ar.total_score,
    ar.dimension_scores, ar.threshold_label, ar.roadmap_content, ar.created_at,
    u.username AS username,
    fs.field_values AS submission_values
FROM assessment_results ar
LEFT JOIN users u ON u.id = ar.user_id
JOIN form_submissions fs ON fs.id = ar.submission_id
WHERE %s
ORDER BY %s %s, ar.id DESC
LIMIT $%d OFFSET $%d`,
		whereClause, sortCol, orderDir, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("assessment: admin list results: %w", err)
	}
	defer rows.Close()

	results := make([]AdminListAssessmentResultsRow, 0)

	for rows.Next() {
		var r AdminListAssessmentResultsRow
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.SubmissionID, &r.TotalScore,
			&r.DimensionScores, &r.ThresholdLabel, &r.RoadmapContent, &r.CreatedAt,
			&r.Username, &r.SubmissionValues,
		); err != nil {
			return nil, 0, fmt.Errorf("assessment: admin list results: scan: %w", err)
		}

		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("assessment: admin list results: rows: %w", err)
	}

	// Count query for pagination total.
	countConditions := conditions[:0]

	var countArgs []any

	if f.MinScore != 0 {
		countConditions = append(countConditions, fmt.Sprintf("total_score >= $%d", len(countArgs)+1))
		countArgs = append(countArgs, f.MinScore)
	}

	if f.MaxScore != 0 {
		countConditions = append(countConditions, fmt.Sprintf("total_score <= $%d", len(countArgs)+1))
		countArgs = append(countArgs, f.MaxScore)
	}

	countWhere := strings.Join(countConditions, " AND ")
	if countWhere == "" {
		countWhere = "1=1"
	}

	countQ := "SELECT COUNT(*) FROM assessment_results WHERE " + countWhere

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("assessment: admin count results: %w", err)
	}

	return results, total, nil
}

// AdminCountAssessmentResults returns the total count of assessment results
// for the platform, optionally filtered by score range. Admin-only.
func (s *Store) AdminCountAssessmentResults(ctx context.Context, minScore, maxScore int32) (int64, error) {
	count, err := s.queries.AdminCountAssessmentResults(ctx, gen.AdminCountAssessmentResultsParams{
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
func (s *Store) AdminScoreDistribution(ctx context.Context) ([]AdminScoreDistributionRow, error) {
	rows, err := s.queries.AdminAssessmentScoreDistribution(ctx)
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
func (s *Store) AdminCompletionStats(ctx context.Context) (AdminCompletionStatsRow, error) {
	row, err := s.queries.AdminAssessmentCompletionStats(ctx)
	if err != nil {
		return AdminCompletionStatsRow{}, fmt.Errorf("assessment: admin completion stats: %w", err)
	}

	return AdminCompletionStatsRow{
		TotalSubmissions: row.TotalSubmissions,
		TotalScored:      row.TotalScored,
		AvgScore:         row.AvgScore,
	}, nil
}

// CleanupAnonymousResults deletes anonymous assessment_results (user_id IS
// NULL) older than the given retention window in hours. Called by the
// assessment:cleanup_anonymous Asynq task (ADR-071 §5).
func (s *Store) CleanupAnonymousResults(ctx context.Context, retentionHours int32) (int64, error) {
	n, err := s.writeQueries.CleanupAnonymousAssessmentResults(ctx, retentionHours)
	if err != nil {
		return 0, fmt.Errorf("assessment: cleanup anonymous results: %w", err)
	}

	// Also purge the now-orphaned anonymous submissions — their field_values
	// carry journey context and must not outlive the anonymous retention
	// window. Only submissions whose result row no longer exists are deleted
	// so a still-referenced submission is never pulled from under a result.
	if _, err := s.writeQueries.CleanupAnonymousAssessmentSubmissions(ctx, retentionHours); err != nil {
		return n, fmt.Errorf("assessment: cleanup anonymous submissions: %w", err)
	}

	return n, nil
}

// CreateAssessmentInput holds the fields for creating a readiness assessment
// form. The form_type is set to 'assessment' after the initial insert because
// the sqlc CreateForm query predates the form_type column.
type CreateAssessmentInput struct {
	Name                 string      `json:"name"`
	Slug                 string      `json:"slug"`
	Description          string      `json:"description"`
	IsMultiStep          bool        `json:"is_multi_step"`
	SubmitLabel          string      `json:"submit_label"`
	SuccessMessage       string      `json:"success_message"`
	Status               string      `json:"status"`
	SpaceID              pgtype.UUID `json:"space_id"`
	ScoringConfig        any         `json:"scoring_config"`
	SubmissionFeeCredits int32       `json:"submission_fee_credits"`
}

// UpdateAssessmentInput holds the mutable fields for a readiness assessment
// form. Pointer fields mean "leave unchanged"; non-pointer fields are optional
// scalars that replace the existing value.
type UpdateAssessmentInput struct {
	Name                 *string     `json:"name,omitempty"`
	Slug                 *string     `json:"slug,omitempty"`
	Description          *string     `json:"description,omitempty"`
	IsMultiStep          *bool       `json:"is_multi_step,omitempty"`
	SubmitLabel          *string     `json:"submit_label,omitempty"`
	SuccessMessage       *string     `json:"success_message,omitempty"`
	Status               *string     `json:"status,omitempty"`
	SpaceID              pgtype.UUID `json:"space_id,omitempty"`
	ScoringConfig        any         `json:"scoring_config,omitempty"`
	SubmissionFeeCredits *int32      `json:"submission_fee_credits,omitempty"`
}

// CreateAssessmentForm inserts a forms row and then sets form_type='assessment'
// and the scoring_config JSONB.
func (s *Store) CreateAssessmentForm(ctx context.Context, in CreateAssessmentInput) (gen.Form, error) {
	description := strPtrOrNil(in.Description)
	successMessage := strPtrOrNil(in.SuccessMessage)

	status := in.Status
	if status == "" {
		status = "active"
	}

	submitLabel := in.SubmitLabel
	if submitLabel == "" {
		submitLabel = "Submit"
	}

	created, err := s.writeQueries.CreateForm(ctx, gen.CreateFormParams{
		SpaceID:              in.SpaceID,
		Name:                 in.Name,
		Slug:                 in.Slug,
		Description:          description,
		IsMultiStep:          in.IsMultiStep,
		SubmitLabel:          submitLabel,
		SuccessMessage:       successMessage,
		Status:               status,
		IsSystem:             false,
		SubmissionFeeCredits: in.SubmissionFeeCredits,
	})
	if err != nil {
		return gen.Form{}, fmt.Errorf("assessment: create form: %w", err)
	}

	scoringConfig := []byte("{}")

	if in.ScoringConfig != nil {
		b, err := json.Marshal(in.ScoringConfig)
		if err != nil {
			return gen.Form{}, fmt.Errorf("assessment: marshal scoring_config: %w", err)
		}

		scoringConfig = b
	}

	if _, err := s.pool.Writer.Exec(ctx, `
		UPDATE forms
		SET form_type = 'assessment', scoring_config = $1
		WHERE id = $2
	`, scoringConfig, created.ID); err != nil {
		return gen.Form{}, fmt.Errorf("assessment: set assessment form metadata: %w", err)
	}

	return s.GetAssessmentFormByID(ctx, created.ID)
}

// UpdateAssessmentForm updates a forms row and its scoring_config. The form_type
// is preserved as 'assessment'.
func (s *Store) UpdateAssessmentForm(ctx context.Context, id pgtype.UUID, in UpdateAssessmentInput) (gen.Form, error) {
	existing, err := s.GetAssessmentFormByID(ctx, id)
	if err != nil {
		return gen.Form{}, fmt.Errorf("assessment: get form for update: %w", err)
	}

	name := existing.Name
	if in.Name != nil {
		name = *in.Name
	}

	description := existing.Description
	if in.Description != nil {
		description = in.Description
	}

	isMultiStep := existing.IsMultiStep
	if in.IsMultiStep != nil {
		isMultiStep = *in.IsMultiStep
	}

	submitLabel := existing.SubmitLabel
	if in.SubmitLabel != nil {
		submitLabel = *in.SubmitLabel
	}

	successMessage := existing.SuccessMessage
	if in.SuccessMessage != nil {
		successMessage = in.SuccessMessage
	}

	status := existing.Status
	if in.Status != nil {
		status = *in.Status
	}

	submissionFeeCredits := existing.SubmissionFeeCredits
	if in.SubmissionFeeCredits != nil {
		submissionFeeCredits = *in.SubmissionFeeCredits
	}

	updated, err := s.writeQueries.UpdateForm(ctx, gen.UpdateFormParams{
		Name:                 name,
		Description:          description,
		IsMultiStep:          isMultiStep,
		SubmitLabel:          submitLabel,
		SuccessMessage:       successMessage,
		Status:               status,
		SubmissionFeeCredits: submissionFeeCredits,
		ID:                   id,
	})
	if err != nil {
		return gen.Form{}, fmt.Errorf("assessment: update form: %w", err)
	}

	if in.Slug != nil {
		if _, err := s.pool.Writer.Exec(ctx, `
			UPDATE forms
			SET slug = $1
			WHERE id = $2 AND is_system = false
		`, *in.Slug, id); err != nil {
			return gen.Form{}, fmt.Errorf("assessment: update slug: %w", err)
		}
	}

	if in.ScoringConfig != nil {
		b, err := json.Marshal(in.ScoringConfig)
		if err != nil {
			return gen.Form{}, fmt.Errorf("assessment: marshal scoring_config: %w", err)
		}

		if _, err := s.pool.Writer.Exec(ctx, `
			UPDATE forms
			SET scoring_config = $1
			WHERE id = $2
		`, b, id); err != nil {
			return gen.Form{}, fmt.Errorf("assessment: update scoring_config: %w", err)
		}
	}

	return s.GetAssessmentFormByID(ctx, updated.ID)
}

// DeleteAssessmentForm soft-deletes a non-system assessment form by id.
func (s *Store) DeleteAssessmentForm(ctx context.Context, id pgtype.UUID) error {
	if err := s.writeQueries.DeleteForm(ctx, id); err != nil {
		return fmt.Errorf("assessment: delete form: %w", err)
	}

	return nil
}

// GetAssessmentFormByID returns a full forms row including form_type and
// scoring_config for the given platform and id.
func (s *Store) GetAssessmentFormByID(ctx context.Context, id pgtype.UUID) (gen.Form, error) {
	const q = `
SELECT id, name, slug, description, is_multi_step, submit_label,
       success_message, status, is_system, created_at, space_id, form_type,
       scoring_config, submission_fee_credits
FROM forms
WHERE id = $1
`

	var f gen.Form

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&f.ID, &f.Name, &f.Slug, &f.Description, &f.IsMultiStep,
		&f.SubmitLabel, &f.SuccessMessage, &f.Status, &f.IsSystem, &f.CreatedAt,
		&f.SpaceID, &f.FormType, &f.ScoringConfig, &f.SubmissionFeeCredits,
	)
	if err != nil {
		return gen.Form{}, fmt.Errorf("assessment: get assessment form by id: %w", err)
	}

	return f, nil
}

// GetSubmissionByID returns a form_submissions row by ID. It is used by the
// result roadmap endpoint to re-build the visitor's journey context from the
// submission's field_values without re-storing that context on the result row.
func (s *Store) GetSubmissionByID(ctx context.Context, submissionID pgtype.UUID) (gen.GetFormSubmissionByIDRow, error) {
	sub, err := s.queries.GetFormSubmissionByID(ctx,
		submissionID,
	)
	if err != nil {
		return gen.GetFormSubmissionByIDRow{}, fmt.Errorf("assessment: get submission by id: %w", err)
	}

	return sub, nil
}

// GetSpaceByGeoCode looks up the country-level space for an ISO country code.
// It is used by the scoring engine to resolve the destination country to a
// geoSpaceID for RAG grounding and feed ranking. A missing space returns a
// zero-value row and a nil error so the caller can fall back to global RAG.
func (s *Store) GetSpaceByGeoCode(ctx context.Context, geoCode string) (gen.GetSpaceByGeoCodeRow, error) {
	code := strings.ToUpper(strings.TrimSpace(geoCode))

	row, err := s.queries.GetSpaceByGeoCode(ctx,
		&code,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.GetSpaceByGeoCodeRow{}, nil
		}

		return gen.GetSpaceByGeoCodeRow{}, fmt.Errorf("assessment: get space by geo code: %w", err)
	}

	return row, nil
}

// FindRoadmapContentByDimension returns up to 100 published roadmap guide
// candidates for a scoring dimension and the visitor's journey context. It is
// used by the scoring worker to resolve a generic curated step to the most
// specific published content for the visitor's destination, stage, visa,
// source, and purpose. The caller scores and picks the best match. An empty
// result means no more specific content exists and the caller should keep the
// original step.
func (s *Store) FindRoadmapContentByDimension(ctx context.Context, dimension, destination, stage, visa, origin, purpose string) ([]gen.FindRoadmapContentByDimensionRow, error) {
	rows, err := s.queries.FindRoadmapContentByDimension(ctx, gen.FindRoadmapContentByDimensionParams{
		Dimension:          []byte(dimension),
		DestinationCountry: destination,
		JourneyStage:       stage,
		VisaRoute:          visa,
		OriginCountry:      origin,
		Purpose:            purpose,
		PageLimit:          100,
		PageOffset:         0,
	})
	if err != nil {
		return nil, fmt.Errorf("assessment: find roadmap content by dimension: %w", err)
	}

	return rows, nil
}

// strPtrOrNil returns a pointer to the string if it is non-empty, otherwise nil.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
