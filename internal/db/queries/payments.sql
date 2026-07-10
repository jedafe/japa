-- Payment Core — payments, subscriptions, discount codes, refunds.

-- =====================
-- Payments CRUD
-- =====================

-- name: CreatePayment :one
INSERT INTO payments (
    tenant_id, user_id, provider, provider_ref, purpose_type, purpose_id,
    amount, currency, status, metadata, subscription_id,
    discount_code_id, discount_amount
) VALUES (
    @tenant_id, @user_id, @provider, sqlc.narg(provider_ref), @purpose_type,
    sqlc.narg(purpose_id), @amount, @currency, @status, @metadata,
    sqlc.narg(subscription_id), sqlc.narg(discount_code_id), @discount_amount
)
RETURNING *;

-- name: GetPaymentByID :one
SELECT * FROM payments
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListPaymentsByUser :many
SELECT * FROM payments
WHERE tenant_id = @tenant_id AND user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPaymentsByStatus :many
SELECT * FROM payments
WHERE tenant_id = @tenant_id AND status = @status
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdatePaymentStatus :one
UPDATE payments
SET status = @status, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- =====================
-- Payment Webhook Events
-- =====================

-- name: InsertWebhookEvent :one
INSERT INTO payment_webhook_events (
    tenant_id, provider, event_id, raw_payload
) VALUES (
    @tenant_id, @provider, @event_id, @raw_payload
)
ON CONFLICT (tenant_id, provider, event_id) DO NOTHING
RETURNING *;

-- name: IsWebhookEventProcessed :one
SELECT EXISTS(
    SELECT 1 FROM payment_webhook_events
    WHERE tenant_id = @tenant_id
      AND provider = @provider
      AND event_id = @event_id
      AND processed_at IS NOT NULL
);

-- name: MarkWebhookEventProcessed :exec
UPDATE payment_webhook_events
SET processed_at = now()
WHERE tenant_id = @tenant_id
  AND provider = @provider
  AND event_id = @event_id;

-- =====================
-- Subscriptions
-- =====================

-- name: CreateSubscription :one
INSERT INTO subscriptions (
    tenant_id, user_id, plan, provider, provider_ref,
    status, current_period_start, current_period_end, cancel_at
) VALUES (
    @tenant_id, @user_id, @plan, @provider, sqlc.narg(provider_ref),
    @status, @current_period_start, @current_period_end, sqlc.narg(cancel_at)
)
RETURNING *;

-- name: GetSubscriptionByID :one
SELECT * FROM subscriptions
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetActiveSubscriptionByUser :one
SELECT * FROM subscriptions
WHERE tenant_id = @tenant_id
  AND user_id = @user_id
  AND status = 'active'
  AND current_period_end > now()
ORDER BY created_at DESC
LIMIT 1;

-- name: ListSubscriptionsByUser :many
SELECT * FROM subscriptions
WHERE tenant_id = @tenant_id AND user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveSubscriptions :many
SELECT * FROM subscriptions
WHERE tenant_id = @tenant_id AND status = 'active'
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateSubscriptionStatus :one
UPDATE subscriptions
SET status = @status, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: CancelSubscription :one
UPDATE subscriptions
SET status = 'canceled', cancel_at = now(), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
  AND status = 'active'
RETURNING *;

-- name: SoftDeleteSubscription :exec
UPDATE subscriptions
SET status = 'cancelled', cancelled_at = now(), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ReactivateSubscription :one
UPDATE subscriptions
SET status = 'active', cancelled_at = NULL, updated_at = now()
WHERE user_id = @user_id AND tenant_id = @tenant_id AND status = 'cancelled'
ORDER BY updated_at DESC
LIMIT 1
RETURNING *;

-- =====================
-- Discount Codes
-- =====================

-- name: CreateDiscountCode :one
INSERT INTO discount_codes (
    tenant_id, code, discount_type, discount_value, applicable_to,
    max_uses, uses_per_user, group_id, valid_from, valid_until, created_by
) VALUES (
    @tenant_id, @code, @discount_type, @discount_value, @applicable_to,
    sqlc.narg(max_uses), @uses_per_user, sqlc.narg(group_id),
    @valid_from, sqlc.narg(valid_until), sqlc.narg(created_by)
)
RETURNING *;

-- name: GetDiscountCodeByCode :one
SELECT * FROM discount_codes
WHERE tenant_id = @tenant_id AND code = @code;

-- name: GetDiscountCodeByID :one
SELECT * FROM discount_codes
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListDiscountCodes :many
SELECT * FROM discount_codes
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: DeleteDiscountCode :exec
DELETE FROM discount_codes
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateDiscountCode :one
UPDATE discount_codes
SET code = COALESCE(NULLIF(@new_code, ''), code), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, code, updated_at;

-- =====================
-- Discount Code Uses
-- =====================

-- name: InsertDiscountCodeUse :one
INSERT INTO discount_code_uses (
    code_id, user_id, payment_id
) VALUES (
    @code_id, @user_id, sqlc.narg(payment_id)
)
ON CONFLICT (code_id, user_id) DO NOTHING
RETURNING *;

-- name: CountDiscountCodeUsesByUser :one
SELECT COUNT(*)::int FROM discount_code_uses
WHERE code_id = @code_id AND user_id = @user_id;

-- name: ListDiscountCodeUses :many
SELECT * FROM discount_code_uses
WHERE code_id = @code_id
ORDER BY used_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Payment Refunds
-- =====================

-- name: CreateRefund :one
INSERT INTO payment_refunds (
    payment_id, amount, reason, status, provider_ref
) VALUES (
    @payment_id, @amount, sqlc.narg(reason), @status, sqlc.narg(provider_ref)
)
RETURNING *;

-- name: ListRefundsByPayment :many
SELECT * FROM payment_refunds
WHERE payment_id = @payment_id
ORDER BY created_at DESC;