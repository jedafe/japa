package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrInsufficientCredit is returned when a user lacks sufficient credits for a purchase.
var ErrInsufficientCredit = errors.New("insufficient credits balance")

// CreditSpendInput holds parameters for spending credits via the wallet.
type CreditSpendInput struct {
	Amount      int64
	Source      string
	Reference   string
	PurposeType string
	PurposeID   string
}

// CreditSpender defines the interface for spending credits from a wallet.
// The wallet.Service type satisfies this interface.
type CreditSpender interface {
	SpendCredits(
		ctx context.Context,

		userID pgtype.UUID,
		input CreditSpendInput,
	) (int64, error)
}

// PayWithCredit spends credits and records the payment in a single logical flow.
//
// This is a direct, synchronous ledger debit — it does not use the Provider
// abstraction because there is no external party, no Send() call, and no webhook
// to verify. The credit balance is checked and debited via the wallet, then a
// payments row is created with provider="credit" and status="succeeded".
//
// Transaction ordering (BUG-004): the payment row is created FIRST with
// status="pending", then credits are debited. If the debit fails (insufficient
// balance), the pending payment row remains and can be cleaned up by a
// background sweep. If the debit succeeds, the payment status is updated to
// "succeeded". This prevents the unrecoverable ledger drift where credits are
// spent but no payment record exists (JEOS §17 — "written to the payments row
// and the Section 28 Credit ledger in the same transaction").
func (s *Service) PayWithCredit(
	ctx context.Context,

	userID pgtype.UUID,
	amount int64,
	purposeType string,
	purposeID string,
) (int64, error) {
	if s.wallet == nil {
		return 0, errors.New("wallet service not configured")
	}

	if amount <= 0 {
		return 0, errors.New("amount must be positive")
	}

	if purposeType == "" {
		return 0, errors.New("purpose_type is required")
	}

	// Read currency from system_settings (Rule 3) with NGN fallback.
	currency := "NGN"

	if s.settings != nil {
		if v, err := s.settings.GetSetting(ctx, "payment.currency"); err == nil && v != "" {
			currency = v
		}
	}

	// Step 1: Create the payment row with status="pending" FIRST. If this
	// fails, no credits are debited — the user's balance is untouched.
	payment, err := s.createPaymentRow(ctx, userID, CreatePaymentInput{
		Provider:    ProviderCredit,
		PurposeType: purposeType,
		PurposeID:   purposeID,
		Amount:      amount,
		Currency:    currency,
	}, currency, "", "", amount, 0, pgtype.UUID{}, PaymentStatusPending)
	if err != nil {
		return 0, fmt.Errorf("recording credit payment: %w", err)
	}

	// Step 2: Debit credits from the wallet (checks balance, writes ledger
	// entry within its own transaction).
	input := CreditSpendInput{
		Amount:      amount,
		Source:      "payment",
		Reference:   fmt.Sprintf("payment:%s:%s", purposeType, purposeID),
		PurposeType: purposeType,
		PurposeID:   purposeID,
	}

	newBalance, err := s.wallet.SpendCredits(ctx, userID, input)
	if err != nil {
		// Debit failed — mark the payment as failed so it's not orphaned.
		_, _ = s.store.updatePaymentStatus(ctx, payment.ID, PaymentStatusFailed)
		return 0, fmt.Errorf("%w: %w", ErrInsufficientCredit, err)
	}

	// Step 3: Mark the payment as succeeded — the debit committed successfully.
	succeeded, err := s.store.updatePaymentStatus(ctx, payment.ID, PaymentStatusSucceeded)
	if err != nil {
		// The debit succeeded but the status update failed. This is a
		// recoverable state — the payment row exists with status="pending"
		// and the ledger entry exists. A background reconciliation task
		// can detect this and update the status. Log the error but return
		// success since the user's credits were correctly debited.
		slog.Error("payments: failed to mark credit payment as succeeded",
			"payment_id", payment.ID, "error", err)
	} else {
		s.enqueueServiceBookingCreated(succeeded)
	}

	return newBalance, nil
}
