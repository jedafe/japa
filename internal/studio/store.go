// Package studio implements the AI Content Studio (Section 75, ADR-076).
// // Admin-facing persona-seeded content generation: an admin configures a
// persona + content type + topic brief, submits a job (or batch), and the
// system generates a draft via the AI gateway. All drafts enter the §22
// content_reviews queue. High-stakes guide categories always require human
// review; low-stakes types may auto-publish per studio.auto_publish_types.
// // Ref: japaconnect-engineering/sections/75-ai-content-studio.md
// //	japaconnect-engineering/adr/ADR-076-ai-content-studio.md
package studio

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store wraps the database pool for studio_queries.
type Store struct {
	db *db.DB
}

// NewStore creates a Store backed by the given database pool.
func NewStore(database *db.DB) *Store {
	return &Store{db: database}
}

// ErrJobNotCancellable is returned when CancelJob is called on a job that is
// not in 'queued' or 'failed' status.
var ErrJobNotCancellable = errors.New("studio: job is not in a cancellable status (only queued/failed can be cancelled)")

// Job is a studio_jobs row.
type Job struct {
	ID pgtype.UUID `json:"id"`

	BatchID        pgtype.UUID        `json:"batch_id"`
	PersonaID      pgtype.UUID        `json:"persona_id"`
	ContentType    string             `json:"content_type"`
	GuideCategory  *string            `json:"guide_category"`
	TopicPrompt    string             `json:"topic_prompt"`
	TargetSpaceID  pgtype.UUID        `json:"target_space_id"`
	TargetTags     []string           `json:"target_tags"`
	IntelRecordID  pgtype.UUID        `json:"intel_record_id"`
	AIModel        *string            `json:"ai_model"`
	VoiceID        pgtype.UUID        `json:"voice_id"`
	Status         string             `json:"status"`
	DraftContentID pgtype.UUID        `json:"draft_content_id"`
	ReviewID       pgtype.UUID        `json:"review_id"`
	GeneratedAt    pgtype.Timestamptz `json:"generated_at"`
	ReviewedAt     pgtype.Timestamptz `json:"reviewed_at"`
	PublishedAt    pgtype.Timestamptz `json:"published_at"`
	ErrorMessage   *string            `json:"error_message"`
	CreatedBy      pgtype.UUID        `json:"created_by"`
	CreatedAt      time.Time          `json:"created_at"`
}

// Batch is a studio_batches row.
type Batch struct {
	ID pgtype.UUID `json:"id"`

	Name        string      `json:"name"`
	Description *string     `json:"description"`
	CreatedBy   pgtype.UUID `json:"created_by"`
	CreatedAt   time.Time   `json:"created_at"`
}

// CreateJobInput is the input for creating a single studio job.
type CreateJobInput struct {
	PersonaID     pgtype.UUID `json:"persona_id"`
	ContentType   string      `json:"content_type"`
	GuideCategory *string     `json:"guide_category"`
	TopicPrompt   string      `json:"topic_prompt"`
	TargetSpaceID pgtype.UUID `json:"target_space_id"`
	TargetTags    []string    `json:"target_tags"`
	IntelRecordID pgtype.UUID `json:"intel_record_id"`
	AIModel       string      `json:"ai_model,omitempty"`
	VoiceID       pgtype.UUID `json:"voice_id,omitempty"`
}

// CreateBatchInput is the input for creating a batch with jobs.
type CreateBatchInput struct {
	Name        string           `json:"name"`
	Description *string          `json:"description"`
	Jobs        []CreateJobInput `json:"jobs"`
}

// JobListFilter holds the optional filters for listing jobs.
type JobListFilter struct {
	Status      *string
	PersonaID   *pgtype.UUID
	ContentType *string
	BatchID     *pgtype.UUID
	Limit       int32
	Offset      int32
}

// CreateJob inserts a single studio_jobs row.
func (s *Store) CreateJob(ctx context.Context, createdBy, batchID pgtype.UUID, in CreateJobInput) (Job, error) {
	var j Job
	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO studio_jobs (batch_id, persona_id, content_type, guide_category,
		                          topic_prompt, target_space_id, target_tags, intel_record_id,
		                          ai_model, voice_id, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, batch_id, persona_id, content_type, guide_category,
		          topic_prompt, target_space_id, target_tags, intel_record_id,
		          ai_model, voice_id, status, draft_content_id,
		          review_id, generated_at, reviewed_at, published_at, error_message,
		          created_by, created_at`, batchID, in.PersonaID, in.ContentType, in.GuideCategory, in.TopicPrompt, in.TargetSpaceID, in.TargetTags, in.IntelRecordID, in.AIModel, in.VoiceID, createdBy).Scan(
		&j.ID, &j.BatchID, &j.PersonaID, &j.ContentType, &j.GuideCategory,
		&j.TopicPrompt, &j.TargetSpaceID, &j.TargetTags, &j.IntelRecordID, &j.AIModel, &j.VoiceID, &j.Status, &j.DraftContentID,
		&j.ReviewID, &j.GeneratedAt, &j.ReviewedAt, &j.PublishedAt, &j.ErrorMessage,
		&j.CreatedBy, &j.CreatedAt,
	)

	return j, err
}

// CreateBatch inserts a studio_batches row and returns it.
func (s *Store) CreateBatch(ctx context.Context, createdBy pgtype.UUID, name string, description *string) (Batch, error) {
	var b Batch
	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO studio_batches (name, description, created_by) VALUES ($1, $2, $3)
		RETURNING id, name, description, created_by, created_at`, name, description, createdBy).Scan(&b.ID, &b.Name, &b.Description, &b.CreatedBy, &b.CreatedAt)

	return b, err
}

// GetJob fetches a single job by ID within a tenant.
func (s *Store) GetJob(ctx context.Context, id pgtype.UUID) (Job, error) {
	var j Job
	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, batch_id, persona_id, content_type, guide_category,
		       topic_prompt, target_space_id, target_tags, intel_record_id,
		       ai_model, voice_id, status, draft_content_id,
		       review_id, generated_at, reviewed_at, published_at, error_message,
		       created_by, created_at
		FROM studio_jobs WHERE id = $1`, id).Scan(
		&j.ID, &j.BatchID, &j.PersonaID, &j.ContentType, &j.GuideCategory,
		&j.TopicPrompt, &j.TargetSpaceID, &j.TargetTags, &j.IntelRecordID, &j.AIModel, &j.VoiceID, &j.Status, &j.DraftContentID,
		&j.ReviewID, &j.GeneratedAt, &j.ReviewedAt, &j.PublishedAt, &j.ErrorMessage,
		&j.CreatedBy, &j.CreatedAt,
	)

	return j, err
}

// ListJobs lists jobs with optional filters.
func (s *Store) ListJobs(ctx context.Context, tenantID pgtype.UUID, f JobListFilter) ([]Job, int64, error) {
	query := `SELECT id, batch_id, persona_id, content_type, guide_category,
	            topic_prompt, target_space_id, target_tags, intel_record_id,
	            ai_model, voice_id, status, draft_content_id,
	            review_id, generated_at, reviewed_at, published_at, error_message,
	            created_by, created_at
	         FROM studio_jobs `
	args := []interface{}{tenantID}

	argIdx := 2
	if f.Status != nil {
		query += ` AND status = $` + fmtArg(argIdx)

		args = append(args, *f.Status)
		argIdx++
	}

	if f.PersonaID != nil {
		query += ` AND persona_id = $` + fmtArg(argIdx)

		args = append(args, *f.PersonaID)
		argIdx++
	}

	if f.ContentType != nil {
		query += ` AND content_type = $` + fmtArg(argIdx)

		args = append(args, *f.ContentType)
		argIdx++
	}

	if f.BatchID != nil {
		query += ` AND batch_id = $` + fmtArg(argIdx)

		args = append(args, *f.BatchID)
		argIdx++
	}

	query += ` ORDER BY created_at DESC`
	if f.Limit > 0 {
		query += ` LIMIT $` + fmtArg(argIdx)

		args = append(args, f.Limit)
		argIdx++
	}

	if f.Offset > 0 {
		query += ` OFFSET $` + fmtArg(argIdx)

		args = append(args, f.Offset)
	}

	rows, err := s.db.Reader.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []Job

	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.BatchID, &j.PersonaID, &j.ContentType, &j.GuideCategory,
			&j.TopicPrompt, &j.TargetSpaceID, &j.TargetTags, &j.IntelRecordID, &j.AIModel, &j.VoiceID, &j.Status, &j.DraftContentID,
			&j.ReviewID, &j.GeneratedAt, &j.ReviewedAt, &j.PublishedAt, &j.ErrorMessage,
			&j.CreatedBy, &j.CreatedAt,
		); err != nil {
			return nil, 0, err
		}

		jobs = append(jobs, j)
	}

	// Count total (without limit/offset) for pagination.
	var total int64

	countArgs := []interface{}{tenantID}
	countQuery := `SELECT count(*) FROM studio_jobs `
	cIdx := 2

	if f.Status != nil {
		countQuery += ` AND status = $` + fmtArg(cIdx)

		countArgs = append(countArgs, *f.Status)
		cIdx++
	}

	if f.PersonaID != nil {
		countQuery += ` AND persona_id = $` + fmtArg(cIdx)

		countArgs = append(countArgs, *f.PersonaID)
		cIdx++
	}

	if f.ContentType != nil {
		countQuery += ` AND content_type = $` + fmtArg(cIdx)

		countArgs = append(countArgs, *f.ContentType)
		cIdx++
	}

	if f.BatchID != nil {
		countQuery += ` AND batch_id = $` + fmtArg(cIdx)

		countArgs = append(countArgs, *f.BatchID)
		cIdx++
	}

	_ = s.db.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total)

	return jobs, total, rows.Err()
}

// DeleteJob cancels a queued or failed job. Returns ErrJobNotCancellable if
// the job is in a status that cannot be cancelled (e.g. generating,
// published, in_review).
func (s *Store) DeleteJob(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `DELETE FROM studio_jobs WHERE id = $1 AND status IN ('queued','failed')`, id)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return ErrJobNotCancellable
	}

	return nil
}

// MarkJobFailedIfActive transitions a job to 'failed' only while it is in an
// active generation state ('queued' or 'generating'). A late failure from a
// redelivered or duplicate generation task must not regress a job that
// already produced a draft ('draft_ready'), entered review ('in_review'), or
// completed ('published'/'rejected') — those states own live contents and
// HITL review rows that must remain actionable. A no-op (0 rows) is not an
// error: the job already progressed past the point where the failure matters.
func (s *Store) MarkJobFailedIfActive(ctx context.Context, id pgtype.UUID, errMsg string) error {
	_, err := s.db.Writer.Exec(ctx, `UPDATE studio_jobs SET status = 'failed', error_message = $2
		 WHERE id = $1 AND status IN ('queued','generating')`, id, errMsg)

	return err
}

// UpdateJobStatus updates the status and optional error message.
func (s *Store) UpdateJobStatus(ctx context.Context, id pgtype.UUID, status string, errMsg *string) error {
	_, err := s.db.Writer.Exec(ctx, `UPDATE studio_jobs SET status = $2, error_message = $3 WHERE id = $1`, id, status, errMsg)

	return err
}

// ErrJobAlreadyGenerating is returned by MarkJobGeneratingCAS when the job is
// no longer in 'queued' status — another task already picked it up. This is
// the compare-and-swap guard that prevents duplicate draft generation when two
// content:generate_draft tasks for the same job_id are processed concurrently
// (ADR-076 §1 job idempotency).
var ErrJobAlreadyGenerating = errors.New("studio: job already picked up by another worker")

// MarkJobGeneratingCAS atomically transitions a job from 'queued' to
// 'generating' using a compare-and-swap UPDATE. Returns ErrJobAlreadyGenerating
// if 0 rows were affected (the job was no longer 'queued' — another worker
// already claimed it). This prevents the race condition where two concurrent
// content:generate_draft tasks both pass the status guard in GenerateJob and
// both proceed to generate duplicate drafts.
func (s *Store) MarkJobGeneratingCAS(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `UPDATE studio_jobs SET status = 'generating' WHERE id = $1 AND status = 'queued'`, id)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return ErrJobAlreadyGenerating
	}

	return nil
}

// UpdateJobGenerated sets the draft_content_id and status to 'draft_ready'
// after the AI gateway has produced the draft content (ADR-076 §2 step 7).
// The job remains in 'draft_ready' until it is routed to the review queue
// (UpdateJobRoutedToReview) or auto-published (UpdateJobPublished).
// Sets ai_generated_at (ADR-088) alongside generated_at.
func (s *Store) UpdateJobGenerated(ctx context.Context, id, draftContentID pgtype.UUID, aiModel string) error {
	_, err := s.db.Writer.Exec(ctx, `UPDATE studio_jobs SET draft_content_id = $2, status = 'draft_ready',
		          generated_at = now(), ai_generated_at = now(), ai_model = NULLIF($3, '')
		  WHERE id = $1`, id, draftContentID, aiModel)

	return err
}

// UpdateJobRoutedToReview sets the review_id and status to 'in_review' after
// the draft has been routed to the content_reviews HITL queue (ADR-076 §2
// step 8). Called after UpdateJobGenerated once the review row exists.
func (s *Store) UpdateJobRoutedToReview(ctx context.Context, id, reviewID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `UPDATE studio_jobs SET review_id = $2, status = 'in_review'
		  WHERE id = $1`, id, reviewID)

	return err
}

// UpdateJobPublished sets the draft_content_id, status, and timestamps for a
// job whose draft was auto-published (studio.auto_publish_types, §75). No
// review_id is written because auto-published types skip the HITL queue.
// Sets ai_generated_at (ADR-088) alongside generated_at.
func (s *Store) UpdateJobPublished(ctx context.Context, id, draftContentID pgtype.UUID, aiModel string) error {
	_, err := s.db.Writer.Exec(ctx, `UPDATE studio_jobs SET draft_content_id = $2, status = 'published',
		          generated_at = now(), ai_generated_at = now(), ai_model = NULLIF($3, ''), published_at = now()
		  WHERE id = $1`, id, draftContentID, aiModel)

	return err
}

// ListBatches lists batches for a tenant.
func (s *Store) ListBatches(ctx context.Context, limit, offset int32) ([]Batch, int64, error) {
	rows, err := s.db.Reader.Query(ctx, `SELECT id, name, description, created_by, created_at
		 FROM studio_batches  ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var batches []Batch

	for rows.Next() {
		var b Batch
		if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.CreatedBy, &b.CreatedAt); err != nil {
			return nil, 0, err
		}

		batches = append(batches, b)
	}

	var total int64
	_ = s.db.Reader.QueryRow(ctx, `SELECT count(*) FROM studio_batches `).Scan(&total)

	return batches, total, rows.Err()
}

// GetBatch fetches a single batch by ID within a tenant.
func (s *Store) GetBatch(ctx context.Context, id pgtype.UUID) (Batch, error) {
	var b Batch
	err := s.db.Reader.QueryRow(ctx, `SELECT id, name, description, created_by, created_at
		 FROM studio_batches WHERE id = $1`, id).Scan(&b.ID, &b.Name, &b.Description, &b.CreatedBy, &b.CreatedAt)

	return b, err
}

// BatchStats holds progress stats for a batch.
type BatchStats struct {
	Total      int `json:"total"`
	Queued     int `json:"queued"`
	Generating int `json:"generating"`
	DraftReady int `json:"draft_ready"`
	InReview   int `json:"in_review"`
	Approved   int `json:"approved"`
	Published  int `json:"published"`
	Rejected   int `json:"rejected"`
	Failed     int `json:"failed"`
}

// GetBatchStats returns progress stats for a batch.
func (s *Store) GetBatchStats(ctx context.Context, batchID pgtype.UUID) (BatchStats, error) {
	var st BatchStats

	rows, err := s.db.Reader.Query(ctx, `SELECT status, count(*) FROM studio_jobs WHERE batch_id = $1 GROUP BY status`, batchID)
	if err != nil {
		return st, err
	}

	defer rows.Close()

	for rows.Next() {
		var status string

		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return st, err
		}

		st.Total += count

		switch status {
		case "queued":
			st.Queued = count
		case "generating":
			st.Generating = count
		case "draft_ready":
			st.DraftReady = count
		case "in_review":
			st.InReview = count
		case "approved":
			st.Approved = count
		case "published":
			st.Published = count
		case "rejected":
			st.Rejected = count
		case "failed":
			st.Failed = count
		}
	}

	return st, rows.Err()
}

// ListFailedJobs returns all failed jobs for a batch.
func (s *Store) ListFailedJobs(ctx context.Context, batchID pgtype.UUID) ([]Job, error) {
	rows, err := s.db.Reader.Query(ctx, `SELECT id, batch_id, persona_id, content_type, guide_category,
		        topic_prompt, target_space_id, target_tags, intel_record_id, status, draft_content_id,
		        review_id, generated_at, reviewed_at, published_at, error_message,
		        created_by, created_at
		 FROM studio_jobs WHERE batch_id = $1 AND status = 'failed'`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job

	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.BatchID, &j.PersonaID, &j.ContentType, &j.GuideCategory,
			&j.TopicPrompt, &j.TargetSpaceID, &j.TargetTags, &j.IntelRecordID, &j.Status, &j.DraftContentID,
			&j.ReviewID, &j.GeneratedAt, &j.ReviewedAt, &j.PublishedAt, &j.ErrorMessage,
			&j.CreatedBy, &j.CreatedAt,
		); err != nil {
			return nil, err
		}

		jobs = append(jobs, j)
	}

	return jobs, rows.Err()
}

// StudioStats holds aggregate stats for the studio dashboard. The status
// breakdown fields (Queued/Generating/...) mirror the statuses the admin
// frontend renders on the Stats tab; TotalJobs is the overall count and
// PublishRate / AvgReviewH are the aggregate quality metrics.
// //nolint:revive // exported: stutter naming is intentional
type StudioStats struct {
	TotalJobs   int64   `json:"total_jobs"`
	Queued      int64   `json:"queued"`
	Generating  int64   `json:"generating"`
	DraftReady  int64   `json:"draft_ready"`
	InReview    int64   `json:"in_review"`
	Published   int64   `json:"published"`
	Rejected    int64   `json:"rejected"`
	Failed      int64   `json:"failed"`
	PublishRate float64 `json:"publish_rate"`
	AvgReviewH  float64 `json:"avg_review_hours"`
}

// GetStats returns aggregate studio stats for a tenant.
func (s *Store) GetStats(ctx context.Context) (StudioStats, error) {
	var st StudioStats
	err := s.db.Reader.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'queued'),
		       count(*) FILTER (WHERE status = 'generating'),
		       count(*) FILTER (WHERE status = 'draft_ready'),
		       count(*) FILTER (WHERE status = 'in_review'),
		       count(*) FILTER (WHERE status = 'published'),
		       count(*) FILTER (WHERE status = 'rejected'),
		       count(*) FILTER (WHERE status = 'failed'),
		       COALESCE(
		         count(*) FILTER (WHERE status = 'published')::float /
		         NULLIF(count(*) FILTER (WHERE status IN ('published','rejected','failed')), 0),
		         0
		       ),
		       COALESCE(
		         avg(EXTRACT(EPOCH FROM (reviewed_at - generated_at)) / 3600)
		           FILTER (WHERE reviewed_at IS NOT NULL AND generated_at IS NOT NULL),
		         0
		       )
		FROM studio_jobs `).Scan(&st.TotalJobs, &st.Queued, &st.Generating, &st.DraftReady, &st.InReview,
		&st.Published, &st.Rejected, &st.Failed, &st.PublishRate, &st.AvgReviewH)

	return st, err
}

// CountActiveJobs returns the number of jobs in 'queued' or 'generating' status
// for a tenant. Used to enforce studio.max_concurrent_jobs (ADR-076, §75).
func (s *Store) CountActiveJobs(ctx context.Context) (int, error) {
	var count int
	err := s.db.Reader.QueryRow(ctx, `SELECT count(*) FROM studio_jobs WHERE status IN ('queued','generating')`).Scan(&count)

	return count, err
}

// GetJobByReviewID fetches the studio job linked to a content_reviews row
// by its review_id. Used by the moderation hook to transition studio_jobs
// status when a source='studio' HITL review is approved/rejected (§75,
// ADR-076 §3).
func (s *Store) GetJobByReviewID(ctx context.Context, reviewID pgtype.UUID) (Job, error) {
	var j Job
	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, batch_id, persona_id, content_type, guide_category,
		       topic_prompt, target_space_id, target_tags, intel_record_id,
		       ai_model, voice_id, status, draft_content_id,
		       review_id, generated_at, reviewed_at, published_at, error_message,
		       created_by, created_at
		FROM studio_jobs WHERE review_id = $1`, reviewID).Scan(
		&j.ID, &j.BatchID, &j.PersonaID, &j.ContentType, &j.GuideCategory,
		&j.TopicPrompt, &j.TargetSpaceID, &j.TargetTags, &j.IntelRecordID, &j.AIModel, &j.VoiceID, &j.Status, &j.DraftContentID,
		&j.ReviewID, &j.GeneratedAt, &j.ReviewedAt, &j.PublishedAt, &j.ErrorMessage,
		&j.CreatedBy, &j.CreatedAt,
	)

	return j, err
}

// UpdateJobReviewed sets the status and reviewed_at timestamp for a job.
// Used by the moderation hook to transition a job to 'published' (approve)
// or 'rejected' (reject) after HITL review (§75, ADR-076 §3).
func (s *Store) UpdateJobReviewed(ctx context.Context, id pgtype.UUID, status string) error {
	_, err := s.db.Writer.Exec(ctx, `UPDATE studio_jobs SET status = $2, reviewed_at = now(),
		          published_at = CASE WHEN $2 = 'published' THEN now() ELSE published_at END
		  WHERE id = $1`, id, status)

	return err
}

// fmtArg converts an int to a string for parameterized query building.
// Uses strconv.Itoa for correct conversion at any argIdx value.
func fmtArg(i int) string {
	return strconv.Itoa(i)
}
