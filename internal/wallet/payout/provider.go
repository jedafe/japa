// Package payout implements pluggable outbound payout providers for the
// credit withdrawal system (Section 58 Credit Withdrawal amendment).
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
package payout

import (
	"context"
	"strconv"
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
	// FiatAmountMinor + FiatCurrency carry the fiat value of the payout when
	// Currency is a crypto coin — providers that support fiat-denominated
	// conversion (NOWPayments fiat_amount) use them instead of AmountMinor
	// so the platform's fiat net amount is never reinterpreted as a coin
	// quantity (SEC-W-001).
	FiatAmountMinor int64
	FiatCurrency    string
	Address         string // Destination address / account identifier
	// Metadata is the raw payout-method metadata blob (bank_code,
	// account_name, memo/tag, chain hints) — providers read what they
	// need without growing per-provider fields.
	Metadata        []byte
	ExtraID         string // Memo / destination tag (optional)
	CallbackURL     string // Webhook URL for provider status updates
}

// PayoutRejectedError marks a definitive provider-side rejection (HTTP 4xx —
// bad address, unsupported coin, insufficient merchant balance). The payout
// definitively did NOT execute, so the caller may safely refund credits.
// Transport-level failures (timeouts, unreadable responses, 5xx) return a
// plain error instead: the request may have reached the provider, so the
// withdrawal must be escalated for manual verification, not refunded
// (SEC-W-009).
//
//nolint:revive // exported: stutter naming is intentional
type PayoutRejectedError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e *PayoutRejectedError) Error() string {
	return e.Provider + " payout rejected (status " +
		strconv.Itoa(e.StatusCode) + "): " + e.Body
}

// PayoutResponse holds the result of an outbound payout initiation.
//
//nolint:revive // exported: stutter naming is intentional
type PayoutResponse struct {
	ProviderRef string // Provider-side transaction/batch/payout ID
	Status      string // "pending", "processing", "completed", "failed"
	Message     string // Human-readable provider message
	// MethodMetadataUpdates carries provider artifacts the caller should
	// merge into the payout method's metadata (e.g. a lazily-created
	// recipient_code). The wallet persists them after a successful
	// InitiatePayout so subsequent payouts reuse the cached value.
	MethodMetadataUpdates map[string]interface{}
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

// StatusProvider is an optional capability interface for providers that can
// report the live status of an in-flight payout. The wallet:reconcile_payouts
// task uses it to settle or refund withdrawals stuck in pending/processing
// (ALG-005).
type StatusProvider interface {
	Provider
	// PayoutStatus returns the latest known status for a provider reference.
	PayoutStatus(ctx context.Context, providerRef string) (PayoutResponse, error)
}

// WebhookVerifier is an optional capability interface for providers that
// deliver signed status callbacks (NOWPayments IPN). The payout webhook
// route resolves the provider by type and delegates signature verification.
type WebhookVerifier interface {
	Provider
	// VerifyWebhookSignature validates the provider's signature over the
	// raw webhook payload.
	VerifyWebhookSignature(payload []byte, signature string) error
}

// WebhookEventParser is an optional capability interface for providers
// whose status webhooks use a custom envelope (Paystack transfer.*). The
// payout webhook handler delegates envelope parsing when implemented;
// providers without it use the flat {payout_id, status} shape.
type WebhookEventParser interface {
	Provider
	// ParsePayoutEvent extracts (providerRef, normalized status) from a
	// raw webhook payload. ok=false means "not a payout event" — caller
	// should ack and ignore (e.g. a payment event on a shared route).
	ParsePayoutEvent(payload []byte) (providerRef, status string, ok bool)
}

// BalanceProvider is an optional capability interface for providers that
// report the available payout float (SEC-W-NGN). The admin float
// endpoint aggregates every registered provider's balance so ops can
// see unfunded rails before payouts stall.
type BalanceProvider interface {
	Provider
	// PayoutBalance returns spendable balances keyed by currency code.
	PayoutBalance(ctx context.Context) (map[string]float64, error)
}
