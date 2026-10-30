package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// EmitActivityEventInput holds parameters for emitting an activity event.
type EmitActivityEventInput struct {
	ActivityKey string
	Reference   string
	Payload     map[string]interface{}
}

// EmitActivityEvent records an activity event for async processing.
func (s *Service) EmitActivityEvent(ctx context.Context, userID pgtype.UUID, input EmitActivityEventInput) (gen.ActivityEvent, error) {
	if input.ActivityKey == "" {
		return gen.ActivityEvent{}, errors.New("activity_key is required")
	}

	if input.Reference == "" {
		return gen.ActivityEvent{}, errors.New("reference is required")
	}

	// Convert payload to JSON bytes
	var payload []byte

	if input.Payload != nil {
		var err error

		payload, err = json.Marshal(input.Payload)
		if err != nil {
			return gen.ActivityEvent{}, fmt.Errorf("marshaling payload: %w", err)
		}
	} else {
		payload = []byte("{}")
	}

	event, err := s.store.insertActivityEvent(ctx, gen.InsertActivityEventParams{
		UserID:      userID,
		ActivityKey: input.ActivityKey,
		Reference:   input.Reference,
		Payload:     payload,
	})
	if err != nil {
		return gen.ActivityEvent{}, err
	}

	return event, nil
}

// ProcessActivityEvent processes a single activity event (called by Asynq task).
func (s *Service) ProcessActivityEvent(ctx context.Context, eventID pgtype.UUID) error {
	// Get the event
	event, err := s.store.getActivityEventByID(ctx, eventID)
	if err != nil {
		return fmt.Errorf("getting activity event: %w", err)
	}

	// BUG-W-001: skip already-processed events. Asynq retries the task if the
	// worker crashes after committing the ledger entry but before acking. The
	// ledger idempotency key would catch a duplicate insert, but as a unique-
	// constraint error that re-fails the task and creates a poison-pill retry
	// loop. Checking the processed flag turns a retry into a clean no-op.
	if event.Processed != nil && *event.Processed {
		return nil
	}

	// Get the activity rule
	rule, err := s.store.getActivityRuleByKey(ctx, event.ActivityKey)
	if err != nil {
		// If rule not found, skip processing (rule may have been deleted).
		// Distinguish "not found" from real DB errors (BUG-007). The activity
		// still counts for badges/challenges even without a point rule.
		if errors.Is(err, pgx.ErrNoRows) {
			s.settleWithoutPoints(ctx, event)
			return nil
		}

		return fmt.Errorf("getting activity rule: %w", err)
	}

	// Check if rule is enabled
	if rule.IsEnabled != nil && !*rule.IsEnabled {
		s.settleWithoutPoints(ctx, event)
		return nil
	}

	// Check the global earning killswitch (BUG-003). EnableEarning gates
	// *earning* only — penalty rules (negative point_value, ADR-136) are
	// moderation signals and always apply.
	if s.settings != nil && rule.PointValue > 0 {
		ws, werr := s.store.getWalletSettings(ctx)
		if werr == nil {
			// EnableEarning killswitch — no points, but the activity still
			// counts toward badges/challenges.
			if !ws.EnableEarning {
				s.settleWithoutPoints(ctx, event)
				return nil
			}
		}
	}

	// Check frequency caps (read-only queries outside tx — safe, caps are advisory)
	if rule.CapPerDay != nil {
		count, err := s.store.countUserActivityToday(ctx, event.UserID, event.ActivityKey)
		if err != nil {
			return fmt.Errorf("checking daily cap: %w", err)
		}

		if count > int64(*rule.CapPerDay) {
			s.settleWithoutPoints(ctx, event)
			return nil // Cap exceeded, skip
		}
	}

	if rule.CapPerWeek != nil {
		count, err := s.store.countUserActivityThisWeek(ctx, event.UserID, event.ActivityKey)
		if err != nil {
			return fmt.Errorf("checking weekly cap: %w", err)
		}

		if count > int64(*rule.CapPerWeek) {
			s.settleWithoutPoints(ctx, event)
			return nil // Cap exceeded, skip
		}
	}

	// Begin transaction to atomically read balance and insert ledger entry.
	//  Previously this path lacked both a tx and an
	// advisory lock, so concurrent earns could silently overwrite each other.
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	// Serialize concurrent balance mutations for this user.
	lockKey := "wallet:" + "" + ":" + event.UserID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// ADR-136 compensation events: a revert (reaction.reverted /
	// reaction.negative.reverted) carries {"undoes_key","undoes"} pointing at
	// the award it undoes and applies the exact negative of that award's
	// ledger amount — not its own rule's point_value. This converges the
	// balance regardless of rule tuning, never deducts for an award that
	// never credited (cap, killswitch, disabled rule), and retries if the
	// award event hasn't processed yet (out-of-order delivery).
	var undoes, undoesKey string
	var payloadObj struct {
		Undoes    string `json:"undoes"`
		UndoesKey string `json:"undoes_key"`
	}
	if len(event.Payload) > 0 {
		_ = json.Unmarshal(event.Payload, &payloadObj)
	}
	undoes, undoesKey = payloadObj.Undoes, payloadObj.UndoesKey

	if undoes != "" && undoesKey != "" {
		refEvent, rerr := qtx.GetActivityEventByReference(ctx, gen.GetActivityEventByReferenceParams{
			ActivityKey: undoesKey,
			Reference:   undoes,
		})
		if errors.Is(rerr, pgx.ErrNoRows) {
			// No award event exists — nothing to undo. Mark processed.
			if err := qtx.MarkEventProcessed(ctx, eventID); err != nil {
				return fmt.Errorf("marking no-op revert processed: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("committing no-op revert: %w", err)
			}
			s.evaluateGamification(ctx, event)
			return nil
		}
		if rerr != nil {
			return fmt.Errorf("getting undone event: %w", rerr)
		}
		if refEvent.Processed == nil || !*refEvent.Processed {
			// The award event hasn't run yet — retry so the compensation
			// always lands after the award it undoes.
			return fmt.Errorf("undone award %s not yet processed", undoes)
		}

		entry, lerr := qtx.GetLedgerEntryByReference(ctx, gen.GetLedgerEntryByReferenceParams{
			UserID:     event.UserID,
			LedgerType: LedgerTypePoints,
			Reference:  &undoes,
		})
		if errors.Is(lerr, pgx.ErrNoRows) {
			// The award event processed but credited nothing (cap,
			// killswitch, disabled) — there is no debt to undo.
			if err := qtx.MarkEventProcessed(ctx, eventID); err != nil {
				return fmt.Errorf("marking uncredited revert processed: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("committing uncredited revert: %w", err)
			}
			s.evaluateGamification(ctx, event)
			return nil
		}
		if lerr != nil {
			return fmt.Errorf("getting undone ledger entry: %w", lerr)
		}

		// Compensation = exact negative of what actually credited. Skip the
		// caps/killswitch entirely — a correction is not an earning.
		compensation := -entry.Amount
		balance, berr := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
			UserID:     event.UserID,
			LedgerType: LedgerTypePoints,
		})
		if berr != nil && !errors.Is(berr, pgx.ErrNoRows) {
			return fmt.Errorf("getting balance for revert: %w", berr)
		}
		newBalance := balance + compensation
		idempotencyKey := fmt.Sprintf("activity::%s:%s:%s", event.UserID.String(), event.ActivityKey, event.Reference)
		if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			UserID:           event.UserID,
			LedgerType:       LedgerTypePoints,
			Operation:        OperationEarn,
			Amount:           compensation,
			ResultingBalance: newBalance,
			Source:           &event.ActivityKey,
			Reference:        &event.Reference,
			IdempotencyKey:   &idempotencyKey,
		}); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				if merr := qtx.MarkEventProcessed(ctx, eventID); merr != nil {
					return fmt.Errorf("marking duplicate revert processed: %w", merr)
				}
				if cerr := tx.Commit(ctx); cerr != nil {
					return fmt.Errorf("committing duplicate revert: %w", cerr)
				}
				s.evaluateGamification(ctx, event)
				return nil
			}
			return fmt.Errorf("inserting revert ledger entry: %w", err)
		}
		if err := qtx.MarkEventProcessed(ctx, eventID); err != nil {
			return fmt.Errorf("marking revert processed: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("committing revert: %w", err)
		}
		s.evaluateGamification(ctx, event)
		return nil
	}

	// One-time rules: award only the first matching activity per user.
	// Checked after the per-user lock so concurrent first-time events for the
	// same source cannot both pass the count and insert.
	if rule.OneTime {
		count, err := qtx.CountUserLedgerBySource(ctx, gen.CountUserLedgerBySourceParams{
			UserID: event.UserID,
			Source: &event.ActivityKey,
		})
		if err != nil {
			return fmt.Errorf("checking one-time rule ledger: %w", err)
		}

		if count > 0 {
			// Mark processed so the event is not retried.
			if err := qtx.MarkEventProcessed(ctx, eventID); err != nil {
				return fmt.Errorf("marking one-time event processed: %w", err)
			}

			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("committing one-time event skip: %w", err)
			}

			// The activity still counts toward badges/challenges.
			s.evaluateGamification(ctx, event)

			return nil
		}
	}

	// Get current balance within the transaction.
	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		UserID:     event.UserID,
		LedgerType: LedgerTypePoints,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("getting current balance: %w", err)
		}

		currentBalance = 0
	}

	// Enforce the global daily earning cap (BUG-003). Sum all earn ledger
	// entries for this user today; if adding this rule's points would exceed
	// the cap, skip the award. The cap is read from wallet_settings
	// (DailyEarningCap) or system_settings (wallet.max_daily_earn_points).
	// Penalty rules (negative point_value, ADR-136) are not earnings — the
	// cap does not gate them.
	dailyCap := int64(0)
	if rule.PointValue > 0 && s.settings != nil {
		dailyCap = int64(s.settings.GetIntOr(ctx, "wallet.max_daily_earn_points", 0))
		// Tier-scoped override (wallet.max_daily_earn_points.{tier}) — a
		// negative value means unlimited for that tier (ADR-136); a missing
		// key falls back to the global cap.
		if s.tierResolver != nil {
			if tier, terr := s.tierResolver.ResolveUserTier(ctx, event.UserID); terr == nil && tier != "" {
				dailyCap = int64(s.settings.GetIntOr(ctx, "wallet.max_daily_earn_points."+tier, int(dailyCap)))
			}
		}
	}

	if rule.PointValue > 0 && dailyCap == 0 {
		// Fall back to wallet_settings DailyEarningCap.
		ws, werr := s.store.getWalletSettings(ctx)
		if werr == nil && ws.DailyEarningCap > 0 {
			dailyCap = int64(ws.DailyEarningCap)
		}
	}

	if dailyCap > 0 {
		var earnedToday int64

		// amount > 0: penalty entries (negative 'earn', ADR-136) must not
		// refund cap headroom — the cap throttles points earned, not net
		// balance change.
		row := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(amount), 0) FROM wallet_ledger
			WHERE user_id = $1 AND ledger_type = 'points' AND amount > 0
			  AND operation = 'earn' AND created_at >= CURRENT_DATE`, event.UserID)
		if qerr := row.Scan(&earnedToday); qerr != nil {
			return fmt.Errorf("checking daily earning cap: %w", qerr)
		}

		if dailyCap > 0 && earnedToday+int64(rule.PointValue) > dailyCap {
			// Cap reached — no points, but the activity still counts toward
			// badges/challenges. Mark processed inside the tx for exactly-once
			// evaluation on any replay (same pattern as the one-time skip).
			if merr := qtx.MarkEventProcessed(ctx, eventID); merr != nil {
				return fmt.Errorf("marking capped event processed: %w", merr)
			}

			if cerr := tx.Commit(ctx); cerr != nil {
				return fmt.Errorf("committing capped event skip: %w", cerr)
			}

			s.evaluateGamification(ctx, event)

			return nil
		}
	}

	// Calculate new balance
	newBalance := currentBalance + int64(rule.PointValue)

	// Create idempotency key from event. Include platform and user to avoid
	// cross-platform collisions (ADR-004, Section 28).
	idempotencyKey := fmt.Sprintf("activity::%s:%s:%s", event.UserID.String(), event.ActivityKey, event.Reference)

	// Insert ledger entry
	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		UserID:           event.UserID,
		LedgerType:       LedgerTypePoints,
		Operation:        OperationEarn,
		Amount:           int64(rule.PointValue),
		ResultingBalance: newBalance,
		Source:           &event.ActivityKey,
		Reference:        &event.Reference,
		IdempotencyKey:   &idempotencyKey,
		Reason:           nil,
	})
	if err != nil {
		// A duplicate idempotency key means an equivalent event was already
		// credited — a second activity_events row for the same
		// (user, key, reference) arrives here when emitters replay. Treat it
		// as already-processed instead of dead-lettering the task (ALG-019).
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if merr := qtx.MarkEventProcessed(ctx, eventID); merr != nil {
				return fmt.Errorf("marking duplicate event processed: %w", merr)
			}

			if cerr := tx.Commit(ctx); cerr != nil {
				return fmt.Errorf("committing duplicate event skip: %w", cerr)
			}

			return nil
		}

		return fmt.Errorf("inserting ledger entry: %w", err)
	}

	// Mark event as processed within the transaction.
	if err := qtx.MarkEventProcessed(ctx, eventID); err != nil {
		return fmt.Errorf("marking event processed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	// Evaluate gamification badges after the ledger earn is committed
	// (BUG-GAMMA-001, ADR-082 §2). Fire-and-forget — a badge evaluation
	// failure must not fail the already-committed ledger entry; the event
	// is marked processed and the points are already credited.
	s.evaluateGamification(ctx, event)

	return nil
}

// evaluateGamification runs badge and challenge evaluation for the activity.
// Fire-and-forget (Rule 8): a failure is logged, never propagated — the
// points decision is already settled by the time this runs.
func (s *Service) evaluateGamification(ctx context.Context, event gen.ActivityEvent) {
	if s.gamificationEvaluator == nil {
		return
	}

	if err := s.gamificationEvaluator.EvaluateForWallet(ctx, event.UserID, event.ActivityKey); err != nil {
		slog.Warn("wallet: gamification badge evaluation failed",
			"user_id", event.UserID.String(),
			"activity_key", event.ActivityKey,
			"error", err)
	}
}

// settleWithoutPoints marks the event processed and runs gamification
// evaluation for it. Used on paths where the activity occurred but no
// points are credited — no rule, disabled rule, or frequency caps.
// Badges/challenges count the activity itself, not the payout (Section 64);
// marking processed gives exactly-once evaluation on any replay.
func (s *Service) settleWithoutPoints(ctx context.Context, event gen.ActivityEvent) {
	if err := s.store.markEventProcessed(ctx, event.ID); err != nil {
		slog.Warn("wallet: mark skipped event processed failed",
			"event_id", event.ID.String(), "error", err)
	}

	s.evaluateGamification(ctx, event)
}

// GetBalance returns the current balance for a user (points and/or credits).
func (s *Service) GetBalance(ctx context.Context, userID pgtype.UUID) (points, credits int64, err error) {
	points, err = s.store.getLatestBalance(ctx, userID, LedgerTypePoints)
	if err != nil {
		// Only treat "no rows" as zero balance; propagate real DB errors
		// (BUG-004 — previously swallowed all errors as "no balance yet").
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, fmt.Errorf("getting points balance: %w", err)
		}

		points = 0
	}

	credits, err = s.store.getLatestBalance(ctx, userID, LedgerTypeCredits)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, fmt.Errorf("getting credits balance: %w", err)
		}

		credits = 0
	}

	return points, credits, nil
}

// GetLifetimePoints returns the total positive points ever earned by a user
// (never decreases — used for badges/tiers per Section 28). BUG-W-003.
func (s *Service) GetLifetimePoints(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return s.store.read.GetLifetimePoints(ctx,

		userID,
	)
}

// GetPointsBalance returns only the Points balance for a user. Used by the
// users service to surface points on a public profile when the user has
// consented via show_points_publicly (Section 28). Credits are never exposed.
func (s *Service) GetPointsBalance(ctx context.Context, userID pgtype.UUID) (int64, error) {
	points, _, err := s.GetBalance(ctx, userID)
	return points, err
}

// SpendCreditsInput holds parameters for spending credits.
type SpendCreditsInput struct {
	Amount      int64
	Source      string
	Reference   string
	PurposeType string
	PurposeID   string
}

// SpendCredits spends credits (synchronous, not a Provider).
func (s *Service) SpendCredits(ctx context.Context, userID pgtype.UUID, input SpendCreditsInput) (int64, error) {
	if input.Amount <= 0 {
		return 0, errors.New("amount must be positive")
	}

	// Begin transaction to atomically check balance and insert ledger entry.
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	// Serialize concurrent balance mutations for this user. See AUDIT-BE-005.
	lockKey := "wallet:" + "" + ":" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return 0, fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// Get current credits balance within the transaction. A user with no
	// ledger rows has a zero balance — NoRows must not leak through as a
	// generic 500/404 upstream.
	currentCredits, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		UserID:     userID,
		LedgerType: LedgerTypeCredits,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%w: no credits ledger", errpkg.ErrInsufficientBalance)
		}

		return 0, fmt.Errorf("getting credits balance: %w", err)
	}

	// Check sufficient balance
	if currentCredits < input.Amount {
		return 0, fmt.Errorf("%w: insufficient credits balance", errpkg.ErrInsufficientBalance)
	}

	// New balance
	newBalance := currentCredits - input.Amount

	// Create idempotency key
	idempotencyKey := fmt.Sprintf("spend:%x:%s:%s", userID.Bytes, input.Source, input.Reference)

	// Insert ledger entry
	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		UserID:           userID,
		LedgerType:       LedgerTypeCredits,
		Operation:        OperationSpend,
		Amount:           -input.Amount,
		ResultingBalance: newBalance,
		Source:           &input.Source,
		Reference:        &input.Reference,
		IdempotencyKey:   &idempotencyKey,
		Reason:           nil,
	})
	if err != nil {
		return 0, fmt.Errorf("inserting spend entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("committing transaction: %w", err)
	}

	return newBalance, nil
}

// AdminAdjustInput holds parameters for admin adjustment.
type AdminAdjustInput struct {
	UserID     string
	LedgerType string
	Amount     int64
	Reason     string
	Reference  string
}

// AdminAdjustBalance performs a manual admin adjustment.
func (s *Service) AdminAdjustBalance(ctx context.Context, actorUserID pgtype.UUID, input AdminAdjustInput) (int64, error) {
	if input.Amount == 0 {
		return 0, errors.New("amount cannot be zero")
	}

	if input.Reason == "" {
		return 0, errors.New("reason is required for admin adjustments")
	}

	if input.LedgerType != LedgerTypePoints && input.LedgerType != LedgerTypeCredits {
		return 0, fmt.Errorf("invalid ledger type: %s", input.LedgerType)
	}

	// Parse target user ID
	var targetUserID pgtype.UUID
	if err := targetUserID.Scan(input.UserID); err != nil {
		return 0, fmt.Errorf("invalid user_id: %w", err)
	}

	// Begin transaction to atomically check balance and insert ledger entry.
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	// Serialize concurrent balance mutations for the target user. See AUDIT-BE-005.
	lockKey := "wallet:" + "" + ":" + targetUserID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return 0, fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// Get current balance within the transaction.
	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		UserID:     targetUserID,
		LedgerType: input.LedgerType,
	})
	if err != nil {
		currentBalance = 0
	}

	// New balance
	newBalance := currentBalance + input.Amount

	// Non-negative balance lock (§1.3, Checklist B.10).
	if newBalance < 0 {
		return 0, fmt.Errorf("%w: current=%d, adjustment=%d", errpkg.ErrInsufficientBalance, currentBalance, input.Amount)
	}

	// Create idempotency key — include a UUID so multiple adjustments with
	// the same reason and amount to the same user don't collide (BUG-006).
	// The admin adjust route lacks client-supplied idempotency middleware, so
	// a server-generated unique key is the correct approach.
	idempotencyKey := fmt.Sprintf("admin_adjust:%s:%x:%s:%d", uuid.NewString(), targetUserID.Bytes, input.Reason, input.Amount)

	// Insert ledger entry
	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		UserID:           targetUserID,
		LedgerType:       input.LedgerType,
		Operation:        OperationAdminAdjust,
		Amount:           input.Amount,
		ResultingBalance: newBalance,
		Source:           strPtr("admin_adjust"),
		ActorUserID:      actorUserID,
		Reference:        &input.Reference,
		IdempotencyKey:   &idempotencyKey,
		Reason:           &input.Reason,
	})
	if err != nil {
		return 0, fmt.Errorf("inserting admin adjust entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("committing transaction: %w", err)
	}

	return newBalance, nil
}

// GetWalletSettings returns wallet settings. When no wallet_settings row
// exists yet, compiled-in defaults are synthesized (ADR-136 — conversion
// settings removed; only earning cap + killswitch remain).
func (s *Service) GetWalletSettings(ctx context.Context) (gen.WalletSetting, error) {
	settings, err := s.store.getWalletSettings(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.WalletSetting{
				DailyEarningCap: 1000,
				EnableEarning:   true,
			}, nil
		}

		return gen.WalletSetting{}, err
	}

	return settings, nil
}

// UpdateWalletSettingsInput holds parameters for updating wallet settings.
type UpdateWalletSettingsInput struct {
	DailyEarningCap int32
	EnableEarning   bool
}

// UpdateWalletSettings updates wallet settings for a platform.
func (s *Service) UpdateWalletSettings(ctx context.Context, input UpdateWalletSettingsInput) (gen.WalletSetting, error) {
	return s.store.upsertWalletSettings(ctx, input.DailyEarningCap, input.EnableEarning)
}

// ListActivityRules returns activity rules for a platform.
func (s *Service) ListActivityRules(ctx context.Context, limit, offset int32) ([]gen.ActivityPointRule, error) {
	return s.store.listActivityRulesByPlatform(ctx, limit, offset)
}

// CreateOrUpdateActivityRuleInput holds parameters for creating/updating an activity rule.
type CreateOrUpdateActivityRuleInput struct {
	ActivityKey string
	PointValue  int32
	IsEnabled   bool
	OneTime     bool
	CapPerDay   *int32
	CapPerWeek  *int32
	Description *string
}

// CreateOrUpdateActivityRule creates or updates an activity rule.
func (s *Service) CreateOrUpdateActivityRule(ctx context.Context, input CreateOrUpdateActivityRuleInput) (gen.ActivityPointRule, error) {
	if input.ActivityKey == "" {
		return gen.ActivityPointRule{}, errors.New("activity_key is required")
	}

	return s.store.upsertActivityRule(ctx, gen.UpsertActivityRuleParams{
		ActivityKey: input.ActivityKey,
		PointValue:  input.PointValue,
		IsEnabled:   &input.IsEnabled,
		OneTime:     input.OneTime,
		CapPerDay:   input.CapPerDay,
		CapPerWeek:  input.CapPerWeek,
		Description: input.Description,
	})
}

// ToggleActivityRule toggles an activity rule's enabled state.
func (s *Service) ToggleActivityRule(ctx context.Context, id pgtype.UUID) (gen.ActivityPointRule, error) {
	return s.store.toggleActivityRule(ctx, id)
}

// UpdateActivityRuleByID updates an activity rule by its primary key (BUG-005).
// Unlike CreateOrUpdateActivityRule (which upserts by activity_key), this
// method updates the rule identified by {id} — the {id} from the URL route
// parameter — so PATCH /admin/wallet/rules/{id} updates the correct rule.
func (s *Service) UpdateActivityRuleByID(ctx context.Context, id pgtype.UUID, input CreateOrUpdateActivityRuleInput) (gen.ActivityPointRule, error) {
	return s.store.updateActivityRuleByID(ctx, gen.UpdateActivityRuleByIDParams{
		ID: id,

		PointValue:  input.PointValue,
		IsEnabled:   &input.IsEnabled,
		OneTime:     input.OneTime,
		CapPerDay:   input.CapPerDay,
		CapPerWeek:  input.CapPerWeek,
		Description: input.Description,
	})
}

// DeleteActivityRule deletes an activity rule.
func (s *Service) DeleteActivityRule(ctx context.Context, id pgtype.UUID) error {
	return s.store.deleteActivityRule(ctx, id)
}

// ListLedgerEntries returns ledger entries for a user.
func (s *Service) ListLedgerEntries(ctx context.Context, userID pgtype.UUID, ledgerType *string, limit, offset int32) ([]gen.WalletLedger, error) {
	return s.store.listLedgerEntriesByUser(ctx, userID, ledgerType, limit, offset)
}

// GetLedgerEntry returns a ledger entry by ID.
func (s *Service) GetLedgerEntry(ctx context.Context, id pgtype.UUID) (gen.WalletLedger, error) {
	return s.store.getLedgerEntryByID(ctx, id)
}

// GetUserBalance returns the balance for any user (admin operation).
func (s *Service) GetUserBalance(ctx context.Context, userID pgtype.UUID) (points, credits int64, err error) {
	return s.GetBalance(ctx, userID)
}

// GetUserBalanceWithInfo returns balance plus basic user info for admin display.
func (s *Service) GetUserBalanceWithInfo(ctx context.Context, userID pgtype.UUID) (username string, points, credits int64, err error) {
	points, credits, err = s.GetBalance(ctx, userID)
	if err != nil {
		return "", 0, 0, err
	}

	user, err := s.store.read.GetUserByID(ctx,

		userID,
	)
	if err != nil {
		return "", 0, 0, fmt.Errorf("looking up user: %w", err)
	}

	return user.Username, points, credits, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string {
	return &s
}
