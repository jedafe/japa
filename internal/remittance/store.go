// Package remittance implements the Remittance Rate Tracker (Section 74,
// ADR-075). It provides live remittance rate comparison across providers,
// historical trends, a send-amount calculator, and user-configurable rate
// alerts. The data model is a hybrid system: live provider APIs where
// available and manual admin curation as fallback.
//
// Ref: japaconnect-engineering/sections/74-remittance-tracker.md
//
//	japaconnect-engineering/adr/ADR-075-remittance-rate-tracker.md
package remittance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for remittance operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a remittance store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// CreateProvider inserts a new remittance provider.
func (s *Store) CreateProvider(ctx context.Context, arg gen.CreateRemittanceProviderParams) (gen.RemittanceProvider, error) {
	return s.queries.CreateRemittanceProvider(ctx, arg)
}

// UpdateProvider updates a remittance provider's mutable fields.
func (s *Store) UpdateProvider(ctx context.Context, arg gen.UpdateRemittanceProviderParams) (gen.RemittanceProvider, error) {
	return s.queries.UpdateRemittanceProvider(ctx, arg)
}

// GetProviderByID fetches a single provider by ID within a tenant.
func (s *Store) GetProviderByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.RemittanceProvider, error) {
	return s.queries.GetRemittanceProviderByID(ctx, gen.GetRemittanceProviderByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
}

// ListProviders lists providers for a tenant, optionally filtering to active only.
func (s *Store) ListProviders(ctx context.Context, tenantID pgtype.UUID, activeOnly bool) ([]gen.RemittanceProvider, error) {
	return s.queries.ListRemittanceProviders(ctx, gen.ListRemittanceProvidersParams{
		TenantID:   tenantID,
		ActiveOnly: activeOnly,
	})
}

// ListProvidersPaginated returns a page of providers and the total count.
func (s *Store) ListProvidersPaginated(ctx context.Context, tenantID pgtype.UUID, activeOnly bool, limit, offset int32) ([]gen.RemittanceProvider, int64, error) {
	rows, err := s.queries.ListRemittanceProvidersPaginated(ctx, gen.ListRemittanceProvidersPaginatedParams{
		TenantID:   tenantID,
		ActiveOnly: activeOnly,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("remittance: list providers paginated: %w", err)
	}

	total, err := s.queries.CountRemittanceProviders(ctx, gen.CountRemittanceProvidersParams{
		TenantID:   tenantID,
		ActiveOnly: activeOnly,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("remittance: count providers: %w", err)
	}

	return rows, total, nil
}

// InsertRate inserts a new rate snapshot row.
func (s *Store) InsertRate(ctx context.Context, arg gen.InsertRemittanceRateParams) (gen.RemittanceRate, error) {
	return s.queries.InsertRemittanceRate(ctx, arg)
}

// GetLatestRates returns the latest rate per active provider for a corridor.
func (s *Store) GetLatestRates(ctx context.Context, tenantID pgtype.UUID, sendCurrency, receiveCurrency string) ([]gen.GetLatestRatesForCorridorRow, error) {
	return s.queries.GetLatestRatesForCorridor(ctx, gen.GetLatestRatesForCorridorParams{
		TenantID:        tenantID,
		SendCurrency:    sendCurrency,
		ReceiveCurrency: receiveCurrency,
	})
}

// GetHistory returns daily best-rate aggregates for a corridor over a date range.
func (s *Store) GetHistory(ctx context.Context, tenantID pgtype.UUID, sendCurrency, receiveCurrency string, fromDate time.Time) ([]gen.GetRateHistoryRow, error) {
	return s.queries.GetRateHistory(ctx, gen.GetRateHistoryParams{
		TenantID:        tenantID,
		SendCurrency:    sendCurrency,
		ReceiveCurrency: receiveCurrency,
		FromDate:        pgtype.Timestamptz{Time: fromDate, Valid: true},
	})
}

// GetCorridors returns the distinct send/receive currency pairs with rate data.
func (s *Store) GetCorridors(ctx context.Context, tenantID pgtype.UUID) ([]gen.GetCorridorsRow, error) {
	return s.queries.GetCorridors(ctx, tenantID)
}

// CreateAlert inserts a new rate alert for a user.
func (s *Store) CreateAlert(ctx context.Context, arg gen.CreateRateAlertParams) (gen.RemittanceRateAlert, error) {
	return s.queries.CreateRateAlert(ctx, arg)
}

// UpdateAlert updates a rate alert's mutable fields.
func (s *Store) UpdateAlert(ctx context.Context, arg gen.UpdateRateAlertParams) (gen.RemittanceRateAlert, error) {
	return s.queries.UpdateRateAlert(ctx, arg)
}

// DeleteAlert deletes a rate alert owned by the given user.
func (s *Store) DeleteAlert(ctx context.Context, tenantID, id, userID pgtype.UUID) error {
	return s.queries.DeleteRateAlert(ctx, gen.DeleteRateAlertParams{
		TenantID: tenantID,
		ID:       id,
		UserID:   userID,
	})
}

// GetAlertByID fetches a single alert owned by the given user.
func (s *Store) GetAlertByID(ctx context.Context, tenantID, id, userID pgtype.UUID) (gen.RemittanceRateAlert, error) {
	return s.queries.GetRateAlertByID(ctx, gen.GetRateAlertByIDParams{
		TenantID: tenantID,
		ID:       id,
		UserID:   userID,
	})
}

// ListUserAlerts lists a user's rate alerts, optionally filtering to active only.
func (s *Store) ListUserAlerts(ctx context.Context, tenantID, userID pgtype.UUID, activeOnly bool) ([]gen.RemittanceRateAlert, error) {
	return s.queries.ListUserRateAlerts(ctx, gen.ListUserRateAlertsParams{
		TenantID:   tenantID,
		UserID:     userID,
		ActiveOnly: activeOnly,
	})
}

// ListActiveAlertsForCorridor returns active alerts for a corridor that are
// eligible for notification (last_triggered_at is null or > 24h ago).
func (s *Store) ListActiveAlertsForCorridor(ctx context.Context, tenantID pgtype.UUID, sendCurrency, receiveCurrency string) ([]gen.RemittanceRateAlert, error) {
	return s.queries.ListActiveAlertsForCorridor(ctx, gen.ListActiveAlertsForCorridorParams{
		TenantID:        tenantID,
		SendCurrency:    sendCurrency,
		ReceiveCurrency: receiveCurrency,
	})
}

// ListAllActiveAlertsForTenant returns all active alerts for a tenant that are
// eligible for notification (last_triggered_at is null or > 24h ago). Used by
// the remittance:check_alerts task to evaluate every corridor in one pass.
func (s *Store) ListAllActiveAlertsForTenant(ctx context.Context, tenantID pgtype.UUID) ([]gen.RemittanceRateAlert, error) {
	return s.queries.ListAllActiveAlertsForTenant(ctx, tenantID)
}

// MarkAlertTriggered sets last_triggered_at to now() for the given alert.
func (s *Store) MarkAlertTriggered(ctx context.Context, id pgtype.UUID) error {
	return s.queries.MarkAlertTriggered(ctx, id)
}

// PruneOldRates deletes rate rows older than the cutoff, keeping one snapshot
// per provider per corridor per day for older data (daily downsampling).
func (s *Store) PruneOldRates(ctx context.Context, tenantID pgtype.UUID, cutoffDate time.Time) error {
	return s.queries.PruneOldRates(ctx, gen.PruneOldRatesParams{
		TenantID:   tenantID,
		CutoffDate: pgtype.Timestamptz{Time: cutoffDate, Valid: true},
	})
}

// CreateRate inserts a new remittance rate snapshot and returns the row.
func (s *Store) CreateRate(ctx context.Context, arg gen.InsertRemittanceRateParams) (gen.RemittanceRate, error) {
	return s.queries.InsertRemittanceRate(ctx, arg)
}
