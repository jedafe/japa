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
	"github.com/jedafe/japa/internal/personas"
)

// TaskEnqueuer is the subset of worker.Enqueuer that studio needs.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service holds the business logic for the AI Content Studio.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
}

// NewService creates a Service backed by the given store and settings.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetEnqueuer injects the task enqueuer (called at server wiring time).
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// highStakesCategories are the guide categories that always require human
// review, regardless of studio.auto_publish_types. Hardcoded per §75 — not
// configurable.
var highStakesCategories = map[string]bool{
	"visa":            true,
	"banking":         true,
	"grants":          true,
	"health_insurance": true,
	"admission":       true,
}

// permanentlyExcludedTypes are content types that can never auto-publish.
var permanentlyExcludedTypes = map[string]bool{
	"blog_post":      true,
	"guide":          true,
	"comparison":     true,
	"event":          true,
	"job_listing":    true,
	"service_listing": true,
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

// CreateBatch creates a batch with multiple jobs and enqueues generation for each.
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

	// Enforce studio.max_concurrent_jobs (ADR-076, §75). Default 3.
	if err := s.checkConcurrency(ctx, tenantID, len(in.Jobs)); err != nil {
		return Batch{}, nil, err
	}

	batch, err := s.store.CreateBatch(ctx, tenantID, createdBy, in.Name, in.Description)
	if err != nil {
		return Batch{}, nil, fmt.Errorf("studio: create batch: %w", err)
	}

	var jobs []Job
	for _, jin := range in.Jobs {
		job, err := s.store.CreateJob(ctx, tenantID, createdBy, batch.ID, jin)
		if err != nil {
			slog.Warn("studio: create job in batch failed", "batch_id", batch.ID, "error", err)
			continue
		}
		if err := s.enqueueGeneration(ctx, job); err != nil {
			_ = s.store.UpdateJobStatus(ctx, tenantID, job.ID, "failed", strPtr(err.Error()))
			slog.Warn("studio: enqueue job in batch failed", "job_id", job.ID, "error", err)
		}
		jobs = append(jobs, job)
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
	job, err := s.store.GetJob(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("studio: get job for retry: %w", err)
	}
	if job.Status != "failed" {
		return fmt.Errorf("studio: only failed jobs can be retried (current: %s)", job.Status)
	}
	if err := s.store.UpdateJobStatus(ctx, tenantID, id, "queued", nil); err != nil {
		return fmt.Errorf("studio: reset job status: %w", err)
	}
	job.Status = "queued"
	return s.enqueueGeneration(ctx, job)
}

// CancelJob deletes a queued or failed job.
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
	jobs, err := s.store.ListFailedJobs(ctx, tenantID, batchID)
	if err != nil {
		return 0, fmt.Errorf("studio: list failed jobs: %w", err)
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

// ListStudioPersonas returns personas available for studio use.
func (s *Service) ListStudioPersonas(ctx context.Context, tenantID pgtype.UUID) ([]personas.PersonaListItem, int64, error) {
	// Delegate to the personas package — studio reuses the same persona list.
	// The personas service is not injected here; the handler will call the
	// personas service directly. This method is a placeholder for the
	// service-level integration point.
	return nil, 0, fmt.Errorf("studio: use personas service directly")
}

// ContentTypeInfo describes a content type available in the studio.
type ContentTypeInfo struct {
	Type           string   `json:"type"`
	Label          string   `json:"label"`
	GuideCategories []string `json:"guide_categories,omitempty"`
	CanAutoPublish bool     `json:"can_auto_publish"`
}

// ListContentTypes returns the content types available for studio generation.
func (s *Service) ListContentTypes(ctx context.Context, tenantID pgtype.UUID) []ContentTypeInfo {
	types := []ContentTypeInfo{
		{Type: "discussion", Label: "Discussion", CanAutoPublish: true},
		{Type: "page", Label: "Page", CanAutoPublish: true},
		{Type: "blog_post", Label: "Blog Post", CanAutoPublish: false},
		{Type: "guide", Label: "Guide", GuideCategories: []string{"visa", "banking", "grants", "health_insurance", "admission", "scholarship", "housing", "general"}, CanAutoPublish: false},
		{Type: "comparison", Label: "Comparison", CanAutoPublish: false},
		{Type: "event", Label: "Event", CanAutoPublish: false},
		{Type: "job_listing", Label: "Job Listing", CanAutoPublish: false},
		{Type: "service_listing", Label: "Service Listing", CanAutoPublish: false},
	}

	// Filter by auto_publish_types setting.
	autoPublishCSV := ""
	if s.settings != nil {
		autoPublishCSV, _ = s.settings.GetSetting(ctx, tenantID, "studio.auto_publish_types")
	}
	autoPublishMap := parseCSV(autoPublishCSV)
	for i := range types {
		if !types[i].CanAutoPublish {
			continue
		}
		// Only keep auto-publish if it's in the configured list.
		if len(autoPublishMap) > 0 && !autoPublishMap[types[i].Type] {
			types[i].CanAutoPublish = false
		}
	}

	return types
}

// enqueueGeneration enqueues a content:generate_draft task for the job.
func (s *Service) enqueueGeneration(ctx context.Context, job Job) error {
	if s.enqueuer == nil {
		return fmt.Errorf("studio: no enqueuer configured")
	}
	payload, err := json.Marshal(map[string]string{
		"job_id":       job.ID.String(),
		"tenant_id":    job.TenantID.String(),
		"persona_id":   job.PersonaID.String(),
		"content_type": job.ContentType,
		"topic_prompt": job.TopicPrompt,
	})
	if err != nil {
		return fmt.Errorf("studio: marshal payload: %w", err)
	}
	if err := s.enqueuer.Enqueue("content:generate_draft", payload); err != nil {
		return fmt.Errorf("studio: enqueue: %w", err)
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

// MarkJobGenerating sets a job's status to 'generating'.
func (s *Service) MarkJobGenerating(ctx context.Context, tenantID, jobID pgtype.UUID) error {
	return s.store.UpdateJobStatus(ctx, tenantID, jobID, "generating", nil)
}

// MarkJobGenerated sets the draft_content_id, review_id, and status after
// successful generation. This is called by the task handler after the AI
// gateway returns the draft content.
func (s *Service) MarkJobGenerated(ctx context.Context, tenantID, jobID, draftContentID, reviewID pgtype.UUID) error {
	return s.store.UpdateJobGenerated(ctx, tenantID, jobID, draftContentID, reviewID)
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
// It loads all queued jobs in the batch and enqueues generation for each.
func (s *Service) RunBatch(ctx context.Context, tenantID, batchID pgtype.UUID) error {
	jobs, _, err := s.store.ListJobs(ctx, tenantID, JobListFilter{
		BatchID: &batchID, Status: strPtr("queued"), Limit: 100,
	})
	if err != nil {
		return fmt.Errorf("studio: run_batch: list jobs: %w", err)
	}

	for _, job := range jobs {
		if err := s.enqueueGeneration(ctx, job); err != nil {
			slog.Warn("studio: run_batch: enqueue failed", "job_id", job.ID, "error", err)
		}
	}
	slog.Info("studio: run_batch: enqueued jobs", "batch_id", batchID, "count", len(jobs))
	return nil
}

// RetryFailed is the processor method called by the studio:retry_failed task.
func (s *Service) RetryFailed(ctx context.Context, tenantID, jobID pgtype.UUID) error {
	return s.RetryJob(ctx, tenantID, jobID)
}
