package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
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

	// Check frequency caps
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

	// Get current balance
	currentBalance, err := s.store.getLatestBalance(ctx, tenantID, event.UserID, LedgerTypePoints)
	if err != nil {
		// If no balance yet, start at 0
		currentBalance = 0
	}

	// Calculate new balance
	newBalance := currentBalance + int64(rule.PointValue)

	// Create idempotency key from event
	idempotencyKey := fmt.Sprintf("activity:%s:%s", event.ActivityKey, event.Reference)

	// Insert ledger entry
	_, err = s.store.insertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
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

	// Mark event as processed
	if err := s.store.markEventProcessed(ctx, eventID, tenantID); err != nil {
		return fmt.Errorf("marking event processed: %w", err)
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
func (s *Service) ConvertPointsToCredits(ctx context.Context, tenantID, userID pgtype.UUID, input ConvertPointsToCreditsInput) (pointsBalance, creditsBalance int64, err error) {
	if input.PointsToConvert <= 0 {
		return 0, 0, errors.New("points to convert must be positive")
	}

	// Get wallet settings for conversion rate
	settings, err := s.store.getWalletSettings(ctx, tenantID)
	if err != nil {
		// Use default rate if settings not configured
		defaultRate := int32(DefaultPointsToCreditsRate)
		settings = gen.WalletSetting{
			PointsToCreditsRate: &defaultRate,
		}
	}

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
		return 0, 0, errors.New("insufficient points balance")
	}

	// Calculate credits to add (integer division)
	creditsToAdd := input.PointsToConvert / int64(*settings.PointsToCreditsRate)
	if creditsToAdd <= 0 {
		return 0, 0, errors.New("conversion rate too high for amount")
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

	// Create idempotency key
	idempotencyKey := fmt.Sprintf("convert:%x:%d", userID.Bytes, input.PointsToConvert)

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
		IdempotencyKey:   &idempotencyKey,
		Reason:           nil,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("inserting points debit entry: %w", err)
	}

	// Insert credits credit entry
	creditsIdempotencyKey := fmt.Sprintf("convert_credits:%x:%d", userID.Bytes, input.PointsToConvert)

	_, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TenantID:         tenantID,
		UserID:           userID,
		LedgerType:       LedgerTypeCredits,
		Operation:        OperationConvert,
		Amount:           creditsToAdd,
		ResultingBalance: newCreditsBalance,
		Source:           strPtr("wallet_conversion"),
		Reference:        strPtr(fmt.Sprintf("Converted %d points to %d credits", input.PointsToConvert, creditsToAdd)),
		IdempotencyKey:   &creditsIdempotencyKey,
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
func (s *Service) GetWalletSettings(ctx context.Context, tenantID pgtype.UUID) (gen.WalletSetting, error) {
	return s.store.getWalletSettings(ctx, tenantID)
}

// UpdateWalletSettingsInput holds parameters for updating wallet settings.
type UpdateWalletSettingsInput struct {
	PointsToCreditsRate int32
}

// UpdateWalletSettings updates wallet settings for a tenant.
func (s *Service) UpdateWalletSettings(ctx context.Context, tenantID pgtype.UUID, input UpdateWalletSettingsInput) (gen.WalletSetting, error) {
	if input.PointsToCreditsRate <= 0 {
		return gen.WalletSetting{}, errors.New("points_to_credits_rate must be positive")
	}

	return s.store.upsertWalletSettings(ctx, tenantID, input.PointsToCreditsRate)
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string {
	return &s
}
