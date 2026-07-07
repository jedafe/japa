package worker

import (
	"log/slog"

	"github.com/hibiken/asynq"
)

// Scheduler registers all periodic cron tasks with the Asynq scheduler.
// Call this once on startup after the scheduler is created.
func Scheduler(scheduler *asynq.Scheduler) {
	register := func(cron string, task *asynq.Task) {
		if _, err := scheduler.Register(cron, task); err != nil {
			slog.Error("scheduler: failed to register task", "task", task.Type(), "cron", cron, "error", err)
		}
	}

	// ── Internal sweep tasks ────────────────────────────────────────
	register("*/60 * * * *", asynq.NewTask(TypeScheduledPublishSweep, nil))
	register("*/5 * * * *", asynq.NewTask(TypeVelocityScoreRecalc, nil))

	// ── Phase 31 — Full cron schedule from Section 11 catalog ───────
	register("*/10 * * * *", asynq.NewTask(TypeMaintenanceUpdateVelocityScores, nil))
	register("*/15 * * * *", asynq.NewTask(TypeMaintenanceCheckSLABreaches, nil))
	register("*/30 * * * *", asynq.NewTask(TypeMaintenanceCheckExpiredBans, nil))
	register("*/30 * * * *", asynq.NewTask(TypeMaintenanceCheckExpiredBoosts, nil))
	register("0 * * * *", asynq.NewTask(TypeComputeTrending, nil))
	register("5 * * * *", asynq.NewTask(TypeMaintenanceRegenerateLLMsTxt, nil))
	register("10 * * * *", asynq.NewTask(TypeAnalyticsRefreshViews, nil))
	register("15 * * * *", asynq.NewTask(TypeRSSRegenerate, nil))
	register("0 */6 * * *", asynq.NewTask(TypeComputeSuggestions, nil))
	register("0 3 * * *", asynq.NewTask(TypeSecurityScan, nil))
	register("0 4 * * 0", asynq.NewTask(TypeMaintenancePruneOldFeeds, nil))
	register("0 5 * * 0", asynq.NewTask(TypeMaintenancePruneAuditLogs, nil))
	register("0 6 * * 0", asynq.NewTask(TypeMaintenancePrunePushTokens, nil))
	register("0 7 * * 0", asynq.NewTask(TypeDMCleanupDeclinedRequests, nil))
	register("0 2 * * 0", asynq.NewTask(TypeMaintenanceUpdateGeoIPDB, nil))
	register("20 * * * *", asynq.NewTask(TypeCouponExpireCheck, nil))
	register("0 1 * * *", asynq.NewTask(TypeAffiliateExpireCheck, nil))

	slog.Info("asynq scheduler registered", "jobs", 20)
}
