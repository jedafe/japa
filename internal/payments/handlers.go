// Package payments — HTTP handlers for the Payment Core system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package payments

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for payment HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
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
	case strings.Contains(err.Error(), "already_exists"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "duplicate"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
		return DefaultPageSize
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return DefaultPageSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return int32(n)
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
	return int32(n)
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

	// Read raw body for signature verification
	body, err := json.Marshal(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Parse event
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event", "Invalid webhook event")
		return
	}
	event.Provider = ProviderPaystack
	event.RawPayload = body

	// Process webhook
	if err := h.Service.ProcessWebhook(r.Context(), tenantID, ProviderPaystack, event); err != nil {
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

	// Read raw body for signature verification
	body, err := json.Marshal(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Parse event
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event", "Invalid webhook event")
		return
	}
	event.Provider = ProviderStripe
	event.RawPayload = body

	// Process webhook
	if err := h.Service.ProcessWebhook(r.Context(), tenantID, ProviderStripe, event); err != nil {
		writeError(w, http.StatusInternalServerError, "webhook_error", "Failed to process webhook")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
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
		Amount  int64  `json:"amount"`
		Reason  string `json:"reason"`
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

// CancelSubscription handles PATCH /api/v1/subscriptions/{id}/cancel.
func (h *Handlers) CancelSubscription(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid subscription ID")
		return
	}

	subscription, err := h.Service.CancelSubscription(r.Context(), id, tenantID)
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
		Code          string   `json:"code"`
		DiscountType  string   `json:"discount_type"`
		DiscountValue float64  `json:"discount_value"`
		ApplicableTo  string   `json:"applicable_to"`
		MaxUses       *int32   `json:"max_uses"`
		UsesPerUser   int32    `json:"uses_per_user"`
		GroupID       string   `json:"group_id"`
		ValidFrom     string   `json:"valid_from"`
		ValidUntil    string   `json:"valid_until"`
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