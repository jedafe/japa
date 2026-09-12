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
	"errors"
	"fmt"
	"strings"
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
	return s.queries.GetRemittanceProviderByID(ctx,

		id,
	)
}

// MapSourcesByRecordIDs returns a map of source attribution keyed by intel record ID.
func (s *Store) MapSourcesByRecordIDs(ctx context.Context, recordIDs []pgtype.UUID) (map[string]gen.ListSourcesByRecordIDsRow, error) {
	rows, err := s.queries.ListSourcesByRecordIDs(ctx, recordIDs)
	if err != nil {
		return nil, fmt.Errorf("remittance: map sources by record ids: %w", err)
	}

	m := make(map[string]gen.ListSourcesByRecordIDsRow, len(rows))
	for _, r := range rows {
		m[r.RecordID.String()] = r
	}

	return m, nil
}

// DeleteProvider deletes a remittance provider by ID and tenant. Deletion is
// blocked while the provider still has remittance_rates rows so that historical
// rate data is not accidentally destroyed (the FK cascades on delete).
func (s *Store) DeleteProvider(ctx context.Context, tenantID, id pgtype.UUID) error {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var rateCount int64

	err = tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM remittance_rates
		WHERE tenant_id = $1 AND provider_id = $2
	`, tenantID, id).Scan(&rateCount)
	if err != nil {
		return fmt.Errorf("count dependent rates: %w", err)
	}

	if rateCount > 0 {
		return fmt.Errorf("provider has %d dependent rate(s)", rateCount)
	}

	cmd, err := tx.Exec(ctx, `
		DELETE FROM remittance_providers
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return errors.New("not found")
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

// ListProviders lists providers for a tenant, optionally filtering to active only.
func (s *Store) ListProviders(ctx context.Context, tenantID pgtype.UUID, activeOnly bool) ([]gen.RemittanceProvider, error) {
	return s.queries.ListRemittanceProviders(ctx,

		activeOnly,
	)
}

// ListProvidersPaginated returns a page of providers and the total count.
func (s *Store) ListProvidersPaginated(ctx context.Context, tenantID pgtype.UUID, activeOnly bool, limit, offset int32) ([]gen.RemittanceProvider, int64, error) {
	rows, err := s.queries.ListRemittanceProvidersPaginated(ctx, gen.ListRemittanceProvidersPaginatedParams{

		ActiveOnly: activeOnly,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("remittance: list providers paginated: %w", err)
	}

	total, err := s.queries.CountRemittanceProviders(ctx,

		activeOnly,
	)
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

		SendCurrency:    sendCurrency,
		ReceiveCurrency: receiveCurrency,
	})
}

// GetHistory returns daily best-rate aggregates for a corridor over a date range.
func (s *Store) GetHistory(ctx context.Context, tenantID pgtype.UUID, sendCurrency, receiveCurrency string, fromDate time.Time) ([]gen.GetRateHistoryRow, error) {
	return s.queries.GetRateHistory(ctx, gen.GetRateHistoryParams{

		SendCurrency:    sendCurrency,
		ReceiveCurrency: receiveCurrency,
		FromDate:        pgtype.Timestamptz{Time: fromDate, Valid: true},
	})
}

// GetCorridors returns the distinct send/receive currency pairs with rate data.
func (s *Store) GetCorridors(ctx context.Context, tenantID pgtype.UUID) ([]gen.GetCorridorsRow, error) {
	return s.queries.GetCorridors(ctx)
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

		ID:     id,
		UserID: userID,
	})
}

// GetAlertByID fetches a single alert owned by the given user.
func (s *Store) GetAlertByID(ctx context.Context, tenantID, id, userID pgtype.UUID) (gen.RemittanceRateAlert, error) {
	return s.queries.GetRateAlertByID(ctx, gen.GetRateAlertByIDParams{

		ID:     id,
		UserID: userID,
	})
}

// ListUserAlerts lists a user's rate alerts, optionally filtering to active only.
func (s *Store) ListUserAlerts(ctx context.Context, tenantID, userID pgtype.UUID, activeOnly bool) ([]gen.RemittanceRateAlert, error) {
	return s.queries.ListUserRateAlerts(ctx, gen.ListUserRateAlertsParams{

		UserID:     userID,
		ActiveOnly: activeOnly,
	})
}

// ListActiveAlertsForCorridor returns active alerts for a corridor that are
// eligible for notification (last_triggered_at is null or > 24h ago).
func (s *Store) ListActiveAlertsForCorridor(ctx context.Context, tenantID pgtype.UUID, sendCurrency, receiveCurrency string) ([]gen.RemittanceRateAlert, error) {
	return s.queries.ListActiveAlertsForCorridor(ctx, gen.ListActiveAlertsForCorridorParams{

		SendCurrency:    sendCurrency,
		ReceiveCurrency: receiveCurrency,
	})
}

// ListAllActiveAlertsForTenant returns all active alerts for a tenant that are
// eligible for notification (last_triggered_at is null or > 24h ago). Used by
// the remittance:check_alerts task to evaluate every corridor in one pass.
func (s *Store) ListAllActiveAlertsForTenant(ctx context.Context, tenantID pgtype.UUID) ([]gen.RemittanceRateAlert, error) {
	return s.queries.ListAllActiveAlertsForTenant(ctx)
}

// MarkAlertTriggered sets last_triggered_at to now() for the given alert.
func (s *Store) MarkAlertTriggered(ctx context.Context, id pgtype.UUID) error {
	return s.queries.MarkAlertTriggered(ctx, id)
}

// PruneOldRates deletes rate rows older than the cutoff, keeping one snapshot
// per provider per corridor per day for older data (daily downsampling).
func (s *Store) PruneOldRates(ctx context.Context, tenantID pgtype.UUID, cutoffDate time.Time) error {
	return s.queries.PruneOldRates(ctx,

		pgtype.Timestamptz{Time: cutoffDate, Valid: true},
	)
}

// CreateRate inserts a new remittance rate snapshot and returns the row.
func (s *Store) CreateRate(ctx context.Context, arg gen.InsertRemittanceRateParams) (gen.RemittanceRate, error) {
	return s.queries.InsertRemittanceRate(ctx, arg)
}

// UpdateRateStoreInput is the data-access input for updating a remittance rate.
// Pointer/nil fields mean "leave unchanged"; non-pointer pgtype.UUID is checked
// with Valid. The float fields are converted to pgtype.Numeric before SQL.
type UpdateRateStoreInput struct {
	ProviderID      pgtype.UUID `json:"provider_id,omitempty"`
	SendCurrency    *string     `json:"send_currency,omitempty"`
	ReceiveCurrency *string     `json:"receive_currency,omitempty"`
	Rate            *float64    `json:"rate,omitempty"`
	FeeFlat         *float64    `json:"fee_flat,omitempty"`
	FeePct          *float64    `json:"fee_pct,omitempty"`
	TransferTime    *string     `json:"transfer_time,omitempty"`
	SourceURL       *string     `json:"source_url,omitempty"`
}

// GetRateByID returns a single remittance rate by id and tenant.
func (s *Store) GetRateByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.RemittanceRate, error) {
	const q = `
SELECT id, tenant_id, provider_id, send_currency, receive_currency, rate, fee_flat, fee_pct,
       transfer_time, fetched_at, is_manual, source_url, intel_record_id, confidence_overall,
       verification_status, freshness_score, override_source, editorial_overrides
FROM remittance_rates
WHERE id = $1 AND tenant_id = $2
`

	var r gen.RemittanceRate

	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&r.ID, &r.TenantID, &r.ProviderID, &r.SendCurrency, &r.ReceiveCurrency, &r.Rate, &r.FeeFlat, &r.FeePct,
		&r.TransferTime, &r.FetchedAt, &r.IsManual, &r.SourceUrl, &r.IntelRecordID, &r.ConfidenceOverall,
		&r.VerificationStatus, &r.FreshnessScore, &r.OverrideSource, &r.EditorialOverrides,
	)
	if err != nil {
		return gen.RemittanceRate{}, fmt.Errorf("remittance: get rate by id: %w", err)
	}

	return r, nil
}

// UpdateRate updates a remittance_rate row by id and tenant, returning the row.
func (s *Store) UpdateRate(ctx context.Context, tenantID, id pgtype.UUID, in UpdateRateStoreInput) (gen.RemittanceRate, error) {
	existing, err := s.GetRateByID(ctx, tenantID, id)
	if err != nil {
		return gen.RemittanceRate{}, fmt.Errorf("remittance: update rate: %w", err)
	}

	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	providerID := existing.ProviderID
	if in.ProviderID.Valid {
		providerID = in.ProviderID
	}

	sets = append(sets, fmt.Sprintf("provider_id = $%d", argIdx))
	args = append(args, providerID)
	argIdx++

	sendCurrency := existing.SendCurrency
	if in.SendCurrency != nil {
		sendCurrency = *in.SendCurrency
	}

	sets = append(sets, fmt.Sprintf("send_currency = $%d", argIdx))
	args = append(args, sendCurrency)
	argIdx++

	receiveCurrency := existing.ReceiveCurrency
	if in.ReceiveCurrency != nil {
		receiveCurrency = *in.ReceiveCurrency
	}

	sets = append(sets, fmt.Sprintf("receive_currency = $%d", argIdx))
	args = append(args, receiveCurrency)
	argIdx++

	if in.Rate != nil {
		sets = append(sets, fmt.Sprintf("rate = $%d", argIdx))
		args = append(args, numericFromFloat(*in.Rate))
		argIdx++
	}

	if in.FeeFlat != nil {
		sets = append(sets, fmt.Sprintf("fee_flat = $%d", argIdx))
		args = append(args, numericFromFloat(*in.FeeFlat))
		argIdx++
	}

	if in.FeePct != nil {
		sets = append(sets, fmt.Sprintf("fee_pct = $%d", argIdx))
		args = append(args, numericFromFloat(*in.FeePct))
		argIdx++
	}

	transferTime := existing.TransferTime
	if in.TransferTime != nil {
		transferTime = in.TransferTime
	}

	sets = append(sets, fmt.Sprintf("transfer_time = $%d", argIdx))
	args = append(args, transferTime)
	argIdx++

	sourceURL := existing.SourceUrl
	if in.SourceURL != nil {
		sourceURL = in.SourceURL
	}

	sets = append(sets, fmt.Sprintf("source_url = $%d", argIdx))
	args = append(args, sourceURL)
	argIdx++

	sets = append(sets, "fetched_at = now()")

	q := fmt.Sprintf(
		"UPDATE remittance_rates SET %s WHERE id = $%d AND tenant_id = $%d RETURNING id, tenant_id, provider_id, send_currency, receive_currency, rate, fee_flat, fee_pct, transfer_time, fetched_at, is_manual, source_url, intel_record_id, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides",
		strings.Join(sets, ", "),
		argIdx,
		argIdx+1,
	)

	args = append(args, id, tenantID)

	var r gen.RemittanceRate

	err = s.pool.Writer.QueryRow(ctx, q, args...).Scan(
		&r.ID, &r.TenantID, &r.ProviderID, &r.SendCurrency, &r.ReceiveCurrency, &r.Rate, &r.FeeFlat, &r.FeePct,
		&r.TransferTime, &r.FetchedAt, &r.IsManual, &r.SourceUrl, &r.IntelRecordID, &r.ConfidenceOverall,
		&r.VerificationStatus, &r.FreshnessScore, &r.OverrideSource, &r.EditorialOverrides,
	)
	if err != nil {
		return gen.RemittanceRate{}, fmt.Errorf("remittance: update rate: %w", err)
	}

	return r, nil
}

// DeleteRate deletes a remittance_rate row by id and tenant.
func (s *Store) DeleteRate(ctx context.Context, tenantID, id pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `DELETE FROM remittance_rates WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("remittance: delete rate: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return errors.New("remittance: delete rate: no rows affected")
	}

	return nil
}
