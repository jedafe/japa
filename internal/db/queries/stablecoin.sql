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
UPDATE service_escrows
SET state = @state,
    funded_sig = sqlc.narg(funded_sig),
    release_sig = sqlc.narg(release_sig),
    arbiter_decision = sqlc.narg(arbiter_decision),
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
