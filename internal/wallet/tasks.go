package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// ProcessActivityEventPayload holds the payload for the process_activity_event task.
type ProcessActivityEventPayload struct {
	EventID  string `json:"event_id"`
	TenantID string `json:"tenant_id"`
}

// EnqueueProcessActivityEvent enqueues a task to process an activity event.
func (s *Service) EnqueueProcessActivityEvent(ctx context.Context, eventID, tenantID pgtype.UUID) error {
	payload, err := json.Marshal(ProcessActivityEventPayload{
		EventID:  eventID.String(),
		TenantID: tenantID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshaling activity event payload: %w", err)
	}

	if s.enqueuer != nil {
		return s.enqueuer.Enqueue(worker.TypeWalletProcessActivityEvent, payload)
	}

	// Fallback: process synchronously when Asynq is not wired.
	return s.ProcessActivityEvent(ctx, eventID, tenantID)
}

// ProcessActivityEventTask is the Asynq task handler for processing activity events.
// This would be registered with an Asynq worker in production.
func (s *Service) ProcessActivityEventTask(ctx context.Context, payload []byte) error {
	var p ProcessActivityEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("unmarshaling payload: %w", err)
	}

	var eventID, tenantID pgtype.UUID
	if err := eventID.Scan(p.EventID); err != nil {
		return fmt.Errorf("invalid event_id: %w", err)
	}

	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	return s.ProcessActivityEvent(ctx, eventID, tenantID)
}

// ProcessCreditPurchasePayload holds the payload for the
// wallet:process_credit_purchase task (Section 58). Enqueued by the payment
// webhook handler when a credit_purchase payment transitions to "succeeded".
type ProcessCreditPurchasePayload struct {
	PaymentID      string `json:"payment_id"`
	TenantID       string `json:"tenant_id"`
	UserID         string `json:"user_id"`
	CreditsToGrant int    `json:"credits_to_grant"`
	CurrencyCode   string `json:"currency_code"`
}

// ProcessCreditPurchaseTask is the Asynq task handler that credits the user's
// wallet after a credit_purchase payment succeeds (Section 58). Idempotent via
// the ledger's idempotency_key (credit_purchase:{payment_id}) — duplicate
// deliveries from Asynq retries are safe.
func (s *Service) ProcessCreditPurchaseTask(ctx context.Context, payload []byte) error {
	var p ProcessCreditPurchasePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("unmarshaling payload: %w", err)
	}

	var tenantID, userID pgtype.UUID
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}
	if err := userID.Scan(p.UserID); err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}
	if p.CreditsToGrant <= 0 {
		return fmt.Errorf("credits_to_grant must be positive, got %d", p.CreditsToGrant)
	}

	return s.grantCreditsFromPurchase(ctx, tenantID, userID, p)
}

// grantCreditsFromPurchase inserts a credits ledger entry for a successful
// credit purchase. Uses an advisory lock + idempotency key to prevent
// double-crediting on duplicate task deliveries (Section 58, Rule 9).
func (s *Service) grantCreditsFromPurchase(ctx context.Context, tenantID, userID pgtype.UUID, p ProcessCreditPurchasePayload) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	// Serialize concurrent balance mutations for this user.
	lockKey := "wallet:" + tenantID.String() + ":" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// Read current credits balance within the transaction.
	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		TenantID:   tenantID,
		UserID:     userID,
		LedgerType: LedgerTypeCredits,
	})
	if err != nil {
		currentBalance = 0
	}

	newBalance := currentBalance + int64(p.CreditsToGrant)

	// Idempotency key derived from the payment ID — duplicate task deliveries
	// (Asynq retries, webhook replays) are safe because the unique constraint
	// on idempotency_key rejects the second insert.
	idempotencyKey := "credit_purchase:" + p.PaymentID

	reference := fmt.Sprintf("Credit purchase — %d credits (%s)", p.CreditsToGrant, p.CurrencyCode)
	if p.PaymentID != "" {
		reference = fmt.Sprintf("Credit purchase %s — %d credits (%s)", p.PaymentID, p.CreditsToGrant, p.CurrencyCode)
	}

	if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TenantID:         tenantID,
		UserID:           userID,
		LedgerType:       LedgerTypeCredits,
		Operation:        OperationPurchase,
		Amount:           int64(p.CreditsToGrant),
		ResultingBalance: newBalance,
		Source:           strPtr("credit_purchase"),
		Reference:        &reference,
		IdempotencyKey:   &idempotencyKey,
		Reason:           nil,
	}); err != nil {
		return fmt.Errorf("inserting credit purchase ledger entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// ── wallet:award_points (Sections 28, 60, 70, 71, 73) ──────────────────────

// AwardPointsPayload holds the payload for the wallet:award_points task.
// Enqueued fire-and-forget by checklist completion, referral conversion,
// assessment scoring, and scholarship save. The value field is flexible:
// callers may use either "points" or "amount" — the handler checks both.
// ledger_type defaults to "points"; referral conversions set "credits".
type AwardPointsPayload struct {
	TenantID    string `json:"tenant_id"`
	UserID      string `json:"user_id"`
	Points      int64  `json:"points"`
	Amount      int64  `json:"amount"`
	LedgerType  string `json:"ledger_type,omitempty"`
	Reason      string `json:"reason"`
	ReferenceID string `json:"reference_id,omitempty"`
	GuideID     string `json:"guide_id,omitempty"`
}

// AwardPointsTask is the Asynq task handler for wallet:award_points. It
// inserts a ledger entry crediting the user's wallet. Idempotent via a
// deterministic idempotency key derived from user_id + reason + reference.
func (s *Service) AwardPointsTask(ctx context.Context, payload []byte) error {
	var p AwardPointsPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("wallet:award_points: unmarshaling payload: %w", err)
	}

	var tenantID, userID pgtype.UUID
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("wallet:award_points: invalid tenant_id: %w", err)
	}
	if err := userID.Scan(p.UserID); err != nil {
		return fmt.Errorf("wallet:award_points: invalid user_id: %w", err)
	}

	// Resolve the award amount — "points" takes precedence, then "amount".
	amount := p.Points
	if amount == 0 {
		amount = p.Amount
	}
	if amount <= 0 {
		return fmt.Errorf("wallet:award_points: amount must be positive, got %d", amount)
	}

	// Resolve ledger type — default to points.
	ledgerType := p.LedgerType
	if ledgerType == "" {
		ledgerType = LedgerTypePoints
	}
	if ledgerType != LedgerTypePoints && ledgerType != LedgerTypeCredits {
		return fmt.Errorf("wallet:award_points: invalid ledger_type %q", ledgerType)
	}

	// Build a human-readable reference.
	reference := p.Reason
	if p.ReferenceID != "" {
		reference = fmt.Sprintf("%s — %s", p.Reason, p.ReferenceID)
	} else if p.GuideID != "" {
		reference = fmt.Sprintf("%s — %s", p.Reason, p.GuideID)
	}

	// Deterministic idempotency key — duplicate deliveries are safe.
	idempotencyKey := fmt.Sprintf("award:%s:%s:%s", userID.String(), p.Reason, p.ReferenceID)
	if p.ReferenceID == "" {
		idempotencyKey = fmt.Sprintf("award:%s:%s:%s", userID.String(), p.Reason, p.GuideID)
	}
	if p.ReferenceID == "" && p.GuideID == "" {
		idempotencyKey = fmt.Sprintf("award:%s:%s:%d", userID.String(), p.Reason, amount)
	}

	source := p.Reason

	return s.creditWallet(ctx, tenantID, userID, ledgerType, OperationEarn, amount, source, reference, idempotencyKey)
}

// ── wallet:deduct_credits (Section 61 — Self-Serve Ads) ────────────────────

// DeductCreditsPayload holds the payload for the wallet:deduct_credits task.
// Enqueued by the self-serve ads service for campaign funding (deduct) and
// campaign refund (credit back). The organization_id is resolved to the
// org owner's user_id for the ledger entry.
type DeductCreditsPayload struct {
	TenantID       string `json:"tenant_id"`
	OrganizationID string `json:"organization_id"`
	UserID         string `json:"user_id,omitempty"`
	Amount         int64  `json:"amount"`
	CampaignID     string `json:"campaign_id,omitempty"`
	Reason         string `json:"reason"`
}

// DeductCreditsTask is the Asynq task handler for wallet:deduct_credits. It
// deducts credits for ad campaign funding or refunds credits on campaign
// rejection/early end. When organization_id is provided, the org owner is
// resolved as the wallet owner. When user_id is provided directly, it is
// used as-is. Idempotent via a deterministic idempotency key.
func (s *Service) DeductCreditsTask(ctx context.Context, payload []byte) error {
	var p DeductCreditsPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("wallet:deduct_credits: unmarshaling payload: %w", err)
	}

	var tenantID pgtype.UUID
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("wallet:deduct_credits: invalid tenant_id: %w", err)
	}

	if p.Amount <= 0 {
		return fmt.Errorf("wallet:deduct_credits: amount must be positive, got %d", p.Amount)
	}

	// Resolve the target user ID — either directly or via org owner.
	var userID pgtype.UUID
	if p.UserID != "" {
		if err := userID.Scan(p.UserID); err != nil {
			return fmt.Errorf("wallet:deduct_credits: invalid user_id: %w", err)
		}
	} else if p.OrganizationID != "" {
		var orgID pgtype.UUID
		if err := orgID.Scan(p.OrganizationID); err != nil {
			return fmt.Errorf("wallet:deduct_credits: invalid organization_id: %w", err)
		}
		org, err := s.store.write.GetOrganizationByIDOnly(ctx, orgID)
		if err != nil {
			return fmt.Errorf("wallet:deduct_credits: resolving org owner: %w", err)
		}
		if !org.OwnerUserID.Valid {
			return fmt.Errorf("wallet:deduct_credits: organization %s has no owner", p.OrganizationID)
		}
		userID = org.OwnerUserID
	} else {
		return fmt.Errorf("wallet:deduct_credits: either user_id or organization_id is required")
	}

	// Determine operation: refund adds credits back, everything else deducts.
	operation := OperationSpend
	ledgerAmount := -p.Amount // deduction is negative
	reference := p.Reason
	if p.CampaignID != "" {
		reference = fmt.Sprintf("%s — %s", p.Reason, p.CampaignID)
	}

	if p.Reason == "ad_campaign_refund" {
		operation = OperationRefund
		ledgerAmount = p.Amount // refund is positive
	}

	idempotencyKey := fmt.Sprintf("deduct:%s:%s:%s", userID.String(), p.Reason, p.CampaignID)
	if p.CampaignID == "" {
		idempotencyKey = fmt.Sprintf("deduct:%s:%s:%d", userID.String(), p.Reason, p.Amount)
	}

	return s.creditWallet(ctx, tenantID, userID, LedgerTypeCredits, operation, ledgerAmount, p.Reason, reference, idempotencyKey)
}

// creditWallet is the shared ledger insertion path for award_points and
// deduct_credits. It acquires a per-user advisory lock, reads the current
// balance, computes the new balance, and inserts the ledger entry — all
// within a single transaction (AUDIT-BE-005 pattern).
func (s *Service) creditWallet(ctx context.Context, tenantID, userID pgtype.UUID, ledgerType, operation string, amount int64, source, reference, idempotencyKey string) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("wallet: beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	// Serialize concurrent balance mutations for this user.
	lockKey := "wallet:" + tenantID.String() + ":" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("wallet: acquiring lock: %w", err)
	}

	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		TenantID:   tenantID,
		UserID:     userID,
		LedgerType: ledgerType,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("wallet: getting balance: %w", err)
		}
		currentBalance = 0
	}

	newBalance := currentBalance + amount

	// Non-negative balance guard (§1.3, Checklist B.10) — except clawback,
	// which per Section 28 may take a Credits balance negative rather than
	// being blocked outright (BUG-W-004).
	if newBalance < 0 && operation != OperationClawback {
		slog.Warn("wallet: deduct would result in negative balance, skipping",
			"user_id", userID.String(), "ledger_type", ledgerType,
			"current", currentBalance, "amount", amount)
		return fmt.Errorf("wallet: insufficient balance: current=%d, delta=%d", currentBalance, amount)
	}

	src := source
	if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TenantID:         tenantID,
		UserID:           userID,
		LedgerType:       ledgerType,
		Operation:        operation,
		Amount:           amount,
		ResultingBalance: newBalance,
		Source:           &src,
		Reference:        &reference,
		IdempotencyKey:   &idempotencyKey,
		Reason:           &source,
	}); err != nil {
		return fmt.Errorf("wallet: inserting ledger entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("wallet: committing transaction: %w", err)
	}

	slog.Info("wallet: ledger entry inserted",
		"user_id", userID.String(), "ledger_type", ledgerType,
		"operation", operation, "amount", amount, "new_balance", newBalance,
		"source", source)

	return nil
}
