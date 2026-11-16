-- newsletter_v2.sql — Newsletter & Digest V2 queries (Section 62, ADR-080).
-- Covers subscriber preferences, unsubscribe tokens, and V2 campaign management.
-- Ref: japaconnect-engineering/sections/62-newsletter-digest.md
--      japaconnect-engineering/adr/ADR-080-newsletter-digest.md

-- =====================================================================
-- Subscriber preferences
-- =====================================================================

-- name: GetSubscriberByEmail :one
SELECT * FROM newsletter_subscribers
WHERE email = @email;

-- name: GetSubscriberByID :one
SELECT * FROM newsletter_subscribers
WHERE id = @id;

-- name: GetSubscriberByUserID :one
SELECT * FROM newsletter_subscribers
WHERE user_id = @user_id;

-- name: Subscribe :one
INSERT INTO newsletter_subscribers (user_id, email, status, source) VALUES (sqlc.narg(user_id), @email, 'subscribed', @source)

-- Re-subscribe resurrects 'unsubscribed' rows only. Provider-flagged
-- 'bounced'/'complained' rows must stay suppressed (ADR-080 §4) — resuming
-- sends to known-bad addresses re-incurs bounce/complaint penalties.
ON CONFLICT (email) DO UPDATE SET
 status = CASE WHEN newsletter_subscribers.status IN ('bounced', 'complained')
               THEN newsletter_subscribers.status ELSE 'subscribed' END,
 unsubscribed_at = CASE WHEN newsletter_subscribers.status IN ('bounced', 'complained')
                        THEN newsletter_subscribers.unsubscribed_at ELSE NULL END,
 subscribed_at = now()
RETURNING *;

-- Double opt-in variant (newsletter.double_opt_in, migration 001170): new
-- and previously-unsubscribed rows land in 'pending' until the emailed
-- confirm link is hit. Already-'subscribed' rows keep their status — a
-- re-signup must not demote an active subscriber back to pending.
-- 'bounced'/'complained' stay suppressed as in Subscribe.
-- name: SubscribePending :one
INSERT INTO newsletter_subscribers (user_id, email, status, source) VALUES (sqlc.narg(user_id), @email, 'pending', @source)

ON CONFLICT (email) DO UPDATE SET
 status = CASE WHEN newsletter_subscribers.status IN ('bounced', 'complained', 'subscribed')
               THEN newsletter_subscribers.status ELSE 'pending' END,
 unsubscribed_at = CASE WHEN newsletter_subscribers.status IN ('bounced', 'complained', 'subscribed')
                        THEN newsletter_subscribers.unsubscribed_at ELSE NULL END,
 subscribed_at = now()
RETURNING *;

-- name: ConfirmSubscriber :one
UPDATE newsletter_subscribers
SET status = 'subscribed', subscribed_at = now(), unsubscribed_at = NULL
WHERE id = @id AND status = 'pending'
RETURNING *;

-- name: Unsubscribe :exec
UPDATE newsletter_subscribers
SET status = 'unsubscribed', unsubscribed_at = now()
WHERE email = @email;

-- name: UnsubscribeByID :one
UPDATE newsletter_subscribers
SET status = 'unsubscribed', unsubscribed_at = now()
WHERE id = @id
RETURNING *;

-- name: UpdatePreferences :one
UPDATE newsletter_subscribers
SET
 digest_enabled = COALESCE(sqlc.narg(digest_enabled), digest_enabled),
 campaign_enabled = COALESCE(sqlc.narg(campaign_enabled), campaign_enabled)
WHERE id = @id
RETURNING *;

-- name: MarkBounced :exec
UPDATE newsletter_subscribers
SET status = 'bounced'
WHERE email = @email;

-- name: MarkComplained :exec
UPDATE newsletter_subscribers
SET status = 'complained'
WHERE email = @email;

-- =====================================================================
-- Unsubscribe tokens
-- =====================================================================

-- name: CreateUnsubscribeToken :one
INSERT INTO newsletter_unsubscribe_tokens (token, subscriber_id, purpose) VALUES (@token, @subscriber_id, 'unsubscribe')

RETURNING *;

-- name: GetUnsubscribeToken :one
SELECT * FROM newsletter_unsubscribe_tokens
WHERE token = @token AND purpose = 'unsubscribe';

-- Double opt-in confirm tokens share the table but carry purpose='confirm'
-- (migration 001170) — a token minted for one flow can never be replayed
-- in the other.
-- name: CreateConfirmToken :one
INSERT INTO newsletter_unsubscribe_tokens (token, subscriber_id, purpose) VALUES (@token, @subscriber_id, 'confirm')

RETURNING *;

-- Confirm tokens carry a dedicated 7-day expiry (§62, audit fix): shorter
-- than the unsubscribe-token window because a confirmation link older than
-- a week is stale by definition. Enforced at read time as well as by the
-- purpose-scoped weekly GC so an expired link is rejected even between GC
-- runs; the subscriber re-subscribes to re-mint one.
-- name: GetConfirmToken :one
SELECT * FROM newsletter_unsubscribe_tokens
WHERE token = @token AND purpose = 'confirm'
 AND created_at >= now() - interval '7 days';

-- name: DeleteUnsubscribeToken :exec
DELETE FROM newsletter_unsubscribe_tokens WHERE token = @token;

-- =====================================================================
-- Admin: subscriber listing and stats
-- =====================================================================

-- name: AdminListSubscribers :many
SELECT * FROM newsletter_subscribers
WHERE (@status::text = '' OR status = @status)
 AND (@search::text = '' OR email ILIKE '%' || @search || '%')
ORDER BY subscribed_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountSubscribers :one
SELECT COUNT(*) AS count
FROM newsletter_subscribers
WHERE (@status::text = '' OR status = @status)
 AND (@search::text = '' OR email ILIKE '%' || @search || '%');

-- name: AdminSubscriberStats :many
SELECT status, COUNT(*) AS count
FROM newsletter_subscribers

GROUP BY status
ORDER BY count DESC;

-- =====================================================================
-- V2 Campaign management
-- =====================================================================

-- name: CreateCampaign :one
INSERT INTO newsletter_campaigns (name, subject, html, preview_text, body, segment, segment_id, status, scheduled_at, created_by) VALUES (@name, @subject, @html, sqlc.narg(preview_text), @body, @segment, sqlc.narg(segment_id), @status, sqlc.narg(scheduled_at), @created_by)

RETURNING *;

-- name: UpdateCampaign :one
UPDATE newsletter_campaigns
SET
 name = COALESCE(sqlc.narg(name), name),
 subject = COALESCE(sqlc.narg(subject), subject),
 html = COALESCE(sqlc.narg(html), html),
 preview_text = COALESCE(sqlc.narg(preview_text), preview_text),
 body = COALESCE(sqlc.narg(body), body),
 segment = COALESCE(sqlc.narg(segment), segment),
 segment_id = COALESCE(sqlc.narg(segment_id), segment_id),
 status = COALESCE(sqlc.narg(status), status),
 scheduled_at = COALESCE(sqlc.narg(scheduled_at), scheduled_at)
WHERE id = @id
RETURNING *;

-- The status='sending' transition is owned by the atomic claims in
-- newsletter.sql (ClaimNewsletterSendNow / ClaimNewsletterScheduledSend) —
-- an unconditional mark would let a stale writer clobber terminal states.

-- Guarded by status='sending' AND the send_id lease: only the current
-- claim's owner may stamp 'sent' — a superseded task (cancelled, reset,
-- or replaced by a newer claim) can never clobber the newer send.
-- name: MarkCampaignSent :exec
UPDATE newsletter_campaigns
SET status = 'sent', sent_at = now(), recipient_count = @recipient_count
WHERE id = @id AND status = 'sending' AND send_id = @send_id;

-- name: IncrementCampaignOpens :exec
UPDATE newsletter_campaigns
SET open_count = open_count + 1
WHERE id = @id;

-- name: IncrementCampaignClicks :exec
UPDATE newsletter_campaigns
SET click_count = click_count + 1
WHERE id = @id;

-- name: AdminListCampaigns :many
SELECT * FROM newsletter_campaigns
WHERE (@status::text = '' OR status = @status)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountCampaigns :one
SELECT COUNT(*) AS count
FROM newsletter_campaigns
WHERE (@status::text = '' OR status = @status);

-- =====================================================================
-- Digest: get subscribers with digest enabled
-- =====================================================================

-- name: GetDigestSubscribers :many
SELECT * FROM newsletter_subscribers
WHERE status = 'subscribed'
 AND digest_enabled = true
ORDER BY subscribed_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountDigestSubscribers :one
SELECT COUNT(*) AS count
FROM newsletter_subscribers
WHERE status = 'subscribed'
 AND digest_enabled = true;

-- =====================================================================
-- Admin: update subscriber status (for the PATCH subscriber endpoint)
-- =====================================================================

-- name: UpdateSubscriberStatus :one
UPDATE newsletter_subscribers
SET status = @status,
 unsubscribed_at = CASE WHEN @status = 'unsubscribed' THEN now() ELSE unsubscribed_at END
WHERE id = @id
RETURNING *;

-- =====================================================================
-- Campaign send: fetch full campaign row (incl. segment) for the worker.
-- =====================================================================

-- name: GetNewsletterCampaignForSend :one
SELECT id, subject, html, body, segment, segment_id, status, scheduled_at
FROM newsletter_campaigns
WHERE id = @id;

-- =====================================================================
-- Segment-aware send-state population lives in newsletter.sql
-- (UpsertNewsletterSendState{,Pro,BySpace,ByTag}) — those INSERT…SELECTs
-- are the single source of truth for segment membership at send time.
-- The old paged recipient SELECTs were removed: the worker consumes
-- send_state 'pending' rows for every segment, which makes crash-resume
-- uniform and stops scoped sends from ever resolving outside their
-- segment (NL-01 class).
-- =====================================================================

-- name: DeleteOldUnsubscribeTokens :exec
-- Weekly GC (§62 Compliance), purpose-scoped: one-click unsubscribe tokens
-- are minted per recipient per send and need no long retention — a link
-- older than 90 days can simply be treated as invalid. Double opt-in
-- confirm tokens get a dedicated 7-day expiry (audit fix): a confirmation
-- link that old is stale by definition — re-subscribing re-mints one.
DELETE FROM newsletter_unsubscribe_tokens
WHERE (purpose = 'unsubscribe' AND created_at < now() - interval '90 days')
   OR (purpose = 'confirm' AND created_at < now() - interval '7 days');
