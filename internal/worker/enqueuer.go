package worker

import (
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

// Enqueuer adapts *asynq.Client to the string/[]byte enqueue interface used by
// the feature services (gdpr, affiliate, coupons, tickets, analytics), whose
// Enqueuer interfaces all have the shape:
//
//	Enqueue(taskType string, payload []byte, opts ...interface{}) error
//
// In development those services fall back to a per-package noopEnqueuer that
// silently drops every task. Wiring this adapter in server setup makes their
// background jobs actually run (e.g. GDPR account-deletion cleanup + Honcho
// erasure, affiliate commission crediting, coupon expiry, ticket status
// emails, analytics event flushing).
type Enqueuer struct {
	client *asynq.Client
}

// NewEnqueuer wraps an *asynq.Client so it satisfies the feature-service
// Enqueuer interfaces via structural typing.
func NewEnqueuer(client *asynq.Client) *Enqueuer {
	return &Enqueuer{client: client}
}

// Enqueue builds an asynq.Task from the task type and payload and enqueues it
// on the shared client. Queue tier is inferred from the task type via the
// tier lookup table. High-risk tasks get asynq.Unique() idempotency by
// default. Any variadic opts that are asynq.Option values are forwarded to the
// client; any other values are ignored so callers that pass nothing (the
// common case) work unchanged.
func (e *Enqueuer) Enqueue(taskType string, payload []byte, opts ...interface{}) error {
	if e == nil {
		return nil // not wired — graceful degradation (tests, development)
	}
	if e.client == nil {
		return errors.New("worker: asynq client not configured — task " + taskType + " dropped")
	}
	queue := QueueForTask(taskType)
	asynqOpts := []asynq.Option{asynq.Queue(queue)}

	// Apply task-type–specific defaults before caller opts (caller can override).
	for _, defaultOpt := range taskDefaults(taskType) {
		asynqOpts = append(asynqOpts, defaultOpt)
	}

	for _, o := range opts {
		if opt, ok := o.(asynq.Option); ok {
			asynqOpts = append(asynqOpts, opt)
		}
	}
	_, err := e.client.Enqueue(asynq.NewTask(taskType, payload), asynqOpts...)
	return err
}

// taskDefaults returns default asynq options for a given task type. High-risk
// tasks that would cause user-visible damage on duplicate processing get a
// Unique() key. One-shot tasks (publish, newsletter sends, commission credits)
// get a 24h TTL; notification/email tasks get a 1h TTL via this method plus a
// shorter TTL via the task-level Unique call.
var idempotentTasks = map[string]bool{
	TypeEmailSend:                       true,
	TypePushDelivery:                    true,
	TypeScheduledPublish:                true,
	TypeNewsletterSend:                  true,
	TypeAffiliateCreditCommission:       true,
	TypeAffiliateCreditAuthorCommission: true, // TASK-005: defense-in-depth for author commission
	TypeWalletProcessActivityEvent:      true,
	TypeWalletAwardPoints:               true,
	TypePaymentWebhookProcess:           true,
	TypeUniversityProcessEnrollment:     true, // AUDIT-BE-005: double-credit guard
	TypeWalletProcessCreditPurchase:     true, // Section 58: double-credit guard
}

func taskDefaults(taskType string) []asynq.Option {
	opts := []asynq.Option{}

	// Idempotency keys for high-risk tasks.
	if idempotentTasks[taskType] {
		ttl := 1 * time.Hour
		switch taskType {
		case TypeScheduledPublish, TypeNewsletterSend, TypeAffiliateCreditCommission, TypeAffiliateCreditAuthorCommission:
			ttl = 24 * time.Hour
		}
		opts = append(opts, asynq.Unique(ttl))
	}

	// Per-task timeout and retry limits.
	switch taskType {
	// External-service tasks: short timeout, more retries.
	case TypeEmailSend, TypeTicketSendConfirmation, TypeTicketSendStatusUpdate,
		TypePushDelivery, TypeDMNotifyRequest, TypeDMNotifyMessage:
		opts = append(opts, asynq.Timeout(30*time.Second), asynq.MaxRetry(10))
	// Webhook delivery: moderate timeout.
	case TypeWebhookDelivery, TypePaymentWebhookProcess:
		opts = append(opts, asynq.Timeout(60*time.Second), asynq.MaxRetry(15))
	// DB-only tasks: standard timeout.
	case TypeScheduledPublish, TypeScheduledPublishSweep:
		opts = append(opts, asynq.Timeout(1*time.Minute), asynq.MaxRetry(5))
		// Default: no explicit timeout/retry — asynq defaults apply.
	}

	return opts
}

// taskTier maps each task type string to its queue tier. Entries are explicit
// per JEOS asynq-tasks.md so the Control Panel cron editor and QueueForTask
// reflect the documented tier for every registered task. Unknown task types
// fall back to QueueDefault.
var taskTier = func() map[string]string {
	m := make(map[string]string, 70)

	// Critical tier
	for _, t := range []string{
		TypeNotificationDispatchBatch, TypeWebhookDelivery, TypePaymentWebhookProcess,
		TypeTicketSendConfirmation, TypePushDelivery, TypeDMNotifyRequest, TypeDMNotifyMessage,
		TypeUniversityProcessEnrollment, TypeUniversitySendInstructorPayout,
		TypeAffiliateCreditCommission, // BUG-012: financial side effect → critical (§56)
		TypeNotificationSendPush,       // §77 V2 — push delivery is user-facing-critical
	} {
		m[t] = QueueCritical
	}

	// Default tier (explicit entries for tasks documented as default in JEOS
	// asynq-tasks.md — the fallback in QueueForTask is also default, but
	// listing them here keeps the tier map authoritative and auditable).
	for _, t := range []string{
		TypeEmailSend, TypeSearchIndexSync, TypeImageProcessing,
		TypeAITaskProcessing, TypeEngagementCountUpdate, TypeModerationScan,
		TypeScheduledPublish, TypeLinkPreviewFetch, TypeLanguagePackAITranslate,
		TypeMediaScan, TypeContentFanOutToFeeds, TypeAIGenerateContentEmbedding,
		TypeContentGenerateDraft,
		TypeReviewAggregateRecalc, TypeTicketSendStatusUpdate,
		TypeLearningUpdateTrackCompletion, TypeVideoProcessing,
		TypeWalletProcessActivityEvent, TypeNewsletterSend,
		TypeAffiliateCreditAuthorCommission,
		TypeDocsGeneratePage,
		TypeFormFireSubmissionTriggers,
		TypeWalletProcessCreditPurchase,
		TypeWalletAwardPoints, TypeWalletDeductCredits,
		TypeUniversityNotifyInstructorApproved, TypeUniversityNotifyCourseApproved,
		// V2 default-tier tasks (§§60-76)
		TypeAssessmentScoreSubmission,
		TypeStudioRunBatch,
		TypePersonaRunActivity,
		TypePersonaDispatchAction,
		TypeRemittanceFetchRates,
		TypeRemittanceCheckAlerts,
		TypeReferralRecordClick,
		TypeReferralCheckConversion,
		TypeAdsRefundCampaignCredits,
		TypeAdsNotifyCampaignStatus,
		TypeJobsNotifyApplicationStatus,
		TypeImportProcessSource,
	} {
		m[t] = QueueDefault
	}

	// Low tier
	for _, t := range []string{
		TypeDigestEmail, TypeSitemapGenerate, TypeRSSRegenerate,
		TypeDataExport, TypeAccountDeletionCleanup, TypeAdEventIngest,
		TypeMaintenanceUpdateVelocityScores, TypeMaintenanceCheckSLABreaches,
		TypeMaintenanceUpdateGeoIPDB, TypeSpaceAutoPopulate,
		TypeAnalyticsRefreshViews, TypeSecurityScan,
		TypeMaintenanceRegenerateLLMsTxt, TypeLearningIssueCertificate,
		TypeDBManualVacuum, TypeNewsletterDigest,
		TypeComputeTrending, TypeComputeSuggestions,
		TypeMaintenanceCheckExpiredBans, TypeMaintenanceCheckExpiredBoosts,
		TypeMaintenancePruneOldFeeds, TypeMaintenancePruneAuditLogs,
		TypeMaintenancePrunePushTokens, TypeModerationAutoHide,
		TypeDMCleanupDeclinedRequests, TypeCouponTrackEvents,
		TypeTenantCleanupData,
		TypeCouponExpireCheck, TypeAffiliateRecordClick,
		TypeAffiliateExpireCheck,
		TypeRedirectRecordHit,
		TypeScheduledPublishSweep, TypeGdprRetentionSweep,
		TypeUniversityCalculatePeriodPayouts, TypeUniversityFetchLogo,
		TypeContentScanAffiliateLinks,
		// V2 low-tier tasks (§§60-76)
		TypeStudioRetryFailed,
		TypeRemittancePruneOldRates,
		TypeReferralReviewFlagged,
		TypeScholarshipCheckDeadlines,
		TypeJobsExpireListings,
		TypeGamificationExpireChallenges,
		TypeImportFetchMedia,
	} {
		m[t] = QueueLow
	}

	return m
}()

// QueueForTask returns the Asynq queue tier name for a given task type string.
// Default tier tasks and unknown task types return QueueDefault.
func QueueForTask(taskType string) string {
	if q, ok := taskTier[taskType]; ok {
		return q
	}
	return QueueDefault
}

// TaskTypeCatalog returns the full Asynq task type catalog with name, tier, and
// description for use by the control panel cron job editor UI.
func TaskTypeCatalog() []map[string]string {
	tasks := []struct {
		Name        string
		Tier        string
		Description string
	}{
		// Critical tier
		{TypeNotificationDispatchBatch, "critical", "Batch send notifications"},
		{TypeWebhookDelivery, "critical", "Deliver outbound webhook events"},
		{TypePaymentWebhookProcess, "critical", "Process inbound payment webhook"},
		{TypeUniversityProcessEnrollment, "critical", "Credit creator ledger for a paid course enrollment"},
		{TypeUniversitySendInstructorPayout, "critical", "Execute a single instructor payout transfer"},
		{TypeTicketSendConfirmation, "critical", "Send ticket confirmation email"},
		{TypePushDelivery, "critical", "Send push notifications"},
		{TypeDMNotifyRequest, "critical", "Notify DM request received"},
		{TypeDMNotifyMessage, "critical", "Notify new DM message"},
		// Default tier
		{TypeEmailSend, "default", "Send transactional email"},
		{TypeSearchIndexSync, "default", "Index entity in Meilisearch"},
		{TypeImageProcessing, "default", "Resize/optimize uploaded images"},
		{TypeAITaskProcessing, "default", "Process AI task"},
		{TypeEngagementCountUpdate, "default", "Update engagement counters"},
		{TypeModerationScan, "default", "Run moderation scan on content"},
		{TypeScheduledPublish, "default", "Publish scheduled content"},
		{TypeLinkPreviewFetch, "default", "Fetch link preview metadata"},
		{TypeLanguagePackAITranslate, "default", "AI-translate language pack"},
		{TypeMediaScan, "default", "Scan uploaded media for policy violations"},
		{TypeContentFanOutToFeeds, "default", "Fan out content to follower feeds"},
		{TypeAIGenerateContentEmbedding, "default", "Generate vector embedding for content"},
		{TypeReviewAggregateRecalc, "default", "Recalculate review aggregate scores"},
		{TypeTicketSendStatusUpdate, "default", "Send ticket status change email"},
		{TypeLearningUpdateTrackCompletion, "default", "Update learning track completion"},
		{TypeVideoProcessing, "default", "Process uploaded video files"},
		{TypeWalletProcessActivityEvent, "default", "Process wallet activity event"},
		{TypeWalletProcessCreditPurchase, "default", "Credit wallet on credit purchase payment success"},
		{TypeWalletAwardPoints, "default", "Award points or credits to a user wallet (checklist, referral, assessment, scholarship)"},
		{TypeWalletDeductCredits, "default", "Deduct or refund credits for self-serve ad campaign funding"},
		{TypeNewsletterSend, "default", "Send newsletter to subscribers"},
		{TypeDocsGeneratePage, "default", "Generate a docs page via AI (fire-and-forget)"},
		{TypeAffiliateCreditAuthorCommission, "default", "Credit author revenue share for an external affiliate commission earned on their content"},
		// Japa University (ADR-063)
		{TypeUniversityNotifyInstructorApproved, "default", "Notify creator their instructor application was approved"},
		{TypeUniversityNotifyCourseApproved, "default", "Notify creator their track was approved to the marketplace"},
		// Low tier
		{TypeDigestEmail, "low", "Send digest email"},
		{TypeSitemapGenerate, "low", "Generate sitemap XML"},
		{TypeRSSRegenerate, "low", "Regenerate RSS feed XML"},
		{TypeDataExport, "low", "Generate GDPR data export"},
		{TypeAccountDeletionCleanup, "low", "GDPR account deletion cleanup"},
		{TypeAdEventIngest, "low", "Ingest ad impression/click events"},
		{TypeMaintenanceUpdateVelocityScores, "low", "Update velocity scores"},
		{TypeMaintenanceCheckSLABreaches, "low", "Check ticket SLA breaches"},
		{TypeMaintenanceUpdateGeoIPDB, "low", "Update GeoIP database"},
		{TypeSpaceAutoPopulate, "low", "Auto-populate space content"},
		{TypeAnalyticsRefreshViews, "low", "Refresh analytics view counts"},
		{TypeSecurityScan, "low", "Run security scan"},
		{TypeMaintenanceRegenerateLLMsTxt, "low", "Regenerate llms.txt file"},
		{TypeLearningIssueCertificate, "low", "Issue learning certificate"},
		{TypeDBManualVacuum, "low", "Run database VACUUM"},
		{TypeNewsletterDigest, "low", "Send newsletter digest"},
		{TypeComputeTrending, "low", "Compute trending content"},
		{TypeComputeSuggestions, "low", "Compute content suggestions"},
		{TypeMaintenanceCheckExpiredBans, "low", "Check and lift expired user bans"},
		{TypeMaintenanceCheckExpiredBoosts, "low", "Check and expire content boosts"},
		{TypeMaintenancePruneOldFeeds, "low", "Prune old feed entries"},
		{TypeMaintenancePruneAuditLogs, "low", "Prune old audit logs"},
		{TypeMaintenancePrunePushTokens, "low", "Prune stale push tokens"},
		{TypeModerationAutoHide, "low", "Auto-hide heavily reported content"},
		{TypeDMCleanupDeclinedRequests, "low", "Clean up declined DM requests"},
		{TypeCouponTrackEvents, "low", "Track coupon usage events"},
		{TypeCouponExpireCheck, "low", "Expire outdated coupons"},
		{TypeAffiliateRecordClick, "low", "Record affiliate link click"},
		{TypeAffiliateCreditCommission, "low", "Credit affiliate commission"},
		{TypeAffiliateExpireCheck, "low", "Expire stale affiliate links"},
		{TypeContentScanAffiliateLinks, "low", "Scan content for unregistered affiliate links and record results for moderation"},
		{TypeRedirectRecordHit, "low", "Record URL redirect hit"},
		{TypeTenantCleanupData, "low", "Hard-delete tenant data after 30-day grace period"},
		{TypeScheduledPublishSweep, "low", "Sweep and publish due scheduled content"},
		{TypeGdprRetentionSweep, "low", "Process GDPR retention-wait deletions"},
		{TypeUniversityCalculatePeriodPayouts, "low", "Roll open revenue splits into instructor payouts for a period"},
		{TypeUniversityFetchLogo, "low", "Fetch and store a university logo from a remote URL during space auto-seeding"},
	}

	result := make([]map[string]string, len(tasks))
	for i, t := range tasks {
		result[i] = map[string]string{
			"value":       t.Name,
			"label":       t.Name,
			"tier":        t.Tier,
			"description": t.Description,
		}
	}
	return result
}
