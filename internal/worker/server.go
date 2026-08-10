package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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

	// schedule registers a periodic task with a Unique key matching the
	// cron period. This prevents duplicate execution across replicas.
	// If the task type has an override in SchedulerCronSpecs, use that instead.
	schedule := func(defaultSpec string, taskType string, uniquePeriod time.Duration) {
		spec := defaultSpec
		if override, ok := cfg.SchedulerCronSpecs[taskType]; ok && override != "" {
			spec = override
		}
		if _, err := scheduler.Register(spec, scheduledTask(taskType, uniquePeriod)); err != nil {
			slog.Error("scheduler: failed to register task",
				"spec", spec, "task", taskType, "error", err)
		}
	}

	// ── Internal sweep tasks ────────────────────────────────────────

	// Scheduled content publishing sweep — every minute.
	schedule("* * * * *", TypeScheduledPublishSweep, 1*time.Minute)

	// Velocity score recalculation — every 5 minutes.
	// Writes to content_scores.velocity_score (used by feed ranking queries).
	schedule("*/5 * * * *", TypeMaintenanceUpdateVelocityScores, 5*time.Minute)

	// Every 15 min: check ticket SLA breaches.
	schedule("*/15 * * * *", TypeMaintenanceCheckSLABreaches, 15*time.Minute)

	// Every 30 min: check expired bans and expired boosts.
	schedule("*/30 * * * *", TypeMaintenanceCheckExpiredBans, 30*time.Minute)
	schedule("*/30 * * * *", TypeMaintenanceCheckExpiredBoosts, 30*time.Minute)

	// Hourly: trending, llms.txt, analytics views, RSS, GDPR retention sweep.
	schedule("0 * * * *", TypeComputeTrending, 1*time.Hour)
	schedule("0 * * * *", TypeMaintenanceRegenerateLLMsTxt, 1*time.Hour)
	schedule("0 * * * *", TypeAnalyticsRefreshViews, 1*time.Hour)
	schedule("0 * * * *", TypeRSSRegenerate, 1*time.Hour)
	schedule("0 * * * *", TypeGdprRetentionSweep, 1*time.Hour)

	// 6-hourly: user/space suggestions.
	schedule("0 */6 * * *", TypeComputeSuggestions, 6*time.Hour)

	// Daily 03:00 UTC: security scan.
	schedule("0 3 * * *", TypeSecurityScan, 24*time.Hour)

	// Weekly: prune old feeds, prune audit logs, DM cleanup, prune push tokens.
	schedule("0 2 * * 0", TypeMaintenancePruneOldFeeds, 7*24*time.Hour)
	schedule("0 2 * * 0", TypeMaintenancePruneAuditLogs, 7*24*time.Hour)
	schedule("0 2 * * 0", TypeDMCleanupDeclinedRequests, 7*24*time.Hour)
	schedule("0 2 * * 0", TypeMaintenancePrunePushTokens, 7*24*time.Hour)

	// Coupon expiry check + event flush — hourly.
	schedule("0 * * * *", TypeCouponExpireCheck, 1*time.Hour)
	schedule("5 * * * *", TypeCouponTrackEvents, 1*time.Hour)

	// Affiliate link expiry — daily.
	schedule("0 4 * * *", TypeAffiliateExpireCheck, 24*time.Hour)

	// Daily digest emails — 08:00 UTC.
	schedule("0 8 * * *", TypeDigestEmail, 24*time.Hour)

	// Weekly newsletter digest — Monday 09:00 UTC.
	schedule("0 9 * * 1", TypeNewsletterDigest, 7*24*time.Hour)

	// Daily sitemap generation — 02:00 UTC.
	schedule("0 2 * * *", TypeSitemapGenerate, 24*time.Hour)

	// Daily jobs listing expiry — 01:00 UTC (§63, ADR-081).
	schedule("0 1 * * *", TypeJobsExpireListings, 24*time.Hour)

	// Daily scholarship deadline-reminder sweep — 05:00 UTC (§73, ADR-073).
	// Offset from other daily tasks to spread Redis load. The handler
	// iterates tenants, finds saved scholarships whose application_deadline
	// is within remind_before_days, dispatches a notification per match,
	// and marks the reminder as sent.
	schedule("0 5 * * *", TypeScholarshipCheckDeadlines, 24*time.Hour)

	// Weekly GeoIP database update — Sunday 02:00 UTC (Section 37 / ADR-037).
	schedule("0 2 * * 0", TypeMaintenanceUpdateGeoIPDB, 7*24*time.Hour)

	// Persona orchestration sweep — every 60 min (default).
	// The actual interval is hot-reloadable via participation.run_interval_minutes
	// (system_settings); the sweep handler skips if the interval hasn't elapsed.
	// Ref: japaconnect-engineering/sections/76-participation-orchestration.md
	schedule("0 * * * *", TypePersonaRunActivity, 1*time.Hour)

	// Hourly anonymous assessment result cleanup (§71, ADR-071 §5).
	// Deletes assessment_results where user_id IS NULL and created_at is
	// older than 24 hours. Anonymous results are persisted temporarily for
	// the async polling architecture and cleaned up periodically.
	schedule("30 * * * *", TypeAssessmentCleanupAnonymous, 1*time.Hour)

	return scheduler
}

// scheduledTask creates a scheduled Asynq task with the correct queue tier
// and a Unique key to prevent duplicate execution across replicas.
func scheduledTask(taskType string, uniquePeriod time.Duration) *asynq.Task {
	return asynq.NewTask(taskType, []byte("{}"),
		asynq.Queue(QueueForTask(taskType)),
		asynq.Unique(uniquePeriod),
	)
}

// Addr returns the Valkey address from the config as a formatted string.
// Useful for logging at startup.
func Addr(cfg *Config) string {
	return fmt.Sprintf("%s (db=%d)", cfg.RedisAddr, cfg.RedisDB)
}
