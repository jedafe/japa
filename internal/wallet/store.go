package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
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

// getActivityRuleByID returns a single activity rule by its primary key.
func (s *Store) getActivityRuleByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ActivityPointRule, error) {
	row, err := s.read.GetActivityRuleByID(ctx, gen.GetActivityRuleByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.ActivityPointRule{}, fmt.Errorf("getting activity rule by ID: %w", err)
	}
	return row, nil
}

// updateActivityRuleByID updates an activity rule by its primary key.
func (s *Store) updateActivityRuleByID(ctx context.Context, p gen.UpdateActivityRuleByIDParams) (gen.ActivityPointRule, error) {
	row, err := s.write.UpdateActivityRuleByID(ctx, p)
	if err != nil {
		return gen.ActivityPointRule{}, fmt.Errorf("updating activity rule by ID: %w", err)
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
func (s *Store) upsertWalletSettings(ctx context.Context, tenantID pgtype.UUID, pointsToCreditsRate int32, dailyEarningCap int32, enableEarning bool) (gen.WalletSetting, error) {
	row, err := s.write.UpsertWalletSettings(ctx, gen.UpsertWalletSettingsParams{
		TenantID:            tenantID,
		PointsToCreditsRate: &pointsToCreditsRate,
		DailyEarningCap:     dailyEarningCap,
		EnableEarning:       enableEarning,
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

// ---------------------------------------------------------------------------
// Credit Currency System (Section 58, ADR-065)
// ---------------------------------------------------------------------------

// listActiveCurrencies returns all active currencies ordered by position.
func (s *Store) listActiveCurrencies(ctx context.Context) ([]gen.Currency, error) {
	rows, err := s.read.ListActiveCurrencies(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing active currencies: %w", err)
	}
	return rows, nil
}

// listAllCurrencies returns all currencies (including inactive) ordered by position.
func (s *Store) listAllCurrencies(ctx context.Context) ([]gen.Currency, error) {
	rows, err := s.read.ListAllCurrencies(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing all currencies: %w", err)
	}
	return rows, nil
}

// getCurrencyByCode returns a single currency by its ISO 4217 code.
func (s *Store) getCurrencyByCode(ctx context.Context, code string) (gen.Currency, error) {
	row, err := s.read.GetCurrencyByCode(ctx, code)
	if err != nil {
		return gen.Currency{}, fmt.Errorf("getting currency %s: %w", code, err)
	}
	return row, nil
}

// createCurrency inserts a new currency.
func (s *Store) createCurrency(ctx context.Context, p gen.CreateCurrencyParams) (gen.Currency, error) {
	row, err := s.write.CreateCurrency(ctx, p)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.Currency{}, fmt.Errorf("creating currency: %w: %s", errpkg.ErrDuplicate, p.Code)
		}
		return gen.Currency{}, fmt.Errorf("creating currency: %w", err)
	}
	return row, nil
}

// updateCurrency updates an existing currency.
func (s *Store) updateCurrency(ctx context.Context, p gen.UpdateCurrencyParams) (gen.Currency, error) {
	row, err := s.write.UpdateCurrency(ctx, p)
	if err != nil {
		return gen.Currency{}, fmt.Errorf("updating currency: %w", err)
	}
	return row, nil
}

// deactivateCurrency sets is_active=false on a currency (soft delete).
func (s *Store) deactivateCurrency(ctx context.Context, code string) error {
	if err := s.write.DeactivateCurrency(ctx, code); err != nil {
		return fmt.Errorf("deactivating currency %s: %w", code, err)
	}
	return nil
}

// countSpacesByCurrency returns the number of country spaces using a currency.
func (s *Store) countSpacesByCurrency(ctx context.Context, code string) (int32, error) {
	count, err := s.read.CountSpacesByCurrency(ctx, &code)
	if err != nil {
		return 0, fmt.Errorf("counting spaces by currency: %w", err)
	}
	return count, nil
}

// getCurrencyByCountryCode resolves a currency from a country's ISO code
// via the spaces.currency_code linkage.
func (s *Store) getCurrencyByCountryCode(ctx context.Context, geoCode string, tenantID pgtype.UUID) (string, error) {
	row, err := s.read.GetCurrencyByCountryCode(ctx, gen.GetCurrencyByCountryCodeParams{
		GeoCode:  &geoCode,
		TenantID: tenantID,
	})
	if err != nil {
		return "", fmt.Errorf("resolving currency for country %s: %w", geoCode, err)
	}
	return row.Code, nil
}

// ---------------------------------------------------------------------------
// Credit Packs
// ---------------------------------------------------------------------------

// listActiveCreditPacks returns all active credit packs for a tenant.
func (s *Store) listActiveCreditPacks(ctx context.Context, tenantID pgtype.UUID) ([]gen.CreditPack, error) {
	rows, err := s.read.ListActiveCreditPacks(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing active credit packs: %w", err)
	}
	return rows, nil
}

// listAllCreditPacks returns all credit packs (including inactive) for a tenant.
func (s *Store) listAllCreditPacks(ctx context.Context, tenantID pgtype.UUID) ([]gen.CreditPack, error) {
	rows, err := s.read.ListAllCreditPacks(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing all credit packs: %w", err)
	}
	return rows, nil
}

// getCreditPackByID returns a single credit pack by ID.
func (s *Store) getCreditPackByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.CreditPack, error) {
	row, err := s.read.GetCreditPackByID(ctx, gen.GetCreditPackByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.CreditPack{}, fmt.Errorf("getting credit pack: %w", err)
	}
	return row, nil
}

// createCreditPack inserts a new credit pack.
func (s *Store) createCreditPack(ctx context.Context, p gen.CreateCreditPackParams) (gen.CreditPack, error) {
	row, err := s.write.CreateCreditPack(ctx, p)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.CreditPack{}, fmt.Errorf("creating credit pack: %w: %s", errpkg.ErrDuplicate, p.Name)
		}
		return gen.CreditPack{}, fmt.Errorf("creating credit pack: %w", err)
	}
	return row, nil
}

// updateCreditPack updates an existing credit pack.
func (s *Store) updateCreditPack(ctx context.Context, p gen.UpdateCreditPackParams) (gen.CreditPack, error) {
	row, err := s.write.UpdateCreditPack(ctx, p)
	if err != nil {
		return gen.CreditPack{}, fmt.Errorf("updating credit pack: %w", err)
	}
	return row, nil
}

// deactivateCreditPack sets is_active=false on a credit pack (soft delete).
func (s *Store) deactivateCreditPack(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeactivateCreditPack(ctx, gen.DeactivateCreditPackParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deactivating credit pack: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// User Preferred Currency
// ---------------------------------------------------------------------------

// getUserPreferredCurrency returns the user's explicit currency preference.
func (s *Store) getUserPreferredCurrency(ctx context.Context, userID pgtype.UUID) (string, error) {
	row, err := s.read.GetUserPreferredCurrency(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("getting user preferred currency: %w", err)
	}
	if row == nil {
		return "", nil
	}
	return *row, nil
}

// setUserPreferredCurrency sets or clears the user's currency preference.
func (s *Store) setUserPreferredCurrency(ctx context.Context, userID pgtype.UUID, currencyCode string) error {
	var code *string
	if currencyCode != "" {
		code = &currencyCode
	}
	if err := s.write.SetUserPreferredCurrency(ctx, gen.SetUserPreferredCurrencyParams{
		UserID:            userID,
		PreferredCurrency: code,
	}); err != nil {
		return fmt.Errorf("setting user preferred currency: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Credit Purchase History
// ---------------------------------------------------------------------------

// listCreditPurchases returns credit purchase payment rows for admin viewing.
func (s *Store) listCreditPurchases(ctx context.Context, p gen.ListCreditPurchasesParams) ([]gen.ListCreditPurchasesRow, error) {
	rows, err := s.read.ListCreditPurchases(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("listing credit purchases: %w", err)
	}
	return rows, nil
}
