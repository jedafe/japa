package worker

import "github.com/hibiken/asynq"

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

	TypeNotificationDispatchBatch = "notification:dispatch_batch"
	TypeWebhookDelivery           = "webhook:deliver"
	TypePaymentWebhookProcess     = "payment:process_webhook"
	TypeTicketSendConfirmation    = "ticket:send_confirmation"
	TypePushDelivery              = "push:deliver"
	TypeDMNotifyRequest           = "dm:notify_request"
	TypeDMNotifyMessage           = "dm:notify_message"

	// ---- Default tier (standard background processing) ----.

	TypeEmailSend                     = "email:send"
	TypeSearchIndexSync               = "search:index_entity"
	TypeImageProcessing               = "content:process_image"
	TypeAITaskProcessing              = "ai:process_task"
	TypeEngagementCountUpdate         = "content:update_engagement"
	TypeModerationScan                = "content:scan_moderation"
	TypeScheduledPublish              = "content:publish_scheduled"
	TypeLinkPreviewFetch              = "content:fetch_link_preview"
	TypeLanguagePackAITranslate       = "i18n:translate_language_pack"
	TypeMediaScan                     = "content:scan_media"
	TypeContentFanOutToFeeds          = "content:fan_out_to_feeds"
	TypeAIGenerateContentEmbedding    = "content:generate_embedding"
	TypeContentGenerateDraft          = "content:generate_draft"
	TypeReviewAggregateRecalc         = "content:recalc_review_aggregate"
	TypeTicketSendStatusUpdate        = "ticket:send_status_update"
	TypeLearningUpdateTrackCompletion = "learning:update_track_completion"
	TypeVideoProcessing               = "content:process_video"
	TypeWalletProcessActivityEvent    = "wallet:process_activity_event"
	TypeWalletProcessCreditPurchase   = "wallet:process_credit_purchase"
	TypeWalletAwardPoints             = "wallet:award_points"
	TypeWalletDeductCredits           = "wallet:deduct_credits"
	TypeNewsletterSend                = "newsletter:send"
	TypeDocsGeneratePage              = "docs:generate_page"
	TypeFormFireSubmissionTriggers    = "form:fire_submission_triggers"

	// ---- Japa University creator economy (ADR-063) ----.
	// Critical: paid enrollment must credit the creator ledger immediately.
	TypeUniversityProcessEnrollment        = "university:process_enrollment"
	TypeUniversityNotifyInstructorApproved = "university:notify_instructor_approved"
	TypeUniversityNotifyCourseApproved     = "university:notify_course_approved"
	TypeUniversitySendInstructorPayout     = "university:send_instructor_payout"
	TypeUniversityCalculatePeriodPayouts   = "university:calculate_period_payouts"
	TypeUniversityFetchLogo                = "university:fetch_logo"

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
	TypeLearningIssueCertificate        = "learning:issue_certificate"
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
	TypeModerationAutoHide              = "moderation:auto_hide"
	TypeDMCleanupDeclinedRequests       = "dm:cleanup_declined_requests"
	TypeTenantCleanupData               = "tenant:cleanup_data"
	TypeCouponTrackEvents               = "coupon:track_events"
	TypeCouponExpireCheck               = "coupon:expire_check"
	TypeAffiliateRecordClick            = "affiliate:record_click"
	TypeAffiliateCreditCommission       = "affiliate:credit_commission"
	TypeAffiliateExpireCheck            = "affiliate:expire_check"
	TypeAffiliateCreditAuthorCommission = "affiliate:credit_author_commission"
	TypeContentScanAffiliateLinks       = "content:scan_affiliate_links"
	TypeRedirectRecordHit               = "redirect:record_hit"

	// ---- Periodic sweep tasks (enqueued by scheduler, not API) ----.

	TypeScheduledPublishSweep = "content:publish_sweep"
	TypeGdprRetentionSweep    = "gdpr:retention_sweep"
)

// RegisterHandlers wires every task type constant to its handler function.
// Call this once on the Asynq ServeMux before starting the server.
func RegisterHandlers(mux *asynq.ServeMux) {
	// Critical handlers
	mux.HandleFunc(TypeNotificationDispatchBatch, HandleNotificationDispatchBatch)
	mux.HandleFunc(TypeWebhookDelivery, HandleWebhookDelivery)
	mux.HandleFunc(TypePaymentWebhookProcess, HandlePaymentWebhookProcess)
	mux.HandleFunc(TypeTicketSendConfirmation, HandleTicketSendConfirmation)
	mux.HandleFunc(TypePushDelivery, HandlePushDelivery)
	mux.HandleFunc(TypeDMNotifyRequest, HandleDMNotifyRequest)
	mux.HandleFunc(TypeDMNotifyMessage, HandleDMNotifyMessage)

	// Default handlers
	mux.HandleFunc(TypeEmailSend, HandleEmailSend)
	mux.HandleFunc(TypeSearchIndexSync, HandleSearchIndexSync)
	mux.HandleFunc(TypeImageProcessing, HandleImageProcessing)
	mux.HandleFunc(TypeAITaskProcessing, HandleAITaskProcessing)
	mux.HandleFunc(TypeEngagementCountUpdate, HandleEngagementCountUpdate)
	mux.HandleFunc(TypeModerationScan, HandleModerationScan)
	mux.HandleFunc(TypeScheduledPublish, HandleScheduledPublish)
	mux.HandleFunc(TypeLinkPreviewFetch, HandleLinkPreviewFetch)
	mux.HandleFunc(TypeLanguagePackAITranslate, HandleLanguagePackAITranslate)
	mux.HandleFunc(TypeMediaScan, HandleMediaScan)
	mux.HandleFunc(TypeContentFanOutToFeeds, HandleContentFanOutToFeeds)
	mux.HandleFunc(TypeAIGenerateContentEmbedding, HandleAIGenerateContentEmbedding)
	mux.HandleFunc(TypeContentGenerateDraft, HandleContentGenerateDraft)
	mux.HandleFunc(TypeReviewAggregateRecalc, HandleReviewAggregateRecalc)
	mux.HandleFunc(TypeTicketSendStatusUpdate, HandleTicketSendStatusUpdate)
	mux.HandleFunc(TypeLearningUpdateTrackCompletion, HandleLearningUpdateTrackCompletion)
	mux.HandleFunc(TypeVideoProcessing, HandleVideoProcessing)
	mux.HandleFunc(TypeWalletProcessActivityEvent, HandleWalletProcessActivityEvent)
	mux.HandleFunc(TypeWalletProcessCreditPurchase, HandleWalletProcessCreditPurchase)
	mux.HandleFunc(TypeWalletAwardPoints, HandleWalletAwardPoints)
	mux.HandleFunc(TypeWalletDeductCredits, HandleWalletDeductCredits)
	mux.HandleFunc(TypeNewsletterSend, HandleNewsletterSend)
	mux.HandleFunc(TypeDocsGeneratePage, HandleDocsGeneratePage)
	mux.HandleFunc(TypeFormFireSubmissionTriggers, HandleFormFireSubmissionTriggers)

	// Japa University creator economy (ADR-063)
	mux.HandleFunc(TypeUniversityProcessEnrollment, HandleUniversityProcessEnrollment)
	mux.HandleFunc(TypeUniversityNotifyInstructorApproved, HandleUniversityNotifyInstructorApproved)
	mux.HandleFunc(TypeUniversityNotifyCourseApproved, HandleUniversityNotifyCourseApproved)
	mux.HandleFunc(TypeUniversitySendInstructorPayout, HandleUniversitySendInstructorPayout)
	mux.HandleFunc(TypeUniversityCalculatePeriodPayouts, HandleUniversityCalculatePeriodPayouts)
	mux.HandleFunc(TypeUniversityFetchLogo, HandleUniversityFetchLogo)

	// Low handlers
	mux.HandleFunc(TypeDigestEmail, HandleDigestEmail)
	mux.HandleFunc(TypeSitemapGenerate, HandleSitemapGenerate)
	mux.HandleFunc(TypeRSSRegenerate, HandleRSSRegenerate)
	mux.HandleFunc(TypeDataExport, HandleDataExport)
	mux.HandleFunc(TypeAccountDeletionCleanup, HandleAccountDeletionCleanup)
	mux.HandleFunc(TypePlatformDataExport, HandlePlatformDataExport)
	mux.HandleFunc(TypePlatformDataImport, HandlePlatformDataImport)
	mux.HandleFunc(TypeAdEventIngest, HandleAdEventIngest)
	mux.HandleFunc(TypeMaintenanceUpdateVelocityScores, HandleMaintenanceUpdateVelocityScores)
	mux.HandleFunc(TypeMaintenanceCheckSLABreaches, HandleMaintenanceCheckSLABreaches)
	mux.HandleFunc(TypeMaintenanceUpdateGeoIPDB, HandleMaintenanceUpdateGeoIPDB)
	mux.HandleFunc(TypeSpaceAutoPopulate, HandleSpaceAutoPopulate)
	mux.HandleFunc(TypeAnalyticsRefreshViews, HandleAnalyticsRefreshViews)
	mux.HandleFunc(TypeSecurityScan, HandleSecurityScan)
	mux.HandleFunc(TypeMaintenanceRegenerateLLMsTxt, HandleMaintenanceRegenerateLLMsTxt)
	mux.HandleFunc(TypeLearningIssueCertificate, HandleLearningIssueCertificate)
	mux.HandleFunc(TypeDBManualVacuum, HandleDBManualVacuum)
	mux.HandleFunc(TypeNewsletterDigest, HandleNewsletterDigest)
	mux.HandleFunc(TypeComputeTrending, HandleComputeTrending)
	mux.HandleFunc(TypeComputeSuggestions, HandleComputeSuggestions)

	// Missing Phase 31 task handlers (catalog amendments)
	mux.HandleFunc(TypeMaintenanceCheckExpiredBans, HandleMaintenanceCheckExpiredBans)
	mux.HandleFunc(TypeMaintenanceCheckExpiredBoosts, HandleMaintenanceCheckExpiredBoosts)
	mux.HandleFunc(TypeMaintenancePruneOldFeeds, HandleMaintenancePruneOldFeeds)
	mux.HandleFunc(TypeMaintenancePruneAuditLogs, HandleMaintenancePruneAuditLogs)
	mux.HandleFunc(TypeMaintenancePrunePushTokens, HandleMaintenancePrunePushTokens)
	mux.HandleFunc(TypeModerationAutoHide, HandleModerationAutoHide)
	mux.HandleFunc(TypeDMCleanupDeclinedRequests, HandleDMCleanupDeclinedRequests)
	mux.HandleFunc(TypeCouponTrackEvents, HandleCouponTrackEvents)
	mux.HandleFunc(TypeCouponExpireCheck, HandleCouponExpireCheck)
	mux.HandleFunc(TypeAffiliateRecordClick, HandleAffiliateRecordClick)
	mux.HandleFunc(TypeAffiliateCreditCommission, HandleAffiliateCreditCommission)
	mux.HandleFunc(TypeAffiliateExpireCheck, HandleAffiliateExpireCheck)
	mux.HandleFunc(TypeAffiliateCreditAuthorCommission, HandleAffiliateCreditAuthorCommission)
	mux.HandleFunc(TypeContentScanAffiliateLinks, HandleContentScanAffiliateLinks)
	mux.HandleFunc(TypeRedirectRecordHit, HandleRedirectRecordHit)
	mux.HandleFunc(TypeTenantCleanupData, HandleTenantCleanupData)

	// Periodic sweep handlers
	mux.HandleFunc(TypeScheduledPublishSweep, HandleScheduledPublishSweep)
	mux.HandleFunc(TypeGdprRetentionSweep, HandleGdprRetentionSweep)

	// V2 feature handlers (Sections 60–77, ADRs 068, 070–085).
	// Each delegates to a registered processor; nil-safe if unregistered.
	mux.HandleFunc(TypeJobsExpireListings, HandleJobsExpireListings)
	mux.HandleFunc(TypeJobsNotifyApplicationStatus, HandleJobsNotifyApplicationStatus)
	mux.HandleFunc(TypeGamificationExpireChallenges, HandleGamificationExpireChallenges)
	mux.HandleFunc(TypeImportProcessSource, HandleImportProcessSource)
	mux.HandleFunc(TypeImportFetchMedia, HandleImportFetchMedia)
	mux.HandleFunc(TypeAssessmentScoreSubmission, HandleAssessmentScoreSubmission)
	mux.HandleFunc(TypeAssessmentCleanupAnonymous, HandleAssessmentCleanupAnonymous)
	mux.HandleFunc(TypeScholarshipCheckDeadlines, HandleScholarshipCheckDeadlines)
	mux.HandleFunc(TypeRemittanceFetchRates, HandleRemittanceFetchRates)
	mux.HandleFunc(TypeRemittanceCheckAlerts, HandleRemittanceCheckAlerts)
	mux.HandleFunc(TypeRemittancePruneOldRates, HandleRemittancePruneOldRates)
	mux.HandleFunc(TypeReferralRecordClick, HandleReferralRecordClick)
	mux.HandleFunc(TypeReferralCheckConversion, HandleReferralCheckConversion)
	mux.HandleFunc(TypeReferralReviewFlagged, HandleReferralReviewFlagged)
	mux.HandleFunc(TypeAdsRefundCampaignCredits, HandleAdsRefundCampaignCredits)
	mux.HandleFunc(TypeAdsNotifyCampaignStatus, HandleAdsNotifyCampaignStatus)
	mux.HandleFunc(TypeNotificationSendPush, HandleNotificationSendPush)
	mux.HandleFunc(TypePersonaRunActivity, HandlePersonaRunActivity)
	mux.HandleFunc(TypePersonaDispatchAction, HandlePersonaDispatchAction)
	mux.HandleFunc(TypeStudioRunBatch, HandleStudioRunBatch)
	mux.HandleFunc(TypeStudioRetryFailed, HandleStudioRetryFailed)

	// §102 Mentorship Matching handlers (ADR-110).
	mux.HandleFunc(TypeMentorGenerateEmbedding, HandleMentorGenerateEmbedding)
	mux.HandleFunc(TypeMentorUpdateMeilisearch, HandleMentorUpdateMeilisearch)
}
