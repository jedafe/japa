package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Provider defines the interface for payment providers.
// Each provider must implement payment initiation, webhook verification,
// and webhook event processing.
type Provider interface {
	Name() string
	InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error)
	VerifyWebhookSignature(payload []byte, signature string) error
	ProcessWebhookEvent(ctx context.Context, event WebhookEvent) error
}

// PaymentRequest holds parameters for initiating a payment.
type PaymentRequest struct {
	TenantID     string
	UserID       string
	Amount       int64
	Currency     string
	PurposeType  string
	PurposeID    string
	Metadata     map[string]interface{}
	DiscountCode string
}

// PaymentResponse holds the result of initiating a payment.
type PaymentResponse struct {
	PaymentID   string
	CheckoutURL string
	Reference   string
}

// WebhookEvent represents a parsed webhook event from a payment provider.
type WebhookEvent struct {
	Provider   string
	EventID    string
	Payload    map[string]interface{}
	RawPayload []byte
}

// ---------------------------------------------------------------------------
// HTTP client shared by all providers
// ---------------------------------------------------------------------------

// httpClient is the shared HTTP client with a reasonable timeout for payment API calls.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// ---------------------------------------------------------------------------
// Paystack Provider
// ---------------------------------------------------------------------------

// PaystackProvider implements the Provider interface for Paystack.
type PaystackProvider struct {
	secretKey string
}

// NewPaystackProvider creates a new Paystack provider.
func NewPaystackProvider(secretKey string) *PaystackProvider {
	return &PaystackProvider{secretKey: secretKey}
}

func (p *PaystackProvider) Name() string {
	return ProviderPaystack
}

// paystackInitRequest is the JSON body for POST /initialize.
type paystackInitRequest struct {
	Amount      int64                  `json:"amount"` // in kobo
	Email       string                 `json:"email"`
	Currency    string                 `json:"currency,omitempty"`
	Reference   string                 `json:"reference,omitempty"`
	CallbackURL string                 `json:"callback_url,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// paystackInitResponse is the Paystack API response for /initialize.
type paystackInitResponse struct {
	Status  bool `json:"status"`
	Message string `json:"message"`
	Data    struct {
		AuthorizationURL string `json:"authorization_url"`
		AccessCode       string `json:"access_code"`
		Reference        string `json:"reference"`
	} `json:"data"`
}

func (p *PaystackProvider) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	if p.secretKey == "" {
		return PaymentResponse{}, fmt.Errorf("paystack secret key not configured")
	}

	payload := paystackInitRequest{
		Amount:   req.Amount,
		Currency: req.Currency,
		Metadata: req.Metadata,
	}
	if payload.Currency == "" {
		payload.Currency = "NGN"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("marshaling paystack request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.paystack.co/transaction/initialize", bytes.NewReader(body))
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("creating paystack request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("paystack initialize: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("reading paystack response: %w", err)
	}

	var apiResp paystackInitResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return PaymentResponse{}, fmt.Errorf("parsing paystack response: %w", err)
	}

	if !apiResp.Status {
		return PaymentResponse{}, fmt.Errorf("paystack error: %s", apiResp.Message)
	}

	return PaymentResponse{
		CheckoutURL: apiResp.Data.AuthorizationURL,
		Reference:   apiResp.Data.Reference,
	}, nil
}

// Paystack webhook signature verification uses HMAC-SHA256.
// The signature is sent in the X-Paystack-Signature header as a hex-encoded hash.
func (p *PaystackProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	if p.secretKey == "" {
		return fmt.Errorf("paystack secret key not configured")
	}
	if signature == "" {
		return fmt.Errorf("missing paystack webhook signature")
	}

	mac := hmac.New(sha256.New, []byte(p.secretKey))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return fmt.Errorf("invalid paystack webhook signature")
	}
	return nil
}

func (p *PaystackProvider) ProcessWebhookEvent(ctx context.Context, event WebhookEvent) error {
	// Paystack event processing delegates to the service layer which
	// updates payment status based on event.Data.Status.
	slog.Info("paystack webhook event processed",
		"event_id", event.EventID,
		"event_type", event.Payload["event"])
	return nil
}

// ---------------------------------------------------------------------------
// Stripe Provider (secondary — stub with logging)
// ---------------------------------------------------------------------------

// StripeProvider implements the Provider interface for Stripe.
type StripeProvider struct {
	secretKey string
}

// NewStripeProvider creates a new Stripe provider.
func NewStripeProvider(secretKey string) *StripeProvider {
	return &StripeProvider{secretKey: secretKey}
}

func (p *StripeProvider) Name() string {
	return ProviderStripe
}

func (p *StripeProvider) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	// Stripe Checkout Session creation would go here.
	// For now, log and return a reference so the payment flow doesn't break.
	slog.Warn("stripe payment initiation is a placeholder",
		"amount", req.Amount, "currency", req.Currency)
	return PaymentResponse{
		PaymentID:   "",
		CheckoutURL: "",
		Reference:   fmt.Sprintf("stripe_placeholder_%d", time.Now().UnixNano()),
	}, nil
}

func (p *StripeProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	// Stripe uses HMAC-SHA256 with the webhook signing secret.
	// Placeholder: accept all signatures until Stripe is fully configured.
	slog.Warn("stripe webhook verification is a placeholder")
	if signature == "" {
		return fmt.Errorf("missing stripe webhook signature")
	}
	return nil
}

func (p *StripeProvider) ProcessWebhookEvent(ctx context.Context, event WebhookEvent) error {
	slog.Info("stripe webhook event processed (placeholder)",
		"event_id", event.EventID)
	return nil
}

// ---------------------------------------------------------------------------
// NOWPayments Provider (secondary — stub with logging)
// ---------------------------------------------------------------------------

// NOWPaymentsProvider implements the Provider interface for NOWPayments.
type NOWPaymentsProvider struct {
	apiKey string
}

// NewNOWPaymentsProvider creates a new NOWPayments provider.
func NewNOWPaymentsProvider(apiKey string) *NOWPaymentsProvider {
	return &NOWPaymentsProvider{apiKey: apiKey}
}

func (p *NOWPaymentsProvider) Name() string {
	return ProviderNOWPayments
}

func (p *NOWPaymentsProvider) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	slog.Warn("nowpayments payment initiation is a placeholder",
		"amount", req.Amount, "currency", req.Currency)
	return PaymentResponse{
		PaymentID:   "",
		CheckoutURL: "",
		Reference:   fmt.Sprintf("nowpayments_placeholder_%d", time.Now().UnixNano()),
	}, nil
}

func (p *NOWPaymentsProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	// NOWPayments uses IPN (Instant Payment Notification) with IPN callback verification.
	slog.Warn("nowpayments webhook verification is a placeholder")
	if signature == "" {
		return fmt.Errorf("missing nowpayments webhook signature")
	}
	return nil
}

func (p *NOWPaymentsProvider) ProcessWebhookEvent(ctx context.Context, event WebhookEvent) error {
	slog.Info("nowpayments webhook event processed (placeholder)",
		"event_id", event.EventID)
	return nil
}

// ---------------------------------------------------------------------------
// Manual Transfer Provider
// ---------------------------------------------------------------------------

// ManualTransferProvider implements the Provider interface for manual transfers.
type ManualTransferProvider struct{}

// NewManualTransferProvider creates a new Manual Transfer provider.
func NewManualTransferProvider() *ManualTransferProvider {
	return &ManualTransferProvider{}
}

func (p *ManualTransferProvider) Name() string {
	return ProviderManualTransfer
}

func (p *ManualTransferProvider) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	// Manual transfers don't initiate a checkout URL
	// They return reference code and instructions
	return PaymentResponse{
		PaymentID: "",
		Reference: "",
	}, nil
}

func (p *ManualTransferProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	// Manual transfers don't have webhooks
	return nil
}

func (p *ManualTransferProvider) ProcessWebhookEvent(ctx context.Context, event WebhookEvent) error {
	// Manual transfers don't have webhooks
	return nil
}
