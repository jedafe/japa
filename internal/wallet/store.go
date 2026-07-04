package wallet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for wallet operations. The Service
// layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// Wallet Ledger
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) insertLedgerEntry(ctx context.Context, p gen.InsertLedgerEntryParams) (gen.WalletLedger, error) {
	row, err := s.write.InsertLedgerEntry(ctx, p)
	if err != nil {
		return gen.WalletLedger{}, fmt.Errorf("inserting ledger entry: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listLedgerEntriesByUser(ctx context.Context, tenantID, userID pgtype.UUID, ledgerType *string, limit, offset int32) ([]gen.WalletLedger, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListLedgerEntriesByUser(ctx, gen.ListLedgerEntriesByUserParams{
		TenantID:   tenantID,
		UserID:     userID,
		LedgerType: derefString(ledgerType),
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing ledger entries by user: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) getLatestBalance(ctx context.Context, tenantID, userID pgtype.UUID, ledgerType string) (int64, error) {
	balance, err := s.read.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		TenantID:   tenantID,
		UserID:     userID,
		LedgerType: ledgerType,
	})
	if err != nil {
		return 0, fmt.Errorf("getting latest balance: %w", err)
	}

	return balance, nil
}

//nolint:unused // reserved for future features
func (s *Store) checkIdempotencyKey(ctx context.Context, idempotencyKey *string) (bool, error) {
	exists, err := s.read.CheckIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		return false, fmt.Errorf("checking idempotency key: %w", err)
	}

	return exists, nil
}

//nolint:unused // reserved for future features
func (s *Store) getLedgerEntryByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.WalletLedger, error) {
	row, err := s.read.GetLedgerEntryByID(ctx, gen.GetLedgerEntryByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.WalletLedger{}, fmt.Errorf("getting ledger entry by ID: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// Activity Point Rules
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) upsertActivityRule(ctx context.Context, p gen.UpsertActivityRuleParams) (gen.ActivityPointRule, error) {
	row, err := s.write.UpsertActivityRule(ctx, p)
	if err != nil {
		return gen.ActivityPointRule{}, fmt.Errorf("upserting activity rule: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getActivityRuleByKey(ctx context.Context, tenantID pgtype.UUID, activityKey string) (gen.ActivityPointRule, error) {
	row, err := s.read.GetActivityRuleByKey(ctx, gen.GetActivityRuleByKeyParams{
		TenantID:    tenantID,
		ActivityKey: activityKey,
	})
	if err != nil {
		return gen.ActivityPointRule{}, fmt.Errorf("getting activity rule by key: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listActivityRulesByTenant(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ActivityPointRule, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListActivityRulesByTenant(ctx, gen.ListActivityRulesByTenantParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing activity rules by tenant: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) toggleActivityRule(ctx context.Context, id, tenantID pgtype.UUID) (gen.ActivityPointRule, error) {
	row, err := s.write.ToggleActivityRule(ctx, gen.ToggleActivityRuleParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.ActivityPointRule{}, fmt.Errorf("toggling activity rule: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteActivityRule(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteActivityRule(ctx, gen.DeleteActivityRuleParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting activity rule: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Activity Events
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) insertActivityEvent(ctx context.Context, p gen.InsertActivityEventParams) (gen.ActivityEvent, error) {
	row, err := s.write.InsertActivityEvent(ctx, p)
	if err != nil {
		return gen.ActivityEvent{}, fmt.Errorf("inserting activity event: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getActivityEventByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ActivityEvent, error) {
	row, err := s.read.GetActivityEventByID(ctx, gen.GetActivityEventByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.ActivityEvent{}, fmt.Errorf("getting activity event by ID: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) markEventProcessed(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.MarkEventProcessed(ctx, gen.MarkEventProcessedParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("marking event processed: %w", err)
	}

	return nil
}

//nolint:unused // reserved for future features
func (s *Store) listUnprocessedEvents(ctx context.Context, limit int32) ([]gen.ActivityEvent, error) {
	if limit <= 0 {
		limit = 100 // Default batch size for processing
	}

	rows, err := s.read.ListUnprocessedEvents(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("listing unprocessed events: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) countUserActivityToday(ctx context.Context, tenantID, userID pgtype.UUID, activityKey string) (int64, error) {
	count, err := s.read.CountUserActivityToday(ctx, gen.CountUserActivityTodayParams{
		TenantID:    tenantID,
		UserID:      userID,
		ActivityKey: activityKey,
	})
	if err != nil {
		return 0, fmt.Errorf("counting user activity today: %w", err)
	}

	return count, nil
}

//nolint:unused // reserved for future features
func (s *Store) countUserActivityThisWeek(ctx context.Context, tenantID, userID pgtype.UUID, activityKey string) (int64, error) {
	count, err := s.read.CountUserActivityThisWeek(ctx, gen.CountUserActivityThisWeekParams{
		TenantID:    tenantID,
		UserID:      userID,
		ActivityKey: activityKey,
	})
	if err != nil {
		return 0, fmt.Errorf("counting user activity this week: %w", err)
	}

	return count, nil
}

// ---------------------------------------------------------------------------
// Wallet Settings
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) getWalletSettings(ctx context.Context, tenantID pgtype.UUID) (gen.WalletSetting, error) {
	row, err := s.read.GetWalletSettings(ctx, tenantID)
	if err != nil {
		return gen.WalletSetting{}, fmt.Errorf("getting wallet settings: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) upsertWalletSettings(ctx context.Context, tenantID pgtype.UUID, pointsToCreditsRate int32) (gen.WalletSetting, error) {
	row, err := s.write.UpsertWalletSettings(ctx, gen.UpsertWalletSettingsParams{
		TenantID:            tenantID,
		PointsToCreditsRate: &pointsToCreditsRate,
	})
	if err != nil {
		return gen.WalletSetting{}, fmt.Errorf("upserting wallet settings: %w", err)
	}

	return row, nil
}

// derefString returns the dereferenced string, or "" if nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
