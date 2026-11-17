// Package crossmint is the Crossmint adapter for stablecoin.Provider —
// wallets, delegated signers, transfers, onramps, offramps, webhooks.
//
// Env config (Rule 3 — JC_* only):
//
//	JC_CROSSMINT_API_KEY        — server-side API key (SECRET)
//	JC_CROSSMINT_BASE_URL       — API base (staging vs production)
//	JC_CROSSMINT_WEBHOOK_SECRET — inbound event signature secret
//
// API surface per public docs (api/2025-06-09 wallets, api/2022-06-09
// orders). Wire shapes match the documented examples; response fields are
// read tolerantly since the commercial contract is still being validated
// (§128 open items).
//
// Ref: japaconnect-engineering/sections/128-stablecoin-wallet-custody.md
package crossmint

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/jedafe/japa/internal/stablecoin"
)

const (
	walletsBase = "/api/2025-06-09/wallets"
	ordersBase  = "/api/2022-06-09/orders"
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
	if baseURL == "" {
		baseURL = "https://staging.crossmint.com"
	}
	return &Client{
		apiKey:        apiKey,
		baseURL:       baseURL,
		webhookSecret: webhookSecret,
		http:          &http.Client{Timeout: 15 * time.Second},
	}
}

// Compile-time check: Client satisfies the Provider seam.
var _ stablecoin.Provider = (*Client)(nil)

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	return c.doHeaders(ctx, method, path, body, nil, out)
}

func (c *Client) doHeaders(ctx context.Context, method, path string, body any, headers map[string]string, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("crossmint: marshal: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("crossmint: request: %w", err)
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("crossmint: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("crossmint: %s %s: status %d: %s", method, path, resp.StatusCode, truncate(string(data), 300))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("crossmint: decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ── Wallet lifecycle ────────────────────────────────────────────────────

type walletResp struct {
	Address  string `json:"address"`
	Locator  string `json:"locator"`
	Chain    string `json:"chain"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Balances []struct {
		Token  string `json:"token"`
		Amount string `json:"amount"`
	} `json:"balances"`
}

func (w walletResp) toProvider() stablecoin.ProviderWallet {
	pw := stablecoin.ProviderWallet{
		Address:  w.Address,
		Locator:  w.Locator,
		Chain:    w.Chain,
		Status:   w.Status,
		Balances: map[string]int64{},
	}
	if pw.Locator == "" {
		pw.Locator = w.Address
	}
	for _, b := range w.Balances {
		if n, err := strconv.ParseInt(b.Amount, 10, 64); err == nil {
			// Normalize "solana:usdc" token locators to bare assets so
			// callers key on usdc/usdt uniformly.
			tok := b.Token
			if i := len("solana:"); len(tok) > i && tok[:i] == "solana:" {
				tok = tok[i:]
			}
			pw.Balances[tok] = n
		}
	}
	return pw
}

// CreateWallet provisions a member smart wallet (counterfactual address).
// POST /wallets — Solana, member-bound via linked user locator.
func (c *Client) CreateWallet(ctx context.Context, in stablecoin.CreateWalletInput) (stablecoin.ProviderWallet, error) {
	chain := in.Chain
	if chain == "" {
		chain = "solana"
	}
	body := map[string]any{
		"type":  "smart",
		"chain": chain,
		// Member binding — the recovery/OTP handle that owns the wallet.
		"linkedUser": "userId:" + in.UserID,
	}
	var out walletResp
	if err := c.do(ctx, http.MethodPost, walletsBase, body, &out); err != nil {
		return stablecoin.ProviderWallet{}, err
	}
	return out.toProvider(), nil
}

// GetWallet fetches wallet state by locator.
func (c *Client) GetWallet(ctx context.Context, locator string) (stablecoin.ProviderWallet, error) {
	var out walletResp
	if err := c.do(ctx, http.MethodGet, walletsBase+"/"+locator, nil, &out); err != nil {
		return stablecoin.ProviderWallet{}, err
	}
	return out.toProvider(), nil
}

// ── Delegated signer ────────────────────────────────────────────────────

// RegisterDelegatedSigner registers Japa.to's scoped operational signer.
// POST /{locator}/signers — transfer scopes per asset, signer-level
// expiresAt. Registration needs the member's recovery approval; the
// provider drives that ceremony (§128.2).
func (c *Client) RegisterDelegatedSigner(ctx context.Context, locator string, in stablecoin.DelegatedSignerInput) error {
	scopes := make([]map[string]any, 0, len(in.Scopes))
	var expiresAt string
	for _, sc := range in.Scopes {
		scope := map[string]any{
			"type":         "transfer",
			"tokenLocator": "solana:" + sc.Asset,
			"spendingLimit": map[string]any{
				// Display units per docs ("10" = 10 USDC) — minor→display
				// via 6-decimal SPL scale. Sending raw minor here would
				// inflate the onchain cap by 10^6.
				"amount":   strconv.FormatFloat(float64(sc.DailyCapMinor)/1e6, 'f', -1, 64),
				"interval": 86400,
			},
		}
		// Docs-confirmed: recipients is the chain-level destination
		// whitelist (addresses or wallet locators). Emitted only when set —
		// an empty array means "any recipient" like omission.
		if len(sc.Recipients) > 0 {
			scope["recipients"] = sc.Recipients
		}
		scopes = append(scopes, scope)
		if !sc.ExpiresAt.IsZero() {
			expiresAt = sc.ExpiresAt.UTC().Format(time.RFC3339)
		}
	}
	body := map[string]any{
		"signer": map[string]any{
			"type":    "external-wallet",
			"address": in.SignerRef,
		},
		"scopes": scopes,
	}
	if expiresAt != "" {
		body["expiresAt"] = expiresAt
	}
	return c.do(ctx, http.MethodPost, walletsBase+"/"+locator+"/signers", body, nil)
}

// RemoveDelegatedSigner deregisters a prior signer by reference — rotation
// cleanup so overlapping signers don't double the blast radius.
func (c *Client) RemoveDelegatedSigner(ctx context.Context, locator, signerRef string) error {
	return c.do(ctx, http.MethodDelete, walletsBase+"/"+locator+"/signers/"+signerRef, nil, nil)
}

// ── Transfers ───────────────────────────────────────────────────────────

type txResp struct {
	ID            string `json:"id"`
	TxSignature   string `json:"txSignature"`
	OnChainTxID   string `json:"onChainTxId"`
	Status        string `json:"status"`
	Approvals     any    `json:"approvals"`
	TransactionID string `json:"transactionId"`
}

// Transfer executes the recipient leg via delegated signer, then the fee
// leg as a second op to the hot treasury when present. Docs confirm
// transfer-only scopes; whether one op can batch multiple SPL instructions
// is an open validation item — until then the two legs are sequential ops
// on the same scope (same daily cap). If the fee leg fails the principal
// has already moved — the reconcile mirror surfaces it for recovery.
func (c *Client) Transfer(ctx context.Context, locator string, in stablecoin.TransferInput) (stablecoin.TransferResult, error) {
	res, err := c.transferLeg(ctx, locator, in.Signer, in.Asset, in.RecipientAddr, in.AmountMinor, in.Idempotency)
	if err != nil {
		return stablecoin.TransferResult{}, err
	}
	if in.FeeMinor > 0 && in.FeeAddr != "" {
		feeKey := in.Idempotency
		if feeKey != "" {
			feeKey += ":fee"
		}
		if _, err := c.transferLeg(ctx, locator, in.Signer, in.Asset, in.FeeAddr, in.FeeMinor, feeKey); err != nil {
			return res, fmt.Errorf("crossmint: fee leg after principal %s: %w", res.TxSignature, err)
		}
	}
	return res, nil
}

// transferLeg uses the documented token-transfer endpoint —
// POST /wallets/{locator}/tokens/{tokenLocator}/transfers. Per the OpenAPI
// spec: amount is DISPLAY units ("10" = 10 USDC), the recipient field is
// `recipient` (address or wallet locator), the delegated signer is
// `signer: "external-wallet:<addr>"`, and idempotency rides the
// x-idempotency-key header — not the body.
func (c *Client) transferLeg(ctx context.Context, locator, signerRef, asset, dest string, amountMinor int64, idem string) (stablecoin.TransferResult, error) {
	body := map[string]any{
		"amount":    displayAmount(amountMinor),
		"recipient": dest,
	}
	if signerRef != "" {
		body["signer"] = "external-wallet:" + signerRef
	}
	headers := map[string]string{}
	if idem != "" {
		headers["x-idempotency-key"] = idem
	}
	var out txResp
	if err := c.doHeaders(ctx, http.MethodPost, walletsBase+"/"+locator+"/tokens/solana:"+asset+"/transfers",
		body, headers, &out); err != nil {
		return stablecoin.TransferResult{}, err
	}
	res := stablecoin.TransferResult{
		TxSignature: out.TxSignature,
		Status:      "submitted",
	}
	if res.TxSignature == "" {
		res.TxSignature = out.OnChainTxID
	}
	return res, nil
}

// displayAmount renders minor units as the API's decimal string
// ("10500000" → "10.5") — the transfers endpoint and scope spending
// limits both take display units per the documented examples.
func displayAmount(minor int64) string {
	return strconv.FormatFloat(float64(minor)/1e6, 'f', -1, 64)
}

// BuildWithdrawal returns a member-signable op — same documented transfers
// endpoint; the created transaction enters the wallet's approval flow and
// only the member's passkey can satisfy it (the delegated signer is NOT
// passed — a member-signed send can't ride our scope).
func (c *Client) BuildWithdrawal(ctx context.Context, locator string, in stablecoin.WithdrawalInput) (stablecoin.SignableOp, error) {
	body := map[string]any{
		"amount":    displayAmount(in.AmountMinor),
		"recipient": in.Destination,
	}
	var out struct {
		txResp
		// Tolerant expiry decode — surface the provider's real signing
		// deadline when it returns one rather than our guessed constant.
		ExpiresAt  string `json:"expiresAt"`
		ValidUntil string `json:"validUntil"`
	}
	if err := c.do(ctx, http.MethodPost, walletsBase+"/"+locator+"/tokens/solana:"+in.Asset+"/transfers",
		body, &out); err != nil {
		return stablecoin.SignableOp{}, err
	}
	payload, _ := json.Marshal(out.txResp)
	expires := time.Now().Add(10 * time.Minute).Unix()
	for _, raw := range []string{out.ExpiresAt, out.ValidUntil} {
		if raw == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			expires = t.Unix()
			break
		}
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			expires = n
			break
		}
	}
	return stablecoin.SignableOp{
		Locator:   locator,
		Payload:   payload,
		ExpiresAt: expires,
	}, nil
}

// ── Onramp ──────────────────────────────────────────────────────────────

// CreateOnrampSession starts direct-to-member fiat funding — the order
// delivers USDC straight to the member wallet address (docs-confirmed
// recipient.walletAddress delivery, §128.4).
func (c *Client) CreateOnrampSession(ctx context.Context, in stablecoin.OnrampInput) (stablecoin.OnrampSession, error) {
	fiat := in.FiatCurrency
	if fiat == "" {
		fiat = "usd"
	}
	asset := in.Asset
	if asset == "" {
		asset = "usdc"
	}
	body := map[string]any{
		"lineItems": []map[string]any{{
			"currencyLocator": "solana:" + asset,
			"executionParameters": map[string]any{
				"mode": "exact-in",
			},
		}},
		"payment": map[string]any{
			"currency": fiat,
			"method":   "card",
		},
		"recipient": map[string]any{
			"walletAddress": in.WalletAddress,
		},
	}
	var out struct {
		Order struct {
			OrderID string `json:"orderId"`
			Phase   string `json:"phase"`
		} `json:"order"`
		ClientSecret string `json:"clientSecret"`
		CheckoutURL  string `json:"checkoutUrl"`
	}
	if err := c.do(ctx, http.MethodPost, ordersBase, body, &out); err != nil {
		return stablecoin.OnrampSession{}, err
	}
	return stablecoin.OnrampSession{
		ID:    out.Order.OrderID,
		Token: out.ClientSecret,
		URL:   out.CheckoutURL,
	}, nil
}

// ── Activity ────────────────────────────────────────────────────────────

// Activity polls onchain events for the reconcile sweep (webhook-gap
// backstop). The wallet activity endpoint is versioned under the wallets
// base.
func (c *Client) Activity(ctx context.Context, locator string, cursor string) ([]stablecoin.ProviderEvent, string, error) {
	path := walletsBase + "/" + locator + "/activity"
	if cursor != "" {
		path += "?cursor=" + cursor
	}
	var out struct {
		Events []struct {
			Type        string `json:"type"`
			TxSignature string `json:"txSignature"`
			Slot        int64  `json:"slot"`
			Token       string `json:"token"`
			Amount      string `json:"amount"`
			CounterAddr string `json:"counterparty"`
			Timestamp   int64  `json:"timestamp"`
		} `json:"events"`
		// Tolerant across likely cursor shapes (docs-unverified).
		NextCursor string `json:"nextCursor"`
		Cursor     string `json:"cursor"`
		Page       struct {
			Next string `json:"next"`
		} `json:"page"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, "", err
	}
	next := out.NextCursor
	if next == "" {
		next = out.Page.Next
	}
	if next == "" {
		next = out.Cursor
	}
	if next == cursor { // provider echoed the request cursor — no more pages
		next = ""
	}
	events := make([]stablecoin.ProviderEvent, 0, len(out.Events))
	for _, e := range out.Events {
		var amt int64
		amt, _ = strconv.ParseInt(e.Amount, 10, 64)
		ts := e.Timestamp
		if ts > 1_000_000_000_000 { // milliseconds, not seconds (secs < 4e10 until year 5138)
			ts /= 1000
		}
		events = append(events, stablecoin.ProviderEvent{
			Type:        e.Type,
			TxSignature: e.TxSignature,
			Slot:        e.Slot,
			Asset:       e.Token,
			AmountMinor: amt,
			CounterAddr: e.CounterAddr,
			OccurredAt:  ts,
		})
	}
	return events, next, nil
}

// ── Webhook verification ────────────────────────────────────────────────

// VerifyWebhook authenticates an inbound event — HMAC-SHA256 over the raw
// body with the configured secret, hex-compared. (Standard webhook scheme;
// confirm the exact Crossmint header/scheme at integration — see §128
// open validations.)
func (c *Client) VerifyWebhook(payload []byte, signature string) error {
	if c.webhookSecret == "" {
		return errors.New("crossmint: webhook secret unset")
	}
	if signature == "" {
		return errors.New("crossmint: missing webhook signature")
	}
	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	// Tolerate "sha256=" prefixed schemes.
	if len(signature) > 7 && signature[:7] == "sha256=" {
		signature = signature[7:]
	}
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return errors.New("crossmint: bad webhook signature")
	}
	return nil
}
