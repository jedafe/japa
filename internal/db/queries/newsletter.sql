-- Newsletter system queries (Section 35 — bulk email campaigns).

-- name: GetNewsletterCampaign :one
SELECT subject, html FROM newsletter_campaigns
WHERE id = @id;

-- name: UpsertNewsletterSendState :exec
INSERT INTO newsletter_send_state (campaign_id, recipient, status)
SELECT @campaign_id, ns.email, 'pending'
FROM newsletter_subscribers ns
WHERE ns.status = 'subscribed' AND ns.campaign_enabled = true
ON CONFLICT (campaign_id, recipient) DO NOTHING;

-- name: ListPendingNewsletterRecipients :many
SELECT recipient FROM newsletter_send_state
WHERE campaign_id = @campaign_id AND status = 'pending';

-- name: MarkNewsletterRecipientFailed :exec
UPDATE newsletter_send_state
SET status = 'failed', error = @error
WHERE campaign_id = @campaign_id AND recipient = @recipient;

-- name: MarkNewsletterRecipientSent :exec
UPDATE newsletter_send_state
SET status = 'sent', sent_at = now()
WHERE campaign_id = @campaign_id AND recipient = @recipient;

-- Digest subscriber population (§11 email:send_digest; BUG-028 — moved from
-- raw SQL in internal/worker/handlers.go).

-- name: ListDigestSubscribedUsers :many
SELECT u.id::text AS user_id, u.email
FROM users u
JOIN newsletter_subscribers ns ON ns.email = u.email
WHERE ns.status = 'subscribed' AND ns.digest_enabled = true
  AND u.status = 'active' AND u.email_verified_at IS NOT NULL AND u.deleted_at IS NULL;

-- name: ListDigestSubscriberEmails :many
SELECT email FROM newsletter_subscribers
WHERE status = 'subscribed' AND digest_enabled = true;
