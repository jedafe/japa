-- Wallet & Credits System (Section 28)
-- Two-tier internal currency: Points (soft, activity-earned) and Credits (spendable, monetary-equivalent).

-- =====================
-- Wallet Ledger
-- =====================

-- name: InsertLedgerEntry :one
INSERT INTO wallet_ledger (
    tenant_id, user_id, ledger_type, operation, amount, resulting_balance,
    source, actor_user_id, reference, idempotency_key, reason
) VALUES (
    @tenant_id, @user_id, @ledger_type, @operation, @amount, @resulting_balance,
    sqlc.narg(source), sqlc.narg(actor_user_id), sqlc.narg(reference),
    sqlc.narg(idempotency_key), sqlc.narg(reason)
)
RETURNING *;

-- name: ListLedgerEntriesByUser :many
SELECT * FROM wallet_ledger
WHERE tenant_id = @tenant_id AND user_id = @user_id
  AND (@ledger_type::text IS NULL OR ledger_type = @ledger_type)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetLatestBalance :one
SELECT resulting_balance FROM wallet_ledger
WHERE tenant_id = @tenant_id AND user_id = @user_id AND ledger_type = @ledger_type
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CheckIdempotencyKey :one
SELECT EXISTS(
    SELECT 1 FROM wallet_ledger
    WHERE idempotency_key = @idempotency_key
);

-- name: GetLedgerEntryByID :one
SELECT * FROM wallet_ledger
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Activity Point Rules
-- =====================

-- name: UpsertActivityRule :one
INSERT INTO activity_point_rules (
    tenant_id, activity_key, point_value, is_enabled, cap_per_day, cap_per_week, description
) VALUES (
    @tenant_id, @activity_key, @point_value, @is_enabled,
    sqlc.narg(cap_per_day), sqlc.narg(cap_per_week), sqlc.narg(description)
)
ON CONFLICT (tenant_id, activity_key) DO UPDATE SET
    point_value = EXCLUDED.point_value,
    is_enabled = EXCLUDED.is_enabled,
    cap_per_day = EXCLUDED.cap_per_day,
    cap_per_week = EXCLUDED.cap_per_week,
    description = EXCLUDED.description,
    updated_at = now()
RETURNING *;

-- name: GetActivityRuleByKey :one
SELECT * FROM activity_point_rules
WHERE tenant_id = @tenant_id AND activity_key = @activity_key;

-- name: ListActivityRulesByTenant :many
SELECT * FROM activity_point_rules
WHERE tenant_id = @tenant_id
ORDER BY activity_key ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ToggleActivityRule :one
UPDATE activity_point_rules
SET is_enabled = NOT is_enabled, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: DeleteActivityRule :exec
DELETE FROM activity_point_rules
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Activity Events
-- =====================

-- name: InsertActivityEvent :one
INSERT INTO activity_events (
    tenant_id, user_id, activity_key, reference, payload
) VALUES (
    @tenant_id, @user_id, @activity_key, @reference, @payload
)
ON CONFLICT (tenant_id, activity_key, reference) DO NOTHING
RETURNING *;

-- name: GetActivityEventByID :one
SELECT * FROM activity_events
WHERE id = @id AND tenant_id = @tenant_id;

-- name: MarkEventProcessed :exec
UPDATE activity_events
SET processed = true, processed_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListUnprocessedEvents :many
SELECT * FROM activity_events
WHERE processed = false
ORDER BY created_at ASC
LIMIT @page_limit;

-- name: CountUserActivityToday :one
SELECT COUNT(*) FROM activity_events
WHERE tenant_id = @tenant_id AND user_id = @user_id AND activity_key = @activity_key
  AND created_at >= date_trunc('day', now());

-- name: CountUserActivityThisWeek :one
SELECT COUNT(*) FROM activity_events
WHERE tenant_id = @tenant_id AND user_id = @user_id AND activity_key = @activity_key
  AND created_at >= date_trunc('week', now());

-- =====================
-- Wallet Settings
-- =====================

-- name: GetWalletSettings :one
SELECT * FROM wallet_settings
WHERE tenant_id = @tenant_id;

-- name: UpsertWalletSettings :one
INSERT INTO wallet_settings (tenant_id, points_to_credits_rate, daily_earning_cap, enable_earning)
VALUES (@tenant_id, @points_to_credits_rate, @daily_earning_cap, @enable_earning)
ON CONFLICT (tenant_id) DO UPDATE SET
    points_to_credits_rate = EXCLUDED.points_to_credits_rate,
    daily_earning_cap = EXCLUDED.daily_earning_cap,
    enable_earning = EXCLUDED.enable_earning,
    updated_at = now()
RETURNING *;