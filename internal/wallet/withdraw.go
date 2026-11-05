package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/ledger"
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

// ErrInsufficientCredits marks a credit mutation that would push the wallet
// negative — e.g. a §22 appeal overturn re-reserving refunded funds the user
// already spent. Callers (the appeals resolver) map it to a retryable
// "resolution blocked" response.
var ErrInsufficientCredits = errors.New("wallet: insufficient credits")

// ErrInvalidPayoutSignature marks a payout status callback that failed
// provider signature verification — the HTTP layer maps it to 401.
var ErrInvalidPayoutSignature = errors.New("wallet: invalid payout webhook signature")

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
	// StepUpToken is a scoped "step_up" JWT proving a fresh TOTP or
	// password check (POST /auth/step-up) — required when
	// wallet.require_step_up is enabled (SEC-W-015).
	StepUpToken string
}

// CreateWithdrawalInput holds parameters for submitting a withdrawal.
type CreateWithdrawalInput struct {
	CreditAmount   int64
	CurrencyCode   string // optional
	PayoutMethodID string // optional; uses default if empty
	IdempotencyKey string
	StepUpToken    string // scoped step_up JWT (SEC-W-015)
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

	// SEC-W-015: a stolen session must not be able to drain a wallet.
	// Withdrawals require a fresh step-up token (TOTP/password) and, when
	// configured, a verified account.
	if err := s.enforceStepUp(ctx, userID, input.StepUpToken); err != nil {
		return WithdrawalResult{}, err
	}
	if err := s.enforceWithdrawalVerification(ctx, userID); err != nil {
		return WithdrawalResult{}, err
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

	// SEC-W-004: rolling-24h velocity cap before any reservation is made —
	// a stolen or forged balance drains fastest through this endpoint.
	if err := s.enforceWithdrawalVelocity(ctx, userID, input.CreditAmount); err != nil {
		return WithdrawalResult{}, err
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

	// Signals that can only escalate to review, never loosen it (SEC-W-003/
	// 005/006). Notes land in admin_note so the reviewer sees exactly why.
	var reviewNotes []string

	if reasons := s.verifyCreditProvenance(ctx, userID); len(reasons) > 0 {
		requiresApproval = true
		status = WithdrawalStatusPendingApproval
		reviewNotes = append(reviewNotes, "provenance: "+strings.Join(reasons, "; "))
		slog.Warn("withdrawal provenance check flagged user", "user_id", userID.String(), "reasons", reasons)
	}

	if hold := s.withdrawalMethodHold(ctx); hold > 0 && method.CreatedAt.Valid && time.Since(method.CreatedAt.Time) < hold {
		requiresApproval = true
		status = WithdrawalStatusPendingApproval
		reviewNotes = append(reviewNotes, fmt.Sprintf("payout method %.0fh old (hold %dh)", time.Since(method.CreatedAt.Time).Hours(), int(hold.Hours())))
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

		// SEC-W-007: never fire a payout for a coin the provider cannot
		// handle — an unsupported ticker burns money at the provider edge.
		if provider.Type() != "manual" && provider.Type() != "manual_crypto" {
			if !provider.Supports(derefString(method.CurrencyCode)) {
				return WithdrawalResult{}, newValidationError("payout_method_id",
					"payout provider does not support "+derefString(method.CurrencyCode))
			}
		}
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := ledger.New(s.store.write.WithTx(tx))

	// Serialize concurrent balance mutations for this user.
	lockKey := "wallet:" + "" + ":" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return WithdrawalResult{}, fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// SEC-W-004: re-check velocity inside the lock — two concurrent
	// withdrawals could both pass the pre-tx check and exceed the cap.
	if err := s.enforceWithdrawalVelocity(ctx, userID, input.CreditAmount); err != nil {
		return WithdrawalResult{}, err
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

	var adminNote *string
	if len(reviewNotes) > 0 {
		note := strings.Join(reviewNotes, " | ")
		adminNote = &note
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
		AdminNote:      adminNote,
		ReviewedBy:     pgtype.UUID{},
		ReviewedAt:     pgtype.Timestamptz{},
		IdempotencyKey: &input.IdempotencyKey,
	})
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("inserting withdrawal: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return WithdrawalResult{}, fmt.Errorf("committing withdrawal: %w", err)
	}

	providerRef := ""

	// Initiate the payout AFTER the debit + withdrawal row are committed
	// (ALG-004). A provider call inside the tx holds the per-user wallet
	// advisory lock for the whole network round-trip, and a post-payout
	// commit failure would send money with no record. Post-commit
	// initiation plus a compensating mark-failed + refund keeps the ledger
	// truthful either way.
	if !requiresApproval && provider != nil {
		// Manual providers are a no-op at creation time; they require an
		// admin to review and mark the withdrawal completed. Real providers
		// (NOWPayments, etc.) can be initiated immediately.
		if provider.Type() != "manual" && provider.Type() != "manual_crypto" {
			resp, perr := s.initiatePayout(ctx, withdrawal, method, calc, provider)
			if perr != nil {
				var rejected *payout.PayoutRejectedError
				if errors.As(perr, &rejected) {
					// Definitive rejection — the payout provably did not
					// run, so failing + refunding is safe (SEC-W-009).
					slog.Error("payout rejected by provider; marking withdrawal failed and refunding credits", "withdrawal_id", withdrawal.ID, "error", perr)

					if _, uerr := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
						ID:     withdrawal.ID,
						Status: WithdrawalStatusFailed,
					}); uerr != nil {
						slog.Error("failed to update withdrawal status to failed", "withdrawal_id", withdrawal.ID, "error", uerr)
					}

					status = WithdrawalStatusFailed

					if rerr := s.refundWithdrawalCredits(ctx, userID, input.CreditAmount, withdrawal.ID); rerr != nil {
						slog.Error("failed to refund rejected auto payout", "withdrawal_id", withdrawal.ID, "error", rerr)
					}
				} else {
					// Uncertain outcome — the request may have reached the
					// provider. Do NOT refund; escalate to the approval
					// queue so an admin verifies the provider side before
					// any reprocessing (SEC-W-009).
					slog.Error("payout initiation outcome uncertain; escalating to manual review", "withdrawal_id", withdrawal.ID, "error", perr)
					errpkg.CaptureException(fmt.Errorf("withdrawal %s payout outcome uncertain: %w", withdrawal.ID, perr))

					note := "payout initiation uncertain — verify provider shows no payout before reprocessing: " + perr.Error()
					if _, uerr := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
						ID:        withdrawal.ID,
						Status:    WithdrawalStatusPendingApproval,
						AdminNote: &note,
					}); uerr != nil {
						slog.Error("failed to escalate uncertain withdrawal", "withdrawal_id", withdrawal.ID, "error", uerr)
					}

					status = WithdrawalStatusPendingApproval
				}
			} else {
				providerRef = resp.ProviderRef

				if resp.Status != "" {
					status = resp.Status
				} else {
					status = WithdrawalStatusProcessing
				}

				// Persist the provider ref before returning — without it the
				// reconcile task cannot track this payout (ALG-005). Retry
				// once, then page ops via the error tracker: a payout is
				// in flight but untraceable.
				uerr := s.persistPayoutStatus(ctx, withdrawal.ID, status, providerRef)
				if uerr != nil {
					uerr = s.persistPayoutStatus(ctx, withdrawal.ID, status, providerRef)
				}

				if uerr != nil {
					slog.Error("withdrawal provider ref NOT persisted — payout in flight but untracked",
						"withdrawal_id", withdrawal.ID, "provider_ref", providerRef, "error", uerr)
					errpkg.CaptureException(fmt.Errorf("withdrawal %s provider_ref %q not persisted: %w",
						withdrawal.ID, providerRef, uerr))
				}
			}
		}
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

	// SEC-W-NGN: disabling a provider in payout_providers must stop NEW
	// withdrawals on existing methods, not just new method creation —
	// ops flips is_active to freeze a rail (e.g. Paystack outage) and
	// expects pending flows to fail fast, not drain mid-outage.
	if provRow, err := s.store.read.GetPayoutProviderByType(ctx, method.Provider); err == nil && !provRow.IsActive {
		return gen.WalletWithdrawalMethod{}, newValidationError("payout_method_id", "provider is disabled")
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
	// SEC-W-016: fold ALL group memberships into the most restrictive
	// policy — a LIMIT-1 group lookup let an arbitrary membership win.
	policy, err := s.store.read.GetUserWithdrawalPolicy(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WithdrawalStatusPendingApproval, true, nil
		}

		return "", false, fmt.Errorf("loading withdrawal policy: %w", err)
	}

	if policy.RequiresApproval {
		return WithdrawalStatusPendingApproval, true, nil
	}

	// A positive limit is required for auto-approval — a limit of 0 means
	// "never auto-approve", not "no limit" (ALG-015). Reading 0 as
	// unlimited would auto-approve arbitrary amounts.
	if policy.AutoLimit <= 0 {
		return WithdrawalStatusPendingApproval, true, nil
	}

	// SEC-W-004: the limit is a cumulative rolling-24h cap, not per-request —
	// without this, unlimited sub-limit withdrawals all auto-approved and
	// drained the float (amount-splitting bypass).
	if creditAmount <= policy.AutoLimit {
		totals, err := s.store.read.GetUserWithdrawalTotalsSince(ctx, gen.GetUserWithdrawalTotalsSinceParams{
			UserID: userID,
			Since:  pgtype.Timestamptz{Time: time.Now().Add(-24 * time.Hour), Valid: true},
		})
		if err != nil {
			return "", false, fmt.Errorf("reading withdrawal totals: %w", err)
		}

		if totals.CreditTotal+creditAmount <= policy.AutoLimit {
			return WithdrawalStatusPending, false, nil
		}
	}

	return WithdrawalStatusPendingApproval, true, nil
}

// ProcessWithdrawal approves a pending withdrawal and initiates the payout.
// It is called by admin action (PATCH /admin/wallet/withdrawals/{id} or
// POST /admin/wallet/withdrawals/{id}/process).
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md.
func (s *Service) ProcessWithdrawal(ctx context.Context, adminUserID, withdrawalID pgtype.UUID, stepUpToken string) (WithdrawalResult, error) {
	// SEC-W-015: the approval action is itself a money-moving decision —
	// the admin's own fresh step-up is required before real funds move.
	if err := s.enforceAdminStepUp(ctx, adminUserID, stepUpToken); err != nil {
		return WithdrawalResult{}, err
	}

	// SEC-W-002: claim the row atomically BEFORE calling the provider. Two
	// concurrent approvals used to both pass the status check and both fire
	// InitiatePayout — one withdrawal, two real payouts. The conditional
	// UPDATE makes the second claimant see no row.
	withdrawal, err := s.store.write.ClaimWithdrawalForProcessing(ctx, gen.ClaimWithdrawalForProcessingParams{
		ID:         withdrawalID,
		ReviewedBy: adminUserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if _, gerr := s.store.read.GetWithdrawalByID(ctx, withdrawalID); errors.Is(gerr, pgx.ErrNoRows) {
				return WithdrawalResult{}, errpkg.ErrNotFound
			}

			current, _ := s.store.read.GetWithdrawalByID(ctx, withdrawalID)
			return WithdrawalResult{}, fmt.Errorf("%w: cannot process withdrawal in status %s", errpkg.ErrInvalidStatusTransition, current.Status)
		}

		return WithdrawalResult{}, fmt.Errorf("claiming withdrawal: %w", err)
	}

	// unclaim returns the claimed withdrawal to the approval queue when a
	// pre-payout step fails — an 'approved' row with no payout in flight
	// would otherwise be unreachable (claim only allows pending states).
	unclaim := func(note string) {
		if _, uerr := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
			ID:        withdrawalID,
			Status:    WithdrawalStatusPendingApproval,
			AdminNote: &note,
		}); uerr != nil {
			slog.Error("failed to unclaim withdrawal", "withdrawal_id", withdrawalID, "error", uerr)
		}
	}

	method, err := s.store.read.GetWithdrawalMethodByID(ctx, gen.GetWithdrawalMethodByIDParams{
		ID: withdrawal.PayoutMethodID,

		UserID: withdrawal.UserID,
	})
	if err != nil {
		unclaim("payout method unavailable: " + err.Error())
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
		unclaim("payout provider unavailable: " + err.Error())
		return WithdrawalResult{}, err
	}

	status := WithdrawalStatusApproved
	providerRef := ""

	resp, perr := s.initiatePayout(ctx, withdrawal, method, calc, provider)
	if perr != nil {
		var rejected *payout.PayoutRejectedError
		if errors.As(perr, &rejected) {
			// Definitive rejection — safe to fail + refund (SEC-W-009).
			slog.Error("payout rejected by provider", "withdrawal_id", withdrawal.ID, "error", perr)
			status = WithdrawalStatusFailed
		} else {
			// Uncertain outcome — the provider may have received the
			// request. Bounce back to the approval queue with a note so a
			// human verifies before reprocessing; never refund
			// (SEC-W-009).
			slog.Error("payout initiation outcome uncertain; returning withdrawal to approval queue",
				"withdrawal_id", withdrawal.ID, "error", perr)
			errpkg.CaptureException(fmt.Errorf("withdrawal %s payout outcome uncertain: %w", withdrawal.ID, perr))
			status = WithdrawalStatusPendingApproval
		}
	} else {
		providerRef = resp.ProviderRef

		if resp.Status != "" {
			status = resp.Status
		} else {
			status = WithdrawalStatusProcessing
		}
	}

	var adminNote *string
	if status == WithdrawalStatusPendingApproval && perr != nil {
		note := "payout initiation uncertain — verify provider shows no payout before reprocessing: " + perr.Error()
		adminNote = &note
	}

	updated, err := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
		ID: withdrawalID,

		Status:      status,
		ProviderRef: &providerRef,
		AdminNote:   adminNote,
		ReviewedBy:  adminUserID,
		ReviewedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("updating withdrawal status: %w", err)
	}

	// Refund credits only on definitive rejection.
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
func (s *Service) RejectWithdrawal(ctx context.Context, adminUserID, withdrawalID pgtype.UUID, note, stepUpToken string) (WithdrawalResult, error) {
	if err := s.enforceAdminStepUp(ctx, adminUserID, stepUpToken); err != nil {
		return WithdrawalResult{}, err
	}

	var noteRef *string
	if note != "" {
		noteRef = &note
	}

	// SEC-W-013: atomic reject — the conditional UPDATE means a second
	// concurrent reject sees no row and cannot double-refund the credits.
	updated, err := s.store.write.RejectWithdrawalAtomic(ctx, gen.RejectWithdrawalAtomicParams{
		ID:         withdrawalID,
		AdminNote:  noteRef,
		ReviewedBy: adminUserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current, gerr := s.store.read.GetWithdrawalByID(ctx, withdrawalID)
			if errors.Is(gerr, pgx.ErrNoRows) {
				return WithdrawalResult{}, errpkg.ErrNotFound
			}
			if gerr != nil {
				return WithdrawalResult{}, fmt.Errorf("loading withdrawal: %w", gerr)
			}
			return WithdrawalResult{}, fmt.Errorf("%w: cannot reject withdrawal in status %s", errpkg.ErrInvalidStatusTransition, current.Status)
		}
		return WithdrawalResult{}, fmt.Errorf("rejecting withdrawal: %w", err)
	}

	if err := s.refundWithdrawalCredits(ctx, updated.UserID, updated.CreditAmount, withdrawalID); err != nil {
		slog.Error("failed to refund rejected withdrawal", "withdrawal_id", updated.ID, "error", err)
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

// persistPayoutStatus records the post-commit payout status + provider ref on
// a withdrawal row. Kept as a small standalone helper so the call site can
// retry it once (ALG-005) — a lost provider_ref orphans an in-flight payout.
func (s *Service) persistPayoutStatus(ctx context.Context, withdrawalID pgtype.UUID, status, providerRef string) error {
	_, err := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
		ID:          withdrawalID,
		Status:      status,
		ProviderRef: &providerRef,
	})
	return err
}

// refundWithdrawalCredits re-credits the user's wallet when a payout fails.
// Takes the same per-user advisory lock as every other balance mutation
// (ALG-007) — without it a concurrent spend could interleave the
// GetLatestBalance read and write a wrong resulting_balance snapshot.
func (s *Service) refundWithdrawalCredits(ctx context.Context, userID pgtype.UUID, creditAmount int64, withdrawalID pgtype.UUID) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning refund tx: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := ledger.New(s.store.write.WithTx(tx))

	lockKey := "wallet::" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("acquiring wallet lock: %w", err)
	}

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
	source := "withdrawal_refund"
	// SEC-W-013: deterministic key — a refund retried by the reconcile task,
	// the IPN webhook, or a repeated admin action is a no-op, not a second
	// credit.
	refundKey := "withdrawal_refund:" + withdrawalID.String()

	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		UserID:           userID,
		LedgerType:       LedgerTypeCredits,
		Operation:        OperationRefund,
		Amount:           creditAmount,
		ResultingBalance: newBalance,
		Source:           &source,
		Reference:        &ref,
		IdempotencyKey:   &refundKey,
		Reason:           &reason,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Refund already applied by a prior delivery — idempotent no-op.
			return nil
		}
		return fmt.Errorf("inserting refund ledger entry: %w", err)
	}

	return tx.Commit(ctx)
}

// ReopenWithdrawalOnAppeal reverses a rejected withdrawal when its appeal
// is granted (§22 'withdrawal' subject). Rejection refunded the reserved
// credits, so reopening must re-deduct them inside the per-user wallet
// advisory lock — otherwise the user could double-spend the refunded
// balance while the withdrawal re-enters the approval queue. Registered
// with the appeals service via RegisterOverturn from server.go.
func (s *Service) ReopenWithdrawalOnAppeal(ctx context.Context, withdrawalID pgtype.UUID) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning reopen tx: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := ledger.New(s.store.write.WithTx(tx))

	// SEC-W-013: claim the rejected → pending_approval transition FIRST and
	// atomically. Two concurrent reopens used to both read 'rejected' and
	// both re-deduct the credits; now the loser sees no row. A rollback on
	// the insufficient-balance path below cleanly undoes the claim.
	withdrawal, err := qtx.ReopenWithdrawalForAppeal(ctx, withdrawalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Not rejected (or doesn't exist) — already re-reviewed or
			// reopened; treat as a no-op so appeal resolution completes.
			return nil
		}

		return fmt.Errorf("reopening withdrawal: %w", err)
	}

	// Serialize concurrent balance mutations for this user (same lock key
	// scheme as CreateWithdrawal).
	lockKey := "wallet:" + "" + ":" + withdrawal.UserID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("acquiring wallet lock: %w", err)
	}

	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		UserID:     withdrawal.UserID,
		LedgerType: LedgerTypeCredits,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("reading credits balance: %w", err)
		}

		currentBalance = 0
	}

	if currentBalance < withdrawal.CreditAmount {
		// The user spent the refunded credits; the appeal resolution is
		// aborted (appeal stays pending) rather than pushing the balance
		// negative.
		return fmt.Errorf("%w: refunded credits were spent", ErrInsufficientCredits)
	}

	newBalance := currentBalance - withdrawal.CreditAmount
	ref := withdrawal.ID.String()
	reason := "withdrawal rejection overturned on appeal - credits re-reserved"
	reopenKey := "withdrawal_reopen:" + withdrawalID.String()
	source := "withdrawal_reopen"

	if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		UserID:           withdrawal.UserID,
		LedgerType:       LedgerTypeCredits,
		Operation:        OperationWithdraw,
		Amount:           -withdrawal.CreditAmount,
		ResultingBalance: newBalance,
		Source:           &source,
		Reference:        &ref,
		IdempotencyKey:   &reopenKey,
		Reason:           &reason,
	}); err != nil {
		return fmt.Errorf("inserting re-reserve ledger entry: %w", err)
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
	"circle",
	"bank_transfer",
	"paypal",
	"paystack",
	"flutterwave",
	"stripe",
	"manual",
	"manual_crypto",
}

// ListWithdrawalProviders returns the ACTIVE payout provider keys from
// the payout_providers registry — falls back to the static list when the
// table is unseeded so older deployments keep working (SEC-W-NGN).
func (s *Service) ListWithdrawalProviders(ctx context.Context) []string {
	rows, err := s.store.read.ListPayoutProviders(ctx)
	if err != nil || len(rows) == 0 {
		return AllowedWithdrawalProviders
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.IsActive {
			out = append(out, r.Type)
		}
	}
	if len(out) == 0 {
		return AllowedWithdrawalProviders
	}
	return out
}

// CreateWithdrawalMethod stores a new user payout method.
//
// Ref: japaconnect-engineering/sections/58-credit-currency-system.md.
func (s *Service) CreateWithdrawalMethod(ctx context.Context, userID pgtype.UUID, input CreateWithdrawalMethodInput) (gen.WalletWithdrawalMethod, error) {
	// SEC-W-015: a fresh payout address is the classic account-takeover
	// exfil vector — require step-up on top of the configured hold.
	if err := s.enforceStepUp(ctx, userID, input.StepUpToken); err != nil {
		return gen.WalletWithdrawalMethod{}, err
	}

	if input.Provider == "" {
		return gen.WalletWithdrawalMethod{}, newValidationError("provider", "provider is required")
	}

	if input.Account == "" {
		return gen.WalletWithdrawalMethod{}, newValidationError("account", "account is required")
	}

	if input.CurrencyCode == "" {
		return gen.WalletWithdrawalMethod{}, newValidationError("currency_code", "currency_code is required")
	}

	// SEC-W-NGN: the payout_providers registry gates which providers and
	// currencies are offered — is_active flips rails on/off without
	// deploys, supported_currencies constrains the method's currency_code.
	// Rows absent for a provider fall back to the static list so unseeded
	// deployments keep working.
	provRow, provErr := s.store.read.GetPayoutProviderByType(ctx, input.Provider)
	if provErr == nil {
		if !provRow.IsActive {
			return gen.WalletWithdrawalMethod{}, newValidationError("provider", "provider is disabled")
		}
		if len(provRow.SupportedCurrencies) > 0 {
			ok := false
			for _, c := range provRow.SupportedCurrencies {
				if strings.EqualFold(c, input.CurrencyCode) {
					ok = true
					break
				}
			}
			if !ok {
				return gen.WalletWithdrawalMethod{}, newValidationError("currency_code", "currency not supported by provider")
			}
		}
	} else if errors.Is(provErr, pgx.ErrNoRows) {
		allowed := make(map[string]bool, len(AllowedWithdrawalProviders))
		for _, p := range AllowedWithdrawalProviders {
			allowed[p] = true
		}
		if !allowed[input.Provider] {
			return gen.WalletWithdrawalMethod{}, newValidationError("provider", "unsupported provider")
		}
	} else {
		return gen.WalletWithdrawalMethod{}, fmt.Errorf("checking payout provider: %w", provErr)
	}

	// SEC-W-006: crypto destinations are validated at save time — a typo'd
	// address burns real money irreversibly once a payout fires.
	if isCryptoProvider(input.Provider) {
		if err := validateCryptoAddress(input.CurrencyCode, input.Account); err != nil {
			return gen.WalletWithdrawalMethod{}, err
		}
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

	// SEC-W-NGN: bank payout methods verify the account against the
	// provider BEFORE save — a typo'd account number can never become a
	// destination, and the resolved name + recipient_code are cached so
	// InitiatePayout doesn't re-create them per withdrawal.
	if input.Provider == "paystack" {
		provider, ok := s.payoutProviders["paystack"]
		if !ok {
			return gen.WalletWithdrawalMethod{}, newValidationError("provider", "paystack payouts are not configured")
		}
		resolver, ok := provider.(interface {
			ResolveAccount(ctx context.Context, bankCode, accountNumber string) (string, error)
		})
		if !ok {
			return gen.WalletWithdrawalMethod{}, newValidationError("provider", "provider cannot verify bank accounts")
		}

		var meta map[string]interface{}
		_ = json.Unmarshal(metadata, &meta)
		bankCode, _ := meta["bank_code"].(string)
		if bankCode == "" {
			return gen.WalletWithdrawalMethod{}, newValidationError("metadata.bank_code", "bank_code is required — choose from GET /me/wallet/banks")
		}
		if !regexp.MustCompile(`^\d{10}$`).MatchString(input.Account) {
			return gen.WalletWithdrawalMethod{}, newValidationError("account", "account must be a 10-digit NUBAN number")
		}

		accountName, err := resolver.ResolveAccount(ctx, bankCode, input.Account)
		if err != nil {
			return gen.WalletWithdrawalMethod{}, newValidationError("account", "bank account could not be verified: "+err.Error())
		}

		// Create the transfer recipient now so payouts later are a single
		// API call and the method carries a usable recipient_code.
		creator, ok := provider.(interface {
			CreateRecipient(ctx context.Context, accountName, accountNumber, bankCode string) (string, error)
		})
		if !ok {
			return gen.WalletWithdrawalMethod{}, newValidationError("provider", "provider cannot create recipients")
		}
		recipientCode, err := creator.CreateRecipient(ctx, accountName, input.Account, bankCode)
		if err != nil {
			return gen.WalletWithdrawalMethod{}, fmt.Errorf("creating transfer recipient: %w", err)
		}

		meta["account_name"] = accountName
		meta["recipient_code"] = recipientCode
		if metadata, err = json.Marshal(meta); err != nil {
			return gen.WalletWithdrawalMethod{}, fmt.Errorf("storing method metadata: %w", err)
		}
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

	qtx := ledger.New(s.store.write.WithTx(tx))

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

	// Pass the net fiat amount in minor units (kobo/cents). Integer
	// math end-to-end — the provider converts to its wire format at the
	// edge with explicit rounding, so crypto dust cannot drift.
	// FiatAmountMinor/FiatCurrency always carry the fiat value so a crypto
	// destination currency is converted provider-side, never read as a
	// coin quantity (SEC-W-001).
	// SEC-W-009 traceability: when no chain memo is needed, extra_id carries
	// the withdrawal ID so ops can verify a payout in the provider dashboard
	// even if our provider_ref was never persisted.
	extraID := extractMemo(method.Metadata)
	if extraID == "" {
		extraID = withdrawal.ID.String()
	}

	resp, err := provider.InitiatePayout(ctx, payout.PayoutRequest{
		WithdrawalID:    withdrawal.ID.String(),
		UserID:          withdrawal.UserID.String(),
		AmountMinor:     calc.NetAmount,
		Currency:        payoutCurrency,
		FiatAmountMinor: calc.NetAmount,
		FiatCurrency:    withdrawal.CurrencyCode,
		Address:         method.Account,
		Metadata:        method.Metadata,
		ExtraID:         extraID,
		CallbackURL:     s.payoutCallbackURL,
	})
	if err != nil {
		return resp, err
	}

	// SEC-W-NGN: providers may hand back lazily-created artifacts (e.g. a
	// Paystack recipient_code for a method saved before caching) — merge
	// them into the method's metadata so subsequent payouts reuse it.
	if len(resp.MethodMetadataUpdates) > 0 {
		s.persistMethodMetadataUpdates(ctx, method.ID, method.UserID, resp.MethodMetadataUpdates)
	}

	return resp, nil
}

// persistMethodMetadataUpdates merges provider-returned artifacts into a
// payout method's metadata blob. Best-effort — payout itself already
// succeeded; a persistence failure just re-triggers lazy creation next time.
func (s *Service) persistMethodMetadataUpdates(ctx context.Context, methodID, ownerID pgtype.UUID, updates map[string]interface{}) {
	method, err := s.store.read.GetWithdrawalMethodByID(ctx, gen.GetWithdrawalMethodByIDParams{
		ID:     methodID,
		UserID: ownerID,
	})
	if err != nil {
		slog.Warn("wallet: method metadata update lookup failed", "method_id", methodID.String(), "error", err)
		return
	}
	var meta map[string]interface{}
	_ = json.Unmarshal(method.Metadata, &meta)
	if meta == nil {
		meta = make(map[string]interface{})
	}
	for k, v := range updates {
		meta[k] = v
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return
	}
	if err := s.store.write.UpdateWithdrawalMethodMetadata(ctx, gen.UpdateWithdrawalMethodMetadataParams{
		ID:       methodID,
		Metadata: b,
	}); err != nil {
		slog.Warn("wallet: method metadata update failed", "method_id", methodID.String(), "error", err)
	}
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

// ── SEC-W: withdrawal security controls ─────────────────────────────────

// withdrawalMethodHold returns the configured hold period for newly added
// payout methods (wallet.withdrawal_method_hold_hours, default 24h).
// Methods younger than the hold force manual approval — an account
// takeover that adds a fresh USDT address cannot drain instantly.
func (s *Service) withdrawalMethodHold(ctx context.Context) time.Duration {
	hours := 24
	if s.settings != nil {
		hours = s.settings.GetIntOr(ctx, "wallet.withdrawal_method_hold_hours", 24)
	}
	if hours <= 0 {
		return 0
	}
	return time.Duration(hours) * time.Hour
}

// enforceWithdrawalVelocity rejects a withdrawal that would push the user's
// rolling-24h credit outflow past wallet.daily_withdrawal_cap_credits
// (0 = platform cap disabled; the per-group auto-approve cap still applies
// separately in resolveWithdrawalStatus).
func (s *Service) enforceWithdrawalVelocity(ctx context.Context, userID pgtype.UUID, creditAmount int64) error {
	dailyCap := 0
	if s.settings != nil {
		dailyCap = s.settings.GetIntOr(ctx, "wallet.daily_withdrawal_cap_credits", 0)
	}
	if dailyCap <= 0 {
		return nil
	}

	totals, err := s.store.read.GetUserWithdrawalTotalsSince(ctx, gen.GetUserWithdrawalTotalsSinceParams{
		UserID: userID,
		Since:  pgtype.Timestamptz{Time: time.Now().Add(-24 * time.Hour), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("reading withdrawal velocity: %w", err)
	}

	if totals.CreditTotal+creditAmount > int64(dailyCap) {
		return newValidationError("credit_amount", "daily withdrawal limit reached")
	}

	return nil
}

// verifyCreditProvenance cross-checks the credits ledger against evidence an
// attacker cannot forge in a single table (SEC-W-003/005). Every check that
// fails becomes a reason string; callers escalate to manual approval rather
// than rejecting outright — a flagged user is reviewed, not stranded.
//
//   - Chain integrity: SUM(amount) must equal the latest resulting_balance.
//     A forged snapshot diverges.
//   - Purchase backing: purchase entries must link to a succeeded §17
//     payment — forged "purchase" rows have none.
//   - Tip backing: tip-sourced earns must reference a completed tips row
//     with matching net — blocks forge→tip→sockpuppet laundering.
//   - Source presence: legitimate positive entries always carry a source.
func (s *Service) verifyCreditProvenance(ctx context.Context, userID pgtype.UUID) []string {
	var reasons []string

	latest, err := s.store.read.GetLatestCreditBalance(ctx, userID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return []string{"provenance check failed: " + err.Error()}
		}
		latest = 0
	}

	sum, err := s.store.read.GetCreditsLedgerSum(ctx, userID)
	if err != nil {
		return []string{"provenance check failed: " + err.Error()}
	}
	if sum != latest {
		reasons = append(reasons, fmt.Sprintf("ledger chain diverged (entries sum to %d, reported balance %d)", sum, latest))
	}

	if r, err := s.store.read.CountUnbackedPurchaseCredits(ctx, userID); err != nil {
		reasons = append(reasons, "purchase provenance check failed")
	} else if r.RowCount > 0 {
		reasons = append(reasons, fmt.Sprintf("%d purchase entries (%d credits) lack a succeeded payment", r.RowCount, r.CreditTotal))
	}

	if r, err := s.store.read.CountUnbackedTipEarns(ctx, userID); err != nil {
		reasons = append(reasons, "tip provenance check failed")
	} else if r.RowCount > 0 {
		reasons = append(reasons, fmt.Sprintf("%d tip earns (%d credits) lack a matching completed tip", r.RowCount, r.CreditTotal))
	}

	if r, err := s.store.read.CountSourcelessCreditGrants(ctx, userID); err != nil {
		reasons = append(reasons, "grant provenance check failed")
	} else if r.RowCount > 0 {
		reasons = append(reasons, fmt.Sprintf("%d credit grants (%d credits) have no source", r.RowCount, r.CreditTotal))
	}

	if r, err := s.store.read.CountAnonymousAdminAdjusts(ctx, userID); err != nil {
		reasons = append(reasons, "admin-adjust provenance check failed")
	} else if r.RowCount > 0 {
		reasons = append(reasons, fmt.Sprintf("%d admin adjustments (%d credits) have no actor", r.RowCount, r.CreditTotal))
	}

	return reasons
}

// ProcessPayoutWebhook validates a signed provider payout callback and
// applies the status transition (SEC-W-008). Unknown payout refs are acked
// (returns nil) so the provider stops retrying; only in-flight withdrawals
// transition — terminal states are never resurrected.
func (s *Service) ProcessPayoutWebhook(ctx context.Context, providerType string, payload []byte, signature string) error {
	p, ok := s.payoutProviders[providerType]
	if !ok {
		return fmt.Errorf("payout provider %q not configured", providerType)
	}

	v, ok := p.(payout.WebhookVerifier)
	if !ok {
		return fmt.Errorf("payout provider %q cannot verify webhooks", providerType)
	}

	if err := v.VerifyWebhookSignature(payload, signature); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPayoutSignature, err)
	}

	var providerRef, rawStatus string
	if ep, ok := p.(payout.WebhookEventParser); ok {
		// Custom envelope (e.g. Paystack {event:"transfer.success",data}).
		ref, st, isPayout := ep.ParsePayoutEvent(payload)
		if !isPayout {
			return nil // not a payout event — ack and ignore
		}
		providerRef, rawStatus = ref, st
	} else {
		var evt struct {
			PayoutID string `json:"payout_id"`
			ID       string `json:"id"`
			Status   string `json:"status"`
		}
		if err := json.Unmarshal(payload, &evt); err != nil {
			return fmt.Errorf("parsing payout ipn: %w", err)
		}
		if evt.PayoutID == "" {
			evt.PayoutID = evt.ID
		}
		providerRef, rawStatus = evt.PayoutID, evt.Status
	}
	if providerRef == "" {
		return errors.New("payout ipn missing provider reference")
	}

	withdrawal, err := s.store.read.GetWithdrawalByProviderRef(ctx, &providerRef)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("payout ipn for unknown ref — acking", "provider_ref", providerRef)
			return nil
		}
		return fmt.Errorf("loading withdrawal for payout ipn: %w", err)
	}

	// Only in-flight states transition; terminal rows are immutable here.
	switch withdrawal.Status {
	case WithdrawalStatusApproved, WithdrawalStatusProcessing, WithdrawalStatusPending:
	default:
		return nil
	}

	status := WithdrawalStatusProcessing
	if n, ok := p.(interface{ NormalizePayoutStatus(string) string }); ok {
		status = n.NormalizePayoutStatus(rawStatus)
	}

	if _, err := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
		ID:     withdrawal.ID,
		Status: status,
	}); err != nil {
		return fmt.Errorf("updating withdrawal from payout ipn: %w", err)
	}

	if status == WithdrawalStatusFailed {
		if err := s.refundWithdrawalCredits(ctx, withdrawal.UserID, withdrawal.CreditAmount, withdrawal.ID); err != nil {
			slog.Error("payout ipn refund failed", "withdrawal_id", withdrawal.ID, "error", err)
		}
	}

	return nil
}

// isCryptoProvider reports whether the provider key denotes a crypto payout
// rail whose account field is an on-chain address.
func isCryptoProvider(provider string) bool {
	return provider == "nowpayments" || provider == "manual_crypto"
}

// evmAddr matches Ethereum-derived addresses (ETH, ERC-20/BEP-20 tokens).
var evmAddr = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

// cryptoAddressPatterns validates destination formats per payout coin.
// Unknown coins skip validation — format lists drift faster than code.
var cryptoAddressPatterns = map[string]*regexp.Regexp{
	"usdttrc20":  regexp.MustCompile(`^T[1-9A-HJ-NP-Za-km-z]{33}$`),
	"usdterc20":  evmAddr,
	"usdtbsc":    evmAddr,
	"eth":        evmAddr,
	"bnbmainnet": evmAddr,
	"btc":        regexp.MustCompile(`^(bc1[a-z0-9]{25,59}|[13][a-km-zA-HJ-NP-Z1-9]{25,34})$`),
	"ltc":        regexp.MustCompile(`^(ltc1[a-z0-9]{25,59}|[LM3][a-km-zA-HJ-NP-Z1-9]{26,33})$`),
	"sol":        regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`),
	"ada":        regexp.MustCompile(`^addr1[a-z0-9]{50,120}$`),
	"xrp":        regexp.MustCompile(`^r[1-9A-HJ-NP-Za-km-z]{24,34}$`),
	"doge":       regexp.MustCompile(`^D[5-9A-HJ-NP-U][1-9A-HJ-NP-Za-km-z]{32}$`),
	"bch":        regexp.MustCompile(`^(bitcoincash:)?[qp][a-z0-9]{41}$|^[13][a-km-zA-HJ-NP-Z1-9]{25,34}$`),
}

// validateCryptoAddress checks the destination format for crypto payout
// methods. A malformed address is rejected at save time — irreversible
// on-chain sends must never rely on downstream typo handling.
func validateCryptoAddress(currencyCode, account string) error {
	coin := strings.ToLower(strings.TrimSpace(currencyCode))
	pattern, ok := cryptoAddressPatterns[coin]
	if !ok {
		return nil
	}

	if !pattern.MatchString(strings.TrimSpace(account)) {
		return newValidationError("account", "invalid "+coin+" address format")
	}

	return nil
}

// ErrStepUpRequired is returned when a sensitive wallet mutation lacks a
// valid scoped step-up token (SEC-W-015).
var ErrStepUpRequired = errors.New("step-up authentication required")

// ErrVerificationRequired is returned when the account does not meet the
// withdrawal verification bar (email verified — the platform-wide signal;
// there is no user-level KYC flag outside instructor_profiles).
var ErrVerificationRequired = errors.New("account verification required before withdrawal")

// enforceStepUp requires a valid scoped "step_up" token when
// wallet.require_step_up is enabled (default true — a stolen session
// must not be able to drain a wallet or register a fresh payout
// destination). The token is minted by POST /auth/step-up after a fresh
// TOTP or password verification and lives only 2 minutes.
func (s *Service) enforceStepUp(ctx context.Context, userID pgtype.UUID, token string) error {
	required := true
	if s.settings != nil {
		required = s.settings.GetBoolOr(ctx, "wallet.require_step_up", true)
	}
	if !required {
		return nil
	}
	if token == "" {
		return ErrStepUpRequired
	}
	if s.stepUpVerifier == nil {
		// Fail closed: configured requirement + no verifier = deny.
		slog.Error("wallet: step-up required but no verifier wired")
		return ErrStepUpRequired
	}
	if err := s.stepUpVerifier(ctx, userID, token); err != nil {
		slog.Warn("wallet: step-up verification failed", "user_id", userID.String(), "error", err)
		return ErrStepUpRequired
	}
	return nil
}

// enforceWithdrawalVerification applies the configured verification bar
// for withdrawals. wallet.withdrawal_verification_level: "none" (gate
// off — legacy), "email" (default — email_verified_at required), "kyc"
// (users.kyc_status='verified' required — SEC-W-015b). The legacy bool
// wallet.withdrawal_requires_verification=false also disables the gate.
func (s *Service) enforceWithdrawalVerification(ctx context.Context, userID pgtype.UUID) error {
	level := "email"
	if s.settings != nil {
		level = s.settings.GetStringOr(ctx, "wallet.withdrawal_verification_level", "email")
		if !s.settings.GetBoolOr(ctx, "wallet.withdrawal_requires_verification", true) {
			level = "none"
		}
	}
	switch level {
	case "none", "":
		return nil
	case "kyc":
		status, err := s.store.read.GetUserKYCStatus(ctx, userID)
		if err != nil {
			return fmt.Errorf("checking kyc status: %w", err)
		}
		if status != "verified" {
			return ErrVerificationRequired
		}
		return nil
	default: // "email"
		verified, err := s.store.read.GetUserEmailVerifiedStatus(ctx, userID)
		if err != nil {
			return fmt.Errorf("checking verification status: %w", err)
		}
		if !verified.Valid {
			return ErrVerificationRequired
		}
		return nil
	}
}

// enforceAdminStepUp gates the admin approve/reject actions on the
// admin's own fresh step-up token when
// wallet.admin_approval_requires_step_up is enabled (default true) —
// an admin session hijack alone shouldn't move real money.
func (s *Service) enforceAdminStepUp(ctx context.Context, adminUserID pgtype.UUID, token string) error {
	required := true
	if s.settings != nil {
		required = s.settings.GetBoolOr(ctx, "wallet.admin_approval_requires_step_up", true)
	}
	if !required {
		return nil
	}
	if token == "" || s.stepUpVerifier == nil {
		if s.stepUpVerifier == nil {
			slog.Error("wallet: admin step-up required but no verifier wired")
		}
		return ErrStepUpRequired
	}
	if err := s.stepUpVerifier(ctx, adminUserID, token); err != nil {
		slog.Warn("wallet: admin step-up verification failed", "admin_id", adminUserID.String(), "error", err)
		return ErrStepUpRequired
	}
	return nil
}

// ListNGNBanks returns the Paystack bank list for the payout-method
// picker (SEC-W-NGN) — the bank_code the user supplies on
// POST /me/wallet/withdrawal-methods. country is a Paystack country
// slug ("nigeria", "ghana") passed through from the request; empty
// defaults to "nigeria".
func (s *Service) ListNGNBanks(ctx context.Context, country string) (interface{}, error) {
	provider, ok := s.payoutProviders["paystack"]
	if !ok {
		return nil, fmt.Errorf("paystack payouts are not configured")
	}
	lister, ok := provider.(interface {
		ListBanks(ctx context.Context, country string) ([]payout.PaystackBank, error)
	})
	if !ok {
		return nil, fmt.Errorf("provider does not support bank listing")
	}
	banks, err := lister.ListBanks(ctx, country)
	if err != nil {
		return nil, fmt.Errorf("listing banks: %w", err)
	}
	return banks, nil
}

// PayoutFloat returns each configured provider's spendable balance
// (SEC-W-NGN ops visibility) — ops sees unfunded rails before payouts
// stall, and the solvency task can correlate float vs. liabilities.
func (s *Service) PayoutFloat(ctx context.Context) map[string]interface{} {
	out := make(map[string]interface{}, len(s.payoutProviders))
	for name, p := range s.payoutProviders {
		bp, ok := p.(payout.BalanceProvider)
		if !ok {
			continue
		}
		bal, err := bp.PayoutBalance(ctx)
		if err != nil {
			out[name] = map[string]interface{}{"error": err.Error()}
			continue
		}
		out[name] = bal
	}
	return out
}

// paystackProvider returns the registered Paystack payout provider with
// the OTP capability surface, or nil.
func (s *Service) paystackOTPCapable() (interface {
	DisableOTP(ctx context.Context) (string, error)
	FinalizeDisableOTP(ctx context.Context, otp string) error
}, bool) {
	p, ok := s.payoutProviders["paystack"]
	if !ok {
		return nil, false
	}
	c, ok := p.(interface {
		DisableOTP(ctx context.Context) (string, error)
		FinalizeDisableOTP(ctx context.Context, otp string) error
	})
	return c, ok
}

// DisableTransferOTP initiates disabling Paystack transfer OTP —
// Paystack sends a code to the business contact; finalize it with
// FinalizeTransferOTP (SEC-W-NGN ops flow).
func (s *Service) DisableTransferOTP(ctx context.Context) (string, error) {
	c, ok := s.paystackOTPCapable()
	if !ok {
		return "", fmt.Errorf("paystack payouts are not configured")
	}
	return c.DisableOTP(ctx)
}

// FinalizeTransferOTP submits the OTP code to complete disabling.
func (s *Service) FinalizeTransferOTP(ctx context.Context, otp string) error {
	c, ok := s.paystackOTPCapable()
	if !ok {
		return fmt.Errorf("paystack payouts are not configured")
	}
	return c.FinalizeDisableOTP(ctx, otp)
}
