package payments

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestPaymentConstants(t *testing.T) {
	t.Parallel()
	// Test provider constants
	if ProviderPaystack != "paystack" {
		t.Errorf("ProviderPaystack = %s, want paystack", ProviderPaystack)
	}

	if ProviderStripe != "stripe" {
		t.Errorf("ProviderStripe = %s, want stripe", ProviderStripe)
	}

	if ProviderNOWPayments != "nowpayments" {
		t.Errorf("ProviderNOWPayments = %s, want nowpayments", ProviderNOWPayments)
	}

	if ProviderManualTransfer != "manual_transfer" {
		t.Errorf("ProviderManualTransfer = %s, want manual_transfer", ProviderManualTransfer)
	}

	if ProviderCredit != "credit" {
		t.Errorf("ProviderCredit = %s, want credit", ProviderCredit)
	}

	// Test purpose type constants
	if PurposeProMembership != "pro_membership" {
		t.Errorf("PurposeProMembership = %s, want pro_membership", PurposeProMembership)
	}

	if PurposeOrgBoost != "org_boost" {
		t.Errorf("PurposeOrgBoost = %s, want org_boost", PurposeOrgBoost)
	}

	if PurposeDonation != "donation" {
		t.Errorf("PurposeDonation = %s, want donation", PurposeDonation)
	}

	if PurposeCourseAccess != "course_access" {
		t.Errorf("PurposeCourseAccess = %s, want course_access", PurposeCourseAccess)
	}

	if PurposeCreditPurchase != "credit_purchase" {
		t.Errorf("PurposeCreditPurchase = %s, want credit_purchase", PurposeCreditPurchase)
	}

	if PurposeAdSlot != "ad_slot" {
		t.Errorf("PurposeAdSlot = %s, want ad_slot", PurposeAdSlot)
	}

	// Test payment status constants
	if PaymentStatusPending != "pending" {
		t.Errorf("PaymentStatusPending = %s, want pending", PaymentStatusPending)
	}

	if PaymentStatusProcessing != "processing" {
		t.Errorf("PaymentStatusProcessing = %s, want processing", PaymentStatusProcessing)
	}

	if PaymentStatusSucceeded != "succeeded" {
		t.Errorf("PaymentStatusSucceeded = %s, want succeeded", PaymentStatusSucceeded)
	}

	if PaymentStatusFailed != "failed" {
		t.Errorf("PaymentStatusFailed = %s, want failed", PaymentStatusFailed)
	}

	if PaymentStatusRefunded != "refunded" {
		t.Errorf("PaymentStatusRefunded = %s, want refunded", PaymentStatusRefunded)
	}

	if PaymentStatusDisputed != "disputed" {
		t.Errorf("PaymentStatusDisputed = %s, want disputed", PaymentStatusDisputed)
	}

	// Test subscription status constants
	if SubscriptionStatusActive != "active" {
		t.Errorf("SubscriptionStatusActive = %s, want active", SubscriptionStatusActive)
	}

	if SubscriptionStatusCanceled != "canceled" {
		t.Errorf("SubscriptionStatusCanceled = %s, want canceled", SubscriptionStatusCanceled)
	}

	if SubscriptionStatusPastDue != "past_due" {
		t.Errorf("SubscriptionStatusPastDue = %s, want past_due", SubscriptionStatusPastDue)
	}

	if SubscriptionStatusTrialing != "trialing" {
		t.Errorf("SubscriptionStatusTrialing = %s, want trialing", SubscriptionStatusTrialing)
	}

	// Test subscription plan constants
	if PlanProMonthly != "pro_monthly" {
		t.Errorf("PlanProMonthly = %s, want pro_monthly", PlanProMonthly)
	}

	if PlanProYearly != "pro_yearly" {
		t.Errorf("PlanProYearly = %s, want pro_yearly", PlanProYearly)
	}

	// Test refund status constants
	if RefundStatusPending != "pending" {
		t.Errorf("RefundStatusPending = %s, want pending", RefundStatusPending)
	}

	if RefundStatusSucceeded != "succeeded" {
		t.Errorf("RefundStatusSucceeded = %s, want succeeded", RefundStatusSucceeded)
	}

	if RefundStatusFailed != "failed" {
		t.Errorf("RefundStatusFailed = %s, want failed", RefundStatusFailed)
	}

	// Test discount type constants
	if DiscountTypePercent != "percent" {
		t.Errorf("DiscountTypePercent = %s, want percent", DiscountTypePercent)
	}

	if DiscountTypeFixed != "fixed" {
		t.Errorf("DiscountTypeFixed = %s, want fixed", DiscountTypeFixed)
	}

	// Test discount applicability constants
	if ApplicableToCourseAccess != "course_access" {
		t.Errorf("ApplicableToCourseAccess = %s, want course_access", ApplicableToCourseAccess)
	}

	if ApplicableToProMembership != "pro_membership" {
		t.Errorf("ApplicableToProMembership = %s, want pro_membership", ApplicableToProMembership)
	}

	if ApplicableToAll != "all" {
		t.Errorf("ApplicableToAll = %s, want all", ApplicableToAll)
	}

	// Test pagination defaults
	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

func TestProviderInterface(t *testing.T) {
	t.Parallel()
	// Test that providers implement the Provider interface
	var _ Provider = (*PaystackProvider)(nil)

	var _ Provider = (*StripeProvider)(nil)

	var _ Provider = (*NOWPaymentsProvider)(nil)

	var _ Provider = (*ManualTransferProvider)(nil)
}

func TestProviderNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		provider Provider
		want     string
	}{
		{NewPaystackProvider("test"), ProviderPaystack},
		{NewStripeProvider("test"), ProviderStripe},
		{NewNOWPaymentsProvider("test"), ProviderNOWPayments},
		{NewManualTransferProvider(), ProviderManualTransfer},
	}

	for _, tt := range tests {
		if got := tt.provider.Name(); got != tt.want {
			t.Errorf("provider.Name() = %s, want %s", got, tt.want)
		}
	}
}

func TestPaymentStatusTransitions(t *testing.T) {
	t.Parallel()
	// Test that payment statuses are distinct
	statuses := map[string]bool{
		PaymentStatusPending:    true,
		PaymentStatusProcessing: true,
		PaymentStatusSucceeded:  true,
		PaymentStatusFailed:     true,
		PaymentStatusRefunded:   true,
		PaymentStatusDisputed:   true,
	}

	for status := range statuses {
		if status == "" {
			t.Error("Empty payment status found")
		}
	}

	// Verify all statuses are unique
	if len(statuses) != 6 {
		t.Errorf("Expected 6 payment statuses, got %d", len(statuses))
	}
}

func TestSubscriptionStatusTransitions(t *testing.T) {
	t.Parallel()
	// Test that subscription statuses are distinct
	statuses := map[string]bool{
		SubscriptionStatusActive:   true,
		SubscriptionStatusCanceled: true,
		SubscriptionStatusPastDue:  true,
		SubscriptionStatusTrialing: true,
	}

	for status := range statuses {
		if status == "" {
			t.Error("Empty subscription status found")
		}
	}

	// Verify all statuses are unique
	if len(statuses) != 4 {
		t.Errorf("Expected 4 subscription statuses, got %d", len(statuses))
	}
}

func TestDiscountCodeValidation(t *testing.T) {
	t.Parallel()
	// Test that discount types are valid
	validTypes := map[string]bool{
		DiscountTypePercent: true,
		DiscountTypeFixed:   true,
	}

	for dt := range validTypes {
		if dt == "" {
			t.Error("Empty discount type found")
		}
	}

	// Verify all types are unique
	if len(validTypes) != 2 {
		t.Errorf("Expected 2 discount types, got %d", len(validTypes))
	}
}

// ---------------------------------------------------------------------------
// Credit Payment Tests
// ---------------------------------------------------------------------------

// mockCreditSpender implements CreditSpender for testing.
type mockCreditSpender struct {
	spendFunc func(ctx context.Context, tenantID, userID pgtype.UUID, input CreditSpendInput) (int64, error)
}

func (m *mockCreditSpender) SpendCredits(
	ctx context.Context,
	tenantID pgtype.UUID,
	userID pgtype.UUID,
	input CreditSpendInput,
) (int64, error) {
	return m.spendFunc(ctx, tenantID, userID, input)
}

func TestPayWithCredit_WalletNotConfigured(t *testing.T) {
	t.Parallel()

	svc := &Service{wallet: nil}

	_, err := svc.PayWithCredit(t.Context(), pgtype.UUID{}, pgtype.UUID{}, 100, PurposeAdSlot, "slot-1")
	if err == nil {
		t.Fatal("expected error for nil wallet, got nil")
	}
}

func TestPayWithCredit_ZeroAmount(t *testing.T) {
	t.Parallel()

	svc := &Service{wallet: &mockCreditSpender{}}

	_, err := svc.PayWithCredit(t.Context(), pgtype.UUID{}, pgtype.UUID{}, 0, PurposeAdSlot, "slot-1")
	if err == nil {
		t.Fatal("expected error for zero amount, got nil")
	}
}

func TestPayWithCredit_EmptyPurposeType(t *testing.T) {
	t.Parallel()

	svc := &Service{wallet: &mockCreditSpender{}}

	_, err := svc.PayWithCredit(t.Context(), pgtype.UUID{}, pgtype.UUID{}, 100, "", "slot-1")
	if err == nil {
		t.Fatal("expected error for empty purpose_type, got nil")
	}
}

func TestPayWithCredit_ErrInsufficientCredit(t *testing.T) {
	t.Parallel()

	svc := &Service{
		wallet: &mockCreditSpender{
			spendFunc: func(
				_ context.Context,
				_ pgtype.UUID,
				_ pgtype.UUID,
				_ CreditSpendInput,
			) (int64, error) {
				return 0, ErrInsufficientCredit
			},
		},
	}

	_, err := svc.PayWithCredit(t.Context(), pgtype.UUID{}, pgtype.UUID{}, 100, PurposeAdSlot, "slot-1")
	if err == nil {
		t.Fatal("expected ErrInsufficientCredit, got nil")
	}

	if !errors.Is(err, ErrInsufficientCredit) {
		t.Fatalf("expected ErrInsufficientCredit, got %v", err)
	}
}

func TestPayWithCredit_Success(t *testing.T) {
	t.Parallel()

	var capturedInput CreditSpendInput

	svc := &Service{
		wallet: &mockCreditSpender{
			spendFunc: func(
				_ context.Context,
				_ pgtype.UUID,
				_ pgtype.UUID,
				input CreditSpendInput,
			) (int64, error) {
				capturedInput = input
				return 900, nil
			},
		},
	}

	// PayWithCredit calls CreatePayment internally, which requires a database.
	// This test verifies SpendCredits is called with correct parameters before
	// the CreatePayment step. The CreatePayment failure is expected in this
	// unit-test context since no database is configured.
	_, _ = svc.PayWithCredit(t.Context(), pgtype.UUID{}, pgtype.UUID{}, 100, PurposeAdSlot, "slot-1")

	if capturedInput.Amount != 100 {
		t.Errorf("capturedInput.Amount = %d, want 100", capturedInput.Amount)
	}

	if capturedInput.PurposeType != PurposeAdSlot {
		t.Errorf("capturedInput.PurposeType = %s, want %s", capturedInput.PurposeType, PurposeAdSlot)
	}

	if capturedInput.PurposeID != "slot-1" {
		t.Errorf("capturedInput.PurposeID = %s, want slot-1", capturedInput.PurposeID)
	}

	if capturedInput.Source != "payment" {
		t.Errorf("capturedInput.Source = %s, want payment", capturedInput.Source)
	}
}

func TestPurposeAdSlotConstant(t *testing.T) {
	t.Parallel()

	if PurposeAdSlot != "ad_slot" {
		t.Errorf("PurposeAdSlot = %s, want ad_slot", PurposeAdSlot)
	}
}
