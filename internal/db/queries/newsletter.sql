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
-- 'sending' owns the send and stamps send_id as the lease identity —
-- the asynq task ID for worker-claimed sends (stable across retries, so
-- the sending-leg is a self-resume) or a caller-generated ID for
-- endpoint-claimed sends. Every other claim loses (pgx.ErrNoRows).
-- ClaimNewsletterSendNow backs the admin send endpoint (claim-before-
-- enqueue) and worker-side claiming for directly-enqueued tasks;
-- ClaimNewsletterScheduledSend backs ProcessAt tasks — the scheduled leg
-- requires the campaign still 'scheduled' for exactly the task's slot, so
-- cancelling, unscheduling, rescheduling, or a prior send-now claim all
-- invalidate the queued task.
-- The prev CTE captures the pre-claim status under a row lock so the send
-- endpoint can revert to exactly what the claim displaced — a concurrent
-- PATCH in the handler's fetch→claim gap can't be silently undone.
-- name: ClaimNewsletterSendNow :one
WITH prev AS (
  SELECT status FROM newsletter_campaigns WHERE id = @id FOR UPDATE
)
UPDATE newsletter_campaigns c
SET status = 'sending', send_id = @send_id, updated_at = now()
FROM prev
WHERE c.id = @id AND (
     c.status IN ('draft', 'scheduled')
  OR (c.status = 'sending' AND c.send_id = @send_id)
)
RETURNING c.send_id, prev.status AS prev_status;

-- name: ClaimNewsletterScheduledSend :one
UPDATE newsletter_campaigns c
SET status = 'sending', send_id = @send_id, updated_at = now()
WHERE c.id = @id AND (
     (c.status = 'scheduled' AND date_trunc('second', c.scheduled_at) = @scheduled_at)
  OR (c.status = 'sending' AND c.send_id = @send_id)
)
RETURNING c.send_id;

-- GetNewsletterSendLease returns the current send lease for the fencing
-- check: the worker proceeds only while status='sending' and send_id is
-- still the lease it claimed (or adopted) at send start.
-- name: GetNewsletterSendLease :one
SELECT status, send_id FROM newsletter_campaigns
WHERE id = @id;

-- RevertNewsletterSendClaim undoes a claim when the send task could not be
-- enqueued — restores the pre-claim status, fenced by status='sending' AND
-- send_id so it can never clobber a claim that landed after ours (PATCH
-- reset → re-claim between our claim and the failed enqueue).
-- name: RevertNewsletterSendClaim :exec
UPDATE newsletter_campaigns
SET status = @status, send_id = NULL, updated_at = now()
WHERE id = @id AND status = 'sending' AND send_id = @send_id;

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

-- Paged like the campaign fan-out (audit DG-11): the full digest-enabled
-- list is never materialized at once. Keyset-paged — `email >
-- @after_email` is stable under mid-scan subscribe/unsubscribe, where
-- OFFSET would skip or dupe recipients as rows enter/leave the
-- digest-enabled set between pages.
-- name: ListDigestSubscriberEmailsPage :many
SELECT email FROM newsletter_subscribers
WHERE status = 'subscribed' AND digest_enabled = true AND email > @after_email
ORDER BY email
LIMIT @page_limit;
