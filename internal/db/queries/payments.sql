-- Payment Core — payments, subscriptions, discount codes, refunds.

-- =====================
-- Payments CRUD
-- =====================

-- name: CreatePayment :one
INSERT INTO payments (user_id, provider, provider_ref, purpose_type, purpose_id, amount, currency, status, metadata, subscription_id, discount_code_id, discount_amount, idempotency_key) VALUES (@user_id, @provider, sqlc.narg(provider_ref), @purpose_type, sqlc.narg(purpose_id), @amount, @currency, @status, @metadata, sqlc.narg(subscription_id), sqlc.narg(discount_code_id), @discount_amount, sqlc.narg(idempotency_key))
RETURNING *;

-- name: GetPaymentByID :one
SELECT * FROM payments
WHERE id = @id;

-- name: GetPaymentByProviderRef :one
-- Direct lookup by provider reference (Stripe pi_.../cs_..., Paystack
-- reference, NOWPayments order_id). Uses idx_payments_provider_ref.
-- Replaces the full-scan ListPaymentsByStatus + iterate match in the
-- webhook worker (BUG-019).
SELECT * FROM payments
WHERE provider_ref = @provider_ref
ORDER BY created_at DESC
LIMIT 1;

-- name: ListPaymentsByUser :many
SELECT * FROM payments
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPaymentsByStatus :many
SELECT * FROM payments
WHERE status = @status
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListAdminPayments :many
-- Admin cross-platform payment list with optional filters (JEOS §17
-- "Admin Payment Routes"). Nullable parameters are skipped via sqlc.narg.
SELECT * FROM payments
WHERE (sqlc.narg(user_id)::uuid IS NULL OR user_id = sqlc.narg(user_id))
 AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
 AND (sqlc.narg(purpose_type)::text IS NULL OR purpose_type = sqlc.narg(purpose_type))
 AND (sqlc.narg(provider)::text IS NULL OR provider = sqlc.narg(provider))
 AND (sqlc.narg(start_date)::timestamptz IS NULL OR created_at >= sqlc.narg(start_date))
 AND (sqlc.narg(end_date)::timestamptz IS NULL OR created_at <= sqlc.narg(end_date))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetAdminPaymentByID :one
-- Admin payment detail (JEOS §17). Same as GetPaymentByID but used by
-- the admin route to signal admin context for audit logging.
SELECT * FROM payments
WHERE id = @id;

-- name: UpdatePaymentStatus :one
UPDATE payments
SET status = @status, updated_at = now()
WHERE id = @id
RETURNING *;

-- =====================
-- Payment Webhook Events
-- =====================

-- name: InsertWebhookEvent :one
INSERT INTO payment_webhook_events (provider, event_id, event_type, raw_payload) VALUES (@provider, @event_id, @event_type, @raw_payload)
ON CONFLICT (provider, event_id) DO NOTHING
RETURNING *;

-- name: IsWebhookEventProcessed :one
SELECT EXISTS(
 SELECT 1 FROM payment_webhook_events
 WHERE provider = @provider
 AND event_id = @event_id
 AND processed_at IS NOT NULL
);

-- name: MarkWebhookEventProcessed :exec
UPDATE payment_webhook_events
SET processed_at = now()
WHERE provider = @provider
 AND event_id = @event_id;

-- =====================
-- Subscriptions
-- =====================

-- name: CreateSubscription :one
INSERT INTO subscriptions (user_id, plan, provider, provider_ref, status, current_period_start, current_period_end, cancel_at) VALUES (@user_id, @plan, @provider, sqlc.narg(provider_ref), @status, @current_period_start, @current_period_end, sqlc.narg(cancel_at))
RETURNING *;

-- name: GetSubscriptionByID :one
SELECT * FROM subscriptions
WHERE id = @id;

-- name: GetActiveSubscriptionByUser :one
SELECT * FROM subscriptions
WHERE user_id = @user_id
 AND status = 'active'
 AND current_period_end > now()
ORDER BY created_at DESC
LIMIT 1;

-- name: ListSubscriptionsByUser :many
SELECT * FROM subscriptions
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveSubscriptions :many
SELECT * FROM subscriptions
WHERE status = 'active'
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateSubscriptionStatus :one
UPDATE subscriptions
SET status = @status, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: CancelSubscription :one
-- JEOS §17: POST /me/subscription/cancel cancels at period end — the user
-- keeps access until current_period_end, then the subscription ends. We set
-- cancel_at_period_end = true and cancelled_at = now() but leave status
-- 'active' so entitlements remain until the period ends. A scheduled job
-- transitions to 'canceled' at current_period_end (BUG-002).
UPDATE subscriptions
SET cancel_at_period_end = true, cancelled_at = now(), updated_at = now()
WHERE id = @id
 AND status = 'active'
RETURNING *;

-- name: SoftDeleteSubscription :exec
UPDATE subscriptions
SET status = 'canceled', cancel_at = now(), cancel_at_period_end = true, cancelled_at = now(), updated_at = now()
WHERE id = @id;

-- name: ReactivateSubscription :one
-- Reactivates the caller's most recently canceled or scheduled-for-cancellation
-- subscription. Matches both 'canceled' rows and 'active' rows with
-- cancel_at_period_end = true so a user can undo a period-end cancel before
-- the period ends (BUG-002).
UPDATE subscriptions
SET status = 'active', cancel_at = NULL, cancel_at_period_end = false, cancelled_at = NULL, updated_at = now()
WHERE id = (
 SELECT s.id FROM subscriptions s
 WHERE s.user_id = @user_id
 AND (s.status = 'canceled' OR (s.status = 'active' AND s.cancel_at_period_end = true))
 ORDER BY s.updated_at DESC
 LIMIT 1
)
RETURNING *;

-- =====================
-- Discount Codes
-- =====================

-- name: CreateDiscountCode :one
INSERT INTO discount_codes (code, discount_type, discount_value, applicable_to, max_uses, uses_per_user, group_id, valid_from, valid_until, created_by) VALUES (@code, @discount_type, @discount_value, @applicable_to, sqlc.narg(max_uses), @uses_per_user, sqlc.narg(group_id), @valid_from, sqlc.narg(valid_until), sqlc.narg(created_by))
RETURNING *;

-- name: GetDiscountCodeByCode :one
SELECT * FROM discount_codes
WHERE code = @code;

-- name: GetDiscountCodeByID :one
SELECT * FROM discount_codes
WHERE id = @id;

-- name: ListDiscountCodes :many
SELECT * FROM discount_codes

ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: DeleteDiscountCode :exec
DELETE FROM discount_codes
WHERE id = @id;

-- name: UpdateDiscountCode :one
UPDATE discount_codes
SET
 code = COALESCE(NULLIF(@new_code, ''), code),
 discount_type = COALESCE(NULLIF(@discount_type, ''), discount_type),
 discount_value = COALESCE(sqlc.narg(discount_value), discount_value),
 applicable_to = COALESCE(NULLIF(@applicable_to, ''), applicable_to),
 max_uses = sqlc.narg(max_uses),
 valid_from = COALESCE(sqlc.narg(valid_from), valid_from),
 valid_until = sqlc.narg(valid_until)
WHERE id = @id
RETURNING *;

-- =====================
-- Discount Code Uses
-- =====================

-- name: InsertDiscountCodeUse :one
INSERT INTO discount_code_uses (code_id, user_id, payment_id) VALUES (@code_id, @user_id, sqlc.narg(payment_id))
ON CONFLICT (code_id, user_id) DO NOTHING
RETURNING *;

-- name: CountDiscountCodeUsesByUser :one
SELECT COUNT(*)::int FROM discount_code_uses
WHERE code_id = @code_id AND user_id = @user_id;

-- name: CountDiscountCodeUses :one
-- Total uses of a code across all users (for max_uses enforcement).
-- Replaces the list+len pattern that fetched up to 1000 rows just to
-- count them and silently under-counted beyond 1000 (BUG-018).
SELECT COUNT(*)::int FROM discount_code_uses
WHERE code_id = @code_id;

-- name: ListDiscountCodeUses :many
SELECT * FROM discount_code_uses
WHERE code_id = @code_id
ORDER BY used_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Payment Refunds
-- =====================

-- name: CreateRefund :one
INSERT INTO payment_refunds (payment_id, amount, reason, status, provider_ref) VALUES (@payment_id, @amount, sqlc.narg(reason), @status, sqlc.narg(provider_ref))
RETURNING *;

-- name: ListRefundsByPayment :many
SELECT * FROM payment_refunds
WHERE payment_id = @payment_id
ORDER BY created_at DESC;