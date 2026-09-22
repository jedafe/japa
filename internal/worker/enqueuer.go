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

	asynqOpts := TaskOpts(taskType)

	for _, o := range opts {
		if opt, ok := o.(asynq.Option); ok {
			asynqOpts = append(asynqOpts, opt)
		}
	}

	_, err := e.client.Enqueue(asynq.NewTask(taskType, payload), asynqOpts...)

	return err
}

// TaskOpts returns the queue tier + per-type timeout/retry defaults for a
// task — the same options enqueueIntel and Enqueue apply. Callers that
// must EnqueueContext directly (HTTP handlers enqueuing intel tasks)
// spread this so direct enqueues honor the documented tiers instead of
// silently landing on the default queue.
func TaskOpts(taskType string) []asynq.Option {
	return append([]asynq.Option{asynq.Queue(QueueForTask(taskType))}, taskDefaults(taskType)...)
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
	// Assessment scoring: pure computation + DB writes. Generous timeout
	// for DB contention headroom; 5 retries with exponential backoff
	// ensures a submission is eventually scored even if the worker is
	// temporarily unavailable (§71, ADR-071).
	case TypeAssessmentScoreSubmission:
		opts = append(opts, asynq.Timeout(2*time.Minute), asynq.MaxRetry(5))
	// Source metadata enrichment: external HTTP fetch, short timeout,
	// limited retries because failures are often site-side and permanent.
	// Channel curation shares the shape: local parse + DB writes, and a
	// deterministic parse failure should not burn many retries.
	case TypeIntelSourceMetadata, TypeIntelCurateChannel:
		opts = append(opts, asynq.Timeout(2*time.Minute), asynq.MaxRetry(3))
	// Channel sync: external fetches (channel page + Atom feed + per-video
	// oEmbed) across a batch of sources — generous timeout, few retries.
	case TypeVideoSyncChannels:
		opts = append(opts, asynq.Timeout(10*time.Minute), asynq.MaxRetry(3))
	// Verify probe: bounded external fetches (up to
	// intel.probe.verify_max_requests) plus channel materialization.
	case TypeIntelProbeVerify:
		opts = append(opts, asynq.Timeout(5*time.Minute), asynq.MaxRetry(2))
	// XHR sniff: one headless HAR capture + channel materialization;
	// bounded like the verify probe but lighter.
	case TypeIntelXHRSniff:
		opts = append(opts, asynq.Timeout(2*time.Minute), asynq.MaxRetry(2))
	// Inbound email: MIME parse + raw doc + detect_changes enqueue — all
	// local work, short timeout; retries cover transient DB failures.
	case TypeIntelInboundEmail:
		opts = append(opts, asynq.Timeout(1*time.Minute), asynq.MaxRetry(3))
	// WebSub subscribe/renew: one hub POST + self re-enqueue.
	case TypeIntelWebSubSubscribe, TypeIntelWebSubRenew:
		opts = append(opts, asynq.Timeout(1*time.Minute), asynq.MaxRetry(3))
	// Search poll: one SERP request + the shared fetch bookkeeping.
	case TypeIntelSearchPoll:
		opts = append(opts, asynq.Timeout(2*time.Minute), asynq.MaxRetry(3))
	}
	// Default: no explicit timeout/retry — asynq defaults apply.

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
		TypeNotificationSendPush,      // §77 V2 — push delivery is user-facing-critical
		TypeEmbassyDispatchAlerts,     // §91 — time-sensitive slot availability (ADR-099)
		TypeIntelScheduleCrawls,       // §INTEL — cron entry point, must fire immediately (ADR-112)
		TypeIntelInboundEmail,         // §INTEL Phase 3 — provider push deliveries are time-sensitive
	} {
		m[t] = QueueCritical
	}

	// Default tier (explicit entries for tasks documented as default in JEOS
	// asynq-tasks.md — the fallback in QueueForTask is also default, but
	// listing them here keeps the tier map authoritative and auditable).
	for _, t := range []string{
		TypeEmailSend, TypeSearchIndexSync, TypeSearchReindexEngagementScores,
		TypeImageProcessing,
		TypeAITaskProcessing, TypeEngagementCountUpdate, TypeModerationScan,
		TypeScheduledPublish, TypeLinkPreviewFetch, TypeLanguagePackAITranslate,
		TypeMediaScan, TypeContentFanOutToFeeds, TypeAIGenerateContentEmbedding,
		TypeContentGenerateDraft,
		TypeContentRegenerateFromSource,
		TypeAgentCreateDraftRequest,
		TypeJCERefreshValues,
		TypeReviewAggregateRecalc, TypeTicketSendStatusUpdate,
		TypeLearningUpdateTrackCompletion, TypeVideoProcessing, TypeVideoImportBulk,
		TypeWalletProcessActivityEvent, TypeNewsletterSend,
		TypeScholarshipSyncFromGuide,
		TypeAffiliateCreditAuthorCommission,
		TypeDocsGeneratePage,
		TypeFormFireSubmissionTriggers,
		TypeWalletProcessCreditPurchase,
		TypeWalletAwardPoints, TypeWalletDeductCredits,
		TypeUniversityNotifyInstructorApproved, TypeUniversityNotifyCourseApproved,
		// V2 default-tier tasks (§§60-76)
		TypeAssessmentScoreSubmission,
		TypeAssessmentCleanupAnonymous,
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
		TypeAffiliateRecordClick,
		// §91 Embassy Appointment Alerter (ADR-099) — default tier per spec.
		TypeEmbassyPoll,
		// §86 Community Calendar (ADR-094) — daily reminders on default tier.
		TypeCalendarSendReminders,
		// §95 CV Formatter (ADR-103) — AI formatting on default tier.
		TypeCvFormatProcess,
		// §84 Community Digest Widget (ADR-092) — email send on publish, default tier.
		TypeDigestSendEmail,
		// §87 Japa Stage Profiles (ADR-095) — stage-change recommendation fan-out, default tier.
		TypeJapaStageRecommend,
		// §82 Japa Planning Groups (ADR-090) — weekly check-in reminder, default tier.
		TypePlanningGroupCheckinReminder,
		// §81 Scam Alert Board (ADR-089) — confirm-event notification fan-out, default tier.
		TypeScamAlertNotifySubscribers,
		// §51 Content Reports — notify moderation team of a new report, default tier.
		TypeReportNotifyAdmin,
		// §INTEL — ingestion spine tasks, default tier (ADR-112).
		TypeIntelFetchSource,
		// §intel-channels Phase 1 — per-channel fetch, default tier.
		TypeIntelFetchChannel,
		TypeIntelDetectChanges,
		// §intel-channels Phase 1 — deterministic curate pipeline, default tier.
		TypeIntelCurateChannel,
		TypeIntelExtract,
		TypeIntelDedup,
		TypeIntelPublishGate,
		TypeIntelPromote,
		TypeIntelPostPublish,
		TypeIntelNotifyDiscoveries,
		TypeIntelRetryFailedSource,
		// §INTEL — source metadata enrichment, default tier.
		TypeIntelSourceMetadata,
		// §intel-channels — deep verify probe on approval, default tier.
		TypeIntelProbeVerify,
		// §intel-channels Phase 3 — SERP provider poll, default tier.
		TypeIntelSearchPoll,
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
		TypeDBManualVacuum, TypeNewsletterDigest, TypeVideoGenerateSummary,
		TypeComputeTrending, TypeComputeSuggestions,
		TypeMaintenanceCheckExpiredBans, TypeMaintenanceCheckExpiredBoosts,
		TypeMaintenancePruneOldFeeds, TypeMaintenancePruneAuditLogs,
		TypeMaintenancePrunePushTokens, TypeModerationAutoHide,
		TypeDMCleanupDeclinedRequests, TypeCouponTrackEvents,
		TypeCouponExpireCheck,
		TypeAffiliateExpireCheck,
		TypeRedirectRecordHit,
		TypeScheduledPublishSweep, TypeGdprRetentionSweep, TypeJceRefreshStale,
		TypeUniversityCalculatePeriodPayouts, TypeUniversityFetchLogo,
		TypeContentScanAffiliateLinks,
		TypePlatformDataExport, TypePlatformDataImport,
		// V2 low-tier tasks (§§60-76)
		TypeStudioRetryFailed,
		TypeRemittancePruneOldRates,
		TypeReferralReviewFlagged,
		TypeScholarshipCheckDeadlines,
		TypeJobsExpireListings,
		TypeGamificationExpireChallenges,
		TypeImportFetchMedia,
		// §102 Mentorship Matching (ADR-110) — low queue per spec.
		TypeMentorGenerateEmbedding,
		TypeMentorUpdateMeilisearch,
		// §91 Embassy Appointment Alerter (ADR-099) — nightly sweep, low tier.
		TypeEmbassyExpireChecks,
		// §91 Embassy Appointment Alerter (ADR-099) — recurring poll
		// scheduler runs every 5 min, low tier (enqueues default-tier
		// embassy:poll tasks per embassy).
		TypeEmbassySchedulePolls,
		// §86 Community Calendar (ADR-094) — nightly staleness check, low tier.
		TypeCalendarStalenessCheck,
		// §85 Mutual Aid Board (ADR-093) — nightly expiry sweep, low tier.
		TypeMutualAidExpire,
		// §94 Professional Credential Checker (ADR-102) — weekly link check, low tier.
		TypeCredentialsCheckLinks,
		// §90 Visa Route Comparison (ADR-098) — weekly link check, low tier.
		TypeVisaRoutesCheckLinks,
		// §95 CV Formatter (ADR-103) — nightly purge of expired sessions, low tier.
		TypeCvFormatSessionsPurge,
		// §84 Community Digest Widget (ADR-092) — weekly generation +
		// hourly auto-publish of editions past the 1-hour review window,
		// low tier.
		TypeDigestGenerate,
		TypeDigestAutoPublish,
		// §82 Japa Planning Groups (ADR-090) — nightly expiry sweep, low tier.
		TypePlanningGroupExpire,
		// §83 Visa Timeline Tracker (ADR-091) — nightly stats refresh +
		// weekly update prompt, low tier.
		TypeVisaTimelineRefreshStats,
		TypeVisaTimelinePromptUpdate,
		// §INTEL — daily freshness decay cron, low tier (ADR-112).
		TypeIntelRefreshFreshness,
		// §106 channel-sync amendment — hourly Atom-feed poll, low tier.
		TypeVideoSyncChannels,
		// §intel-channels Phase 2 — headless XHR sniff, low tier.
		TypeIntelXHRSniff,
		// §intel-channels Phase 3 — hub subscription lifecycle, low tier.
		TypeIntelWebSubSubscribe,
		TypeIntelWebSubRenew,
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

// taskTypeCatalog is the full Asynq task type catalog (name, tier, description)
// backing TaskTypeCatalog.
var taskTypeCatalog = []struct {
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
	{TypeNotificationSendPush, "critical", "Dispatch push notification to FCM/APNs for a device token (§77, ADR-074)"},
	// §INTEL — cron entry point for intelligence crawl scheduling, critical tier (ADR-112).
	{TypeIntelScheduleCrawls, "critical", "Cron entry point for intelligence crawl scheduling"},
	// Default tier
	{TypeEmailSend, "default", "Send transactional email"},
	{TypeSearchIndexSync, "default", "Index entity in Meilisearch"},
	{TypeSearchReindexEngagementScores, "default", "Batched re-index of content whose engagement_score changed (§13)"},
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
	{TypeAgentCreateDraftRequest, "default", "Create draft content from external sources for admin review (Section 103, Phase 8)"},
	{TypeJCERefreshValues, "default", "Refresh stale/missing comparison values for a JCE comparison"},
	{TypeReviewAggregateRecalc, "default", "Recalculate review aggregate scores"},
	{TypeTicketSendStatusUpdate, "default", "Send ticket status change email"},
	{TypeLearningUpdateTrackCompletion, "default", "Update learning track completion"},
	{TypeVideoProcessing, "default", "Process uploaded video files"},
	{TypeVideoImportBulk, "default", "Bulk import YouTube channel, playlist, or keyword results as video sources"},
	{TypeVideoGenerateSummary, "low", "AI-generated title/body draft from video metadata (§106, ADR-117)"},
	{TypeVideoSyncChannels, "low", "Hourly poll of video_channel sources' YouTube Atom feeds; imports new uploads as video_sources + contents (§106, ADR-117)"},
	{TypeWalletProcessActivityEvent, "default", "Process wallet activity event"},
	{TypeWalletProcessCreditPurchase, "default", "Credit wallet on credit purchase payment success"},
	{TypeWalletAwardPoints, "default", "Award points or credits to a user wallet (checklist, referral, assessment, scholarship)"},
	{TypeWalletDeductCredits, "default", "Deduct or refund credits for self-serve ad campaign funding"},
	{TypeNewsletterSend, "default", "Send newsletter to subscribers"},
	{TypeScholarshipSyncFromGuide, "default", "Derive or update a scholarships projection row from a published scholarship guide (§73)"},
	{TypeDocsGeneratePage, "default", "Generate a docs page via AI (fire-and-forget)"},
	{TypeAffiliateCreditAuthorCommission, "default", "Credit author revenue share for an external affiliate commission earned on their content"},
	// Japa University (ADR-063)
	{TypeUniversityNotifyInstructorApproved, "default", "Notify creator their instructor application was approved"},
	{TypeUniversityNotifyCourseApproved, "default", "Notify creator their track was approved to the marketplace"},
	// V2 default-tier tasks (§§60-76, ADRs 068-085)
	{TypeContentGenerateDraft, "default", "Generate a draft content item via the AI Gateway (studio/persona/agent)"},
	{TypeContentRegenerateFromSource, "default", "Rewrite an existing content row from a linked intelligence extraction (§75, ADR-076 Amendment 2)"},
	{TypeFormFireSubmissionTriggers, "default", "Fire form submission triggers (assign_user_group / send_email / send_webhook / create_ticket / notify_admin)"},
	{TypeAssessmentScoreSubmission, "default", "Score a completed readiness assessment, generate roadmap, notify (§71, ADR-071)"},
	{TypeAssessmentCleanupAnonymous, "default", "Hourly cleanup of anonymous assessment_results older than 24h (§71)"},
	{TypeStudioRunBatch, "default", "Enqueue individual content.generate_draft jobs for a studio batch (§75, ADR-076)"},
	{TypePersonaRunActivity, "default", "Persona orchestration loop — select, generate, dispatch or queue actions (§76, ADR-077)"},
	{TypePersonaDispatchAction, "default", "Execute a single approved queued persona action (§76)"},
	{TypeRemittanceFetchRates, "default", "Fetch live rates from API-sourced remittance providers (§74, ADR-075)"},
	{TypeRemittanceCheckAlerts, "default", "Check rate alerts against current best rate, dispatch notifications (§74)"},
	{TypeReferralRecordClick, "default", "Persist referral click + set Valkey attribution key (§60, ADR-078)"},
	{TypeReferralCheckConversion, "default", "On payment — fraud check, status transition, credit award (§60)"},
	{TypeAdsRefundCampaignCredits, "default", "Refund unused credits on campaign rejection or early end (§61, ADR-079)"},
	{TypeAdsNotifyCampaignStatus, "default", "Notify org of self-serve campaign review decision (§61)"},
	{TypeJobsNotifyApplicationStatus, "default", "Notify applicant of job application status change (§63, ADR-081)"},
	{TypeImportProcessSource, "default", "Parse import source, create draft contents rows (§65, ADR-083)"},
	// §86 Community Calendar (ADR-094) — daily reminders on default tier.
	{TypeCalendarSendReminders, "default", "Dispatch notifications to subscribers at 90/30/7-day marks before each published entry (§86)"},
	// §87 Japa Stage Profiles (ADR-095) — stage-change recommendation fan-out, default tier.
	{TypeJapaStageRecommend, "default", "Generate stage-transition recommendations on PATCH /me/japa-stage event (§87)"},
	// §INTEL — intelligence pipeline tasks, default tier (ADR-112).
	{TypeIntelFetchSource, "default", "Fetch raw document from a single intelligence source"},
	{TypeIntelFetchChannel, "default", "Fetch raw document for a single intel source channel (§intel-channels Phase 1)"},
	{TypeIntelDetectChanges, "default", "Detect content changes vs prior crawl hash"},
	{TypeIntelCurateChannel, "default", "Deterministic no-LLM curation of a channel's raw document into intel records (§intel-channels Phase 1)"},
	{TypeIntelExtract, "default", "AI extraction of structured fields from raw document"},
	{TypeIntelDedup, "default", "Dedup check via (source_id, external_key) uniqueness"},
	{TypeIntelPublishGate, "default", "Confidence x trust-tier routing decision"},
	{TypeIntelPromote, "default", "Promote intel record to scholarships + contents projection"},
	{TypeIntelPostPublish, "default", "Post-publish fan-out: Meilisearch index + RAG embedding + event publish"},
	{TypeIntelNotifyDiscoveries, "default", "Fan-out discovery notifications to country space followers"},
	{TypeIntelRetryFailedSource, "default", "Manual retry trigger for a failed intelligence source"},
	{TypeIntelSourceMetadata, "default", "Fetch and store public-site metadata for a source"},
	{TypeIntelProbeVerify, "default", "Deep verify probe for an approved intel source; writes probe_report and materializes channels"},
	{TypeIntelXHRSniff, "low", "Headless XHR/fetch capture; materializes discovered api channels (§intel-channels Phase 2)"},
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
	{TypeReportNotifyAdmin, "default", "Notify moderation team of a new report"},
	{TypeDMCleanupDeclinedRequests, "low", "Clean up declined DM requests"},
	{TypeCouponTrackEvents, "low", "Track coupon usage events"},
	{TypeCouponExpireCheck, "low", "Expire outdated coupons"},
	{TypeAffiliateRecordClick, "default", "Record affiliate link click"},
	{TypeAffiliateCreditCommission, "critical", "Credit affiliate commission (financial side effect — §56, BUG-012)"},
	{TypeAffiliateExpireCheck, "low", "Expire stale affiliate links"},
	{TypeContentScanAffiliateLinks, "low", "Scan content for unregistered affiliate links and record results for moderation"},
	{TypeRedirectRecordHit, "low", "Record URL redirect hit"},
	{TypeScheduledPublishSweep, "low", "Sweep and publish due scheduled content"},
	{TypeGdprRetentionSweep, "low", "Process GDPR retention-wait deletions"},
	{TypeJceRefreshStale, "low", "Enqueue refresh tasks for stale JCE comparisons"},
	{TypeUniversityCalculatePeriodPayouts, "low", "Roll open revenue splits into instructor payouts for a period"},
	{TypeUniversityFetchLogo, "low", "Fetch and store a university logo from a remote URL during space auto-seeding"},
	{TypePlatformDataExport, "low", "Generate platform data export file (CSV/JSON/XLSX) from platform data (Section 67)"},
	{TypePlatformDataImport, "low", "Parse platform data import file and insert/update rows (Section 67)"},
	// §102 Mentorship Matching (ADR-110) — low queue per spec.
	{TypeMentorGenerateEmbedding, "low", "Generate pgvector embedding for a mentor's bio"},
	{TypeMentorUpdateMeilisearch, "low", "Sync a mentor's profile to the Meilisearch mentors index"},
	// §91 Embassy Appointment Alerter (ADR-099)
	{TypeEmbassyPoll, "default", "Poll a monitored embassy booking page for open appointment slots"},
	{TypeEmbassyDispatchAlerts, "critical", "Dispatch notifications to users with a matching active appointment alert"},
	{TypeEmbassyExpireChecks, "low", "Expire appointment alerts past their expires_at date"},
	{TypeEmbassySchedulePolls, "low", "Enqueue embassy:poll tasks for active embassies whose poll interval has elapsed"},
	{TypeCredentialsCheckLinks, "low", "HEAD-check regulatory_body_url on active credential pathways for 404s"},
	// §90 Visa Route Comparison (ADR-098)
	{TypeVisaRoutesCheckLinks, "low", "HEAD-check gov_source_url on active+published visa routes for 404s"},
	// §95 CV Formatter (ADR-103)
	{TypeCvFormatProcess, "default", "Process a pending CV format session via the AI Gateway"},
	{TypeCvFormatSessionsPurge, "low", "Purge expired CV format sessions past their expires_at"},
	// §84 Community Digest Widget (ADR-092)
	{TypeDigestGenerate, "low", "Assemble a new community digest edition from platform analytics"},
	{TypeDigestAutoPublish, "low", "Publish draft/review digest editions past their 1-hour review window and enqueue email send (§84)"},
	{TypeDigestSendEmail, "default", "Send a published digest edition to subscribed members"},
	// §82 Japa Planning Groups (ADR-090)
	{TypePlanningGroupExpire, "low", "Soft-delete planning groups past their expires_at date"},
	{TypePlanningGroupCheckinReminder, "default", "Send weekly check-in reminders to planning group members who haven't checked in"},
	// §81 Scam Alert Board (ADR-089)
	{TypeScamAlertNotifySubscribers, "default", "Fan out scam-alert notifications to subscribers on report confirmation + award Community Guardian badge"},
	// §83 Visa Timeline Tracker (ADR-091)
	{TypeVisaTimelineRefreshStats, "low", "Recompute visa_processing_stats aggregate from shared timelines + milestones (nightly)"},
	{TypeVisaTimelinePromptUpdate, "low", "Prompt members with in-progress visa timelines to add a milestone (weekly)"},
	// V2 low-tier tasks (§§60-76, ADRs 078-085)
	{TypeStudioRetryFailed, "low", "Retry all failed jobs in a studio batch (§75)"},
	{TypeRemittancePruneOldRates, "low", "Weekly — daily-downsample + delete rate rows past retention (§74)"},
	{TypeReferralReviewFlagged, "low", "Daily sweep — surface flagged referrals in admin queue (§60)"},
	{TypeScholarshipCheckDeadlines, "low", "Daily — check saved scholarship deadlines, send reminders (§73, ADR-073)"},
	{TypeJobsExpireListings, "low", "Daily — archive job listings past application_deadline (§63)"},
	{TypeGamificationExpireChallenges, "low", "Daily — deactivate challenges past ends_at (§64, ADR-082)"},
	{TypeImportFetchMedia, "low", "Fetch + store external images referenced in imported content (§65)"},
	// §86 Community Calendar (ADR-094) — nightly staleness check, low tier.
	{TypeCalendarStalenessCheck, "low", "Mark recurring calendar entries as unverified when their staleness_check_due date has passed (§86)"},
	// §85 Mutual Aid Board (ADR-093) — nightly expiry sweep, low tier.
	{TypeMutualAidExpire, "low", "Expire mutual aid posts past their 30-day expires_at and notify owners 3 days before (§85)"},
	// §INTEL — daily freshness decay cron, low tier (ADR-112).
	{TypeIntelRefreshFreshness, "low", "Daily freshness decay + stale re-crawl enqueue"},
}

// TaskTypeCatalog returns the full Asynq task type catalog with name, tier, and
// description for use by the control panel cron job editor UI.
func TaskTypeCatalog() []map[string]string {
	result := make([]map[string]string, len(taskTypeCatalog))
	for i, t := range taskTypeCatalog {
		result[i] = map[string]string{
			"value":       t.Name,
			"label":       t.Name,
			"tier":        t.Tier,
			"description": t.Description,
		}
	}

	return result
}
