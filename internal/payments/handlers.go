// Package payments — HTTP handlers for the Payment Core system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package payments

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for payment HTTP handlers.
type Handlers struct {
	Service             *Service
	StripeWebhookSecret string // JC_STRIPE_WEBHOOK_SECRET — Stripe webhook signing secret
	NOWPaymentsIPNKey   string // JC_NOWPAYMENTS_IPN_SECRET — NOWPayments IPN HMAC key
	PaystackSecretKey   string // Paystack secret key for webhook HMAC verification
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Pricing Plans & Payment Providers
// ---------------------------------------------------------------------------

// PricingPlan represents a subscription plan available to users.
type PricingPlan struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Features    []PlanFeature  `json:"features"`
	Monthly     PlanPricing    `json:"monthly"`
	Yearly      PlanPricing    `json:"yearly"`
	Highlighted bool           `json:"highlighted"`
}

// PlanPricing holds price information for a billing cycle.
type PlanPricing struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// PlanFeature describes a feature included in a plan.
type PlanFeature struct {
	Text      string `json:"text"`
	Included  bool   `json:"included"`
}

// PaymentProviderInfo describes an available payment provider.
type PaymentProviderInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// ListPricingPlans handles GET /api/v1/pricing/plans.
// Returns the available subscription plans with pricing details.
func (h *Handlers) ListPricingPlans(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Pricing plans are platform-wide configuration.
	// In the future, these could be stored in system_settings per tenant.
	_ = tenantID

	plans := []PricingPlan{
		{
			ID:          "free",
			Name:        "Free",
			Description: "Get started with core community features",
			Highlighted: false,
			Features: []PlanFeature{
				{Text: "Community forum access", Included: true},
				{Text: "Basic profile", Included: true},
				{Text: "5 posts per month", Included: true},
				{Text: "Public spaces", Included: true},
				{Text: "AI assistant (10 queries/day)", Included: true},
				{Text: "Priority support", Included: false},
				{Text: "Advanced analytics", Included: false},
				{Text: "Custom branding", Included: false},
			},
			Monthly: PlanPricing{Amount: 0, Currency: "NGN"},
			Yearly:  PlanPricing{Amount: 0, Currency: "NGN"},
		},
		{
			ID:          "pro",
			Name:        "Pro",
			Description: "For professionals building their diaspora network",
			Highlighted: true,
			Features: []PlanFeature{
				{Text: "Everything in Free", Included: true},
				{Text: "Unlimited posts", Included: true},
				{Text: "Professional profile", Included: true},
				{Text: "AI assistant (100 queries/day)", Included: true},
				{Text: "Priority support", Included: true},
				{Text: "Advanced analytics", Included: true},
				{Text: "Document verification", Included: true},
				{Text: "Custom branding", Included: false},
			},
			Monthly: PlanPricing{Amount: 500000, Currency: "NGN"},
			Yearly:  PlanPricing{Amount: 5000000, Currency: "NGN"},
		},
		{
			ID:          "business",
			Name:        "Business",
			Description: "For organisations and power users",
			Highlighted: false,
			Features: []PlanFeature{
				{Text: "Everything in Pro", Included: true},
				{Text: "Unlimited AI queries", Included: true},
				{Text: "Team management", Included: true},
				{Text: "API access", Included: true},
				{Text: "Dedicated support", Included: true},
				{Text: "Custom integrations", Included: true},
				{Text: "Advanced analytics", Included: true},
				{Text: "Custom branding", Included: true},
			},
			Monthly: PlanPricing{Amount: 1500000, Currency: "NGN"},
			Yearly:  PlanPricing{Amount: 15000000, Currency: "NGN"},
		},
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": plans})
}

// ListPaymentProviders handles GET /api/v1/payment-providers.
// Returns the available payment providers with their availability status.
func (h *Handlers) ListPaymentProviders(w http.ResponseWriter, r *http.Request) {
	providers := []PaymentProviderInfo{
		{
			ID:          ProviderPaystack,
			Name:        "Paystack",
			Description: "Card, bank transfer, USSD",
			Enabled:     h.PaystackSecretKey != "",
		},
		{
			ID:          ProviderStripe,
			Name:        "Stripe",
			Description: "International cards",
			Enabled:     h.StripeWebhookSecret != "",
		},
		{
			ID:          ProviderNOWPayments,
			Name:        "Crypto",
			Description: "Bitcoin, USDT, ETH",
			Enabled:     h.NOWPaymentsIPNKey != "",
		},
		{
			ID:          ProviderCredit,
			Name:        "Wallet Credits",
			Description: "Pay from your balance",
			Enabled:     true, // Credit payments are always available
		},
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// ---------------------------------------------------------------------------
// Payment Handlers
// ---------------------------------------------------------------------------

// InitiatePayment handles POST /api/v1/payments/initiate.
func (h *Handlers) InitiatePayment(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Provider     string                 `json:"provider"`
		PurposeType  string                 `json:"purpose_type"`
		PurposeID    string                 `json:"purpose_id"`
		Amount       int64                  `json:"amount"`
		Currency     string                 `json:"currency"`
		Metadata     map[string]interface{} `json:"metadata"`
		DiscountCode string                 `json:"discount_code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Provider == "" {
		writeError(w, http.StatusBadRequest, "missing_provider", "provider is required")
		return
	}

	if input.PurposeType == "" {
		writeError(w, http.StatusBadRequest, "missing_purpose_type", "purpose_type is required")
		return
	}

	if input.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_amount", "Amount must be positive")
		return
	}

	payment, err := h.Service.CreatePayment(r.Context(), tenantID, user.UserID, CreatePaymentInput{
		Provider:     input.Provider,
		PurposeType:  input.PurposeType,
		PurposeID:    input.PurposeID,
		Amount:       input.Amount,
		Currency:     input.Currency,
		Metadata:     input.Metadata,
		DiscountCode: input.DiscountCode,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": payment})
}

// HandlePaystackWebhook handles POST /api/v1/payments/webhook/paystack.
// This endpoint is public (no auth required) as webhooks come from external services.
func (h *Handlers) HandlePaystackWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Read raw body for signature verification.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Failed to read request body")
		return
	}

	defer func() { _ = r.Body.Close() }()

	// Verify Paystack webhook signature (HMAC-SHA256 of body with secret key).
	if h.PaystackSecretKey != "" {
		sig := r.Header.Get("X-Paystack-Signature")
		if err := verifyHMACSHA256(body, sig, h.PaystackSecretKey); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_signature", "Invalid webhook signature")
			return
		}
	}

	// Parse event
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event", "Invalid webhook event")
		return
	}

	event.Provider = ProviderPaystack
	event.RawPayload = body

	// Resolve payment status from the Paystack event type in the payload.
	status := resolvePaystackStatus(event)

	// Process webhook
	if err := h.Service.ProcessWebhook(r.Context(), tenantID, ProviderPaystack, event, status); err != nil {
		slog.Error("paystack webhook processing failed", "tenant_id", tenantID, "error", err)
		errpkg.CaptureException(fmt.Errorf("paystack webhook processing failed (tenant=%s): %w", tenantID, err))
		writeError(w, http.StatusInternalServerError, "webhook_error", "Failed to process webhook")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// HandleStripeWebhook handles POST /api/v1/payments/webhook/stripe.
// This endpoint is public (no auth required) as webhooks come from external services.
func (h *Handlers) HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Read raw body for signature verification.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Failed to read request body")
		return
	}

	defer func() { _ = r.Body.Close() }()

	// Verify Stripe webhook signature (HMAC-SHA256 with webhook secret).
	if h.StripeWebhookSecret != "" {
		sig := r.Header.Get("Stripe-Signature")
		if err := verifyStripeSignature(body, sig, h.StripeWebhookSecret); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_signature", "Invalid webhook signature")
			return
		}
	}

	// Parse event
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event", "Invalid webhook event")
		return
	}

	event.Provider = ProviderStripe
	event.RawPayload = body

	// Resolve payment status from the Stripe event type.
	status := resolveStripeStatus(event)

	// Process webhook
	if err := h.Service.ProcessWebhook(r.Context(), tenantID, ProviderStripe, event, status); err != nil {
		slog.Error("stripe webhook processing failed", "tenant_id", tenantID, "error", err)
		errpkg.CaptureException(fmt.Errorf("stripe webhook processing failed (tenant=%s): %w", tenantID, err))
		writeError(w, http.StatusInternalServerError, "webhook_error", "Failed to process webhook")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// HandleNOWPaymentsWebhook handles POST /api/v1/payments/webhook/nowpayments.
// This endpoint is public (no auth required) as webhooks come from external services.
// NOWPayments IPN sends HMAC-SHA256 of the payload using the IPN secret,
// with the signature in the x-nowpayments-sig header.
func (h *Handlers) HandleNOWPaymentsWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Read raw body for signature verification.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Failed to read request body")
		return
	}

	defer func() { _ = r.Body.Close() }()

	// Verify NOWPayments IPN signature (HMAC-SHA256 with IPN secret).
	if h.NOWPaymentsIPNKey != "" {
		sig := r.Header.Get("x-nowpayments-sig")
		if err := verifyHMACSHA256(body, sig, h.NOWPaymentsIPNKey); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_signature", "Invalid webhook signature")
			return
		}
	}

	// Parse event
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event", "Invalid webhook event")
		return
	}

	event.Provider = ProviderNOWPayments
	event.RawPayload = body

	// Resolve payment status from the NOWPayments event payload.
	status := resolveNOWPaymentsStatus(event)

	// Process webhook
	if err := h.Service.ProcessWebhook(r.Context(), tenantID, ProviderNOWPayments, event, status); err != nil {
		slog.Error("nowpayments webhook processing failed", "tenant_id", tenantID, "error", err)
		errpkg.CaptureException(fmt.Errorf("nowpayments webhook processing failed (tenant=%s): %w", tenantID, err))
		writeError(w, http.StatusInternalServerError, "webhook_error", "Failed to process webhook")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// resolvePaystackStatus maps Paystack event types to payment status strings.
// Paystack sends "charge.success" for successful payments, other events map to "failed".
func resolvePaystackStatus(event WebhookEvent) string {
	if ev, ok := event.Payload["event"].(string); ok && ev == "charge.success" {
		return "succeeded"
	}
	return "failed"
}

// resolveStripeStatus maps Stripe event types to payment status strings.
// Stripe sends "payment_intent.succeeded" for successful payments.
func resolveStripeStatus(event WebhookEvent) string {
	if ev, ok := event.Payload["type"].(string); ok && ev == "payment_intent.succeeded" {
		return "succeeded"
	}
	return "failed"
}

// resolveNOWPaymentsStatus maps NOWPayments event data for payment status.
// NOWPayments sends "payment_status" or "payment_status.created" with specific statuses.
func resolveNOWPaymentsStatus(event WebhookEvent) string {
	if ps, ok := event.Payload["payment_status"].(string); ok {
		switch ps {
		case "finished", "confirmed", "completed":
			return "succeeded"
		case "failed", "expired":
			return "failed"
		case "refunded":
			return "refunded"
		}
	}
	return "failed"
}

// ListPayments handles GET /api/v1/payments.
func (h *Handlers) ListPayments(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	payments, err := h.Service.ListPaymentsByUser(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": payments})
}

// GetPayment handles GET /api/v1/payments/{id}.
func (h *Handlers) GetPayment(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid payment ID")
		return
	}

	payment, err := h.Service.GetPayment(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": payment})
}

// RefundPayment handles POST /api/v1/payments/{id}/refund.
func (h *Handlers) RefundPayment(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid payment ID")
		return
	}

	var input struct {
		Amount int64  `json:"amount"`
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_amount", "Amount must be positive")
		return
	}

	refund, err := h.Service.CreateRefund(r.Context(), CreateRefundInput{
		PaymentID: id.String(),
		Amount:    input.Amount,
		Reason:    input.Reason,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": refund})
}

// ---------------------------------------------------------------------------
// Subscription Handlers
// ---------------------------------------------------------------------------

// CreateSubscription handles POST /api/v1/subscriptions.
func (h *Handlers) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Plan     string `json:"plan"`
		Provider string `json:"provider"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Plan == "" {
		writeError(w, http.StatusBadRequest, "missing_plan", "plan is required")
		return
	}

	if input.Provider == "" {
		writeError(w, http.StatusBadRequest, "missing_provider", "provider is required")
		return
	}

	subscription, err := h.Service.CreateSubscription(r.Context(), tenantID, user.UserID, CreateSubscriptionInput{
		Plan:     input.Plan,
		Provider: input.Provider,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": subscription})
}

// ListSubscriptions handles GET /api/v1/subscriptions.
func (h *Handlers) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	subscriptions, err := h.Service.ListSubscriptionsByUser(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": subscriptions})
}

// GetActiveSubscription handles GET /api/v1/subscriptions/active.
func (h *Handlers) GetActiveSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	subscription, err := h.Service.GetActiveSubscription(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": subscription})
}

// CancelMySubscription handles POST /api/v1/me/subscription/cancel — cancels
// the caller's own active subscription (catalog: no ID in the path, derived
// from the authenticated user via GetActiveSubscription).
func (h *Handlers) CancelMySubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	active, err := h.Service.GetActiveSubscription(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	subscription, err := h.Service.CancelSubscription(r.Context(), active.ID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": subscription})
}

// ---------------------------------------------------------------------------
// Discount Code Handlers
// ---------------------------------------------------------------------------

// CreateDiscountCode handles POST /api/v1/discount-codes.
func (h *Handlers) CreateDiscountCode(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Code          string  `json:"code"`
		DiscountType  string  `json:"discount_type"`
		DiscountValue float64 `json:"discount_value"`
		ApplicableTo  string  `json:"applicable_to"`
		MaxUses       *int32  `json:"max_uses"`
		UsesPerUser   int32   `json:"uses_per_user"`
		GroupID       string  `json:"group_id"`
		ValidFrom     string  `json:"valid_from"`
		ValidUntil    string  `json:"valid_until"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Code == "" {
		writeError(w, http.StatusBadRequest, "missing_code", "code is required")
		return
	}

	if input.DiscountType == "" {
		writeError(w, http.StatusBadRequest, "missing_discount_type", "discount_type is required")
		return
	}

	if input.ApplicableTo == "" {
		writeError(w, http.StatusBadRequest, "missing_applicable_to", "applicable_to is required")
		return
	}

	validFrom := time.Now()
	if input.ValidFrom != "" {
		if err := validFrom.UnmarshalText([]byte(input.ValidFrom)); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_valid_from", "Invalid valid_from format")
			return
		}
	}

	var validUntil *time.Time

	if input.ValidUntil != "" {
		t, err := time.Parse(time.RFC3339, input.ValidUntil)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_valid_until", "Invalid valid_until format")
			return
		}

		validUntil = &t
	}

	code, err := h.Service.CreateDiscountCode(r.Context(), tenantID, CreateDiscountCodeInput{
		Code:          input.Code,
		DiscountType:  input.DiscountType,
		DiscountValue: input.DiscountValue,
		ApplicableTo:  input.ApplicableTo,
		MaxUses:       input.MaxUses,
		UsesPerUser:   input.UsesPerUser,
		GroupID:       input.GroupID,
		ValidFrom:     validFrom,
		ValidUntil:    validUntil,
		CreatedBy:     user.UserID.String(),
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": code})
}

// ListDiscountCodes handles GET /api/v1/discount-codes.
func (h *Handlers) ListDiscountCodes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	codes, err := h.Service.ListDiscountCodes(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": codes})
}

// ValidateDiscountCode handles POST /api/v1/discount-codes/validate.
func (h *Handlers) ValidateDiscountCode(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Code        string `json:"code"`
		PurposeType string `json:"purpose_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Code == "" {
		writeError(w, http.StatusBadRequest, "missing_code", "code is required")
		return
	}

	if input.PurposeType == "" {
		writeError(w, http.StatusBadRequest, "missing_purpose_type", "purpose_type is required")
		return
	}

	code, err := h.Service.ValidateDiscountCode(r.Context(), tenantID, user.UserID, input.Code, input.PurposeType)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": code})
}

// DeleteDiscountCode handles DELETE /api/v1/discount-codes/{id}.
func (h *Handlers) DeleteDiscountCode(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid discount code ID")
		return
	}

	if err := h.Service.DeleteDiscountCode(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Webhook Signature Verification Helpers
// ---------------------------------------------------------------------------

// verifyHMACSHA256 computes HMAC-SHA256 of payload with the given secret and
// compares it against the provided signature using constant-time comparison.
func verifyHMACSHA256(payload []byte, signature, secret string) error {
	if signature == "" {
		return errors.New("missing signature")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return errors.New("invalid signature")
	}

	return nil
}

// verifyStripeSignature verifies a Stripe webhook signature.
// Header format: t=<timestamp>,v1=<signature>[,v1=<signature>...]
// Rejects payloads older than 5 minutes to prevent replay attacks.
func verifyStripeSignature(payload []byte, header, secret string) error {
	if secret == "" {
		return errors.New("stripe webhook secret not configured")
	}

	if header == "" {
		return errors.New("missing Stripe-Signature header")
	}

	var timestamp, sig string

	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}

		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			if sig == "" {
				sig = kv[1]
			}
		}
	}

	if timestamp == "" {
		return errors.New("missing timestamp in Stripe-Signature header")
	}

	if sig == "" {
		return errors.New("missing v1 signature in Stripe-Signature header")
	}

	// Reject payloads older than 5 minutes (replay protection).
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	if age := time.Now().Unix() - ts; age > 300 || age < -300 {
		return fmt.Errorf("timestamp too old or in the future (age=%ds)", age)
	}

	// Compute expected: HMAC-SHA256(secret, timestamp + "." + payload)
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return errors.New("invalid signature")
	}

	return nil
}

// ─── P5.6 Payment Extras ─────────────────────────────────────────────────

// UpdateDiscountCode handles PATCH /admin/discount-codes/:id.
func (h *Handlers) UpdateDiscountCode(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid discount code ID")
		return
	}
	var input struct {
		Code      string `json:"code"`
		Percent   *int   `json:"percent"`
		Amount    *int64 `json:"amount"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	queries := gen.New(h.Service.db.Writer)
	_, err = queries.UpdateDiscountCode(r.Context(), gen.UpdateDiscountCodeParams{
		ID:       id,
		TenantID: tenantID,
		NewCode:  input.Code,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update discount code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": id.String()}})
}

// GetDiscountCodeUsage handles GET /admin/discount-codes/:id/usage.
func (h *Handlers) GetDiscountCodeUsage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	code := chi.URLParam(r, "id")
	usage, err := h.Service.GetDiscountCodeUsageStats(r.Context(), tenantID, code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get usage stats")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": usage})
}

// DeleteSubscription handles DELETE /subscriptions/:id.
func (h *Handlers) DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid subscription ID")
		return
	}
	queries := gen.New(h.Service.db.Writer)
	err = queries.SoftDeleteSubscription(r.Context(), gen.SoftDeleteSubscriptionParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete subscription")
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ReactivateSubscription handles POST /me/subscription/reactivate.
func (h *Handlers) ReactivateSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	queries := gen.New(h.Service.db.Writer)
	_, err := queries.ReactivateSubscription(r.Context(), gen.ReactivateSubscriptionParams{
		UserID:   user.UserID,
		TenantID: tenantID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reactivate subscription")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reactivated": true}})
}
