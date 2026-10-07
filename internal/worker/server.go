package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	errtracking "github.com/jedafe/japa/internal/errors"
)

// ErrUpstreamRateLimited marks task failures caused by an upstream API
// rate-limiting us (e.g. Wikipedia 429 during space:auto_populate).
// Packages that cannot be imported here wrap their own rate-limit
// sentinels with this one so the server RetryDelayFunc can back off hard
// instead of letting exponential retries keep the rate limit hot.
var ErrUpstreamRateLimited = errors.New("worker: upstream rate limited")

// retryDelay backs off hard on upstream rate-limit errors — default
// exponential retries restart within seconds and keep the provider's
// rate limit saturated (a 76k-task enrichment backlog can otherwise
// re-ban the egress IP indefinitely). Other errors keep the standard
// exponential backoff.
func retryDelay(n int, e error, t *asynq.Task) time.Duration {
	if errors.Is(e, ErrUpstreamRateLimited) {
		return 10 * time.Minute
	}

	return asynq.DefaultRetryDelayFunc(n, e, t)
}

// permanentTaskError is a ServeMux middleware that downgrades
// cannot-ever-succeed configuration errors to asynq.SkipRetry so the task
// archives after the first attempt instead of burning the retry budget.
// errors.Is cannot reach the ai.ErrNoProviders sentinel from this package
// (internal/ai imports worker — the reverse would cycle), so the stable
// message substring is matched. Covers the AI gateway and the email gateway.
func permanentTaskError(next asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
		err := next.ProcessTask(ctx, t)
		switch {
		case err == nil || errors.Is(err, asynq.SkipRetry):
			return err
		case strings.Contains(err.Error(), "no providers registered"):
			return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
		default:
			return err
		}
	})
}

// NewServer creates a new Asynq server configured with all three task tiers.
// The returned server is ready to have handlers registered via NewMux and then
// be started with srv.Run().
// // The default retry policy uses exponential backoff (asynq.DefaultRetryDelayFunc)
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
			// retryDelay wraps DefaultRetryDelayFunc (exponential backoff)
			// with a hard 10-minute backoff for upstream rate limits.
			RetryDelayFunc: retryDelay,
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
	mux.Use(permanentTaskError)
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
	schedule := func(defaultSpec, taskType string, uniquePeriod time.Duration) {
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

	// §13: batched engagement-score re-index — every 5 minutes.
	// Re-indexes content documents whose engagement_score changed since the
	// last sweep, so ?sort=top stays fresh without re-indexing on every
	// engagement event. The handler tracks the last-swept timestamp in
	// Valkey (jc:search:engagement_reindex_cursor) and only re-indexes
	// content whose content_scores.updated_at > cursor. Default queue.
	schedule("*/5 * * * *", TypeSearchReindexEngagementScores, 5*time.Minute)

	// Every 15 min: check ticket SLA breaches.
	schedule("*/15 * * * *", TypeMaintenanceCheckSLABreaches, 15*time.Minute)

	// Every 30 min: check expired bans and expired boosts.
	schedule("*/30 * * * *", TypeMaintenanceCheckExpiredBans, 30*time.Minute)
	schedule("*/30 * * * *", TypeMaintenanceCheckExpiredBoosts, 30*time.Minute)

	// §17: subscription expiry sweep — expires rows whose entitlement window
	// ended (cancel_at_period_end reached, or past_due past grace) and
	// downgrades the granted tier. Webhook-miss safety net; default 30 min,
	// overridable via JC_SCHEDULER_CRON_SUBSCRIPTION_EXPIRY_SWEEP.
	schedule("*/30 * * * *", TypeSubscriptionExpirySweep, 30*time.Minute)

	// Hourly: trending, llms.txt, analytics views, RSS, GDPR retention sweep.
	schedule("0 * * * *", TypeComputeTrending, 1*time.Hour)
	schedule("0 * * * *", TypeMaintenanceRegenerateLLMsTxt, 1*time.Hour)
	schedule("0 * * * *", TypeAnalyticsRefreshViews, 1*time.Hour)
	schedule("0 * * * *", TypeRSSRegenerate, 1*time.Hour)
	schedule("0 * * * *", TypeGdprRetentionSweep, 1*time.Hour)

	// §09: incomplete-onboarding reminders — daily at 09:00. Sends are capped
	// by onboarding.reminder_max_count and the account-age window inside the
	// handler; the cron spec is overridable via JC_SCHEDULER_CRON_*.
	schedule("0 9 * * *", TypeOnboardingReminderSweep, 24*time.Hour)

	// 6-hourly: user/space suggestions.
	schedule("0 */6 * * *", TypeComputeSuggestions, 6*time.Hour)

	// Daily 03:00 UTC: security scan.
	schedule("0 3 * * *", TypeSecurityScan, 24*time.Hour)

	// Nightly 02:30 UTC: purge soft-deleted media objects+rows past
	// media.deleted_retention_days (Section 34, Rule 11 approved path).
	schedule("30 2 * * *", TypeMediaCleanupDeleted, 24*time.Hour)

	// Weekly: prune old feeds, prune audit logs, DM cleanup, prune push tokens.
	schedule("0 2 * * 0", TypeMaintenancePruneOldFeeds, 7*24*time.Hour)
	schedule("0 2 * * 0", TypeMaintenancePruneAuditLogs, 7*24*time.Hour)
	schedule("0 2 * * 0", TypeDMCleanupDeclinedRequests, 7*24*time.Hour)
	schedule("0 2 * * 0", TypeMaintenancePrunePushTokens, 7*24*time.Hour)

	// Weekly: delete travel_search_results rows expired > results_retention_days
	// (§67 — stale-fallback retention is bounded, default 30 days).
	schedule("0 2 * * 0", TypeMaintenancePruneTravelResults, 7*24*time.Hour)

	// Weekly: prune expired/revoked OAuth rows past their retention windows
	// (§120 — codes and expired tokens 30d, revoked tokens 90d by default).
	schedule("0 2 * * 0", TypeMaintenancePruneOAuthRecords, 7*24*time.Hour)

	// Daily: pull configured provider hotel-content feeds into travel_hotels
	// (§67, ADR-124 hotel directory).
	schedule("0 3 * * *", TypeTravelSyncHotelDirectory, 24*time.Hour)

	// Coupon expiry check + event flush — hourly.
	schedule("0 * * * *", TypeCouponExpireCheck, 1*time.Hour)
	schedule("5 * * * *", TypeCouponTrackEvents, 1*time.Hour)
	// Deal feed import — hourly fan-out to enabled coupon_feed_sources (§55).
	schedule("30 * * * *", TypeCouponSyncFeeds, 1*time.Hour)

	// §111: pending service bookings past their org-response SLA — hourly.
	// Overridable via JC_SCHEDULER_CRON_SERVICE_BOOKING_EXPIRE_PENDING.
	schedule("15 * * * *", TypeServiceBookingExpirePending, 1*time.Hour)

	// Affiliate link expiry — daily.
	schedule("0 4 * * *", TypeAffiliateExpireCheck, 24*time.Hour)

	// Daily digest emails — 08:00 UTC.
	schedule("0 8 * * *", TypeDigestEmail, 24*time.Hour)

	// Weekly newsletter digest — day/hour from newsletter.digest_day and
	// newsletter.digest_hour_utc system_settings (§62, ADR-080 §5), falling
	// back to JC_NEWSLETTER_DIGEST_DAY / JC_NEWSLETTER_DIGEST_HOUR_UTC env
	// vars, then to the spec defaults (Monday 08:00 UTC). The per-platform
	// settings are read from the global platform (super-admin) row; individual
	// platforms override via the env vars or SchedulerCronSpecs config.
	schedule(newsletterDigestCronSpec(), TypeNewsletterDigest, 7*24*time.Hour)

	// Daily sitemap generation — 02:00 UTC.
	schedule("0 2 * * *", TypeSitemapGenerate, 24*time.Hour)

	// Daily jobs listing expiry — 01:00 UTC (§63, ADR-081).
	schedule("0 1 * * *", TypeJobsExpireListings, 24*time.Hour)

	// Daily scholarship deadline-reminder sweep — 05:00 UTC (§73, ADR-073).
	// Offset from other daily tasks to spread Redis load. The handler
	// iterates platforms, finds saved scholarships whose application_deadline
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

	// Nightly calendar staleness check — 04:00 UTC (§86, ADR-094).
	// Marks recurring entries as 'unverified' when their staleness_check_due
	// date has passed. Low queue.
	schedule("0 4 * * *", TypeCalendarStalenessCheck, 24*time.Hour)

	// Daily calendar reminder dispatch — 08:00 UTC (§86, ADR-094).
	// Dispatches notifications to subscribers at 90/30/7-day marks before
	// each published entry. Default queue.
	schedule("0 8 * * *", TypeCalendarSendReminders, 24*time.Hour)

	// Nightly mutual aid post expiry — 03:00 UTC (§85, ADR-093).
	// Expires posts past their 30-day expires_at and notifies owners
	// 3 days before expiration. Low queue.
	schedule("0 3 * * *", TypeMutualAidExpire, 24*time.Hour)

	// Weekly credential checker link validation — Sunday 02:00 UTC (§94,
	// ADR-102). HEAD-checks regulatory_body_url on active pathways and
	// logs pathways whose URLs return 404. Low queue.
	schedule("0 2 * * 0", TypeCredentialsCheckLinks, 7*24*time.Hour)

	// Weekly visa route link validation — Sunday 01:00 UTC (§90, ADR-098).
	// HEAD-checks gov_source_url on active+published visa routes and logs
	// routes whose URLs return 404 so admins can review. Low queue.
	schedule("0 1 * * 0", TypeVisaRoutesCheckLinks, 7*24*time.Hour)

	// Nightly CV format session purge — 05:00 UTC (§95, ADR-103).
	// Deletes cv_format_sessions rows past their expires_at (24h after
	// creation). Low queue.
	schedule("0 5 * * *", TypeCvFormatSessionsPurge, 24*time.Hour)

	// Daily gamification challenge expiry — 04:00 UTC (§64, ADR-082).
	// Deactivates challenges past their ends_at. Low queue.
	// BUG: was missing from the scheduler — the task was registered as a
	// handler but never enqueued, so expired challenges were never swept.
	schedule("0 4 * * *", TypeGamificationExpireChallenges, 24*time.Hour)

	// Daily JCE stale refresh sweep — 04:15 UTC (Sub-spec 4).
	// Enqueues jce:refresh_values tasks for comparisons whose last completed
	// refresh is older than the configured sweep window. Low queue.
	schedule("15 4 * * *", TypeJceRefreshStale, 24*time.Hour)

	// Nightly embassy alert expiry — 03:00 UTC (§91, ADR-099).
	// Expires appointment alerts past their expires_at date. Low queue.
	// BUG: was missing from the scheduler — the task was registered as a
	// handler but never enqueued, so expired alerts were never swept.
	schedule("0 3 * * *", TypeEmbassyExpireChecks, 24*time.Hour)

	// Recurring embassy poll scheduler — every 5 minutes (§91, ADR-099).
	// Enqueues embassy:poll tasks for each active monitored embassy whose
	// poll_interval_seconds has elapsed since last_polled_at. Low queue.
	// This is the per-embassy poll loop: the scheduler fans out default-
	// tier embassy:poll tasks, each of which scrapes one booking page.
	schedule("*/5 * * * *", TypeEmbassySchedulePolls, 5*time.Minute)

	// Weekly community digest generation — Sunday 22:00 UTC (§84, ADR-092).
	// Assembles a new digest edition from platform analytics (top threads,
	// new members, events, featured story, poll) for each active platform.
	// The edition enters the 1-hour admin review window before auto-publish.
	// Low queue.
	schedule("0 22 * * 0", TypeDigestGenerate, 7*24*time.Hour)

	// Hourly digest auto-publish — top of each hour (§84, ADR-092).
	// Publishes draft/review editions whose 1-hour admin review window has
	// elapsed (scheduled_for <= now()) and enqueues digest:send_email per
	// edition. Idempotent: the UPDATE only matches rows still in
	// draft/review, so overlapping runs across replicas are no-ops (plus
	// the Unique key guards duplicate execution). Low queue.
	schedule("0 * * * *", TypeDigestAutoPublish, 1*time.Hour)

	// Nightly planning group expiry — 03:30 UTC (§82, ADR-090).
	// Soft-deletes planning groups whose expires_at has passed. Low queue.
	schedule("30 3 * * *", TypePlanningGroupExpire, 24*time.Hour)

	// Weekly planning group check-in reminder — Monday 09:00 UTC (§82,
	// ADR-090). Sends check-in reminders to members who haven't checked in
	// this week. Default queue.
	schedule("0 9 * * 1", TypePlanningGroupCheckinReminder, 7*24*time.Hour)

	// Nightly visa timeline stats refresh — 02:00 UTC (§83, ADR-091).
	// Recomputes the visa_processing_stats aggregate table from shared,
	// non-deleted visa_timelines + milestones. Low queue.
	schedule("0 2 * * *", TypeVisaTimelineRefreshStats, 24*time.Hour)

	// Weekly visa timeline update prompt — Monday 09:00 UTC (§83, ADR-091).
	// Nudges members with in-progress timelines to add a milestone. Low queue.
	schedule("0 9 * * 1", TypeVisaTimelinePromptUpdate, 7*24*time.Hour)

	// Remittance rate fetch — every 30 min (§74, ADR-075 §8).
	// The cron spec can be overridden via JC_SCHEDULER_CRON_REMITTANCE_FETCH_RATES.
	// The per-platform fetch interval (remittance.fetch_interval_minutes) is
	// enforced inside the handler — the scheduler fires globally every 30 min
	// and the handler skips platforms whose interval hasn't elapsed.
	schedule("*/30 * * * *", TypeRemittanceFetchRates, 30*time.Minute)

	// Remittance alert check — every 30 min (§74, ADR-075 §4).
	// Evaluates active alerts against current best rates and dispatches
	// notifications for thresholds met (24h debounce per alert).
	schedule("*/30 * * * *", TypeRemittanceCheckAlerts, 30*time.Minute)

	// Weekly remittance rate pruning — Sunday 03:00 UTC (§74, ADR-075 §7).
	// Deletes rate rows older than remittance.history_retention_days
	// (default 365), keeping one snapshot per provider/corridor/day.
	// Low queue.
	schedule("0 3 * * 0", TypeRemittancePruneOldRates, 7*24*time.Hour)

	// Weekly FX matrix pruning — Sunday 03:30 UTC (§112, ADR-125).
	// Deletes fx_rates rows older than fx.history_retention_days (default 90).
	// Low queue.
	schedule("30 3 * * 0", TypeFxPruneRates, 7*24*time.Hour)

	// §60 Referral — daily sweep to surface flagged referrals for admin review.
	// The handler is a no-op placeholder (the flagged queue is exposed via
	// GET /admin/referrals/flagged); the cron exists so the task catalog entry
	// is exercised and future notification wiring has a schedule to attach to.
	// Low queue. BUG-006 fix: this was missing entirely.
	schedule("0 6 * * *", TypeReferralReviewFlagged, 24*time.Hour)

	// Japa Academy — monthly payout aggregation sweep (ADR-063 §6).
	// Runs on the 1st of each month at 00:00 UTC. The handler tolerates an
	// empty (cron-mode) and iterates all active platforms, rolling
	// open revenue_splits into academy_instructor_payouts rows for the previous
	// calendar month. The cadence matches the academy.payout_schedule
	// system_setting default ("monthly"); a future weekly cadence would add
	// a second entry gated by the setting. Low queue. BUG-001 fix: this was
	// missing entirely — payouts were never auto-aggregated.
	// Ref: japaconnect-engineering/adr/ADR-063-japa-academy-creator-economy.md (§6)
	schedule("0 0 1 * *", TypeAcademyCalculatePeriodPayouts, 24*time.Hour)

	// Intelligence System (§INTEL, ADR-112) — two cron entry points.
	// schedule_crawls runs every 15 min on the critical queue: selects due
	// intel_source_channels and enqueues intel:fetch_channel per channel
	// (intel-channels Phase 1 moved the scheduling unit from source to
	// channel). The handler
	// acquires a distributed leader lock (jc:intel:scheduler_leader) so
	// duplicate replicas do not double-enqueue. refresh_freshness runs daily
	// at 04:30 UTC (offset from other 04:00 tasks to spread load): decays
	// freshness scores and re-enqueues fetch_source for stale sources.
	// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md §4
	schedule("*/15 * * * *", TypeIntelScheduleCrawls, 15*time.Minute)
	schedule("30 4 * * *", TypeIntelRefreshFreshness, 24*time.Hour)
	// Daily inbound-event retention sweep at 04:45 UTC (offset from the
	// 04:00/04:30 tasks): deletes intel_inbound_events rows older than
	// intel.inbound.retention_days — raw delivery payloads carry sender
	// PII, so retention is bounded. Low queue.
	schedule("45 4 * * *", TypeIntelPurgeInboundEvents, 24*time.Hour)
	// Daily rejected-record retention sweep at 04:50 UTC (offset from the
	// other 04:xx tasks): deletes intel_records rows in status='rejected'
	// older than intel.rejected_retention_days (default 7). Low queue.
	schedule("50 4 * * *", TypeIntelPruneRejectedRecords, 24*time.Hour)

	// §106 channel-sync amendment — hourly YouTube Atom-feed poll for
	// video_channel directory sources. Runs at :20 to avoid the top-of-hour
	// cron cluster; low queue. New uploads land as video_sources + contents
	// via the same path as bulk import (no YouTube Data API key required).
	schedule("20 * * * *", TypeVideoSyncChannels, 1*time.Hour)

	return scheduler
}

// scheduledTask creates a scheduled Asynq task with the correct queue tier
// and a Unique key to prevent duplicate execution across replicas.
func scheduledTask(taskType string, uniquePeriod time.Duration) *asynq.Task {
	return asynq.NewTask(
		taskType, []byte("{}"),
		asynq.Queue(QueueForTask(taskType)),
		asynq.Unique(uniquePeriod),
	)
}

// Addr returns the Valkey address from the config as a formatted string.
// Useful for logging at startup.
func Addr(cfg *Config) string {
	return fmt.Sprintf("%s (db=%d)", cfg.RedisAddr, cfg.RedisDB)
}

// newsletterDigestCronSpec builds the weekly digest cron expression from
// the newsletter.digest_day (1=Mon..7=Sun) and newsletter.digest_hour_utc
// system_settings (§62, ADR-080 §5). Falls back to JC_NEWSLETTER_DIGEST_DAY
// and JC_NEWSLETTER_DIGEST_HOUR_UTC env vars, then to the spec defaults
// (Monday at 08:00 UTC). The cron day-of-week is 0=Sunday..6=Saturday in
// standard cron, so day 1 (Monday) maps to cron "1", day 7 (Sunday) maps
// to cron "0".
func newsletterDigestCronSpec() string {
	day := 1  // Monday (spec default)
	hour := 8 // 08:00 UTC (spec default)

	if v := os.Getenv("JC_NEWSLETTER_DIGEST_DAY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 7 {
			day = n
		}
	}

	if v := os.Getenv("JC_NEWSLETTER_DIGEST_HOUR_UTC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
			hour = n
		}
	}

	// Map 1=Mon..7=Sun to cron DOW (0=Sun..6=Sat).
	cronDOW := strconv.Itoa(day % 7) // 1→1(Mon)..6→6(Sat),7→0(Sun)

	return fmt.Sprintf("0 %d * * %s", hour, cronDOW)
}
