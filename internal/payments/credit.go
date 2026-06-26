package payments

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrInsufficientCredit is returned when a user lacks sufficient credits for a purchase.
var ErrInsufficientCredit = fmt.Errorf("insufficient credits balance")

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
		tenantID pgtype.UUID,
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
func (s *Service) PayWithCredit(
	ctx context.Context,
	tenantID pgtype.UUID,
	userID pgtype.UUID,
	amount int64,
	purposeType string,
	purposeID string,
) (int64, error) {
	if s.wallet == nil {
		return 0, fmt.Errorf("wallet service not configured")
	}
	if amount <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}
	if purposeType == "" {
		return 0, fmt.Errorf("purpose_type is required")
	}

	input := CreditSpendInput{
		Amount:      amount,
		Source:      "payment",
		Reference:   fmt.Sprintf("payment:%s:%s", purposeType, purposeID),
		PurposeType: purposeType,
		PurposeID:   purposeID,
	}

	// Debit credits from the wallet (checks balance, writes ledger entry).
	newBalance, err := s.wallet.SpendCredits(ctx, tenantID, userID, input)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInsufficientCredit, err)
	}

	// Record the payment row — single source of truth for "what was purchased."
	_, err = s.CreatePayment(ctx, tenantID, userID, CreatePaymentInput{
		Provider:    ProviderCredit,
		PurposeType: purposeType,
		PurposeID:   purposeID,
		Amount:      amount,
		Currency:    "NGN",
	})
	if err != nil {
		return 0, fmt.Errorf("recording credit payment: %w", err)
	}

	return newBalance, nil
}