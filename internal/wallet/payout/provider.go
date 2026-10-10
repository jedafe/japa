// Package payout implements pluggable outbound payout providers for the
// credit withdrawal system (Section 58 Credit Withdrawal amendment).
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
package payout

import (
	"context"
)

// PayoutRequest holds the parameters for an outbound payout.
//
//nolint:revive // exported: stutter naming is intentional
type PayoutRequest struct {
	WithdrawalID string // Internal withdrawal ID (for idempotency/reference)
	UserID       string
	// AmountMinor is the payout amount in the currency's minor units
	// (kobo/cents/satoshi-equivalent integer). Integer math end-to-end —
	// never float — so crypto dust and fiat rounding cannot drift. Each
	// provider converts to its wire format at the edge with explicit
	// rounding.
	AmountMinor int64
	Currency    string // Fiat or crypto currency code (ISO-4217 or ticker)
	Address     string // Destination address / account identifier
	ExtraID     string // Memo / destination tag (optional)
	CallbackURL string // Webhook URL for provider status updates
}

// PayoutResponse holds the result of an outbound payout initiation.
//
//nolint:revive // exported: stutter naming is intentional
type PayoutResponse struct {
	ProviderRef string // Provider-side transaction/batch/payout ID
	Status      string // "pending", "processing", "completed", "failed"
	Message     string // Human-readable provider message
}

// Provider defines the interface for an outbound payout provider.
type Provider interface {
	// Type returns the provider type slug (e.g. "nowpayments", "paypal").
	Type() string
	// InitiatePayout submits the payout to the provider.
	InitiatePayout(ctx context.Context, req PayoutRequest) (PayoutResponse, error)
	// Supports reports whether this provider can handle the given currency.
	Supports(currency string) bool
}
