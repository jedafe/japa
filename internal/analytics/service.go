package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
	"golang.org/x/sync/errgroup"
)

// Enqueuer is the interface for enqueuing background tasks.
type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service provides analytics operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer Enqueuer
	settings *config.Settings // nil disables system_settings lookups (tests)
}

// NewService creates a Service backed by both Reader and Writer pools.
func NewService(database *db.DB) *Service {
	return &Service{
		store:    NewStore(database),
		db:       database,
		enqueuer: &noopEnqueuer{},
	}
}

// NewServiceWithEnqueuer creates a Service with a custom enqueuer for background tasks.
func NewServiceWithEnqueuer(database *db.DB, enqueuer Enqueuer) *Service {
	svc := NewService(database)
	svc.enqueuer = enqueuer

	return svc
}

// SetSettings attaches the system_settings reader for Rule 3 compliance
// (security scan thresholds and other configurable values).
func (s *Service) SetSettings(settings *config.Settings) {
	s.settings = settings
}

// noopEnqueuer is a no-op implementation for development.
type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error {
	return nil
}

// ---------------------------------------------------------------------------
// Event Tracking
// ---------------------------------------------------------------------------

// TrackEventInput holds parameters for recording a platform event.
type TrackEventInput struct {
	EventType   EventType
	UserID      pgtype.UUID
	EntityType  string
	EntityID    pgtype.UUID
	Meta        map[string]any
	CountryCode string
	SessionID   string
	IsSynthetic bool // ADR-068 §4 / P10: true for persona-originated events
}

// TrackEvent records a platform event in the write-optimised append-only table.
// BUG-004 fix: validate event_type against the ValidEventTypes allowlist so
// arbitrary strings cannot pollute platform_events.
func (s *Service) TrackEvent(ctx context.Context, tenantID pgtype.UUID, input TrackEventInput) (gen.CreatePlatformEventRow, error) {
	if input.EventType == "" {
		return gen.CreatePlatformEventRow{}, fmt.Errorf("%w: event_type is required", errpkg.ErrRequired)
	}

	if !isValidEventType(input.EventType) {
		return gen.CreatePlatformEventRow{}, fmt.Errorf("%w: unknown event_type %q", errpkg.ErrInvalid, input.EventType)
	}

	metaBytes := []byte("{}")

	if len(input.Meta) > 0 {
		b, err := json.Marshal(input.Meta)
		if err != nil {
			return gen.CreatePlatformEventRow{}, fmt.Errorf("marshaling event meta: %w", err)
		}

		metaBytes = b
	}

	var userID pgtype.UUID
	if input.UserID.Valid {
		userID = input.UserID
	}

	var entityType *string
	if input.EntityType != "" {
		entityType = &input.EntityType
	}

	var countryCode *string
	if input.CountryCode != "" {
		countryCode = &input.CountryCode
	}

	var sessionID *string
	if input.SessionID != "" {
		sessionID = &input.SessionID
	}

	return s.store.CreatePlatformEvent(ctx, gen.CreatePlatformEventParams{
		TenantID:    tenantID,
		EventType:   string(input.EventType),
		UserID:      userID,
		EntityType:  entityType,
		EntityID:    input.EntityID,
		Meta:        metaBytes,
		CountryCode: countryCode,
		SessionID:   sessionID,
		IsSynthetic: input.IsSynthetic,
	})
}

// TrackSyntheticEvent records a platform event with is_synthetic = true for
// persona-originated activity. This is the participation engine's metrics-
// integrity hook (ADR-068 §4, P10): every dispatched or queued persona action
// is recorded here so the persona activity log (WHERE is_synthetic = true)
// works and real-metric materialized views (WHERE NOT is_synthetic) exclude
// synthetic activity.
func (s *Service) TrackSyntheticEvent(ctx context.Context, tenantID, userID pgtype.UUID, eventType, entityType string, entityID pgtype.UUID, meta map[string]any) error {
	_, err := s.TrackEvent(ctx, tenantID, TrackEventInput{
		EventType:   EventType(eventType),
		UserID:      userID,
		EntityType:  entityType,
		EntityID:    entityID,
		Meta:        meta,
		IsSynthetic: true,
	})
	return err
}

// ListEvents returns platform events matching the given filters.
func (s *Service) ListEvents(ctx context.Context, tenantID pgtype.UUID, filter EventFilter) ([]gen.ListPlatformEventsRow, error) {
	if filter.Limit <= 0 || filter.Limit > MaxPageSize {
		filter.Limit = DefaultPageSize
	}

	var userID pgtype.UUID
	if filter.UserID != "" {
		if err := userID.Scan(filter.UserID); err != nil {
			return nil, fmt.Errorf("invalid user_id: %w", err)
		}
	}

	return s.store.ListPlatformEvents(ctx, tenantID, filter)
}

// GetEventCountsByType aggregates event counts by type within a time range.
func (s *Service) GetEventCountsByType(ctx context.Context, tenantID pgtype.UUID, startDate, endDate *time.Time) ([]gen.GetEventCountsByTypeRow, error) {
	return s.store.GetEventCountsByType(ctx, tenantID, startDate, endDate)
}

// ---------------------------------------------------------------------------
// Materialized View Refresh
// ---------------------------------------------------------------------------

// RefreshViews triggers CONCURRENT refresh of all materialized views,
// including the §40 base views and the §66 (ADR-084) advanced-analytics views.
// This is typically invoked by the analytics:refresh_views Asynq task.
func (s *Service) RefreshViews(ctx context.Context) error {
	if err := s.store.RefreshDailyContentStatsView(ctx); err != nil {
		slog.Error("refreshing mv_daily_content_stats", "error", err)
		return fmt.Errorf("refreshing content stats view: %w", err)
	}

	slog.Info("refreshed mv_daily_content_stats")

	if err := s.store.RefreshDailySpaceStatsView(ctx); err != nil {
		slog.Error("refreshing mv_daily_space_stats", "error", err)
		return fmt.Errorf("refreshing space stats view: %w", err)
	}

	slog.Info("refreshed mv_daily_space_stats")

	if err := s.store.RefreshUserEngagementStatsView(ctx); err != nil {
		slog.Error("refreshing mv_user_engagement_stats", "error", err)
		return fmt.Errorf("refreshing user engagement stats view: %w", err)
	}

	slog.Info("refreshed mv_user_engagement_stats")

	// Section 66 (ADR-084) — Advanced Analytics materialized views.
	// mv_top_content_* are refreshed hourly; mv_revenue_streams daily.
	// Ref: sections/66-advanced-analytics.md
	if err := s.store.RefreshTopContent7dView(ctx); err != nil {
		slog.Error("refreshing mv_top_content_7d", "error", err)
		return fmt.Errorf("refreshing top content 7d view: %w", err)
	}
	if err := s.store.RefreshTopContent30dView(ctx); err != nil {
		slog.Error("refreshing mv_top_content_30d", "error", err)
		return fmt.Errorf("refreshing top content 30d view: %w", err)
	}
	if err := s.store.RefreshTopContent90dView(ctx); err != nil {
		slog.Error("refreshing mv_top_content_90d", "error", err)
		return fmt.Errorf("refreshing top content 90d view: %w", err)
	}
	slog.Info("refreshed mv_top_content_7d/30d/90d")

	if err := s.store.RefreshRevenueStreamsView(ctx); err != nil {
		slog.Error("refreshing mv_revenue_streams", "error", err)
		return fmt.Errorf("refreshing revenue streams view: %w", err)
	}
	slog.Info("refreshed mv_revenue_streams")

	return nil
}

// EnqueueRefreshViews enqueues a background job to refresh all materialized views.
func (s *Service) EnqueueRefreshViews(_ context.Context) error {
	payload, err := json.Marshal(map[string]any{})
	if err != nil {
		return fmt.Errorf("marshaling refresh views payload: %w", err)
	}

	return s.enqueuer.Enqueue(worker.TypeAnalyticsRefreshViews, payload)
}

// ---------------------------------------------------------------------------
// Dashboard Data
// ---------------------------------------------------------------------------

// UserDashboard holds the user-facing analytics dashboard response.
type UserDashboard struct {
	TopContent      []gen.GetTopContentByViewsRow     `json:"top_content"`
	UserEngagement  []gen.GetUserEngagementStatsRow   `json:"user_engagement"`
	ContentOverTime []gen.GetContentVolumeOverTimeRow `json:"content_over_time"`
	TrafficSources  []gen.GetUserTrafficSourcesRow    `json:"traffic_sources"`
	AdPerformance   []gen.GetUserAdPerformanceRow     `json:"ad_performance"`
}

// GetUserDashboardData returns the user's personal analytics dashboard data.
// from/to define the query window; a nil to means open-ended (up to now).
func (s *Service) GetUserDashboardData(ctx context.Context, tenantID, userID pgtype.UUID, from, to *time.Time) (UserDashboard, error) {
	if from == nil || from.IsZero() {
		defaultStart := time.Now().AddDate(0, 0, -30)
		from = &defaultStart
	}

	var startDate, endDate *pgtype.Date
	var startTZ, endTZ *pgtype.Timestamptz

	startDate = &pgtype.Date{Time: *from, Valid: true}
	startTZ = &pgtype.Timestamptz{Time: *from, Valid: true}
	if to != nil && !to.IsZero() {
		endDate = &pgtype.Date{Time: *to, Valid: true}
		endTZ = &pgtype.Timestamptz{Time: *to, Valid: true}
	}

	// Run the dashboard queries concurrently — each is an independent aggregate
	// over the same time window (BUG-ANALYTICS-12).
	var (
		topContent      []gen.GetTopContentByViewsRow
		engagement      []gen.GetUserEngagementStatsRow
		contentOverTime []gen.GetContentVolumeOverTimeRow
		trafficSources  []gen.GetUserTrafficSourcesRow
		adPerformance   []gen.GetUserAdPerformanceRow
	)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		topContent, err = s.store.GetTopContentByViews(ctx, tenantID, startDate, endDate, 10)
		return err
	})
	g.Go(func() error {
		var err error
		engagement, err = s.store.GetUserEngagementStats(ctx, tenantID, startTZ, endTZ, userID)
		return err
	})
	g.Go(func() error {
		var err error
		contentOverTime, err = s.store.GetContentVolumeOverTime(ctx, tenantID, startTZ, endTZ)
		return err
	})
	g.Go(func() error {
		var err error
		trafficSources, err = s.store.GetUserTrafficSources(ctx, tenantID, userID, startTZ, endTZ, 10)
		return err
	})
	g.Go(func() error {
		var err error
		adPerformance, err = s.store.GetUserAdPerformance(ctx, tenantID, userID, startDate, endDate, 10)
		return err
	})

	if err := g.Wait(); err != nil {
		return UserDashboard{}, fmt.Errorf("loading dashboard data: %w", err)
	}

	if topContent == nil {
		topContent = []gen.GetTopContentByViewsRow{}
	}
	if engagement == nil {
		engagement = []gen.GetUserEngagementStatsRow{}
	}
	if contentOverTime == nil {
		contentOverTime = []gen.GetContentVolumeOverTimeRow{}
	}
	if trafficSources == nil {
		trafficSources = []gen.GetUserTrafficSourcesRow{}
	}
	if adPerformance == nil {
		adPerformance = []gen.GetUserAdPerformanceRow{}
	}

	return UserDashboard{
		TopContent:      topContent,
		UserEngagement:  engagement,
		ContentOverTime: contentOverTime,
		TrafficSources:  trafficSources,
		AdPerformance:   adPerformance,
	}, nil
}

// AdminDashboard holds the admin-facing analytics dashboard response.
type AdminDashboard struct {
	DailyActiveUsers   []gen.GetDailyActiveUsersRow      `json:"daily_active_users"`
	MonthlyActiveUsers []gen.GetMonthlyActiveUsersRow    `json:"monthly_active_users"`
	EventCounts        []gen.GetEventCountsByTypeRow     `json:"event_counts"`
	ContentOverTime    []gen.GetContentVolumeOverTimeRow `json:"content_over_time"`
	UserGrowth         []gen.GetUserGrowthOverTimeRow    `json:"user_growth"`
	TrafficSources     []gen.GetTrafficSourcesRow        `json:"traffic_sources"`
	TopContent         []gen.GetTopContentByViewsRow     `json:"top_content"`
	ContentStats       []gen.GetDailyContentStatsRow     `json:"content_stats"`
	SpaceStats         []gen.GetDailySpaceStatsRow       `json:"space_stats"`
	PendingReports     int32                             `json:"pending_reports"`
	PendingTickets     int32                             `json:"pending_tickets"`
	SpamQueue          int32                             `json:"spam_queue"`
	GenerationsToday   int32                             `json:"generations_today"`
	HITLPending        int32                             `json:"hitl_pending"`
}

// UserAnalytics holds the admin user analytics response.
type UserAnalytics struct {
	TotalUsers    int32 `json:"total_users"`
	ActiveUsers   int32 `json:"active_users"`
	NewUsersToday int32 `json:"new_users_today"`
}

// GetAdminDashboardData returns the admin analytics dashboard data for the
// window [from, to]. When to is nil the window is left-open (up to now),
// preserving the historical days-based behavior.
func (s *Service) GetAdminDashboardData(ctx context.Context, tenantID pgtype.UUID, from time.Time, to *time.Time) (AdminDashboard, error) {
	start := from
	startTZ := pgtype.Timestamptz{Time: start, Valid: true}
	startDate := pgtype.Date{Time: start, Valid: true}

	var (
		endTime *time.Time
		endTZ   *pgtype.Timestamptz
		endDate *pgtype.Date
	)

	if to != nil {
		end := *to
		endTime = &end
		endTZ = &pgtype.Timestamptz{Time: end, Valid: true}
		endDate = &pgtype.Date{Time: end, Valid: true}
	}

	dau, err := s.store.GetDailyActiveUsers(ctx, tenantID, &startTZ, endTZ)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching DAU: %w", err)
	}

	mau, err := s.store.GetMonthlyActiveUsers(ctx, tenantID, &startTZ, endTZ)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching MAU: %w", err)
	}

	eventCounts, err := s.store.GetEventCountsByType(ctx, tenantID, &start, endTime)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching event counts: %w", err)
	}

	contentVolume, err := s.store.GetContentVolumeOverTime(ctx, tenantID, &startTZ, endTZ)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching content volume: %w", err)
	}

	userGrowth, err := s.store.GetUserGrowthOverTime(ctx, tenantID, &startDate, endDate)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching user growth: %w", err)
	}

	traffic, err := s.store.GetTrafficSources(ctx, tenantID, &startTZ, endTZ, 20)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching traffic sources: %w", err)
	}

	topContent, err := s.store.GetTopContentByViews(ctx, tenantID, &startDate, endDate, 20)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching top content: %w", err)
	}

	contentStats, err := s.store.GetDailyContentStats(ctx, tenantID, &startDate, endDate, pgtype.UUID{Bytes: [16]byte{}, Valid: true})
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching content stats: %w", err)
	}

	spaceStats, err := s.store.GetDailySpaceStats(ctx, tenantID, &startDate, endDate, pgtype.UUID{Bytes: [16]byte{}, Valid: true})
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching space stats: %w", err)
	}

	pendingReports, err := s.store.CountPendingReports(ctx, tenantID)
	if err != nil {
		slog.Error("fetching pending reports count", "error", err)
	}
	pendingTickets, err := s.store.CountOpenTickets(ctx, tenantID)
	if err != nil {
		slog.Error("fetching open tickets count", "error", err)
	}
	spamQueue, err := s.store.CountSpamQueue(ctx, tenantID)
	if err != nil {
		slog.Error("fetching spam queue count", "error", err)
	}
	generationsToday, err := s.store.CountGenerationsToday(ctx, tenantID)
	if err != nil {
		slog.Error("fetching generations today count", "error", err)
	}
	hitlPending, err := s.store.CountPendingReviews(ctx, tenantID)
	if err != nil {
		slog.Error("fetching pending reviews count", "error", err)
	}

	return AdminDashboard{
		DailyActiveUsers:   dau,
		MonthlyActiveUsers: mau,
		EventCounts:        eventCounts,
		ContentOverTime:    contentVolume,
		UserGrowth:         userGrowth,
		TrafficSources:     traffic,
		TopContent:         topContent,
		ContentStats:       contentStats,
		SpaceStats:         spaceStats,
		PendingReports:     pendingReports,
		PendingTickets:     pendingTickets,
		SpamQueue:          spamQueue,
		GenerationsToday:   generationsToday,
		HITLPending:        hitlPending,
	}, nil
}

// GetUserAnalytics returns aggregate user counts for the admin dashboard.
func (s *Service) GetUserAnalytics(ctx context.Context, tenantID pgtype.UUID) (UserAnalytics, error) {
	total, err := s.store.GetTotalUsers(ctx, tenantID)
	if err != nil {
		return UserAnalytics{}, fmt.Errorf("fetching total users: %w", err)
	}
	active, err := s.store.GetActiveUsers(ctx, tenantID)
	if err != nil {
		return UserAnalytics{}, fmt.Errorf("fetching active users: %w", err)
	}
	newToday, err := s.store.GetNewUsersToday(ctx, tenantID)
	if err != nil {
		return UserAnalytics{}, fmt.Errorf("fetching new users today: %w", err)
	}
	return UserAnalytics{TotalUsers: total, ActiveUsers: active, NewUsersToday: newToday}, nil
}

// ---------------------------------------------------------------------------
// Security Scans
// ---------------------------------------------------------------------------

// RunSecurityScan executes all security checks and records findings.
func (s *Service) RunSecurityScan(ctx context.Context, tenantID pgtype.UUID) error {
	// 1. Check for failed login spikes.
	if err := s.scanFailedLoginSpike(ctx, tenantID); err != nil {
		slog.Error("security scan: failed login spike", "error", err)
	}

	// 2. Check SLA breach count.
	if err := s.scanSLABreachCount(ctx, tenantID); err != nil {
		slog.Error("security scan: SLA breach count", "error", err)
	}

	// 3. Check for stale tokens.
	if err := s.scanStaleTokens(ctx, tenantID); err != nil {
		slog.Error("security scan: stale tokens", "error", err)
	}

	// 4. Check for unverified users.
	if err := s.scanUnverifiedUsers(ctx, tenantID); err != nil {
		slog.Error("security scan: unverified users", "error", err)
	}

	// 5. Check for weak password settings.
	if err := s.scanWeakPasswordSettings(ctx, tenantID); err != nil {
		slog.Error("security scan: weak password settings", "error", err)
	}

	// 6. Check for excessive API keys without expiration.
	if err := s.scanExcessiveAPIKeys(ctx, tenantID); err != nil {
		slog.Error("security scan: excessive API keys", "error", err)
	}

	return nil
}

// EnqueueSecurityScan enqueues a background job to run the security scanner.
// BUG-ANALYTICS-08 fix: pass the tenant ID in the payload so the worker
// scans only the requesting tenant, not all tenants.
func (s *Service) EnqueueSecurityScan(_ context.Context, tenantID pgtype.UUID) error {
	payload, err := json.Marshal(map[string]any{
		"tenant_id": tenantID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshaling security scan payload: %w", err)
	}

	return s.enqueuer.Enqueue(worker.TypeSecurityScan, payload)
}

// ListSecurityScanResults returns scan results matching filters.
func (s *Service) ListSecurityScanResults(ctx context.Context, tenantID pgtype.UUID, filter SecurityScanFilter) ([]gen.SecurityScanResult, error) {
	if filter.Limit <= 0 || filter.Limit > MaxPageSize {
		filter.Limit = DefaultPageSize
	}

	return s.store.ListSecurityScanResults(ctx, tenantID, filter)
}

// ResolveScanResult marks a security finding as resolved.
func (s *Service) ResolveScanResult(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.ResolveSecurityScanResult(ctx, id, tenantID)
}

// DeleteScanResult permanently deletes a security finding. Used to clear
// resolved findings from the control panel audit view.
func (s *Service) DeleteScanResult(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.DeleteSecurityScanResult(ctx, id, tenantID)
}

// GetUnresolvedCriticalFindings returns unresolved critical findings.
func (s *Service) GetUnresolvedCriticalFindings(ctx context.Context, tenantID pgtype.UUID) ([]gen.GetUnresolvedCriticalFindingsRow, error) {
	return s.store.GetUnresolvedCriticalFindings(ctx, tenantID)
}

// RevenueAnalytics holds the admin revenue analytics response.
type RevenueAnalytics struct {
	TotalRevenue        int64                       `json:"total_revenue"`
	MonthlyRevenue      int64                       `json:"monthly_revenue"`
	ActiveSubscriptions int32                       `json:"active_subscriptions"`
	Periods             []gen.GetRevenueByPeriodRow `json:"periods"`
}

// GetRevenueAnalytics returns aggregated revenue data for the admin dashboard.
func (s *Service) GetRevenueAnalytics(ctx context.Context, tenantID pgtype.UUID, from time.Time, to *time.Time) (RevenueAnalytics, error) {
	summary, err := s.store.GetRevenueSummary(ctx, tenantID)
	if err != nil {
		return RevenueAnalytics{}, fmt.Errorf("fetching revenue summary: %w", err)
	}

	periods, err := s.store.GetRevenueByPeriod(ctx, tenantID, &from, to)
	if err != nil {
		return RevenueAnalytics{}, fmt.Errorf("fetching revenue by period: %w", err)
	}

	return RevenueAnalytics{
		TotalRevenue:        summary.TotalRevenue,
		MonthlyRevenue:      summary.MonthlyRevenue,
		ActiveSubscriptions: summary.ActiveSubscriptions,
		Periods:             periods,
	}, nil
}
