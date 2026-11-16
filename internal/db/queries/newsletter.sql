-- Newsletter system queries (Section 35 — bulk email campaigns).

-- name: GetNewsletterCampaign :one
SELECT subject, html FROM newsletter_campaigns
WHERE id = @id;

-- Send-state population is segment-aware: each upsert mirrors exactly one
-- segment's membership rules so pending-row consumption (resume) works
-- uniformly for every campaign type and the send_state table is an accurate
-- record of the campaign's real recipient set (§62, ADR-080 §3).
-- name: UpsertNewsletterSendState :exec
INSERT INTO newsletter_send_state (campaign_id, recipient, status)
SELECT @campaign_id, ns.email, 'pending'
FROM newsletter_subscribers ns
WHERE ns.status = 'subscribed' AND ns.campaign_enabled = true
ON CONFLICT (campaign_id, recipient) DO NOTHING;

-- name: UpsertNewsletterSendStatePro :exec
INSERT INTO newsletter_send_state (campaign_id, recipient, status)
SELECT @campaign_id, ns.email, 'pending'
FROM newsletter_subscribers ns
JOIN subscriptions sub ON sub.user_id = ns.user_id
WHERE ns.status = 'subscribed' AND ns.campaign_enabled = true
 AND sub.status = 'active' AND sub.plan LIKE 'pro_%'
ON CONFLICT (campaign_id, recipient) DO NOTHING;

-- name: UpsertNewsletterSendStateBySpace :exec
INSERT INTO newsletter_send_state (campaign_id, recipient, status)
SELECT @campaign_id, ns.email, 'pending'
FROM newsletter_subscribers ns
JOIN follows f ON f.follower_id = ns.user_id
WHERE ns.status = 'subscribed' AND ns.campaign_enabled = true
 AND f.entity_type = 'space' AND f.entity_id = @segment_id
ON CONFLICT (campaign_id, recipient) DO NOTHING;

-- name: UpsertNewsletterSendStateByTag :exec
INSERT INTO newsletter_send_state (campaign_id, recipient, status)
SELECT @campaign_id, ns.email, 'pending'
FROM newsletter_subscribers ns
JOIN follows f ON f.follower_id = ns.user_id
WHERE ns.status = 'subscribed' AND ns.campaign_enabled = true
 AND f.entity_type = 'tag' AND f.entity_id = @segment_id
ON CONFLICT (campaign_id, recipient) DO NOTHING;

-- Atomic send claims (§62): the first writer to flip a campaign into
-- 'sending' owns the send; every other claim loses. ClaimNewsletterSendNow
-- backs the admin send endpoint (claim-before-enqueue) and worker-side
-- claiming for directly-enqueued tasks; ClaimNewsletterScheduledSend backs
-- ProcessAt tasks — it succeeds only while the campaign is still
-- 'scheduled' for exactly the task's slot, so cancelling, unscheduling,
-- rescheduling, or a prior send-now claim all invalidate the queued task.
-- name: ClaimNewsletterSendNow :execrows
UPDATE newsletter_campaigns
SET status = 'sending', updated_at = now()
WHERE id = @id AND status IN ('draft', 'scheduled');

-- name: ClaimNewsletterScheduledSend :execrows
UPDATE newsletter_campaigns
SET status = 'sending', updated_at = now()
WHERE id = @id AND status = 'scheduled'
 AND date_trunc('second', scheduled_at) = @scheduled_at;

-- RevertNewsletterSendClaim undoes a claim when the send task could not be
-- enqueued — restores the pre-claim status, guarded by status='sending' so
-- it can never clobber a state that arrived after our claim.
-- name: RevertNewsletterSendClaim :exec
UPDATE newsletter_campaigns
SET status = @status, updated_at = now()
WHERE id = @id AND status = 'sending';

-- name: GetNewsletterCampaignStatus :one
SELECT status FROM newsletter_campaigns
WHERE id = @id;

-- name: ListPendingNewsletterRecipients :many
SELECT recipient FROM newsletter_send_state
WHERE campaign_id = @campaign_id AND status = 'pending'
ORDER BY recipient
LIMIT @page_limit;

-- name: CountNewsletterRecipientStates :one
SELECT
  COUNT(*) FILTER (WHERE status = 'sent')   AS sent,
  COUNT(*) FILTER (WHERE status = 'failed') AS failed
FROM newsletter_send_state
WHERE campaign_id = @campaign_id;

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
SELECT u.id::text AS user_id, u.email, u.username
FROM users u
JOIN newsletter_subscribers ns ON ns.email = u.email
WHERE ns.status = 'subscribed' AND ns.digest_enabled = true
  AND u.status = 'active' AND u.email_verified_at IS NOT NULL AND u.deleted_at IS NULL;

-- name: ListDigestSubscriberEmails :many
SELECT email FROM newsletter_subscribers
WHERE status = 'subscribed' AND digest_enabled = true;
