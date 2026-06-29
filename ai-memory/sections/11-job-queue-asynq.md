# Section 11 — Job Queue (Asynq)

**Status:** ✅ Locked (audit pass 2 — task naming standardised to namespace:action, catalog completed)

## Decided
Three tiers — `critical`, `default`, `low` — plus a dead-letter queue (DLQ) with exponential backoff. Worker concurrency env-configurable; starting defaults: critical=10, default=5, low=2 (single modest VPS). All task names follow the `namespace:action` convention — no exceptions.

## Canonical Task Catalog

All tasks registered in the Asynq server at startup. Claude Code agents: never invent a new task name — add it here first.

### critical tier (SLA: process within seconds; PagerDuty alert on DLQ entry)
```
email:send_verification           — send email verification link on registration
email:send_password_reset         — send password reset link
email:send_ticket_confirmation    — ticket submitted confirmation (Section 38)
notification:dispatch             — in-app + email + push notification to a recipient
payment:process_webhook           — Stripe/Paystack webhook handler (Section 17)
dm:notify_request                 — DM request received push notification (Section 48)
dm:notify_message                 — DM message received push notification when conv not open (Section 48)
```

### default tier (SLA: process within minutes)
```
content:fan_out_author_followers  — push published post to all author's followers (Section 32)
content:fan_out_space             — push published post to members of one assigned space (Section 32)
content:index_search              — sync a content row to Meilisearch after create/update/delete (Section 13)
content:generate_embedding        — pgvector embedding for Relevant sort + Mentorship match (Section 20/32)
content:scan_media                — scan uploaded media for CSAM/malware before confirming (Section 34)
content:process_video             — ffmpeg thumbnail + transcode (Section 34)
content:fetch_link_preview        — OG/oEmbed scrape for external links in TipTap body (Section 15)
content:publish_scheduled         — flip status=scheduled → published when publish_at is due (Section 14)
content:recalc_review_aggregate   — recalculate review_aggregates after review CRUD (Section 29)
user:index_search                 — sync a user row to Meilisearch after profile update (Section 13)
space:index_search                — sync a space row to Meilisearch after create/update (Section 13)
wallet:process_activity_event     — Points ledger entry from activity event (Section 28)
learning:update_track_completion  — recalculate track completion % on lesson complete (Section 36)
ticket:send_status_update         — email on ticket status change (Section 38)
notification:send_broadcast_batch — fan-out chunk of a broadcast_notifications send (Section 22)
i18n:translate_language_pack      — AI translation of a language pack chunk (Section 18)
```

### low tier (SLA: process within hours; never time-critical)
```
content:fan_out_bubble            — bubble post up to parent primary spaces with 'from [X]' label (Section 32)
maintenance:update_velocity_scores — recalculate Rising sort velocity for posts < 7 days old (every 10 min)
maintenance:prune_old_feeds       — delete feeds rows older than retention window (weekly)
maintenance:prune_audit_logs      — delete audit_logs rows older than audit.log_retention_days (weekly)
maintenance:check_expired_bans    — lift expired user_bans, re-enable users (every 30 min)
maintenance:check_sla_breaches    — flag tickets past SLA deadline (every 15 min) (Section 38)
maintenance:update_geoip_db       — MaxMind GeoLite2 weekly update (Section 37)
maintenance:regenerate_llms_txt   — llms.txt + robots.txt regeneration (hourly) (Section 43)
space:auto_populate               — Wikipedia + flagcdn.com + Pexels seeding for new geo space (Section 05)
analytics:refresh_views           — refresh materialized analytics views (hourly) (Section 40)
security:scan                     — platform security scan (daily 03:00 UTC) (Section 40)
learning:issue_certificate        — generate + store PDF certificate on course completion (Section 36)
data:export_user_data             — GDPR data export package generation (Section 19)
account:deletion_cleanup          — remove user data after deletion grace period (Section 19)
email:send_digest                 — periodic digest email to subscribed users
dm:cleanup_declined_requests      — purge dm_requests declined > 90 days ago (Section 48)
sitemap:generate                  — full sitemap XML regeneration (triggered on publish)
rss:regenerate                    — RSS feed XML rebuild for a space/tag (triggered on publish)
ad:ingest_event_batch             — flush buffered ad impression/click events (Section 25)
referral:process_qualifying_action — Credit award on referral conversion (V2 deferred, schema ready)
```

## Monitoring
Asynqmon dashboard in Control Panel (Section 21), gated by `super_admin`. GlitchTip (Section 27) alerts on DLQ entries. A cron-based health check runs independently of the queue itself (avoids the alert system failing alongside the queue).

## DLQ Behaviour
Failed tasks retry with exponential backoff (1 min → 5 min → 20 min → 1 hr → 4 hr → 24 hr, max 6 retries). After max retries, task moves to DLQ. DLQ entries are visible in Asynqmon and trigger a GlitchTip alert. Support can re-queue or discard manually.
