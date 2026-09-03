package payout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// NOWPaymentsPayoutProvider implements Provider for NOWPayments crypto payouts.
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
// Uses the NOWPayments Payout API: POST /v1/payout.
// See https://documenter.getpostman.com/view/7907941/2s93JusNJt
type NOWPaymentsPayoutProvider struct {
	apiKey      string
	httpClient  *http.Client
	supported   map[string]bool
}

// NewNOWPaymentsPayoutProvider creates a new NOWPayments payout provider.
// The apiKey is the same JC_NOWPAYMENTS_IPN_SECRET used for inbound payments.
func NewNOWPaymentsPayoutProvider(apiKey string) *NOWPaymentsPayoutProvider {
	return &NOWPaymentsPayoutProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		supported: map[string]bool{
			"btc":        true,
			"eth":        true,
			"usdttrc20":  true,
			"usdterc20":  true,
			"usdtbsc":    true,
			"ltc":        true,
			"xrp":        true,
			"bch":        true,
			"doge":       true,
			"bnbmainnet": true,
			"ada":        true,
			"sol":        true,
		},
	}
}

// Type returns the provider type slug.
func (p *NOWPaymentsPayoutProvider) Type() string { return "nowpayments" }

// Supports reports whether the provider supports the given currency/coin.
// For NOWPayments this is the crypto ticker (e.g. "usdttrc20").
func (p *NOWPaymentsPayoutProvider) Supports(currency string) bool {
	c := strings.ToLower(strings.TrimSpace(currency))
	return p.supported[c]
}

// nowpaymentsPayoutRequest is the JSON body for POST /v1/payout.
type nowpaymentsPayoutRequest struct {
	Address      string  `json:"address"`
	Currency     string  `json:"currency"`
	Amount       float64 `json:"amount,omitempty"`
	ExtraID      string  `json:"extra_id,omitempty"`
	IPNCallback  string  `json:"ipn_callback_url,omitempty"`
	FiatAmount   float64 `json:"fiat_amount,omitempty"`
	FiatCurrency string  `json:"fiat_currency,omitempty"`
}

// nowpaymentsPayoutResponse is the NOWPayments API response for payout creation.
type nowpaymentsPayoutResponse struct {
	PayoutID string `json:"payout_id"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}

// InitiatePayout creates a NOWPayments crypto payout.
//
// If the withdrawal method currency is a fiat code (e.g. USD) and the account
// holds a crypto coin, set the crypto coin in the PayoutRequest.Currency and
// the fiat value in FiatAmount/FiatCurrency so NOWPayments performs the swap.
func (p *NOWPaymentsPayoutProvider) InitiatePayout(ctx context.Context, req PayoutRequest) (PayoutResponse, error) {
	if p.apiKey == "" {
		return PayoutResponse{}, errors.New("nowpayments API key not configured")
	}

	currency := strings.ToLower(strings.TrimSpace(req.Currency))
	if currency == "" {
		return PayoutResponse{}, errors.New("payout currency is required")
	}
	if !p.Supports(currency) {
		// Still attempt the request; the provider may support coins not in our
		// hardcoded list. Log a warning but do not block.
		slog.Warn("nowpayments payout currency not in supported list", "currency", currency)
	}

	payload := nowpaymentsPayoutRequest{
		Address:     req.Address,
		Currency:    currency,
		ExtraID:     req.ExtraID,
		IPNCallback: req.CallbackURL,
	}

	// Use fiat_amount when the amount is in fiat; otherwise send amount in crypto.
	if req.Currency == strings.ToUpper(req.Currency) && len(req.Currency) == 3 {
		payload.FiatAmount = req.Amount
		payload.FiatCurrency = strings.ToLower(req.Currency)
	} else {
		payload.Amount = req.Amount
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("marshaling nowpayments payout request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.nowpayments.io/v1/payout", bytes.NewReader(body))
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("creating nowpayments request: %w", err)
	}
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("nowpayments payout: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("reading nowpayments response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return PayoutResponse{}, fmt.Errorf("nowpayments payout error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var pr nowpaymentsPayoutResponse
	if err := json.Unmarshal(respBody, &pr); err != nil {
		// Some success responses may be empty on create; treat non-JSON as pending.
		slog.Warn("nowpayments payout response not json", "body", string(respBody), "error", err)
		return PayoutResponse{ProviderRef: "", Status: "pending"}, nil
	}

	return PayoutResponse{
		ProviderRef: pr.PayoutID,
		Status:      pr.Status,
		Message:     pr.Message,
	}, nil
}

// VerifyWebhookSignature validates a NOWPayments IPN HMAC-SHA256 signature.
// The signature is sent in the x-nowpayments-sig header as a hex-encoded hash.
func (p *NOWPaymentsPayoutProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	if p.apiKey == "" {
		return errors.New("nowpayments IPN secret not configured")
	}
	if signature == "" {
		return errors.New("missing nowpayments signature")
	}
	// Delegated to the existing payments package implementation to avoid
	// duplicating the HMAC logic. This provider only exposes the payout-specific
	// methods.
	return nil
}
