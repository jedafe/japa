-- Stablecoin Wallets (Section 128)
-- Non-custodial member smart wallets, onchain activity mirror, Squads
-- service escrow vaults, treasury registry. Chain is truth; these tables
-- are render cache + orchestration state.

-- =====================
-- Member wallets
-- =====================

-- name: InsertStablecoinWallet :one
INSERT INTO stablecoin_wallets (user_id, chain, address, locator, recovery_methods)
VALUES (@user_id, @chain, @address, @locator, @recovery_methods)
RETURNING *;

-- name: GetStablecoinWalletByUser :one
SELECT * FROM stablecoin_wallets
WHERE user_id = @user_id;

-- name: GetStablecoinWalletByID :one
SELECT * FROM stablecoin_wallets WHERE id = @id;

-- name: GetStablecoinWalletByAddress :one
SELECT * FROM stablecoin_wallets
WHERE address = @address;

-- name: SetStablecoinWalletStatus :one
UPDATE stablecoin_wallets
SET status = @status, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SetStablecoinWalletRecoveryMethods :exec
UPDATE stablecoin_wallets
SET recovery_methods = @recovery_methods, updated_at = now()
WHERE id = @id;

-- name: SetStablecoinWalletDelegatedSigner :exec
UPDATE stablecoin_wallets
SET delegated_signer = @delegated_signer, updated_at = now()
WHERE id = @id;

-- name: ListStablecoinWalletsByStatus :many
SELECT * FROM stablecoin_wallets
WHERE status = @status
ORDER BY created_at ASC
LIMIT @limit_count;

-- name: ListStablecoinWallets :many
-- Admin wallet registry — optional status filter ('' = all), address/user
-- search substring via sqlc.narg.
SELECT * FROM stablecoin_wallets
WHERE (@status::text = '' OR status = @status::text)
  AND (sqlc.narg(q) IS NULL OR address ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY created_at DESC
LIMIT @limit_count OFFSET @offset_count;

-- =====================
-- Activity mirror
-- =====================

-- name: InsertStablecoinActivity :one
-- tx_signature UNIQUE makes webhook replays idempotent — on conflict the
-- provider event is a dedupe hit, not a new row.
INSERT INTO stablecoin_activity (wallet_id, direction, kind, asset, amount_minor, credits_amount, counterparty_user_id, tx_signature, slot, status, occurred_at, raw)
VALUES (@wallet_id, @direction, @kind, @asset, @amount_minor, @credits_amount, sqlc.narg(counterparty_user_id), sqlc.narg(tx_signature), sqlc.narg(slot), @status, @occurred_at, sqlc.narg(raw))
ON CONFLICT (tx_signature) DO NOTHING
RETURNING *;

-- name: ListStablecoinActivityByWallet :many
SELECT * FROM stablecoin_activity
WHERE wallet_id = @wallet_id
ORDER BY occurred_at DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: GetStablecoinActivityByTxSignature :one
SELECT * FROM stablecoin_activity
WHERE tx_signature = @tx_signature;

-- name: SetStablecoinActivityStatus :one
UPDATE stablecoin_activity
SET status = @status
WHERE id = @id
RETURNING *;

-- name: ListPendingStablecoinActivity :many
-- Reconciliation sweep source — pending rows older than a webhook gap.
SELECT * FROM stablecoin_activity
WHERE status = 'pending'
ORDER BY occurred_at ASC
LIMIT @limit_count;

-- =====================
-- Service escrows
-- =====================

-- name: InsertServiceEscrow :one
INSERT INTO service_escrows (booking_id, buyer_wallet_id, seller_wallet_id, vault_address, asset, amount_minor, fee_minor, confirmation_deadline)
VALUES (@booking_id, @buyer_wallet_id, @seller_wallet_id, @vault_address, @asset, @amount_minor, @fee_minor, sqlc.narg(confirmation_deadline))
RETURNING *;

-- name: GetServiceEscrow :one
SELECT * FROM service_escrows
WHERE id = @id;

-- name: GetServiceEscrowByBooking :one
SELECT * FROM service_escrows
WHERE booking_id = @booking_id
ORDER BY created_at DESC
LIMIT 1;

-- name: SetServiceEscrowState :one
-- COALESCE: transitions that don't carry a sig/decision must not wipe
-- what earlier transitions recorded.
UPDATE service_escrows
SET state = @state,
    funded_sig = COALESCE(sqlc.narg(funded_sig), funded_sig),
    release_sig = COALESCE(sqlc.narg(release_sig), release_sig),
    arbiter_decision = COALESCE(sqlc.narg(arbiter_decision), arbiter_decision),
    confirmation_deadline = COALESCE(sqlc.narg(confirmation_deadline), confirmation_deadline),
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: ListServiceEscrowsByState :many
-- stablecoin:escrow_timeout sweep source.
SELECT * FROM service_escrows
WHERE state = @state
ORDER BY created_at ASC
LIMIT @limit_count;

-- name: ListExpiredFundedEscrows :many
-- Buyer-ghost timeout: funded vaults past confirmation deadline.
SELECT * FROM service_escrows
WHERE state = 'funded' AND confirmation_deadline < now()
ORDER BY confirmation_deadline ASC
LIMIT @limit_count;

-- =====================
-- Treasury registry
-- =====================

-- name: UpsertTreasuryWallet :one
INSERT INTO treasury_wallets (role, chain, address, locator, float_cap_minor)
VALUES (@role, @chain, @address, sqlc.narg(locator), sqlc.narg(float_cap_minor))
ON CONFLICT (chain, role) DO UPDATE SET
    address = EXCLUDED.address,
    locator = EXCLUDED.locator,
    float_cap_minor = EXCLUDED.float_cap_minor
RETURNING *;

-- name: GetTreasuryWallet :one
SELECT * FROM treasury_wallets
WHERE chain = @chain AND role = @role;

-- name: ListTreasuryWallets :many
SELECT * FROM treasury_wallets
ORDER BY role ASC;

-- name: SetServiceEscrowVaultAddress :exec
-- Client derives the Squads PDA off-chain and registers it once the vault
-- exists onchain.
UPDATE service_escrows
SET vault_address = @vault_address, updated_at = now()
WHERE id = @id;

-- name: GetStablecoinWalletByUserStatus :one
SELECT * FROM stablecoin_wallets
WHERE user_id = @user_id AND status = @status;

-- =====================
-- Treasury sweep proposals (§128.7)
-- =====================

-- name: InsertTreasurySweep :one
INSERT INTO treasury_sweeps (chain, asset, amount_minor, hot_wallet_id, cold_address, proposed_by)
VALUES (@chain, @asset, @amount_minor, @hot_wallet_id, @cold_address, @proposed_by)
RETURNING *;

-- name: GetTreasurySweep :one
SELECT * FROM treasury_sweeps
WHERE id = @id;

-- name: ListTreasurySweepsByState :many
SELECT * FROM treasury_sweeps
WHERE state = @state
ORDER BY created_at DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: DecideTreasurySweep :one
-- Approve or reject — records the treasury.sweep_approve decision.
UPDATE treasury_sweeps
SET state = @state,
    approved_by = sqlc.narg(approved_by),
    squads_proposal_id = sqlc.narg(squads_proposal_id),
    decided_at = now()
WHERE id = @id AND state = 'proposed'
RETURNING *;

-- name: MarkTreasurySweepExecuted :exec
UPDATE treasury_sweeps
SET state = 'executed', executed_sig = @executed_sig, executed_at = now()
WHERE id = @id;

-- name: ListActiveTreasurySweeps :many
-- Reconcile watches proposals awaiting Squads execution.
SELECT * FROM treasury_sweeps
WHERE state IN ('proposed', 'approved')
ORDER BY created_at ASC;
