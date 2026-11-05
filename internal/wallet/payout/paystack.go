// Package payout — Paystack Transfers provider (SEC-W-NGN).
//
// The cheapest NGN payout rail: flat ~₦10-100 per transfer vs ~2% on a
// crypto route. Transfers API authenticates with the SAME secret key as
// inbound payments (Bearer JC_PAYSTACK_SECRET_KEY).
//
//   - recipient:  POST /transferrecipient (type "nuban") — created at
//     method-save time via RecipientCreator and cached in method metadata
//   - payout:     POST /transfer — `reference` is our withdrawal ID and
//     is UNIQUE on Paystack's side, giving server-enforced idempotency:
//     a retried create can't duplicate a real transfer (SEC-W-009)
//   - status:     GET /transfer/{id_or_code}
//   - verify acct: GET /bank/resolve — confirms account number + returns
//     the real account name before a method is ever saved
//   - webhooks:   transfer.* events signed with x-paystack-signature
//     (HMAC-SHA256, same secret key)
//
// Caveat: Paystack "transfers with OTP" must be disabled in the Paystack
// dashboard for API-initiated payouts, else transfers stall awaiting an
// OTP the API can't supply.
package payout

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const paystackAPIBase = "https://api.paystack.co"

// PaystackPayoutProvider implements Provider + StatusProvider +
// WebhookVerifier + WebhookEventParser for NGN bank payouts.
type PaystackPayoutProvider struct {
	secretKey  string
	httpClient *http.Client
}

// NewPaystackPayoutProvider builds a provider from JC_PAYSTACK_SECRET_KEY.
func NewPaystackPayoutProvider(secretKey string) *PaystackPayoutProvider {
	return &PaystackPayoutProvider{
		secretKey:  secretKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *PaystackPayoutProvider) Type() string { return "paystack" }

// Supports reports whether this provider can handle the currency — NGN
// bank payouts only.
func (p *PaystackPayoutProvider) Supports(currency string) bool {
	return strings.EqualFold(currency, "ngn")
}

// InitiatePayout creates a Paystack transfer. req.WithdrawalID is sent
// as `reference` — Paystack dedupes on it, so a retried create can't
// double-pay (SEC-W-009). Amount is already NGN kobo in FiatAmountMinor/
// AmountMinor — no conversion needed.
func (p *PaystackPayoutProvider) InitiatePayout(ctx context.Context, req PayoutRequest) (PayoutResponse, error) {
	if req.FiatCurrency != "" && !strings.EqualFold(req.FiatCurrency, "ngn") {
		return PayoutResponse{}, &PayoutRejectedError{
			Provider: "paystack", StatusCode: 400,
			Body: "paystack pays NGN only",
		}
	}
	if !p.Supports(req.Currency) {
		return PayoutResponse{}, &PayoutRejectedError{
			Provider: "paystack", StatusCode: 400, Body: "paystack pays NGN only",
		}
	}

	var meta struct {
		RecipientCode string `json:"recipient_code"`
		AccountName   string `json:"account_name"`
		BankCode      string `json:"bank_code"`
	}
	_ = json.Unmarshal(req.Metadata, &meta)

	// SEC-W-NGN lazy path: methods saved before recipient caching (or a
	// failed create) still pay out — create the recipient now and hand
	// it back for the wallet to persist via MethodMetadataUpdates.
	var metaUpdates map[string]interface{}
	if meta.RecipientCode == "" {
		name := meta.AccountName
		if name == "" {
			resolved, err := p.ResolveAccount(ctx, meta.BankCode, req.Address)
			if err != nil {
				return PayoutResponse{}, &PayoutRejectedError{
					Provider: "paystack", StatusCode: 400,
					Body: "account could not be verified: " + err.Error(),
				}
			}
			name = resolved
		}
		rc, err := p.CreateRecipient(ctx, name, req.Address, meta.BankCode)
		if err != nil {
			return PayoutResponse{}, fmt.Errorf("creating recipient: %w", err)
		}
		meta.RecipientCode = rc
		metaUpdates = map[string]interface{}{
			"recipient_code": rc,
			"account_name":   name,
		}
	}

	amountKobo := req.FiatAmountMinor
	if amountKobo <= 0 {
		amountKobo = req.AmountMinor
	}

	payload := map[string]interface{}{
		"source":    "balance",
		"amount":    amountKobo,
		"recipient": meta.RecipientCode,
		"reason":    "JapaConnect withdrawal " + req.WithdrawalID,
		"reference": req.WithdrawalID,
	}
	if req.ExtraID != "" {
		payload["reason"] = req.ExtraID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return PayoutResponse{}, fmt.Errorf("marshaling transfer request: %w", err)
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/transfer", bytes.NewReader(body))
	if err != nil {
		return PayoutResponse{}, err
	}

	// 4xx = definitive rejection → fail + refund. 5xx/timeout = ambiguous
	// (transfer may exist under our reference) → uncertain path.
	if status >= 400 && status < 500 {
		return PayoutResponse{}, &PayoutRejectedError{
			Provider: "paystack", StatusCode: status, Body: string(respBody),
		}
	}
	if status >= 500 {
		return PayoutResponse{}, fmt.Errorf("paystack transfer: status %d: %s", status, respBody)
	}

	var result struct {
		Data struct {
			ID           int64  `json:"id"`
			TransferCode string `json:"transfer_code"`
			Status       string `json:"status"`
			Reference    string `json:"reference"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return PayoutResponse{}, fmt.Errorf("decoding transfer response: %w", err)
	}

	providerRef := result.Data.TransferCode
	if providerRef == "" {
		providerRef = strconv.FormatInt(result.Data.ID, 10)
	}

	return PayoutResponse{
		ProviderRef:           providerRef,
		Status:                normalizePaystackStatus(result.Data.Status),
		Message:               result.Data.Reference,
		MethodMetadataUpdates: metaUpdates,
	}, nil
}

// DisableOTP requests transfer-OTP disablement — Paystack emails the
// business an OTP which must then be sent to FinalizeDisableOTP
// (SEC-W-NGN ops flow).
func (p *PaystackPayoutProvider) DisableOTP(ctx context.Context) (string, error) {
	respBody, status, err := p.do(ctx, http.MethodPost, "/transfer/disable_otp", nil)
	if err != nil {
		return "", err
	}
	var result struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(respBody, &result)
	if status >= 400 {
		return "", fmt.Errorf("paystack disable_otp failed (%d): %s", status, respBody)
	}
	return result.Message, nil
}

// FinalizeDisableOTP submits the OTP code sent to the business contact
// to complete disabling transfer OTP.
func (p *PaystackPayoutProvider) FinalizeDisableOTP(ctx context.Context, otp string) error {
	body, _ := json.Marshal(map[string]string{"otp": otp})
	respBody, status, err := p.do(ctx, http.MethodPost, "/transfer/finalize_disable_otp", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("paystack finalize_otp failed (%d): %s", status, respBody)
	}
	return nil
}

// PayoutBalance returns the spendable transfer balance (NGN kobo → ₦).
func (p *PaystackPayoutProvider) PayoutBalance(ctx context.Context) (map[string]float64, error) {
	respBody, status, err := p.do(ctx, http.MethodGet, "/balance", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("paystack balance %d: %s", status, respBody)
	}
	var result struct {
		Data []struct {
			Currency string `json:"currency"`
			Balance  int64  `json:"balance"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding balance: %w", err)
	}
	out := make(map[string]float64, len(result.Data))
	for _, b := range result.Data {
		out[b.Currency] = float64(b.Balance) / 100.0
	}
	return out, nil
}

// PayoutStatus polls GET /transfer/{id_or_code} — the reconcile task
// settles or refunds in-flight withdrawals from this (ALG-005).
func (p *PaystackPayoutProvider) PayoutStatus(ctx context.Context, providerRef string) (PayoutResponse, error) {
	respBody, status, err := p.do(ctx, http.MethodGet, "/transfer/"+providerRef, nil)
	if err != nil {
		return PayoutResponse{}, err
	}
	if status != http.StatusOK {
		return PayoutResponse{}, fmt.Errorf("paystack transfer status %d: %s", status, respBody)
	}

	var result struct {
		Data struct {
			TransferCode string `json:"transfer_code"`
			Status       string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return PayoutResponse{}, fmt.Errorf("decoding status: %w", err)
	}

	return PayoutResponse{
		ProviderRef: result.Data.TransferCode,
		Status:      normalizePaystackStatus(result.Data.Status),
	}, nil
}

// VerifyWebhookSignature validates x-paystack-signature. Paystack
// documents HMAC-SHA512 keyed with the secret key; the platform's
// existing inbound handler verifies with SHA256 — accept either so both
// implementations interoperate during any provider-side migration.
func (p *PaystackPayoutProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	for _, h := range []hash.Hash{sha512.New(), sha256.New()} {
		mac := hmac.New(func() hash.Hash { return h }, []byte(p.secretKey))
		mac.Write(payload)
		if hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(signature)) {
			return nil
		}
	}
	return fmt.Errorf("invalid paystack signature")
}

// ParsePayoutEvent extracts (providerRef, status) from a Paystack
// transfer.* webhook envelope {event, data:{transfer_code,status}}.
func (p *PaystackPayoutProvider) ParsePayoutEvent(payload []byte) (providerRef, status string, ok bool) {
	var evt struct {
		Event string `json:"event"`
		Data  struct {
			TransferCode string `json:"transfer_code"`
			ID           int64  `json:"id"`
			Status       string `json:"status"`
			Reference    string `json:"reference"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &evt); err != nil {
		return "", "", false
	}
	if !strings.HasPrefix(evt.Event, "transfer.") {
		return "", "", false
	}
	providerRef = evt.Data.TransferCode
	if providerRef == "" {
		providerRef = strconv.FormatInt(evt.Data.ID, 10)
	}
	if providerRef == "" {
		return "", "", false
	}
	return providerRef, normalizePaystackStatus(evt.Data.Status), true
}

// ResolveAccount verifies a NUBAN account against Paystack
// (/bank/resolve) and returns the real account name — called at
// method-save so a typo'd number can never become a payout destination.
func (p *PaystackPayoutProvider) ResolveAccount(ctx context.Context, bankCode, accountNumber string) (accountName string, err error) {
	q := url.Values{"account_number": {accountNumber}, "bank_code": {bankCode}}
	respBody, status, err := p.do(ctx, http.MethodGet, "/bank/resolve?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("paystack resolve failed (%d): %s", status, respBody)
	}
	var result struct {
		Data struct {
			AccountName string `json:"account_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || result.Data.AccountName == "" {
		return "", fmt.Errorf("account could not be verified")
	}
	return result.Data.AccountName, nil
}

// CreateRecipient registers a NUBAN recipient and returns its
// recipient_code (cached in method metadata at save time).
func (p *PaystackPayoutProvider) CreateRecipient(ctx context.Context, accountName, accountNumber, bankCode string) (string, error) {
	payload := map[string]string{
		"type":           "nuban",
		"name":           accountName,
		"account_number": accountNumber,
		"bank_code":      bankCode,
		"currency":       "NGN",
	}
	body, _ := json.Marshal(payload)
	respBody, status, err := p.do(ctx, http.MethodPost, "/transferrecipient", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	if status >= 400 {
		return "", fmt.Errorf("paystack recipient failed (%d): %s", status, respBody)
	}
	var result struct {
		Data struct {
			RecipientCode string `json:"recipient_code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || result.Data.RecipientCode == "" {
		return "", fmt.Errorf("recipient_code missing in response")
	}
	return result.Data.RecipientCode, nil
}

// PaystackBank is one entry in the bank list returned by ListBanks.
type PaystackBank struct {
	Name string `json:"name"`
	Code string `json:"code"`
	Slug string `json:"slug"`
}

// ListBanks returns supported banks for the method-creation picker.
// country is a Paystack country slug ("nigeria", "ghana", ...) —
// empty defaults to "nigeria" (the NGN rail).
func (p *PaystackPayoutProvider) ListBanks(ctx context.Context, country string) ([]PaystackBank, error) {
	if country == "" {
		country = "nigeria"
	}
	respBody, status, err := p.do(ctx, http.MethodGet, "/bank?country="+url.QueryEscape(country)+"&perPage=200", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("paystack banks %d: %s", status, respBody)
	}
	var result struct {
		Data []PaystackBank `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding bank list: %w", err)
	}
	return result.Data, nil
}

func (p *PaystackPayoutProvider) do(ctx context.Context, method, path string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, paystackAPIBase+path, body)
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("paystack request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("reading response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// normalizePaystackStatus maps Paystack transfer states onto the
// withdrawal vocabulary.
func normalizePaystackStatus(s string) string {
	switch strings.ToLower(s) {
	case "success":
		return "completed"
	case "failed", "reversed":
		return "failed"
	default: // pending, processing, otp
		return "processing"
	}
}
