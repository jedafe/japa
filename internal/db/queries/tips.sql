-- §125 Donations & Tips queries.
-- Ref: .jeos/sections/125-donations-tips.md

-- name: UpsertTipSettings :one
INSERT INTO tip_settings (owner_type, owner_id, tips_enabled, accept_points, accept_credits,
    accept_fiat, accept_external, oc_collective_slug, oc_api_token_encrypted,
    min_tip_amount, thank_you_message)
VALUES (@owner_type, @owner_id, @tips_enabled, @accept_points, @accept_credits,
    @accept_fiat, @accept_external, @oc_collective_slug, @oc_api_token_encrypted,
    @min_tip_amount, @thank_you_message)
ON CONFLICT (owner_type, owner_id) DO UPDATE SET
    tips_enabled = @tips_enabled, accept_points = @accept_points,
    accept_credits = @accept_credits, accept_fiat = @accept_fiat,
    accept_external = @accept_external,
    oc_collective_slug = COALESCE(@oc_collective_slug, tip_settings.oc_collective_slug),
    oc_api_token_encrypted = COALESCE(@oc_api_token_encrypted, tip_settings.oc_api_token_encrypted),
    min_tip_amount = @min_tip_amount, thank_you_message = @thank_you_message,
    updated_at = now()
RETURNING *;

-- name: GetTipSettings :one
SELECT * FROM tip_settings WHERE owner_type = @owner_type AND owner_id = @owner_id;

-- name: ListTipSettingsWithOCToken :many
SELECT * FROM tip_settings WHERE oc_collective_slug IS NOT NULL AND oc_api_token_encrypted IS NOT NULL;

-- name: SetOCCredentials :exec
UPDATE tip_settings SET oc_collective_slug = @oc_collective_slug,
    oc_api_token_encrypted = @oc_api_token_encrypted, oc_webhook_secret = @oc_webhook_secret,
    updated_at = now()
WHERE owner_type = @owner_type AND owner_id = @owner_id;

-- name: ClearOCCredentials :exec
UPDATE tip_settings SET oc_collective_slug = NULL, oc_api_token_encrypted = NULL,
    oc_webhook_secret = NULL, updated_at = now()
WHERE owner_type = @owner_type AND owner_id = @owner_id;

-- name: GetTipSettingsByOCWebhookSecret :one
SELECT * FROM tip_settings WHERE oc_webhook_secret = @oc_webhook_secret;

-- name: CreateTipExternalLink :one
INSERT INTO tip_external_links (tip_settings_id, kind, label, url, position)
VALUES (@tip_settings_id, @kind, @label, @url, @position) RETURNING *;

-- name: UpdateTipExternalLink :one
UPDATE tip_external_links SET kind = @kind, label = @label, url = @url, position = @position
WHERE id = @id AND tip_settings_id = @tip_settings_id RETURNING *;

-- name: DeleteTipExternalLink :exec
DELETE FROM tip_external_links WHERE id = @id AND tip_settings_id = @tip_settings_id;

-- name: ListTipExternalLinks :many
SELECT * FROM tip_external_links WHERE tip_settings_id = @tip_settings_id ORDER BY position, created_at;

-- name: InsertTip :one
INSERT INTO tips (sender_user_id, external_sender_name, recipient_type, recipient_user_id,
    recipient_organization_id, content_id, method, points_amount, credits_amount,
    fiat_amount, fiat_currency, fee_amount, net_credits, payment_id, oc_contribution_id,
    message, is_anonymous, status, idempotency_key)
VALUES (@sender_user_id, @external_sender_name, @recipient_type, @recipient_user_id,
    @recipient_organization_id, @content_id, @method, @points_amount, @credits_amount,
    @fiat_amount, @fiat_currency, @fee_amount, @net_credits, @payment_id, @oc_contribution_id,
    @message, @is_anonymous, @status, @idempotency_key)
RETURNING *;

-- name: GetTip :one
SELECT * FROM tips WHERE id = @id;

-- name: GetTipByPaymentID :one
SELECT * FROM tips WHERE payment_id = @payment_id;

-- name: GetTipByOCContributionID :one
SELECT * FROM tips WHERE oc_contribution_id = @oc_contribution_id;

-- name: GetTipByIdempotencyKey :one
SELECT * FROM tips WHERE sender_user_id = @sender_user_id AND idempotency_key = @idempotency_key;

-- name: UpdateTipStatus :exec
UPDATE tips SET status = @status, updated_at = now()
WHERE id = @id;

-- name: AttachTipPayment :exec
UPDATE tips SET payment_id = @payment_id, updated_at = now()
WHERE id = @id;

-- name: SettleTip :exec
-- Conditional on pending so a settle racing a fail/refund can't resurrect.
UPDATE tips SET status = 'completed', net_credits = @net_credits, updated_at = now()
WHERE id = @id AND status = 'pending';

-- name: ListTipsSent :many
SELECT * FROM tips WHERE sender_user_id = @sender_user_id
ORDER BY created_at DESC LIMIT @page_limit OFFSET @page_offset;

-- name: ListTipsReceived :many
SELECT * FROM tips WHERE recipient_type = 'user' AND recipient_user_id = @recipient_user_id
   AND status = 'completed'
ORDER BY created_at DESC LIMIT @page_limit OFFSET @page_offset;

-- name: CountTipsReceived :one
SELECT count(*)::int FROM tips WHERE recipient_type = 'user' AND recipient_user_id = @recipient_user_id AND status = 'completed';

-- name: SumTipperPointsToday :one
-- Only completed tips count — a refunded tip returned the points, so it
-- must not keep eating the sender's daily anti-laundering cap.
SELECT COALESCE(sum(points_amount),0)::bigint FROM tips
WHERE sender_user_id = @sender_user_id AND method = 'points'
  AND status = 'completed'
  AND created_at >= (date_trunc('day', timezone('UTC', now()))) AT TIME ZONE 'UTC';

-- name: ContentTipSummary :one
SELECT count(*)::int AS tip_count,
       COALESCE(sum(net_credits),0)::bigint AS credits_total,
       COALESCE(sum(points_amount),0)::bigint AS points_total
FROM tips WHERE content_id = @content_id AND status = 'completed';

-- name: AdminListTips :many
SELECT t.* FROM tips t
WHERE (@status::text = '' OR t.status = @status)
  AND (@method::text = '' OR t.method = @method)
  AND (@recipient_type::text = '' OR t.recipient_type = @recipient_type)
  AND (@q::text = '' OR t.external_sender_name ILIKE '%' || @q || '%'
       OR EXISTS (SELECT 1 FROM users u WHERE u.id = t.sender_user_id AND u.username ILIKE '%' || @q || '%'))
ORDER BY t.created_at DESC LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountTips :one
SELECT count(*)::int FROM tips t
WHERE (@status::text = '' OR t.status = @status)
  AND (@method::text = '' OR t.method = @method)
  AND (@recipient_type::text = '' OR t.recipient_type = @recipient_type)
  AND (@q::text = '' OR t.external_sender_name ILIKE '%' || @q || '%'
       OR EXISTS (SELECT 1 FROM users u WHERE u.id = t.sender_user_id AND u.username ILIKE '%' || @q || '%'));

-- name: AdminTipStats :one
SELECT count(*)::int AS total,
       COALESCE(sum(net_credits) FILTER (WHERE status='completed'),0)::bigint AS credits_settled,
       COALESCE(sum(fiat_amount) FILTER (WHERE status='completed'),0)::bigint AS fiat_minor_settled,
       count(*) FILTER (WHERE method='opencollective')::int AS oc_count
FROM tips WHERE created_at >= @since;

-- name: GetUserDisplayNames :many
SELECT u.id, u.username, up.display_name
FROM users u LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE u.id = ANY(@ids::uuid[]);

-- name: GetOrgDisplayNames :many
SELECT o.id, o.name FROM organizations o WHERE o.id = ANY(@ids::uuid[]);

-- name: IsMentorProfile :one
SELECT COALESCE(is_mentor, false) FROM user_profiles WHERE user_id = @user_id;

-- name: FailTip :exec
-- Marks a pending tip failed and releases the idempotency key so the same
-- Idempotency-Key can retry a fresh attempt (the unique partial index
-- would otherwise trap the key on the dead row).
UPDATE tips SET status = 'failed', idempotency_key = NULL, updated_at = now()
WHERE id = @id AND status = 'pending';

-- name: GetContentTitle :one
-- Title lookup for tip.received notifications on content-anchored tips.
SELECT title FROM contents WHERE id = @id;

-- name: SumRecipientPointsToday :one
-- Recipient-side mirror of the sender cap — co-conspirator pairs can't
-- launder unlimited points through one wallet per UTC day (0=unlimited).
SELECT COALESCE(sum(points_amount),0)::bigint FROM tips
WHERE recipient_type = 'user' AND recipient_user_id = @recipient_user_id
  AND method = 'points' AND status = 'completed'
  AND created_at >= (date_trunc('day', timezone('UTC', now()))) AT TIME ZONE 'UTC';
