package payments

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
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Provider defines the interface for payment providers.
// Each provider must implement payment initiation, webhook verification,
// webhook event processing, and provider-side refunds.
type Provider interface {
	Name() string
	InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error)
	VerifyWebhookSignature(payload []byte, signature string) error
	ProcessWebhookEvent(ctx context.Context, event WebhookEvent) error
	// RefundPayment issues a provider-side refund for a captured payment.
	// providerRef is the provider's transaction reference, amountMinor the
	// refund amount in minor units (kobo/cents). Returns the provider's
	// refund reference. Providers without a refund API return
	// ErrRefundNotSupported so the caller keeps the refund pending for
	// manual review instead of failing the whole flow.
	RefundPayment(ctx context.Context, providerRef string, amountMinor int64) (string, error)
}

// ErrRefundNotSupported is returned by Provider.RefundPayment when the
// provider has no refund API (NOWPayments invoices, manual transfer).
// Callers treat this as "leave pending for manual review", not as a failure.
var ErrRefundNotSupported = errors.New("provider does not support API refunds")

// PaymentRequest holds parameters for initiating a payment.
type PaymentRequest struct {
	UserID       string
	Email        string // Required by Paystack /transaction/initialize
	Amount       int64
	Currency     string
	PurposeType  string
	PurposeID    string
	Metadata     map[string]interface{}
	DiscountCode string
	// ReturnURL is where the provider sends the user after checkout —
	// Paystack callback_url, Stripe success/cancel URL.
	ReturnURL string
}

// PaymentResponse holds the result of initiating a payment.
type PaymentResponse struct {
	PaymentID   string
	CheckoutURL string
	Reference   string
}

// WebhookEvent represents a parsed webhook event from a payment provider.
type WebhookEvent struct {
	Provider    string
	EventID     string
	EventType   string // provider event type (charge.succeeded, payment_intent.succeeded, etc.)
	ProviderRef string // transaction reference matching payments.provider_ref
	Payload     map[string]interface{}
	RawPayload  []byte
}

// SubscriptionProvider is the subset of Provider capabilities needed for
// recurring billing (JEOS §17). Only Stripe and Paystack implement it —
// NOWPayments, manual transfer, and Credit have no recurring-charge mechanism.
type SubscriptionProvider interface {
	Provider
	// CreateSubscriptionCheckout starts a hosted recurring checkout and
	// returns the URL the user is redirected to. No local subscription row
	// exists until the provider's webhook confirms the first charge.
	CreateSubscriptionCheckout(ctx context.Context, req SubscriptionRequest) (PaymentResponse, error)
	// CancelSubscription schedules cancellation at the end of the current
	// billing period (Stripe) or stops renewals (Paystack).
	CancelSubscription(ctx context.Context, providerSubID string) error
	// ReactivateSubscription undoes a pending cancellation.
	ReactivateSubscription(ctx context.Context, providerSubID string) error
}

// SubscriptionRequest carries everything a provider needs to open a
// recurring-billing checkout.
type SubscriptionRequest struct {
	UserID    string
	Email     string
	Plan      string // local plan code (pro_monthly, business_yearly, …)
	Amount    int64  // smallest currency unit; Paystack requires it on initialize
	Currency  string
	PriceCode string // Stripe Price ID or Paystack plan_code (from settings)
	ReturnURL string // where the provider sends the user after checkout
	Metadata  map[string]interface{}
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

// Name returns the provider name.
func (p *PaystackProvider) Name() string {
	return ProviderPaystack
}

// paystackInitRequest is the JSON body for POST /initialize.
type paystackInitRequest struct {
	Amount      int64                  `json:"amount"` // in kobo
	Email       string                 `json:"email"`
	Currency    string                 `json:"currency,omitempty"`
	Reference   string                 `json:"reference,omitempty"`
	Plan        string                 `json:"plan,omitempty"` // plan_code — makes the charge recurring
	CallbackURL string                 `json:"callback_url,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// paystackInitResponse is the Paystack API response for /initialize.
type paystackInitResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		AuthorizationURL string `json:"authorization_url"`
		AccessCode       string `json:"access_code"`
		Reference        string `json:"reference"`
	} `json:"data"`
}

// InitiatePayment creates a Paystack checkout session.
func (p *PaystackProvider) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	if p.secretKey == "" {
		return PaymentResponse{}, errors.New("paystack secret key not configured")
	}

	payload := paystackInitRequest{
		Amount:      req.Amount,
		Email:       req.Email,
		Currency:    req.Currency,
		Metadata:    req.Metadata,
		CallbackURL: req.ReturnURL,
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

	defer func() { _ = resp.Body.Close() }()

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

// CreateSubscriptionCheckout opens a Paystack checkout that creates a
// subscription: /transaction/initialize with a plan code makes Paystack
// bill the customer on the plan's interval and fire subscription.* webhooks.
// Ref: JEOS §17 (Recurring Subscription Lifecycle).
func (p *PaystackProvider) CreateSubscriptionCheckout(ctx context.Context, req SubscriptionRequest) (PaymentResponse, error) {
	if p.secretKey == "" {
		return PaymentResponse{}, errors.New("paystack secret key not configured")
	}

	if req.PriceCode == "" {
		return PaymentResponse{}, errors.New("paystack plan code is required")
	}

	payload := paystackInitRequest{
		Amount:      req.Amount,
		Email:       req.Email,
		Currency:    req.Currency,
		Plan:        req.PriceCode,
		CallbackURL: req.ReturnURL,
		Metadata:    req.Metadata,
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

	defer func() { _ = resp.Body.Close() }()

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

// paystackSubscriptionResponse is the subset of GET /subscription/{code}
// needed for lifecycle management — email_token is required by the
// disable/enable endpoints.
type paystackSubscriptionResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		SubscriptionCode string `json:"subscription_code"`
		EmailToken       string `json:"email_token"`
		Status           string `json:"status"`
		NextPaymentDate  string `json:"next_payment_date"`
	} `json:"data"`
}

// paystackEmailToken fetches the subscription's email_token — Paystack's
// disable/enable endpoints authenticate with (code, token), not just the
// secret key.
func (p *PaystackProvider) paystackEmailToken(ctx context.Context, code string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.paystack.co/subscription/"+code, nil)
	if err != nil {
		return "", fmt.Errorf("creating paystack request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("paystack fetch subscription: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading paystack response: %w", err)
	}

	var apiResp paystackSubscriptionResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("parsing paystack response: %w", err)
	}

	if !apiResp.Status {
		return "", fmt.Errorf("paystack error: %s", apiResp.Message)
	}

	if apiResp.Data.EmailToken == "" {
		return "", errors.New("paystack subscription has no email_token")
	}

	return apiResp.Data.EmailToken, nil
}

// paystackSubscriptionAction posts to /subscription/{disable,enable} with
// the subscription code + email token pair Paystack requires.
func (p *PaystackProvider) paystackSubscriptionAction(ctx context.Context, code, action string) error {
	token, err := p.paystackEmailToken(ctx, code)
	if err != nil {
		return err
	}

	body, err := json.Marshal(map[string]string{"code": code, "token": token})
	if err != nil {
		return fmt.Errorf("marshaling paystack request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.paystack.co/subscription/"+action, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating paystack request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("paystack %s: %w", action, err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading paystack response: %w", err)
	}

	var apiResp struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("parsing paystack response: %w", err)
	}

	if !apiResp.Status {
		return fmt.Errorf("paystack error: %s", apiResp.Message)
	}

	return nil
}

// CancelSubscription stops Paystack renewals. Paystack has no native
// cancel-at-period-end — the local row keeps status active until
// current_period_end so the entitlement is unaffected (JEOS §17).
func (p *PaystackProvider) CancelSubscription(ctx context.Context, providerSubID string) error {
	return p.paystackSubscriptionAction(ctx, providerSubID, "disable")
}

// ReactivateSubscription re-enables a disabled Paystack subscription.
func (p *PaystackProvider) ReactivateSubscription(ctx context.Context, providerSubID string) error {
	return p.paystackSubscriptionAction(ctx, providerSubID, "enable")
}

// VerifyWebhookSignature validates the Paystack webhook signature.
// The signature is sent in the X-Paystack-Signature header as a hex-encoded hash.
func (p *PaystackProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	if p.secretKey == "" {
		return errors.New("paystack secret key not configured")
	}

	if signature == "" {
		return errors.New("missing paystack webhook signature")
	}

	mac := hmac.New(sha256.New, []byte(p.secretKey))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return errors.New("invalid paystack webhook signature")
	}

	return nil
}

// ProcessWebhookEvent processes a Paystack webhook event.
func (p *PaystackProvider) ProcessWebhookEvent(_ context.Context, event WebhookEvent) error {
	// Paystack event processing delegates to the service layer which
	// updates payment status based on event.Data.Status.
	slog.Info("paystack webhook event processed",
		"event_id", event.EventID,
		"event_type", event.Payload["event"])

	return nil
}

// RefundPayment issues a provider-side refund via POST /refund.
// amountMinor is passed through in minor units (kobo), matching the
// Paystack API contract.
func (p *PaystackProvider) RefundPayment(ctx context.Context, providerRef string, amountMinor int64) (string, error) {
	if p.secretKey == "" {
		return "", errors.New("paystack secret key not configured")
	}

	if providerRef == "" {
		return "", errors.New("provider reference is required")
	}

	payload := map[string]any{"transaction": providerRef}
	if amountMinor > 0 {
		payload["amount"] = amountMinor
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshaling paystack refund request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.paystack.co/refund", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating paystack refund request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("paystack refund: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading paystack refund response: %w", err)
	}

	var apiResp struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("parsing paystack refund response: %w", err)
	}

	if !apiResp.Status {
		return "", fmt.Errorf("paystack refund error: %s", apiResp.Message)
	}

	return apiResp.Data.ID, nil
}

// ---------------------------------------------------------------------------
// Stripe Provider
// ---------------------------------------------------------------------------

// StripeProvider implements the Provider interface for Stripe.
type StripeProvider struct {
	secretKey     string
	webhookSecret string
}

// NewStripeProvider creates a new Stripe provider.
func NewStripeProvider(secretKey string) *StripeProvider {
	return &StripeProvider{secretKey: secretKey}
}

// SetWebhookSecret configures the webhook signing secret for signature verification.
func (p *StripeProvider) SetWebhookSecret(secret string) {
	p.webhookSecret = secret
}

// Name returns the provider name.
func (p *StripeProvider) Name() string {
	return ProviderStripe
}

// stripeCheckoutRequest is the form-encoded body for POST /v1/checkout/sessions.
type stripeCheckoutResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// InitiatePayment creates a Stripe Checkout Session via the REST API.
func (p *StripeProvider) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	if p.secretKey == "" {
		return PaymentResponse{}, errors.New("stripe secret key not configured")
	}

	body := fmt.Sprintf(
		"mode=payment&line_items[0][price_data][currency]=%s&line_items[0][price_data][product_data][name]=Payment&line_items[0][price_data][unit_amount]=%d&line_items[0][quantity]=1",
		strings.ToLower(req.Currency),
		req.Amount,
	)

	if req.ReturnURL != "" {
		body += "&success_url=" + url.QueryEscape(req.ReturnURL)
		body += "&cancel_url=" + url.QueryEscape(req.ReturnURL)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(body))
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("creating stripe request: %w", err)
	}

	httpReq.SetBasicAuth(p.secretKey, "")
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("stripe checkout: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("reading stripe response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return PaymentResponse{}, fmt.Errorf("stripe error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var session stripeCheckoutResponse
	if err := json.Unmarshal(respBody, &session); err != nil {
		return PaymentResponse{}, fmt.Errorf("parsing stripe response: %w", err)
	}

	return PaymentResponse{
		PaymentID:   session.ID,
		CheckoutURL: session.URL,
		Reference:   session.ID,
	}, nil
}

// CreateSubscriptionCheckout opens a Stripe Checkout Session in subscription
// mode against a pre-created Price (payments.stripe.price.* settings keys).
// user_id + plan land on the Subscription via subscription_data.metadata so
// lifecycle webhooks can resolve the owner. Ref: JEOS §17.
func (p *StripeProvider) CreateSubscriptionCheckout(ctx context.Context, req SubscriptionRequest) (PaymentResponse, error) {
	if p.secretKey == "" {
		return PaymentResponse{}, errors.New("stripe secret key not configured")
	}

	if req.PriceCode == "" {
		return PaymentResponse{}, errors.New("stripe price ID is required")
	}

	// Metadata is set on both the session and the subscription: the
	// checkout.session.completed webhook exposes session metadata on
	// data.object.metadata, while customer.subscription.* events expose
	// subscription_data.metadata. Both are read by stripeActivate.
	form := fmt.Sprintf(
		"mode=subscription&line_items[0][price]=%s&line_items[0][quantity]=1"+
			"&metadata[user_id]=%s&metadata[plan]=%s"+
			"&subscription_data[metadata][user_id]=%s&subscription_data[metadata][plan]=%s",
		url.QueryEscape(req.PriceCode),
		url.QueryEscape(req.UserID),
		url.QueryEscape(req.Plan),
		url.QueryEscape(req.UserID),
		url.QueryEscape(req.Plan),
	)
	if req.Email != "" {
		form += "&customer_email=" + url.QueryEscape(req.Email)
	}

	if req.ReturnURL != "" {
		form += "&success_url=" + url.QueryEscape(req.ReturnURL+"?subscribed=1")
		form += "&cancel_url=" + url.QueryEscape(req.ReturnURL)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(form))
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("creating stripe request: %w", err)
	}

	httpReq.SetBasicAuth(p.secretKey, "")
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("stripe checkout: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("reading stripe response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return PaymentResponse{}, fmt.Errorf("stripe error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var session stripeCheckoutResponse
	if err := json.Unmarshal(respBody, &session); err != nil {
		return PaymentResponse{}, fmt.Errorf("parsing stripe response: %w", err)
	}

	return PaymentResponse{
		PaymentID:   session.ID,
		CheckoutURL: session.URL,
		Reference:   session.ID,
	}, nil
}

// stripeSetCancelAtPeriodEnd posts cancel_at_period_end to the subscription.
// Stripe keeps the subscription active until the period ends — the
// customer.subscription.deleted webhook arrives then, not now.
func (p *StripeProvider) stripeSetCancelAtPeriodEnd(ctx context.Context, subID string, cancel bool) error {
	if p.secretKey == "" {
		return errors.New("stripe secret key not configured")
	}

	body := fmt.Sprintf("cancel_at_period_end=%t", cancel)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/subscriptions/"+subID, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating stripe request: %w", err)
	}

	httpReq.SetBasicAuth(p.secretKey, "")
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("stripe subscription update: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading stripe response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("stripe error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// CancelSubscription schedules Stripe cancellation at period end.
func (p *StripeProvider) CancelSubscription(ctx context.Context, providerSubID string) error {
	return p.stripeSetCancelAtPeriodEnd(ctx, providerSubID, true)
}

// ReactivateSubscription clears a pending Stripe cancellation.
func (p *StripeProvider) ReactivateSubscription(ctx context.Context, providerSubID string) error {
	return p.stripeSetCancelAtPeriodEnd(ctx, providerSubID, false)
}

// VerifyWebhookSignature validates the Stripe webhook signature.
func (p *StripeProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	if p.webhookSecret == "" {
		return errors.New("stripe webhook secret not configured (JC_STRIPE_WEBHOOK_SECRET)")
	}

	if signature == "" {
		return errors.New("missing stripe webhook signature")
	}

	// Stripe-Signature header format: t=<timestamp>,v1=<signature>[v1=<signature>...]
	timestamp, sig, err := parseStripeSignatureHeader(signature)
	if err != nil {
		return fmt.Errorf("parsing stripe signature: %w", err)
	}

	// Reject payloads older than 5 minutes to prevent replay attacks.
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid stripe timestamp: %w", err)
	}

	if age := time.Now().Unix() - ts; age > 300 || age < -300 {
		return fmt.Errorf("stripe webhook timestamp too old or in the future (age=%ds)", age)
	}

	// Compute expected signature: HMAC-SHA256(webhook_secret, timestamp + "." + payload)
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return errors.New("invalid stripe webhook signature")
	}

	return nil
}

// parseStripeSignatureHeader extracts the timestamp and first v1 signature
// from the Stripe-Signature header value.
func parseStripeSignatureHeader(header string) (timestamp, signature string, err error) {
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}

		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			if signature == "" {
				signature = kv[1]
			}
		}
	}

	if timestamp == "" {
		return "", "", errors.New("missing timestamp (t=) in signature header")
	}

	if signature == "" {
		return "", "", errors.New("missing v1 signature in signature header")
	}

	return timestamp, signature, nil
}

// ProcessWebhookEvent processes a Stripe webhook event.
func (p *StripeProvider) ProcessWebhookEvent(_ context.Context, event WebhookEvent) error {
	slog.Info("stripe webhook event processed (placeholder)",
		"event_id", event.EventID)
	return nil
}

// RefundPayment issues a provider-side refund via POST /v1/refunds.
// amountMinor is passed through in minor units (cents), matching the
// Stripe API contract. providerRef is the charge/payment-intent id.
func (p *StripeProvider) RefundPayment(ctx context.Context, providerRef string, amountMinor int64) (string, error) {
	if p.secretKey == "" {
		return "", errors.New("stripe secret key not configured")
	}

	if providerRef == "" {
		return "", errors.New("provider reference is required")
	}

	form := "charge=" + url.QueryEscape(providerRef)
	if amountMinor > 0 {
		form += "&amount=" + strconv.FormatInt(amountMinor, 10)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/refunds", strings.NewReader(form))
	if err != nil {
		return "", fmt.Errorf("creating stripe refund request: %w", err)
	}

	httpReq.SetBasicAuth(p.secretKey, "")
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("stripe refund: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading stripe refund response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("stripe refund error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var refundResp struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(respBody, &refundResp); err != nil {
		return "", fmt.Errorf("parsing stripe refund response: %w", err)
	}

	return refundResp.ID, nil
}

// ---------------------------------------------------------------------------
// NOWPayments Provider
// ---------------------------------------------------------------------------

// NOWPaymentsProvider implements the Provider interface for NOWPayments.
type NOWPaymentsProvider struct {
	apiKey string
}

// NewNOWPaymentsProvider creates a new NOWPayments provider.
func NewNOWPaymentsProvider(apiKey string) *NOWPaymentsProvider {
	return &NOWPaymentsProvider{apiKey: apiKey}
}

// nowpaymentsInvoiceRequest is the JSON body for POST /v1/invoice.
type nowpaymentsInvoiceRequest struct {
	PriceAmount   float64 `json:"price_amount"`
	PriceCurrency string  `json:"price_currency"`
	OrderID       string  `json:"order_id"`
	OrderName     string  `json:"order_name,omitempty"`
}

// nowpaymentsInvoiceResponse is the NOWPayments API response for invoice creation.
type nowpaymentsInvoiceResponse struct {
	ID         string `json:"id"`
	InvoiceURL string `json:"invoice_url"`
	PaymentID  string `json:"payment_id"`
}

// Name returns the provider name.
func (p *NOWPaymentsProvider) Name() string {
	return ProviderNOWPayments
}

// InitiatePayment creates a NOWPayments invoice via the REST API.
func (p *NOWPaymentsProvider) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	if p.apiKey == "" {
		return PaymentResponse{}, errors.New("nowpayments API key not configured")
	}

	currency := req.Currency
	if currency == "" {
		currency = "USD"
	}

	// NOWPayments expects decimal amount, not kobo/cents.
	amount := float64(req.Amount) / 100.0

	payload := nowpaymentsInvoiceRequest{
		PriceAmount:   amount,
		PriceCurrency: strings.ToLower(currency),
		OrderID:       req.PurposeID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("marshaling nowpayments request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.nowpayments.io/v1/invoice", bytes.NewReader(body))
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("creating nowpayments request: %w", err)
	}

	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("nowpayments invoice: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("reading nowpayments response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return PaymentResponse{}, fmt.Errorf("nowpayments error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var invoice nowpaymentsInvoiceResponse
	if err := json.Unmarshal(respBody, &invoice); err != nil {
		return PaymentResponse{}, fmt.Errorf("parsing nowpayments response: %w", err)
	}

	return PaymentResponse{
		PaymentID:   invoice.PaymentID,
		CheckoutURL: invoice.InvoiceURL,
		Reference:   invoice.PaymentID,
	}, nil
}

// VerifyWebhookSignature validates the NOWPayments webhook signature.
// NOWPayments IPN signs HMAC-SHA512 of the key-sorted JSON payload (ALG-012).
func (p *NOWPaymentsProvider) VerifyWebhookSignature(payload []byte, signature string) error {
	if p.apiKey == "" {
		return errors.New("nowpayments IPN secret not configured (JC_NOWPAYMENTS_IPN_SECRET)")
	}

	return verifyNOWPaymentsSignature(payload, signature, p.apiKey)
}

// ProcessWebhookEvent processes a NOWPayments webhook event.
func (p *NOWPaymentsProvider) ProcessWebhookEvent(_ context.Context, event WebhookEvent) error {
	slog.Info("nowpayments webhook event processed (placeholder)",
		"event_id", event.EventID)
	return nil
}

// RefundPayment is unsupported: NOWPayments invoices have no refund API.
// Returns ErrRefundNotSupported so the caller keeps the refund pending
// for manual review instead of failing the flow.
func (p *NOWPaymentsProvider) RefundPayment(_ context.Context, _ string, _ int64) (string, error) {
	return "", ErrRefundNotSupported
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

// Name returns the provider name.
func (p *ManualTransferProvider) Name() string {
	return ProviderManualTransfer
}

// InitiatePayment creates a manual transfer payment reference.
func (p *ManualTransferProvider) InitiatePayment(_ context.Context, _ PaymentRequest) (PaymentResponse, error) {
	// Manual transfers don't initiate a checkout URL
	// They return reference code and instructions
	return PaymentResponse{
		PaymentID: "",
		Reference: "",
	}, nil
}

// VerifyWebhookSignature is a no-op for manual transfers.
func (p *ManualTransferProvider) VerifyWebhookSignature(_ []byte, _ string) error {
	// Manual transfers don't have webhooks
	return nil
}

// ProcessWebhookEvent is a no-op for manual transfers.
func (p *ManualTransferProvider) ProcessWebhookEvent(_ context.Context, _ WebhookEvent) error {
	// Manual transfers don't have webhooks
	return nil
}

// RefundPayment is unsupported: manual transfers settle off-platform.
// Returns ErrRefundNotSupported so the caller keeps the refund pending
// for manual review instead of failing the flow.
func (p *ManualTransferProvider) RefundPayment(_ context.Context, _ string, _ int64) (string, error) {
	return "", ErrRefundNotSupported
}
