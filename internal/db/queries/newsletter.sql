-- Newsletter system queries (Section 35 — bulk email campaigns).

-- name: GetNewsletterCampaign :one
SELECT subject, html FROM newsletter_campaigns
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpsertNewsletterSendState :exec
INSERT INTO newsletter_send_state (campaign_id, recipient, status)
SELECT @campaign_id, ns.email, 'pending'
FROM newsletter_subscribers ns
WHERE ns.tenant_id = @tenant_id AND ns.status = 'subscribed' AND ns.campaign_enabled = true
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
