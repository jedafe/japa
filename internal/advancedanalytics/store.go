// Package advancedanalytics implements the Advanced Analytics Dashboard
// (Section 66, ADR-084). Extends §40 Platform Analytics with revenue-stream
// breakdown, top-content leaderboards, ad performance reporting, newsletter
// engagement rates, user retention cohorts, and CSV export.
//
// Ref: japaconnect-engineering/sections/66-advanced-analytics.md
//      japaconnect-engineering/adr/ADR-084-advanced-analytics.md
package advancedanalytics

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for advanced analytics.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates an advanced analytics store.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

func (s *Store) TopContent(ctx context.Context, arg gen.AdvancedAnalyticsTopContentParams) ([]gen.AdvancedAnalyticsTopContentRow, error) {
	rows, err := s.queries.AdvancedAnalyticsTopContent(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("advancedanalytics: top content: %w", err)
	}
	return rows, nil
}

func (s *Store) RevenueStreams(ctx context.Context, arg gen.AdvancedAnalyticsRevenueStreamsParams) ([]gen.MvRevenueStream, error) {
	rows, err := s.queries.AdvancedAnalyticsRevenueStreams(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("advancedanalytics: revenue streams: %w", err)
	}
	return rows, nil
}

func (s *Store) AdPerformance(ctx context.Context, arg gen.AdvancedAnalyticsAdPerformanceParams) ([]gen.AdvancedAnalyticsAdPerformanceRow, error) {
	rows, err := s.queries.AdvancedAnalyticsAdPerformance(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("advancedanalytics: ad performance: %w", err)
	}
	return rows, nil
}

func (s *Store) NewsletterEngagement(ctx context.Context, arg gen.AdvancedAnalyticsNewsletterEngagementParams) ([]gen.AdvancedAnalyticsNewsletterEngagementRow, error) {
	rows, err := s.queries.AdvancedAnalyticsNewsletterEngagement(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("advancedanalytics: newsletter engagement: %w", err)
	}
	return rows, nil
}

func (s *Store) SubscriberGrowth(ctx context.Context, tenantID pgtype.UUID) ([]gen.AdvancedAnalyticsSubscriberGrowthRow, error) {
	rows, err := s.queries.AdvancedAnalyticsSubscriberGrowth(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("advancedanalytics: subscriber growth: %w", err)
	}
	return rows, nil
}

func (s *Store) RetentionCohorts(ctx context.Context, arg gen.AdvancedAnalyticsRetentionCohortsParams) ([]gen.AdvancedAnalyticsRetentionCohortsRow, error) {
	rows, err := s.queries.AdvancedAnalyticsRetentionCohorts(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("advancedanalytics: retention cohorts: %w", err)
	}
	return rows, nil
}
