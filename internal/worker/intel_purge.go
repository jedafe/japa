package worker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// HandleIntelPruneRejectedRecords is the intel:prune_rejected_records task
// handler (daily cron, low queue): permanently deletes intel_records rows
// whose status is 'rejected' and whose last_verified_at is older than
// intel.rejected_retention_days (default 7). Rejected records are terminal
// — they can never promote — so they are retained only for short-term
// auditability, then removed. retention_days <= 0 disables the sweep
// entirely (keep-forever sentinel).
func HandleIntelPruneRejectedRecords(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelPruneRejectedRecords")
	slog.Info("task:intel:prune_rejected_records")

	if intelStores == nil {
		slog.Warn("task:intel:prune_rejected_records: dependencies not wired, skipping")
		return nil
	}

	days := 7
	if appSettings != nil {
		days = appSettings.GetIntOr(ctx, "intel.rejected_retention_days", 7)
	}

	if days <= 0 {
		slog.Info("task:intel:prune_rejected_records: retention disabled (retention_days <= 0), skipping")
		return nil
	}

	deleted, err := intelStores.PurgeRejectedIntelRecords(ctx, int32(days))
	if err != nil {
		return fmt.Errorf("task:intel:prune_rejected_records: %w", err)
	}

	slog.Info("task:intel:prune_rejected_records: sweep complete", "deleted", deleted, "retention_days", days)

	return nil
}
