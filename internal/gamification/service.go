// Package gamification — service.go holds the business logic for badges,
// challenges, and the leaderboard. It reads gamification.* system_settings
// via the hot-reloadable config.Settings and caches leaderboard results in
// Valkey (jc:leaderboard:{tenant_id}:{scope}:{period}, 1hr TTL).
//
// Badge evaluation (EvaluateBadges / EvaluatePointsBadges) and challenge
// progress incrementing are called from the existing
// wallet:process_activity_event Asynq task — no separate gamification task
// for most operations. ExpireChallenges is called by the daily
// gamification:expire_challenges task.
//
// Ref: japaconnect-engineering/sections/64-gamification.md
//      japaconnect-engineering/adr/ADR-082-advanced-gamification.md
package gamification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
)

// Trigger type constants — match the badge_definitions.trigger_type CHECK.
const (
	TriggerTypeLifetimePoints = "lifetime_points"
	TriggerTypeActivityCount  = "activity_count"
	TriggerTypeManual         = "manual"
)

// Leaderboard scope constants.
const (
	ScopeGlobal = "global"
	// ScopeSpace is the prefix for space-scoped leaderboards: "space:{id}".
	ScopeSpace = "space"
)

// Leaderboard period constants.
const (
	PeriodAllTime = "all_time"
	PeriodMonthly = "monthly"
	PeriodWeekly  = "weekly"
)

// leaderboardCacheTTL is the TTL for jc:leaderboard:{tenant_id}:{scope}:{period}
// (ADR-082: 1 hour).
const leaderboardCacheTTL = 1 * time.Hour

// defaultLeaderboardTopN is the default number of leaderboard entries returned.
const defaultLeaderboardTopN = 100

// ValkeyCache is the interface for the leaderboard cache. The
// valkeycompat.Cmdable satisfies this.
type ValkeyCache interface {
	Get(ctx context.Context, key string) *valkeycompat.StringCmd
	Set(ctx context.Context, key string, value any, ttl time.Duration) *valkeycompat.StatusCmd
}

// Service holds the business logic for the gamification layer.
type Service struct {
	store    *Store
	settings *config.Settings
	cache    valkeycompat.Cmdable // nil = caching disabled
}

// NewService creates a gamification service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetCache injects the Valkey client for leaderboard caching.
func (s *Service) SetCache(c valkeycompat.Cmdable) { s.cache = c }

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

// BadgeInfo is the JSON-serialisable representation of a badge definition.
type BadgeInfo struct {
	ID                 string  `json:"id"`
	Slug               string  `json:"slug"`
	Name               string  `json:"name"`
	Description        string  `json:"description,omitempty"`
	IconMediaID        string  `json:"icon_media_id,omitempty"`
	TriggerType        string  `json:"trigger_type"`
	TriggerValue       int     `json:"trigger_value,omitempty"`
	TriggerActivityKey string  `json:"trigger_activity_key,omitempty"`
	IsActive           bool    `json:"is_active"`
	IsSystem           bool    `json:"is_system"`
	DisplayOrder       int     `json:"display_order"`
	CreatedAt          string  `json:"created_at"`
}

// BadgeWithDetails is a user's earned badge with the badge definition details.
type BadgeWithDetails struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	IconMediaID  *string `json:"icon_media_id,omitempty"`
	AwardedAt    string  `json:"awarded_at"`
}

// ChallengeInfo is the JSON-serialisable representation of a challenge.
type ChallengeInfo struct {
	ID            string  `json:"id"`
	Slug          string  `json:"slug"`
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	ImageMediaID  string  `json:"image_media_id,omitempty"`
	ActivityKey   string  `json:"activity_key"`
	TargetCount   int     `json:"target_count"`
	RewardPoints  int     `json:"reward_points"`
	RewardBadgeID string  `json:"reward_badge_id,omitempty"`
	StartsAt      string  `json:"starts_at"`
	EndsAt        *string `json:"ends_at,omitempty"`
	IsActive      bool    `json:"is_active"`
	CreatedAt     string  `json:"created_at"`
}

// ChallengeWithProgress is a user's challenge progress with challenge details.
type ChallengeWithProgress struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	TargetCount  int     `json:"target_count"`
	CurrentCount int     `json:"current_count"`
	Completed    bool    `json:"completed"`
	CompletedAt  *string `json:"completed_at,omitempty"`
	EndsAt       *string `json:"ends_at,omitempty"`
}

// LeaderboardEntry is a single row in the leaderboard.
type LeaderboardEntry struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Points   int64  `json:"points"`
	Rank     int    `json:"rank"`
}

// CreateBadgeInput is the admin request body for creating a badge.
type CreateBadgeInput struct {
	Slug               string  `json:"slug"`
	Name               string  `json:"name"`
	Description        *string `json:"description,omitempty"`
	IconMediaID        *string `json:"icon_media_id,omitempty"`
	TriggerType        string  `json:"trigger_type"`
	TriggerValue       *int32  `json:"trigger_value,omitempty"`
	TriggerActivityKey *string `json:"trigger_activity_key,omitempty"`
	IsActive           *bool   `json:"is_active,omitempty"`
	DisplayOrder       *int32  `json:"display_order,omitempty"`
}

// UpdateBadgeInput is the admin request body for updating a badge.
type UpdateBadgeInput struct {
	Name               *string  `json:"name,omitempty"`
	Description        *string  `json:"description,omitempty"`
	IconMediaID        *string  `json:"icon_media_id,omitempty"`
	TriggerType        *string  `json:"trigger_type,omitempty"`
	TriggerValue       *int32   `json:"trigger_value,omitempty"`
	TriggerActivityKey *string  `json:"trigger_activity_key,omitempty"`
	IsActive           *bool    `json:"is_active,omitempty"`
	DisplayOrder       *int32   `json:"display_order,omitempty"`
}

// CreateChallengeInput is the admin request body for creating a challenge.
type CreateChallengeInput struct {
	Slug           string  `json:"slug"`
	Name           string  `json:"name"`
	Description    *string `json:"description,omitempty"`
	ImageMediaID   *string `json:"image_media_id,omitempty"`
	ActivityKey    string  `json:"activity_key"`
	TargetCount    int32   `json:"target_count"`
	RewardPoints   int32   `json:"reward_points,omitempty"`
	RewardBadgeID  *string `json:"reward_badge_id,omitempty"`
	StartsAt       string  `json:"starts_at"`
	EndsAt         *string `json:"ends_at,omitempty"`
	IsActive       *bool   `json:"is_active,omitempty"`
}

// ---------------------------------------------------------------------------
// Settings helpers
// ---------------------------------------------------------------------------

// badgesEnabled returns whether the badge system is enabled.
func (s *Service) badgesEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "gamification.badges_enabled", true)
}

// challengesEnabled returns whether the challenge system is enabled.
func (s *Service) challengesEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "gamification.challenges_enabled", true)
}

// leaderboardEnabled returns whether the leaderboard is visible.
func (s *Service) leaderboardEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "gamification.leaderboard_enabled", true)
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

// rfc3339 formats a pgtype.Timestamptz as RFC 3339 UTC, or "" if invalid.
func rfc3339(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// ptrStr dereferences a *string safely, returning "" for nil.
func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// uuidStr returns the string representation of a pgtype.UUID, or "" if invalid.
func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return u.String()
}

// scanUUID parses a UUID string into a pgtype.UUID.
func scanUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}

// scanTimestamp parses an RFC 3339 string into a pgtype.Timestamptz.
func scanTimestamp(s string) (pgtype.Timestamptz, error) {
	if s == "" {
		return pgtype.Timestamptz{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return pgtype.Timestamptz{}, fmt.Errorf("gamification: parse timestamp %q: %w", s, err)
	}
	return pgtype.Timestamptz{Time: t, Valid: true}, nil
}

// badgeToInfo converts a gen.BadgeDefinition to a BadgeInfo DTO.
func badgeToInfo(b gen.BadgeDefinition) BadgeInfo {
	info := BadgeInfo{
		ID:           uuidStr(b.ID),
		Slug:         b.Slug,
		Name:         b.Name,
		Description:  ptrStr(b.Description),
		IconMediaID:  uuidStr(b.IconMediaID),
		TriggerType:  b.TriggerType,
		IsActive:     b.IsActive,
		IsSystem:     b.IsSystem,
		DisplayOrder: int(b.DisplayOrder),
		CreatedAt:    rfc3339(b.CreatedAt),
	}
	if b.TriggerValue != nil {
		info.TriggerValue = int(*b.TriggerValue)
	}
	if b.TriggerActivityKey != nil {
		info.TriggerActivityKey = *b.TriggerActivityKey
	}
	return info
}

// challengeToInfo converts a gen.ChallengeDefinition to a ChallengeInfo DTO.
func challengeToInfo(c gen.ChallengeDefinition) ChallengeInfo {
	info := ChallengeInfo{
		ID:           uuidStr(c.ID),
		Slug:         c.Slug,
		Name:         c.Name,
		Description:  ptrStr(c.Description),
		ImageMediaID: uuidStr(c.ImageMediaID),
		ActivityKey:  c.ActivityKey,
		TargetCount:  int(c.TargetCount),
		RewardPoints: int(c.RewardPoints),
		StartsAt:     rfc3339(c.StartsAt),
		IsActive:     c.IsActive,
		CreatedAt:    rfc3339(c.CreatedAt),
	}
	if c.RewardBadgeID.Valid {
		id := c.RewardBadgeID.String()
		info.RewardBadgeID = id
	}
	if c.EndsAt.Valid {
		ends := rfc3339(c.EndsAt)
		info.EndsAt = &ends
	}
	return info
}

// ---------------------------------------------------------------------------
// Public read operations
// ---------------------------------------------------------------------------

// ListBadges returns all active badge definitions for a tenant.
func (s *Service) ListBadges(ctx context.Context, tenantID pgtype.UUID) ([]BadgeInfo, error) {
	if !s.badgesEnabled(ctx, tenantID) {
		return []BadgeInfo{}, nil
	}

	badges, err := s.store.ListBadgeDefinitions(ctx, tenantID, true)
	if err != nil {
		return nil, fmt.Errorf("gamification: list badges: %w", err)
	}

	results := make([]BadgeInfo, 0, len(badges))
	for _, b := range badges {
		results = append(results, badgeToInfo(b))
	}
	return results, nil
}

// ListActiveChallenges returns active challenges within their date range.
func (s *Service) ListActiveChallenges(ctx context.Context, tenantID pgtype.UUID) ([]ChallengeInfo, error) {
	if !s.challengesEnabled(ctx, tenantID) {
		return []ChallengeInfo{}, nil
	}

	challenges, err := s.store.ListActiveChallenges(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("gamification: list active challenges: %w", err)
	}

	results := make([]ChallengeInfo, 0, len(challenges))
	for _, c := range challenges {
		results = append(results, challengeToInfo(c))
	}
	return results, nil
}

// ValidateLeaderboardScope validates a leaderboard scope string. Returns the
// parsed scope type and optional space ID. Valid scopes are "global" or
// "space:{uuid}".
func ValidateLeaderboardScope(scope string) (scopeType, spaceID string, ok bool) {
	if scope == ScopeGlobal {
		return ScopeGlobal, "", true
	}
	if strings.HasPrefix(scope, ScopeSpace+":") {
		id := strings.TrimPrefix(scope, ScopeSpace+":")
		if id == "" {
			return "", "", false
		}
		return ScopeSpace, id, true
	}
	return "", "", false
}

// ValidateLeaderboardPeriod validates a leaderboard period string.
func ValidateLeaderboardPeriod(period string) bool {
	switch period {
	case PeriodAllTime, PeriodMonthly, PeriodWeekly:
		return true
	default:
		return false
	}
}

// leaderboardCacheKey builds the Valkey key for a cached leaderboard.
func leaderboardCacheKey(tenantID pgtype.UUID, scope, period string) string {
	return fmt.Sprintf("jc:leaderboard:%s:%s:%s", uuidStr(tenantID), scope, period)
}

// periodStartDate returns the UTC start-of-period time for weekly/monthly.
func periodStartDate(period string) time.Time {
	now := time.Now().UTC()
	switch period {
	case PeriodWeekly:
		return now.AddDate(0, 0, -7)
	case PeriodMonthly:
		return now.AddDate(0, -1, 0)
	default:
		return time.Time{}
	}
}

// GetLeaderboard returns the leaderboard for a scope and period. Results are
// cached in Valkey (jc:leaderboard:{tenant_id}:{scope}:{period}, 1hr TTL).
// Scope: "global" or "space:{id}". Period: "all_time", "monthly", "weekly".
func (s *Service) GetLeaderboard(ctx context.Context, tenantID pgtype.UUID, scope, period string) ([]LeaderboardEntry, error) {
	if !s.leaderboardEnabled(ctx, tenantID) {
		return []LeaderboardEntry{}, nil
	}

	scopeType, _, ok := ValidateLeaderboardScope(scope)
	if !ok {
		return nil, fmt.Errorf("gamification: invalid scope %q", scope)
	}
	if !ValidateLeaderboardPeriod(period) {
		return nil, fmt.Errorf("gamification: invalid period %q", period)
	}

	// Space-scoped leaderboards require additional filtering not present in
	// the current generated queries. Until space-scoped queries are added,
	// fall back to the global query for space scope (the scope is still
	// validated and cached separately).
	_ = scopeType

	key := leaderboardCacheKey(tenantID, scope, period)
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, key).Result(); err == nil && cached != "" {
			var entries []LeaderboardEntry
			if err := json.Unmarshal([]byte(cached), &entries); err == nil {
				return entries, nil
			}
		}
	}

	topN := int32(defaultLeaderboardTopN)
	var entries []LeaderboardEntry

	if period == PeriodAllTime {
		rows, err := s.store.LeaderboardAllTime(ctx, tenantID, topN)
		if err != nil {
			return nil, fmt.Errorf("gamification: leaderboard all_time: %w", err)
		}
		entries = make([]LeaderboardEntry, 0, len(rows))
		for i, r := range rows {
			entries = append(entries, LeaderboardEntry{
				UserID:   uuidStr(r.UserID),
				Username: r.Username,
				Points:   r.LifetimePoints,
				Rank:     i + 1,
			})
		}
	} else {
		fromDate := periodStartDate(period)
		rows, err := s.store.LeaderboardByPeriod(ctx, tenantID, pgtype.Timestamptz{Time: fromDate, Valid: true}, topN)
		if err != nil {
			return nil, fmt.Errorf("gamification: leaderboard by period: %w", err)
		}
		entries = make([]LeaderboardEntry, 0, len(rows))
		for i, r := range rows {
			entries = append(entries, LeaderboardEntry{
				UserID:   uuidStr(r.UserID),
				Username: r.Username,
				Points:   r.PeriodPoints,
				Rank:     i + 1,
			})
		}
	}

	if s.cache != nil && len(entries) > 0 {
		if data, err := json.Marshal(entries); err == nil {
			if err := s.cache.Set(ctx, key, data, leaderboardCacheTTL).Err(); err != nil {
				slog.Warn("gamification: leaderboard cache set failed", "error", err)
			}
		}
	}

	return entries, nil
}

// ListUserBadges returns a user's earned badges with badge definition details.
func (s *Service) ListUserBadges(ctx context.Context, tenantID, userID pgtype.UUID) ([]BadgeWithDetails, error) {
	if !s.badgesEnabled(ctx, tenantID) {
		return []BadgeWithDetails{}, nil
	}

	rows, err := s.store.ListUserBadges(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("gamification: list user badges: %w", err)
	}

	results := make([]BadgeWithDetails, 0, len(rows))
	for _, r := range rows {
		var iconID *string
		if r.BadgeIconMediaID.Valid {
			s := r.BadgeIconMediaID.String()
			iconID = &s
		}
		results = append(results, BadgeWithDetails{
			ID:          uuidStr(r.BadgeID),
			Slug:        r.BadgeSlug,
			Name:        r.BadgeName,
			Description: ptrStr(r.BadgeDescription),
			IconMediaID: iconID,
			AwardedAt:   rfc3339(r.AwardedAt),
		})
	}
	return results, nil
}

// ListUserBadgesByUsername returns a user's earned badges by username — used
// for the public profile badge display.
func (s *Service) ListUserBadgesByUsername(ctx context.Context, tenantID pgtype.UUID, username string) ([]BadgeWithDetails, error) {
	if !s.badgesEnabled(ctx, tenantID) {
		return []BadgeWithDetails{}, nil
	}

	rows, err := s.store.ListUserBadgesByUsername(ctx, tenantID, username)
	if err != nil {
		return nil, fmt.Errorf("gamification: list user badges by username: %w", err)
	}

	results := make([]BadgeWithDetails, 0, len(rows))
	for _, r := range rows {
		results = append(results, BadgeWithDetails{
			ID:          uuidStr(r.BadgeID),
			Slug:        r.BadgeSlug,
			Name:        r.BadgeName,
			Description: ptrStr(r.BadgeDescription),
			AwardedAt:   rfc3339(r.AwardedAt),
		})
	}
	return results, nil
}

// ListUserChallengeProgress returns a user's challenge progress with details.
func (s *Service) ListUserChallengeProgress(ctx context.Context, tenantID, userID pgtype.UUID) ([]ChallengeWithProgress, error) {
	if !s.challengesEnabled(ctx, tenantID) {
		return []ChallengeWithProgress{}, nil
	}

	rows, err := s.store.ListUserChallengeProgress(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("gamification: list user challenge progress: %w", err)
	}

	results := make([]ChallengeWithProgress, 0, len(rows))
	for _, r := range rows {
		var completedAt *string
		if r.CompletedAt.Valid {
			ts := rfc3339(r.CompletedAt)
			completedAt = &ts
		}
		var endsAt *string
		if r.EndsAt.Valid {
			ts := rfc3339(r.EndsAt)
			endsAt = &ts
		}
		results = append(results, ChallengeWithProgress{
			ID:           uuidStr(r.ChallengeID),
			Slug:         r.ChallengeSlug,
			Name:         r.ChallengeName,
			Description:  ptrStr(r.ChallengeDescription),
			TargetCount:  int(r.TargetCount),
			CurrentCount: int(r.CurrentCount),
			Completed:    r.Completed,
			CompletedAt:  completedAt,
			EndsAt:       endsAt,
		})
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Admin operations
// ---------------------------------------------------------------------------

// CreateBadge creates a new badge definition (admin).
func (s *Service) CreateBadge(ctx context.Context, tenantID pgtype.UUID, in CreateBadgeInput) (BadgeInfo, error) {
	if in.TriggerType != TriggerTypeLifetimePoints && in.TriggerType != TriggerTypeActivityCount && in.TriggerType != TriggerTypeManual {
		return BadgeInfo{}, fmt.Errorf("gamification: invalid trigger_type %q", in.TriggerType)
	}
	if in.Slug == "" || in.Name == "" {
		return BadgeInfo{}, fmt.Errorf("gamification: slug and name are required")
	}

	arg := gen.CreateBadgeDefinitionParams{
		TenantID:           tenantID,
		Slug:               in.Slug,
		Name:               in.Name,
		Description:        in.Description,
		TriggerType:        in.TriggerType,
		TriggerValue:       in.TriggerValue,
		TriggerActivityKey: in.TriggerActivityKey,
		IsSystem:           false,
		DisplayOrder:       0,
		IsActive:           true,
	}
	if in.IconMediaID != nil {
		id, err := scanUUID(*in.IconMediaID)
		if err != nil {
			return BadgeInfo{}, fmt.Errorf("gamification: invalid icon_media_id: %w", err)
		}
		arg.IconMediaID = id
	}
	if in.IsActive != nil {
		arg.IsActive = *in.IsActive
	}
	if in.DisplayOrder != nil {
		arg.DisplayOrder = *in.DisplayOrder
	}

	badge, err := s.store.CreateBadgeDefinition(ctx, arg)
	if err != nil {
		return BadgeInfo{}, fmt.Errorf("gamification: create badge: %w", err)
	}
	return badgeToInfo(badge), nil
}

// UpdateBadge updates a badge definition's mutable fields (admin). Only
// non-nil fields in the input are updated.
func (s *Service) UpdateBadge(ctx context.Context, tenantID, badgeID pgtype.UUID, in UpdateBadgeInput) (BadgeInfo, error) {
	arg := gen.UpdateBadgeDefinitionParams{
		TenantID: tenantID,
		ID:       badgeID,
	}
	if in.Name != nil {
		arg.Name = in.Name
	}
	if in.Description != nil {
		arg.Description = in.Description
	}
	if in.IconMediaID != nil {
		id, err := scanUUID(*in.IconMediaID)
		if err != nil {
			return BadgeInfo{}, fmt.Errorf("gamification: invalid icon_media_id: %w", err)
		}
		arg.IconMediaID = id
	}
	if in.TriggerType != nil {
		arg.TriggerType = in.TriggerType
	}
	if in.TriggerValue != nil {
		arg.TriggerValue = in.TriggerValue
	}
	if in.TriggerActivityKey != nil {
		arg.TriggerActivityKey = in.TriggerActivityKey
	}
	if in.IsActive != nil {
		arg.IsActive = in.IsActive
	}
	if in.DisplayOrder != nil {
		arg.DisplayOrder = in.DisplayOrder
	}

	badge, err := s.store.UpdateBadgeDefinition(ctx, arg)
	if err != nil {
		return BadgeInfo{}, fmt.Errorf("gamification: update badge: %w", err)
	}
	return badgeToInfo(badge), nil
}

// AwardBadge manually awards a badge to a user (admin). Returns the awarded
// badge details. If the user already has the badge, returns the existing row.
func (s *Service) AwardBadge(ctx context.Context, tenantID, badgeID, userID, awardedBy pgtype.UUID) (BadgeWithDetails, error) {
	// Verify the badge exists.
	badge, err := s.store.GetBadgeDefinitionByID(ctx, tenantID, badgeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BadgeWithDetails{}, fmt.Errorf("gamification: badge not found")
		}
		return BadgeWithDetails{}, fmt.Errorf("gamification: get badge: %w", err)
	}

	_, err = s.store.AwardBadge(ctx, gen.AwardBadgeParams{
		TenantID:  tenantID,
		UserID:    userID,
		BadgeID:   badgeID,
		AwardedBy: awardedBy,
	})
	if err != nil {
		// ON CONFLICT DO NOTHING returns no rows if duplicate — check existing.
		_, getErr := s.store.GetUserBadge(ctx, tenantID, userID, badgeID)
		if getErr != nil {
			return BadgeWithDetails{}, fmt.Errorf("gamification: award badge: %w", err)
		}
	}

	var iconID *string
	if badge.IconMediaID.Valid {
		s := badge.IconMediaID.String()
		iconID = &s
	}

	// Fetch the awarded_at timestamp from the user_badge row.
	ub, err := s.store.GetUserBadge(ctx, tenantID, userID, badgeID)
	if err != nil {
		return BadgeWithDetails{
			ID:          uuidStr(badge.ID),
			Slug:        badge.Slug,
			Name:        badge.Name,
			Description: ptrStr(badge.Description),
			IconMediaID: iconID,
		}, nil
	}

	return BadgeWithDetails{
		ID:          uuidStr(badge.ID),
		Slug:        badge.Slug,
		Name:        badge.Name,
		Description: ptrStr(badge.Description),
		IconMediaID: iconID,
		AwardedAt:   rfc3339(ub.AwardedAt),
	}, nil
}

// ListAllBadges returns all badge definitions (admin view, includes inactive).
func (s *Service) ListAllBadges(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]BadgeInfo, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	badges, total, err := s.store.ListBadgeDefinitionsPaginated(ctx, tenantID, false, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("gamification: list all badges: %w", err)
	}

	results := make([]BadgeInfo, 0, len(badges))
	for _, b := range badges {
		results = append(results, badgeToInfo(b))
	}
	return results, total, nil
}

// ListAllChallenges returns all challenges (admin view, includes inactive), paginated.
func (s *Service) ListAllChallenges(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]ChallengeInfo, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	challenges, total, err := s.store.ListAllChallenges(ctx, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("gamification: list all challenges: %w", err)
	}

	results := make([]ChallengeInfo, 0, len(challenges))
	for _, c := range challenges {
		results = append(results, challengeToInfo(c))
	}
	return results, total, nil
}

// CreateChallenge creates a new challenge definition (admin).
func (s *Service) CreateChallenge(ctx context.Context, tenantID pgtype.UUID, in CreateChallengeInput) (ChallengeInfo, error) {
	if in.Slug == "" || in.Name == "" || in.ActivityKey == "" {
		return ChallengeInfo{}, fmt.Errorf("gamification: slug, name, and activity_key are required")
	}
	if in.TargetCount <= 0 {
		return ChallengeInfo{}, fmt.Errorf("gamification: target_count must be positive")
	}

	startsAt, err := scanTimestamp(in.StartsAt)
	if err != nil {
		return ChallengeInfo{}, err
	}
	if !startsAt.Valid {
		return ChallengeInfo{}, fmt.Errorf("gamification: starts_at is required")
	}

	arg := gen.CreateChallengeParams{
		TenantID:     tenantID,
		Slug:         in.Slug,
		Name:         in.Name,
		Description:  in.Description,
		ActivityKey:  in.ActivityKey,
		TargetCount:  in.TargetCount,
		RewardPoints: in.RewardPoints,
		StartsAt:     startsAt,
		IsActive:     true,
	}
	if in.ImageMediaID != nil {
		id, err := scanUUID(*in.ImageMediaID)
		if err != nil {
			return ChallengeInfo{}, fmt.Errorf("gamification: invalid image_media_id: %w", err)
		}
		arg.ImageMediaID = id
	}
	if in.RewardBadgeID != nil {
		id, err := scanUUID(*in.RewardBadgeID)
		if err != nil {
			return ChallengeInfo{}, fmt.Errorf("gamification: invalid reward_badge_id: %w", err)
		}
		arg.RewardBadgeID = id
	}
	if in.EndsAt != nil {
		endsAt, err := scanTimestamp(*in.EndsAt)
		if err != nil {
			return ChallengeInfo{}, err
		}
		arg.EndsAt = endsAt
	}
	if in.IsActive != nil {
		arg.IsActive = *in.IsActive
	}

	challenge, err := s.store.CreateChallenge(ctx, arg)
	if err != nil {
		return ChallengeInfo{}, fmt.Errorf("gamification: create challenge: %w", err)
	}
	return challengeToInfo(challenge), nil
}

// UpdateChallengeInput is the admin request body for updating a challenge.
type UpdateChallengeInput struct {
	Name          *string `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
	ImageMediaID  *string `json:"image_media_id,omitempty"`
	ActivityKey   *string `json:"activity_key,omitempty"`
	TargetCount   *int32  `json:"target_count,omitempty"`
	RewardPoints  *int32  `json:"reward_points,omitempty"`
	RewardBadgeID *string `json:"reward_badge_id,omitempty"`
	StartsAt      *string `json:"starts_at,omitempty"`
	EndsAt        *string `json:"ends_at,omitempty"`
	IsActive      *bool   `json:"is_active,omitempty"`
}

// UpdateChallenge updates a challenge definition's mutable fields (admin).
func (s *Service) UpdateChallenge(ctx context.Context, tenantID, challengeID pgtype.UUID, in UpdateChallengeInput) (ChallengeInfo, error) {
	arg := gen.UpdateChallengeParams{
		TenantID: tenantID,
		ID:       challengeID,
	}
	if in.Name != nil {
		arg.Name = in.Name
	}
	if in.Description != nil {
		arg.Description = in.Description
	}
	if in.ImageMediaID != nil {
		id, err := scanUUID(*in.ImageMediaID)
		if err != nil {
			return ChallengeInfo{}, fmt.Errorf("gamification: invalid image_media_id: %w", err)
		}
		arg.ImageMediaID = id
	}
	if in.ActivityKey != nil {
		arg.ActivityKey = in.ActivityKey
	}
	if in.TargetCount != nil {
		arg.TargetCount = in.TargetCount
	}
	if in.RewardPoints != nil {
		arg.RewardPoints = in.RewardPoints
	}
	if in.RewardBadgeID != nil {
		id, err := scanUUID(*in.RewardBadgeID)
		if err != nil {
			return ChallengeInfo{}, fmt.Errorf("gamification: invalid reward_badge_id: %w", err)
		}
		arg.RewardBadgeID = id
	}
	if in.StartsAt != nil {
		startsAt, err := scanTimestamp(*in.StartsAt)
		if err != nil {
			return ChallengeInfo{}, err
		}
		arg.StartsAt = startsAt
	}
	if in.EndsAt != nil {
		endsAt, err := scanTimestamp(*in.EndsAt)
		if err != nil {
			return ChallengeInfo{}, err
		}
		arg.EndsAt = endsAt
	}
	if in.IsActive != nil {
		arg.IsActive = in.IsActive
	}

	challenge, err := s.store.UpdateChallenge(ctx, arg)
	if err != nil {
		return ChallengeInfo{}, fmt.Errorf("gamification: update challenge: %w", err)
	}
	return challengeToInfo(challenge), nil
}

// DeleteBadge deletes a non-system badge definition (admin). System badges
// cannot be deleted — the query filters them out and returns 0 rows affected.
func (s *Service) DeleteBadge(ctx context.Context, tenantID, badgeID pgtype.UUID) error {
	count, err := s.store.DeleteBadgeDefinition(ctx, tenantID, badgeID)
	if err != nil {
		return fmt.Errorf("gamification: delete badge: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("gamification: badge not found or is a system badge")
	}
	return nil
}

// DeleteChallenge deletes a challenge definition (admin).
func (s *Service) DeleteChallenge(ctx context.Context, tenantID, challengeID pgtype.UUID) error {
	count, err := s.store.DeleteChallenge(ctx, tenantID, challengeID)
	if err != nil {
		return fmt.Errorf("gamification: delete challenge: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("gamification: challenge not found")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Badge evaluation — called by wallet:process_activity_event
// ---------------------------------------------------------------------------

// EvaluateBadges checks activity_count badges for the given activity_key,
// determines if thresholds are newly crossed, and awards badges. Also
// increments challenge progress for matching challenges. Returns the newly
// awarded badges.
//
// This is called by the wallet:process_activity_event task after writing the
// points ledger entry.
func (s *Service) EvaluateBadges(ctx context.Context, tenantID, userID pgtype.UUID, activityKey string) ([]BadgeWithDetails, error) {
	if !s.badgesEnabled(ctx, tenantID) {
		return nil, nil
	}

	// Get the current activity count for this activity_key.
	currentCount, err := s.store.CountUserActivityByKey(ctx, tenantID, userID, activityKey)
	if err != nil {
		return nil, fmt.Errorf("gamification: count user activity: %w", err)
	}

	// Get all activity_count badges for this activity_key.
	badges, err := s.store.GetBadgesByActivityTrigger(ctx, tenantID, activityKey)
	if err != nil {
		return nil, fmt.Errorf("gamification: get badges by activity trigger: %w", err)
	}

	awarded := make([]BadgeWithDetails, 0)
	for _, b := range badges {
		if b.TriggerValue == nil {
			continue
		}
		threshold := int64(*b.TriggerValue)
		if currentCount < threshold {
			continue
		}

		// Check if the user already has this badge.
		_, err := s.store.GetUserBadge(ctx, tenantID, userID, b.ID)
		if err == nil {
			continue // already awarded
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("gamification: check user badge: %w", err)
		}

		// Award the badge (awarded_by = NULL = system).
		ub, err := s.store.AwardBadge(ctx, gen.AwardBadgeParams{
			TenantID: tenantID,
			UserID:   userID,
			BadgeID:  b.ID,
		})
		if err != nil {
			// ON CONFLICT DO NOTHING may return no rows — skip duplicates.
			slog.Warn("gamification: evaluate: award badge failed",
				"badge_id", uuidStr(b.ID), "error", err)
			continue
		}

		var iconID *string
		if b.IconMediaID.Valid {
			s := b.IconMediaID.String()
			iconID = &s
		}
		awarded = append(awarded, BadgeWithDetails{
			ID:          uuidStr(b.ID),
			Slug:        b.Slug,
			Name:        b.Name,
			Description: ptrStr(b.Description),
			IconMediaID: iconID,
			AwardedAt:   rfc3339(ub.AwardedAt),
		})
	}

	// Increment challenge progress for matching challenges.
	if s.challengesEnabled(ctx, tenantID) {
		if err := s.incrementChallengeProgress(ctx, tenantID, userID, activityKey); err != nil {
			slog.Error("gamification: evaluate: increment challenge progress",
				"activity_key", activityKey, "error", err)
		}
	}

	return awarded, nil
}

// incrementChallengeProgress increments progress for all active challenges
// matching the activity_key by 1.
func (s *Service) incrementChallengeProgress(ctx context.Context, tenantID, userID pgtype.UUID, activityKey string) error {
	challenges, err := s.store.GetChallengesByActivityKey(ctx, tenantID, activityKey)
	if err != nil {
		return fmt.Errorf("get challenges by activity key: %w", err)
	}

	for _, c := range challenges {
		progress, err := s.store.UpsertChallengeProgress(ctx, gen.UpsertChallengeProgressParams{
			TenantID:     tenantID,
			UserID:       userID,
			ChallengeID:  c.ID,
			CurrentCount: 1,
		})
		if err != nil {
			slog.Warn("gamification: upsert challenge progress failed",
				"challenge_id", uuidStr(c.ID), "error", err)
			continue
		}

		// If the challenge just completed, award the reward badge if set.
		if progress.Completed && progress.CompletedAt.Valid && c.RewardBadgeID.Valid {
			// Only award the badge if it wasn't already awarded (the
			// completed_at timestamp changing indicates a fresh completion).
			_, err := s.store.GetUserBadge(ctx, tenantID, userID, c.RewardBadgeID)
			if err == nil {
				continue
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				slog.Warn("gamification: check reward badge", "error", err)
				continue
			}
			if _, err := s.store.AwardBadge(ctx, gen.AwardBadgeParams{
				TenantID: tenantID,
				UserID:   userID,
				BadgeID:  c.RewardBadgeID,
			}); err != nil {
				slog.Warn("gamification: award challenge reward badge failed",
					"challenge_id", uuidStr(c.ID), "error", err)
			}
		}
	}
	return nil
}

// EvaluatePointsBadges checks lifetime_points badges against the user's
// current lifetime points and awards any newly-earned badges.
func (s *Service) EvaluatePointsBadges(ctx context.Context, tenantID, userID pgtype.UUID, currentLifetimePoints int64) ([]BadgeWithDetails, error) {
	if !s.badgesEnabled(ctx, tenantID) {
		return nil, nil
	}

	badges, err := s.store.GetBadgesByPointsTrigger(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("gamification: get points badges: %w", err)
	}

	awarded := make([]BadgeWithDetails, 0)
	for _, b := range badges {
		if b.TriggerValue == nil {
			continue
		}
		threshold := int64(*b.TriggerValue)
		if currentLifetimePoints < threshold {
			continue
		}

		_, err := s.store.GetUserBadge(ctx, tenantID, userID, b.ID)
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("gamification: check user badge: %w", err)
		}

		ub, err := s.store.AwardBadge(ctx, gen.AwardBadgeParams{
			TenantID: tenantID,
			UserID:   userID,
			BadgeID:  b.ID,
		})
		if err != nil {
			slog.Warn("gamification: evaluate points: award badge failed",
				"badge_id", uuidStr(b.ID), "error", err)
			continue
		}

		var iconID *string
		if b.IconMediaID.Valid {
			s := b.IconMediaID.String()
			iconID = &s
		}
		awarded = append(awarded, BadgeWithDetails{
			ID:          uuidStr(b.ID),
			Slug:        b.Slug,
			Name:        b.Name,
			Description: ptrStr(b.Description),
			IconMediaID: iconID,
			AwardedAt:   rfc3339(ub.AwardedAt),
		})
	}

	return awarded, nil
}

// ---------------------------------------------------------------------------
// ExpireChallenges — called by gamification:expire_challenges task
// ---------------------------------------------------------------------------

// ExpireChallenges deactivates challenges whose ends_at is in the past.
// Called daily by the gamification:expire_challenges Asynq task.
func (s *Service) ExpireChallenges(ctx context.Context, tenantID pgtype.UUID) (int64, error) {
	count, err := s.store.ExpireChallengesPastEndDate(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("gamification: expire challenges: %w", err)
	}
	if count > 0 {
		slog.Info("gamification: expired challenges",
			"tenant_id", uuidStr(tenantID), "count", count)
	}
	return count, nil
}
