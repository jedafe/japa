package worker

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"
)

// NextActionsSweeper is the §122 engine's sweep entrypoint — kept as a
// narrow interface because importing internal/nextactions would create an
// import cycle (nextactions → analytics → auth → worker).
type NextActionsSweeper interface {
	Sweep(ctx context.Context) error
}

var nextActionsSvc NextActionsSweeper

// RegisterNextActionsService injects the §122 engine for the sweep handler.
func RegisterNextActionsService(s NextActionsSweeper) { nextActionsSvc = s }

// HandleNextActionsSweep runs the §122 materialization pass — evaluates
// push-channel rules over recently-active users, writes user_next_actions
// rows, and dispatches via §35 (next_action.suggested). The service takes
// the jc:nextactions:sweep:lock Valkey lock and re-checks the configured
// interval, so hourly scheduling is safe.
// Ref: japaconnect-engineering/sections/122-next-action-suggestions.md.
func HandleNextActionsSweep(ctx context.Context, _ *asynq.Task) error {
	if nextActionsSvc == nil {
		slog.Warn("task:nextactions:sweep: no service registered, skipping")
		return nil
	}
	return nextActionsSvc.Sweep(ctx)
}
