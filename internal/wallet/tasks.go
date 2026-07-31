package wallet

import (
	"context"
	"encoding/json"
	"fmt"

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
