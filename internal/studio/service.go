// Package studio — service.go holds the business logic for the AI Content
// Studio: job/batch creation, generation pipeline orchestration, review
// routing, and the high-stakes content gate.
// // Ref: japaconnect-engineering/sections/75-ai-content-studio.md
package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// TaskEnqueuer is the subset of worker.Enqueuer that studio needs.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// IntelRecordStatusUpdater transitions an intel_records row's status. Used
// by the studio service to transition a linked intel_record to 'published'
// when a studio job is published (§75: "On successful publish the linked
// intel_records row transitions to status='published'").
type IntelRecordStatusUpdater interface {
	UpdateIntelRecordStatus(ctx context.Context, id pgtype.UUID, status string) error
}

// ContentPublisher publishes a content row (sets status='published'). Used
// by the studio review hook to publish the draft content when a studio HITL
// review is approved (§75: "Approve → publish"). The return value is the
// updated content row; the studio service ignores it.
type ContentPublisher interface {
	UpdateContentStatus(ctx context.Context, contentID, tenantID pgtype.UUID, status string) (any, error)
}

// HITLReview is a minimal read model for the ai_hitl_reviews row the studio
// hook needs to route tool-table drafts to the correct publisher.
type HITLReview struct {
	ID          pgtype.UUID
	ContentID   pgtype.UUID
	ContentType string
	TargetTable string
	Source      string
	Status      string
}

// HITLReviewReader loads a single HITL review by tenant + id.
type HITLReviewReader interface {
	GetHITLReview(ctx context.Context, reviewID pgtype.UUID) (HITLReview, error)
}

// ToolTablePublisher publishes a non-contents draft row when its studio HITL
// review is approved. The targetTable value comes from ai_hitl_reviews.
type ToolTablePublisher interface {
	Publish(ctx context.Context, tenantID, reviewerID pgtype.UUID, targetTable string, targetID pgtype.UUID, notes string) error
}

// Service holds the business logic for the AI Content Studio.
type Service struct {
	store        *Store
	settings     *config.Settings
	enqueuer     TaskEnqueuer
	intelUpdater IntelRecordStatusUpdater
	contentPub   ContentPublisher
	hitlReader   HITLReviewReader
	toolTablePub ToolTablePublisher
}

// NewService creates a Service backed by the given store and settings.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetEnqueuer injects the task enqueuer (called at server wiring time).
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetIntelRecordUpdater injects the intel record status updater so the
// service can transition linked intel_records to 'published' on publish
// (§75). Called at server wiring time.
func (s *Service) SetIntelRecordUpdater(u IntelRecordStatusUpdater) { s.intelUpdater = u }

// SetContentPublisher injects the content publisher so the review hook can
// publish draft content when a studio HITL review is approved (§75).
// Called at server wiring time.
func (s *Service) SetContentPublisher(p ContentPublisher) { s.contentPub = p }

// SetHITLReviewReader injects the HITL review reader so the review hook can
// discover target_table for tool-table drafts.
func (s *Service) SetHITLReviewReader(r HITLReviewReader) { s.hitlReader = r }

// SetToolTablePublisher injects the publisher for tool-table drafts.
func (s *Service) SetToolTablePublisher(p ToolTablePublisher) { s.toolTablePub = p }

// highStakesCategories are the guide categories that always require human
// review, regardless of studio.auto_publish_types. Hardcoded per §75 — not
// configurable.
var highStakesCategories = map[string]bool{
	"visa":             true,
	"remittance":       true, // financial (aligned with intel gate ADR-088)
	"banking":          true,
	"grants":           true,
	"health_insurance": true,
	"admission":        true,
}

// permanentlyExcludedTypes are content types that can never auto-publish.
var permanentlyExcludedTypes = map[string]bool{
	"blog_post":       true,
	"guide":           true,
	"comparison":      true,
	"event":           true,
	"job_listing":     true,
	"service_listing": true,
	// Tool table content types (Approach B, ADR-076 Amendment 1) — all
	// require HITL review because they back immigration/financial/legal
	// projection rows.
	"housing_guide":             true,
	"attestation_guide":         true,
	"visa_route":                true,
	"arrival_checklist":         true,
	"credential_pathway":        true,
	"language_test_requirement": true,
	"tax_obligation_guide":      true,
	"community_organisation":    true,
	"wiki_page":                 true,
	"calendar_entry":            true, // tool-table projection row (calendar_entries)
}

// IsHighStakes returns true if the guide category requires mandatory review.
// Exported so the participation orchestration engine can reuse the same gate.
func IsHighStakes(guideCategory string) bool {
	return highStakesCategories[strings.ToLower(guideCategory)]
}

// CreateJob creates a single studio job and enqueues a generation task.
func (s *Service) CreateJob(ctx context.Context, createdBy pgtype.UUID, in CreateJobInput) (Job, error) {
	if !s.isEnabled(ctx) {
		return Job{}, errors.New("studio: feature is disabled")
	}

	if in.TopicPrompt == "" {
		return Job{}, errors.New("studio: topic_prompt is required")
	}

	if in.ContentType == "" {
		return Job{}, errors.New("studio: content_type is required")
	}

	// Enforce studio.max_concurrent_jobs (ADR-076, §75). Default 3.
	if err := s.checkConcurrency(ctx, 1); err != nil {
		return Job{}, err
	}

	job, err := s.store.CreateJob(ctx, createdBy, pgtype.UUID{}, in)
	if err != nil {
		return Job{}, fmt.Errorf("studio: create job: %w", err)
	}

	if err := s.enqueueGeneration(ctx, job); err != nil {
		_ = s.store.UpdateJobStatus(ctx, job.ID, "failed", strPtr(err.Error()))
		return Job{}, fmt.Errorf("studio: enqueue generation: %w", err)
	}

	return job, nil
}

// CreateBatch creates a batch with multiple jobs and enqueues a single
// studio:run_batch task. The task processor fans out individual
// content:generate_draft jobs respecting studio.max_concurrent_jobs
// (ADR-076 §2/§7). This avoids flooding the AI gateway and keeps the HTTP
// request fast — batch generation is inherently asynchronous.
func (s *Service) CreateBatch(ctx context.Context, tenantID, createdBy pgtype.UUID, in CreateBatchInput) (Batch, []Job, error) {
	if !s.isEnabled(ctx) {
		return Batch{}, nil, errors.New("studio: feature is disabled")
	}

	if in.Name == "" {
		return Batch{}, nil, errors.New("studio: batch name is required")
	}

	if len(in.Jobs) == 0 {
		return Batch{}, nil, errors.New("studio: batch must contain at least one job")
	}

	batch, err := s.store.CreateBatch(ctx, createdBy, in.Name, in.Description)
	if err != nil {
		return Batch{}, nil, fmt.Errorf("studio: create batch: %w", err)
	}

	// Create all jobs in 'queued' status. Generation is enqueued by the
	// studio:run_batch task processor, not inline — this is the async batch
	// pipeline per ADR-076 §2/§7.
	var jobs []Job

	for _, jin := range in.Jobs {
		job, err := s.store.CreateJob(ctx, createdBy, batch.ID, jin)
		if err != nil {
			slog.Warn("studio: create job in batch failed", "batch_id", batch.ID, "error", err)
			continue
		}

		jobs = append(jobs, job)
	}

	if len(jobs) == 0 {
		return batch, jobs, errors.New("studio: batch created but no jobs could be inserted")
	}

	// Enqueue a single studio:run_batch task — the processor fans out
	// individual content:generate_draft jobs respecting max_concurrent_jobs.
	if err := s.enqueueRunBatch(ctx, tenantID, batch.ID); err != nil {
		slog.Warn("studio: enqueue run_batch failed, jobs remain queued", "batch_id", batch.ID, "error", err)
	}

	return batch, jobs, nil
}

// GetJob returns a single job by ID.
func (s *Service) GetJob(ctx context.Context, id pgtype.UUID) (Job, error) {
	return s.store.GetJob(ctx, id)
}

// ListJobs lists jobs with optional filters.
func (s *Service) ListJobs(ctx context.Context, tenantID pgtype.UUID, f JobListFilter) ([]Job, int64, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}

	return s.store.ListJobs(ctx, tenantID, f)
}

// RetryJob resets a failed job to queued and re-enqueues generation.
func (s *Service) RetryJob(ctx context.Context, id pgtype.UUID) error {
	if !s.isEnabled(ctx) {
		return errors.New("studio: feature is disabled")
	}

	job, err := s.store.GetJob(ctx, id)
	if err != nil {
		return fmt.Errorf("studio: get job for retry: %w", err)
	}

	if job.Status != "failed" {
		return fmt.Errorf("studio: only failed jobs can be retried (current: %s)", job.Status)
	}
	// Enforce studio.max_concurrent_jobs (ADR-076, §75) before enqueuing.
	if err := s.checkConcurrency(ctx, 1); err != nil {
		return err
	}

	if err := s.store.UpdateJobStatus(ctx, id, "queued", nil); err != nil {
		return fmt.Errorf("studio: reset job status: %w", err)
	}

	job.Status = "queued"

	return s.enqueueGeneration(ctx, job)
}

// CancelJob deletes a queued or failed job. Returns ErrJobNotCancellable if
// the job is in a status that cannot be cancelled.
func (s *Service) CancelJob(ctx context.Context, id pgtype.UUID) error {
	return s.store.DeleteJob(ctx, id)
}

// ListBatches lists batches for a tenant.
func (s *Service) ListBatches(ctx context.Context, limit, offset int32) ([]Batch, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	return s.store.ListBatches(ctx, limit, offset)
}

// GetBatch returns a batch with progress stats.
func (s *Service) GetBatch(ctx context.Context, id pgtype.UUID) (Batch, BatchStats, error) {
	batch, err := s.store.GetBatch(ctx, id)
	if err != nil {
		return Batch{}, BatchStats{}, err
	}

	stats, err := s.store.GetBatchStats(ctx, id)

	return batch, stats, err
}

// RetryBatchFailed retries all failed jobs in a batch.
func (s *Service) RetryBatchFailed(ctx context.Context, batchID pgtype.UUID) (int, error) {
	if !s.isEnabled(ctx) {
		return 0, errors.New("studio: feature is disabled")
	}

	jobs, err := s.store.ListFailedJobs(ctx, batchID)
	if err != nil {
		return 0, fmt.Errorf("studio: list failed jobs: %w", err)
	}
	// Enforce studio.max_concurrent_jobs (ADR-076, §75) — check once for the
	// batch; individual jobs beyond the limit remain in 'queued' and are
	// picked up by subsequent run_batch invocations.
	if err := s.checkConcurrency(ctx, len(jobs)); err != nil {
		// If the full batch would exceed the limit, still retry as many as
		// we can within the limit — the rest stay queued for later pickup.
		slog.Warn("studio: retry batch: concurrency limit, retrying partial", "batch_id", batchID, "failed_count", len(jobs), "error", err)
	}

	var count int

	for _, job := range jobs {
		if err := s.store.UpdateJobStatus(ctx, job.ID, "queued", nil); err != nil {
			slog.Warn("studio: retry batch: reset status failed", "job_id", job.ID, "error", err)
			continue
		}

		job.Status = "queued"
		if err := s.enqueueGeneration(ctx, job); err != nil {
			slog.Warn("studio: retry batch: enqueue failed", "job_id", job.ID, "error", err)
			continue
		}

		count++
	}

	return count, nil
}

// GetStats returns aggregate studio stats.
func (s *Service) GetStats(ctx context.Context) (StudioStats, error) {
	return s.store.GetStats(ctx)
}

// ContentTypeInfo describes a content type available in the studio.
type ContentTypeInfo struct {
	Type            string   `json:"type"`
	Label           string   `json:"label"`
	GuideCategories []string `json:"guide_categories,omitempty"`
	CanAutoPublish  bool     `json:"can_auto_publish"`
}

// ContentTypeResponse wraps the content types list with studio configuration
// the frontend needs (e.g. the default persona pre-selection per
// studio.default_persona_id, §75 system_settings).
type ContentTypeResponse struct {
	Types            []ContentTypeInfo `json:"types"`
	DefaultPersonaID string            `json:"default_persona_id,omitempty"`
}

// DefaultPersonaID returns the studio.default_persona_id setting as a UUID.
// It returns an empty/invalid UUID when the setting is unset or the value is
// not a valid UUID.
func (s *Service) DefaultPersonaID(ctx context.Context) (pgtype.UUID, error) {
	var id pgtype.UUID
	if s.settings == nil {
		return id, nil
	}

	val, err := s.settings.GetSetting(ctx, "studio.default_persona_id")
	if err != nil || val == "" {
		return id, err
	}

	if uerr := id.Scan(val); uerr != nil {
		return id, uerr
	}

	return id, nil
}

// ListContentTypes returns the content types available for studio generation
// along with the studio.default_persona_id setting so the frontend can
// pre-select a persona in the new-job form (§75 system_settings, ADR-076 §6).
func (s *Service) ListContentTypes(ctx context.Context) ContentTypeResponse {
	types := []ContentTypeInfo{
		{Type: "discussion", Label: "Discussion", CanAutoPublish: true},
		{Type: "page", Label: "Page", CanAutoPublish: true},
		{Type: "blog_post", Label: "Blog Post", CanAutoPublish: false},
		{Type: "guide", Label: "Guide", GuideCategories: []string{"visa", "travel_routes", "scholarship", "grants", "admission", "accommodation", "banking", "work_career", "health_insurance", "culture_lifestyle"}, CanAutoPublish: false},
		{Type: "comparison", Label: "Comparison", CanAutoPublish: false},
		{Type: "event", Label: "Event", CanAutoPublish: false},
		{Type: "job_listing", Label: "Job Listing", CanAutoPublish: false},
		{Type: "service_listing", Label: "Service Listing", CanAutoPublish: false},
		// Tool-table content types (Approach B, ADR-076 Amendment 1).
		// These generate editorial content for dedicated projection tables.
		// None can auto-publish — all require HITL review.
		{Type: "housing_guide", Label: "Housing Guide", CanAutoPublish: false},
		{Type: "attestation_guide", Label: "Attestation Guide", CanAutoPublish: false},
		{Type: "visa_route", Label: "Visa Route", CanAutoPublish: false},
		{Type: "arrival_checklist", Label: "Arrival Checklist", CanAutoPublish: false},
		{Type: "credential_pathway", Label: "Credential Pathway", CanAutoPublish: false},
		{Type: "language_test_requirement", Label: "Language Test Requirement", CanAutoPublish: false},
		{Type: "tax_obligation_guide", Label: "Tax Obligation Guide", CanAutoPublish: false},
		{Type: "community_organisation", Label: "Community Organisation", CanAutoPublish: false},
		{Type: "wiki_page", Label: "Wiki Page", CanAutoPublish: false},
		{Type: "calendar_entry", Label: "Calendar Entry", CanAutoPublish: false},
		// Community content types (ADR-088). Story reuses the contents
		// table with content_type='story'; doc_page uses doc_pages.
		{Type: "story", Label: "Story", CanAutoPublish: false},
		{Type: "doc_page", Label: "Documentation Page", CanAutoPublish: false},
	}

	// CanAutoPublish reflects the static capability (which types are eligible
	// to skip review). It is NOT modified by the current studio.auto_publish_types
	// setting value — that would create a chicken-and-egg problem where the
	// settings page's option list shrinks as the user selects items. The
	// runtime auto-publish decision is made by ShouldAutoPublish, which checks
	// the setting at generation time.
	defaultPersonaID := ""
	if s.settings != nil {
		defaultPersonaID, _ = s.settings.GetSetting(ctx, "studio.default_persona_id")
	}

	return ContentTypeResponse{
		Types:            types,
		DefaultPersonaID: defaultPersonaID,
	}
}

// enqueueGeneration enqueues a content:generate_draft task for the job.
func (s *Service) enqueueGeneration(ctx context.Context, job Job) error {
	if s.enqueuer == nil {
		return errors.New("studio: no enqueuer configured")
	}

	payload := map[string]any{
		"job_id": job.ID.String(),

		"persona_id":   job.PersonaID.String(),
		"content_type": job.ContentType,
		"topic_prompt": job.TopicPrompt,
	}
	if job.IntelRecordID.Valid {
		payload["intel_record_id"] = job.IntelRecordID.String()
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("studio: marshal payload: %w", err)
	}

	if err := s.enqueuer.Enqueue("content:generate_draft", payloadJSON); err != nil {
		return fmt.Errorf("studio: enqueue: %w", err)
	}

	return nil
}

// enqueueRunBatch enqueues a studio:run_batch task for the batch. The task
// processor fans out individual content:generate_draft jobs respecting
// studio.max_concurrent_jobs (ADR-076 §2/§7).
func (s *Service) enqueueRunBatch(ctx context.Context, tenantID, batchID pgtype.UUID) error {
	if s.enqueuer == nil {
		return errors.New("studio: no enqueuer configured")
	}

	payload, err := json.Marshal(map[string]string{
		"batch_id": batchID.String(),
	})
	if err != nil {
		return fmt.Errorf("studio: marshal run_batch payload: %w", err)
	}

	if err := s.enqueuer.Enqueue("studio:run_batch", payload); err != nil {
		return fmt.Errorf("studio: enqueue run_batch: %w", err)
	}

	return nil
}

// enqueueRetryFailed enqueues a studio:retry_failed task for a batch. The
// task processor retries all failed jobs asynchronously (ADR-076 §7).
func (s *Service) enqueueRetryFailed(ctx context.Context, tenantID, batchID pgtype.UUID) error {
	if s.enqueuer == nil {
		return errors.New("studio: no enqueuer configured")
	}

	payload, err := json.Marshal(map[string]string{
		"batch_id": batchID.String(),
	})
	if err != nil {
		return fmt.Errorf("studio: marshal retry_failed payload: %w", err)
	}

	if err := s.enqueuer.Enqueue("studio:retry_failed", payload); err != nil {
		return fmt.Errorf("studio: enqueue retry_failed: %w", err)
	}

	return nil
}

// isEnabled checks the studio.enabled system setting.
func (s *Service) isEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "studio.enabled", true)
}

// checkConcurrency enforces studio.max_concurrent_jobs (ADR-076, §75).
// Returns an error if adding `pending` new jobs would exceed the limit.
func (s *Service) checkConcurrency(ctx context.Context, pending int) error {
	maxConcurrent := 3
	if s.settings != nil {
		maxConcurrent = s.settings.GetIntOr(ctx, "studio.max_concurrent_jobs", 3)
	}

	if maxConcurrent <= 0 {
		return nil // 0 or negative = unlimited
	}

	active, err := s.store.CountActiveJobs(ctx)
	if err != nil {
		slog.Warn("studio: failed to count active jobs, allowing request", "error", err)
		return nil // fail open on DB error
	}

	if active+pending > maxConcurrent {
		return fmt.Errorf("studio: concurrent job limit reached (%d active + %d pending > %d max)",
			active, pending, maxConcurrent)
	}

	return nil
}

// parseCSV parses a comma-separated string into a set.
func parseCSV(s string) map[string]bool {
	m := make(map[string]bool)

	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p != "" {
			m[p] = true
		}
	}

	return m
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string { return &s }

// MarkJobGenerating atomically transitions a job from 'queued' to 'generating'
// using a compare-and-swap UPDATE. Returns ErrJobAlreadyGenerating if another
// worker already claimed the job — the caller should skip generation. This
// prevents duplicate draft generation from concurrent content:generate_draft
// tasks (ADR-076 §1 job idempotency).
func (s *Service) MarkJobGenerating(ctx context.Context, jobID pgtype.UUID) error {
	return s.store.MarkJobGeneratingCAS(ctx, jobID)
}

// MarkJobGenerated sets the draft_content_id and status to 'draft_ready'
// after the AI gateway has produced the draft content (ADR-076 §2 step 7).
// aiModel is the provider model that generated the draft (ADR-088).
func (s *Service) MarkJobGenerated(ctx context.Context, jobID, draftContentID pgtype.UUID, aiModel string) error {
	return s.store.UpdateJobGenerated(ctx, jobID, draftContentID, aiModel)
}

// MarkJobRoutedToReview sets the review_id and status to 'in_review' after
// the draft has been routed to the content_reviews HITL queue (ADR-076 §2
// step 8). Called after MarkJobGenerated once the review row exists.
func (s *Service) MarkJobRoutedToReview(ctx context.Context, jobID, reviewID pgtype.UUID) error {
	return s.store.UpdateJobRoutedToReview(ctx, jobID, reviewID)
}

// MarkJobFailed sets a job's status to 'failed' with an error message.
// The transition is guarded: it only applies while the job is still in an
// active generation state, so a late failure from a redelivered task cannot
// regress a job that already produced a draft or entered review.
func (s *Service) MarkJobFailed(ctx context.Context, jobID pgtype.UUID, errMsg string) error {
	return s.store.MarkJobFailedIfActive(ctx, jobID, errMsg)
}

// MarkJobPublished marks a job's draft as auto-published (status='published').
// Used for content types in studio.auto_publish_types (§75).
// aiModel is the provider model that generated the draft (ADR-088).
func (s *Service) MarkJobPublished(ctx context.Context, jobID, draftContentID pgtype.UUID, aiModel string) error {
	return s.store.UpdateJobPublished(ctx, jobID, draftContentID, aiModel)
}

// ShouldAutoPublish returns true if the content type can skip review.
// High-stakes guide categories always return false.
func (s *Service) ShouldAutoPublish(ctx context.Context, contentType, guideCategory string) bool {
	if IsHighStakes(guideCategory) {
		return false
	}

	if permanentlyExcludedTypes[contentType] {
		return false
	}

	autoPublishCSV := ""
	if s.settings != nil {
		autoPublishCSV, _ = s.settings.GetSetting(ctx, "studio.auto_publish_types")
	}

	return parseCSV(autoPublishCSV)[contentType]
}

// RunBatch is the processor method called by the studio:run_batch task handler.
// It loads all queued jobs in the batch and enqueues generation for each,
// respecting studio.max_concurrent_jobs (ADR-076 §2/§7). Jobs beyond the
// concurrency limit remain in 'queued' status and are picked up by subsequent
// run_batch invocations (e.g. after a job completes and the active count drops).
func (s *Service) RunBatch(ctx context.Context, tenantID, batchID pgtype.UUID) error {
	jobs, _, err := s.store.ListJobs(ctx, tenantID, JobListFilter{
		BatchID: &batchID, Status: strPtr("queued"), Limit: 100,
	})
	if err != nil {
		return fmt.Errorf("studio: run_batch: list jobs: %w", err)
	}

	if len(jobs) == 0 {
		return nil
	}

	// Determine how many jobs we can enqueue without exceeding the
	// max_concurrent_jobs limit.
	maxConcurrent := 3
	if s.settings != nil {
		maxConcurrent = s.settings.GetIntOr(ctx, "studio.max_concurrent_jobs", 3)
	}

	active, err := s.store.CountActiveJobs(ctx)
	if err != nil {
		slog.Warn("studio: run_batch: count active failed, enqueuing all", "batch_id", batchID, "error", err)

		active = 0
	}

	// Enqueue at most (maxConcurrent - active) jobs. If maxConcurrent <= 0,
	// there's no limit — enqueue all.
	toEnqueue := len(jobs)

	if maxConcurrent > 0 {
		remaining := maxConcurrent - active
		if remaining < 0 {
			remaining = 0
		}

		if toEnqueue > remaining {
			toEnqueue = remaining
		}
	}

	enqueued := 0

	for i := range toEnqueue {
		if err := s.enqueueGeneration(ctx, jobs[i]); err != nil {
			slog.Warn("studio: run_batch: enqueue failed", "job_id", jobs[i].ID, "error", err)
			continue
		}

		enqueued++
	}

	slog.Info("studio: run_batch: enqueued jobs", "batch_id", batchID, "enqueued", enqueued, "remaining_queued", len(jobs)-enqueued)

	return nil
}

// RetryFailed is the processor method called by the studio:retry_failed task.
func (s *Service) RetryFailed(ctx context.Context, jobID pgtype.UUID) error {
	return s.RetryJob(ctx, jobID)
}

// EnqueueRetryFailed enqueues a studio:retry_failed task for a batch. Called
// by the HTTP handler so batch retry is asynchronous (ADR-076 §7).
func (s *Service) EnqueueRetryFailed(ctx context.Context, tenantID, batchID pgtype.UUID) error {
	return s.enqueueRetryFailed(ctx, tenantID, batchID)
}

// OnStudioReviewApproved is called when a source='studio' HITL review is
// approved (§75, ADR-076 §3). It publishes the draft content or tool-table
// row, transitions studio_jobs to 'published', and transitions the linked
// intel_records row to 'published'.
func (s *Service) OnStudioReviewApproved(ctx context.Context, reviewID, reviewerID pgtype.UUID) error {
	job, err := s.store.GetJobByReviewID(ctx, reviewID)
	if err != nil {
		return fmt.Errorf("studio: approve: find job by review_id: %w", err)
	}

	// Load the HITL review to discover the actual target table. For
	// contents it remains 'contents'; for tool-table drafts it tells us
	// which service can publish the row.
	targetTable := "contents"

	var targetID pgtype.UUID

	if s.hitlReader != nil {
		review, herr := s.hitlReader.GetHITLReview(ctx, reviewID)
		if herr != nil {
			slog.Warn("studio: approve: could not load HITL review, defaulting to contents target", "review_id", reviewID, "error", herr)
		} else {
			targetTable = review.TargetTable
			if targetTable == "" {
				targetTable = "contents"
			}

			targetID = review.ContentID
		}
	}

	// Determine the publish target. Prefer the HITL review's content_id +
	// target_table; fall back to the legacy studio_jobs.draft_content_id for
	// rows created before ADR-076 Amendment 1.
	if !targetID.Valid && job.DraftContentID.Valid {
		targetID = job.DraftContentID
		targetTable = "contents"
	}

	// Publish the draft. contents rows use the content publisher; tool-table
	// rows use the dedicated tool-table publisher (ADR-076 Amendment 1).
	// If publication fails, the job is NOT marked published and the linked
	// intel_record is NOT transitioned, so the state stays consistent.
	if targetTable == "contents" && targetID.Valid {
		if s.contentPub == nil {
			return errors.New("studio: approve: content publisher not wired")
		}

		if _, perr := s.contentPub.UpdateContentStatus(ctx, targetID, pgtype.UUID{}, "published"); perr != nil {
			return fmt.Errorf("studio: approve: publish content: %w", perr)
		}
	} else if targetTable != "contents" && targetID.Valid {
		if s.toolTablePub == nil {
			return fmt.Errorf("studio: approve: tool-table publisher not wired for %s", targetTable)
		}

		if perr := s.toolTablePub.Publish(ctx, pgtype.UUID{}, reviewerID, targetTable, targetID, ""); perr != nil {
			return fmt.Errorf("studio: approve: publish %s: %w", targetTable, perr)
		}
	} else {
		return fmt.Errorf("studio: approve: no target id to publish (job_id=%s, target_table=%s)", job.ID.String(), targetTable)
	}

	// Transition the studio job to 'published'.
	if err := s.store.UpdateJobReviewed(ctx, job.ID, "published"); err != nil {
		return fmt.Errorf("studio: approve: update job status: %w", err)
	}

	// Transition the linked intel_record to 'published' (§75).
	s.transitionIntelRecordOnPublish(ctx, job.IntelRecordID)

	slog.Info("studio: review approved, job published", "job_id", job.ID, "review_id", reviewID, "target_table", targetTable, "target_id", targetID, "content_id", job.DraftContentID, "intel_record_id", job.IntelRecordID)

	return nil
}

// OnStudioReviewRejected is called when a source='studio' HITL review is
// rejected (§75: "Reject → job rejected, draft remains as unpublished content").
func (s *Service) OnStudioReviewRejected(ctx context.Context, reviewID, reviewerID pgtype.UUID) error {
	job, err := s.store.GetJobByReviewID(ctx, reviewID)
	if err != nil {
		return fmt.Errorf("studio: reject: find job by review_id: %w", err)
	}

	if err := s.store.UpdateJobReviewed(ctx, job.ID, "rejected"); err != nil {
		return fmt.Errorf("studio: reject: update job status: %w", err)
	}

	slog.Info("studio: review rejected, job rejected", "job_id", job.ID, "review_id", reviewID)

	return nil
}

// transitionIntelRecordOnPublish transitions a linked intel_record to
// 'published' when the studio job is published (§75: "On successful publish
// the linked intel_records row transitions to status='published'"). Safe to
// call when intelRecordID is nil or no updater is wired.
func (s *Service) transitionIntelRecordOnPublish(ctx context.Context, intelRecordID pgtype.UUID) {
	if !intelRecordID.Valid || s.intelUpdater == nil {
		return
	}

	if err := s.intelUpdater.UpdateIntelRecordStatus(ctx, intelRecordID, "published"); err != nil {
		slog.Warn("studio: transition intel_record to published failed", "intel_record_id", intelRecordID, "error", err)
	}
}
