package payments

import (
	"context"
	"fmt"
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
	Provider  string
	EventID   string
	Payload   map[string]interface{}
	RawPayload []byte
}

// ---------------------------------------------------------------------------
// Paystack Provider (stub)
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

func (p *PaystackProvider) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	// TODO: Implement Paystack payment initiation
	return PaymentResponse{}, fmt.Errorf("paystack provider not implemented")
}

func (p *PaystackProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	// TODO: Implement Paystack webhook signature verification
	return fmt.Errorf("paystack webhook verification not implemented")
}

func (p *PaystackProvider) ProcessWebhookEvent(ctx context.Context, event WebhookEvent) error {
	// TODO: Implement Paystack webhook event processing
	return fmt.Errorf("paystack webhook processing not implemented")
}

// ---------------------------------------------------------------------------
// Stripe Provider (stub)
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
	// TODO: Implement Stripe payment initiation
	return PaymentResponse{}, fmt.Errorf("stripe provider not implemented")
}

func (p *StripeProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	// TODO: Implement Stripe webhook signature verification
	return fmt.Errorf("stripe webhook verification not implemented")
}

func (p *StripeProvider) ProcessWebhookEvent(ctx context.Context, event WebhookEvent) error {
	// TODO: Implement Stripe webhook event processing
	return fmt.Errorf("stripe webhook processing not implemented")
}

// ---------------------------------------------------------------------------
// NOWPayments Provider (stub)
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
	// TODO: Implement NOWPayments payment initiation
	return PaymentResponse{}, fmt.Errorf("nowpayments provider not implemented")
}

func (p *NOWPaymentsProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	// TODO: Implement NOWPayments webhook signature verification
	return fmt.Errorf("nowpayments webhook verification not implemented")
}

func (p *NOWPaymentsProvider) ProcessWebhookEvent(ctx context.Context, event WebhookEvent) error {
	// TODO: Implement NOWPayments webhook event processing
	return fmt.Errorf("nowpayments webhook processing not implemented")
}

// ---------------------------------------------------------------------------
// Manual Transfer Provider (stub)
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