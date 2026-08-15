// Package payments — HTTP handlers for the Payment Core system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package payments

import (
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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// SettingsProvider abstracts system_settings access for pricing config
// (SETTING-001 / Rule 3). *config.Settings satisfies this structurally.
// When nil, the handler falls back to compiled-in defaults.
type SettingsProvider interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback string) string
}

// Handlers holds dependencies for payment HTTP handlers.
type Handlers struct {
	Service             *Service
	StripeWebhookSecret string // JC_STRIPE_WEBHOOK_SECRET — Stripe webhook signing secret
	NOWPaymentsIPNKey   string // JC_NOWPAYMENTS_IPN_SECRET — NOWPayments IPN HMAC key
	PaystackSecretKey   string // Paystack secret key for webhook HMAC verification
	settings            SettingsProvider
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// SetSettings wires the hot-reloadable settings reader for pricing config
// (SETTING-001). When wired, plan prices are read from system_settings;
// otherwise compiled-in defaults are used.
func (h *Handlers) SetSettings(sp SettingsProvider) { h.settings = sp }

// ---------------------------------------------------------------------------
// Pricing Plans & Payment Providers
// ---------------------------------------------------------------------------

// PricingPlan represents a subscription plan available to users.
type PricingPlan struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Features    []PlanFeature `json:"features"`
	Monthly     PlanPricing   `json:"monthly"`
	Yearly      PlanPricing   `json:"yearly"`
	Highlighted bool          `json:"highlighted"`
}

// PlanPricing holds price information for a billing cycle.
type PlanPricing struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// PlanFeature describes a feature included in a plan.
type PlanFeature struct {
	Text     string `json:"text"`
	Included bool   `json:"included"`
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
// Plans are read from the subscription_plans table (managed via the admin
// panel). If no rows exist yet, falls back to system_settings pricing config
// and ultimately to compiled-in defaults (Rule 3).
func (h *Handlers) ListPricingPlans(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Try DB-managed plans first.
	dbPlans, err := gen.New(h.Service.db.Reader).ListActiveSubscriptionPlans(r.Context(), tenantID)
	if err == nil && len(dbPlans) > 0 {
		plans := make([]PricingPlan, 0, len(dbPlans))
		for _, p := range dbPlans {
			var features []PlanFeature
			_ = json.Unmarshal(p.Features, &features)
			plans = append(plans, PricingPlan{
				ID:          p.Slug,
				Name:        p.Name,
				Description: p.Description,
				Highlighted: p.Highlighted,
				Features:    features,
				Monthly:     PlanPricing{Amount: p.MonthlyAmount, Currency: p.Currency},
				Yearly:      PlanPricing{Amount: p.YearlyAmount, Currency: p.Currency},
			})
		}
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": plans})
		return
	}

	// Fallback: system_settings-configured defaults (SETTING-001 / Rule 3).
	currency := "NGN"
	proMonthly := int64(500000)
	proYearly := int64(5000000)
	bizMonthly := int64(1500000)
	bizYearly := int64(15000000)
	if h.settings != nil {
		currency = h.settings.GetStringOr(r.Context(), tenantID, "pricing.currency", "NGN")
		proMonthly = int64(h.settings.GetIntOr(r.Context(), tenantID, "pricing.plan.pro.monthly_amount", 500000))
		proYearly = int64(h.settings.GetIntOr(r.Context(), tenantID, "pricing.plan.pro.yearly_amount", 5000000))
		bizMonthly = int64(h.settings.GetIntOr(r.Context(), tenantID, "pricing.plan.business.monthly_amount", 1500000))
		bizYearly = int64(h.settings.GetIntOr(r.Context(), tenantID, "pricing.plan.business.yearly_amount", 15000000))
	}

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
			Monthly: PlanPricing{Amount: 0, Currency: currency},
			Yearly:  PlanPricing{Amount: 0, Currency: currency},
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
			Monthly: PlanPricing{Amount: proMonthly, Currency: currency},
			Yearly:  PlanPricing{Amount: proYearly, Currency: currency},
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
			Monthly: PlanPricing{Amount: bizMonthly, Currency: currency},
			Yearly:  PlanPricing{Amount: bizYearly, Currency: currency},
		},
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": plans})
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
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
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Provider == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_provider", "provider is required")
		return
	}

	// BUG-P015: validate provider against the recognized enum so an invalid
	// value gets a structured 400 instead of surfacing as a raw 500 from the
	// DB CHECK constraint (migration 000211).
	if !IsValidProvider(input.Provider) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_provider",
			fmt.Sprintf("provider must be one of: paystack, stripe, nowpayments, manual_transfer, credit (got %q)", input.Provider))
		return
	}

	if input.PurposeType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_purpose_type", "purpose_type is required")
		return
	}

	// BUG-P015: validate purpose_type against the recognized enum so an
	// invalid value gets a structured 400 instead of surfacing as a raw 500
	// from the DB CHECK constraint (migration 000358).
	if !IsValidPurposeType(input.PurposeType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_purpose_type",
			fmt.Sprintf("purpose_type must be one of: pro_membership, org_boost, donation, course_access, credit_purchase, ad_slot, form_submission (got %q)", input.PurposeType))
		return
	}

	if input.Amount <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_amount", "Amount must be positive")
		return
	}

	// Look up the payer's email — Paystack /transaction/initialize requires
	// it (BUG-004). Other providers ignore the field.
	email, _ := gen.New(h.Service.db.Reader).GetUserEmail(r.Context(), gen.GetUserEmailParams{
		UserID:   user.UserID,
		TenantID: tenantID,
	})

	result, err := h.Service.CreatePayment(r.Context(), tenantID, user.UserID, CreatePaymentInput{
		Provider:     input.Provider,
		PurposeType:  input.PurposeType,
		PurposeID:    input.PurposeID,
		Amount:       input.Amount,
		Currency:     input.Currency,
		Email:        email,
		Metadata:     input.Metadata,
		DiscountCode: input.DiscountCode,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": toPaymentAPIResponse(result.Payment, result.CheckoutURL),
	})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Failed to read request body")
		return
	}

	defer func() { _ = r.Body.Close() }()

	// Verify Paystack webhook signature (HMAC-SHA256 of body with secret key).
	// Reject unsigned webhooks unless explicitly allowed in dev (Guide §5.2).
	if h.PaystackSecretKey == "" {
		if !strings.EqualFold(os.Getenv("JC_ALLOW_UNSIGNED_WEBHOOKS"), "true") {
			errpkg.WriteError(w, http.StatusInternalServerError, "misconfigured", "Paystack webhook secret not configured")
			return
		}
	} else {
		sig := r.Header.Get("X-Paystack-Signature")
		if err := verifyHMACSHA256(body, sig, h.PaystackSecretKey); err != nil {
			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_signature", "Invalid webhook signature")
			return
		}
	}

	// Parse event — populate Payload map from raw body for provider-specific
	// field access, then extract event ID and provider reference.
	var event WebhookEvent
	event.Payload = make(map[string]interface{})
	if err := json.Unmarshal(body, &event.Payload); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_event", "Invalid webhook event")
		return
	}

	event.Provider = ProviderPaystack
	event.RawPayload = body
	event.EventID = extractPaystackEventID(event.Payload)
	event.ProviderRef = extractPaystackProviderRef(event.Payload)
	event.EventType, _ = event.Payload["event"].(string)

	// Resolve payment status from the Paystack event type in the payload.
	status := resolvePaystackStatus(event)

	// Process webhook
	if err := h.Service.ProcessWebhook(r.Context(), tenantID, ProviderPaystack, event, status); err != nil {
		slog.Error("paystack webhook processing failed", "tenant_id", tenantID, "error", err)
		errpkg.CaptureException(fmt.Errorf("paystack webhook processing failed (tenant=%s): %w", tenantID, err))
		errpkg.WriteError(w, http.StatusInternalServerError, "webhook_error", "Failed to process webhook")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Failed to read request body")
		return
	}

	defer func() { _ = r.Body.Close() }()

	// Verify Stripe webhook signature (HMAC-SHA256 with webhook secret).
	// Reject unsigned webhooks unless explicitly allowed in dev (Guide §5.2).
	if h.StripeWebhookSecret == "" {
		if !strings.EqualFold(os.Getenv("JC_ALLOW_UNSIGNED_WEBHOOKS"), "true") {
			errpkg.WriteError(w, http.StatusInternalServerError, "misconfigured", "Stripe webhook secret not configured")
			return
		}
	} else {
		sig := r.Header.Get("Stripe-Signature")
		if err := verifyStripeSignature(body, sig, h.StripeWebhookSecret); err != nil {
			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_signature", "Invalid webhook signature")
			return
		}
	}

	// Parse event — populate Payload map from raw body for provider-specific
	// field access, then extract event ID and provider reference.
	var event WebhookEvent
	event.Payload = make(map[string]interface{})
	if err := json.Unmarshal(body, &event.Payload); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_event", "Invalid webhook event")
		return
	}

	event.Provider = ProviderStripe
	event.RawPayload = body
	event.EventID = extractStripeEventID(event.Payload)
	event.ProviderRef = extractStripeProviderRef(event.Payload)
	event.EventType, _ = event.Payload["type"].(string)

	// Resolve payment status from the Stripe event type.
	status := resolveStripeStatus(event)

	// Process webhook
	if err := h.Service.ProcessWebhook(r.Context(), tenantID, ProviderStripe, event, status); err != nil {
		slog.Error("stripe webhook processing failed", "tenant_id", tenantID, "error", err)
		errpkg.CaptureException(fmt.Errorf("stripe webhook processing failed (tenant=%s): %w", tenantID, err))
		errpkg.WriteError(w, http.StatusInternalServerError, "webhook_error", "Failed to process webhook")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Failed to read request body")
		return
	}

	defer func() { _ = r.Body.Close() }()

	// Verify NOWPayments IPN signature (HMAC-SHA256 with IPN secret).
	// Reject unsigned webhooks unless explicitly allowed in dev (Guide §5.2).
	if h.NOWPaymentsIPNKey == "" {
		if !strings.EqualFold(os.Getenv("JC_ALLOW_UNSIGNED_WEBHOOKS"), "true") {
			errpkg.WriteError(w, http.StatusInternalServerError, "misconfigured", "NOWPayments IPN secret not configured")
			return
		}
	} else {
		sig := r.Header.Get("x-nowpayments-sig")
		if err := verifyHMACSHA256(body, sig, h.NOWPaymentsIPNKey); err != nil {
			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_signature", "Invalid webhook signature")
			return
		}
	}

	// Parse event — populate Payload map from raw body for provider-specific
	// field access, then extract event ID and provider reference.
	var event WebhookEvent
	event.Payload = make(map[string]interface{})
	if err := json.Unmarshal(body, &event.Payload); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_event", "Invalid webhook event")
		return
	}

	event.Provider = ProviderNOWPayments
	event.RawPayload = body
	event.EventID = extractNOWPaymentsEventID(event.Payload)
	event.ProviderRef = extractNOWPaymentsProviderRef(event.Payload)
	if ps, ok := event.Payload["payment_status"].(string); ok {
		event.EventType = ps
	}

	// Resolve payment status from the NOWPayments event payload.
	status := resolveNOWPaymentsStatus(event)

	// Process webhook
	if err := h.Service.ProcessWebhook(r.Context(), tenantID, ProviderNOWPayments, event, status); err != nil {
		slog.Error("nowpayments webhook processing failed", "tenant_id", tenantID, "error", err)
		errpkg.CaptureException(fmt.Errorf("nowpayments webhook processing failed (tenant=%s): %w", tenantID, err))
		errpkg.WriteError(w, http.StatusInternalServerError, "webhook_error", "Failed to process webhook")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// resolvePaystackStatus maps Paystack event types to payment status strings.
// Handles the common Paystack event types; unknown events default to
// "processing" rather than "failed" so pending/in-progress payments are not
// prematurely marked failed (Guide §3.2).
func resolvePaystackStatus(event WebhookEvent) string {
	ev, _ := event.Payload["event"].(string)
	switch ev {
	case "charge.success", "transfer.success":
		return "succeeded"
	case "refund.processed":
		return "refunded"
	case "not.processed", "transfer.failed", "charge.failed":
		return "failed"
	}
	return "processing"
}

// resolveStripeStatus maps Stripe event types to payment status strings.
// Handles payment_intent, charge, and refund events; unknown events default
// to "processing" rather than "failed".
func resolveStripeStatus(event WebhookEvent) string {
	ev, _ := event.Payload["type"].(string)
	switch ev {
	case "payment_intent.succeeded", "charge.succeeded":
		return "succeeded"
	case "payment_intent.payment_failed", "charge.failed":
		return "failed"
	case "charge.refunded":
		return "refunded"
	}
	return "processing"
}

// resolveNOWPaymentsStatus maps NOWPayments event data for payment status.
// NOWPayments sends "payment_status" with specific statuses.
func resolveNOWPaymentsStatus(event WebhookEvent) string {
	if ps, ok := event.Payload["payment_status"].(string); ok {
		switch ps {
		case "finished", "confirmed", "completed":
			return "succeeded"
		case "failed", "expired":
			return "failed"
		case "refunded":
			return "refunded"
		case "waiting", "confirming", "partially_paid":
			return "processing"
		}
	}
	return "processing"
}

// extractPaystackEventID extracts the event ID from a Paystack webhook payload.
// Paystack events carry "event" (type) and a "data.reference" (transaction ref).
//
// The event ID is the dedup key for payment_webhook_events
// (UNIQUE(provider, event_id)), so it MUST be unique per logical event.
// Falling back to the bare event type (e.g. "charge.success") is unsafe —
// every charge.success in the tenant would collide and the second payment
// would be silently skipped (BUG-007). Returning the bare transaction
// reference is equally unsafe — Paystack sends multiple event types for the
// same transaction (charge.success then refund.processed), and they would
// collide on the reference, silently dropping the refund event (BUG-P010).
// We therefore compose the event ID from reference + event_type so distinct
// events for the same transaction are processed independently while retries
// of the same event are deduped.
func extractPaystackEventID(payload map[string]interface{}) string {
	ev, _ := payload["event"].(string)
	providerRef := extractPaystackProviderRef(payload)
	if providerRef != "" && ev != "" {
		return fmt.Sprintf("%s:%s", providerRef, ev)
	}
	// Fall back to top-level id if present (some Paystack event formats).
	if id, ok := payload["id"].(string); ok && id != "" {
		return id
	}
	return ""
}

// extractPaystackProviderRef extracts the transaction reference that matches
// payments.provider_ref from a Paystack webhook payload.
func extractPaystackProviderRef(payload map[string]interface{}) string {
	if data, ok := payload["data"].(map[string]interface{}); ok {
		if ref, ok := data["reference"].(string); ok && ref != "" {
			return ref
		}
	}
	return ""
}

// extractStripeEventID extracts the event ID from a Stripe webhook payload.
// Stripe events have a top-level "id" field (evt_...) and "data.object.id".
func extractStripeEventID(payload map[string]interface{}) string {
	if id, ok := payload["id"].(string); ok && id != "" {
		return id
	}
	return ""
}

// extractStripeProviderRef extracts the object ID that matches
// payments.provider_ref from a Stripe webhook payload. For payment_intent
// events this is data.object.id (pi_... or cs_... for checkout sessions).
func extractStripeProviderRef(payload map[string]interface{}) string {
	if data, ok := payload["data"].(map[string]interface{}); ok {
		if obj, ok := data["object"].(map[string]interface{}); ok {
			// For payment_intent.succeeded, object.id is the payment intent ID.
			// For checkout.session.completed, object.id is the checkout session ID.
			if id, ok := obj["id"].(string); ok && id != "" {
				return id
			}
		}
	}
	return ""
}

// extractNOWPaymentsEventID extracts the event ID from a NOWPayments IPN payload.
//
// NOWPayments sends multiple IPN events for the same payment as it
// transitions waiting → confirming → finished, all sharing the same
// payment_id. Because payment_webhook_events dedups on
// UNIQUE(provider, event_id), using the bare payment_id would collapse
// all status transitions into one event — only the first IPN (e.g.
// "waiting" → processing) is processed and the "finished" IPN is
// silently dropped, so the payment is never marked succeeded (BUG-008).
// We therefore compose the event ID from payment_id + payment_status so
// each status transition is a distinct, dedup-safe event.
func extractNOWPaymentsEventID(payload map[string]interface{}) string {
	paymentID, _ := payload["payment_id"].(string)
	if paymentID == "" {
		// Fall back to order_id when payment_id is absent (early IPNs).
		paymentID, _ = payload["order_id"].(string)
	}
	if paymentID == "" {
		return ""
	}
	status, _ := payload["payment_status"].(string)
	if status == "" {
		return paymentID
	}
	return fmt.Sprintf("%s:%s", paymentID, status)
}

// extractNOWPaymentsProviderRef extracts the reference matching
// payments.provider_ref from a NOWPayments IPN payload.
func extractNOWPaymentsProviderRef(payload map[string]interface{}) string {
	if id, ok := payload["pay_address"].(string); ok && id != "" {
		return id
	}
	if id, ok := payload["order_id"].(string); ok && id != "" {
		return id
	}
	return ""
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toPaymentAPIResponses(payments)})
}

// GetPayment handles GET /api/v1/me/payments/{id}.
// The route is user-scoped (/me/) so the payment must belong to the
// authenticated user — without this check any user in the tenant could
// read another user's payment details by ID enumeration (BUG-P004).
func (h *Handlers) GetPayment(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid payment ID")
		return
	}

	payment, err := h.Service.GetPayment(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enforce user ownership — the /me/ prefix means only the payer may
	// view their own payment. A mismatch indicates cross-user access.
	if payment.UserID != user.UserID {
		errpkg.WriteError(w, http.StatusForbidden, "payment_not_owned", "You do not have access to this payment")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toPaymentAPIResponse(payment, "")})
}

// RefundPayment handles POST /api/v1/payments/{id}/refund.
// Gated by payments.refund (admin-only). Verifies the payment exists in
// the tenant and is in a refundable state (succeeded) before creating
// the refund record (BUG-P008, BUG-P009).
func (h *Handlers) RefundPayment(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid payment ID")
		return
	}

	// Verify the payment exists in this tenant and is refundable.
	// Without this check, a refund could be created for a payment in
	// another tenant (BUG-P008) or for a payment that was never
	// succeeded (BUG-P009).
	payment, err := h.Service.GetPayment(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if payment.Status != PaymentStatusSucceeded {
		errpkg.WriteError(w, http.StatusConflict, "payment_not_refundable",
			"Payment must be in 'succeeded' status to be refunded")
		return
	}

	var input struct {
		Amount int64  `json:"amount"`
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Amount <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_amount", "Amount must be positive")
		return
	}

	if input.Amount > payment.Amount {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_amount", "Refund amount cannot exceed payment amount")
		return
	}

	refund, err := h.Service.CreateRefund(r.Context(), CreateRefundInput{
		PaymentID: id.String(),
		TenantID:  tenantID,
		Amount:    input.Amount,
		Reason:    input.Reason,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": refund})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Plan == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_plan", "plan is required")
		return
	}

	if input.Provider == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_provider", "provider is required")
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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": subscription})
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subscriptions})
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subscription})
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subscription})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_code", "code is required")
		return
	}

	if input.DiscountType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_discount_type", "discount_type is required")
		return
	}

	if input.ApplicableTo == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_applicable_to", "applicable_to is required")
		return
	}

	validFrom := time.Now()
	if input.ValidFrom != "" {
		if err := validFrom.UnmarshalText([]byte(input.ValidFrom)); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_valid_from", "Invalid valid_from format")
			return
		}
	}

	var validUntil *time.Time

	if input.ValidUntil != "" {
		t, err := time.Parse(time.RFC3339, input.ValidUntil)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_valid_until", "Invalid valid_until format")
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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": code})
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": codes})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_code", "code is required")
		return
	}

	if input.PurposeType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_purpose_type", "purpose_type is required")
		return
	}

	code, err := h.Service.ValidateDiscountCode(r.Context(), tenantID, user.UserID, input.Code, input.PurposeType)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": code})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid discount code ID")
		return
	}

	if err := h.Service.DeleteDiscountCode(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Webhook Signature Verification Helpers
// ---------------------------------------------------------------------------

// verifyHMACSHA256 computes HMAC-SHA256 of payload with the given secret and
// compares it against the provided signature using constant-time comparison.
func verifyHMACSHA256(payload []byte, signature, secret string) error {
	if secret == "" {
		// Without a secret every caller could compute a valid signature.
		return errors.New("webhook secret not configured")
	}

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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid discount code ID")
		return
	}
	var input struct {
		Code          string  `json:"code"`
		DiscountType  string  `json:"discount_type"`
		DiscountValue float64 `json:"discount_value"`
		ApplicableTo  string  `json:"applicable_to"`
		MaxUses       *int32  `json:"max_uses"`
		ValidFrom     string  `json:"valid_from"`
		ValidUntil    *string `json:"valid_until"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Validate discount_type if provided (BUG-P007): the DB CHECK allows
	// 'percent', 'fixed', and 'percentage', but calculateDiscount only
	// handled 'percent' and 'fixed' — a 'percentage' code would silently
	// compute zero discount. Validate here to reject unsupported values
	// early with a clear error instead of a silent zero-discount.
	if input.DiscountType != "" {
		switch input.DiscountType {
		case DiscountTypePercent, DiscountTypeFixed, "percentage":
			// valid
		default:
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_discount_type",
				"discount_type must be 'percent' or 'fixed'")
			return
		}
	}

	// Validate applicable_to if provided.
	if input.ApplicableTo != "" {
		switch input.ApplicableTo {
		case ApplicableToCourseAccess, ApplicableToProMembership, ApplicableToAll:
			// valid
		default:
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_applicable_to",
				"applicable_to must be 'course_access', 'pro_membership', or 'all'")
			return
		}
	}

	// Build nullable valid_until: explicit null clears the field, missing key keeps existing.
	var validUntil pgtype.Timestamptz
	if input.ValidUntil != nil {
		if *input.ValidUntil == "" {
			validUntil = pgtype.Timestamptz{Valid: false}
		} else {
			t, err := time.Parse(time.RFC3339, *input.ValidUntil)
			if err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_valid_until", "Invalid valid_until format")
				return
			}
			validUntil = pgtype.Timestamptz{Time: t, Valid: true}
		}
	} else {
		// Key not present — keep existing value (pass NULL so COALESCE preserves it).
		validUntil = pgtype.Timestamptz{Valid: false}
	}

	var validFrom pgtype.Timestamptz
	if input.ValidFrom != "" {
		t, err := time.Parse(time.RFC3339, input.ValidFrom)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_valid_from", "Invalid valid_from format")
			return
		}
		validFrom = pgtype.Timestamptz{Time: t, Valid: true}
	}

	var discountValue pgtype.Numeric
	if input.DiscountValue != 0 {
		discountValue = pgtype.Numeric{}
		_ = discountValue.Scan(input.DiscountValue)
	}

	queries := gen.New(h.Service.db.Writer)
	updated, err := queries.UpdateDiscountCode(r.Context(), gen.UpdateDiscountCodeParams{
		ID:            id,
		TenantID:      tenantID,
		NewCode:       input.Code,
		DiscountType:  input.DiscountType,
		DiscountValue: discountValue,
		ApplicableTo:  input.ApplicableTo,
		MaxUses:       input.MaxUses,
		ValidFrom:     validFrom,
		ValidUntil:    validUntil,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update discount code")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// GetDiscountCodeUsage handles GET /admin/discount-codes/:id/usage.
func (h *Handlers) GetDiscountCodeUsage(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	codeID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid discount code ID")
		return
	}

	// Use a COUNT query for the total instead of len(list) capped at 1000
	// (BUG-012 — same class as BUG-018). The user list is still paginated
	// with a 1000-row window for the slide-over display.
	total, err := h.Service.store.countDiscountCodeUses(r.Context(), codeID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get usage count")
		return
	}

	queries := gen.New(h.Service.db.Reader)
	uses, err := queries.ListDiscountCodeUses(r.Context(), gen.ListDiscountCodeUsesParams{
		CodeID:     codeID,
		PageLimit:  1000,
		PageOffset: 0,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get usage stats")
		return
	}

	type userUse struct {
		UserID string `json:"user_id"`
		UsedAt string `json:"used_at"`
	}
	users := make([]userUse, 0, len(uses))
	for _, u := range uses {
		users = append(users, userUse{
			UserID: u.UserID.String(),
			UsedAt: u.UsedAt.Time.Format("2006-01-02T15:04:05Z"),
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"users": users,
			"total": total,
		},
	})
}

// DeleteSubscription handles DELETE /subscriptions/:id.
// Verifies the subscription belongs to the authenticated user before
// soft-deleting — without this check any user in the tenant could delete
// another user's subscription by ID enumeration (BUG-P003).
func (h *Handlers) DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid subscription ID")
		return
	}

	// Fetch the subscription and verify ownership before mutating.
	sub, err := h.Service.GetSubscription(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if sub.UserID != user.UserID {
		errpkg.WriteError(w, http.StatusForbidden, "subscription_not_owned", "You do not have access to this subscription")
		return
	}

	queries := gen.New(h.Service.db.Writer)
	err = queries.SoftDeleteSubscription(r.Context(), gen.SoftDeleteSubscriptionParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete subscription")
		return
	}
	errpkg.WriteJSON(w, http.StatusNoContent, nil)
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
	subscription, err := h.Service.ReactivateSubscription(r.Context(), user.UserID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subscription})
}

// ---------------------------------------------------------------------------
// Admin: Subscription Plans CRUD (GET/POST/PATCH/DELETE /admin/plans)
// ---------------------------------------------------------------------------

// AdminPlanFeature is the JSON shape for a single feature row in plan create/update.
type AdminPlanFeature struct {
	Text     string `json:"text"`
	Included bool   `json:"included"`
}

// AdminPlanInput is the request body for creating/updating a plan.
type AdminPlanInput struct {
	Slug          string             `json:"slug"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	Features      []AdminPlanFeature `json:"features"`
	MonthlyAmount int64              `json:"monthly_amount"`
	YearlyAmount  int64              `json:"yearly_amount"`
	Currency      string             `json:"currency"`
	Highlighted   bool               `json:"highlighted"`
	SortOrder     int32              `json:"sort_order"`
	Status        string             `json:"status"`
}

// AdminPlanResponse is the JSON-serializable plan shape returned by admin endpoints.
// The gen.SubscriptionPlan.Features field is []byte (raw JSONB), which json.Marshal
// base64-encodes. This struct parses features into a proper JSON array so the
// frontend receives a usable array.
type AdminPlanResponse struct {
	ID            string             `json:"id"`
	TenantID      string             `json:"tenant_id"`
	Slug          string             `json:"slug"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	Features      []AdminPlanFeature `json:"features"`
	MonthlyAmount int64              `json:"monthly_amount"`
	YearlyAmount  int64              `json:"yearly_amount"`
	Currency      string             `json:"currency"`
	Highlighted   bool               `json:"highlighted"`
	SortOrder     int32              `json:"sort_order"`
	Status        string             `json:"status"`
	CreatedAt     string             `json:"created_at"`
	UpdatedAt     string             `json:"updated_at"`
}

// toAdminPlanResponse converts a gen.SubscriptionPlan to an AdminPlanResponse,
// parsing the raw JSONB features bytes into a proper slice.
func toAdminPlanResponse(p gen.SubscriptionPlan) AdminPlanResponse {
	var features []AdminPlanFeature
	if len(p.Features) > 0 {
		_ = json.Unmarshal(p.Features, &features)
	}
	resp := AdminPlanResponse{
		Slug:          p.Slug,
		Name:          p.Name,
		Description:   p.Description,
		Features:      features,
		MonthlyAmount: p.MonthlyAmount,
		YearlyAmount:  p.YearlyAmount,
		Currency:      p.Currency,
		Highlighted:   p.Highlighted,
		SortOrder:     p.SortOrder,
		Status:        p.Status,
	}
	if p.ID.Valid {
		resp.ID = p.ID.String()
	}
	if p.TenantID.Valid {
		resp.TenantID = p.TenantID.String()
	}
	if p.CreatedAt.Valid {
		resp.CreatedAt = p.CreatedAt.Time.Format(time.RFC3339)
	}
	if p.UpdatedAt.Valid {
		resp.UpdatedAt = p.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// ListAdminPlans handles GET /api/v1/admin/plans.
func (h *Handlers) ListAdminPlans(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	plans, err := gen.New(h.Service.db.Reader).ListSubscriptionPlans(r.Context(), tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list plans")
		return
	}
	resp := make([]AdminPlanResponse, 0, len(plans))
	for _, p := range plans {
		resp = append(resp, toAdminPlanResponse(p))
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// CreateAdminPlan handles POST /api/v1/admin/plans.
func (h *Handlers) CreateAdminPlan(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input AdminPlanInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Name == "" || input.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "name and slug are required")
		return
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if input.Currency == "" {
		input.Currency = "NGN"
	}
	featuresJSON, _ := json.Marshal(input.Features)

	plan, err := gen.New(h.Service.db.Writer).CreateSubscriptionPlan(r.Context(), gen.CreateSubscriptionPlanParams{
		TenantID:      tenantID,
		Slug:          input.Slug,
		Name:          input.Name,
		Description:   input.Description,
		Features:      featuresJSON,
		MonthlyAmount: input.MonthlyAmount,
		YearlyAmount:  input.YearlyAmount,
		Currency:      input.Currency,
		Highlighted:   input.Highlighted,
		SortOrder:     input.SortOrder,
		Status:        input.Status,
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			errpkg.WriteError(w, http.StatusConflict, "plan_exists", "A plan with this slug already exists")
			return
		}
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create plan")
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": toAdminPlanResponse(plan)})
}

// UpdateAdminPlan handles PATCH /api/v1/admin/plans/{id}.
func (h *Handlers) UpdateAdminPlan(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid plan ID")
		return
	}
	var input AdminPlanInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	featuresJSON, _ := json.Marshal(input.Features)

	plan, err := gen.New(h.Service.db.Writer).UpdateSubscriptionPlan(r.Context(), gen.UpdateSubscriptionPlanParams{
		ID:            id,
		TenantID:      tenantID,
		Slug:          input.Slug,
		Name:          input.Name,
		Description:   input.Description,
		Features:      featuresJSON,
		MonthlyAmount: input.MonthlyAmount,
		YearlyAmount:  input.YearlyAmount,
		Currency:      input.Currency,
		Highlighted:   input.Highlighted,
		SortOrder:     input.SortOrder,
		Status:        input.Status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "plan_not_found", "Plan not found")
			return
		}
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update plan")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toAdminPlanResponse(plan)})
}

// DeleteAdminPlan handles DELETE /api/v1/admin/plans/{id}.
func (h *Handlers) DeleteAdminPlan(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid plan ID")
		return
	}
	err = gen.New(h.Service.db.Writer).DeleteSubscriptionPlan(r.Context(), gen.DeleteSubscriptionPlanParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete plan")
		return
	}
	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ListAdminPayments handles GET /api/v1/admin/payments.
// Lists all payments across the platform with optional filters (JEOS §17
// "Admin Payment Routes"). Gated by payments.refund permission (admin-only).
func (h *Handlers) ListAdminPayments(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	// Parse optional filters from query params.
	var params gen.ListAdminPaymentsParams
	params.TenantID = tenantID
	params.PageLimit = limit
	params.PageOffset = offset

	if v := r.URL.Query().Get("user_id"); v != "" {
		var uid pgtype.UUID
		if err := uid.Scan(v); err == nil {
			params.UserID = uid
		}
	}
	if v := r.URL.Query().Get("status"); v != "" {
		params.Status = &v
	}
	if v := r.URL.Query().Get("purpose_type"); v != "" {
		params.PurposeType = &v
	}
	if v := r.URL.Query().Get("provider"); v != "" {
		params.Provider = &v
	}
	if v := r.URL.Query().Get("start"); v != "" {
		var t pgtype.Timestamptz
		if err := t.Scan(v); err == nil {
			params.StartDate = t
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		var t pgtype.Timestamptz
		if err := t.Scan(v); err == nil {
			params.EndDate = t
		}
	}

	payments, err := h.Service.ListAdminPayments(r.Context(), params)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toPaymentAPIResponses(payments)})
}

// GetAdminPayment handles GET /api/v1/admin/payments/:id.
// Returns payment detail (JEOS §17 "Admin Payment Routes").
func (h *Handlers) GetAdminPayment(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid payment ID")
		return
	}

	payment, err := h.Service.GetAdminPayment(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toPaymentAPIResponse(payment, "")})
}

// ─── Subscription Plan ↔ Track management (migration 000273) ──────────────

// PlanTrackResponse is the DTO for a track listed under a plan.
type PlanTrackResponse struct {
	TrackID string `json:"track_id"`
}

// AdminListPlanTracks handles GET /api/v1/admin/plans/{id}/tracks.
func (h *Handlers) AdminListPlanTracks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	planID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid plan ID")
		return
	}
	trackIDs, err := gen.New(h.Service.db.Reader).ListTrackIDsForPlan(r.Context(), gen.ListTrackIDsForPlanParams{
		PlanID:   planID,
		TenantID: tenantID,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list plan tracks")
		return
	}
	resp := make([]PlanTrackResponse, 0, len(trackIDs))
	for _, tid := range trackIDs {
		resp = append(resp, PlanTrackResponse{TrackID: tid.String()})
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// AdminSetPlanTracks handles PUT /api/v1/admin/plans/{id}/tracks.
// Replaces the entire track list for a plan.
func (h *Handlers) AdminSetPlanTracks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	planID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid plan ID")
		return
	}
	var input struct {
		TrackIDs []string `json:"track_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// BUG-P016: wrap the clear + re-add in a single transaction so a failure
	// mid-way doesn't leave the plan with a partial track list (some old
	// tracks removed, some new tracks missing). The previous code issued
	// RemoveAllTracksFromPlan + N x AddTrackToPlan as independent DB calls.
	ctx := r.Context()
	tx, err := h.Service.db.Writer.Begin(ctx)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to begin transaction")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := gen.New(tx)
	if err := qtx.RemoveAllTracksFromPlan(ctx, gen.RemoveAllTracksFromPlanParams{
		PlanID:   planID,
		TenantID: tenantID,
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to clear plan tracks")
		return
	}
	added := 0
	for _, tidStr := range input.TrackIDs {
		var tid pgtype.UUID
		if err := tid.Scan(tidStr); err != nil {
			continue // skip invalid IDs
		}
		if err := qtx.AddTrackToPlan(ctx, gen.AddTrackToPlanParams{
			PlanID:   planID,
			TrackID:  tid,
			TenantID: tenantID,
		}); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to add track to plan")
			return
		}
		added++
	}
	if err := tx.Commit(ctx); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to commit plan tracks")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]int{"count": added}})
}

// AdminAddTrackToPlan handles POST /api/v1/admin/plans/{id}/tracks/{trackId}.
func (h *Handlers) AdminAddTrackToPlan(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	planID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid plan ID")
		return
	}
	trackID, err := parseUUIDParam(r, "trackId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}
	if err := gen.New(h.Service.db.Writer).AddTrackToPlan(r.Context(), gen.AddTrackToPlanParams{
		PlanID:   planID,
		TrackID:  trackID,
		TenantID: tenantID,
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to add track to plan")
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]bool{"added": true}})
}

// AdminRemoveTrackFromPlan handles DELETE /api/v1/admin/plans/{id}/tracks/{trackId}.
func (h *Handlers) AdminRemoveTrackFromPlan(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	planID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid plan ID")
		return
	}
	trackID, err := parseUUIDParam(r, "trackId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid track ID")
		return
	}
	if err := gen.New(h.Service.db.Writer).RemoveTrackFromPlan(r.Context(), gen.RemoveTrackFromPlanParams{
		PlanID:   planID,
		TrackID:  trackID,
		TenantID: tenantID,
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove track from plan")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"removed": true}})
}

// PublicListPlanTracks handles GET /api/v1/pricing/plans/{slug}/tracks.
// Returns the track IDs included in a plan (empty = all tracks).
func (h *Handlers) PublicListPlanTracks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Plan slug is required")
		return
	}
	plan, err := gen.New(h.Service.db.Reader).GetSubscriptionPlanBySlug(r.Context(), gen.GetSubscriptionPlanBySlugParams{
		TenantID: tenantID,
		Slug:     slug,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "plan_not_found", "Plan not found")
		return
	}
	trackIDs, err := gen.New(h.Service.db.Reader).ListTrackIDsForPlan(r.Context(), gen.ListTrackIDsForPlanParams{
		PlanID:   plan.ID,
		TenantID: tenantID,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list plan tracks")
		return
	}
	resp := make([]string, 0, len(trackIDs))
	for _, tid := range trackIDs {
		resp = append(resp, tid.String())
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}
