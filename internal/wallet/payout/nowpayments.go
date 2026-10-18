package payout

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// NOWPaymentsPayoutProvider implements Provider for NOWPayments crypto payouts.
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
// Uses the NOWPayments Payout API: POST /v1/payout.
// See https://documenter.getpostman.com/view/7907941/2s93JusNJt
type NOWPaymentsPayoutProvider struct {
	apiKey     string
	httpClient *http.Client
	supported  map[string]bool
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

	// NOWPayments expects decimal major units on the wire. Convert from
	// minor units at this edge, rounded to 8 decimals (crypto precision)
	// so binary-float dust cannot drift the settled amount.
	amountMajor := math.Round(float64(req.AmountMinor)/100.0*1e8) / 1e8

	// Use fiat_amount when the amount is in fiat; otherwise send amount in crypto.
	if req.Currency == strings.ToUpper(req.Currency) && len(req.Currency) == 3 {
		payload.FiatAmount = amountMajor
		payload.FiatCurrency = strings.ToLower(req.Currency)
	} else {
		payload.Amount = amountMajor
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

// PayoutStatus implements StatusProvider (ALG-005) via GET /v1/payout/{id}.
// NOWPayments payout statuses: waiting, processing, sending, finished,
// failed, rejected. The reconcile task maps these onto the wallet
// withdrawal state machine (pending/processing/completed/failed).
func (p *NOWPaymentsPayoutProvider) PayoutStatus(ctx context.Context, providerRef string) (PayoutResponse, error) {
	if p.apiKey == "" {
		return PayoutResponse{}, errors.New("nowpayments API key not configured")
	}

	providerRef = strings.TrimSpace(providerRef)
	if providerRef == "" {
		return PayoutResponse{}, errors.New("provider ref is required")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.nowpayments.io/v1/payout/"+providerRef, nil)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("creating nowpayments status request: %w", err)
	}

	httpReq.Header.Set("x-api-key", p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("nowpayments payout status: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("reading nowpayments status response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return PayoutResponse{}, fmt.Errorf("nowpayments payout status error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var pr struct {
		PayoutID string `json:"payout_id"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &pr); err != nil {
		return PayoutResponse{}, fmt.Errorf("parsing nowpayments status response: %w", err)
	}

	return PayoutResponse{
		ProviderRef: providerRef,
		Status:      mapNOWPaymentsPayoutStatus(pr.Status),
	}, nil
}

// mapNOWPaymentsPayoutStatus normalizes NOWPayments payout statuses onto the
// wallet withdrawal state machine.
func mapNOWPaymentsPayoutStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "finished":
		return "completed"
	case "failed", "rejected":
		return "failed"
	case "waiting", "processing", "sending":
		return "processing"
	default:
		return "pending"
	}
}

// VerifyWebhookSignature validates a NOWPayments IPN signature. Their scheme
// (ALG-012) is HMAC-SHA512 of the payload canonicalized with object keys
// sorted recursively — not HMAC-SHA256 of the raw body. The apiKey here must
// be the IPN secret (JC_NOWPAYMENTS_IPN_SECRET).
func (p *NOWPaymentsPayoutProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	if p.apiKey == "" {
		return errors.New("nowpayments IPN secret not configured")
	}

	if signature == "" {
		return errors.New("missing nowpayments signature")
	}

	canonical, err := canonicalizeSortedJSON(payload)
	if err != nil {
		return fmt.Errorf("canonicalizing ipn payload: %w", err)
	}

	mac := hmac.New(sha512.New, []byte(p.apiKey))
	mac.Write(canonical)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(strings.TrimSpace(signature)), []byte(expected)) {
		return errors.New("invalid nowpayments webhook signature")
	}

	return nil
}

// canonicalizeSortedJSON re-serializes a JSON document with every object's
// keys sorted alphabetically (recursive), matching the NOWPayments IPN
// signing convention.
func canonicalizeSortedJSON(raw []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := writeSortedJSON(&buf, v); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func writeSortedJSON(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}

			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')

			if err := writeSortedJSON(buf, t[k]); err != nil {
				return err
			}
		}

		buf.WriteByte('}')

		return nil
	case []any:
		buf.WriteByte('[')

		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}

			if err := writeSortedJSON(buf, e); err != nil {
				return err
			}
		}

		buf.WriteByte(']')

		return nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}

		buf.Write(b)

		return nil
	}
}
