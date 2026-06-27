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
	// ---- Critical tier (immediate processing, highest concurrency) ----

	TypeNotificationDispatchBatch = "notification:dispatch_batch"
	TypeWebhookDelivery           = "webhook_delivery"
	TypePaymentWebhookProcess     = "payment_webhook_process"
	TypeTicketSendConfirmation    = "ticket:send_confirmation"

	// ---- Default tier (standard background processing) ----

	TypeEmailSend                  = "email_send"
	TypeSearchIndexSync            = "search_index_sync"
	TypeImageProcessing            = "image_processing"
	TypeAITaskProcessing           = "ai_task_processing"
	TypeEngagementCountUpdate      = "engagement_count_update"
	TypeModerationScan             = "moderation_scan"
	TypeScheduledPublish           = "scheduled_publish"
	TypeLinkPreviewFetch           = "link_preview_fetch"
	TypeLanguagePackAITranslate    = "language_pack_ai_translate"
	TypeMediaScan                  = "media_scan"
	TypeContentFanOutToFeeds       = "content:fan_out_to_feeds"
	TypeAIGenerateContentEmbedding = "ai:generate_content_embedding"
	TypeReviewAggregateRecalc      = "review_aggregate_recalc"
	TypeTicketSendStatusUpdate     = "ticket:send_status_update"
	TypeLearningUpdateTrackCompletion = "learning:update_track_completion"
	TypeVideoProcessing            = "video_processing"
	TypeWalletProcessActivityEvent = "wallet:process_activity_event"
	TypeNewsletterSend             = "newsletter_send"

	// ---- Low tier (background maintenance, lowest concurrency) ----

	TypeDigestEmail                          = "digest_email"
	TypeSitemapGenerate                      = "sitemap_generate"
	TypeRSSRegenerate                        = "rss_regenerate"
	TypeDataExport                           = "data_export"
	TypeAccountDeletionCleanup               = "account_deletion_cleanup"
	TypeAdEventIngest                        = "ad_event_ingest"
	TypeMaintenanceUpdateVelocityScores      = "maintenance:update_velocity_scores"
	TypeMaintenanceCheckSLABreaches          = "maintenance:check_sla_breaches"
	TypeMaintenanceUpdateGeoIPDB             = "maintenance:update_geoip_db"
	TypeSpaceAutoPopulate                    = "space:auto_populate"
	TypeAnalyticsRefreshViews                = "analytics:refresh_views"
	TypeSecurityScan                         = "security:scan"
	TypeMaintenanceRegenerateLLMsTxt         = "maintenance:regenerate_llms_txt"
	TypeLearningIssueCertificate             = "learning:issue_certificate"
	TypeDBManualVacuum                       = "db_manual_vacuum"
	TypeNewsletterDigest                     = "newsletter_digest"
)

// RegisterHandlers wires every task type constant to its handler function.
// Call this once on the Asynq ServeMux before starting the server.
func RegisterHandlers(mux *asynq.ServeMux) {
	// Critical handlers
	mux.HandleFunc(TypeNotificationDispatchBatch, HandleNotificationDispatchBatch)
	mux.HandleFunc(TypeWebhookDelivery, HandleWebhookDelivery)
	mux.HandleFunc(TypePaymentWebhookProcess, HandlePaymentWebhookProcess)
	mux.HandleFunc(TypeTicketSendConfirmation, HandleTicketSendConfirmation)

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
	mux.HandleFunc(TypeSpaceAutoPopulate, HandleSpaceAutoPopulate)
	mux.HandleFunc(TypeAnalyticsRefreshViews, HandleAnalyticsRefreshViews)
	mux.HandleFunc(TypeSecurityScan, HandleSecurityScan)
	mux.HandleFunc(TypeMaintenanceRegenerateLLMsTxt, HandleMaintenanceRegenerateLLMsTxt)
	mux.HandleFunc(TypeLearningIssueCertificate, HandleLearningIssueCertificate)
	mux.HandleFunc(TypeDBManualVacuum, HandleDBManualVacuum)
	mux.HandleFunc(TypeNewsletterDigest, HandleNewsletterDigest)
}
