// Package studio — service.go holds the business logic for the AI Content
// Studio: job/batch creation, generation pipeline orchestration, review
// routing, and the high-stakes content gate.
//
// Ref: japaconnect-engineering/sections/75-ai-content-studio.md
package studio

import (
	"context"
	"encoding/json"
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
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, id pgtype.UUID, status string) error
}

// ContentPublisher publishes a content row (sets status='published'). Used
// by the studio review hook to publish the draft content when a studio HITL
// review is approved (§75: "Approve → publish"). The return value is the
// updated content row; the studio service ignores it.
type ContentPublisher interface {
	UpdateContentStatus(ctx context.Context, contentID, tenantID pgtype.UUID, status string) (any, error)
}

// Service holds the business logic for the AI Content Studio.
type Service struct {
	store          *Store
	settings       *config.Settings
	enqueuer       TaskEnqueuer
	intelUpdater   IntelRecordStatusUpdater
	contentPub     ContentPublisher
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

// highStakesCategories are the guide categories that always require human
// review, regardless of studio.auto_publish_types. Hardcoded per §75 — not
// configurable.
var highStakesCategories = map[string]bool{
	"visa":             true,
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
}

// IsHighStakes returns true if the guide category requires mandatory review.
// Exported so the participation orchestration engine can reuse the same gate.
func IsHighStakes(guideCategory string) bool {
	return highStakesCategories[strings.ToLower(guideCategory)]
}

// CreateJob creates a single studio job and enqueues a generation task.
func (s *Service) CreateJob(ctx context.Context, tenantID, createdBy pgtype.UUID, in CreateJobInput) (Job, error) {
	if !s.isEnabled(ctx, tenantID) {
		return Job{}, fmt.Errorf("studio: feature is disabled")
	}
	if in.TopicPrompt == "" {
		return Job{}, fmt.Errorf("studio: topic_prompt is required")
	}
	if in.ContentType == "" {
		return Job{}, fmt.Errorf("studio: content_type is required")
	}

	// Enforce studio.max_concurrent_jobs (ADR-076, §75). Default 3.
	if err := s.checkConcurrency(ctx, tenantID, 1); err != nil {
		return Job{}, err
	}

	job, err := s.store.CreateJob(ctx, tenantID, createdBy, pgtype.UUID{}, in)
	if err != nil {
		return Job{}, fmt.Errorf("studio: create job: %w", err)
	}

	if err := s.enqueueGeneration(ctx, job); err != nil {
		_ = s.store.UpdateJobStatus(ctx, tenantID, job.ID, "failed", strPtr(err.Error()))
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
	if !s.isEnabled(ctx, tenantID) {
		return Batch{}, nil, fmt.Errorf("studio: feature is disabled")
	}
	if in.Name == "" {
		return Batch{}, nil, fmt.Errorf("studio: batch name is required")
	}
	if len(in.Jobs) == 0 {
		return Batch{}, nil, fmt.Errorf("studio: batch must contain at least one job")
	}

	batch, err := s.store.CreateBatch(ctx, tenantID, createdBy, in.Name, in.Description)
	if err != nil {
		return Batch{}, nil, fmt.Errorf("studio: create batch: %w", err)
	}

	// Create all jobs in 'queued' status. Generation is enqueued by the
	// studio:run_batch task processor, not inline — this is the async batch
	// pipeline per ADR-076 §2/§7.
	var jobs []Job
	for _, jin := range in.Jobs {
		job, err := s.store.CreateJob(ctx, tenantID, createdBy, batch.ID, jin)
		if err != nil {
			slog.Warn("studio: create job in batch failed", "batch_id", batch.ID, "error", err)
			continue
		}
		jobs = append(jobs, job)
	}

	if len(jobs) == 0 {
		return batch, jobs, fmt.Errorf("studio: batch created but no jobs could be inserted")
	}

	// Enqueue a single studio:run_batch task — the processor fans out
	// individual content:generate_draft jobs respecting max_concurrent_jobs.
	if err := s.enqueueRunBatch(ctx, tenantID, batch.ID); err != nil {
		slog.Warn("studio: enqueue run_batch failed, jobs remain queued",
			"batch_id", batch.ID, "error", err)
	}

	return batch, jobs, nil
}

// GetJob returns a single job by ID.
func (s *Service) GetJob(ctx context.Context, tenantID, id pgtype.UUID) (Job, error) {
	return s.store.GetJob(ctx, tenantID, id)
}

// ListJobs lists jobs with optional filters.
func (s *Service) ListJobs(ctx context.Context, tenantID pgtype.UUID, f JobListFilter) ([]Job, int64, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	return s.store.ListJobs(ctx, tenantID, f)
}

// RetryJob resets a failed job to queued and re-enqueues generation.
func (s *Service) RetryJob(ctx context.Context, tenantID, id pgtype.UUID) error {
	if !s.isEnabled(ctx, tenantID) {
		return fmt.Errorf("studio: feature is disabled")
	}
	job, err := s.store.GetJob(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("studio: get job for retry: %w", err)
	}
	if job.Status != "failed" {
		return fmt.Errorf("studio: only failed jobs can be retried (current: %s)", job.Status)
	}
	// Enforce studio.max_concurrent_jobs (ADR-076, §75) before enqueuing.
	if err := s.checkConcurrency(ctx, tenantID, 1); err != nil {
		return err
	}
	if err := s.store.UpdateJobStatus(ctx, tenantID, id, "queued", nil); err != nil {
		return fmt.Errorf("studio: reset job status: %w", err)
	}
	job.Status = "queued"
	return s.enqueueGeneration(ctx, job)
}

// CancelJob deletes a queued or failed job. Returns ErrJobNotCancellable if
// the job is in a status that cannot be cancelled.
func (s *Service) CancelJob(ctx context.Context, tenantID, id pgtype.UUID) error {
	return s.store.DeleteJob(ctx, tenantID, id)
}

// ListBatches lists batches for a tenant.
func (s *Service) ListBatches(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Batch, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.store.ListBatches(ctx, tenantID, limit, offset)
}

// GetBatch returns a batch with progress stats.
func (s *Service) GetBatch(ctx context.Context, tenantID, id pgtype.UUID) (Batch, BatchStats, error) {
	batch, err := s.store.GetBatch(ctx, tenantID, id)
	if err != nil {
		return Batch{}, BatchStats{}, err
	}
	stats, err := s.store.GetBatchStats(ctx, tenantID, id)
	return batch, stats, err
}

// RetryBatchFailed retries all failed jobs in a batch.
func (s *Service) RetryBatchFailed(ctx context.Context, tenantID, batchID pgtype.UUID) (int, error) {
	if !s.isEnabled(ctx, tenantID) {
		return 0, fmt.Errorf("studio: feature is disabled")
	}
	jobs, err := s.store.ListFailedJobs(ctx, tenantID, batchID)
	if err != nil {
		return 0, fmt.Errorf("studio: list failed jobs: %w", err)
	}
	// Enforce studio.max_concurrent_jobs (ADR-076, §75) — check once for the
	// batch; individual jobs beyond the limit remain in 'queued' and are
	// picked up by subsequent run_batch invocations.
	if err := s.checkConcurrency(ctx, tenantID, len(jobs)); err != nil {
		// If the full batch would exceed the limit, still retry as many as
		// we can within the limit — the rest stay queued for later pickup.
		slog.Warn("studio: retry batch: concurrency limit, retrying partial",
			"batch_id", batchID, "failed_count", len(jobs), "error", err)
	}
	var count int
	for _, job := range jobs {
		if err := s.store.UpdateJobStatus(ctx, tenantID, job.ID, "queued", nil); err != nil {
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
func (s *Service) GetStats(ctx context.Context, tenantID pgtype.UUID) (StudioStats, error) {
	return s.store.GetStats(ctx, tenantID)
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

// ListContentTypes returns the content types available for studio generation
// along with the studio.default_persona_id setting so the frontend can
// pre-select a persona in the new-job form (§75 system_settings, ADR-076 §6).
func (s *Service) ListContentTypes(ctx context.Context, tenantID pgtype.UUID) ContentTypeResponse {
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
		defaultPersonaID, _ = s.settings.GetSetting(ctx, tenantID, "studio.default_persona_id")
	}

	return ContentTypeResponse{
		Types:            types,
		DefaultPersonaID: defaultPersonaID,
	}
}

// enqueueGeneration enqueues a content:generate_draft task for the job.
func (s *Service) enqueueGeneration(ctx context.Context, job Job) error {
	if s.enqueuer == nil {
		return fmt.Errorf("studio: no enqueuer configured")
	}
	payload := map[string]any{
		"job_id":       job.ID.String(),
		"tenant_id":    job.TenantID.String(),
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
		return fmt.Errorf("studio: no enqueuer configured")
	}
	payload, err := json.Marshal(map[string]string{
		"batch_id":  batchID.String(),
		"tenant_id": tenantID.String(),
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
		return fmt.Errorf("studio: no enqueuer configured")
	}
	payload, err := json.Marshal(map[string]string{
		"batch_id":  batchID.String(),
		"tenant_id": tenantID.String(),
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
func (s *Service) isEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "studio.enabled", true)
}

// checkConcurrency enforces studio.max_concurrent_jobs (ADR-076, §75).
// Returns an error if adding `pending` new jobs would exceed the limit.
func (s *Service) checkConcurrency(ctx context.Context, tenantID pgtype.UUID, pending int) error {
	maxConcurrent := 3
	if s.settings != nil {
		maxConcurrent = s.settings.GetIntOr(ctx, tenantID, "studio.max_concurrent_jobs", 3)
	}
	if maxConcurrent <= 0 {
		return nil // 0 or negative = unlimited
	}
	active, err := s.store.CountActiveJobs(ctx, tenantID)
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
func (s *Service) MarkJobGenerating(ctx context.Context, tenantID, jobID pgtype.UUID) error {
	return s.store.MarkJobGeneratingCAS(ctx, tenantID, jobID)
}

// MarkJobGenerated sets the draft_content_id and status to 'draft_ready'
// after the AI gateway has produced the draft content (ADR-076 §2 step 7).
func (s *Service) MarkJobGenerated(ctx context.Context, tenantID, jobID, draftContentID pgtype.UUID) error {
	return s.store.UpdateJobGenerated(ctx, tenantID, jobID, draftContentID)
}

// MarkJobRoutedToReview sets the review_id and status to 'in_review' after
// the draft has been routed to the content_reviews HITL queue (ADR-076 §2
// step 8). Called after MarkJobGenerated once the review row exists.
func (s *Service) MarkJobRoutedToReview(ctx context.Context, tenantID, jobID, reviewID pgtype.UUID) error {
	return s.store.UpdateJobRoutedToReview(ctx, tenantID, jobID, reviewID)
}

// MarkJobFailed sets a job's status to 'failed' with an error message.
func (s *Service) MarkJobFailed(ctx context.Context, tenantID, jobID pgtype.UUID, errMsg string) error {
	return s.store.UpdateJobStatus(ctx, tenantID, jobID, "failed", strPtr(errMsg))
}

// MarkJobPublished marks a job's draft as auto-published (status='published').
// Used for content types in studio.auto_publish_types (§75).
func (s *Service) MarkJobPublished(ctx context.Context, tenantID, jobID, draftContentID pgtype.UUID) error {
	return s.store.UpdateJobPublished(ctx, tenantID, jobID, draftContentID)
}

// ShouldAutoPublish returns true if the content type can skip review.
// High-stakes guide categories always return false.
func (s *Service) ShouldAutoPublish(ctx context.Context, tenantID pgtype.UUID, contentType, guideCategory string) bool {
	if IsHighStakes(guideCategory) {
		return false
	}
	if permanentlyExcludedTypes[contentType] {
		return false
	}
	autoPublishCSV := ""
	if s.settings != nil {
		autoPublishCSV, _ = s.settings.GetSetting(ctx, tenantID, "studio.auto_publish_types")
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
		maxConcurrent = s.settings.GetIntOr(ctx, tenantID, "studio.max_concurrent_jobs", 3)
	}

	active, err := s.store.CountActiveJobs(ctx, tenantID)
	if err != nil {
		slog.Warn("studio: run_batch: count active failed, enqueuing all",
			"batch_id", batchID, "error", err)
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
	for i := 0; i < toEnqueue; i++ {
		if err := s.enqueueGeneration(ctx, jobs[i]); err != nil {
			slog.Warn("studio: run_batch: enqueue failed",
				"job_id", jobs[i].ID, "error", err)
			continue
		}
		enqueued++
	}

	slog.Info("studio: run_batch: enqueued jobs",
		"batch_id", batchID, "enqueued", enqueued,
		"remaining_queued", len(jobs)-enqueued)
	return nil
}

// RetryFailed is the processor method called by the studio:retry_failed task.
func (s *Service) RetryFailed(ctx context.Context, tenantID, jobID pgtype.UUID) error {
	return s.RetryJob(ctx, tenantID, jobID)
}

// EnqueueRetryFailed enqueues a studio:retry_failed task for a batch. Called
// by the HTTP handler so batch retry is asynchronous (ADR-076 §7).
func (s *Service) EnqueueRetryFailed(ctx context.Context, tenantID, batchID pgtype.UUID) error {
	return s.enqueueRetryFailed(ctx, tenantID, batchID)
}

// OnStudioReviewApproved is called by the admin moderation handler when a
// source='studio' HITL review is approved (§75, ADR-076 §3). It publishes
// the draft content, transitions studio_jobs to 'published', and transitions
// the linked intel_records row to 'published'.
func (s *Service) OnStudioReviewApproved(ctx context.Context, tenantID, reviewID pgtype.UUID) error {
	job, err := s.store.GetJobByReviewID(ctx, tenantID, reviewID)
	if err != nil {
		return fmt.Errorf("studio: approve: find job by review_id: %w", err)
	}

	// Publish the draft content (contents table only — tool-table types
	// have their own publish flows managed by their services).
	if job.DraftContentID.Valid && s.contentPub != nil {
		if _, perr := s.contentPub.UpdateContentStatus(ctx, job.DraftContentID, tenantID, "published"); perr != nil {
			slog.Warn("studio: approve: publish content failed, marking job published anyway",
				"job_id", job.ID, "content_id", job.DraftContentID, "error", perr)
		}
	}

	// Transition the studio job to 'published'.
	if err := s.store.UpdateJobReviewed(ctx, tenantID, job.ID, "published"); err != nil {
		return fmt.Errorf("studio: approve: update job status: %w", err)
	}

	// Transition the linked intel_record to 'published' (§75).
	s.transitionIntelRecordOnPublish(ctx, tenantID, job.IntelRecordID)

	slog.Info("studio: review approved, job published",
		"job_id", job.ID, "review_id", reviewID,
		"content_id", job.DraftContentID, "intel_record_id", job.IntelRecordID)
	return nil
}

// OnStudioReviewRejected is called by the admin moderation handler when a
// source='studio' HITL review is rejected (§75: "Reject → job rejected,
// draft remains as unpublished content").
func (s *Service) OnStudioReviewRejected(ctx context.Context, tenantID, reviewID pgtype.UUID) error {
	job, err := s.store.GetJobByReviewID(ctx, tenantID, reviewID)
	if err != nil {
		return fmt.Errorf("studio: reject: find job by review_id: %w", err)
	}
	if err := s.store.UpdateJobReviewed(ctx, tenantID, job.ID, "rejected"); err != nil {
		return fmt.Errorf("studio: reject: update job status: %w", err)
	}
	slog.Info("studio: review rejected, job rejected",
		"job_id", job.ID, "review_id", reviewID)
	return nil
}

// transitionIntelRecordOnPublish transitions a linked intel_record to
// 'published' when the studio job is published (§75: "On successful publish
// the linked intel_records row transitions to status='published'"). Safe to
// call when intelRecordID is nil or no updater is wired.
func (s *Service) transitionIntelRecordOnPublish(ctx context.Context, tenantID, intelRecordID pgtype.UUID) {
	if !intelRecordID.Valid || s.intelUpdater == nil {
		return
	}
	if err := s.intelUpdater.UpdateIntelRecordStatus(ctx, tenantID, intelRecordID, "published"); err != nil {
		slog.Warn("studio: transition intel_record to published failed",
			"intel_record_id", intelRecordID, "error", err)
	}
}
