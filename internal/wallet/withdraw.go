package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/wallet/payout"
)

// Withdrawal statuses.
const (
	WithdrawalStatusPending         = "pending"
	WithdrawalStatusPendingApproval = "pending_approval"
	WithdrawalStatusApproved        = "approved"
	WithdrawalStatusProcessing      = "processing"
	WithdrawalStatusCompleted       = "completed"
	WithdrawalStatusFailed          = "failed"
	WithdrawalStatusRejected        = "rejected"
	WithdrawalStatusCancelled       = "cancelled"
)

// CalculateWithdrawalInput holds parameters for previewing a withdrawal.
type CalculateWithdrawalInput struct {
	CreditAmount int64
	CurrencyCode string // optional; defaults to user's resolved currency
}

// CalculateWithdrawalResult holds the computed withdrawal preview.
type CalculateWithdrawalResult struct {
	CreditAmount   int64   `json:"credit_amount"`
	CurrencyCode   string  `json:"currency_code"`
	CreditsPerUnit int32   `json:"credits_per_unit"`
	FiatAmount     int64   `json:"fiat_amount"`
	FeeAmount      int64   `json:"fee_amount"`
	FeeBasis       string  `json:"fee_basis"`
	FeeRate        float64 `json:"fee_rate"`
	NetAmount      int64   `json:"net_amount"`
}

// CreateWithdrawalMethodInput holds parameters for adding a user payout method.
type CreateWithdrawalMethodInput struct {
	Provider     string
	Label        string
	Account      string
	CurrencyCode string
	IsDefault    bool
	Metadata     any
}

// CreateWithdrawalInput holds parameters for submitting a withdrawal.
type CreateWithdrawalInput struct {
	CreditAmount   int64
	CurrencyCode   string // optional
	PayoutMethodID string // optional; uses default if empty
	IdempotencyKey string
}

// WithdrawalResult is the public shape of a created withdrawal.
type WithdrawalResult struct {
	ID             string `json:"id"`
	CreditAmount   int64  `json:"credit_amount"`
	CurrencyCode   string `json:"currency_code"`
	CreditsPerUnit int32  `json:"credits_per_unit"`
	FiatAmount     int64  `json:"fiat_amount"`
	FeeAmount      int64  `json:"fee_amount"`
	NetAmount      int64  `json:"net_amount"`
	Status         string `json:"status"`
	PayoutProvider string `json:"payout_provider"`
	ProviderRef    string `json:"provider_ref,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// CalculateWithdrawal previews the fiat value, fee, and net amount for a
// proposed credit withdrawal without mutating state.
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md.
func (s *Service) CalculateWithdrawal(ctx context.Context, userID pgtype.UUID, input CalculateWithdrawalInput) (CalculateWithdrawalResult, error) {
	if input.CreditAmount <= 0 {
		return CalculateWithdrawalResult{}, newValidationError("credit_amount", "credit amount must be positive")
	}

	currency, err := s.resolveWithdrawalCurrency(ctx, userID, input.CurrencyCode)
	if err != nil {
		return CalculateWithdrawalResult{}, err
	}

	// Check minimum withdrawal amount.
	minCredits := 1000
	if s.settings != nil {
		minCredits = s.settings.GetIntOr(ctx, "wallet.min_withdrawal_credits", 1000)
	}

	if input.CreditAmount < int64(minCredits) {
		return CalculateWithdrawalResult{}, newValidationError("credit_amount", fmt.Sprintf("minimum withdrawal is %d credits", minCredits))
	}

	fiatAmount := CreditsToFiat(int(input.CreditAmount), currency)

	feeAmount, feeBasis, feeRate, err := s.calculateWithdrawalFee(ctx, input.CreditAmount, fiatAmount)
	if err != nil {
		return CalculateWithdrawalResult{}, err
	}

	netAmount := fiatAmount - feeAmount
	if netAmount <= 0 {
		return CalculateWithdrawalResult{}, newValidationError("credit_amount", "withdrawal amount too small after fee")
	}

	return CalculateWithdrawalResult{
		CreditAmount:   input.CreditAmount,
		CurrencyCode:   currency.Code,
		CreditsPerUnit: currency.CreditsPerUnit,
		FiatAmount:     fiatAmount,
		FeeAmount:      feeAmount,
		FeeBasis:       feeBasis,
		FeeRate:        feeRate,
		NetAmount:      netAmount,
	}, nil
}

// CreateWithdrawal submits a new credit withdrawal request, reserves credits
// from the user's wallet, and (if auto-approval is enabled and a provider is
// configured) initiates the payout.
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md.
func (s *Service) CreateWithdrawal(ctx context.Context, userID pgtype.UUID, input CreateWithdrawalInput) (WithdrawalResult, error) {
	if input.CreditAmount <= 0 {
		return WithdrawalResult{}, newValidationError("credit_amount", "credit amount must be positive")
	}

	if input.IdempotencyKey == "" {
		return WithdrawalResult{}, newValidationError("idempotency_key", "Idempotency-Key is required")
	}

	currency, err := s.resolveWithdrawalCurrency(ctx, userID, input.CurrencyCode)
	if err != nil {
		return WithdrawalResult{}, err
	}

	calc, err := s.CalculateWithdrawal(ctx, userID, CalculateWithdrawalInput{
		CreditAmount: input.CreditAmount,
		CurrencyCode: currency.Code,
	})
	if err != nil {
		return WithdrawalResult{}, err
	}

	// Verify the user has enough credits.
	_, credits, err := s.GetBalance(ctx, userID)
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("reading credits balance: %w", err)
	}

	if credits < input.CreditAmount {
		return WithdrawalResult{}, newValidationError("credit_amount", "insufficient credits")
	}

	// Resolve payout method.
	method, err := s.resolvePayoutMethod(ctx, userID, input.PayoutMethodID)
	if err != nil {
		return WithdrawalResult{}, err
	}

	// Determine if admin approval is required.
	status, requiresApproval, err := s.resolveWithdrawalStatus(ctx, userID, calc.CreditAmount)
	if err != nil {
		return WithdrawalResult{}, err
	}

	// Resolve payout provider only when not pending approval.
	providerType := method.Provider

	var provider payout.Provider
	if !requiresApproval {
		provider, err = s.resolvePayoutProvider(ctx, method)
		if err != nil {
			return WithdrawalResult{}, err
		}

		providerType = provider.Type()
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	// Serialize concurrent balance mutations for this user.
	lockKey := "wallet:" + "" + ":" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return WithdrawalResult{}, fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// Re-read balance inside the lock.
	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{

		UserID:     userID,
		LedgerType: LedgerTypeCredits,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return WithdrawalResult{}, fmt.Errorf("reading credits balance: %w", err)
		}

		currentBalance = 0
	}

	if currentBalance < input.CreditAmount {
		return WithdrawalResult{}, newValidationError("credit_amount", "insufficient credits")
	}

	newBalance := currentBalance - input.CreditAmount
	reference := input.IdempotencyKey
	reason := "credit withdrawal"

	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{

		UserID:           userID,
		LedgerType:       LedgerTypeCredits,
		Operation:        OperationWithdraw,
		Amount:           -input.CreditAmount,
		ResultingBalance: newBalance,
		Source:           nil,
		Reference:        &reference,
		IdempotencyKey:   &input.IdempotencyKey,
		Reason:           &reason,
	})
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("inserting withdrawal ledger entry: %w", err)
	}

	withdrawal, err := qtx.InsertWithdrawal(ctx, gen.InsertWithdrawalParams{

		UserID:         userID,
		CreditAmount:   input.CreditAmount,
		CurrencyCode:   calc.CurrencyCode,
		CreditsPerUnit: calc.CreditsPerUnit,
		FiatAmount:     calc.FiatAmount,
		FeeAmount:      calc.FeeAmount,
		FeeBasis:       calc.FeeBasis,
		FeeRate:        float64ToNumeric(calc.FeeRate),
		NetAmount:      calc.NetAmount,
		Status:         status,
		PayoutMethodID: method.ID,
		PayoutProvider: providerType,
		ProviderRef:    nil,
		AdminNote:      nil,
		ReviewedBy:     pgtype.UUID{},
		ReviewedAt:     pgtype.Timestamptz{},
		IdempotencyKey: &input.IdempotencyKey,
	})
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("inserting withdrawal: %w", err)
	}

	providerRef := ""

	if !requiresApproval && provider != nil {
		// Manual providers are a no-op at creation time; they require an
		// admin to review and mark the withdrawal completed. Real providers
		// (NOWPayments, etc.) can be initiated immediately.
		if provider.Type() != "manual" && provider.Type() != "manual_crypto" {
			resp, perr := s.initiatePayout(ctx, withdrawal, method, calc, provider)
			if perr != nil {
				slog.Error("payout initiation failed; marking withdrawal failed and refunding credits", "withdrawal_id", withdrawal.ID, "error", perr)

				_, uerr := qtx.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
					ID: withdrawal.ID,

					Status: WithdrawalStatusFailed,
				})
				if uerr != nil {
					slog.Error("failed to update withdrawal status to failed", "withdrawal_id", withdrawal.ID, "error", uerr)
				}

				ref := withdrawal.ID.String()
				reason := "payout initiation failed - refund"

				_, lerr := qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{

					UserID:           userID,
					LedgerType:       LedgerTypeCredits,
					Operation:        OperationRefund,
					Amount:           input.CreditAmount,
					ResultingBalance: currentBalance,
					Reference:        &ref,
					Reason:           &reason,
				})
				if lerr != nil {
					slog.Error("failed to insert refund ledger entry for failed auto payout", "withdrawal_id", withdrawal.ID, "error", lerr)
				} else {
					status = WithdrawalStatusFailed
				}
			} else {
				providerRef = resp.ProviderRef

				if resp.Status != "" {
					status = resp.Status
				} else {
					status = WithdrawalStatusProcessing
				}

				_, uerr := qtx.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
					ID: withdrawal.ID,

					Status:      status,
					ProviderRef: &providerRef,
				})
				if uerr != nil {
					slog.Error("failed to update withdrawal provider ref", "withdrawal_id", withdrawal.ID, "error", uerr)
				}
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return WithdrawalResult{}, fmt.Errorf("committing withdrawal: %w", err)
	}

	return WithdrawalResult{
		ID:             withdrawal.ID.String(),
		CreditAmount:   withdrawal.CreditAmount,
		CurrencyCode:   withdrawal.CurrencyCode,
		CreditsPerUnit: withdrawal.CreditsPerUnit,
		FiatAmount:     withdrawal.FiatAmount,
		FeeAmount:      withdrawal.FeeAmount,
		NetAmount:      withdrawal.NetAmount,
		Status:         status,
		PayoutProvider: withdrawal.PayoutProvider,
		ProviderRef:    providerRef,
		CreatedAt:      withdrawal.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// resolveWithdrawalCurrency returns the active currency row to use for the
// withdrawal. If input.CurrencyCode is empty, it resolves via the user's
// preferences and geo context.
func (s *Service) resolveWithdrawalCurrency(ctx context.Context, userID pgtype.UUID, explicit string) (gen.Currency, error) {
	// Explicit selection.
	if explicit != "" {
		currency, err := s.store.read.GetCurrencyByCode(ctx, explicit)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return gen.Currency{}, newValidationError("currency_code", "currency not found")
			}

			return gen.Currency{}, fmt.Errorf("loading currency: %w", err)
		}

		if !currency.IsActive {
			return gen.Currency{}, newValidationError("currency_code", "currency is not active")
		}

		return currency, nil
	}

	// Resolve from user preference and geo context.
	pref, err := s.store.read.GetUserPreferredCurrency(ctx, userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return gen.Currency{}, fmt.Errorf("loading user preferred currency: %w", err)
	}

	cc := CurrencyContext{
		PreferredCurrency: derefString(pref),
	}

	code, err := s.currencyResolver.ResolveCurrency(ctx, cc)
	if err != nil {
		return gen.Currency{}, fmt.Errorf("resolving currency: %w", err)
	}

	currency, err := s.store.read.GetCurrencyByCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.Currency{}, newValidationError("currency_code", "resolved currency not found")
		}

		return gen.Currency{}, fmt.Errorf("loading resolved currency: %w", err)
	}

	if !currency.IsActive {
		return gen.Currency{}, newValidationError("currency_code", "resolved currency is not active")
	}

	return currency, nil
}

// calculateWithdrawalFee resolves the first matching active fee tier and
// returns the fee amount, basis, and rate.
func (s *Service) calculateWithdrawalFee(ctx context.Context, creditAmount, fiatAmount int64) (int64, string, float64, error) {
	tiers, err := s.store.read.ListWithdrawalFeeTiers(ctx)
	if err != nil {
		return 0, "", 0, fmt.Errorf("loading fee tiers: %w", err)
	}

	var basisAmount int64

	for _, tier := range tiers {
		if tier.TierMode == "fiat" {
			basisAmount = fiatAmount
		} else {
			basisAmount = creditAmount
		}

		if basisAmount < int64(tier.MinAmount) {
			continue
		}

		if tier.MaxAmount != nil && *tier.MaxAmount != 0 && basisAmount > int64(*tier.MaxAmount) {
			continue
		}

		var fee int64

		var rate float64
		if tier.FeeType == "percentage" {
			// fee_value is basis points (e.g. 500 = 5%).
			rate = float64(tier.FeeValue) / 100.0
			fee = basisAmount * int64(tier.FeeValue) / 10000
		} else {
			rate = float64(tier.FeeValue)
			fee = int64(tier.FeeValue)
		}

		return fee, tier.FeeType, rate, nil
	}

	// No tier matched: no fee.
	return 0, "percentage", 0, nil
}

// resolvePayoutMethod loads the user's payout method, or their default if none
// is provided.
func (s *Service) resolvePayoutMethod(ctx context.Context, userID pgtype.UUID, methodID string) (gen.WalletWithdrawalMethod, error) {
	if methodID == "" {
		methods, err := s.store.read.ListWithdrawalMethodsByUser(ctx,

			userID,
		)
		if err != nil {
			return gen.WalletWithdrawalMethod{}, fmt.Errorf("loading payout methods: %w", err)
		}

		for _, m := range methods {
			if m.IsDefault {
				return m, nil
			}
		}

		if len(methods) > 0 {
			return methods[0], nil
		}

		return gen.WalletWithdrawalMethod{}, newValidationError("payout_method_id", "no payout method found; add one first")
	}

	id, err := parseUUID(methodID)
	if err != nil {
		return gen.WalletWithdrawalMethod{}, newValidationError("payout_method_id", "invalid payout method id")
	}

	method, err := s.store.read.GetWithdrawalMethodByID(ctx, gen.GetWithdrawalMethodByIDParams{
		ID: id,

		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.WalletWithdrawalMethod{}, newValidationError("payout_method_id", "payout method not found")
		}

		return gen.WalletWithdrawalMethod{}, fmt.Errorf("loading payout method: %w", err)
	}

	if !method.IsActive {
		return gen.WalletWithdrawalMethod{}, newValidationError("payout_method_id", "payout method is inactive")
	}

	return method, nil
}

// resolvePayoutProvider returns the configured provider for the given method.
func (s *Service) resolvePayoutProvider(ctx context.Context, method gen.WalletWithdrawalMethod) (payout.Provider, error) {
	// In-memory registered provider (NOWPayments, etc.) has priority.
	providerType := method.Provider
	if p, ok := s.payoutProviders[providerType]; ok {
		return p, nil
	}

	// Fall back to manual / unconfigured flow.
	if providerType == "manual_crypto" || providerType == "manual" {
		return &manualPayoutProvider{typ: providerType}, nil
	}

	return nil, newValidationError("payout_provider", "payout provider not configured for "+providerType)
}

// resolveWithdrawalStatus checks the user's group policy to decide whether the
// withdrawal requires admin approval.
func (s *Service) resolveWithdrawalStatus(ctx context.Context, userID pgtype.UUID, creditAmount int64) (string, bool, error) {
	group, err := s.store.read.GetUserGroupByUserID(ctx,

		userID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WithdrawalStatusPendingApproval, true, nil
		}

		return "", false, fmt.Errorf("loading user group: %w", err)
	}

	if group.RequiresWithdrawalApproval {
		return WithdrawalStatusPendingApproval, true, nil
	}

	if group.AutoWithdrawalLimit == nil {
		return WithdrawalStatusPendingApproval, true, nil
	}

	if *group.AutoWithdrawalLimit == 0 || creditAmount <= int64(*group.AutoWithdrawalLimit) {
		return WithdrawalStatusPending, false, nil
	}

	return WithdrawalStatusPendingApproval, true, nil
}

// ProcessWithdrawal approves a pending withdrawal and initiates the payout.
// It is called by admin action (PATCH /admin/wallet/withdrawals/{id} or
// POST /admin/wallet/withdrawals/{id}/process).
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md.
func (s *Service) ProcessWithdrawal(ctx context.Context, adminUserID, withdrawalID pgtype.UUID) (WithdrawalResult, error) {
	withdrawal, err := s.store.read.GetWithdrawalByID(ctx,
		withdrawalID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WithdrawalResult{}, errpkg.ErrNotFound
		}

		return WithdrawalResult{}, fmt.Errorf("loading withdrawal: %w", err)
	}

	if withdrawal.Status != WithdrawalStatusPendingApproval && withdrawal.Status != WithdrawalStatusPending {
		return WithdrawalResult{}, fmt.Errorf("%w: cannot process withdrawal in status %s", errpkg.ErrInvalidStatusTransition, withdrawal.Status)
	}

	method, err := s.store.read.GetWithdrawalMethodByID(ctx, gen.GetWithdrawalMethodByIDParams{
		ID: withdrawal.PayoutMethodID,

		UserID: withdrawal.UserID,
	})
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("loading payout method: %w", err)
	}

	calc := CalculateWithdrawalResult{
		CreditAmount:   withdrawal.CreditAmount,
		CurrencyCode:   withdrawal.CurrencyCode,
		CreditsPerUnit: withdrawal.CreditsPerUnit,
		FiatAmount:     withdrawal.FiatAmount,
		FeeAmount:      withdrawal.FeeAmount,
		FeeBasis:       withdrawal.FeeBasis,
		FeeRate:        0, // not needed for payout
		NetAmount:      withdrawal.NetAmount,
	}

	provider, err := s.resolvePayoutProvider(ctx, method)
	if err != nil {
		return WithdrawalResult{}, err
	}

	status := WithdrawalStatusApproved
	providerRef := ""

	resp, perr := s.initiatePayout(ctx, withdrawal, method, calc, provider)
	if perr != nil {
		slog.Error("payout initiation failed", "withdrawal_id", withdrawal.ID, "error", perr)

		status = WithdrawalStatusFailed
	} else {
		providerRef = resp.ProviderRef

		if resp.Status != "" {
			status = resp.Status
		} else {
			status = WithdrawalStatusProcessing
		}
	}

	updated, err := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
		ID: withdrawalID,

		Status:      status,
		ProviderRef: &providerRef,
		ReviewedBy:  adminUserID,
		ReviewedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("updating withdrawal status: %w", err)
	}

	// Refund credits on failure.
	if status == WithdrawalStatusFailed {
		if rerr := s.refundWithdrawalCredits(ctx, withdrawal.UserID, withdrawal.CreditAmount, withdrawalID); rerr != nil {
			slog.Error("failed to refund failed withdrawal", "withdrawal_id", withdrawal.ID, "error", rerr)
		}
	}

	return WithdrawalResult{
		ID:             updated.ID.String(),
		CreditAmount:   updated.CreditAmount,
		CurrencyCode:   updated.CurrencyCode,
		CreditsPerUnit: updated.CreditsPerUnit,
		FiatAmount:     updated.FiatAmount,
		FeeAmount:      updated.FeeAmount,
		NetAmount:      updated.NetAmount,
		Status:         status,
		PayoutProvider: updated.PayoutProvider,
		ProviderRef:    providerRef,
		CreatedAt:      updated.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// RejectWithdrawal rejects a pending or pending_approval withdrawal and
// refunds the user's credits.
func (s *Service) RejectWithdrawal(ctx context.Context, adminUserID, withdrawalID pgtype.UUID, note string) (WithdrawalResult, error) {
	withdrawal, err := s.store.read.GetWithdrawalByID(ctx,
		withdrawalID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WithdrawalResult{}, errpkg.ErrNotFound
		}

		return WithdrawalResult{}, fmt.Errorf("loading withdrawal: %w", err)
	}

	if withdrawal.Status != WithdrawalStatusPendingApproval && withdrawal.Status != WithdrawalStatusPending {
		return WithdrawalResult{}, fmt.Errorf("%w: cannot reject withdrawal in status %s", errpkg.ErrInvalidStatusTransition, withdrawal.Status)
	}

	var noteRef *string
	if note != "" {
		noteRef = &note
	}

	updated, err := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
		ID: withdrawalID,

		Status:     WithdrawalStatusRejected,
		AdminNote:  noteRef,
		ReviewedBy: adminUserID,
		ReviewedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("updating withdrawal status: %w", err)
	}

	if err := s.refundWithdrawalCredits(ctx, withdrawal.UserID, withdrawal.CreditAmount, withdrawalID); err != nil {
		slog.Error("failed to refund rejected withdrawal", "withdrawal_id", withdrawal.ID, "error", err)
	}

	return WithdrawalResult{
		ID:             updated.ID.String(),
		CreditAmount:   updated.CreditAmount,
		CurrencyCode:   updated.CurrencyCode,
		CreditsPerUnit: updated.CreditsPerUnit,
		FiatAmount:     updated.FiatAmount,
		FeeAmount:      updated.FeeAmount,
		NetAmount:      updated.NetAmount,
		Status:         WithdrawalStatusRejected,
		PayoutProvider: updated.PayoutProvider,
		CreatedAt:      updated.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// refundWithdrawalCredits re-credits the user's wallet when a payout fails.
func (s *Service) refundWithdrawalCredits(ctx context.Context, userID pgtype.UUID, creditAmount int64, withdrawalID pgtype.UUID) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning refund tx: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{

		UserID:     userID,
		LedgerType: LedgerTypeCredits,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("reading credits balance: %w", err)
		}

		currentBalance = 0
	}

	newBalance := currentBalance + creditAmount
	ref := withdrawalID.String()
	reason := "withdrawal payout failed - refund"

	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{

		UserID:           userID,
		LedgerType:       LedgerTypeCredits,
		Operation:        OperationRefund,
		Amount:           creditAmount,
		ResultingBalance: newBalance,
		Reference:        &ref,
		Reason:           &reason,
	})
	if err != nil {
		return fmt.Errorf("inserting refund ledger entry: %w", err)
	}

	return tx.Commit(ctx)
}

// ListWithdrawals returns a paginated list of the user's withdrawals.
func (s *Service) ListWithdrawals(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.WalletWithdrawal, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	return s.store.read.ListWithdrawalsByUser(ctx, gen.ListWithdrawalsByUserParams{

		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

// AllowedWithdrawalProviders is the canonical list of supported outbound
// payout providers. It is used by ListWithdrawalProviders and by method
// creation validation (Rule 16).
var AllowedWithdrawalProviders = []string{
	"nowpayments",
	"bank_transfer",
	"paypal",
	"paystack",
	"flutterwave",
	"stripe",
	"manual",
	"manual_crypto",
}

// ListWithdrawalProviders returns the supported payout provider keys.
func (s *Service) ListWithdrawalProviders(ctx context.Context) []string {
	return AllowedWithdrawalProviders
}

// CreateWithdrawalMethod stores a new user payout method.
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md.
func (s *Service) CreateWithdrawalMethod(ctx context.Context, userID pgtype.UUID, input CreateWithdrawalMethodInput) (gen.WalletWithdrawalMethod, error) {
	if input.Provider == "" {
		return gen.WalletWithdrawalMethod{}, newValidationError("provider", "provider is required")
	}

	if input.Account == "" {
		return gen.WalletWithdrawalMethod{}, newValidationError("account", "account is required")
	}

	if input.CurrencyCode == "" {
		return gen.WalletWithdrawalMethod{}, newValidationError("currency_code", "currency_code is required")
	}

	allowed := make(map[string]bool, len(AllowedWithdrawalProviders))
	for _, p := range AllowedWithdrawalProviders {
		allowed[p] = true
	}

	if !allowed[input.Provider] {
		return gen.WalletWithdrawalMethod{}, newValidationError("provider", "unsupported provider")
	}

	var metadata []byte

	if input.Metadata != nil {
		b, err := json.Marshal(input.Metadata)
		if err != nil {
			return gen.WalletWithdrawalMethod{}, newValidationError("metadata", "invalid metadata")
		}

		metadata = b
	}

	if len(metadata) == 0 {
		metadata = []byte("{}")
	}

	label := &input.Label
	if input.Label == "" {
		label = nil
	}

	currencyCode := &input.CurrencyCode

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return gen.WalletWithdrawalMethod{}, fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	// If this is the new default, clear any existing default.
	if input.IsDefault {
		if err := qtx.UpdateWithdrawalMethodDefault(ctx, userID); err != nil {
			return gen.WalletWithdrawalMethod{}, fmt.Errorf("clearing default method: %w", err)
		}
	}

	method, err := qtx.InsertWithdrawalMethod(ctx, gen.InsertWithdrawalMethodParams{

		UserID:       userID,
		Provider:     input.Provider,
		Label:        label,
		Account:      input.Account,
		CurrencyCode: currencyCode,
		IsVerified:   false,
		IsDefault:    input.IsDefault,
		IsActive:     true,
		Metadata:     metadata,
	})
	if err != nil {
		return gen.WalletWithdrawalMethod{}, fmt.Errorf("inserting payout method: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return gen.WalletWithdrawalMethod{}, fmt.Errorf("committing payout method: %w", err)
	}

	return method, nil
}

// ListWithdrawalMethods returns the user's saved payout methods.
func (s *Service) ListWithdrawalMethods(ctx context.Context, userID pgtype.UUID) ([]gen.WalletWithdrawalMethod, error) {
	return s.store.read.ListWithdrawalMethodsByUser(ctx,

		userID,
	)
}

// AdminListWithdrawals returns a paginated list of withdrawals for a platform.
func (s *Service) AdminListWithdrawals(ctx context.Context, status, userID string, limit, offset int32) ([]gen.WalletWithdrawal, int64, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	var statusArg *string
	if status != "" {
		statusArg = &status
	}

	var userIDArg pgtype.UUID
	if userID != "" {
		if err := userIDArg.Scan(userID); err != nil {
			return nil, 0, newValidationError("user_id", "invalid user id")
		}
	} else {
		// Ensure Valid is false so the query treats it as NULL.
		userIDArg = pgtype.UUID{}
	}

	withdrawals, err := s.store.read.ListWithdrawals(ctx, gen.ListWithdrawalsParams{

		Status:     statusArg,
		UserID:     userIDArg,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing withdrawals: %w", err)
	}

	count, err := s.store.read.CountWithdrawals(ctx, gen.CountWithdrawalsParams{

		Status: statusArg,
		UserID: userIDArg,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("counting withdrawals: %w", err)
	}

	return withdrawals, count, nil
}

// initiatePayout calls the provider's outbound payout API.
func (s *Service) initiatePayout(ctx context.Context, withdrawal gen.WalletWithdrawal, method gen.WalletWithdrawalMethod, calc CalculateWithdrawalResult, provider payout.Provider) (payout.PayoutResponse, error) {
	// Use the method currency as the payout coin/destination currency.
	payoutCurrency := derefString(method.CurrencyCode)
	if payoutCurrency == "" {
		payoutCurrency = withdrawal.CurrencyCode
	}

	// Convert net fiat amount to major units.
	amount := float64(calc.NetAmount) / 100.0

	return provider.InitiatePayout(ctx, payout.PayoutRequest{
		WithdrawalID: withdrawal.ID.String(),
		UserID:       withdrawal.UserID.String(),
		Amount:       amount,
		Currency:     payoutCurrency,
		Address:      method.Account,
		ExtraID:      extractMemo(method.Metadata),
	})
}

// manualPayoutProvider is a no-op provider for manual/admin payouts.
type manualPayoutProvider struct {
	typ string
}

func (p *manualPayoutProvider) Type() string         { return p.typ }
func (p *manualPayoutProvider) Supports(string) bool { return true }
func (p *manualPayoutProvider) InitiatePayout(context.Context, payout.PayoutRequest) (payout.PayoutResponse, error) {
	return payout.PayoutResponse{Status: WithdrawalStatusCompleted, ProviderRef: "manual"}, nil
}

// parseUUID converts a string UUID to pgtype.UUID.
func parseUUID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// extractMemo reads an optional memo/destination tag from method metadata.
func extractMemo(metadata []byte) string {
	if len(metadata) == 0 {
		return ""
	}
	// Fast path: look for "memo" or "extra_id" in the JSON without a full decode.
	for _, key := range []string{"memo", "extra_id", "tag"} {
		k := strconv.Quote(key) + ":"

		idx := strings.Index(string(metadata), k)
		if idx == -1 {
			continue
		}

		start := idx + len(k)
		// Skip whitespace
		for start < len(metadata) && (metadata[start] == ' ' || metadata[start] == '\t') {
			start++
		}

		if start >= len(metadata) {
			continue
		}

		if metadata[start] == '"' {
			end := start + 1
			for end < len(metadata) && metadata[end] != '"' {
				if metadata[end] == '\\' && end+1 < len(metadata) {
					end += 2
				} else {
					end++
				}
			}

			return string(metadata[start+1 : end])
		}
	}

	return ""
}

// float64ToNumeric converts a float64 to pgtype.Numeric.
func float64ToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(fmt.Sprintf("%f", f)); err != nil {
		return pgtype.Numeric{}
	}

	return n
}

// newValidationError returns an ErrInvalid sentinel error with a field hint.
func newValidationError(field, message string) error {
	return fmt.Errorf("%w: %s: %s", errpkg.ErrInvalid, field, message)
}

// newFeatureDisabledError returns an ErrFeatureDisabled sentinel error with a code hint.
func newFeatureDisabledError(code, message string) error {
	return fmt.Errorf("%w: %s: %s", errpkg.ErrFeatureDisabled, code, message)
}
