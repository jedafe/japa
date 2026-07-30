package analytics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides database operations for the analytics subsystem.
// All queries use sqlc-generated Queries against the Writer pool for
// writes and Reader pool for reads.
type Store struct {
	queries *gen.Queries
}

// NewStore creates a Store backed by the Writer pool.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Writer),
	}
}

// NewStoreWithReader creates a Store using the Reader pool.
func NewStoreWithReader(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
	}
}

// CreatePlatformEvent records a new platform event.
func (s *Store) CreatePlatformEvent(ctx context.Context, p gen.CreatePlatformEventParams) (gen.PlatformEvent, error) {
	return s.queries.CreatePlatformEvent(ctx, p)
}

// ListPlatformEvents returns events matching the given filters.
func (s *Store) ListPlatformEvents(ctx context.Context, tenantID pgtype.UUID, filter EventFilter) ([]gen.PlatformEvent, error) {
	return s.queries.ListPlatformEvents(ctx, gen.ListPlatformEventsParams{
		TenantID: tenantID,
		Column2:  filter.EventType,
		Column3:  pgtype.UUID{Bytes: [16]byte{}, Valid: true}, // zero UUID = no user filter
		Column4:  filter.EntityType,
		Column5:  nullableTimestamptz(filter.StartDate),
		Column6:  nullableTimestamptz(filter.EndDate),
		Limit:    filter.Limit,
		Offset:   filter.Page * filter.Limit,
	})
}

// GetEventCountsByType aggregates event counts by event_type.
func (s *Store) GetEventCountsByType(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *time.Time) ([]gen.GetEventCountsByTypeRow, error) {
	return s.queries.GetEventCountsByType(ctx, gen.GetEventCountsByTypeParams{
		TenantID: tenantID,
		Column2:  nullableTimestamptz(startDate),
		Column3:  nullableTimestamptz(endDate),
	})
}

// GetDailyContentStats returns per-content daily stats.
func (s *Store) GetDailyContentStats(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Date, contentID pgtype.UUID) ([]gen.GetDailyContentStatsRow, error) {
	return s.queries.GetDailyContentStats(ctx, gen.GetDailyContentStatsParams{
		TenantID: tenantID,
		Column2:  nullableDate(startDate),
		Column3:  nullableDate(endDate),
		Column4:  contentID,
	})
}

// GetTopContentByViews returns the top N content items by views.
func (s *Store) GetTopContentByViews(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Date, limit int32) ([]gen.GetTopContentByViewsRow, error) {
	return s.queries.GetTopContentByViews(ctx, gen.GetTopContentByViewsParams{
		TenantID: tenantID,
		Column2:  nullableDate(startDate),
		Column3:  nullableDate(endDate),
		Limit:    limit,
	})
}

// GetDailySpaceStats returns per-space daily stats.
func (s *Store) GetDailySpaceStats(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Date, spaceID pgtype.UUID) ([]gen.GetDailySpaceStatsRow, error) {
	return s.queries.GetDailySpaceStats(ctx, gen.GetDailySpaceStatsParams{
		TenantID: tenantID,
		Column2:  nullableDate(startDate),
		Column3:  nullableDate(endDate),
		Column4:  spaceID,
	})
}

// GetUserEngagementStats returns materialized view data for user engagement.
func (s *Store) GetUserEngagementStats(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Date, userID pgtype.UUID) ([]gen.MvUserEngagementStat, error) {
	return s.queries.GetUserEngagementStats(ctx, gen.GetUserEngagementStatsParams{
		TenantID: tenantID,
		Column2:  nullableDate(startDate),
		Column3:  nullableDate(endDate),
		Column4:  userID,
	})
}

// GetUserGrowthOverTime returns new user registrations over time.
func (s *Store) GetUserGrowthOverTime(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Date) ([]gen.GetUserGrowthOverTimeRow, error) {
	return s.queries.GetUserGrowthOverTime(ctx, gen.GetUserGrowthOverTimeParams{
		TenantID: tenantID,
		Column2:  nullableDate(startDate),
		Column3:  nullableDate(endDate),
	})
}

// GetTrafficSources returns referrer breakdown from platform events.
func (s *Store) GetTrafficSources(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Timestamptz, limit int32) ([]gen.GetTrafficSourcesRow, error) {
	return s.queries.GetTrafficSources(ctx, gen.GetTrafficSourcesParams{
		TenantID: tenantID,
		Column2:  nullableTimestamptzPtr(startDate),
		Column3:  nullableTimestamptzPtr(endDate),
		Limit:    limit,
	})
}

// RefreshDailyContentStatsView refreshes the content stats materialized view.
func (s *Store) RefreshDailyContentStatsView(ctx context.Context) error {
	return s.queries.RefreshDailyContentStatsView(ctx)
}

// RefreshDailySpaceStatsView refreshes the space stats materialized view.
func (s *Store) RefreshDailySpaceStatsView(ctx context.Context) error {
	return s.queries.RefreshDailySpaceStatsView(ctx)
}

// RefreshUserEngagementStatsView refreshes the user engagement stats materialized view.
func (s *Store) RefreshUserEngagementStatsView(ctx context.Context) error {
	return s.queries.RefreshUserEngagementStatsView(ctx)
}

// CreateSecurityScanResult records a new security scan finding.
func (s *Store) CreateSecurityScanResult(ctx context.Context, p gen.CreateSecurityScanResultParams) (gen.SecurityScanResult, error) {
	return s.queries.CreateSecurityScanResult(ctx, p)
}

// ListSecurityScanResults returns scan results matching filters.
func (s *Store) ListSecurityScanResults(ctx context.Context, tenantID pgtype.UUID, filter SecurityScanFilter) ([]gen.SecurityScanResult, error) {
	return s.queries.ListSecurityScanResults(ctx, gen.ListSecurityScanResultsParams{
		TenantID: tenantID,
		Column2:  filter.Severity,
		Column3:  filter.Unresolved,
		Limit:    filter.Limit,
		Offset:   filter.Page * filter.Limit,
	})
}

// ResolveSecurityScanResult marks a finding as resolved.
func (s *Store) ResolveSecurityScanResult(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.queries.ResolveSecurityScanResult(ctx, gen.ResolveSecurityScanResultParams{
		ID:       id,
		TenantID: tenantID,
	})
}

// GetUnresolvedCriticalFindings returns unresolved critical findings for alerting.
func (s *Store) GetUnresolvedCriticalFindings(ctx context.Context, tenantID pgtype.UUID) ([]gen.GetUnresolvedCriticalFindingsRow, error) {
	return s.queries.GetUnresolvedCriticalFindings(ctx, tenantID)
}

// GetDailyActiveUsers returns DAU over time.
func (s *Store) GetDailyActiveUsers(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Timestamptz) ([]gen.GetDailyActiveUsersRow, error) {
	return s.queries.GetDailyActiveUsers(ctx, gen.GetDailyActiveUsersParams{
		TenantID: tenantID,
		Column2:  nullableTimestamptzPtr(startDate),
		Column3:  nullableTimestamptzPtr(endDate),
	})
}

// GetMonthlyActiveUsers returns MAU over time.
func (s *Store) GetMonthlyActiveUsers(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Timestamptz) ([]gen.GetMonthlyActiveUsersRow, error) {
	return s.queries.GetMonthlyActiveUsers(ctx, gen.GetMonthlyActiveUsersParams{
		TenantID: tenantID,
		Column2:  nullableTimestamptzPtr(startDate),
		Column3:  nullableTimestamptzPtr(endDate),
	})
}

// GetContentVolumeOverTime returns content creation volume.
func (s *Store) GetContentVolumeOverTime(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Timestamptz) ([]gen.GetContentVolumeOverTimeRow, error) {
	return s.queries.GetContentVolumeOverTime(ctx, gen.GetContentVolumeOverTimeParams{
		TenantID: tenantID,
		Column2:  nullableTimestamptzPtr(startDate),
		Column3:  nullableTimestamptzPtr(endDate),
	})
}

// GetTotalUsers returns the total number of users for a tenant.
func (s *Store) GetTotalUsers(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.queries.GetTotalUsers(ctx, tenantID)
}

// GetActiveUsers returns the number of active users (logged in within 30 days).
func (s *Store) GetActiveUsers(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.queries.GetActiveUsers(ctx, tenantID)
}

// GetNewUsersToday returns the number of users registered today.
func (s *Store) GetNewUsersToday(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.queries.GetNewUsersToday(ctx, tenantID)
}

// nullableTimestamptz converts a *time.Time to a pgtype.Timestamptz.
func nullableTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}

	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// nullableTimestamptzPtr converts a *pgtype.Timestamptz to itself, or returns
// a zero-value if nil. Used when the caller already has a pgtype.Timestamptz.
func nullableTimestamptzPtr(t *pgtype.Timestamptz) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}

	return *t
}

// nullableDate converts a *pgtype.Date to itself, or returns a zero-value if nil.
func nullableDate(d *pgtype.Date) pgtype.Date {
	if d == nil {
		return pgtype.Date{Valid: false}
	}

	return *d
}

// GetSearchAnalytics returns search analytics summary for the given time window.
func (s *Store) GetSearchAnalytics(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *time.Time, limit int32) ([]gen.GetSearchAnalyticsSummaryRow, error) {
	return s.queries.GetSearchAnalyticsSummary(ctx, gen.GetSearchAnalyticsSummaryParams{
		TenantID: tenantID,
		Column2:  nullableTimestamptz(startDate),
		Column3:  nullableTimestamptz(endDate),
		Limit:    limit,
		Offset:   0,
	})
}

// GetRevenueSummary returns total revenue, monthly revenue, and active subscription count.
func (s *Store) GetRevenueSummary(ctx context.Context, tenantID pgtype.UUID) (gen.GetRevenueSummaryRow, error) {
	return s.queries.GetRevenueSummary(ctx, tenantID)
}

// GetRevenueByPeriod returns revenue aggregated by month for the given time window.
func (s *Store) GetRevenueByPeriod(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *time.Time) ([]gen.GetRevenueByPeriodRow, error) {
	return s.queries.GetRevenueByPeriod(ctx, gen.GetRevenueByPeriodParams{
		TenantID: tenantID,
		Column2:  nullableTimestamptz(startDate),
		Column3:  nullableTimestamptz(endDate),
	})
}
