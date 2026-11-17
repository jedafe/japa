package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
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

// ErrSpendNotPermitted is returned when the caller lacks the
// wallet.credits.spend grant — the RBAC gate for the credits payment rail
// (pilot-group rollouts assign the node before opening the feature).
var ErrSpendNotPermitted = errors.New("spending credits is not enabled for this account")

// permChecker resolves the wallet.credits.spend RBAC gate — nil seam means
// no gate (tests, internal callers that wire it explicitly).
type permChecker interface {
	Can(ctx context.Context, userID pgtype.UUID, node string) bool
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

	// RBAC gate — the credits payment rail is a gated feature (§28);
	// ops assigns wallet.credits.spend to member tiers / pilot groups.
	if s.perms != nil && !s.perms.Can(ctx, userID, "wallet.credits.spend") {
		return 0, ErrSpendNotPermitted
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

	// Step 3: Mark the payment as succeeded — the debit committed
	// successfully. Retry once: the only recoverable states are a
	// committed debit with a pending row (transient update failure) —
	// the payments:reconcile_credit sweep is the durable backstop.
	succeeded, err := s.store.updatePaymentStatus(ctx, payment.ID, PaymentStatusSucceeded)
	if err != nil {
		succeeded, err = s.store.updatePaymentStatus(ctx, payment.ID, PaymentStatusSucceeded)
	}
	if err != nil {
		slog.Error("payments: failed to mark credit payment as succeeded",
			"payment_id", payment.ID, "error", err)
	} else {
		// Synchronous-settlement fan-out — covers enrollment, credit grants,
		// referral conversion, donation/tip settle, form activation, and
		// §111 booking fan-out (the worker dispatch enqueues it identically).
		s.enqueueSyncSettlement(succeeded)
	}

	return newBalance, nil
}

// ReconcileStaleCreditPayments is the payments:reconcile_credit sweep —
// closes credit payments left pending when the debit committed but the
// status update failed (the create→debit→update flow is not one DB
// transaction; §17 acknowledges the recoverable gap). The credits_ledger
// spend row is the proof: debit found → mark succeeded + fan out the
// settlement effects that were skipped; no debit → mark failed so the row
// stops dangling.
func (s *Service) ReconcileStaleCreditPayments(ctx context.Context, limit int32) error {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.store.read.ListStalePendingCreditPayments(ctx, limit)
	if err != nil {
		return err
	}
	for _, p := range rows {
		reference := "payment:" + p.PurposeType + ":"
		if p.PurposeID.Valid {
			reference += p.PurposeID.String()
		}
		debited, derr := s.store.read.HasCreditDebitForReference(ctx, gen.HasCreditDebitForReferenceParams{
			UserID:    p.UserID,
			Reference: &reference,
		})
		status := PaymentStatusFailed
		if derr == nil && debited {
			status = PaymentStatusSucceeded
		}
		updated, uerr := s.store.updatePaymentStatus(ctx, p.ID, status)
		if uerr != nil {
			slog.Warn("payments: reconcile credit payment failed", "payment_id", p.ID, "error", uerr)
			continue
		}
		if status == PaymentStatusSucceeded {
			s.enqueueSyncSettlement(updated)
		}
	}
	return nil
}
