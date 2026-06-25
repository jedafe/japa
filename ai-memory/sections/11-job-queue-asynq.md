# Section 11 — Job Queue (Asynq)

**Status:** ✅ Locked

## Decided
Three tiers — critical, default, low — plus a dead-letter queue with exponential backoff. Worker concurrency env-configurable, starting defaults critical=10, default=5, low=2 for a single modest VPS.

## Full V1 Task Catalog
- `email_send` (default)
- `notification_dispatch` (critical)
- `webhook_delivery` (critical)
- `search_index_sync` (default)
- `image_processing` (default)
- `ai_task_processing` (default)
- `engagement_count_update` (default, batched)
- `moderation_scan` (default)
- `digest_email` (low, periodic)
- `scheduled_publish` (default, ETA-scheduled)
- `sitemap_generate` (low, periodic)
- `rss_regenerate` (low, triggered on publish)
- `data_export` (low)
- `account_deletion_cleanup` (low)
- `payment_webhook_process` (critical)
- `link_preview_fetch` (default)
- `language_pack_ai_translate` (default)
- `ad_event_ingest` (low, batched)
- `media_scan` (default, blocks persistence until clear)

Monitoring via Asynqmon, exposed inside Control Panel behind `super_admin`. Alerting via a cron-based health check independent of the queue itself (avoids the alert system failing alongside the queue), surfaced via GlitchTip (Section 27).

## Full Updated Task Catalog (post-audit additions)

**Added tasks (not in original catalog):**
- `notification:dispatch_batch` (critical) — dispatches batched/deduplicated notifications (Section 35)
- `content:fan_out_to_feeds` (default) — pushes new post to follower feeds (Section 32)
- `maintenance:update_velocity_scores` (low, every 10 min) — recalculates Rising sort velocity for recent posts
- `ai:generate_content_embedding` (default) — pgvector embedding for Relevant feed sort and Mentorship matching
- `review_aggregate_recalc` (default) — recalculates review_aggregates on review CRUD (Section 29)
- `db_manual_vacuum` (maintenance tier) — triggered from Control Panel DB health panel (Section 21)
- `newsletter_send` (default) — one-off campaign dispatch
- `newsletter_digest` (low, scheduled) — recurring digest for subscribed lists
- `ticket:send_confirmation` (critical) — email to ticket submitter on create (Section 38)
- `ticket:send_status_update` (default) — email on ticket status change
- `maintenance:check_sla_breaches` (low, every 15 min) — SLA monitoring (Section 38)
- `learning:issue_certificate` (low) — generate + store PDF certificate (Section 36)
- `learning:update_track_completion` (default) — recalculate track completion % (Section 36)
- `maintenance:update_geoip_db` (low, weekly Sunday 02:00 UTC) — MaxMind GeoLite2 update (Section 37)
- `space:auto_populate` (low) — Wikipedia + flagcdn.com + Pexels seeding (Section 05)
- `analytics:refresh_views` (low, hourly) — refresh materialized analytics views (Section 40)
- `security:scan` (low, daily 03:00 UTC) — platform security scan (Section 40)
- `maintenance:regenerate_llms_txt` (low, hourly) — llms.txt regeneration (Section 43)
- `video_processing` (default) — ffmpeg thumbnail + transcode (Section 34)
- `wallet:process_activity_event` (default) — Points ledger entry from activity event (Section 28)
- `referral:process_qualifying_action` (default) — Credit award on referral conversion (V2)
