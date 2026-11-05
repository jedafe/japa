// Package payout — Circle Payouts provider (SEC-W-009).
//
// Circle Payouts API: https://developers.circle.com — business account
// sends USDC to on-chain addresses. Unlike NOWPayments, Circle REQUIRES
// an idempotencyKey on payout creation — the withdrawal ID maps 1:1,
// giving server-side dedup a timeout/retry can't defeat.
//
// Env: JC_CIRCLE_API_KEY. Amounts are USD-denominated at Circle; chain
// comes from the payout method's metadata (metadata.chain: ETH|MATIC|
// SOL|TRX|BASE|ARB — default MATIC). Destination currency is USDC.
package payout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const circleAPIBase = "https://api.circle.com/v1/business"

// circleChains maps method-metadata chain values to Circle's chain codes.
var circleChains = map[string]string{
	"ETH": "ETH", "MATIC": "MATIC", "POLY": "MATIC", "SOL": "SOL",
	"TRX": "TRX", "BASE": "BASE", "ARB": "ARB", "AVAX": "AVAX",
	"OP": "OP", "NEAR": "NEAR", "XLM": "XLM",
}

// CirclePayoutProvider implements Provider + StatusProvider for Circle.
type CirclePayoutProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewCirclePayoutProvider builds a provider from the JC_CIRCLE_API_KEY
// bearer credential.
func NewCirclePayoutProvider(apiKey string) *CirclePayoutProvider {
	return &CirclePayoutProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *CirclePayoutProvider) Type() string { return "circle" }

// Supports reports whether Circle can handle the destination currency —
// USDC payouts only (a "usdc:<chain>" suffix selects the destination chain).
func (p *CirclePayoutProvider) Supports(currency string) bool {
	base := currency
	if i := strings.Index(currency, ":"); i > 0 {
		base = currency[:i]
	}
	return strings.EqualFold(base, "usdc")
}

// InitiatePayout creates a Circle payout. The withdrawal ID is sent as
// the REQUIRED idempotencyKey — retries return the same payout instead
// of duplicating real money (SEC-W-009 root fix).
func (p *CirclePayoutProvider) InitiatePayout(ctx context.Context, req PayoutRequest) (PayoutResponse, error) {
	if !p.Supports(req.Currency) {
		return PayoutResponse{}, &PayoutRejectedError{Provider: "circle", StatusCode: 400, Body: "circle pays USDC only"}
	}

	chain := "MATIC"
	if i := strings.Index(req.Currency, ":"); i > 0 {
		if c, ok := circleChains[strings.ToUpper(req.Currency[i+1:])]; ok {
			chain = c
		}
	}

	// Circle balances are USD — payout amounts are USD-denominated. When a
	// fiat context is present it must already be USD; a non-USD fiat can't
	// be converted at Circle, so it's a definitive rejection (not a guess).
	fiatMinor := req.AmountMinor
	if req.FiatCurrency != "" {
		if !strings.EqualFold(req.FiatCurrency, "usd") {
			return PayoutResponse{}, &PayoutRejectedError{
				Provider: "circle", StatusCode: 400,
				Body: "circle payouts require USD-denominated amounts; use a USD withdrawal currency",
			}
		}
		fiatMinor = req.FiatAmountMinor
	}
	usdMajor := fmt.Sprintf("%.2f", float64(fiatMinor)/100.0)

	payload := map[string]interface{}{
		"idempotencyKey": req.WithdrawalID,
		"destination": map[string]string{
			"type":    "blockchain",
			"address": req.Address,
			"chain":   chain,
		},
		"amount": map[string]string{"amount": usdMajor, "currency": "USD"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("marshaling payout request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		circleAPIBase+"/payouts", bytes.NewReader(body))
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("building payout request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("circle payout request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	// Definitive rejection (4xx) → refund + fail. Everything else is
	// ambiguous → uncertain path, never auto-refund.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return PayoutResponse{}, &PayoutRejectedError{
			Provider: "circle", StatusCode: resp.StatusCode, Body: string(respBody),
		}
	}
	if resp.StatusCode >= 500 {
		return PayoutResponse{}, fmt.Errorf("circle payout: status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Data struct {
			ID            string `json:"id"`
			Status        string `json:"status"`
			TransactionID string `json:"transactionRef"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return PayoutResponse{}, fmt.Errorf("decoding payout response: %w", err)
	}

	return PayoutResponse{
		ProviderRef: result.Data.ID,
		Status:      normalizeCircleStatus(result.Data.Status),
		Message:     result.Data.TransactionID,
	}, nil
}

// PayoutStatus polls GET /payouts/{id} — reconcile task settles or
// refunds from this (ALG-005).
func (p *CirclePayoutProvider) PayoutStatus(ctx context.Context, providerRef string) (PayoutResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		circleAPIBase+"/payouts/"+providerRef, nil)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("building status request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("circle status request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return PayoutResponse{}, fmt.Errorf("circle status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return PayoutResponse{}, fmt.Errorf("decoding status: %w", err)
	}

	return PayoutResponse{
		ProviderRef: result.Data.ID,
		Status:      normalizeCircleStatus(result.Data.Status),
	}, nil
}

func (p *CirclePayoutProvider) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// normalizeCircleStatus maps Circle payout states onto japa.to's
// withdrawal vocabulary.
func normalizeCircleStatus(s string) string {
	switch strings.ToLower(s) {
	case "pending":
		return "processing"
	case "complete", "confirmed":
		return "completed"
	case "failed":
		return "failed"
	default:
		return "processing"
	}
}

// PayoutBalance returns available USD/USDC balances from GET /balances.
func (p *CirclePayoutProvider) PayoutBalance(ctx context.Context) (map[string]float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, circleAPIBase+"/balances", nil)
	if err != nil {
		return nil, fmt.Errorf("building balance request: %w", err)
	}
	p.setHeaders(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("circle balance request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("circle balance %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Data struct {
			Available []struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			} `json:"available"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding balance: %w", err)
	}
	out := make(map[string]float64, len(result.Data.Available))
	for _, b := range result.Data.Available {
		if v, err := strconv.ParseFloat(b.Amount, 64); err == nil {
			out[b.Currency] = v
		}
	}
	return out, nil
}
