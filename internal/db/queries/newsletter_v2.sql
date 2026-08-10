-- newsletter_v2.sql — Newsletter & Digest V2 queries (Section 62, ADR-080).
-- Covers subscriber preferences, unsubscribe tokens, and V2 campaign management.
-- Ref: japaconnect-engineering/sections/62-newsletter-digest.md
--      japaconnect-engineering/adr/ADR-080-newsletter-digest.md

-- =====================================================================
-- Subscriber preferences
-- =====================================================================

-- name: GetSubscriberByEmail :one
SELECT * FROM newsletter_subscribers
WHERE tenant_id = @tenant_id AND email = @email;

-- name: GetSubscriberByID :one
SELECT * FROM newsletter_subscribers
WHERE tenant_id = @tenant_id AND id = @id;

-- name: GetSubscriberByUserID :one
SELECT * FROM newsletter_subscribers
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: Subscribe :one
INSERT INTO newsletter_subscribers (tenant_id, user_id, email, status, source)
VALUES (@tenant_id, sqlc.narg(user_id), @email, 'subscribed', @source)
ON CONFLICT (tenant_id, email) DO UPDATE SET
    status = 'subscribed',
    unsubscribed_at = NULL,
    subscribed_at = now()
RETURNING *;

-- name: Unsubscribe :exec
UPDATE newsletter_subscribers
SET status = 'unsubscribed', unsubscribed_at = now()
WHERE tenant_id = @tenant_id AND email = @email;

-- name: UnsubscribeByID :exec
UPDATE newsletter_subscribers
SET status = 'unsubscribed', unsubscribed_at = now()
WHERE tenant_id = @tenant_id AND id = @id;

-- name: UpdatePreferences :one
UPDATE newsletter_subscribers
SET
    digest_enabled   = COALESCE(sqlc.narg(digest_enabled), digest_enabled),
    campaign_enabled = COALESCE(sqlc.narg(campaign_enabled), campaign_enabled)
WHERE tenant_id = @tenant_id AND id = @id
RETURNING *;

-- name: MarkBounced :exec
UPDATE newsletter_subscribers
SET status = 'bounced'
WHERE tenant_id = @tenant_id AND email = @email;

-- name: MarkComplained :exec
UPDATE newsletter_subscribers
SET status = 'complained'
WHERE tenant_id = @tenant_id AND email = @email;

-- =====================================================================
-- Unsubscribe tokens
-- =====================================================================

-- name: CreateUnsubscribeToken :one
INSERT INTO newsletter_unsubscribe_tokens (token, subscriber_id)
VALUES (@token, @subscriber_id)
RETURNING *;

-- name: GetUnsubscribeToken :one
SELECT * FROM newsletter_unsubscribe_tokens
WHERE token = @token;

-- name: DeleteUnsubscribeToken :exec
DELETE FROM newsletter_unsubscribe_tokens WHERE token = @token;

-- =====================================================================
-- Admin: subscriber listing and stats
-- =====================================================================

-- name: AdminListSubscribers :many
SELECT * FROM newsletter_subscribers
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status)
ORDER BY subscribed_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountSubscribers :one
SELECT COUNT(*) AS count
FROM newsletter_subscribers
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status);

-- name: AdminSubscriberStats :many
SELECT status, COUNT(*) AS count
FROM newsletter_subscribers
WHERE tenant_id = @tenant_id
GROUP BY status
ORDER BY count DESC;

-- =====================================================================
-- V2 Campaign management
-- =====================================================================

-- name: CreateCampaign :one
INSERT INTO newsletter_campaigns (
    tenant_id, name, subject, html, preview_text, body, segment, segment_id,
    status, scheduled_at, created_by
)
VALUES (
    @tenant_id, @name, @subject, @html, sqlc.narg(preview_text), @body,
    @segment, sqlc.narg(segment_id), @status, sqlc.narg(scheduled_at), @created_by
)
RETURNING *;

-- name: UpdateCampaign :one
UPDATE newsletter_campaigns
SET
    name          = COALESCE(sqlc.narg(name), name),
    subject       = COALESCE(sqlc.narg(subject), subject),
    preview_text  = COALESCE(sqlc.narg(preview_text), preview_text),
    body          = COALESCE(sqlc.narg(body), body),
    segment       = COALESCE(sqlc.narg(segment), segment),
    segment_id    = COALESCE(sqlc.narg(segment_id), segment_id),
    status        = COALESCE(sqlc.narg(status), status),
    scheduled_at  = COALESCE(sqlc.narg(scheduled_at), scheduled_at)
WHERE tenant_id = @tenant_id AND id = @id
RETURNING *;

-- name: MarkCampaignSending :exec
UPDATE newsletter_campaigns
SET status = 'sending'
WHERE tenant_id = @tenant_id AND id = @id;

-- name: MarkCampaignSent :exec
UPDATE newsletter_campaigns
SET status = 'sent', sent_at = now(), recipient_count = @recipient_count
WHERE tenant_id = @tenant_id AND id = @id;

-- name: IncrementCampaignOpens :exec
UPDATE newsletter_campaigns
SET open_count = open_count + 1
WHERE tenant_id = @tenant_id AND id = @id;

-- name: IncrementCampaignClicks :exec
UPDATE newsletter_campaigns
SET click_count = click_count + 1
WHERE tenant_id = @tenant_id AND id = @id;

-- name: AdminListCampaigns :many
SELECT * FROM newsletter_campaigns
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountCampaigns :one
SELECT COUNT(*) AS count
FROM newsletter_campaigns
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status);

-- =====================================================================
-- Digest: get subscribers with digest enabled
-- =====================================================================

-- name: GetDigestSubscribers :many
SELECT * FROM newsletter_subscribers
WHERE tenant_id = @tenant_id
  AND status = 'subscribed'
  AND digest_enabled = true
ORDER BY subscribed_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountDigestSubscribers :one
SELECT COUNT(*) AS count
FROM newsletter_subscribers
WHERE tenant_id = @tenant_id
  AND status = 'subscribed'
  AND digest_enabled = true;

-- =====================================================================
-- Admin: update subscriber status (for the PATCH subscriber endpoint)
-- =====================================================================

-- name: UpdateSubscriberStatus :one
UPDATE newsletter_subscribers
SET status = @status,
    unsubscribed_at = CASE WHEN @status = 'unsubscribed' THEN now() ELSE unsubscribed_at END
WHERE tenant_id = @tenant_id AND id = @id
RETURNING *;
