package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// CreatePaymentInput holds parameters for creating a new payment.
type CreatePaymentInput struct {
	Provider     string
	PurposeType  string
	PurposeID    string
	Amount       int64
	Currency     string
	Metadata     map[string]interface{}
	DiscountCode string
}

// CreatePayment creates a new payment record and initiates the payment with the provider.
func (s *Service) CreatePayment(ctx context.Context, tenantID, userID pgtype.UUID, input CreatePaymentInput) (gen.Payment, error) {
	if input.Provider == "" {
		return gen.Payment{}, errors.New("provider is required")
	}

	if input.PurposeType == "" {
		return gen.Payment{}, errors.New("purpose_type is required")
	}

	if input.Amount <= 0 {
		return gen.Payment{}, errors.New("amount must be positive")
	}

	currency := input.Currency
	if currency == "" {
		currency = "NGN"
		if s.settings != nil {
			if v, err := s.settings.GetSetting(ctx, tenantID, "payment.currency"); err == nil && v != "" {
				currency = v
			}
		}
	}

	// Credit payments are synchronous wallet debits — no external provider call.
	if input.Provider == ProviderCredit {
		return s.payWithCreditInternal(ctx, tenantID, userID, input, currency)
	}

	// Initiate payment with external provider (Paystack, Stripe, NOWPayments).
	provider := s.providers[input.Provider]
	var providerRef, checkoutURL string
	if provider != nil {
		resp, err := provider.InitiatePayment(ctx, PaymentRequest{
			TenantID:    tenantID.String(),
			UserID:      userID.String(),
			Amount:      input.Amount,
			Currency:    currency,
			PurposeType: input.PurposeType,
			PurposeID:   input.PurposeID,
			Metadata:    input.Metadata,
		})
		if err != nil {
			return gen.Payment{}, fmt.Errorf("initiating payment with %s: %w", input.Provider, err)
		}
		providerRef = resp.Reference
		checkoutURL = resp.CheckoutURL
	}

	return s.createPaymentRow(ctx, tenantID, userID, input, currency, providerRef, checkoutURL)
}

// createPaymentRow inserts the payment record after validation and discount calculation.
func (s *Service) createPaymentRow(ctx context.Context, tenantID, userID pgtype.UUID, input CreatePaymentInput, currency, providerRef, checkoutURL string) (gen.Payment, error) {
	var purposeID pgtype.UUID
	if input.PurposeID != "" {
		if err := purposeID.Scan(input.PurposeID); err != nil {
			return gen.Payment{}, fmt.Errorf("invalid purpose_id: %w", err)
		}
	}

	var discountCodeID pgtype.UUID
	var discountAmount int64

	if input.DiscountCode != "" {
		code, err := s.store.getDiscountCodeByCode(ctx, tenantID, input.DiscountCode)
		if err != nil {
			return gen.Payment{}, fmt.Errorf("invalid discount code: %w", err)
		}
		if err := s.validateDiscountCode(ctx, code, userID); err != nil {
			return gen.Payment{}, err
		}
		discountCodeID = code.ID
		discountAmount = s.calculateDiscount(code, input.Amount)
	}

	var metadata []byte
	if input.Metadata != nil {
		var err error
		metadata, err = json.Marshal(input.Metadata)
		if err != nil {
			return gen.Payment{}, fmt.Errorf("marshaling metadata: %w", err)
		}
	} else {
		metadata = []byte("{}")
	}

	var ref *string
	if providerRef != "" {
		ref = &providerRef
	}

	payment, err := s.store.createPayment(ctx, gen.CreatePaymentParams{
		TenantID:       tenantID,
		UserID:         userID,
		Provider:       input.Provider,
		ProviderRef:    ref,
		PurposeType:    input.PurposeType,
		PurposeID:      purposeID,
		Amount:         input.Amount,
		Currency:       currency,
		Status:         PaymentStatusPending,
		Metadata:       metadata,
		DiscountCodeID: discountCodeID,
		DiscountAmount: discountAmount,
	})
	if err != nil {
		return gen.Payment{}, err
	}

	_ = checkoutURL
	return payment, nil
}

// payWithCreditInternal handles credit wallet payments.
func (s *Service) payWithCreditInternal(ctx context.Context, tenantID, userID pgtype.UUID, input CreatePaymentInput, currency string) (gen.Payment, error) {
	if s.wallet == nil {
		return gen.Payment{}, errors.New("wallet service not configured")
	}
	creditInput := CreditSpendInput{
		Amount:      input.Amount,
		Source:      "payment",
		Reference:   fmt.Sprintf("payment:%s:%s", input.PurposeType, input.PurposeID),
		PurposeType: input.PurposeType,
		PurposeID:   input.PurposeID,
	}
	if _, err := s.wallet.SpendCredits(ctx, tenantID, userID, creditInput); err != nil {
		return gen.Payment{}, fmt.Errorf("%w: %w", ErrInsufficientCredit, err)
	}
	return s.createPaymentRow(ctx, tenantID, userID, input, currency, "", "")
}

// GetPayment retrieves a payment by ID.
func (s *Service) GetPayment(ctx context.Context, id, tenantID pgtype.UUID) (gen.Payment, error) {
	return s.store.getPaymentByID(ctx, id, tenantID)
}

// ListPaymentsByUser returns payments for a user.
func (s *Service) ListPaymentsByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.Payment, error) {
	return s.store.listPaymentsByUser(ctx, tenantID, userID, limit, offset)
}

// ListPaymentsByStatus returns payments with a specific status.
func (s *Service) ListPaymentsByStatus(ctx context.Context, tenantID pgtype.UUID, status string, limit, offset int32) ([]gen.Payment, error) {
	return s.store.listPaymentsByStatus(ctx, tenantID, status, limit, offset)
}

// UpdatePaymentStatus updates a payment's status.
func (s *Service) UpdatePaymentStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) (gen.Payment, error) {
	return s.store.updatePaymentStatus(ctx, id, tenantID, status)
}

// ProcessWebhook processes a webhook event from a payment provider.
// Status indicates the event outcome ("succeeded", "failed", "refunded") and is extracted
// from the provider-specific payload by each HTTP handler before calling this method.
func (s *Service) ProcessWebhook(ctx context.Context, tenantID pgtype.UUID, provider string, event WebhookEvent, status string) error {
	// Check idempotency
	processed, err := s.store.isWebhookEventProcessed(ctx, tenantID, provider, event.EventID)
	if err != nil {
		return fmt.Errorf("checking webhook idempotency: %w", err)
	}

	if processed {
		return nil // Already processed
	}

	// Record the event (idempotent via ON CONFLICT DO NOTHING on unique constraint).
	// ErrNoRows from ON CONFLICT DO NOTHING means duplicate — treat as success.
	// 
	if _, err := s.store.insertWebhookEvent(ctx, gen.InsertWebhookEventParams{
		TenantID:   tenantID,
		Provider:   provider,
		EventID:    event.EventID,
		RawPayload: event.RawPayload,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("recording webhook event: %w", err)
	}

	// Enqueue async processing — the worker handles payment status updates, subscription
	// activation, wallet adjustments, and affiliate commission crediting.
	if s.enqueuer != nil {
		payload, _ := json.Marshal(map[string]string{
			"event_id":  event.EventID,
			"tenant_id": tenantID.String(),
			"status":    status,
		})
		// Return the error so the provider receives a 5xx and retries.
		// The event is already recorded in the DB; on retry the idempotency
		// check sees it as processed and returns 200.
		if err := s.enqueuer.Enqueue(worker.TypePaymentWebhookProcess, payload); err != nil {
			slog.Error("payments: failed to enqueue webhook processing",
				"event_id", event.EventID, "provider", provider, "error", err)
			errpkg.CaptureException(fmt.Errorf("enqueue %s (event %s): %w", worker.TypePaymentWebhookProcess, event.EventID, err))
			return fmt.Errorf("enqueue %s (event %s): %w", worker.TypePaymentWebhookProcess, event.EventID, err)
		}
	}

	// Mark as processed
	if err := s.store.markWebhookEventProcessed(ctx, tenantID, provider, event.EventID); err != nil {
		return fmt.Errorf("marking webhook event processed: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Subscriptions
// ---------------------------------------------------------------------------

// CreateSubscriptionInput holds parameters for creating a subscription.
type CreateSubscriptionInput struct {
	Plan     string
	Provider string
}

// CreateSubscription creates a new subscription.
func (s *Service) CreateSubscription(ctx context.Context, tenantID, userID pgtype.UUID, input CreateSubscriptionInput) (gen.Subscription, error) {
	if input.Plan == "" {
		return gen.Subscription{}, errors.New("plan is required")
	}

	if input.Provider == "" {
		return gen.Subscription{}, errors.New("provider is required")
	}

	// Calculate period based on plan
	now := time.Now()

	var periodEnd time.Time

	switch input.Plan {
	case PlanProMonthly:
		periodEnd = now.AddDate(0, 1, 0)
	case PlanProYearly:
		periodEnd = now.AddDate(1, 0, 0)
	default:
		return gen.Subscription{}, fmt.Errorf("invalid plan: %s", input.Plan)
	}

	subscription, err := s.store.createSubscription(ctx, gen.CreateSubscriptionParams{
		TenantID:           tenantID,
		UserID:             userID,
		Plan:               input.Plan,
		Provider:           input.Provider,
		Status:             SubscriptionStatusActive,
		CurrentPeriodStart: pgtype.Timestamptz{Time: now, Valid: true},
		CurrentPeriodEnd:   pgtype.Timestamptz{Time: periodEnd, Valid: true},
	})
	if err != nil {
		return gen.Subscription{}, err
	}

	return subscription, nil
}

// GetSubscription retrieves a subscription by ID.
func (s *Service) GetSubscription(ctx context.Context, id, tenantID pgtype.UUID) (gen.Subscription, error) {
	return s.store.getSubscriptionByID(ctx, id, tenantID)
}

// GetActiveSubscription returns the active subscription for a user.
func (s *Service) GetActiveSubscription(ctx context.Context, tenantID, userID pgtype.UUID) (gen.Subscription, error) {
	return s.store.getActiveSubscriptionByUser(ctx, tenantID, userID)
}

// ListSubscriptionsByUser returns subscriptions for a user.
func (s *Service) ListSubscriptionsByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.Subscription, error) {
	return s.store.listSubscriptionsByUser(ctx, tenantID, userID, limit, offset)
}

// CancelSubscription cancels a subscription.
func (s *Service) CancelSubscription(ctx context.Context, id, tenantID pgtype.UUID) (gen.Subscription, error) {
	return s.store.cancelSubscription(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Discount Codes
// ---------------------------------------------------------------------------

// CreateDiscountCodeInput holds parameters for creating a discount code.
type CreateDiscountCodeInput struct {
	Code          string
	DiscountType  string
	DiscountValue float64
	ApplicableTo  string
	MaxUses       *int32
	UsesPerUser   int32
	GroupID       string
	ValidFrom     time.Time
	ValidUntil    *time.Time
	CreatedBy     string
}

// CreateDiscountCode creates a new discount code.
func (s *Service) CreateDiscountCode(ctx context.Context, tenantID pgtype.UUID, input CreateDiscountCodeInput) (gen.DiscountCode, error) {
	if input.Code == "" {
		return gen.DiscountCode{}, errors.New("code is required")
	}

	if input.DiscountType == "" {
		return gen.DiscountCode{}, errors.New("discount_type is required")
	}

	if input.DiscountType != DiscountTypePercent && input.DiscountType != DiscountTypeFixed {
		return gen.DiscountCode{}, fmt.Errorf("invalid discount_type: %s", input.DiscountType)
	}

	if input.ApplicableTo == "" {
		return gen.DiscountCode{}, errors.New("applicable_to is required")
	}

	var maxUses *int32
	if input.MaxUses != nil {
		maxUses = input.MaxUses
	}

	var groupID pgtype.UUID
	if input.GroupID != "" {
		if err := groupID.Scan(input.GroupID); err != nil {
			return gen.DiscountCode{}, fmt.Errorf("invalid group_id: %w", err)
		}
	}

	var createdBy pgtype.UUID
	if input.CreatedBy != "" {
		if err := createdBy.Scan(input.CreatedBy); err != nil {
			return gen.DiscountCode{}, fmt.Errorf("invalid created_by: %w", err)
		}
	}

	usesPerUser := input.UsesPerUser
	if usesPerUser <= 0 {
		usesPerUser = 1
	}

	code, err := s.store.createDiscountCode(ctx, gen.CreateDiscountCodeParams{
		TenantID:      tenantID,
		Code:          input.Code,
		DiscountType:  input.DiscountType,
		DiscountValue: float64ToNumeric(input.DiscountValue),
		ApplicableTo:  input.ApplicableTo,
		MaxUses:       maxUses,
		UsesPerUser:   usesPerUser,
		GroupID:       groupID,
		ValidFrom:     pgtype.Timestamptz{Time: input.ValidFrom, Valid: true},
		ValidUntil: func() pgtype.Timestamptz {
			if input.ValidUntil != nil {
				return pgtype.Timestamptz{Time: *input.ValidUntil, Valid: true}
			}
			return pgtype.Timestamptz{}
		}(),
		CreatedBy: createdBy,
	})
	if err != nil {
		return gen.DiscountCode{}, err
	}

	return code, nil
}

// GetDiscountCode retrieves a discount code by code string.
func (s *Service) GetDiscountCode(ctx context.Context, tenantID pgtype.UUID, code string) (gen.DiscountCode, error) {
	return s.store.getDiscountCodeByCode(ctx, tenantID, code)
}

// ListDiscountCodes returns all discount codes for a tenant.
func (s *Service) ListDiscountCodes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.DiscountCode, error) {
	return s.store.listDiscountCodes(ctx, tenantID, limit, offset)
}

// DeleteDiscountCode deletes a discount code.
func (s *Service) DeleteDiscountCode(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteDiscountCode(ctx, id, tenantID)
}

// ValidateDiscountCode validates a discount code for a user and purpose.
func (s *Service) ValidateDiscountCode(ctx context.Context, tenantID, userID pgtype.UUID, code, purposeType string) (gen.DiscountCode, error) {
	discountCode, err := s.store.getDiscountCodeByCode(ctx, tenantID, code)
	if err != nil {
		return gen.DiscountCode{}, errors.New("invalid discount code")
	}

	if err := s.validateDiscountCode(ctx, discountCode, userID); err != nil {
		return gen.DiscountCode{}, err
	}

	// Check applicability
	if discountCode.ApplicableTo != ApplicableToAll && discountCode.ApplicableTo != purposeType {
		return gen.DiscountCode{}, fmt.Errorf("discount code not applicable to %s", purposeType)
	}

	return discountCode, nil
}

// validateDiscountCode performs all validation checks on a discount code.
func (s *Service) validateDiscountCode(ctx context.Context, code gen.DiscountCode, userID pgtype.UUID) error {
	now := time.Now()

	// Check validity period
	if code.ValidFrom.Valid && now.Before(code.ValidFrom.Time) {
		return errors.New("discount code not yet valid")
	}

	if code.ValidUntil.Valid && now.After(code.ValidUntil.Time) {
		return errors.New("discount code expired")
	}

	// Check max uses (if limited)
	if code.MaxUses != nil {
		// Count total uses
		uses, err := s.store.listDiscountCodeUses(ctx, code.ID, 1000, 0)
		if err != nil {
			return fmt.Errorf("checking discount code uses: %w", err)
		}

		if int32(len(uses)) >= *code.MaxUses { //nolint:gosec // G115: len bounded by limit param (1000)
			return errors.New("discount code usage limit reached")
		}
	}

	// Check per-user uses
	userUses, err := s.store.countDiscountCodeUsesByUser(ctx, code.ID, userID)
	if err != nil {
		return fmt.Errorf("checking user discount code uses: %w", err)
	}

	if userUses >= code.UsesPerUser {
		return errors.New("discount code usage limit reached for user")
	}

	return nil
}

// calculateDiscount calculates the discount amount based on code type.
func (s *Service) calculateDiscount(code gen.DiscountCode, amount int64) int64 {
	discountValue, err := code.DiscountValue.Float64Value()
	if err != nil || !discountValue.Valid {
		return 0
	}

	switch code.DiscountType {
	case DiscountTypePercent:
		// Calculate percentage discount
		discount := float64(amount) * (discountValue.Float64 / 100.0)
		return int64(discount)
	case DiscountTypeFixed:
		// Fixed discount in same currency unit (kobo/cents)
		return int64(discountValue.Float64)
	default:
		return 0
	}
}

// ApplyDiscountCodeUse records that a user used a discount code.
func (s *Service) ApplyDiscountCodeUse(ctx context.Context, codeID, userID, paymentID pgtype.UUID) error {
	_, err := s.store.insertDiscountCodeUse(ctx, gen.InsertDiscountCodeUseParams{
		CodeID:    codeID,
		UserID:    userID,
		PaymentID: paymentID,
	})
	if err != nil {
		return fmt.Errorf("record discount code use: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Refunds
// ---------------------------------------------------------------------------

// CreateRefundInput holds parameters for creating a refund.
type CreateRefundInput struct {
	PaymentID   string
	Amount      int64
	Reason      string
	ProviderRef string
}

// CreateRefund creates a new refund for a payment.
func (s *Service) CreateRefund(ctx context.Context, input CreateRefundInput) (gen.PaymentRefund, error) {
	if input.PaymentID == "" {
		return gen.PaymentRefund{}, errors.New("payment_id is required")
	}

	if input.Amount <= 0 {
		return gen.PaymentRefund{}, errors.New("amount must be positive")
	}

	var paymentID pgtype.UUID
	if err := paymentID.Scan(input.PaymentID); err != nil {
		return gen.PaymentRefund{}, fmt.Errorf("invalid payment_id: %w", err)
	}

	var reason *string
	if input.Reason != "" {
		reason = &input.Reason
	}

	var providerRef *string
	if input.ProviderRef != "" {
		providerRef = &input.ProviderRef
	}

	refund, err := s.store.createRefund(ctx, gen.CreateRefundParams{
		PaymentID:   paymentID,
		Amount:      input.Amount,
		Reason:      reason,
		Status:      RefundStatusPending,
		ProviderRef: providerRef,
	})
	if err != nil {
		return gen.PaymentRefund{}, err
	}

	return refund, nil
}

// ListRefundsByPayment returns refunds for a payment.
func (s *Service) ListRefundsByPayment(ctx context.Context, paymentID pgtype.UUID) ([]gen.PaymentRefund, error) {
	return s.store.listRefundsByPayment(ctx, paymentID)
}

// float64ToNumeric converts a float64 to pgtype.Numeric.
func float64ToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(fmt.Sprintf("%f", f)); err != nil {
		return pgtype.Numeric{}
	}

	return n
}
