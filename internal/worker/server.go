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
	_, _ = scheduler.Register("*/60 * * * *", scheduledTask(TypeScheduledPublishSweep))

	// Velocity score recalculation — every 5 minutes.
	_, _ = scheduler.Register("*/5 * * * *", scheduledTask(TypeVelocityScoreRecalc))

	// Every 15 min: check ticket SLA breaches.
	_, _ = scheduler.Register("*/15 * * * *", scheduledTask(TypeMaintenanceCheckSLABreaches))

	// Every 30 min: check expired bans and expired boosts.
	_, _ = scheduler.Register("*/30 * * * *", scheduledTask(TypeMaintenanceCheckExpiredBans))
	_, _ = scheduler.Register("*/30 * * * *", scheduledTask(TypeMaintenanceCheckExpiredBoosts))

	// Hourly: trending, llms.txt, analytics views, RSS.
	_, _ = scheduler.Register("0 * * * *", scheduledTask(TypeComputeTrending))
	_, _ = scheduler.Register("0 * * * *", scheduledTask(TypeMaintenanceRegenerateLLMsTxt))
	_, _ = scheduler.Register("0 * * * *", scheduledTask(TypeAnalyticsRefreshViews))
	_, _ = scheduler.Register("0 * * * *", scheduledTask(TypeRSSRegenerate))

	// 6-hourly: user/space suggestions.
	_, _ = scheduler.Register("0 */6 * * *", scheduledTask(TypeComputeSuggestions))

	// Daily 03:00 UTC: security scan.
	_, _ = scheduler.Register("0 3 * * *", scheduledTask(TypeSecurityScan))

	// Weekly: prune old feeds, prune audit logs, DM cleanup, prune push tokens.
	_, _ = scheduler.Register("0 2 * * 0", scheduledTask(TypeMaintenancePruneOldFeeds))
	_, _ = scheduler.Register("0 2 * * 0", scheduledTask(TypeMaintenancePruneAuditLogs))
	_, _ = scheduler.Register("0 2 * * 0", scheduledTask(TypeDMCleanupDeclinedRequests))
	_, _ = scheduler.Register("0 2 * * 0", scheduledTask(TypeMaintenancePrunePushTokens))

	// Coupon expiry check + event flush — hourly.
	_, _ = scheduler.Register("0 * * * *", scheduledTask(TypeCouponExpireCheck))
	_, _ = scheduler.Register("5 * * * *", scheduledTask(TypeCouponTrackEvents))

	// Affiliate link expiry — daily.
	_, _ = scheduler.Register("0 4 * * *", scheduledTask(TypeAffiliateExpireCheck))

	return scheduler
}

// scheduledTask creates a scheduled Asynq task with the correct queue tier.
func scheduledTask(taskType string) *asynq.Task {
	return asynq.NewTask(taskType, nil, asynq.Queue(QueueForTask(taskType)))
}

// Addr returns the Valkey address from the config as a formatted string.
// Useful for logging at startup.
func Addr(cfg *Config) string {
	return fmt.Sprintf("%s (db=%d)", cfg.RedisAddr, cfg.RedisDB)
}
