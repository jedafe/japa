-- Wallet Withdrawal queries (Section 58 credit withdrawal amendment).

-- name: GetUserGroupByUserID :one
SELECT ug.id, ug.name, ug.slug, ug.is_system,
 ug.auto_withdrawal_limit, ug.requires_withdrawal_approval,
 ug.created_at, ug.updated_at
FROM user_groups ug
JOIN user_group_memberships ugm ON ugm.group_id = ug.id
WHERE ugm.user_id = @user_id
LIMIT 1;

-- name: GetUserWithdrawalPolicy :one
-- Effective withdrawal policy across ALL of a user's group memberships
-- (SEC-W-016): the old LIMIT-1 lookup let an arbitrary group win. Fold
-- to the most restrictive policy — approval required if ANY membership
-- demands it; auto-approve limit is the minimum positive limit, and 0
-- if any membership has no positive limit.
SELECT COALESCE(bool_or(ug.requires_withdrawal_approval), false)::boolean AS requires_approval,
 CASE WHEN bool_or(ug.auto_withdrawal_limit IS NULL OR ug.auto_withdrawal_limit <= 0)
   THEN 0::bigint
   ELSE min(ug.auto_withdrawal_limit)::bigint
 END AS auto_limit
FROM user_groups ug
JOIN user_group_memberships ugm ON ugm.group_id = ug.id
WHERE ugm.user_id = @user_id;

-- name: InsertWithdrawal :one
INSERT INTO wallet_withdrawals (user_id, credit_amount, currency_code, credits_per_unit, fiat_amount, fee_amount, fee_basis, fee_rate, net_amount, status, payout_method_id, payout_provider, provider_ref, admin_note, reviewed_by, reviewed_at, idempotency_key) VALUES (@user_id, @credit_amount, @currency_code, @credits_per_unit, @fiat_amount, @fee_amount, @fee_basis, @fee_rate, @net_amount, @status, @payout_method_id, @payout_provider, @provider_ref, @admin_note, @reviewed_by, @reviewed_at, @idempotency_key)
RETURNING *;

-- name: GetWithdrawalByID :one
SELECT * FROM wallet_withdrawals
WHERE id = @id;

-- name: ListWithdrawalsByUser :many
SELECT * FROM wallet_withdrawals
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountWithdrawalsByUser :one
SELECT COUNT(*) FROM wallet_withdrawals
WHERE user_id = @user_id;

-- name: ListWithdrawals :many
SELECT * FROM wallet_withdrawals
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
 AND (sqlc.narg(user_id)::uuid IS NULL OR user_id = sqlc.narg(user_id))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountWithdrawals :one
SELECT COUNT(*) FROM wallet_withdrawals
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
 AND (sqlc.narg(user_id)::uuid IS NULL OR user_id = sqlc.narg(user_id));

-- name: UpdateWithdrawalStatus :one
UPDATE wallet_withdrawals
SET status = @status,
 provider_ref = COALESCE(@provider_ref, provider_ref),
 admin_note = COALESCE(@admin_note, admin_note),
 reviewed_by = COALESCE(@reviewed_by, reviewed_by),
 reviewed_at = COALESCE(@reviewed_at, reviewed_at),
 updated_at = now()
WHERE id = @id
RETURNING *;

-- name: InsertWithdrawalMethod :one
INSERT INTO wallet_withdrawal_methods (user_id, provider, label, account, currency_code, is_verified, is_default, is_active, metadata) VALUES (@user_id, @provider, @label, @account, @currency_code, @is_verified, @is_default, @is_active, @metadata)
RETURNING *;

-- name: ListWithdrawalMethodsByUser :many
SELECT * FROM wallet_withdrawal_methods
WHERE user_id = @user_id AND is_active = true
ORDER BY is_default DESC, created_at DESC;

-- name: GetWithdrawalMethodByID :one
SELECT * FROM wallet_withdrawal_methods
WHERE id = @id AND user_id = @user_id;

-- name: UpdateWithdrawalMethodDefault :exec
UPDATE wallet_withdrawal_methods
SET is_default = false
WHERE user_id = @user_id AND is_default = true;

-- name: UpdateWithdrawalMethod :one
UPDATE wallet_withdrawal_methods
SET is_default = @is_default,
 is_active = @is_active,
 updated_at = now()
WHERE id = @id
RETURNING *;

-- name: ListPayoutProviders :many
SELECT * FROM payout_providers

ORDER BY is_default DESC, name;

-- name: GetPayoutProviderByType :one
SELECT * FROM payout_providers
WHERE type = @type;

-- name: UpsertPayoutProvider :one
INSERT INTO payout_providers (type, name, is_active, is_default, config, supported_currencies) VALUES (@type, @name, @is_active, @is_default, @config, @supported_currencies)
ON CONFLICT (type) DO UPDATE SET
 name = EXCLUDED.name,
 is_active = EXCLUDED.is_active,
 is_default = EXCLUDED.is_default,
 config = EXCLUDED.config,
 supported_currencies = EXCLUDED.supported_currencies,
 updated_at = now()
RETURNING *;

-- name: DeletePayoutProvider :exec
DELETE FROM payout_providers
WHERE id = @id;

-- name: ListWithdrawalFeeTiers :many
SELECT * FROM wallet_withdrawal_fee_tiers
WHERE is_active = true
ORDER BY position, min_amount;

-- name: InsertWithdrawalFeeTier :one
INSERT INTO wallet_withdrawal_fee_tiers (min_amount, max_amount, fee_type, fee_value, tier_mode, is_active, position) VALUES (@min_amount, @max_amount, @fee_type, @fee_value, @tier_mode, @is_active, @position)
RETURNING *;

-- name: UpdateWithdrawalFeeTier :one
UPDATE wallet_withdrawal_fee_tiers
SET min_amount = @min_amount,
 max_amount = @max_amount,
 fee_type = @fee_type,
 fee_value = @fee_value,
 tier_mode = @tier_mode,
 is_active = @is_active,
 position = @position,
 updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteWithdrawalFeeTier :one
UPDATE wallet_withdrawal_fee_tiers
SET is_active = false, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: ClaimWithdrawalForProcessing :one
-- Atomic claim for the approval → payout transition (SEC-W-002). Only a row
-- still in a processable state can be claimed — the loser of a concurrent
-- approval race gets no row and must never reach the provider call.
UPDATE wallet_withdrawals
SET status = 'approved',
 reviewed_by = @reviewed_by,
 reviewed_at = now(),
 updated_at = now()
WHERE id = @id
 AND status IN ('pending', 'pending_approval')
RETURNING *;

-- name: GetUserWithdrawalTotalsSince :one
-- Rolling-window velocity totals for withdrawal caps (SEC-W-004). Failed,
-- rejected and cancelled rows released no value, so they don't count.
SELECT COALESCE(SUM(credit_amount), 0)::bigint AS credit_total,
 COUNT(*)::int AS withdrawal_count
FROM wallet_withdrawals
WHERE user_id = @user_id
 AND status IN ('pending', 'pending_approval', 'approved', 'processing', 'completed')
 AND created_at >= @since;

-- name: GetWithdrawalByProviderRef :one
SELECT * FROM wallet_withdrawals
WHERE provider_ref = @provider_ref
LIMIT 1;

-- name: RejectWithdrawalAtomic :one
-- Atomic reject claim (SEC-W-013): only a row still in a rejectable state
-- transitions — concurrent rejects can't both refund.
UPDATE wallet_withdrawals
SET status = 'rejected',
 admin_note = COALESCE(@admin_note, admin_note),
 reviewed_by = @reviewed_by,
 reviewed_at = now(),
 updated_at = now()
WHERE id = @id
 AND status IN ('pending', 'pending_approval')
RETURNING *;

-- name: ReopenWithdrawalForAppeal :one
-- Atomic rejected → pending_approval reopen (SEC-W-013): conditional so a
-- concurrent re-review can't double-deduct the re-reserved credits.
UPDATE wallet_withdrawals
SET status = 'pending_approval',
 reviewed_by = NULL,
 reviewed_at = NULL,
 updated_at = now()
WHERE id = @id
 AND status = 'rejected'
RETURNING *;

-- name: UpdateWithdrawalMethodMetadata :exec
-- Cache provider-side artifacts on the method (e.g. Paystack
-- recipient_code created lazily at first payout — SEC-W-NGN).
UPDATE wallet_withdrawal_methods
SET metadata = @metadata, updated_at = now()
WHERE id = @id;

-- name: GetInFlightWithdrawalTotalsByProvider :many
-- SEC-W-NGN float-vs-liability: sum of net payout amounts still in flight
-- per provider+currency — reconciler alerts when provider float < this.
SELECT payout_provider, currency_code,
 COUNT(*)::int AS row_count,
 COALESCE(SUM(net_amount), 0)::bigint AS net_total
FROM wallet_withdrawals
WHERE status IN ('pending', 'pending_approval', 'approved', 'processing')
GROUP BY payout_provider, currency_code;
