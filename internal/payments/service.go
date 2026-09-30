package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	mw "github.com/jedafe/japa/internal/middleware"
	"github.com/jedafe/japa/internal/worker"
)

// CreatePaymentInput holds parameters for creating a new payment.
type CreatePaymentInput struct {
	Provider     string
	PurposeType  string
	PurposeID    string
	Amount       int64
	Currency     string
	Email        string // Payer email — required by Paystack /transaction/initialize
	Metadata     map[string]interface{}
	DiscountCode string
	// ReturnURL is where external providers send the user after checkout
	// (Paystack callback_url, Stripe success/cancel URL). Passed through from
	// the request body's return_url field.
	ReturnURL string
}

// CreatePaymentResult — CreatePayment creates a new payment record and initiates the payment with the provider.
// CreatePaymentResult holds the created payment row and the provider's
// checkout URL (for external providers). Credit payments have an empty
// checkout URL since there's no external checkout step.
type CreatePaymentResult struct {
	Payment     gen.Payment
	CheckoutURL string
}

func (s *Service) CreatePayment(ctx context.Context, userID pgtype.UUID, input CreatePaymentInput) (CreatePaymentResult, error) {
	if input.Provider == "" {
		return CreatePaymentResult{}, errors.New("provider is required")
	}

	if input.PurposeType == "" {
		return CreatePaymentResult{}, errors.New("purpose_type is required")
	}

	if input.Amount <= 0 {
		return CreatePaymentResult{}, errors.New("amount must be positive")
	}

	currency := input.Currency
	if currency == "" {
		currency = "NGN"

		if s.settings != nil {
			if v, err := s.settings.GetSetting(ctx, "payment.currency"); err == nil && v != "" {
				currency = v
			}
		}
	}

	discountCodeID, discountAmount, netAmount, err := s.resolveDiscount(ctx, userID, input)
	if err != nil {
		return CreatePaymentResult{}, err
	}

	// A 100% discount leaves nothing to charge. Record a succeeded payment
	// so the post-payment fulfillment (course enrollment) is triggered.
	// The payment row stores the original gross Amount and the DiscountAmount.
	if netAmount <= 0 {
		p, err := s.createPaymentRow(ctx, userID, input, currency, "", "", input.Amount, discountAmount, discountCodeID, PaymentStatusSucceeded)
		if err != nil {
			return CreatePaymentResult{}, err
		}

		if _, uErr := s.UpdatePaymentStatus(ctx, p.ID, PaymentStatusSucceeded); uErr != nil {
			slog.Error("payments: failed to emit succeeded webhook for zero-amount payment", "payment_id", p.ID, "error", uErr)
		}

		return CreatePaymentResult{Payment: p}, nil
	}

	// Credit payments are synchronous wallet debits — no external provider call.
	if input.Provider == ProviderCredit {
		p, err := s.payWithCreditInternal(ctx, userID, input, currency, netAmount, discountAmount, discountCodeID)
		if err != nil {
			return CreatePaymentResult{}, err
		}

		return CreatePaymentResult{Payment: p}, nil
	}

	// Initiate payment with external provider (Paystack, Stripe, NOWPayments).
	provider := s.providers[input.Provider]

	var providerRef, checkoutURL string

	if provider != nil {
		resp, err := provider.InitiatePayment(ctx, PaymentRequest{

			UserID:      userID.String(),
			Email:       input.Email,
			Amount:      netAmount,
			Currency:    currency,
			PurposeType: input.PurposeType,
			PurposeID:   input.PurposeID,
			Metadata:    input.Metadata,
			ReturnURL:   input.ReturnURL,
		})
		if err != nil {
			return CreatePaymentResult{}, fmt.Errorf("initiating payment with %s: %w", input.Provider, err)
		}

		providerRef = resp.Reference
		checkoutURL = resp.CheckoutURL
	}

	p, err := s.createPaymentRow(ctx, userID, input, currency, providerRef, checkoutURL, input.Amount, discountAmount, discountCodeID, PaymentStatusPending)
	if err != nil {
		return CreatePaymentResult{}, err
	}

	return CreatePaymentResult{Payment: p, CheckoutURL: checkoutURL}, nil
}

// resolveDiscount validates a discount code (if supplied) and returns the
// net amount to charge. input.Amount is the gross amount; discountAmount is
// the deduction computed from the code. netAmount is never negative.
func (s *Service) resolveDiscount(ctx context.Context, userID pgtype.UUID, input CreatePaymentInput) (pgtype.UUID, int64, int64, error) {
	var discountCodeID pgtype.UUID

	discountAmount := int64(0)
	netAmount := input.Amount

	if input.DiscountCode == "" {
		return discountCodeID, discountAmount, netAmount, nil
	}

	code, err := s.store.getDiscountCodeByCode(ctx, input.DiscountCode)
	if err != nil {
		return discountCodeID, discountAmount, netAmount, fmt.Errorf("invalid discount code: %w", err)
	}

	if err := s.validateDiscountCode(ctx, code, userID); err != nil {
		return discountCodeID, discountAmount, netAmount, err
	}

	discountCodeID = code.ID

	discountAmount = s.calculateDiscount(code, input.Amount)
	if discountAmount > input.Amount {
		discountAmount = input.Amount
	}

	netAmount = input.Amount - discountAmount
	if netAmount < 0 {
		netAmount = 0
	}

	return discountCodeID, discountAmount, netAmount, nil
}

// createPaymentRow inserts the payment record. amount is the gross invoice
// amount, discountAmount the deduction, and status the initial state.
func (s *Service) createPaymentRow(ctx context.Context, userID pgtype.UUID, input CreatePaymentInput, currency, providerRef, checkoutURL string, amount, discountAmount int64, discountCodeID pgtype.UUID, status string) (gen.Payment, error) {
	var purposeID pgtype.UUID
	if input.PurposeID != "" {
		if err := purposeID.Scan(input.PurposeID); err != nil {
			return gen.Payment{}, fmt.Errorf("invalid purpose_id: %w", err)
		}
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

	// DB-level idempotency key (BUG-015). Prefer the Idempotency-Key
	// header propagated via context by the idempotency middleware; fall
	// back to nil when no header is present (the HTTP middleware still
	// deduplicates at the request level via Valkey SET NX EX).
	var idempotencyKey *string
	if key := mw.IdempotencyKeyFromContext(ctx); key != "" {
		idempotencyKey = &key
	}

	payment, err := s.store.createPayment(ctx, gen.CreatePaymentParams{

		UserID:         userID,
		Provider:       input.Provider,
		ProviderRef:    ref,
		PurposeType:    input.PurposeType,
		PurposeID:      purposeID,
		Amount:         amount,
		Currency:       currency,
		Status:         status,
		Metadata:       metadata,
		DiscountCodeID: discountCodeID,
		DiscountAmount: discountAmount,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return gen.Payment{}, err
	}

	// Record the discount code use so max_uses and uses_per_user limits are
	// enforced on subsequent checkouts (JEOS §17 — "Applied at checkout: code
	// is validated... usage caps not exceeded"). The discount_code_uses table
	// has PRIMARY KEY (code_id, user_id), so a duplicate use by the same user
	// is rejected at the DB level.
	if discountCodeID.Valid {
		if err := s.ApplyDiscountCodeUse(ctx, discountCodeID, userID, payment.ID); err != nil {
			slog.Warn("payments: failed to record discount code use", "payment_id", payment.ID, "code_id", discountCodeID, "error", err)
			// Non-fatal: the payment is already created. The use just won't
			// be counted, which is a softer failure than blocking checkout.
		}
	}

	_ = checkoutURL

	return payment, nil
}

// payWithCreditInternal handles credit wallet payments.
// Uses pending-first ordering (BUG-004): create payment row, then debit
// credits, then mark succeeded. See PayWithCredit for full rationale.
func (s *Service) payWithCreditInternal(ctx context.Context, userID pgtype.UUID, input CreatePaymentInput, currency string, amount, discountAmount int64, discountCodeID pgtype.UUID) (gen.Payment, error) {
	if s.wallet == nil {
		return gen.Payment{}, errors.New("wallet service not configured")
	}

	// Step 1: Create the payment row with the original gross Amount and DiscountAmount.
	payment, err := s.createPaymentRow(ctx, userID, input, currency, "", "", input.Amount, discountAmount, discountCodeID, PaymentStatusPending)
	if err != nil {
		return gen.Payment{}, fmt.Errorf("recording credit payment: %w", err)
	}

	// Step 2: Debit credits from the wallet for the net amount.
	creditInput := CreditSpendInput{
		Amount:      amount,
		Source:      "payment",
		Reference:   fmt.Sprintf("payment:%s:%s", input.PurposeType, input.PurposeID),
		PurposeType: input.PurposeType,
		PurposeID:   input.PurposeID,
	}
	if _, err := s.wallet.SpendCredits(ctx, userID, creditInput); err != nil {
		// Debit failed — mark the payment as failed.
		failed, fErr := s.store.updatePaymentStatus(ctx, payment.ID, PaymentStatusFailed)
		if fErr != nil {
			slog.Error("payments: failed to mark credit payment as failed", "payment_id", payment.ID, "error", fErr)
		} else {
			s.emitPaymentStatusWebhook(ctx, failed)
		}

		return gen.Payment{}, fmt.Errorf("%w: %w", ErrInsufficientCredit, err)
	}

	// Step 3: Mark the payment as succeeded and return the updated row —
	// callers (e.g. the Learn checkout) read payment.status from the response,
	// so returning the stale pending row would misreport a completed payment.
	succeeded, err := s.store.updatePaymentStatus(ctx, payment.ID, PaymentStatusSucceeded)
	if err != nil {
		slog.Error("payments: failed to mark credit payment as succeeded", "payment_id", payment.ID, "error", err)
	} else {
		s.emitPaymentStatusWebhook(ctx, succeeded)
		return succeeded, nil
	}

	return payment, nil
}

// GetPayment retrieves a payment by ID.
func (s *Service) GetPayment(ctx context.Context, id pgtype.UUID) (gen.Payment, error) {
	return s.store.getPaymentByID(ctx, id)
}

// ListPaymentsByUser returns payments for a user.
func (s *Service) ListPaymentsByUser(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.Payment, error) {
	return s.store.listPaymentsByUser(ctx, userID, limit, offset)
}

// ListPaymentsByStatus returns payments with a specific status.
func (s *Service) ListPaymentsByStatus(ctx context.Context, status string, limit, offset int32) ([]gen.Payment, error) {
	return s.store.listPaymentsByStatus(ctx, status, limit, offset)
}

// UpdatePaymentStatus updates a payment's status.
func (s *Service) UpdatePaymentStatus(ctx context.Context, id pgtype.UUID, status string) (gen.Payment, error) {
	payment, err := s.store.updatePaymentStatus(ctx, id, status)
	if err != nil {
		return gen.Payment{}, err
	}

	s.emitPaymentStatusWebhook(ctx, payment)

	return payment, nil
}

// emitPaymentStatusWebhook fires payment.succeeded/payment.failed when a
// payment transitions to those terminal statuses (Section 12). No-op for
// other statuses. Fire-and-forget per Rule 8.
func (s *Service) emitPaymentStatusWebhook(ctx context.Context, payment gen.Payment) {
	var eventType string

	switch payment.Status {
	case PaymentStatusSucceeded:
		eventType = "payment.succeeded"
	case PaymentStatusFailed:
		eventType = "payment.failed"
	default:
		return
	}

	s.emitWebhook(ctx, eventType, map[string]any{
		"payment_id": payment.ID,
		"amount":     payment.Amount,
		"currency":   payment.Currency,
		"user_id":    payment.UserID,
	})
}

// ListAdminPayments lists payments with admin filters (JEOS §17).
func (s *Service) ListAdminPayments(ctx context.Context, params gen.ListAdminPaymentsParams) ([]gen.Payment, error) {
	return s.store.listAdminPayments(ctx, params)
}

// GetAdminPayment retrieves a payment for admin detail view (JEOS §17).
func (s *Service) GetAdminPayment(ctx context.Context, id pgtype.UUID) (gen.Payment, error) {
	return s.store.getAdminPaymentByID(ctx, id)
}

// ProcessWebhook processes a webhook event from a payment provider.
// Status indicates the event outcome ("succeeded", "failed", "refunded") and is extracted
// from the provider-specific payload by each HTTP handler before calling this method.
// // The idempotency check + insert + mark are performed atomically using
// INSERT ... ON CONFLICT DO NOTHING RETURNING to prevent the race where two
// concurrent webhook deliveries both pass the read check and both enqueue
// the worker task (Guide §13 — "Webhook replay processed twice → duplicate
// credit").
func (s *Service) ProcessWebhook(ctx context.Context, provider string, event WebhookEvent, status string) error {
	// Atomic insert-or-skip: if the event already exists (ON CONFLICT DO
	// NOTHING), no rows are returned and we treat it as already recorded.
	// This collapses the previous read-check + insert + mark into a single
	// atomic step that is race-free under concurrent delivery.
	inserted, err := s.store.insertWebhookEventIfNew(ctx, gen.InsertWebhookEventParams{

		Provider:   provider,
		EventID:    event.EventID,
		EventType:  event.EventType,
		RawPayload: event.RawPayload,
	})
	if err != nil {
		return fmt.Errorf("recording webhook event: %w", err)
	}

	// If the event already existed, check whether it was already processed.
	// If not (a prior enqueue failed — BUG-P011), re-enqueue the task. If
	// already processed, skip — the worker already handled this event.
	if !inserted {
		alreadyProcessed, err := s.store.isWebhookEventProcessed(ctx, provider, event.EventID)
		if err != nil {
			return fmt.Errorf("checking webhook event processed: %w", err)
		}

		if alreadyProcessed {
			return nil
		}
		// Event exists but was never processed — fall through to enqueue.
	}

	// Enqueue async processing BEFORE marking the event as processed
	// (BUG-P011). Subscription lifecycle events route to
	// subscription:webhook_process (§17 recurring billing); everything else
	// stays on payment:process_webhook. The provider_ref is passed so the
	// worker can match the payment directly (BUG-002 / BUG-019).
	if s.enqueuer != nil {
		taskType := worker.TypePaymentWebhookProcess
		payload, _ := json.Marshal(map[string]string{
			"event_id": event.EventID,

			"status":       status,
			"provider_ref": event.ProviderRef,
			"provider":     provider,
		})

		if isSubscriptionLifecycleEvent(provider, event.EventType, event.Payload) {
			taskType = worker.TypeSubscriptionWebhookProcess
			payload, _ = json.Marshal(map[string]interface{}{
				"provider":   provider,
				"event_id":   event.EventID,
				"event_type": event.EventType,
				"payload":    json.RawMessage(event.RawPayload),
			})
		}

		if err := s.enqueuer.Enqueue(taskType, payload); err != nil {
			slog.Error("payments: failed to enqueue webhook processing", "event_id", event.EventID, "provider", provider, "task", taskType, "error", err)
			errpkg.CaptureException(fmt.Errorf("enqueue %s (event %s): %w", taskType, event.EventID, err))
			// Return the error so the provider receives a 5xx and retries.
			// The event is NOT marked processed, so on retry we re-enqueue.
			return fmt.Errorf("enqueue %s (event %s): %w", taskType, event.EventID, err)
		}
	}

	// Mark as processed AFTER successful enqueue (BUG-P011). If the enqueue
	// failed above, the event stays unprocessed and the provider retry will
	// re-enqueue via the !inserted + !alreadyProcessed path.
	if err := s.store.markWebhookEventProcessed(ctx, provider, event.EventID); err != nil {
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

// CreateSubscription — see subscriptions.go. It returns a hosted-checkout
// URL; the subscriptions row is created by the provider's first-charge
// webhook (§17: deferred creation, webhook is the source of truth).

// GetSubscription retrieves a subscription by ID.
func (s *Service) GetSubscription(ctx context.Context, id pgtype.UUID) (gen.Subscription, error) {
	return s.store.getSubscriptionByID(ctx, id)
}

// GetActiveSubscription returns the active subscription for a user.
func (s *Service) GetActiveSubscription(ctx context.Context, userID pgtype.UUID) (gen.Subscription, error) {
	return s.store.getActiveSubscriptionByUser(ctx, userID)
}

// ListSubscriptionsByUser returns subscriptions for a user.
func (s *Service) ListSubscriptionsByUser(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.Subscription, error) {
	return s.store.listSubscriptionsByUser(ctx, userID, limit, offset)
}

// CancelSubscription / ReactivateSubscription — see subscriptions.go.
// Both call the provider first and only mutate local state on success.

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
func (s *Service) CreateDiscountCode(ctx context.Context, input CreateDiscountCodeInput) (gen.DiscountCode, error) {
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
func (s *Service) GetDiscountCode(ctx context.Context, code string) (gen.DiscountCode, error) {
	return s.store.getDiscountCodeByCode(ctx, code)
}

// ListDiscountCodes returns all discount codes for a platform.
func (s *Service) ListDiscountCodes(ctx context.Context, limit, offset int32) ([]gen.DiscountCode, error) {
	return s.store.listDiscountCodes(ctx, limit, offset)
}

// DeleteDiscountCode deletes a discount code.
func (s *Service) DeleteDiscountCode(ctx context.Context, id pgtype.UUID) error {
	return s.store.deleteDiscountCode(ctx, id)
}

// ValidateDiscountCode validates a discount code for a user and purpose.
func (s *Service) ValidateDiscountCode(ctx context.Context, userID pgtype.UUID, code, purposeType string) (gen.DiscountCode, error) {
	discountCode, err := s.store.getDiscountCodeByCode(ctx, code)
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

	// Check max uses (if limited) — use COUNT query instead of list+len
	// to avoid fetching up to 1000 rows and silently under-counting beyond
	// that limit (BUG-018).
	if code.MaxUses != nil {
		totalUses, err := s.store.countDiscountCodeUses(ctx, code.ID)
		if err != nil {
			return fmt.Errorf("checking discount code uses: %w", err)
		}

		if totalUses >= *code.MaxUses {
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
// Uses integer minor-unit math to avoid float precision loss (Guide §1.3).
func (s *Service) calculateDiscount(code gen.DiscountCode, amount int64) int64 {
	discountValue, err := code.DiscountValue.Float64Value()
	if err != nil || !discountValue.Valid {
		return 0
	}

	switch code.DiscountType {
	case DiscountTypePercent, "percentage":
		// Fractional percentage support (BUG-P006): the previous code
		// truncated the percentage to int64 (12.5% → 12%), losing the
		// fractional part. To preserve fractional percentages while
		// keeping integer minor-unit math, we scale by 10000 (basis
		// points): discount = (amount * pct_bps) / 10000, where
		// pct_bps = round(pct * 100). E.g. 12.5% → 1250 bps →
		// (amount * 1250) / 10000.
		pctBps := int64(discountValue.Float64 * 100)
		discount := (amount * pctBps) / 10000
		// Round the remainder to nearest whole minor unit.
		remainder := (amount * pctBps) % 10000
		if remainder >= 5000 {
			discount++
		}

		if discount > amount {
			discount = amount // cap at amount
		}

		return discount
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
	PaymentID string

	Amount      int64
	Reason      string
	ProviderRef string
}

// CreateRefund creates a new refund for a payment.
// // In addition to recording the refund row, this finalizes the payment's
// status transition to "refunded" and enqueues the webhook processor with
// status="refunded" so the worker runs reversePaymentSideEffects (course
// access revocation, org boost expiry, wallet credit reversal) per JEOS §17.
// Without this, an admin-initiated refund would leave a dangling pending
// refund row and never reverse the side effects of the original purchase
// (BUG-001 — Guide §13 "Refund path doesn't reverse the ledger entry").
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

	// Finalize the payment status as "refunded" so the canonical payment
	// row reflects the provider state and downstream queries see the
	// refund. The handler has already verified the payment was succeeded.
	updated, err := s.store.updatePaymentStatus(ctx, paymentID, PaymentStatusRefunded)
	if err != nil {
		// The refund row is recorded; surface the error but don't lose it.
		return refund, fmt.Errorf("updating payment status to refunded: %w", err)
	}

	// Enqueue the webhook processor with status="refunded" so the worker
	// runs reversePaymentSideEffects (course access, org boost, wallet
	// credit reversal) per JEOS §17. The provider_ref is forwarded so the
	// worker can match the payment. Fire-and-forget per Rule 8 — the
	// payment is already marked refunded.
	if s.enqueuer != nil {
		providerRefStr := ""
		if updated.ProviderRef != nil && *updated.ProviderRef != "" {
			providerRefStr = *updated.ProviderRef
		}

		payload, _ := json.Marshal(map[string]string{
			"event_id": "admin_refund:" + refund.ID.String(),

			"status":       PaymentStatusRefunded,
			"provider_ref": providerRefStr,
			"provider":     updated.Provider,
		})
		if err := s.enqueuer.Enqueue(worker.TypePaymentWebhookProcess, payload); err != nil {
			slog.Error("payments: failed to enqueue refund reversal", "payment_id", paymentID, "refund_id", refund.ID, "error", err)
			errpkg.CaptureException(fmt.Errorf("enqueue refund reversal (payment %s): %w", paymentID, err))
			// Non-fatal: the payment is already marked refunded. The
			// reversal can be retried by re-running the refund or by a
			// future sweep job.
		}
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
