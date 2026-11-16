// Package stablecoin implements non-custodial member smart wallets powered
// by stablecoin settlement (Section 128, ADR-140).
//
// Members own Crossmint smart wallets on Solana (device passkey + OTP
// recovery); Japa.to holds only a scope-capped delegated signer for seamless
// in-app spending — never member keys, never unilateral control. Platform
// revenue accumulates in a custodial hot/cold treasury.
//
// Two credit tiers face the member: balance credits (display denomination of
// the onchain wallet balance — withdrawable by member signature) and bonus
// credits (spend-only ledger instrument in internal/wallet — referral +
// signup-bonus sources only).
//
// Ref: japaconnect-engineering/sections/128-stablecoin-wallet-custody.md
// Ref: japaconnect-engineering/adr/ADR-140-non-custodial-stablecoin-wallets.md
package stablecoin

import (
	"errors"
	"time"
)

// ErrNotImplemented marks provider methods stubbed pending the Crossmint
// commercial/technical validations (§128 open items) — callers should map
// it to 503/feature-unavailable, never 500.
var ErrNotImplemented = errors.New("stablecoin: provider method not implemented")

// Asynq task names — registered in Section 11 (Rule 7).
const (
	TaskProvisionWallet     = "stablecoin:provision_wallet"  // lazy wallet provisioning
	TaskSyncActivity        = "stablecoin:sync_activity"     // webhook event → mirror
	TaskReconcileWallets    = "stablecoin:reconcile_wallets" // mirror-vs-chain sweep
	TaskSignerScopeRenew    = "stablecoin:signer_scope_renew"
	TaskEscrowTimeout       = "stablecoin:escrow_timeout"
	TaskTreasurySweep       = "stablecoin:treasury_sweep"
	TaskProcessOfframp      = "stablecoin:process_offramp"
	TaskTipSettleStablecoin = "tips:settle_stablecoin" // onchain tip settlement
)

// Wallet lifecycle states (stablecoin_wallets.status).
const (
	WalletStatusProvisioned = "provisioned"
	WalletStatusActive      = "active"
	WalletStatusSuspended   = "suspended"
)

// Assets accepted for settlement (native Solana SPL).
const (
	AssetUSDC = "usdc"
	AssetUSDT = "usdt"
)

// Activity kinds (stablecoin_activity.kind).
const (
	ActivityDeposit       = "deposit"
	ActivityTipIn         = "tip_in"
	ActivityTipOut        = "tip_out"
	ActivityPurchase      = "purchase"
	ActivityFee           = "fee"
	ActivityWithdrawal    = "withdrawal"
	ActivityOfframp       = "offramp"
	ActivityEscrowFunding = "escrow_funding"
	ActivityEscrowRelease = "escrow_release"
)

// Escrow states (service_escrows.state).
const (
	EscrowFunding  = "funding"
	EscrowFunded   = "funded"
	EscrowReleased = "released"
	EscrowRefunded = "refunded"
	EscrowDisputed = "disputed"
)

// Treasury wallet roles (treasury_wallets.role).
const (
	TreasuryHot     = "hot"
	TreasuryCold    = "cold"
	TreasuryRevenue = "revenue"
)

// Wallet is a member's non-custodial smart wallet (mirror of
// stablecoin_wallets). Private keys never live here — the member's device
// secure enclave holds the operational signer; recovery is their email/phone
// OTP. All this row records is metadata + our delegated-signer registration.
type Wallet struct {
	ID              string
	UserID          string
	Chain           string // "solana"
	Address         string // smart wallet / PDA address
	Locator         string // Crossmint wallet locator
	Status          string
	RecoveryMethods []string // "email", "phone"
	DelegatedSigner *DelegatedSigner
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// DelegatedSigner is Japa.to's scope-capped operational signer on a member
// wallet. Scopes are enforced onchain — the platform cannot exceed them even
// if fully compromised.
type DelegatedSigner struct {
	SignerRef    string // provider signer reference / public key
	Scopes       []SignerScope
	RegisteredAt time.Time
}

// SignerScope bounds one delegated-signer capability onchain.
type SignerScope struct {
	Asset         string    // usdc | usdt
	DailyCapMinor int64     // USD minor units/day
	ExpiresAt     time.Time // short-lived; renewed by stablecoin:signer_scope_renew
}

// Activity is one mirrored onchain event (stablecoin_activity). Chain is
// truth; these rows are the render cache for the unified history feed.
type Activity struct {
	ID                 string
	WalletID           string
	Direction          string // in | out
	Kind               string
	Asset              string
	AmountMinor        int64
	CreditsAmount      int64 // display value at event-time ratio
	CounterpartyUserID *string
	TxSignature        string
	Slot               int64
	Status             string // pending | confirmed | failed
	OccurredAt         time.Time
}

// Escrow is a Squads 2-of-3 multisig vault settling one §111 service
// booking (buyer, seller, platform-arbiter). The arbiter key can co-sign
// but never move funds alone.
type Escrow struct {
	ID                   string
	BookingID            string
	BuyerWalletID        string
	SellerWalletID       string
	VaultAddress         string // Squads multisig PDA
	Asset                string
	AmountMinor          int64
	FeeMinor             int64
	State                string
	FundedSig            string
	ReleaseSig           string
	ConfirmationDeadline time.Time
	ArbiterDecision      *ArbiterDecision
}

// ArbiterDecision records a dispute ruling. The node grants decision
// authority only — execution still needs an onchain co-signer.
type ArbiterDecision struct {
	ReleasedTo string // buyer | seller
	DecidedBy  string // admin user id
	DecidedAt  time.Time
	Note       string
}

// TreasuryWallet is a platform-owned custodial wallet (hot float / cold
// vault / revenue collection). This is OUR money — the only pool whose keys
// the platform controls.
type TreasuryWallet struct {
	ID            string
	Role          string // hot | cold | revenue
	Chain         string
	Address       string
	Locator       string // provider locator; empty for Squads cold vault
	FloatCapMinor int64  // hot only
}

// BalanceSummary is what the wallet surface shows — credits-denominated.
type BalanceSummary struct {
	BackedCredits int64 `json:"backed_credits"` // onchain USDC/USDT mirror
	BonusCredits  int64 `json:"bonus_credits"`  // spend-only ledger tier
	Pending       int64 `json:"pending"`        // mirrored but unconfirmed
}
