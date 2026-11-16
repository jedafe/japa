// Package crossmint is the Crossmint adapter for stablecoin.Provider —
// wallets, delegated signers, transfers, onramps, offramps, webhooks.
//
// Env config (Rule 3 — JC_* only):
//
//	JC_CROSSMINT_API_KEY        — server-side API key (SECRET)
//	JC_CROSSMINT_BASE_URL       — API base (staging vs production)
//	JC_CROSSMINT_WEBHOOK_SECRET — inbound event signature secret
//
// Ref: japaconnect-engineering/sections/128-stablecoin-wallet-custody.md
package crossmint

import (
	"context"
	"net/http"
	"time"

	"github.com/jedafe/japa/internal/stablecoin"
)

// Client implements stablecoin.Provider against the Crossmint API.
type Client struct {
	apiKey        string
	baseURL       string
	webhookSecret string
	http          *http.Client
}

// NewClient wires the adapter from env-sourced config.
func NewClient(apiKey, baseURL, webhookSecret string) *Client {
	return &Client{
		apiKey:        apiKey,
		baseURL:       baseURL,
		webhookSecret: webhookSecret,
		http:          &http.Client{Timeout: 15 * time.Second},
	}
}

// Compile-time check: Client satisfies the Provider seam.
var _ stablecoin.Provider = (*Client)(nil)

// CreateWallet provisions a member smart wallet (counterfactual address).
func (c *Client) CreateWallet(ctx context.Context, in stablecoin.CreateWalletInput) (stablecoin.ProviderWallet, error) {
	// TODO(§128.2): POST /wallets — Solana smart wallet, member auth binding.
	return stablecoin.ProviderWallet{}, stablecoin.ErrNotImplemented
}

// GetWallet fetches wallet state by locator.
func (c *Client) GetWallet(ctx context.Context, locator string) (stablecoin.ProviderWallet, error) {
	return stablecoin.ProviderWallet{}, stablecoin.ErrNotImplemented
}

// RegisterDelegatedSigner registers Japa.to's scoped operational signer —
// member recovery approval ceremony driven by the provider.
func (c *Client) RegisterDelegatedSigner(ctx context.Context, locator string, in stablecoin.DelegatedSignerInput) error {
	// TODO(§128.2): POST .../signers with scopes (transfer, daily cap, expiresAt)
	// — open validation §128 item 1 (Solana scope capabilities).
	return stablecoin.ErrNotImplemented
}

// Transfer executes a delegated-signer op — recipient + fee legs batched.
func (c *Client) Transfer(ctx context.Context, locator string, in stablecoin.TransferInput) (stablecoin.TransferResult, error) {
	return stablecoin.TransferResult{}, stablecoin.ErrNotImplemented
}

// BuildWithdrawal returns a member-signable op (passkey relay).
func (c *Client) BuildWithdrawal(ctx context.Context, locator string, in stablecoin.WithdrawalInput) (stablecoin.SignableOp, error) {
	return stablecoin.SignableOp{}, stablecoin.ErrNotImplemented
}

// CreateOnrampSession starts direct-to-member fiat funding.
func (c *Client) CreateOnrampSession(ctx context.Context, in stablecoin.OnrampInput) (stablecoin.OnrampSession, error) {
	return stablecoin.OnrampSession{}, stablecoin.ErrNotImplemented
}

// Activity polls onchain events for the reconcile sweep.
func (c *Client) Activity(ctx context.Context, locator string, cursor string) ([]stablecoin.ProviderEvent, error) {
	return nil, stablecoin.ErrNotImplemented
}

// VerifyWebhook authenticates an inbound event signature.
func (c *Client) VerifyWebhook(payload []byte, signature string) error {
	// TODO: HMAC signature verify against webhookSecret.
	return stablecoin.ErrNotImplemented
}
