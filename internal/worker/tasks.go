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
	TypeReviewAggregateRecalc         = "content:recalc_review_aggregate"
	TypeTicketSendStatusUpdate        = "ticket:send_status_update"
	TypeLearningUpdateTrackCompletion = "learning:update_track_completion"
	TypeVideoProcessing               = "content:process_video"
	TypeWalletProcessActivityEvent    = "wallet:process_activity_event"
	TypeNewsletterSend                = "newsletter:send"

	// ---- Low tier (background maintenance, lowest concurrency) ----.

	TypeDigestEmail                     = "email:send_digest"
	TypeSitemapGenerate                 = "sitemap:generate"
	TypeRSSRegenerate                   = "rss:regenerate"
	TypeDataExport                      = "data:export_user_data"
	TypeAccountDeletionCleanup          = "account:deletion_cleanup"
	TypeAdEventIngest                   = "ad:ingest_event_batch"
	TypeMaintenanceUpdateVelocityScores = "maintenance:update_velocity_scores"
	TypeMaintenanceCheckSLABreaches     = "maintenance:check_sla_breaches"
	TypeMaintenanceUpdateGeoIPDB        = "maintenance:update_geoip_db"
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
	TypeCouponTrackEvents               = "coupon:track_events"
	TypeCouponExpireCheck               = "coupon:expire_check"
	TypeAffiliateRecordClick            = "affiliate:record_click"
	TypeAffiliateCreditCommission       = "affiliate:credit_commission"
	TypeAffiliateExpireCheck            = "affiliate:expire_check"
	TypeRedirectRecordHit               = "redirect:record_hit"

	// ---- Periodic sweep tasks (enqueued by scheduler, not API) ----.

	TypeScheduledPublishSweep = "content:publish_sweep"
	TypeVelocityScoreRecalc   = "maintenance:recalc_velocity_scores"
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
	mux.HandleFunc(TypeReviewAggregateRecalc, HandleReviewAggregateRecalc)
	mux.HandleFunc(TypeTicketSendStatusUpdate, HandleTicketSendStatusUpdate)
	mux.HandleFunc(TypeLearningUpdateTrackCompletion, HandleLearningUpdateTrackCompletion)
	mux.HandleFunc(TypeVideoProcessing, HandleVideoProcessing)
	mux.HandleFunc(TypeWalletProcessActivityEvent, HandleWalletProcessActivityEvent)
	mux.HandleFunc(TypeNewsletterSend, HandleNewsletterSend)

	// Low handlers
	mux.HandleFunc(TypeDigestEmail, HandleDigestEmail)
	mux.HandleFunc(TypeSitemapGenerate, HandleSitemapGenerate)
	mux.HandleFunc(TypeRSSRegenerate, HandleRSSRegenerate)
	mux.HandleFunc(TypeDataExport, HandleDataExport)
	mux.HandleFunc(TypeAccountDeletionCleanup, HandleAccountDeletionCleanup)
	mux.HandleFunc(TypeAdEventIngest, HandleAdEventIngest)
	mux.HandleFunc(TypeMaintenanceUpdateVelocityScores, HandleMaintenanceUpdateVelocityScores)
	mux.HandleFunc(TypeMaintenanceCheckSLABreaches, HandleMaintenanceCheckSLABreaches)
	mux.HandleFunc(TypeMaintenanceUpdateGeoIPDB, HandleMaintenanceUpdateGeoIPDB)
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
	mux.HandleFunc(TypeRedirectRecordHit, HandleRedirectRecordHit)

	// Periodic sweep handlers
	mux.HandleFunc(TypeScheduledPublishSweep, HandleScheduledPublishSweep)
	mux.HandleFunc(TypeVelocityScoreRecalc, HandleVelocityScoreRecalc)
	mux.HandleFunc(TypeGdprRetentionSweep, HandleGdprRetentionSweep)
}
