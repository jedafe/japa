package spaces

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// TaskTypeSpaceAutoPopulate is the Asynq task type for populating a space
// from external sources (Wikipedia, flagcdn.com, Pexels). The actual task
// handler will be wired in Phase 6 (Section 11 — Job Queue).
const TaskTypeSpaceAutoPopulate = "space:auto_populate"

// TaskEnqueuer is the interface for enqueueing background tasks.
// The real implementation will use Asynq; for now, the noopEnqueuer
// logs the task and returns nil.
type TaskEnqueuer interface {
	EnqueueSpaceAutoPopulate(ctx context.Context, spaceID pgtype.UUID) error
}

// noopEnqueuer is a no-op implementation for development. It logs the
// enqueue request via slog but does not actually queue anything.
type noopEnqueuer struct{}

// NewNoopEnqueuer creates a TaskEnqueuer that does not actually enqueue
// tasks. Use this until Asynq is wired in Phase 6.
func NewNoopEnqueuer() TaskEnqueuer {
	return &noopEnqueuer{}
}

// EnqueueSpaceAutoPopulate logs the request and returns nil immediately.
func (n *noopEnqueuer) EnqueueSpaceAutoPopulate(ctx context.Context, spaceID pgtype.UUID) error {
	slog.Info("space:auto_populate task enqueued (noop)", "space_id", spaceID.String())
	return nil
}
