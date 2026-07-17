# Asynq Task Catalog

**Last updated:** 2026-07-17
**Total tasks:** 58 (7 critical, 17 default, 31 low, 3 periodic sweep)

Each task type is listed with its queue tier, timeout/retry policy, idempotency key, producer location, and consumer handler.

---

## Critical Tier (7)

| Task Type | Timeout | MaxRetry | Unique | Producer | Consumer |
|-----------|---------|----------|--------|----------|----------|
| `notification:dispatch_batch` | 30s | 10 | Yes (24h) | `internal/notification/service.go` | `HandleNotificationDispatchBatch` |
| `webhook:deliver` | 60s | 15 | Yes (24h) | `internal/webhooks/service.go` | `HandleWebhookDelivery` |
| `payment:process_webhook` | 60s | 15 | Yes (24h) | `internal/payments/service.go` | `HandlePaymentWebhookProcess` |
| `ticket:send_confirmation` | 30s | 10 | No | `internal/tickets/service.go` | `HandleTicketSendConfirmation` |
| `push:deliver` | 30s | 10 | Yes (24h) | `internal/worker/handlers.go` (chained from DM/notifications) | `HandlePushDelivery` |
| `dm:notify_request` | 30s | 10 | No | `internal/websocket/dm.go` | `HandleDMNotifyRequest` |
| `dm:notify_message` | 30s | 10 | No | `internal/websocket/dm.go` | `HandleDMNotifyMessage` |

## Default Tier (17)

| Task Type | Timeout | MaxRetry | Unique | Producer | Consumer |
|-----------|---------|----------|--------|----------|----------|
| `email:send` | 30s | 10 | Yes (24h) | `internal/auth/handlers.go`, `internal/tickets/service.go` | `HandleEmailSend` |
| `search:index_entity` | — | — | No | `internal/content/handlers.go` (create/update/delete) | `HandleSearchIndexSync` |
| `content:process_image` | — | — | No | `internal/media/handlers.go` (ConfirmUpload) | `HandleImageProcessing` |
| `ai:process_task` | — | — | No | — (orphaned) | `HandleAITaskProcessing` |
| `content:update_engagement` | — | — | No | — (superseded, dead code) | `HandleEngagementCountUpdate` |
| `content:scan_moderation` | — | — | No | — (orphaned) | `HandleModerationScan` |
| `content:publish_scheduled` | 1min | 5 | Yes (1min) | `internal/content/handlers.go` | `HandleScheduledPublish` |
| `content:fetch_link_preview` | — | — | No | — (orphaned) | `HandleLinkPreviewFetch` |
| `i18n:translate_language_pack` | — | — | No | — (orphaned) | `HandleLanguagePackAITranslate` |
| `content:scan_media` | — | — | No | `internal/media/handlers.go` (ConfirmUpload) | `HandleMediaScan` |
| `content:fan_out_to_feeds` | — | — | No | `internal/feed/service.go` | `HandleContentFanOutToFeeds` |
| `content:generate_embedding` | — | — | No | `internal/content/handlers.go` (create/update) | `HandleAIGenerateContentEmbedding` |
| `content:recalc_review_aggregate` | — | — | No | — (orphaned) | `HandleReviewAggregateRecalc` |
| `ticket:send_status_update` | 30s | 10 | No | `internal/tickets/service.go` | `HandleTicketSendStatusUpdate` |
| `learning:update_track_completion` | — | — | No | — (orphaned) | `HandleLearningUpdateTrackCompletion` |
| `content:process_video` | — | — | No | `internal/media/handlers.go` (ConfirmUpload) | `HandleVideoProcessing` |
| `wallet:process_activity_event` | — | — | Yes (24h) | `internal/wallet/tasks.go` | `HandleWalletProcessActivityEvent` |
| `newsletter:send` | — | — | Yes (24h) | `internal/newsletter/service.go` | `HandleNewsletterSend` |

## Low Tier (31)

| Task Type | Timeout | MaxRetry | Unique | Producer | Consumer |
|-----------|---------|----------|--------|----------|----------|
| `email:send_digest` | — | — | No | — (orphaned) | `HandleDigestEmail` |
| `sitemap:generate` | — | — | No | — (orphaned) | `HandleSitemapGenerate` |
| `rss:regenerate` | — | — | No | — (orphaned) | `HandleRSSRegenerate` |
| `data:export_user_data` | — | — | No | `internal/gdpr/service.go` | `HandleDataExport` |
| `account:deletion_cleanup` | — | — | No | `internal/gdpr/service.go` | `HandleAccountDeletionCleanup` |
| `ad:ingest_event_batch` | — | — | No | — (orphaned) | `HandleAdEventIngest` |
| `maintenance:update_velocity_scores` | — | — | No | Scheduler (*/5 min) | `HandleMaintenanceUpdateVelocityScores` |
| `maintenance:check_sla_breaches` | — | — | No | Scheduler (*/15 min) | `HandleMaintenanceCheckSLABreaches` |
| `maintenance:update_geoip_db` | — | — | No | — (orphaned, unscheduled) | `HandleMaintenanceUpdateGeoIPDB` |
| `space:auto_populate` | — | — | No | — (known dead) | `HandleSpaceAutoPopulate` |
| `analytics:refresh_views` | — | — | No | Scheduler (hourly) | `HandleAnalyticsRefreshViews` |
| `security:scan` | — | — | No | Scheduler (daily 03:00) | `HandleSecurityScan` |
| `maintenance:regenerate_llms_txt` | — | — | No | Scheduler (hourly) | `HandleMaintenanceRegenerateLLMsTxt` |
| `learning:issue_certificate` | — | — | No | — (orphaned) | `HandleLearningIssueCertificate` |
| `db:manual_vacuum` | — | — | No | — (orphaned, no control route) | `HandleDBManualVacuum` |
| `newsletter:digest` | — | — | No | — (orphaned, unscheduled) | `HandleNewsletterDigest` |
| `maintenance:compute_trending` | — | — | No | Scheduler (hourly) | `HandleComputeTrending` |
| `maintenance:compute_suggestions` | — | — | No | Scheduler (6-hourly) | `HandleComputeSuggestions` |
| `maintenance:check_expired_bans` | — | — | No | Scheduler (*/30 min) | `HandleMaintenanceCheckExpiredBans` |
| `maintenance:check_expired_boosts` | — | — | No | Scheduler (*/30 min) | `HandleMaintenanceCheckExpiredBoosts` |
| `maintenance:prune_old_feeds` | — | — | No | Scheduler (weekly) | `HandleMaintenancePruneOldFeeds` |
| `maintenance:prune_audit_logs` | — | — | No | Scheduler (weekly) | `HandleMaintenancePruneAuditLogs` |
| `maintenance:prune_push_tokens` | — | — | No | Scheduler (weekly) | `HandleMaintenancePrunePushTokens` |
| `moderation:auto_hide` | — | — | No | — (orphaned) | `HandleModerationAutoHide` |
| `dm:cleanup_declined_requests` | — | — | No | Scheduler (weekly) | `HandleDMCleanupDeclinedRequests` |
| `coupon:track_events` | — | — | No | Scheduler (hourly) | `HandleCouponTrackEvents` |
| `coupon:expire_check` | — | — | No | Scheduler (hourly) | `HandleCouponExpireCheck` |
| `affiliate:record_click` | — | — | No | — (orphaned) | `HandleAffiliateRecordClick` |
| `affiliate:credit_commission` | 30s | 10 | Yes (24h) | `internal/worker/handlers.go` (payment webhook) | `HandleAffiliateCreditCommission` |
| `affiliate:expire_check` | — | — | No | Scheduler (daily 04:00) | `HandleAffiliateExpireCheck` |
| `redirect:record_hit` | — | — | No | `internal/redirects/service.go` | `HandleRedirectRecordHit` |

## Periodic Sweep Tasks (3)

| Task Type | Timeout | MaxRetry | Unique | Schedule | Consumer |
|-----------|---------|----------|--------|----------|----------|
| `content:publish_sweep` | 1min | 5 | Yes (1min) | `* * * * *` (every minute) | `HandleScheduledPublishSweep` |
| `gdpr:retention_sweep` | 1min | 5 | Yes (1min) | `0 * * * *` (hourly) | `HandleGdprRetentionSweep` |

---

## Orphaned Tasks (registered, no producer)

These tasks have registered handlers but no code enqueues them. They need producers wired or should be deleted.

| Task | Disposition |
|------|-------------|
| `ai:process_task` | Wire producer or delete |
| `content:scan_moderation` | Wire at content create/update |
| `content:fetch_link_preview` | Wire from block editor flow |
| `i18n:translate_language_pack` | Wire from admin i18n endpoint |
| `learning:update_track_completion` | Wire from lesson-completion flow |
| `learning:issue_certificate` | Wire from track-completion flow |
| `email:send_digest` | Schedule + wire digest generation |
| `newsletter:digest` | Schedule + wire digest generation |
| `sitemap:generate` | Schedule (currently served on-request) |
| `ad:ingest_event_batch` | Wire from ad impression tracking |
| `maintenance:update_geoip_db` | Schedule (currently never runs) |
| `db:manual_vacuum` | Add control-panel trigger |
| `space:auto_populate` | Known dead per prior decision |
| `affiliate:record_click` | Wire from referral link click tracking |
| `content:update_engagement` | Superseded by inline paths, delete |
| `content:recalc_review_aggregate` | Wire or delete |

## Deleted Tasks

| Task | Reason |
|------|--------|
| `maintenance:recalc_velocity_scores` | Superseded by `maintenance:update_velocity_scores` which writes to `content_scores.velocity_score` (the table feeds actually read) |

---

## Notes

- **Idempotency keys** use the format `entity_type:entity_id` or `task_type:entity_id` to prevent duplicate processing.
- **Unique keys** use `asynq.Unique(ttl)` to prevent duplicate task enqueueing within the TTL window.
- **Cron specs** can be overridden via `JC_SCHEDULER_CRON_<TASK_TYPE>` environment variables (see `internal/worker/config.go`).
- **Queue tiers** are defined in `internal/worker/enqueuer.go` `taskTier` map.
- **Timeout/Retry policies** are defined in `internal/worker/enqueuer.go` `taskDefaults` function.
