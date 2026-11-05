package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/ledger"
	"github.com/jedafe/japa/internal/wallet/payout"
	"github.com/jedafe/japa/internal/worker"
)

// ProcessActivityEventPayload holds the payload for the process_activity_event task.
type ProcessActivityEventPayload struct {
	EventID string `json:"event_id"`
}

// EnqueueProcessActivityEvent enqueues a task to process an activity event.
func (s *Service) EnqueueProcessActivityEvent(ctx context.Context, eventID pgtype.UUID) error {
	payload, err := json.Marshal(ProcessActivityEventPayload{
		EventID: eventID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshaling activity event payload: %w", err)
	}

	if s.enqueuer != nil {
		if err := s.enqueuer.Enqueue(worker.TypeWalletProcessActivityEvent, payload); err != nil {
			// A failed enqueue would strand the event unprocessed forever
			// (ADR-136 review) — fall back to synchronous processing instead
			// of dropping the award/penalty.
			slog.Warn("wallet: activity enqueue failed, processing inline", "event_id", eventID.String(), "error", err)
			return s.ProcessActivityEvent(ctx, eventID)
		}
		return nil
	}

	// Fallback: process synchronously when Asynq is not wired.
	return s.ProcessActivityEvent(ctx, eventID)
}

// ProcessActivityEventTask is the Asynq task handler for processing activity events.
// This would be registered with an Asynq worker in production.
func (s *Service) ProcessActivityEventTask(ctx context.Context, payload []byte) error {
	var p ProcessActivityEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("unmarshaling payload: %w", err)
	}

	var eventID pgtype.UUID
	if err := eventID.Scan(p.EventID); err != nil {
		return fmt.Errorf("invalid event_id: %w", err)
	}

	return s.ProcessActivityEvent(ctx, eventID)
}

// ProcessCreditPurchasePayload holds the payload for the
// wallet:process_credit_purchase task (Section 58). Enqueued by the payment
// webhook handler when a credit_purchase payment transitions to "succeeded".
type ProcessCreditPurchasePayload struct {
	PaymentID string `json:"payment_id"`

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

	var userID pgtype.UUID
	if err := userID.Scan(p.UserID); err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	if p.CreditsToGrant <= 0 {
		return fmt.Errorf("credits_to_grant must be positive, got %d", p.CreditsToGrant)
	}

	return s.grantCreditsFromPurchase(ctx, userID, p)
}

// grantCreditsFromPurchase inserts a credits ledger entry for a successful
// credit purchase. Uses an advisory lock + idempotency key to prevent
// double-crediting on duplicate task deliveries (Section 58, Rule 9).
func (s *Service) grantCreditsFromPurchase(ctx context.Context, userID pgtype.UUID, p ProcessCreditPurchasePayload) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := ledger.New(s.store.write.WithTx(tx))

	// Serialize concurrent balance mutations for this user.
	lockKey := "wallet::" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// Read current credits balance within the transaction.
	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		UserID:     userID,
		LedgerType: LedgerTypeCredits,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("getting credits balance: %w", err)
		}
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

	var paymentID pgtype.UUID
	if p.PaymentID != "" {
		if err := paymentID.Scan(p.PaymentID); err != nil {
			return fmt.Errorf("invalid payment_id: %w", err)
		}
	}

	if _, err = qtx.InsertCreditPurchaseEntry(ctx, gen.InsertLedgerEntryParams{
		UserID:           userID,
		LedgerType:       LedgerTypeCredits,
		Operation:        OperationPurchase,
		Amount:           int64(p.CreditsToGrant),
		ResultingBalance: newBalance,
		Source:           strPtr("credit_purchase"),
		Reference:        &reference,
		IdempotencyKey:   &idempotencyKey,
		Reason:           nil,
	}, paymentID); err != nil {
		// A unique-violation on the credit_purchase:{payment_id} key means a
		// prior delivery already credited — a successful no-op, not a
		// retryable failure (the old code dead-lettered on every replay).
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.Info("wallet: credit purchase already credited (idempotent)", "payment_id", p.PaymentID)

			return nil
		}
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
// // IdempotencyKey is optional. When empty the key is derived from
// user_id + reason + reference. Callers that need a cross-reference
// idempotency scope (e.g. assessment completion per form) should provide one.
type AwardPointsPayload struct {
	UserID         string `json:"user_id"`
	Points         int64  `json:"points"`
	Amount         int64  `json:"amount"`
	LedgerType     string `json:"ledger_type,omitempty"`
	Reason         string `json:"reason"`
	ReferenceID    string `json:"reference_id,omitempty"`
	GuideID        string `json:"guide_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	Nonce          string `json:"nonce,omitempty"`
}

// AwardPointsTask is the Asynq task handler for wallet:award_points. It
// inserts a ledger entry crediting the user's wallet. Idempotent via a
// deterministic idempotency key derived from user_id + reason + reference.
func (s *Service) AwardPointsTask(ctx context.Context, payload []byte) error {
	var p AwardPointsPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("wallet:award_points: unmarshaling payload: %w", err)
	}

	var userID pgtype.UUID
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

	source := p.Reason

	// If an admin configured a one-time activity point rule for this reason,
	// only award the first matching credit per user (Section 28).
	rule, err := s.store.getActivityRuleByKey(ctx, p.Reason)
	if err == nil && rule.OneTime {
		count, err := s.store.countUserLedgerBySource(ctx, userID, p.Reason, ledgerType)
		if err != nil {
			return fmt.Errorf("wallet:award_points: checking one-time rule: %w", err)
		}

		if count > 0 {
			slog.Info("wallet:award_points: one-time rule already satisfied, skipping", "reason", p.Reason, "user_id", userID.String())
			return nil
		}
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("wallet:award_points: failed to load activity rule", "reason", p.Reason, "error", err)
	}

	// Deterministic idempotency key — duplicate deliveries are safe.
	// Callers may override to enforce one-time awards across multiple
	// distinct references (e.g. assessment points once per form).
	idempotencyKey := p.IdempotencyKey
	if idempotencyKey == "" {
		if rule.OneTime {
			// Collapse references so a one-time rule credits exactly once per user.
			idempotencyKey = fmt.Sprintf("one_time::%s:%s", userID.String(), p.Reason)
		} else {
			idempotencyKey = fmt.Sprintf("award::%s:%s:%s", userID.String(), p.Reason, p.ReferenceID)
			if p.ReferenceID == "" {
				idempotencyKey = fmt.Sprintf("award::%s:%s:%s", userID.String(), p.Reason, p.GuideID)
			}

			if p.ReferenceID == "" && p.GuideID == "" {
				// Reference-less grants: the nonce (stamped at enqueue, stable
				// across retries) replaces the old per-amount collapse that
				// silently dropped a second same-amount award (PTS-020).
				idempotencyKey = fmt.Sprintf("award::%s:%s:%s", userID.String(), p.Reason, p.Nonce)
			}
		}
	}

	return s.creditWallet(ctx, userID, ledgerType, OperationEarn, amount, source, reference, idempotencyKey, true)
}

// ── wallet:reprocess_events (Section 28, PTS-024) ──────────────────────────
//
// Sweep for activity_events stranded unprocessed — a task that exhausted
// Asynq retries, an enqueue whose inline fallback also failed, or a
// dependent 'undoes' event whose award died before enqueueDependents ran.
// Without a sweep these rows sit unprocessed forever (the points/badge
// progress they represent is silently lost). Re-enqueueing is safe: the
// processor no-ops on processed events and dependents re-sleep until their
// award event settles.

// ReprocessEventsTask is the Asynq handler for wallet:reprocess_events.
// Runs on a cron schedule; the payload is empty.
func (s *Service) ReprocessEventsTask(ctx context.Context, _ []byte) error {
	return s.ReprocessUnprocessedEvents(ctx)
}

// ── wallet:reconcile_payouts (ALG-005, Section 58) ─────────────────────────

// ReconcilePayoutsTask is the Asynq task handler for wallet:reconcile_payouts.
// It settles withdrawals stuck in pending/processing by asking each provider
// for the live payout status.
func (s *Service) ReconcilePayoutsTask(ctx context.Context, _ []byte) error {
	return s.ReconcilePayouts(ctx)
}

// ReconcilePayouts polls in-flight withdrawals (pending / processing) and asks
// each provider for the current payout status. Providers without a status API
// (manual, manual_crypto, or any non-StatusProvider) are skipped — admins
// settle those by hand. "completed" marks the withdrawal completed; "failed"
// marks it failed and refunds the reserved credits.
func (s *Service) ReconcilePayouts(ctx context.Context) error {
	for _, st := range []string{WithdrawalStatusPending, WithdrawalStatusProcessing, WithdrawalStatusApproved} {
		status := st

		rows, err := s.store.read.ListWithdrawals(ctx, gen.ListWithdrawalsParams{
			Status:    &status,
			PageLimit: 200,
		})
		if err != nil {
			return fmt.Errorf("listing %s withdrawals: %w", st, err)
		}

		for _, w := range rows {
			s.settleWithdrawal(ctx, w)
		}
	}

	return nil
}

// settleWithdrawal resolves one in-flight withdrawal against its provider.
// 'approved' rows arrive from the atomic claim path (SEC-W-002): a crash
// between claim and provider call leaves status='approved' with no
// provider_ref — those are sent back to the approval queue, not polled.
func (s *Service) settleWithdrawal(ctx context.Context, w gen.WalletWithdrawal) {
	// Claimed but payout never initiated — nothing to poll; requeue for
	// review so the row can't strand in 'approved' forever.
	if w.Status == WithdrawalStatusApproved && (w.ProviderRef == nil || *w.ProviderRef == "") {
		note := "claimed but payout never initiated — auto-returned to approval queue by reconciler"
		if _, err := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
			ID:        w.ID,
			Status:    WithdrawalStatusPendingApproval,
			AdminNote: &note,
		}); err != nil {
			slog.Error("wallet: reconcile: unclaim stuck approved withdrawal failed", "withdrawal_id", w.ID.String(), "error", err)
		}
		return
	}

	provider, ok := s.payoutProviders[w.PayoutProvider]
	if !ok {
		return
	}

	sp, ok := provider.(payout.StatusProvider)
	if !ok {
		return
	}

	if w.ProviderRef == nil || *w.ProviderRef == "" {
		return
	}

	resp, err := sp.PayoutStatus(ctx, *w.ProviderRef)
	if err != nil {
		slog.Warn("wallet: reconcile: payout status check failed",
			"withdrawal_id", w.ID.String(), "provider", provider.Type(), "error", err)
		return
	}

	switch resp.Status {
	case WithdrawalStatusCompleted:
		if _, err := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
			ID: w.ID, Status: WithdrawalStatusCompleted,
		}); err != nil {
			slog.Error("wallet: reconcile: mark completed failed", "withdrawal_id", w.ID.String(), "error", err)
		}
	case WithdrawalStatusFailed:
		if _, err := s.store.write.UpdateWithdrawalStatus(ctx, gen.UpdateWithdrawalStatusParams{
			ID: w.ID, Status: WithdrawalStatusFailed,
		}); err != nil {
			slog.Error("wallet: reconcile: mark failed failed", "withdrawal_id", w.ID.String(), "error", err)
		} else if rerr := s.refundWithdrawalCredits(ctx, w.UserID, w.CreditAmount, w.ID); rerr != nil {
			slog.Error("wallet: reconcile: refund failed withdrawal", "withdrawal_id", w.ID.String(), "error", rerr)
		}
	}
}

// ── wallet:deduct_credits (Section 61 — Self-Serve Ads) ────────────────────

// DeductCreditsPayload holds the payload for the wallet:deduct_credits task.
// Enqueued by the self-serve ads service for campaign funding (deduct) and
// campaign refund (credit back). The organization_id is resolved to the
// org owner's user_id for the ledger entry.
type DeductCreditsPayload struct {
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
		return errors.New("wallet:deduct_credits: either user_id or organization_id is required")
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

	return s.creditWallet(ctx, userID, LedgerTypeCredits, operation, ledgerAmount, p.Reason, reference, idempotencyKey, false)
}

// earnAllowed reports whether a positive points earn of `amount` may be
// credited to the user — the EnableEarning killswitch and the daily earn
// cap (tier-scoped `wallet.max_daily_earn_points.{tier}`, then the global
// key, then the wallet_settings.daily_earning_cap fallback; -1 = unlimited)
// per ADR-136. Called inside the per-user advisory lock so the cap check
// and the ledger insert are atomic with concurrent earns.
//
// exemptCap skips the daily-cap check only (PTS-026): the killswitch always
// applies — it is the admin emergency brake — but committed rewards issued
// via wallet:award_points / AwardPoints (badge points_reward, challenge
// reward_points, checklist/referral/scholarship grants) must not be
// silently voided by the activity-earning cap.
func (s *Service) earnAllowed(ctx context.Context, tx pgx.Tx, userID pgtype.UUID, amount int64, exemptCap bool) (bool, error) {
	// Killswitch — gates earnings only (penalties/compensation never reach
	// this path).
	if ws, werr := s.store.getWalletSettings(ctx); werr == nil && !ws.EnableEarning {
		return false, nil
	}

	if exemptCap || s.settings == nil {
		return true, nil
	}

	dailyCap := int64(s.settings.GetIntOr(ctx, "wallet.max_daily_earn_points", 0))
	if s.tierResolver != nil {
		if tier, terr := s.tierResolver.ResolveUserTier(ctx, userID); terr == nil && tier != "" {
			fallback := int(dailyCap)
			if d, ok := defaultStaffEarnCaps[tier]; ok {
				fallback = d
			}
			dailyCap = int64(s.settings.GetIntOr(ctx, "wallet.max_daily_earn_points."+tier, fallback))
		}
	}

	if dailyCap == 0 {
		if ws, werr := s.store.getWalletSettings(ctx); werr == nil && ws.DailyEarningCap > 0 {
			dailyCap = int64(ws.DailyEarningCap)
		}
	}

	if dailyCap <= 0 {
		return true, nil
	}

	var earnedToday int64
	// idempotency_key LIKE 'activity::%' scopes the sum to activity-event
	// earns only (Section 28) — reward earns routed through
	// wallet:award_points ('award::'/'one_time::' keys: badge points_reward,
	// challenge reward_points, checklist/referral/scholarship grants) are
	// real achievements and never consume the daily cap.
	row := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM wallet_ledger
		WHERE user_id = $1 AND ledger_type = 'points' AND amount > 0
		  AND operation = 'earn' AND created_at >= CURRENT_DATE
		  AND idempotency_key LIKE 'activity::%'`, userID)
	if err := row.Scan(&earnedToday); err != nil {
		return false, fmt.Errorf("checking daily earning cap: %w", err)
	}

	return earnedToday+amount <= dailyCap, nil
}

// creditWallet is the shared ledger insertion path for award_points and
// deduct_credits. It acquires a per-user advisory lock, reads the current
// balance, computes the new balance, and inserts the ledger entry — all
// within a single transaction (AUDIT-BE-005 pattern).
func (s *Service) creditWallet(ctx context.Context, userID pgtype.UUID, ledgerType, operation string, amount int64, source, reference, idempotencyKey string, exemptCap bool) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("wallet: beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := ledger.New(s.store.write.WithTx(tx))

	// Serialize concurrent balance mutations for this user.
	lockKey := "wallet::" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("wallet: acquiring lock: %w", err)
	}

	// PTS-004: the earn gate (EnableEarning killswitch + daily earn cap)
	// applies to every positive points earn — not just the activity-event
	// path. Credits, spends/refunds, and penalties are exempt per ADR-136
	// (the gates guard earnings only). PTS-026: reward earns routed via
	// wallet:award_points / AwardPoints pass exemptCap=true — the daily cap
	// throttles activity earnings, but a committed reward must pay out;
	// the killswitch still applies. A gated award is a settled no-op,
	// not a retryable failure.
	if ledgerType == LedgerTypePoints && operation == OperationEarn && amount > 0 {
		allowed, gerr := s.earnAllowed(ctx, tx, userID, amount, exemptCap)
		if gerr != nil {
			return fmt.Errorf("wallet: checking earn gate: %w", gerr)
		}

		if !allowed {
			slog.Info("wallet: earn gated by killswitch/daily cap, skipping",
				"user_id", userID.String(), "amount", amount, "source", source)
			return nil
		}
	}

	// SEC-W-014: earned credits are minted without fiat backing — cap the
	// rolling-24h earn total so activity-farming can't inflate a balance
	// faster than ops can respond. Unlike the points gate this applies even
	// to exemptCap callers (award/referral tasks): exemptCap was designed
	// for the activity cap — a monetary cap is a different control and a
	// reward that would exceed it is an ops-level event, not a silent drop.
	// wallet.max_daily_earn_credits=0 disables.
	if ledgerType == LedgerTypeCredits && operation == OperationEarn && amount > 0 && s.settings != nil {
		if cap := s.settings.GetIntOr(ctx, "wallet.max_daily_earn_credits", 0); cap > 0 {
			earned, cerr := qtx.GetUserCreditEarnsSince(ctx, gen.GetUserCreditEarnsSinceParams{
				UserID: userID,
				Since:  pgtype.Timestamptz{Time: time.Now().Add(-24 * time.Hour), Valid: true},
			})
			if cerr != nil {
				return fmt.Errorf("wallet: checking credits earn cap: %w", cerr)
			}

			if earned+amount > int64(cap) {
				// A capped award is reported, not silently dropped — it
				// likely signals farming or a runaway task, and the reward
				// is still owed (ops reviews and grants manually).
				slog.Warn("wallet: credits earn gated by daily cap",
					"user_id", userID.String(), "amount", amount, "earned_24h", earned, "cap", cap, "source", source)
				errpkg.CaptureException(fmt.Errorf("wallet: credits earn capped: user=%s amount=%d earned_24h=%d cap=%d source=%s",
					userID.String(), amount, earned, cap, source))
				return nil
			}
		}
	}

	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
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

	// Non-negative balance guard (§1.3, Checklist B.10) — floors apply to
	// EVERY ledger. Deductions that would go negative are blocked; clawback
	// clamps to what exists (recover all available, log the shortfall)
	// instead of pushing the balance negative (SEC-W-017 policy: balances
	// never go below 0).
	if newBalance < 0 {
		if operation != OperationClawback {
			slog.Warn("wallet: deduct would result in negative balance, skipping", "user_id", userID.String(), "ledger_type", ledgerType, "current", currentBalance, "amount", amount)

			return fmt.Errorf("wallet: insufficient balance: current=%d, delta=%d", currentBalance, amount)
		}
		// Clamped clawback: take what's there, flag the unrecoverable delta.
		slog.Warn("wallet: clawback clamped to available balance",
			"user_id", userID.String(), "ledger_type", ledgerType,
			"current", currentBalance, "requested", amount,
			"shortfall", newBalance)
		amount = -currentBalance
		newBalance = 0
	}

	src := source
	if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
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
		// A duplicate idempotency key means this award was already processed;
		// treat it as a successful no-op (Rule 9).
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.Info("wallet: duplicate award idempotency key, skipping", "user_id", userID.String(), "ledger_type", ledgerType, "idempotency_key", idempotencyKey)

			return nil
		}

		return fmt.Errorf("wallet: inserting ledger entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("wallet: committing transaction: %w", err)
	}

	slog.Info("wallet: ledger entry inserted", "user_id", userID.String(), "ledger_type", ledgerType, "operation", operation, "amount", amount, "new_balance", newBalance, "source", source)

	return nil
}

// ── wallet:reconcile_solvency (SEC-W-012) ────────────────────────────────

// ReconcileSolvencyTask is the Asynq handler for wallet:reconcile_solvency.
// It runs the platform-wide integrity probes — the aggregate form of the
// per-user provenance checks applied at withdrawal time. Any non-zero
// result means unbacked credits exist somewhere; that's an ops page, not a
// silent log line.
func (s *Service) ReconcileSolvencyTask(ctx context.Context, _ []byte) error {
	var flagged bool

	diverged, err := s.store.read.CountDivergentCreditsLedgers(ctx)
	if err != nil {
		return fmt.Errorf("solvency: counting divergent ledgers: %w", err)
	}
	if diverged > 0 {
		flagged = true
		slog.Error("wallet: solvency: credits ledgers diverged from entry sums",
			"user_count", diverged)
		errpkg.CaptureException(fmt.Errorf("wallet solvency: %d credits ledgers diverged (sum != reported balance)", diverged))
	}

	unbacked, err := s.store.read.CountAllUnbackedPurchaseCredits(ctx)
	if err != nil {
		return fmt.Errorf("solvency: counting unbacked purchases: %w", err)
	}
	if unbacked.RowCount > 0 {
		flagged = true
		slog.Error("wallet: solvency: unbacked purchase credits exist",
			"row_count", unbacked.RowCount, "credit_total", unbacked.CreditTotal)
		errpkg.CaptureException(fmt.Errorf("wallet solvency: %d purchase entries (%d credits) lack a succeeded payment",
			unbacked.RowCount, unbacked.CreditTotal))
	}

	anonAdjusts, err := s.store.read.CountAllAnonymousAdminAdjusts(ctx)
	if err != nil {
		return fmt.Errorf("solvency: counting anonymous admin adjusts: %w", err)
	}
	if anonAdjusts.RowCount > 0 {
		flagged = true
		slog.Error("wallet: solvency: admin adjustments without actor exist",
			"row_count", anonAdjusts.RowCount, "credit_total", anonAdjusts.CreditTotal)
		errpkg.CaptureException(fmt.Errorf("wallet solvency: %d admin adjustments (%d credits) lack an actor",
			anonAdjusts.RowCount, anonAdjusts.CreditTotal))
	}

	// SEC-W-NGN float-vs-liability: an underfunded payout rail means
	// pending withdrawals will fail at the provider — flag before ops
	// notices from support tickets. Compares per provider+currency;
	// providers without a balance API are skipped.
	providerFloats := s.PayoutFloat(ctx)
	liabilities, err := s.store.read.GetInFlightWithdrawalTotalsByProvider(ctx)
	if err != nil {
		return fmt.Errorf("solvency: reading in-flight withdrawal totals: %w", err)
	}
	for _, liab := range liabilities {
		if liab.RowCount == 0 || liab.NetTotal <= 0 {
			continue
		}
		provFloat, ok := providerFloats[liab.PayoutProvider]
		if !ok {
			continue
		}
		balances, ok := provFloat.(map[string]float64)
		if !ok {
			continue // provider reported an error or no balance API
		}
		avail, ok := balances[strings.ToUpper(liab.CurrencyCode)]
		if !ok {
			continue // provider holds no balance in this currency
		}
		liability := float64(liab.NetTotal) / math.Pow10(payout.FiatExponent(liab.CurrencyCode))
		if avail < liability {
			flagged = true
			slog.Error("wallet: solvency: payout float below in-flight liability",
				"provider", liab.PayoutProvider, "currency", liab.CurrencyCode,
				"available", avail, "liability", liability, "pending_withdrawals", liab.RowCount)
			errpkg.CaptureException(fmt.Errorf("wallet solvency: %s float %.2f %s < %.2f in %d in-flight withdrawals",
				liab.PayoutProvider, avail, liab.CurrencyCode, liability, liab.RowCount))
		}
	}

	if !flagged {
		slog.Info("wallet: solvency check clean")
	}

	return nil
}
