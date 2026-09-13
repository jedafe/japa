package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides database operations for the analytics subsystem.
// Uses the Reader pool for read queries and Writer pool for writes,
// matching the reader/writer separation pattern used across the codebase.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewStore creates a Store backed by both Reader and Writer pools.
func NewStore(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// NewStoreWithReader creates a Store using only the Reader pool (read-only).
func NewStoreWithReader(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Reader),
	}
}

// CreatePlatformEvent records a new platform event.
func (s *Store) CreatePlatformEvent(ctx context.Context, p gen.CreatePlatformEventParams) (gen.CreatePlatformEventRow, error) {
	return s.write.CreatePlatformEvent(ctx, p)
}

// ListPlatformEvents returns events matching the given filters.
// BUG-001 fix: pass filter.UserID as a parsed UUID so the admin event list
// can actually be filtered by user. A zero/empty UserID produces a zero
// UUID which the SQL treats as "no user filter" (the $3 = zero-uuid OR
// branch).
func (s *Store) ListPlatformEvents(ctx context.Context, tenantID pgtype.UUID, filter EventFilter) ([]gen.ListPlatformEventsRow, error) {
	var userID pgtype.UUID
	if filter.UserID != "" {
		if err := userID.Scan(filter.UserID); err != nil {
			return nil, fmt.Errorf("invalid user_id filter: %w", err)
		}
	}

	return s.read.ListPlatformEvents(ctx, gen.ListPlatformEventsParams{Column1: filter.EventType, Column2: userID, Column3: filter.EntityType, Column4: nullableTimestamptz(filter.StartDate), Column5: nullableTimestamptz(filter.EndDate), Limit: filter.Limit,
		Offset: filter.Page * filter.Limit,
	})
}

// GetEventCountsByType aggregates event counts by event_type.
func (s *Store) GetEventCountsByType(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *time.Time) ([]gen.GetEventCountsByTypeRow, error) {
	return s.read.GetEventCountsByType(ctx, gen.GetEventCountsByTypeParams{Column1: nullableTimestamptz(startDate), Column2: nullableTimestamptz(endDate)})
}

// GetDailyContentStats returns per-content daily stats.
func (s *Store) GetDailyContentStats(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Date, contentID pgtype.UUID) ([]gen.GetDailyContentStatsRow, error) {
	return s.read.GetDailyContentStats(ctx, gen.GetDailyContentStatsParams{Column1: nullableDate(startDate), Column2: nullableDate(endDate), Column3: contentID})
}

// GetTopContentByViews returns the top N content items by views.
func (s *Store) GetTopContentByViews(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Date, limit int32) ([]gen.GetTopContentByViewsRow, error) {
	return s.read.GetTopContentByViews(ctx, gen.GetTopContentByViewsParams{Column1: nullableDate(startDate), Column2: nullableDate(endDate), Limit: limit})
}

// GetDailySpaceStats returns per-space daily stats.
func (s *Store) GetDailySpaceStats(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Date, spaceID pgtype.UUID) ([]gen.GetDailySpaceStatsRow, error) {
	return s.read.GetDailySpaceStats(ctx, gen.GetDailySpaceStatsParams{Column1: nullableDate(startDate), Column2: nullableDate(endDate), Column3: spaceID})
}

// GetUserEngagementStats returns daily user engagement directly from the
// operational tables (contents, reactions, comments, user_content_views).
// Bypasses the materialized view so the dashboard works without waiting for
// the hourly refresh or the platform_events pipeline.
func (s *Store) GetUserEngagementStats(ctx context.Context, userID pgtype.UUID, startDate, endDate *pgtype.Timestamptz) ([]gen.GetUserEngagementStatsRow, error) {
	return s.read.GetUserEngagementStats(ctx, gen.GetUserEngagementStatsParams{Column1: nullableTimestamptzPtr(startDate), Column2: nullableTimestamptzPtr(endDate), Column3: userID})
}

// GetUserGrowthOverTime returns new user registrations over time.
func (s *Store) GetUserGrowthOverTime(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Date) ([]gen.GetUserGrowthOverTimeRow, error) {
	return s.read.GetUserGrowthOverTime(ctx, gen.GetUserGrowthOverTimeParams{Column1: nullableDate(startDate), Column2: nullableDate(endDate)})
}

// GetUserTrafficSources returns traffic-source breakdown for the user's content.
// Ref: sections/40-platform-analytics-reports.md (user dashboard traffic sources).
func (s *Store) GetUserTrafficSources(ctx context.Context, tenantID, authorID pgtype.UUID, startDate, endDate *pgtype.Timestamptz, limit int32) ([]gen.GetUserTrafficSourcesRow, error) {
	return s.read.GetUserTrafficSources(ctx, gen.GetUserTrafficSourcesParams{

		AuthorID: authorID, Column2: nullableTimestamptzPtr(startDate), Column3: nullableTimestamptzPtr(endDate), Limit: limit,
	})
}

// GetUserAdPerformance returns aggregate ad stats for the user's campaigns.
// Ref: sections/40-platform-analytics-reports.md (user dashboard ad performance).
func (s *Store) GetUserAdPerformance(ctx context.Context, tenantID, createdBy pgtype.UUID, startDate, endDate *pgtype.Date, limit int32) ([]gen.GetUserAdPerformanceRow, error) {
	return s.read.GetUserAdPerformance(ctx, gen.GetUserAdPerformanceParams{

		CreatedBy: createdBy, Column2: nullableDate(startDate), Column3: nullableDate(endDate), Limit: limit,
	})
}

// GetTrafficSources returns referrer breakdown from platform events.
func (s *Store) GetTrafficSources(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Timestamptz, limit int32) ([]gen.GetTrafficSourcesRow, error) {
	return s.read.GetTrafficSources(ctx, gen.GetTrafficSourcesParams{Column1: nullableTimestamptzPtr(startDate), Column2: nullableTimestamptzPtr(endDate), Limit: limit})
}

// RefreshDailyContentStatsView refreshes the content stats materialized view.
func (s *Store) RefreshDailyContentStatsView(ctx context.Context) error {
	return s.write.RefreshDailyContentStatsView(ctx)
}

// RefreshDailySpaceStatsView refreshes the space stats materialized view.
func (s *Store) RefreshDailySpaceStatsView(ctx context.Context) error {
	return s.write.RefreshDailySpaceStatsView(ctx)
}

// RefreshUserEngagementStatsView refreshes the user engagement stats materialized view.
func (s *Store) RefreshUserEngagementStatsView(ctx context.Context) error {
	return s.write.RefreshUserEngagementStatsView(ctx)
}

// RefreshTopContent7dView refreshes the §66 top-content (7d) materialized view.
func (s *Store) RefreshTopContent7dView(ctx context.Context) error {
	return s.write.RefreshTopContent7dView(ctx)
}

// RefreshTopContent30dView refreshes the §66 top-content (30d) materialized view.
func (s *Store) RefreshTopContent30dView(ctx context.Context) error {
	return s.write.RefreshTopContent30dView(ctx)
}

// RefreshTopContent90dView refreshes the §66 top-content (90d) materialized view.
func (s *Store) RefreshTopContent90dView(ctx context.Context) error {
	return s.write.RefreshTopContent90dView(ctx)
}

// RefreshRevenueStreamsView refreshes the §66 revenue-streams materialized view.
func (s *Store) RefreshRevenueStreamsView(ctx context.Context) error {
	return s.write.RefreshRevenueStreamsView(ctx)
}

// CreateSecurityScanResult records a new security scan finding, or refreshes
// the existing unresolved finding of the same scan_type (upsert by
// tenant_id + scan_type for unresolved rows) to avoid duplicate findings
// on repeated scans.
func (s *Store) CreateSecurityScanResult(ctx context.Context, p gen.CreateSecurityScanResultParams) (gen.CreateSecurityScanResultRow, error) {
	return s.write.CreateSecurityScanResult(ctx, p)
}

// ListSecurityScanResults returns scan results matching filters.
func (s *Store) ListSecurityScanResults(ctx context.Context, tenantID pgtype.UUID, filter SecurityScanFilter) ([]gen.SecurityScanResult, error) {
	return s.read.ListSecurityScanResults(ctx, gen.ListSecurityScanResultsParams{Column1: filter.Severity, Column2: filter.Unresolved, Limit: filter.Limit,
		Offset: filter.Page * filter.Limit,
	})
}

// ResolveSecurityScanResult marks a finding as resolved.
func (s *Store) ResolveSecurityScanResult(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.write.ResolveSecurityScanResult(ctx,
		id,
	)
}

// DeleteSecurityScanResult permanently deletes a finding (tenant-scoped).
func (s *Store) DeleteSecurityScanResult(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.write.DeleteSecurityScanResult(ctx,
		id,
	)
}

// GetUnresolvedCriticalFindings returns unresolved critical findings for alerting.
func (s *Store) GetUnresolvedCriticalFindings(ctx context.Context, tenantID pgtype.UUID) ([]gen.GetUnresolvedCriticalFindingsRow, error) {
	return s.read.GetUnresolvedCriticalFindings(ctx)
}

// GetDailyActiveUsers returns DAU over time.
func (s *Store) GetDailyActiveUsers(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Timestamptz) ([]gen.GetDailyActiveUsersRow, error) {
	return s.read.GetDailyActiveUsers(ctx, gen.GetDailyActiveUsersParams{Column1: nullableTimestamptzPtr(startDate), Column2: nullableTimestamptzPtr(endDate)})
}

// GetMonthlyActiveUsers returns MAU over time.
func (s *Store) GetMonthlyActiveUsers(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Timestamptz) ([]gen.GetMonthlyActiveUsersRow, error) {
	return s.read.GetMonthlyActiveUsers(ctx, gen.GetMonthlyActiveUsersParams{Column1: nullableTimestamptzPtr(startDate), Column2: nullableTimestamptzPtr(endDate)})
}

// GetContentVolumeOverTime returns content creation volume.
func (s *Store) GetContentVolumeOverTime(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *pgtype.Timestamptz) ([]gen.GetContentVolumeOverTimeRow, error) {
	return s.read.GetContentVolumeOverTime(ctx, gen.GetContentVolumeOverTimeParams{Column1: nullableTimestamptzPtr(startDate), Column2: nullableTimestamptzPtr(endDate)})
}

// GetTotalUsers returns the total number of users for a tenant.
func (s *Store) GetTotalUsers(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.read.GetTotalUsers(ctx)
}

// GetActiveUsers returns the number of active users (logged in within 30 days).
func (s *Store) GetActiveUsers(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.read.GetActiveUsers(ctx)
}

// GetNewUsersToday returns the number of users registered today.
func (s *Store) GetNewUsersToday(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.read.GetNewUsersToday(ctx)
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
	return s.read.GetSearchAnalyticsSummary(ctx, gen.GetSearchAnalyticsSummaryParams{Column1: nullableTimestamptz(startDate), Column2: nullableTimestamptz(endDate), Limit: limit,
		Offset: 0,
	})
}

// GetRevenueSummary returns total revenue, monthly revenue, and active subscription count.
func (s *Store) GetRevenueSummary(ctx context.Context, tenantID pgtype.UUID) (gen.GetRevenueSummaryRow, error) {
	return s.read.GetRevenueSummary(ctx)
}

// GetRevenueByPeriod returns revenue aggregated by month for the given time window.
func (s *Store) GetRevenueByPeriod(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *time.Time) ([]gen.GetRevenueByPeriodRow, error) {
	return s.read.GetRevenueByPeriod(ctx, gen.GetRevenueByPeriodParams{Column1: nullableTimestamptz(startDate), Column2: nullableTimestamptz(endDate)})
}

// CountPendingReports returns the total number of pending content reports.
func (s *Store) CountPendingReports(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.read.CountPendingReports(ctx)
}

// CountOpenTickets returns the number of tickets not yet resolved/closed/spam.
func (s *Store) CountOpenTickets(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.read.CountOpenTickets(ctx)
}

// CountSpamQueue returns the total number of flagged media and auto-hidden content.
func (s *Store) CountSpamQueue(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.read.CountSpamQueue(ctx)
}

// CountGenerationsToday returns the number of AI studio jobs created today.
func (s *Store) CountGenerationsToday(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.read.CountGenerationsToday(ctx)
}

// CountPendingReviews returns the number of content reviews awaiting HITL review.
func (s *Store) CountPendingReviews(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	return s.read.CountPendingReviews(ctx)
}

// GetContentAuthorID returns the author_id of a content item, used for
// analytics access control (GET /content/{id}/analytics is owner+admin only).
func (s *Store) GetContentAuthorID(ctx context.Context, contentID, tenantID pgtype.UUID) (pgtype.UUID, error) {
	return s.read.GetContentAuthorID(ctx,
		contentID,
	)
}
