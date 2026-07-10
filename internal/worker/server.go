package worker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	errtracking "github.com/jedafe/japa/internal/errors"
)

// NewServer creates a new Asynq server configured with all three task tiers.
// The returned server is ready to have handlers registered via NewMux and then
// be started with srv.Run().
//
// The default retry policy uses exponential backoff (asynq.DefaultRetryDelayFunc)
// with up to 25 retries per task. Individual tasks can override MaxRetry via
// the asynq.MaxRetry option when enqueued.
func NewServer(cfg *Config) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		},
		asynq.Config{
			Queues: map[string]int{
				QueueCritical: cfg.CriticalConcurrency,
				QueueDefault:  cfg.DefaultConcurrency,
				QueueLow:      cfg.LowConcurrency,
			},
			// DefaultRetryDelayFunc implements exponential backoff.
			RetryDelayFunc: asynq.DefaultRetryDelayFunc,
			// Forward every task processing error to GlitchTip (AUDIT-BE-002).
			// This fires on each failed attempt, including the final one before
			// a task moves to the DLQ. No-op when no DSN is configured.
			ErrorHandler: asynq.ErrorHandlerFunc(func(_ context.Context, task *asynq.Task, err error) {
				slog.Error("asynq task failed", "type", task.Type(), "error", err)
				errtracking.CaptureException(fmt.Errorf("asynq task %q failed: %w", task.Type(), err))
			}),
		},
	)
}

// NewMux creates a new Asynq ServeMux and registers all task handlers.
func NewMux() *asynq.ServeMux {
	mux := asynq.NewServeMux()
	RegisterHandlers(mux)

	return mux
}

// NewScheduler creates a new Asynq scheduler that enqueues periodic tasks.
// The scheduler runs in its own goroutine and must be shut down during
// application shutdown.
func NewScheduler(cfg *Config) *asynq.Scheduler {
	scheduler := asynq.NewScheduler(
		asynq.RedisClientOpt{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		},
		&asynq.SchedulerOpts{},
	)

	// ── Internal sweep tasks ────────────────────────────────────────

	// Scheduled content publishing sweep — every 60 seconds.
	_, _ = scheduler.Register("*/60 * * * *", asynq.NewTask(TypeScheduledPublishSweep, nil))

	// Velocity score recalculation — every 5 minutes.
	_, _ = scheduler.Register("*/5 * * * *", asynq.NewTask(TypeVelocityScoreRecalc, nil))

	// Every 15 min: check ticket SLA breaches.
	_, _ = scheduler.Register("*/15 * * * *", asynq.NewTask(TypeMaintenanceCheckSLABreaches, nil))

	// Every 30 min: check expired bans and expired boosts.
	_, _ = scheduler.Register("*/30 * * * *", asynq.NewTask(TypeMaintenanceCheckExpiredBans, nil))
	_, _ = scheduler.Register("*/30 * * * *", asynq.NewTask(TypeMaintenanceCheckExpiredBoosts, nil))

	// Hourly: trending, llms.txt, analytics views, RSS.
	_, _ = scheduler.Register("0 * * * *", asynq.NewTask(TypeComputeTrending, nil))
	_, _ = scheduler.Register("0 * * * *", asynq.NewTask(TypeMaintenanceRegenerateLLMsTxt, nil))
	_, _ = scheduler.Register("0 * * * *", asynq.NewTask(TypeAnalyticsRefreshViews, nil))
	_, _ = scheduler.Register("0 * * * *", asynq.NewTask(TypeRSSRegenerate, nil))

	// 6-hourly: user/space suggestions.
	_, _ = scheduler.Register("0 */6 * * *", asynq.NewTask(TypeComputeSuggestions, nil))

	// Daily 03:00 UTC: security scan.
	_, _ = scheduler.Register("0 3 * * *", asynq.NewTask(TypeSecurityScan, nil))

	// Weekly: prune old feeds, prune audit logs, DM cleanup, prune push tokens.
	_, _ = scheduler.Register("0 2 * * 0", asynq.NewTask(TypeMaintenancePruneOldFeeds, nil))
	_, _ = scheduler.Register("0 2 * * 0", asynq.NewTask(TypeMaintenancePruneAuditLogs, nil))
	_, _ = scheduler.Register("0 2 * * 0", asynq.NewTask(TypeDMCleanupDeclinedRequests, nil))
	_, _ = scheduler.Register("0 2 * * 0", asynq.NewTask(TypeMaintenancePrunePushTokens, nil))

	// Coupon expiry check — hourly.
	_, _ = scheduler.Register("0 * * * *", asynq.NewTask(TypeCouponExpireCheck, nil))

	// Affiliate link expiry — daily.
	_, _ = scheduler.Register("0 4 * * *", asynq.NewTask(TypeAffiliateExpireCheck, nil))

	return scheduler
}

// Addr returns the Valkey address from the config as a formatted string.
// Useful for logging at startup.
func Addr(cfg *Config) string {
	return fmt.Sprintf("%s (db=%d)", cfg.RedisAddr, cfg.RedisDB)
}
