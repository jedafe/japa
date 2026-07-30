package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/db/gen"
	mw "github.com/jedafe/japa/internal/middleware"
)

// EmitActivityEventInput holds parameters for emitting an activity event.
type EmitActivityEventInput struct {
	ActivityKey string
	Reference   string
	Payload     map[string]interface{}
}

// EmitActivityEvent records an activity event for async processing.
func (s *Service) EmitActivityEvent(ctx context.Context, tenantID, userID pgtype.UUID, input EmitActivityEventInput) (gen.ActivityEvent, error) {
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
		TenantID:    tenantID,
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
func (s *Service) ProcessActivityEvent(ctx context.Context, eventID, tenantID pgtype.UUID) error {
	// Get the event
	event, err := s.store.getActivityEventByID(ctx, eventID, tenantID)
	if err != nil {
		return fmt.Errorf("getting activity event: %w", err)
	}

	// Get the activity rule
	rule, err := s.store.getActivityRuleByKey(ctx, tenantID, event.ActivityKey)
	if err != nil {
		// If rule not found, skip processing (rule may have been deleted)
		return nil //nolint:nilerr // intentional: missing rule = skip
	}

	// Check if rule is enabled
	if rule.IsEnabled != nil && !*rule.IsEnabled {
		return nil
	}

	// Check frequency caps (read-only queries outside tx — safe, caps are advisory)
	if rule.CapPerDay != nil {
		count, err := s.store.countUserActivityToday(ctx, tenantID, event.UserID, event.ActivityKey)
		if err != nil {
			return fmt.Errorf("checking daily cap: %w", err)
		}

		if count >= int64(*rule.CapPerDay) {
			return nil // Cap reached, skip
		}
	}

	if rule.CapPerWeek != nil {
		count, err := s.store.countUserActivityThisWeek(ctx, tenantID, event.UserID, event.ActivityKey)
		if err != nil {
			return fmt.Errorf("checking weekly cap: %w", err)
		}

		if count >= int64(*rule.CapPerWeek) {
			return nil // Cap reached, skip
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
	lockKey := "wallet:" + tenantID.String() + ":" + event.UserID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// Get current balance within the transaction.
	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		TenantID:   tenantID,
		UserID:     event.UserID,
		LedgerType: LedgerTypePoints,
	})
	if err != nil {
		currentBalance = 0
	}

	// Calculate new balance
	newBalance := currentBalance + int64(rule.PointValue)

	// Create idempotency key from event
	idempotencyKey := fmt.Sprintf("activity:%s:%s", event.ActivityKey, event.Reference)

	// Insert ledger entry
	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TenantID:         tenantID,
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
		return fmt.Errorf("inserting ledger entry: %w", err)
	}

	// Mark event as processed within the transaction.
	if err := qtx.MarkEventProcessed(ctx, gen.MarkEventProcessedParams{
		ID:       eventID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("marking event processed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// GetBalance returns the current balance for a user (points and/or credits).
func (s *Service) GetBalance(ctx context.Context, tenantID, userID pgtype.UUID) (points, credits int64, err error) {
	points, err = s.store.getLatestBalance(ctx, tenantID, userID, LedgerTypePoints)
	if err != nil {
		// If no balance yet, return 0
		points = 0
	}

	credits, err = s.store.getLatestBalance(ctx, tenantID, userID, LedgerTypeCredits)
	if err != nil {
		// If no balance yet, return 0
		credits = 0
	}

	return points, credits, nil
}

// ConvertPointsToCreditsInput holds parameters for converting points to credits.
type ConvertPointsToCreditsInput struct {
	PointsToConvert int64
}

// ConvertPointsToCredits converts points to credits at the configured rate.
// The rate is resolved in priority order: (1) per-tenant wallet_settings row,
// (2) global system_settings key wallet.points_to_credit_rate, (3) compiled-in
// DefaultPointsToCreditsRate (AUDIT-BE2-SETTING-007). The minimum convertible
// amount is enforced from wallet.min_convert_points (AUDIT-BE2-SETTING-008).
func (s *Service) ConvertPointsToCredits(ctx context.Context, tenantID, userID pgtype.UUID, input ConvertPointsToCreditsInput) (pointsBalance, creditsBalance int64, err error) {
	if input.PointsToConvert <= 0 {
		return 0, 0, fmt.Errorf("%w: points to convert must be positive", errpkg.ErrInvalid)
	}

	// AUDIT-BE2-SETTING-008: enforce minimum convertible points from settings.
	minConvert := int64(DefaultMinConvertPoints)
	if s.settings != nil {
		minConvert = int64(s.settings.GetIntOr(ctx, tenantID, "wallet.min_convert_points", DefaultMinConvertPoints))
	}
	if input.PointsToConvert < minConvert {
		return 0, 0, fmt.Errorf("%w: minimum %d points required for conversion", errpkg.ErrInvalid, minConvert)
	}

	// Get wallet settings for conversion rate
	settings, err := s.store.getWalletSettings(ctx, tenantID)
	rate := int32(DefaultPointsToCreditsRate)
	if err == nil && settings.PointsToCreditsRate != nil && *settings.PointsToCreditsRate > 0 {
		// (1) per-tenant wallet_settings row takes precedence.
		rate = *settings.PointsToCreditsRate
	} else if s.settings != nil {
		// (2) fall back to global system_settings (AUDIT-BE2-SETTING-007).
		// The catalog key is a float (0.01 means 1 point = 0.01 credits), but
		// the wallet ledger uses an integer "points per credit" rate, so we
		// invert: rate = round(1 / points_to_credit_rate). Guard against zero.
		if perCredit := s.settings.GetFloatOr(ctx, tenantID, "wallet.points_to_credit_rate", 0); perCredit > 0 {
			if computed := int32(1.0 / perCredit); computed > 0 {
				rate = computed
			}
		}
	}
	settings = gen.WalletSetting{PointsToCreditsRate: &rate}

	// Begin transaction to atomically check balance and insert ledger entries.
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	// Serialize concurrent balance mutations for this user. The Wallet ledger is
	// append-only (no mutable balance row to lock with FOR UPDATE), so a plain
	// SELECT of the latest balance does not stop two concurrent conversions from
	// both passing the sufficient-funds check and double-spending. A
	// transaction-scoped advisory lock keyed on the user serializes them; it is
	// released automatically on commit/rollback. See AUDIT-BE-005.
	lockKey := "wallet:" + tenantID.String() + ":" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return 0, 0, fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// Get current points balance within the transaction.
	currentPoints, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		TenantID:   tenantID,
		UserID:     userID,
		LedgerType: LedgerTypePoints,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("getting points balance: %w", err)
	}

	// Check sufficient balance
	if currentPoints < input.PointsToConvert {
		return 0, 0, fmt.Errorf("%w: insufficient points balance", errpkg.ErrInsufficientBalance)
	}

	// Calculate credits to add (integer division)
	creditsToAdd := input.PointsToConvert / int64(*settings.PointsToCreditsRate)
	if creditsToAdd <= 0 {
		return 0, 0, fmt.Errorf("%w: conversion rate too high for amount", errpkg.ErrInvalid)
	}

	// New points balance
	newPointsBalance := currentPoints - input.PointsToConvert

	// Get current credits balance within the transaction.
	currentCredits, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		TenantID:   tenantID,
		UserID:     userID,
		LedgerType: LedgerTypeCredits,
	})
	if err != nil {
		currentCredits = 0
	}

	newCreditsBalance := currentCredits + creditsToAdd

	// Create idempotency keys. Prefer the request's Idempotency-Key header
	// (propagated via context by the idempotency middleware) so retries of the
	// same logical request dedupe at the ledger level too. When no header is
	// present, generate a unique UUID per request — deriving the key from
	// (userID, points) would falsely collide across distinct same-amount
	// conversions, permanently blocking a user from converting the same amount
	// twice (Rule 9 / §13).
	reqKey := mw.IdempotencyKeyFromContext(ctx)
	pointsKey := reqKey
	if pointsKey == "" {
		pointsKey = "convert:" + uuid.NewString()
	} else {
		pointsKey = "convert:" + pointsKey + ":points"
	}
	creditsKey := reqKey
	if creditsKey == "" {
		creditsKey = "convert_credits:" + uuid.NewString()
	} else {
		creditsKey = "convert_credits:" + creditsKey + ":credits"
	}

	// Insert points debit entry
	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TenantID:         tenantID,
		UserID:           userID,
		LedgerType:       LedgerTypePoints,
		Operation:        OperationConvert,
		Amount:           -input.PointsToConvert,
		ResultingBalance: newPointsBalance,
		Source:           strPtr("wallet_conversion"),
		Reference:        strPtr(fmt.Sprintf("Converted %d points to %d credits", input.PointsToConvert, creditsToAdd)),
		IdempotencyKey:   &pointsKey,
		Reason:           nil,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("inserting points debit entry: %w", err)
	}

	// Insert credits credit entry
	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TenantID:         tenantID,
		UserID:           userID,
		LedgerType:       LedgerTypeCredits,
		Operation:        OperationConvert,
		Amount:           creditsToAdd,
		ResultingBalance: newCreditsBalance,
		Source:           strPtr("wallet_conversion"),
		Reference:        strPtr(fmt.Sprintf("Converted %d points to %d credits", input.PointsToConvert, creditsToAdd)),
		IdempotencyKey:   &creditsKey,
		Reason:           nil,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("inserting credits credit entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("committing transaction: %w", err)
	}

	return newPointsBalance, newCreditsBalance, nil
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
func (s *Service) SpendCredits(ctx context.Context, tenantID, userID pgtype.UUID, input SpendCreditsInput) (int64, error) {
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
	lockKey := "wallet:" + tenantID.String() + ":" + userID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return 0, fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// Get current credits balance within the transaction.
	currentCredits, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		TenantID:   tenantID,
		UserID:     userID,
		LedgerType: LedgerTypeCredits,
	})
	if err != nil {
		return 0, fmt.Errorf("getting credits balance: %w", err)
	}

	// Check sufficient balance
	if currentCredits < input.Amount {
		return 0, errors.New("insufficient credits balance")
	}

	// New balance
	newBalance := currentCredits - input.Amount

	// Create idempotency key
	idempotencyKey := fmt.Sprintf("spend:%x:%s:%s", userID.Bytes, input.Source, input.Reference)

	// Insert ledger entry
	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TenantID:         tenantID,
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
func (s *Service) AdminAdjustBalance(ctx context.Context, tenantID, actorUserID pgtype.UUID, input AdminAdjustInput) (int64, error) {
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
	lockKey := "wallet:" + tenantID.String() + ":" + targetUserID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return 0, fmt.Errorf("acquiring wallet lock: %w", err)
	}

	// Get current balance within the transaction.
	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		TenantID:   tenantID,
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

	// Create idempotency key
	idempotencyKey := fmt.Sprintf("admin_adjust:%x:%s:%d", targetUserID.Bytes, input.Reason, input.Amount)

	// Insert ledger entry
	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TenantID:         tenantID,
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

// GetWalletSettings returns wallet settings for a tenant.
// If no per-tenant wallet_settings row exists yet, the conversion rate is
// resolved from the global system_settings key wallet.points_to_credit_rate
// (a float: credits-per-point, e.g. 0.001 = 1000 points per credit) and
// inverted to the integer "points per credit" rate the ledger uses, falling
// back to DefaultPointsToCreditsRate when unset. This MUST stay consistent
// with the resolution in ConvertPointsToCredits so the displayed rate matches
// the rate actually enforced at conversion time (AUDIT-BE2-SETTING-007).
func (s *Service) GetWalletSettings(ctx context.Context, tenantID pgtype.UUID) (gen.WalletSetting, error) {
	settings, err := s.store.getWalletSettings(ctx, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No settings row yet — resolve rate from system_settings, then defaults.
			rate := int32(DefaultPointsToCreditsRate)
			if s.settings != nil {
				if perCredit := s.settings.GetFloatOr(ctx, tenantID, "wallet.points_to_credit_rate", 0); perCredit > 0 {
					if computed := int32(1.0 / perCredit); computed > 0 {
						rate = computed
					}
				}
			}
			return gen.WalletSetting{
				TenantID:            tenantID,
				PointsToCreditsRate: &rate,
				DailyEarningCap:     1000,
				EnableEarning:       true,
			}, nil
		}
		return gen.WalletSetting{}, err
	}
	return settings, nil
}

// GetWalletSettingsWithMinConvert returns wallet settings plus the effective
// minimum points required for a single conversion. The minimum is read from
// system_settings key wallet.min_convert_points, falling back to
// DefaultMinConvertPoints when unset.
func (s *Service) GetWalletSettingsWithMinConvert(ctx context.Context, tenantID pgtype.UUID) (gen.WalletSetting, int64, error) {
	settings, err := s.GetWalletSettings(ctx, tenantID)
	if err != nil {
		return gen.WalletSetting{}, 0, err
	}
	minConvert := int64(DefaultMinConvertPoints)
	if s.settings != nil {
		minConvert = int64(s.settings.GetIntOr(ctx, tenantID, "wallet.min_convert_points", DefaultMinConvertPoints))
	}
	return settings, minConvert, nil
}

// UpdateWalletSettingsInput holds parameters for updating wallet settings.
type UpdateWalletSettingsInput struct {
	PointsToCreditsRate int32
	DailyEarningCap     int32
	EnableEarning       bool
}

// UpdateWalletSettings updates wallet settings for a tenant.
func (s *Service) UpdateWalletSettings(ctx context.Context, tenantID pgtype.UUID, input UpdateWalletSettingsInput) (gen.WalletSetting, error) {
	if input.PointsToCreditsRate <= 0 {
		return gen.WalletSetting{}, errors.New("points_to_credits_rate must be positive")
	}

	return s.store.upsertWalletSettings(ctx, tenantID, input.PointsToCreditsRate, input.DailyEarningCap, input.EnableEarning)
}

// ListActivityRules returns activity rules for a tenant.
func (s *Service) ListActivityRules(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ActivityPointRule, error) {
	return s.store.listActivityRulesByTenant(ctx, tenantID, limit, offset)
}

// CreateOrUpdateActivityRuleInput holds parameters for creating/updating an activity rule.
type CreateOrUpdateActivityRuleInput struct {
	ActivityKey string
	PointValue  int32
	IsEnabled   bool
	CapPerDay   *int32
	CapPerWeek  *int32
	Description *string
}

// CreateOrUpdateActivityRule creates or updates an activity rule.
func (s *Service) CreateOrUpdateActivityRule(ctx context.Context, tenantID pgtype.UUID, input CreateOrUpdateActivityRuleInput) (gen.ActivityPointRule, error) {
	if input.ActivityKey == "" {
		return gen.ActivityPointRule{}, errors.New("activity_key is required")
	}

	return s.store.upsertActivityRule(ctx, gen.UpsertActivityRuleParams{
		TenantID:    tenantID,
		ActivityKey: input.ActivityKey,
		PointValue:  input.PointValue,
		IsEnabled:   &input.IsEnabled,
		CapPerDay:   input.CapPerDay,
		CapPerWeek:  input.CapPerWeek,
		Description: input.Description,
	})
}

// ToggleActivityRule toggles an activity rule's enabled state.
func (s *Service) ToggleActivityRule(ctx context.Context, id, tenantID pgtype.UUID) (gen.ActivityPointRule, error) {
	return s.store.toggleActivityRule(ctx, id, tenantID)
}

// DeleteActivityRule deletes an activity rule.
func (s *Service) DeleteActivityRule(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteActivityRule(ctx, id, tenantID)
}

// ListLedgerEntries returns ledger entries for a user.
func (s *Service) ListLedgerEntries(ctx context.Context, tenantID, userID pgtype.UUID, ledgerType *string, limit, offset int32) ([]gen.WalletLedger, error) {
	return s.store.listLedgerEntriesByUser(ctx, tenantID, userID, ledgerType, limit, offset)
}

// GetLedgerEntry returns a ledger entry by ID.
func (s *Service) GetLedgerEntry(ctx context.Context, id, tenantID pgtype.UUID) (gen.WalletLedger, error) {
	return s.store.getLedgerEntryByID(ctx, id, tenantID)
}

// GetUserBalance returns the balance for any user (admin operation).
func (s *Service) GetUserBalance(ctx context.Context, tenantID, userID pgtype.UUID) (points, credits int64, err error) {
	return s.GetBalance(ctx, tenantID, userID)
}

// GetUserBalanceWithInfo returns balance plus basic user info for admin display.
func (s *Service) GetUserBalanceWithInfo(ctx context.Context, tenantID, userID pgtype.UUID) (username string, points, credits int64, err error) {
	points, credits, err = s.GetBalance(ctx, tenantID, userID)
	if err != nil {
		return "", 0, 0, err
	}

	user, err := s.store.read.GetUserByID(ctx, gen.GetUserByIDParams{
		TenantID: tenantID,
		UserID:   userID,
	})
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
