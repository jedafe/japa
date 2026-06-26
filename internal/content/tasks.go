package content

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// Task type constants for Asynq. The actual task handlers will be wired
// in Phase 6 (Section 11 — Job Queue). For now, the noopEnqueuer logs
// the request and returns nil.
const (
	TaskTypeScheduledPublish = "content:scheduled_publish"
	TaskTypeComputeScores    = "content:compute_scores"
	TaskTypeGenerateSlug     = "content:generate_slug"
)

// TaskEnqueuer is the interface for enqueueing background tasks.
// The real implementation will use Asynq; for now, the noopEnqueuer
// logs the task and returns nil.
type TaskEnqueuer interface {
	EnqueueScheduledPublish(ctx context.Context, contentID pgtype.UUID) error
	EnqueueComputeScores(ctx context.Context, contentID pgtype.UUID) error
}

// noopEnqueuer is a no-op implementation for development. It logs the
// enqueue request via slog but does not actually queue anything.
type noopEnqueuer struct{}

// NewNoopEnqueuer creates a TaskEnqueuer that does not actually enqueue
// tasks. Use this until Asynq is wired in Phase 6.
func NewNoopEnqueuer() TaskEnqueuer {
	return &noopEnqueuer{}
}

// EnqueueScheduledPublish logs the request and returns nil immediately.
func (n *noopEnqueuer) EnqueueScheduledPublish(ctx context.Context, contentID pgtype.UUID) error {
	slog.Info("content:scheduled_publish task enqueued (noop)", "content_id", contentID.String())
	return nil
}

// EnqueueComputeScores logs the request and returns nil immediately.
func (n *noopEnqueuer) EnqueueComputeScores(ctx context.Context, contentID pgtype.UUID) error {
	slog.Info("content:compute_scores task enqueued (noop)", "content_id", contentID.String())
	return nil
}