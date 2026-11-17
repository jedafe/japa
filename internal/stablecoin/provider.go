package stablecoin

import "context"

// Provider is the wallet-infrastructure seam — Crossmint today, swappable.
// The wallet domain never knows whether settlement runs through Crossmint,
// Circle, or another Wallet-as-a-Service (design doc §9 adapter pattern).
type Provider interface {
	// CreateWallet provisions a non-custodial smart wallet. The returned
	// address may be counterfactual (assigned now, deployed on first tx).
	CreateWallet(ctx context.Context, in CreateWalletInput) (ProviderWallet, error)

	// GetWallet fetches wallet state by provider locator.
	GetWallet(ctx context.Context, locator string) (ProviderWallet, error)

	// RegisterDelegatedSigner adds a scope-capped operational signer to a
	// member wallet. Registration requires the member's recovery approval —
	// the provider drives that ceremony.
	RegisterDelegatedSigner(ctx context.Context, locator string, in DelegatedSignerInput) error

	// RemoveDelegatedSigner deregisters a prior signer — rotation drops the
	// old registration so overlapping signers don't double the blast radius.
	// Removal needs member recovery approval, same as registration.
	RemoveDelegatedSigner(ctx context.Context, locator, signerRef string) error

	// Transfer executes a delegated-signer operation. Calls must batch
	// recipient + fee legs atomically where both are set.
	Transfer(ctx context.Context, locator string, in TransferInput) (TransferResult, error)

	// BuildWithdrawal returns a signable operation for the member's passkey
	// flow (external send or offramp deposit). The backend relays; only the
	// member can sign.
	BuildWithdrawal(ctx context.Context, locator string, in WithdrawalInput) (SignableOp, error)

	// CreateOnrampSession starts a fiat→stablecoin deposit destined
	// directly for the member's wallet address.
	CreateOnrampSession(ctx context.Context, in OnrampInput) (OnrampSession, error)

	// Activity returns onchain events for a wallet (fallback when webhooks
	// miss; reconcile sweep consumes this).
	// Activity returns one page of wallet events; nextCursor is empty when
	// the page is last. Callers page until nextCursor=="".
	Activity(ctx context.Context, locator string, cursor string) (events []ProviderEvent, nextCursor string, err error)

	// VerifyWebhook authenticates an inbound provider event.
	VerifyWebhook(payload []byte, signature string) error
}

// CreateWalletInput requests a member wallet.
type CreateWalletInput struct {
	UserID          string
	Chain           string   // "solana"
	RecoveryMethods []string // "email", "phone"
	// MemberAuth binds the wallet to the member's passkey/device signer —
	// provider-specific (email OTP handle, passkey ceremony token, etc).
	MemberAuth string
}

// DelegatedSignerInput describes the signer registration.
type DelegatedSignerInput struct {
	SignerRef string
	Scopes    []SignerScope
}

// TransferInput is one delegated-signer operation. RecipientLeg and FeeLeg
// are batched atomically — the fee leg is how tips/purchases monetize
// (§128.5, §128.8).
type TransferInput struct {
	Asset         string
	RecipientAddr string // member wallet, treasury hot, or escrow vault
	AmountMinor   int64
	FeeAddr       string // treasury hot wallet — empty = no fee leg
	FeeMinor      int64
	Idempotency   string // caller idempotency key (Rule 9)
	Reference     string // internal correlation (tip id, booking id, ...)
}

// WithdrawalInput requests a member-signable send.
type WithdrawalInput struct {
	Asset       string
	Destination string // external address or offramp deposit
	AmountMinor int64
	FeeMinor    int64 // batched: amount−fee → dest, fee → treasury
}

// SignableOp is an unsigned smart-wallet operation returned to the client
// for passkey signature (WebAuthn inside japa.to — the provider never sees
// a Crossmint-branded surface).
type SignableOp struct {
	Locator   string
	Payload   []byte // provider wire format — opaque to us
	ExpiresAt int64
}

// OnrampInput starts a funding session.
type OnrampInput struct {
	WalletAddress string // direct-to-member delivery (BLOCKER 1.4)
	Asset         string
	FiatCurrency  string
	Ref           string
}

// OnrampSession is a provider checkout handle.
type OnrampSession struct {
	URL   string
	Token string
	ID    string
}

// ProviderWallet is the provider's view of a wallet.
type ProviderWallet struct {
	Locator  string
	Address  string
	Chain    string
	Status   string
	Balances map[string]int64 // asset → minor units (treasury balance reads)
}

// ProviderEvent is one onchain activity event from webhook or poll.
type ProviderEvent struct {
	Type        string // transfer_in, transfer_out, confirmed, failed, ...
	TxSignature string
	Slot        int64
	Asset       string
	AmountMinor int64
	CounterAddr string
	OccurredAt  int64
	Raw         []byte
}

// TransferResult is the execution receipt.
type TransferResult struct {
	TxSignature string
	Status      string // submitted | confirmed | failed
}
