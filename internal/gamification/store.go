// Package gamification implements the Advanced Gamification layer (Section
// 64, ADR-082): badges, challenges, and a leaderboard built on top of the
// existing Points/Credits ledger and lifetime_points counter.
//
// Badge evaluation and challenge progress increment inside the existing
// wallet:process_activity_event Asynq task — no separate gamification task
// for most operations. The leaderboard is served from wallet_ledger with
// Valkey caching (jc:leaderboard:{tenant_id}:{scope}:{period}, 1hr TTL).
//
// Ref: japaconnect-engineering/sections/64-gamification.md
//
//	japaconnect-engineering/adr/ADR-082-advanced-gamification.md
package gamification

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for gamification operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a gamification store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// =====================================================================
// Badge Definitions
// =====================================================================

// ListBadgeDefinitions lists badge definitions for a tenant, optionally
// filtering to active only.
func (s *Store) ListBadgeDefinitions(ctx context.Context, tenantID pgtype.UUID, activeOnly bool) ([]gen.BadgeDefinition, error) {
	return s.queries.ListBadgeDefinitions(ctx, gen.ListBadgeDefinitionsParams{
		TenantID:   tenantID,
		ActiveOnly: activeOnly,
	})
}

// ListBadgeDefinitionsPaginated returns a page of badge definitions and the
// total count for pagination metadata.
func (s *Store) ListBadgeDefinitionsPaginated(ctx context.Context, tenantID pgtype.UUID, activeOnly bool, limit, offset int32) ([]gen.BadgeDefinition, int64, error) {
	rows, err := s.queries.ListBadgeDefinitionsPaginated(ctx, gen.ListBadgeDefinitionsPaginatedParams{
		TenantID:   tenantID,
		ActiveOnly: activeOnly,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("gamification: list badges paginated: %w", err)
	}

	total, err := s.queries.CountBadgeDefinitions(ctx, gen.CountBadgeDefinitionsParams{
		TenantID:   tenantID,
		ActiveOnly: activeOnly,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("gamification: count badges: %w", err)
	}

	return rows, total, nil
}

// GetBadgeDefinitionByID fetches a single badge definition by ID within a tenant.
func (s *Store) GetBadgeDefinitionByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.BadgeDefinition, error) {
	return s.queries.GetBadgeDefinitionByID(ctx, gen.GetBadgeDefinitionByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
}

// CreateBadgeDefinition inserts a new badge definition.
func (s *Store) CreateBadgeDefinition(ctx context.Context, arg gen.CreateBadgeDefinitionParams) (gen.BadgeDefinition, error) {
	return s.queries.CreateBadgeDefinition(ctx, arg)
}

// UpdateBadgeDefinition updates a badge definition's mutable fields.
func (s *Store) UpdateBadgeDefinition(ctx context.Context, arg gen.UpdateBadgeDefinitionParams) (gen.BadgeDefinition, error) {
	return s.queries.UpdateBadgeDefinition(ctx, arg)
}

// DeleteBadgeDefinition deletes a non-system badge definition. Returns the
// number of rows affected (0 if the badge was not found or is a system badge).
func (s *Store) DeleteBadgeDefinition(ctx context.Context, tenantID, id pgtype.UUID) (int64, error) {
	return s.queries.DeleteBadgeDefinition(ctx, gen.DeleteBadgeDefinitionParams{
		TenantID: tenantID,
		ID:       id,
	})
}

// =====================================================================
// User Badges
// =====================================================================

// AwardBadge inserts a user_badge row, ignoring duplicates via ON CONFLICT.
func (s *Store) AwardBadge(ctx context.Context, arg gen.AwardBadgeParams) (gen.UserBadge, error) {
	return s.queries.AwardBadge(ctx, arg)
}

// GetUserBadge fetches a single user_badge row, if it exists.
func (s *Store) GetUserBadge(ctx context.Context, tenantID, userID, badgeID pgtype.UUID) (gen.UserBadge, error) {
	return s.queries.GetUserBadge(ctx, gen.GetUserBadgeParams{
		TenantID: tenantID,
		UserID:   userID,
		BadgeID:  badgeID,
	})
}

// ListUserBadges lists a user's earned badges with badge definition details.
func (s *Store) ListUserBadges(ctx context.Context, tenantID, userID pgtype.UUID) ([]gen.ListUserBadgesRow, error) {
	return s.queries.ListUserBadges(ctx, gen.ListUserBadgesParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

// ListUserBadgesByUsername lists a user's earned badges by username — used
// for the public profile badge display.
func (s *Store) ListUserBadgesByUsername(ctx context.Context, tenantID pgtype.UUID, username string) ([]gen.ListUserBadgesByUsernameRow, error) {
	return s.queries.ListUserBadgesByUsername(ctx, gen.ListUserBadgesByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
}

// =====================================================================
// Challenge Definitions
// =====================================================================

// ListActiveChallenges lists active challenges within their date range.
func (s *Store) ListActiveChallenges(ctx context.Context, tenantID pgtype.UUID) ([]gen.ChallengeDefinition, error) {
	return s.queries.ListActiveChallenges(ctx, tenantID)
}

// ListAllChallenges lists all challenges for a tenant (admin view), paginated.
func (s *Store) ListAllChallenges(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ChallengeDefinition, int64, error) {
	rows, err := s.queries.ListAllChallenges(ctx, gen.ListAllChallengesParams{
		TenantID:   tenantID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("gamification: list challenges: %w", err)
	}

	total, err := s.queries.CountAllChallenges(ctx, tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("gamification: count challenges: %w", err)
	}

	return rows, total, nil
}

// GetChallengeByID fetches a single challenge definition by ID within a tenant.
func (s *Store) GetChallengeByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.ChallengeDefinition, error) {
	return s.queries.GetChallengeByID(ctx, gen.GetChallengeByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
}

// CreateChallenge inserts a new challenge definition.
func (s *Store) CreateChallenge(ctx context.Context, arg gen.CreateChallengeParams) (gen.ChallengeDefinition, error) {
	return s.queries.CreateChallenge(ctx, arg)
}

// UpdateChallenge updates a challenge definition's mutable fields.
func (s *Store) UpdateChallenge(ctx context.Context, arg gen.UpdateChallengeParams) (gen.ChallengeDefinition, error) {
	return s.queries.UpdateChallenge(ctx, arg)
}

// DeleteChallenge deletes a challenge definition. Returns the number of rows
// affected (0 if the challenge was not found).
func (s *Store) DeleteChallenge(ctx context.Context, tenantID, id pgtype.UUID) (int64, error) {
	return s.queries.DeleteChallenge(ctx, gen.DeleteChallengeParams{
		TenantID: tenantID,
		ID:       id,
	})
}

// ExpireChallengesPastEndDate deactivates challenges whose ends_at is in the
// past. Returns the number of rows affected.
func (s *Store) ExpireChallengesPastEndDate(ctx context.Context, tenantID pgtype.UUID) (int64, error) {
	return s.queries.ExpireChallengesPastEndDate(ctx, tenantID)
}

// =====================================================================
// User Challenge Progress
// =====================================================================

// UpsertChallengeProgress increments a user's progress on a challenge,
// marking it completed if the target_count is reached.
func (s *Store) UpsertChallengeProgress(ctx context.Context, arg gen.UpsertChallengeProgressParams) (gen.UserChallengeProgress, error) {
	return s.queries.UpsertChallengeProgress(ctx, arg)
}

// GetChallengeProgress fetches a user's progress on a specific challenge.
func (s *Store) GetChallengeProgress(ctx context.Context, tenantID, userID, challengeID pgtype.UUID) (gen.UserChallengeProgress, error) {
	return s.queries.GetChallengeProgress(ctx, gen.GetChallengeProgressParams{
		TenantID:    tenantID,
		UserID:      userID,
		ChallengeID: challengeID,
	})
}

// ListUserChallengeProgress lists a user's challenge progress with challenge
// definition details.
func (s *Store) ListUserChallengeProgress(ctx context.Context, tenantID, userID pgtype.UUID) ([]gen.ListUserChallengeProgressRow, error) {
	return s.queries.ListUserChallengeProgress(ctx, gen.ListUserChallengeProgressParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

// MarkChallengeRewardClaimed marks a challenge reward as claimed.
func (s *Store) MarkChallengeRewardClaimed(ctx context.Context, id pgtype.UUID) error {
	return s.queries.MarkChallengeRewardClaimed(ctx, id)
}

// =====================================================================
// Leaderboard
// =====================================================================

// LeaderboardAllTime returns the all-time points leaderboard from
// wallet_ledger. spaceID is optional — when valid, only members of that
// space are ranked (per-space scope, Section 64).
func (s *Store) LeaderboardAllTime(ctx context.Context, tenantID, spaceID pgtype.UUID, topN int32) ([]gen.LeaderboardAllTimeRow, error) {
	return s.queries.LeaderboardAllTime(ctx, gen.LeaderboardAllTimeParams{
		TenantID: tenantID,
		SpaceID:  spaceID,
		TopN:     topN,
	})
}

// LeaderboardByPeriod returns the period-scoped points leaderboard from
// wallet_ledger earns since fromDate. spaceID is optional — when valid,
// only members of that space are ranked.
func (s *Store) LeaderboardByPeriod(ctx context.Context, tenantID, spaceID pgtype.UUID, fromDate pgtype.Timestamptz, topN int32) ([]gen.LeaderboardByPeriodRow, error) {
	return s.queries.LeaderboardByPeriod(ctx, gen.LeaderboardByPeriodParams{
		TenantID: tenantID,
		SpaceID:  spaceID,
		FromDate: fromDate,
		TopN:     topN,
	})
}

// =====================================================================
// Badge evaluation helpers
// =====================================================================

// GetBadgesByActivityTrigger returns active activity_count badges for a
// specific activity_key.
func (s *Store) GetBadgesByActivityTrigger(ctx context.Context, tenantID pgtype.UUID, activityKey string) ([]gen.BadgeDefinition, error) {
	return s.queries.GetBadgesByActivityTrigger(ctx, gen.GetBadgesByActivityTriggerParams{
		TenantID:    tenantID,
		ActivityKey: &activityKey,
	})
}

// GetBadgesByPointsTrigger returns active lifetime_points badges.
func (s *Store) GetBadgesByPointsTrigger(ctx context.Context, tenantID pgtype.UUID) ([]gen.BadgeDefinition, error) {
	return s.queries.GetBadgesByPointsTrigger(ctx, tenantID)
}

// CountUserActivityByKey counts a user's platform_events for a specific
// activity_key (non-synthetic only).
func (s *Store) CountUserActivityByKey(ctx context.Context, tenantID, userID pgtype.UUID, activityKey string) (int64, error) {
	return s.queries.CountUserActivityByKey(ctx, gen.CountUserActivityByKeyParams{
		TenantID:    tenantID,
		UserID:      userID,
		ActivityKey: activityKey,
	})
}

// GetUserLifetimePoints returns the sum of positive points earns for a user —
// the lifetime-points counter used by lifetime_points badge triggers (§28).
func (s *Store) GetUserLifetimePoints(ctx context.Context, tenantID, userID pgtype.UUID) (int64, error) {
	return s.queries.GetLifetimePoints(ctx, gen.GetLifetimePointsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

// GetChallengesByActivityKey returns active challenges matching an activity_key.
func (s *Store) GetChallengesByActivityKey(ctx context.Context, tenantID pgtype.UUID, activityKey string) ([]gen.ChallengeDefinition, error) {
	return s.queries.GetChallengesByActivityKey(ctx, gen.GetChallengesByActivityKeyParams{
		TenantID:    tenantID,
		ActivityKey: activityKey,
	})
}
