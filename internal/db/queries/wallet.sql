-- Wallet & Credits System (Section 28)
-- Two-tier internal currency: Points (soft, activity-earned) and Credits (spendable, monetary-equivalent).

-- =====================
-- Wallet Ledger
-- =====================

-- name: InsertLedgerEntry :one
INSERT INTO wallet_ledger (user_id, ledger_type, operation, amount, resulting_balance, source, actor_user_id, reference, idempotency_key, reason) VALUES (@user_id, @ledger_type, @operation, @amount, @resulting_balance, sqlc.narg(source), sqlc.narg(actor_user_id), sqlc.narg(reference), sqlc.narg(idempotency_key), sqlc.narg(reason))
RETURNING *;

-- name: ListLedgerEntriesByUser :many
SELECT * FROM wallet_ledger
WHERE user_id = @user_id
 AND (@ledger_type::text IS NULL OR ledger_type = @ledger_type)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetLatestBalance :one
SELECT resulting_balance FROM wallet_ledger
WHERE user_id = @user_id AND ledger_type = @ledger_type
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetLifetimePoints :one
-- Sum of all positive points earns (never decreases; used for badges/tiers,
-- Section 28). BUG-W-003: surfaced in GET /me/wallet response.
SELECT COALESCE(SUM(amount), 0)::bigint FROM wallet_ledger
WHERE user_id = @user_id
 AND ledger_type = 'points' AND operation = 'earn' AND amount > 0;

-- name: CheckIdempotencyKey :one
SELECT EXISTS(
 SELECT 1 FROM wallet_ledger
 WHERE idempotency_key = @idempotency_key
);

-- name: GetLedgerEntryByID :one
SELECT * FROM wallet_ledger
WHERE id = @id;

-- =====================
-- Activity Point Rules
-- =====================

-- name: CountUserLedgerBySource :one
SELECT COUNT(*) FROM wallet_ledger
WHERE user_id = @user_id
 AND source = @source;

-- name: UpsertActivityRule :one
INSERT INTO activity_point_rules (activity_key, point_value, is_enabled, one_time, cap_per_day, cap_per_week, description) VALUES (@activity_key, @point_value, @is_enabled, @one_time, sqlc.narg(cap_per_day), sqlc.narg(cap_per_week), sqlc.narg(description))
ON CONFLICT (activity_key) DO UPDATE SET
 point_value = EXCLUDED.point_value,
 is_enabled = EXCLUDED.is_enabled,
 one_time = EXCLUDED.one_time,
 cap_per_day = EXCLUDED.cap_per_day,
 cap_per_week = EXCLUDED.cap_per_week,
 description = EXCLUDED.description,
 updated_at = now()
RETURNING *;

-- name: GetActivityRuleByKey :one
SELECT * FROM activity_point_rules
WHERE activity_key = @activity_key;

-- name: GetActivityRuleByID :one
SELECT * FROM activity_point_rules
WHERE id = @id;

-- name: UpdateActivityRuleByID :one
UPDATE activity_point_rules
SET point_value = @point_value,
 is_enabled = @is_enabled,
 one_time = @one_time,
 cap_per_day = sqlc.narg(cap_per_day),
 cap_per_week = sqlc.narg(cap_per_week),
 description = sqlc.narg(description),
 updated_at = now()
WHERE id = @id
RETURNING *;

-- name: ListActivityRules :many
SELECT * FROM activity_point_rules

ORDER BY activity_key ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ToggleActivityRule :one
UPDATE activity_point_rules
SET is_enabled = NOT is_enabled, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteActivityRule :exec
DELETE FROM activity_point_rules
WHERE id = @id;

-- =====================
-- Activity Events
-- =====================

-- name: InsertActivityEvent :one
INSERT INTO activity_events (user_id, activity_key, reference, payload) VALUES (@user_id, @activity_key, @reference, @payload)
ON CONFLICT (activity_key, reference) DO NOTHING
RETURNING *;

-- name: GetActivityEventByID :one
SELECT * FROM activity_events
WHERE id = @id;

-- name: GetActivityEventByReference :one
SELECT * FROM activity_events
WHERE activity_key = @activity_key AND reference = @reference;

-- name: GetLatestActivityEventReferenceByPrefixAndKey :one
-- The most recent event reference for a key under a reference prefix —
-- used by vote emitters to link a revert to the award it undoes (ADR-136).
SELECT reference FROM activity_events
WHERE activity_key = @activity_key AND reference LIKE @prefix
ORDER BY created_at DESC LIMIT 1;

-- name: GetLedgerEntryByReference :one
-- The wallet entry written for a specific activity reference (ADR-136).
SELECT * FROM wallet_ledger
WHERE user_id = @user_id AND ledger_type = @ledger_type AND reference = @reference
ORDER BY created_at DESC LIMIT 1;

-- name: MarkEventProcessed :exec
UPDATE activity_events
SET processed = true, processed_at = now()
WHERE id = @id;

-- name: ListUnprocessedEvents :many
SELECT * FROM activity_events
WHERE processed = false
ORDER BY created_at ASC
LIMIT @page_limit;

-- name: CountUserActivityToday :one
SELECT COUNT(*) FROM activity_events
WHERE user_id = @user_id AND activity_key = @activity_key
 AND created_at >= date_trunc('day', now());

-- name: ListActivityEventsUndoing :many
-- Unprocessed events whose undoes payload references the given award
-- event — dependents to run after that award completes (ADR-136).
SELECT * FROM activity_events
WHERE payload->>'undoes' = @undoes::text AND payload->>'undoes_key' = @activity_key::text
  AND processed IS NOT TRUE;

-- name: CountUserActivityThisWeek :one
SELECT COUNT(*) FROM activity_events
WHERE user_id = @user_id AND activity_key = @activity_key
 AND created_at >= date_trunc('week', now());

-- =====================
-- Wallet Settings
-- =====================

-- name: GetWalletSettings :one
SELECT * FROM wallet_settings
;

-- name: UpsertWalletSettings :one
WITH existing AS (SELECT id FROM wallet_settings LIMIT 1)
INSERT INTO wallet_settings (id, daily_earning_cap, enable_earning)
SELECT COALESCE((SELECT id FROM existing), gen_random_uuid()),
       @daily_earning_cap, @enable_earning
ON CONFLICT (id) DO UPDATE SET
 daily_earning_cap = EXCLUDED.daily_earning_cap,
 enable_earning = EXCLUDED.enable_earning,
 updated_at = now()
RETURNING *;

-- name: CountUserPointsEarned :one
-- Total positive points earned by the user within a date range.
-- Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
SELECT COALESCE(SUM(amount), 0)::bigint AS points
FROM wallet_ledger
WHERE user_id = $1
 AND ledger_type = 'points'
 AND amount > 0
 AND ($2::timestamptz IS NULL OR created_at >= $2)
 AND ($3::timestamptz IS NULL OR created_at <= $3);