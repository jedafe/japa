package worker

import (
	"context"

	"github.com/hibiken/asynq"
)

// Task type constants for all V1 background jobs. Each constant maps to a
// unique Asynq task type string. The grouping comments indicate which tier
// the task belongs to by default.
//
// When adding a new task type:
//  1. Add the constant here
//  2. Add the handler function in handlers.go
//  3. Register it in RegisterHandlers
//  4. Enqueue from the relevant service with the matching constant
const (
	// ---- Critical tier (immediate processing, highest concurrency) ----.

	TypeNotificationDispatchBatch  = "notification:dispatch_batch"
	TypeWebhookDelivery            = "webhook:deliver"
	TypePaymentWebhookProcess      = "payment:process_webhook"
	TypeSubscriptionWebhookProcess = "subscription:webhook_process" // §17 recurring billing lifecycle
	TypePaymentRefundProcess       = "payment:process_refund"       // §17 provider refund retry (kept pending until provider confirms)
	TypeTicketSendConfirmation     = "ticket:send_confirmation"
	TypePushDelivery               = "push:deliver"
	TypeDMNotifyRequest            = "dm:notify_request"
	TypeDMNotifyMessage            = "dm:notify_message"

	// ---- Default tier (standard background processing) ----.

	TypeEmailSend                     = "email:send"
	TypeSearchIndexSync               = "search:index_entity"
	TypeSearchReindexEngagementScores = "search:reindex_engagement_scores"
	TypeImageProcessing               = "content:process_image"
	TypeAITaskProcessing              = "ai:process_task"
	TypeAIMigrateConversations        = "ai:migrate_conversations" // ADR-130 Phase 4: Honcho → ai_conversations backfill for one user
	TypeEngagementCountUpdate         = "content:update_engagement"
	TypeModerationScan                = "content:scan_moderation"
	TypeScheduledPublish              = "content:publish_scheduled"
	TypeLinkPreviewFetch              = "content:fetch_link_preview"
	TypeLanguagePackAITranslate       = "i18n:translate_language_pack"
	TypeMediaScan                     = "content:scan_media"
	TypeMediaCleanupDeleted           = "media:cleanup_deleted"
	TypeContentFanOutToFeeds          = "content:fan_out_to_feeds"
	TypeAIGenerateContentEmbedding    = "content:generate_embedding"
	TypeContentGenerateDraft          = "content:generate_draft"
	TypeContentRegenerateFromSource   = "content:regenerate_from_source"
	TypeAgentCreateDraftRequest       = "agent:create_draft_request"
	TypeReviewAggregateRecalc         = "content:recalc_review_aggregate"
	TypeTicketSendStatusUpdate        = "ticket:send_status_update"
	TypeAcademyUpdateTrackCompletion  = "academy:update_track_completion"
	TypeJCERefreshValues              = "jce:refresh_values"
	TypeVideoProcessing               = "content:process_video"
	TypeVideoImportBulk               = "video:import_bulk"
	TypeVideoGenerateSummary          = "video:generate_summary"
	TypeVideoSyncChannels             = "video:sync_channels"
	TypeWalletProcessActivityEvent    = "wallet:process_activity_event"
	TypeWalletProcessCreditPurchase   = "wallet:process_credit_purchase"
	TypeWalletAwardPoints             = "wallet:award_points"
	TypeWalletDeductCredits           = "wallet:deduct_credits" // #nosec
	TypeNewsletterSend                = "newsletter:send"
	TypeScholarshipSyncFromGuide      = "scholarship:sync_from_guide"
	TypeDocsGeneratePage              = "docs:generate_page"
	TypeFormFireSubmissionTriggers    = "form:fire_submission_triggers"
	// §113 Public Product Roadmap (ADR-127) — notification fan-out when an
	// item transitions to shipped; enqueued fire-and-forget by
	// PATCH /admin/roadmap/items/{id}/status.
	TypeRoadmapShipNotify = "roadmap:ship_notify"

	// ---- Service Packages & Bookings (§111, ADR-123) ----.
	// booking_created/status_changed fan out org/booker notifications on the
	// default tier; fee_settle is a financial side effect → critical;
	// expire_pending is the hourly pending-SLA sweep → low tier.
	TypeServiceBookingCreated       = "service:booking_created"
	TypeServiceBookingStatusChanged = "service:booking_status_changed"
	TypeServiceBookingFeeSettle     = "service:booking_fee_settle"
	TypeServiceBookingExpirePending = "service:booking_expire_pending"

	// ---- Japa Academy creator economy (ADR-063) ----.
	// Critical: paid enrollment must credit the creator ledger immediately.
	TypeAcademyProcessEnrollment        = "academy:process_enrollment"
	TypeAcademyNotifyInstructorApproved = "academy:notify_instructor_approved"
	TypeAcademyNotifyCourseApproved     = "academy:notify_course_approved"
	TypeAcademySendInstructorPayout     = "academy:send_instructor_payout"
	TypeAcademyCalculatePeriodPayouts   = "academy:calculate_period_payouts"
	TypeUniversityFetchLogo             = "university:fetch_logo"

	// ---- Low tier (background maintenance, lowest concurrency) ----.

	TypeDigestEmail                     = "email:send_digest"
	TypeSitemapGenerate                 = "sitemap:generate"
	TypeRSSRegenerate                   = "rss:regenerate"
	TypeDataExport                      = "data:export_user_data"
	TypeAccountDeletionCleanup          = "account:deletion_cleanup"
	TypePlatformDataExport              = "platform:data_export"
	TypePlatformDataImport              = "platform:data_import"
	TypeAdEventIngest                   = "ad:ingest_event_batch"
	TypeMaintenanceUpdateVelocityScores = "maintenance:update_velocity_scores"
	TypeMaintenanceCheckSLABreaches     = "maintenance:check_sla_breaches"
	TypeMaintenanceUpdateGeoIPDB        = "maintenance:update_geoip_db"
	TypeSpaceAutoPopulate               = "space:auto_populate"
	TypeAnalyticsRefreshViews           = "analytics:refresh_views"
	TypeSecurityScan                    = "security:scan"
	TypeMaintenanceRegenerateLLMsTxt    = "maintenance:regenerate_llms_txt"
	TypeAcademyIssueCertificate         = "academy:issue_certificate"
	TypeDBManualVacuum                  = "db:manual_vacuum"
	TypeNewsletterDigest                = "newsletter:digest"
	TypeComputeTrending                 = "maintenance:compute_trending"
	TypeComputeSuggestions              = "maintenance:compute_suggestions"

	// ---- Missing Phase 31 tasks (catalog amendments) ----.

	TypeMaintenanceCheckExpiredBans     = "maintenance:check_expired_bans"
	TypeMaintenanceCheckExpiredBoosts   = "maintenance:check_expired_boosts"
	TypeMaintenancePruneOldFeeds        = "maintenance:prune_old_feeds"
	TypeMaintenancePruneAuditLogs       = "maintenance:prune_audit_logs"
	TypeMaintenancePrunePushTokens      = "maintenance:prune_push_tokens"
	TypeMaintenancePruneTravelResults   = "maintenance:prune_travel_results"
	TypeMaintenancePruneOAuthRecords    = "maintenance:prune_oauth_records"
	TypeTravelSyncHotelDirectory        = "travel:sync_hotel_directory"
	TypeModerationAutoHide              = "moderation:auto_hide"
	TypeReportNotifyAdmin               = "report:notify_admin"
	TypeDMCleanupDeclinedRequests       = "dm:cleanup_declined_requests"
	TypeCouponTrackEvents               = "coupon:track_events"
	TypeCouponExpireCheck               = "coupon:expire_check"
	TypeCouponSyncFeeds                 = "coupon:sync_feeds"
	TypeCouponImportFeed                = "coupon:import_feed"
	TypeAffiliateRecordClick            = "affiliate:record_click"
	TypeAffiliateCreditCommission       = "affiliate:credit_commission"
	TypeAffiliateExpireCheck            = "affiliate:expire_check"
	TypeAffiliateCreditAuthorCommission = "affiliate:credit_author_commission"
	TypeContentScanAffiliateLinks       = "content:scan_affiliate_links"
	TypeRedirectRecordHit               = "redirect:record_hit"

	// ---- Destination Wiki (§80, ADR-088) ----.

	TypeWikiSeedStubPages   = "wiki:seed_stub_pages"
	TypeWikiFieldsPropagate = "wiki:fields_propagate"
	// TypeWikiSeedAttractions bulk-seeds attraction wiki spaces under a geo
	// parent from Wikidata (§80 attractions amendment).
	TypeWikiSeedAttractions = "wiki:seed_attractions"

	// ---- Periodic sweep tasks (enqueued by scheduler, not API) ----.

	TypeScheduledPublishSweep   = "content:publish_sweep"
	TypeBroadcastDispatch       = "broadcast:dispatch_scheduled" // §35 scheduled admin broadcasts
	TypeHonchoRetentionSweep    = "honcho:retention_sweep"       // daily Honcho memory retention
	TypeGdprRetentionSweep      = "gdpr:retention_sweep"
	TypeJceRefreshStale         = "jce:refresh_stale"
	TypeSubscriptionExpirySweep = "subscription:expiry_sweep" // §17 webhook-miss safety net
	TypeOnboardingReminderSweep = "onboarding:reminder_sweep" // §09: nudge users who haven't finished onboarding

	// ---- Intelligence System (Section INTEL) ----.

	TypeIntelScheduleCrawls       = "intel:schedule_crawls"        // critical (cron)
	TypeIntelFetchSource          = "intel:fetch_source"           // default
	TypeIntelFetchChannel         = "intel:fetch_channel"          // default
	TypeIntelDetectChanges        = "intel:detect_changes"         // default
	TypeIntelCurateChannel        = "intel:curate_channel"         // default
	TypeIntelExtract              = "intel:extract"                // default
	TypeIntelDedup                = "intel:dedup"                  // default
	TypeIntelPublishGate          = "intel:publish_gate"           // default
	TypeIntelPromote              = "intel:promote"                // default
	TypeIntelPostPublish          = "intel:post_publish"           // default
	TypeIntelNotifyDiscoveries    = "intel:notify_discoveries"     // default
	TypeIntelRefreshFreshness     = "intel:refresh_freshness"      // low (daily cron)
	TypeIntelRetryFailedSource    = "intel:retry_failed_source"    // default
	TypeIntelSourceMetadata       = "intel:source_metadata"        // default
	TypeIntelProbeVerify          = "intel:probe_verify"           // default
	TypeIntelXHRSniff             = "intel:xhr_sniff"              // low (intel-channels Phase 2)
	TypeIntelInboundEmail         = "intel:inbound_email"          // critical (intel-channels Phase 3)
	TypeIntelWebSubSubscribe      = "intel:websub_subscribe"       // low (intel-channels Phase 3)
	TypeIntelWebSubRenew          = "intel:websub_renew"           // low (intel-channels Phase 3)
	TypeIntelSearchPoll           = "intel:search_poll"            // default (intel-channels Phase 3)
	TypeIntelPurgeInboundEvents   = "intel:purge_inbound_events"   // low (daily retention sweep)
	TypeIntelPruneRejectedRecords = "intel:prune_rejected_records" // low (daily retention sweep)
	TypeIntelHarvestOutbound      = "intel:harvest_outbound"       // low (intel-channels Phase 4)
)

// RegisterHandlers wires every task type constant to its handler function.
// Call this once on the Asynq ServeMux before starting the server.
// handlerRegistry maps every task type constant to its handler function,
// preserving the domain grouping comments from the original RegisterHandlers.
var handlerRegistry = []struct {
	taskType string
	handler  func(context.Context, *asynq.Task) error
}{
	// Critical handlers
	{TypeNotificationDispatchBatch, HandleNotificationDispatchBatch},
	{TypeWebhookDelivery, HandleWebhookDelivery},
	{TypePaymentWebhookProcess, HandlePaymentWebhookProcess},
	{TypeSubscriptionWebhookProcess, HandleSubscriptionWebhookProcess},
	{TypeTicketSendConfirmation, HandleTicketSendConfirmation},
	{TypePushDelivery, HandlePushDelivery},
	{TypeDMNotifyRequest, HandleDMNotifyRequest},
	{TypeDMNotifyMessage, HandleDMNotifyMessage},
	// Default handlers
	{TypeEmailSend, HandleEmailSend},
	{TypeSearchIndexSync, HandleSearchIndexSync},
	{TypeSearchReindexEngagementScores, HandleSearchReindexEngagementScores},
	{TypeImageProcessing, HandleImageProcessing},
	{TypeAITaskProcessing, HandleAITaskProcessing},
	{TypeAIMigrateConversations, HandleAIMigrateConversations},
	{TypeEngagementCountUpdate, HandleEngagementCountUpdate},
	{TypeModerationScan, HandleModerationScan},
	{TypeScheduledPublish, HandleScheduledPublish},
	{TypeLinkPreviewFetch, HandleLinkPreviewFetch},
	{TypeLanguagePackAITranslate, HandleLanguagePackAITranslate},
	{TypeMediaScan, HandleMediaScan},
	{TypeMediaCleanupDeleted, HandleMediaCleanupDeleted},
	{TypeContentFanOutToFeeds, HandleContentFanOutToFeeds},
	{TypeAIGenerateContentEmbedding, HandleAIGenerateContentEmbedding},
	{TypeContentGenerateDraft, HandleContentGenerateDraft},
	{TypeContentRegenerateFromSource, HandleContentRegenerateFromSource},
	{TypeAgentCreateDraftRequest, HandleAgentCreateDraftRequest},
	{TypeReviewAggregateRecalc, HandleReviewAggregateRecalc},
	{TypeTicketSendStatusUpdate, HandleTicketSendStatusUpdate},
	{TypeAcademyUpdateTrackCompletion, HandleAcademyUpdateTrackCompletion},
	{TypeJCERefreshValues, HandleJceRefreshValues},
	{TypeVideoProcessing, HandleVideoProcessing},
	{TypeVideoImportBulk, HandleVideoImportBulk},
	{TypeVideoGenerateSummary, HandleVideoGenerateSummary},
	{TypeVideoSyncChannels, HandleVideoSyncChannels},
	{TypeWalletProcessActivityEvent, HandleWalletProcessActivityEvent},
	{TypeWalletProcessCreditPurchase, HandleWalletProcessCreditPurchase},
	{TypeWalletAwardPoints, HandleWalletAwardPoints},
	{TypeWalletDeductCredits, HandleWalletDeductCredits},
	{TypeNewsletterSend, HandleNewsletterSend},
	{TypeScholarshipSyncFromGuide, HandleScholarshipSyncFromGuide},
	{TypeDocsGeneratePage, HandleDocsGeneratePage},
	{TypeFormFireSubmissionTriggers, HandleFormFireSubmissionTriggers},
	// §113 Public Product Roadmap (ADR-127)
	{TypeRoadmapShipNotify, HandleRoadmapShipNotify},

	// §111 Service Packages & Bookings (ADR-123)
	{TypeServiceBookingCreated, HandleServiceBookingCreated},
	{TypeServiceBookingStatusChanged, HandleServiceBookingStatusChanged},
	{TypeServiceBookingFeeSettle, HandleServiceBookingFeeSettle},
	{TypeServiceBookingExpirePending, HandleServiceBookingExpirePending},
	// Japa Academy creator economy (ADR-063)
	{TypeAcademyProcessEnrollment, HandleAcademyProcessEnrollment},
	{TypeAcademyNotifyInstructorApproved, HandleAcademyNotifyInstructorApproved},
	{TypeAcademyNotifyCourseApproved, HandleAcademyNotifyCourseApproved},
	{TypeAcademySendInstructorPayout, HandleAcademySendInstructorPayout},
	{TypeAcademyCalculatePeriodPayouts, HandleAcademyCalculatePeriodPayouts},
	{TypeUniversityFetchLogo, HandleUniversityFetchLogo},
	// Low handlers
	{TypeDigestEmail, HandleDigestEmail},
	{TypeSitemapGenerate, HandleSitemapGenerate},
	{TypeRSSRegenerate, HandleRSSRegenerate},
	{TypeDataExport, HandleDataExport},
	{TypeAccountDeletionCleanup, HandleAccountDeletionCleanup},
	{TypePlatformDataExport, HandlePlatformDataExport},
	{TypePlatformDataImport, HandlePlatformDataImport},
	{TypeAdEventIngest, HandleAdEventIngest},
	{TypeMaintenanceUpdateVelocityScores, HandleMaintenanceUpdateVelocityScores},
	{TypeMaintenanceCheckSLABreaches, HandleMaintenanceCheckSLABreaches},
	{TypeMaintenanceUpdateGeoIPDB, HandleMaintenanceUpdateGeoIPDB},
	{TypeSpaceAutoPopulate, HandleSpaceAutoPopulate},
	{TypeAnalyticsRefreshViews, HandleAnalyticsRefreshViews},
	{TypeSecurityScan, HandleSecurityScan},
	{TypeMaintenanceRegenerateLLMsTxt, HandleMaintenanceRegenerateLLMsTxt},
	{TypeAcademyIssueCertificate, HandleAcademyIssueCertificate},
	{TypeDBManualVacuum, HandleDBManualVacuum},
	{TypeNewsletterDigest, HandleNewsletterDigest},
	{TypeComputeTrending, HandleComputeTrending},
	{TypeComputeSuggestions, HandleComputeSuggestions},
	// Missing Phase 31 task handlers (catalog amendments)
	{TypeMaintenanceCheckExpiredBans, HandleMaintenanceCheckExpiredBans},
	{TypeMaintenanceCheckExpiredBoosts, HandleMaintenanceCheckExpiredBoosts},
	{TypeMaintenancePruneOldFeeds, HandleMaintenancePruneOldFeeds},
	{TypeMaintenancePruneAuditLogs, HandleMaintenancePruneAuditLogs},
	{TypeMaintenancePrunePushTokens, HandleMaintenancePrunePushTokens},
	{TypeMaintenancePruneTravelResults, HandleMaintenancePruneTravelResults},
	{TypeMaintenancePruneOAuthRecords, HandleMaintenancePruneOAuthRecords},
	{TypeTravelSyncHotelDirectory, HandleTravelSyncHotelDirectory},
	{TypeModerationAutoHide, HandleModerationAutoHide},
	{TypeReportNotifyAdmin, HandleReportNotifyAdmin},
	{TypeDMCleanupDeclinedRequests, HandleDMCleanupDeclinedRequests},
	{TypeCouponTrackEvents, HandleCouponTrackEvents},
	{TypeCouponExpireCheck, HandleCouponExpireCheck},
	{TypeCouponSyncFeeds, HandleCouponSyncFeeds},
	{TypeCouponImportFeed, HandleCouponImportFeed},
	{TypeSubscriptionExpirySweep, HandleSubscriptionExpirySweep},
	{TypeAffiliateRecordClick, HandleAffiliateRecordClick},
	{TypeAffiliateCreditCommission, HandleAffiliateCreditCommission},
	{TypeAffiliateExpireCheck, HandleAffiliateExpireCheck},
	{TypeAffiliateCreditAuthorCommission, HandleAffiliateCreditAuthorCommission},
	{TypeContentScanAffiliateLinks, HandleContentScanAffiliateLinks},
	{TypeRedirectRecordHit, HandleRedirectRecordHit},
	{TypeWikiSeedStubPages, HandleWikiSeedStubPages},
	{TypeWikiFieldsPropagate, HandleWikiFieldsPropagate},
	{TypeWikiSeedAttractions, HandleWikiSeedAttractions},
	// Periodic sweep handlers
	{TypeScheduledPublishSweep, HandleScheduledPublishSweep},
	{TypeBroadcastDispatch, HandleBroadcastDispatchScheduled},
	{TypeHonchoRetentionSweep, HandleHonchoRetentionSweep},
	{TypeGdprRetentionSweep, HandleGdprRetentionSweep},
	{TypeOnboardingReminderSweep, HandleOnboardingReminderSweep},
	{TypeJceRefreshStale, HandleJceRefreshStale},
	// V2 feature handlers (Sections 60–77, ADRs 068, 070–085).
	// Each delegates to a registered processor; nil-safe if unregistered.
	{TypeJobsExpireListings, HandleJobsExpireListings},
	{TypeJobsNotifyApplicationStatus, HandleJobsNotifyApplicationStatus},
	{TypeGamificationExpireChallenges, HandleGamificationExpireChallenges},
	{TypeImportProcessSource, HandleImportProcessSource},
	{TypeImportFetchMedia, HandleImportFetchMedia},
	{TypeAssessmentScoreSubmission, HandleAssessmentScoreSubmission},
	{TypeAssessmentCleanupAnonymous, HandleAssessmentCleanupAnonymous},
	{TypeScholarshipCheckDeadlines, HandleScholarshipCheckDeadlines},
	{TypeRemittanceFetchRates, HandleRemittanceFetchRates},
	{TypeRemittanceCheckAlerts, HandleRemittanceCheckAlerts},
	{TypeRemittancePruneOldRates, HandleRemittancePruneOldRates},
	{TypeFxPruneRates, HandleFxPruneRates},
	{TypeReferralRecordClick, HandleReferralRecordClick},
	{TypeReferralCheckConversion, HandleReferralCheckConversion},
	{TypeReferralReviewFlagged, HandleReferralReviewFlagged},
	{TypeAdsRefundCampaignCredits, HandleAdsRefundCampaignCredits},
	{TypeAdsNotifyCampaignStatus, HandleAdsNotifyCampaignStatus},
	{TypeAdsAccrueSpend, HandleAdsAccrueSpendDaily},
	{TypeNotificationSendPush, HandleNotificationSendPush},
	{TypePersonaRunActivity, HandlePersonaRunActivity},
	{TypePersonaDispatchAction, HandlePersonaDispatchAction},
	{TypeStudioRunBatch, HandleStudioRunBatch},
	{TypeStudioRetryFailed, HandleStudioRetryFailed},
	// §102 Mentorship Matching handlers (ADR-110).
	{TypeMentorGenerateEmbedding, HandleMentorGenerateEmbedding},
	{TypeMentorUpdateMeilisearch, HandleMentorUpdateMeilisearch},
	// §91 Embassy Appointment Alerter handlers (ADR-099).
	{TypeEmbassyPoll, HandleEmbassyPoll},
	{TypeEmbassyDispatchAlerts, HandleEmbassyDispatchAlerts},
	{TypeEmbassyExpireChecks, HandleEmbassyExpireChecks},
	{TypeEmbassySchedulePolls, HandleEmbassySchedulePolls},
	// §86 Community Calendar handlers (ADR-094).
	{TypeCalendarStalenessCheck, HandleCalendarStalenessCheck},
	{TypeCalendarSendReminders, HandleCalendarSendReminders},
	// §85 Mutual Aid Board handlers (ADR-093).
	{TypeMutualAidExpire, HandleMutualAidExpire},
	// §94 Professional Credential Checker handlers (ADR-102).
	{TypeCredentialsCheckLinks, HandleCredentialsCheckLinks},
	// §90 Visa Route Comparison handlers (ADR-098).
	{TypeVisaRoutesCheckLinks, HandleVisaRoutesCheckLinks},
	// §95 CV Formatter handlers (ADR-103).
	{TypeCvFormatProcess, HandleCvFormatProcess},
	{TypeCvFormatSessionsPurge, HandleCvFormatSessionsPurge},
	// §110 AI SOP Generator handlers (ADR-121).
	{TypeSopGenerate, HandleSopGenerate},
	// §§107–110 SEO growth tools — Content Agent draft generation (ADR-118..121).
	{TypeToolGenerateDraft, HandleToolGenerateDraft},
	// §84 Community Digest Widget handlers (ADR-092).
	{TypeDigestGenerate, HandleDigestGenerate},
	{TypeDigestAutoPublish, HandleDigestAutoPublish},
	{TypeDigestSendEmail, HandleDigestSendEmail},
	{TypeJapaStageRecommend, HandleJapaStageRecommend},
	// §82 Japa Planning Groups handlers (ADR-090).
	{TypePlanningGroupExpire, HandlePlanningGroupExpire},
	{TypePlanningGroupCheckinReminder, HandlePlanningGroupCheckinReminder},
	// §81 Scam Alert Board handlers (ADR-089).
	{TypeScamAlertNotifySubscribers, HandleScamAlertNotifySubscribers},
	// §83 Visa Timeline Tracker handlers (ADR-091).
	{TypeVisaTimelineRefreshStats, HandleVisaTimelineRefreshStats},
	{TypeVisaTimelinePromptUpdate, HandleVisaTimelinePromptUpdate},
	// Intelligence System (Section INTEL) handlers — ingestion spine.
	{TypeIntelScheduleCrawls, HandleIntelScheduleCrawls},
	{TypeIntelFetchSource, HandleIntelFetchSource},
	{TypeIntelFetchChannel, HandleIntelFetchChannel},
	{TypeIntelDetectChanges, HandleIntelDetectChanges},
	{TypeIntelCurateChannel, HandleIntelCurateChannel},
	{TypeIntelExtract, HandleIntelExtract},
	{TypeIntelDedup, HandleIntelDedup},
	{TypeIntelPublishGate, HandleIntelPublishGate},
	{TypeIntelPromote, HandleIntelPromote},
	{TypeIntelPostPublish, HandleIntelPostPublish},
	{TypeIntelNotifyDiscoveries, HandleIntelNotifyDiscoveries},
	{TypeIntelRefreshFreshness, HandleIntelRefreshFreshness},
	{TypeIntelRetryFailedSource, HandleIntelRetryFailedSource},
	{TypeIntelSourceMetadata, HandleSourceMetadata},
	{TypeIntelProbeVerify, HandleIntelProbeVerify},
	{TypeIntelXHRSniff, HandleIntelXHRSniff},
	{TypeIntelInboundEmail, HandleIntelInboundEmail},
	{TypeIntelWebSubSubscribe, HandleIntelWebSubSubscribe},
	{TypeIntelWebSubRenew, HandleIntelWebSubRenew},
	{TypeIntelSearchPoll, HandleIntelSearchPoll},
	{TypeIntelPurgeInboundEvents, HandleIntelPurgeInboundEvents},
	{TypeIntelPruneRejectedRecords, HandleIntelPruneRejectedRecords},
	{TypeIntelHarvestOutbound, HandleIntelHarvestOutbound},
}

// RegisterHandlers wires every task type constant to its handler function.
// Call this once on the Asynq ServeMux before starting the server.
func RegisterHandlers(mux *asynq.ServeMux) {
	for _, h := range handlerRegistry {
		mux.HandleFunc(h.taskType, h.handler)
	}
}
