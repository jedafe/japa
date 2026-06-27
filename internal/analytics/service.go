package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
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
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			queries: gen.New(database.Writer),
		},
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

// noopEnqueuer is a no-op implementation for development.
type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(taskType string, payload []byte, opts ...interface{}) error {
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
}

// TrackEvent records a platform event in the write-optimised append-only table.
func (s *Service) TrackEvent(ctx context.Context, tenantID pgtype.UUID, input TrackEventInput) (gen.PlatformEvent, error) {
	if input.EventType == "" {
		return gen.PlatformEvent{}, fmt.Errorf("event_type is required")
	}

	metaBytes := []byte("{}")
	if len(input.Meta) > 0 {
		b, err := json.Marshal(input.Meta)
		if err != nil {
			return gen.PlatformEvent{}, fmt.Errorf("marshaling event meta: %w", err)
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
	})
}

// ListEvents returns platform events matching the given filters.
func (s *Service) ListEvents(ctx context.Context, tenantID pgtype.UUID, filter EventFilter) ([]gen.PlatformEvent, error) {
	if filter.Limit <= 0 || filter.Limit > MaxPageSize {
		filter.Limit = DefaultPageSize
	}

	var userID pgtype.UUID
	if filter.UserID != "" {
		_ = userID.Scan(filter.UserID)
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

// RefreshViews triggers CONCURRENT refresh of all three materialized views.
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

	return nil
}

// EnqueueRefreshViews enqueues a background job to refresh all materialized views.
func (s *Service) EnqueueRefreshViews(ctx context.Context) error {
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
	TopContent        []gen.GetTopContentByViewsRow   `json:"top_content"`
	UserEngagement    []gen.MvUserEngagementStat       `json:"user_engagement"`
	ContentOverTime   []gen.GetContentVolumeOverTimeRow `json:"content_over_time"`
}

// GetUserDashboardData returns the user's personal analytics dashboard data.
func (s *Service) GetUserDashboardData(ctx context.Context, tenantID, userID pgtype.UUID, days int32) (UserDashboard, error) {
	start := time.Now().AddDate(0, 0, -int(days))
	startDate := pgtype.Date{Time: start, Valid: true}
	zeroUUID := pgtype.UUID{}

	topContent, err := s.store.GetTopContentByViews(ctx, tenantID, &startDate, nil, 10)
	if err != nil {
		return UserDashboard{}, fmt.Errorf("fetching top content: %w", err)
	}

	engagement, err := s.store.GetUserEngagementStats(ctx, tenantID, &startDate, nil, userID)
	if err != nil {
		return UserDashboard{}, fmt.Errorf("fetching user engagement: %w", err)
	}

	contentOverTime, err := s.store.GetContentVolumeOverTime(ctx, tenantID, &pgtype.Timestamptz{Time: start, Valid: true}, nil)
	if err != nil {
		return UserDashboard{}, fmt.Errorf("fetching content volume: %w", err)
	}

	_ = zeroUUID

	return UserDashboard{
		TopContent:      topContent,
		UserEngagement:  engagement,
		ContentOverTime: contentOverTime,
	}, nil
}

// AdminDashboard holds the admin-facing analytics dashboard response.
type AdminDashboard struct {
	DailyActiveUsers  []gen.GetDailyActiveUsersRow     `json:"daily_active_users"`
	MonthlyActiveUsers []gen.GetMonthlyActiveUsersRow  `json:"monthly_active_users"`
	EventCounts       []gen.GetEventCountsByTypeRow    `json:"event_counts"`
	ContentOverTime   []gen.GetContentVolumeOverTimeRow `json:"content_over_time"`
	UserGrowth        []gen.GetUserGrowthOverTimeRow    `json:"user_growth"`
	TrafficSources    []gen.GetTrafficSourcesRow        `json:"traffic_sources"`
	TopContent        []gen.GetTopContentByViewsRow     `json:"top_content"`
	ContentStats      []gen.MvDailyContentStat          `json:"content_stats"`
	SpaceStats        []gen.MvDailySpaceStat            `json:"space_stats"`
}

// GetAdminDashboardData returns the admin analytics dashboard data.
func (s *Service) GetAdminDashboardData(ctx context.Context, tenantID pgtype.UUID, days int32) (AdminDashboard, error) {
	start := time.Now().AddDate(0, 0, -int(days))
	startTZ := pgtype.Timestamptz{Time: start, Valid: true}
	startDate := pgtype.Date{Time: start, Valid: true}

	dau, err := s.store.GetDailyActiveUsers(ctx, tenantID, &startTZ, nil)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching DAU: %w", err)
	}

	mau, err := s.store.GetMonthlyActiveUsers(ctx, tenantID, &startTZ, nil)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching MAU: %w", err)
	}

	eventCounts, err := s.store.GetEventCountsByType(ctx, tenantID, &start, nil)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching event counts: %w", err)
	}

	contentVolume, err := s.store.GetContentVolumeOverTime(ctx, tenantID, &startTZ, nil)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching content volume: %w", err)
	}

	userGrowth, err := s.store.GetUserGrowthOverTime(ctx, tenantID, &startDate, nil)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching user growth: %w", err)
	}

	traffic, err := s.store.GetTrafficSources(ctx, tenantID, &startTZ, nil, 20)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching traffic sources: %w", err)
	}

	topContent, err := s.store.GetTopContentByViews(ctx, tenantID, &startDate, nil, 20)
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching top content: %w", err)
	}

	contentStats, err := s.store.GetDailyContentStats(ctx, tenantID, &startDate, nil, pgtype.UUID{})
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching content stats: %w", err)
	}

	spaceStats, err := s.store.GetDailySpaceStats(ctx, tenantID, &startDate, nil, pgtype.UUID{})
	if err != nil {
		return AdminDashboard{}, fmt.Errorf("fetching space stats: %w", err)
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
	}, nil
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

	return nil
}

// EnqueueSecurityScan enqueues a background job to run the security scanner.
func (s *Service) EnqueueSecurityScan(ctx context.Context) error {
	payload, err := json.Marshal(map[string]any{})
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

// GetUnresolvedCriticalFindings returns unresolved critical findings.
func (s *Service) GetUnresolvedCriticalFindings(ctx context.Context, tenantID pgtype.UUID) ([]gen.GetUnresolvedCriticalFindingsRow, error) {
	return s.store.GetUnresolvedCriticalFindings(ctx, tenantID)
}
