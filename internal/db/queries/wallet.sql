-- Wallet & Credits System (Section 28)
-- Two-tier internal currency: Points (soft, activity-earned) and Credits (spendable, monetary-equivalent).

-- =====================
-- Wallet Ledger
-- =====================

-- name: InsertLedgerEntry :one
INSERT INTO wallet_ledger (user_id, ledger_type, operation, amount, resulting_balance, source, actor_user_id, reference, idempotency_key, reason) VALUES (@user_id, @ledger_type, @operation, @amount, @resulting_balance, sqlc.narg(source), sqlc.narg(actor_user_id), sqlc.narg(reference), sqlc.narg(idempotency_key), sqlc.narg(reason))
RETURNING *;

-- name: GetLatestBalance :one
SELECT resulting_balance FROM wallet_ledger
WHERE user_id = @user_id AND ledger_type = @ledger_type
ORDER BY seq DESC
LIMIT 1;

-- name: GetLatestCreditBalance :one
SELECT resulting_balance FROM credits_ledger
WHERE user_id = @user_id
ORDER BY seq DESC
LIMIT 1;

-- name: InsertCreditLedgerEntry :one
INSERT INTO credits_ledger (user_id, operation, amount, resulting_balance, source, actor_user_id, reference, idempotency_key, reason, payment_id, reverses_id) VALUES (@user_id, @operation, @amount, @resulting_balance, sqlc.narg(source), sqlc.narg(actor_user_id), sqlc.narg(reference), sqlc.narg(idempotency_key), sqlc.narg(reason), sqlc.narg(payment_id), sqlc.narg(reverses_id))
RETURNING *;

-- name: ListMergedLedgerEntriesByUser :many
-- Unified ledger listing for /me/wallet/ledger and the admin user ledger —
-- the response shape matches the old shared-table contract (split plan
-- step 6). Historical credits rows still mirrored in wallet_ledger are
-- excluded once their copy exists in credits_ledger (id match), while
-- unreconciled stragglers from a stale binary still surface.
SELECT w.id, w.user_id, w.ledger_type, w.operation, w.amount, w.resulting_balance, w.source, w.actor_user_id, w.reference, w.idempotency_key, w.reason, w.created_at
FROM wallet_ledger w
WHERE w.user_id = @user_id
 AND (w.ledger_type = 'points'
      OR NOT EXISTS (SELECT 1 FROM credits_ledger m WHERE m.id = w.id))
 AND (@ledger_type = '' OR w.ledger_type = @ledger_type)
UNION ALL
SELECT c.id, c.user_id, 'credits'::text AS ledger_type, c.operation, c.amount, c.resulting_balance, c.source, c.actor_user_id, c.reference, c.idempotency_key, c.reason, c.created_at
FROM credits_ledger c
WHERE c.user_id = @user_id
 AND (@ledger_type = '' OR 'credits' = @ledger_type)
ORDER BY created_at DESC, id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetMergedLedgerEntryByID :one
-- Finds a ledger row across both streams (admin tooling / error
-- semantics). Mirrored wallet_ledger credits rows are hidden once their
-- credits_ledger copy exists, so ids never resolve twice.
SELECT w.id, w.user_id, w.ledger_type, w.operation, w.amount, w.resulting_balance, w.source, w.actor_user_id, w.reference, w.idempotency_key, w.reason, w.created_at
FROM wallet_ledger w
WHERE w.id = @id
 AND (w.ledger_type = 'points'
      OR NOT EXISTS (SELECT 1 FROM credits_ledger m WHERE m.id = w.id))
UNION ALL
SELECT c.id, c.user_id, 'credits'::text AS ledger_type, c.operation, c.amount, c.resulting_balance, c.source, c.actor_user_id, c.reference, c.idempotency_key, c.reason, c.created_at
FROM credits_ledger c
WHERE c.id = @id
LIMIT 1;

-- name: CheckAnyLedgerIdempotencyKey :one
-- Post-split the unique key scope is per-table; this preserves the old
-- global-uniqueness check across both streams.
SELECT EXISTS(
 SELECT 1 FROM wallet_ledger w WHERE w.idempotency_key = @idempotency_key
 UNION ALL
 SELECT 1 FROM credits_ledger c WHERE c.idempotency_key = @idempotency_key
);

-- name: CountUserCreditLedgerBySource :one
SELECT COUNT(*) FROM credits_ledger
WHERE user_id = @user_id
 AND source = @source;

-- name: GetCreditLedgerEntryByReference :one
SELECT * FROM credits_ledger
WHERE user_id = @user_id AND reference = @reference
ORDER BY seq DESC LIMIT 1;

-- name: GetCreditLedgerEntryByID :one
-- Full credits_ledger row including payment_id/reverses_id — needed by the
-- refund path (split plan: credits reversal design).
SELECT * FROM credits_ledger
WHERE id = @id;

-- name: GetCreditRefundedTotal :one
-- Total already refunded against an original credits entry (refund linkage
-- enforcement: partial refunds may not exceed the original magnitude).
SELECT COALESCE(SUM(ABS(amount)), 0)::bigint FROM credits_ledger
WHERE reverses_id = @reverses_id;

-- name: GetLifetimePoints :one
-- Sum of all positive points earns (never decreases; used for badges/tiers,
-- Section 28). BUG-W-003: surfaced in GET /me/wallet response.
SELECT COALESCE(SUM(amount), 0)::bigint FROM wallet_ledger
WHERE user_id = @user_id
 AND ledger_type = 'points' AND operation = 'earn' AND amount > 0;

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
-- Dedupe scope is (user_id, activity_key, reference) per Section 28 —
-- entity-scoped references are correct per-user, so shared entities
-- (polls, wiki pages, jobs) no longer single-winner (PTS-015, migration 001021).
INSERT INTO activity_events (user_id, activity_key, reference, payload) VALUES (@user_id, @activity_key, @reference, @payload)
ON CONFLICT (user_id, activity_key, reference) DO NOTHING
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
ORDER BY reference DESC LIMIT 1;

-- name: GetLedgerEntryByReference :one
-- The wallet entry written for a specific activity reference (ADR-136).
SELECT * FROM wallet_ledger
WHERE user_id = @user_id AND ledger_type = @ledger_type AND reference = @reference
ORDER BY created_at DESC LIMIT 1;

-- name: MarkEventProcessed :exec
UPDATE activity_events
SET processed = true, processed_at = now(), outcome = sqlc.narg(outcome)
WHERE id = @id;

-- name: ListUnprocessedEvents :many
SELECT * FROM activity_events
WHERE processed = false
ORDER BY created_at ASC
LIMIT @page_limit;

-- name: ListUserActivityEvents :many
-- Member-facing activity history — includes the settle outcome so "did X
-- but earned nothing" rows are explainable (capped, floored, no_rule, …).
SELECT id, user_id, activity_key, reference, processed, outcome, created_at
FROM activity_events
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

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
-- operation='earn' matches GetLifetimePoints — admin_adjust floor
-- corrections and reversals are balance repairs, not earned points.
-- Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
SELECT COALESCE(SUM(amount), 0)::bigint AS points
FROM wallet_ledger
WHERE user_id = $1
 AND ledger_type = 'points'
 AND operation = 'earn'
 AND amount > 0
 AND ($2::timestamptz IS NULL OR created_at >= $2)
 AND ($3::timestamptz IS NULL OR created_at <= $3);
-- name: InsertWalletAuditLog :exec
-- Canonical admin-action trail for monetary wallet mutations.
INSERT INTO audit_logs (actor_user_id, action_type, target_type, target_id, metadata)
VALUES (@actor_user_id, @action_type, @target_type, @target_id, @metadata::jsonb);

-- name: GetCreditsLedgerSum :one
-- Chain-integrity probe (SEC-W-003): SUM(amount) must equal the latest
-- resulting_balance. Divergence means a row was inserted with a forged
-- balance snapshot.
SELECT COALESCE(SUM(amount), 0)::bigint AS amount_sum
FROM credits_ledger
WHERE user_id = @user_id;

-- name: CountUnbackedPurchaseCredits :one
-- Every purchase entry must be backed by a succeeded §17 payment
-- (SEC-W-003). Rows lacking payment linkage (or whose payment did not
-- succeed) are unbacked credit issuance.
SELECT COUNT(*)::int AS row_count, COALESCE(SUM(cl.amount), 0)::bigint AS credit_total
FROM credits_ledger cl
LEFT JOIN payments p ON p.id = cl.payment_id AND p.user_id = cl.user_id
 AND p.status IN ('succeeded', 'refunded')
WHERE cl.user_id = @user_id
 AND cl.operation = 'purchase'
 AND p.id IS NULL;

-- name: CountUnbackedTipEarns :one
-- Tip-sourced earns must reference a completed tips row with matching net
-- credits (SEC-W-005 tip-laundering guard).
SELECT COUNT(*)::int AS row_count, COALESCE(SUM(cl.amount), 0)::bigint AS credit_total
FROM credits_ledger cl
WHERE cl.user_id = @user_id
 AND cl.operation = 'earn'
 AND cl.source = 'tip'
 AND NOT EXISTS (
   SELECT 1 FROM tips t
   WHERE t.id::text = cl.reference
     AND t.status IN ('completed', 'refunded')
     AND t.net_credits = cl.amount
 );

-- name: CountSourcelessCreditGrants :one
-- Legitimate positive entries always carry a source (tip, credit_purchase,
-- admin_*, award reasons). A positive row without one is suspicious
-- (SEC-W-003).
SELECT COUNT(*)::int AS row_count, COALESCE(SUM(cl.amount), 0)::bigint AS credit_total
FROM credits_ledger cl
WHERE cl.user_id = @user_id
 AND cl.amount > 0
 AND cl.source IS NULL;

-- name: CountDivergentCreditsLedgers :one
-- Global chain-integrity sweep (SEC-W-012 solvency reconcile): counts users
-- whose SUM(amount) diverges from their latest resulting_balance.
-- Single-pass over the table — no per-user correlated lookups.
WITH sums AS (
  SELECT user_id, COALESCE(SUM(amount), 0)::bigint AS s
  FROM credits_ledger
  GROUP BY user_id
), tails AS (
  SELECT DISTINCT ON (user_id) user_id, resulting_balance
  FROM credits_ledger
  ORDER BY user_id, seq DESC
)
SELECT COUNT(*)::int
FROM sums
JOIN tails USING (user_id)
WHERE sums.s <> tails.resulting_balance;

-- name: GetUserCreditEarnsSince :one
-- Rolling-window total of positive credits earns (SEC-W-014 earn cap).
SELECT COALESCE(SUM(amount), 0)::bigint
FROM credits_ledger
WHERE user_id = @user_id
 AND operation = 'earn'
 AND amount > 0
 AND created_at >= @since;

-- name: CountAllUnbackedPurchaseCredits :one
-- Global purchase-backing sweep (SEC-W-012): purchase entries platform-wide
-- lacking a succeeded payment.
SELECT COUNT(*)::int AS row_count, COALESCE(SUM(cl.amount), 0)::bigint AS credit_total
FROM credits_ledger cl
LEFT JOIN payments p ON p.id = cl.payment_id AND p.user_id = cl.user_id
 AND p.status IN ('succeeded', 'refunded')
WHERE cl.operation = 'purchase'
 AND p.id IS NULL;

-- name: CountAnonymousAdminAdjusts :one
-- Admin adjustments via the app always carry actor_user_id. A positive
-- admin_adjust with NULL actor was written outside the service path
-- (SEC-W-011 — forged mint detection).
SELECT COUNT(*)::int AS row_count, COALESCE(SUM(cl.amount), 0)::bigint AS credit_total
FROM credits_ledger cl
WHERE cl.user_id = @user_id
 AND cl.operation = 'admin_adjust'
 AND cl.amount > 0
 AND cl.actor_user_id IS NULL;

-- name: CountAllAnonymousAdminAdjusts :one
-- Global form of CountAnonymousAdminAdjusts for the solvency sweep.
SELECT COUNT(*)::int AS row_count, COALESCE(SUM(cl.amount), 0)::bigint AS credit_total
FROM credits_ledger cl
WHERE cl.operation = 'admin_adjust'
 AND cl.amount > 0
 AND cl.actor_user_id IS NULL;
