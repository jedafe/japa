// Package tiers resolves and manages a user's effective tier from user_groups.
//
// The tier system replaces the legacy Professional Profiles feature with a
// unified regular / pro / business model, extended by migration 001049 with
// staff tiers (staff / admin / super_admin) ranked above business. A user's
// effective tier is the highest tier among their group memberships; the
// subscription tier is the highest of regular/pro/business only and is
// synced to users.profile_type for fast display and filtering.
//
// Ref: docs/superpowers/specs/2026-09-03-unified-user-tiers-and-professional-profile-removal-design.md
package tiers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// SettingsReader is the subset of the settings provider used to read
// per-tier limits and feature configuration.
type SettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// Service resolves and manages user tiers.
type Service struct {
	store  *Store
	db     *db.DB
	valkey valkeycompat.Cmdable
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{
		store: NewStore(database),
		db:    database,
	}
}

// SetValkey wires the Valkey client used for permission cache invalidation.
func (s *Service) SetValkey(v valkeycompat.Cmdable) {
	s.valkey = v
}

// subscriptionTierRanks are the purchasable tiers managed by SetUserTier and
// synced to users.profile_type. Staff tiers are deliberately excluded —
// they are assigned through group membership, never through subscription
// grant/revoke flows.
var subscriptionTierRanks = map[string]int{
	"regular":  1,
	"pro":      2,
	"business": 3,
}

// effectiveTierRanks adds the staff tiers (migration 001049) ranked above
// business. ResolveUserTier returns the effective tier so every tier-scoped
// quota (jce.comparisons.max_per_day.{tier}, ai.assistant.daily_queries.{tier},
// organizations.max_per_user.{tier}, etc.) honours staff overrides.
var effectiveTierRanks = map[string]int{
	"regular":     1,
	"pro":         2,
	"business":    3,
	"staff":       4,
	"admin":       5,
	"super_admin": 6,
}

// ResolveUserTier returns the user's effective tier — the highest tier across
// all tier-bearing group memberships, including staff tiers
// (regular/pro/business/staff/admin/super_admin).
func (s *Service) ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error) {
	rank, err := s.store.read.ResolveUserTier(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("resolve user tier: %w", err)
	}

	return tierNameForRank(effectiveTierRanks, rank), nil
}

// ResolveSubscriptionTier returns the user's highest subscription tier only
// (regular/pro/business), ignoring staff-tier groups. Payment grant/revoke
// flows use this so a staff rank never masks the plan state.
func (s *Service) ResolveSubscriptionTier(ctx context.Context, userID pgtype.UUID) (string, error) {
	rank, err := s.store.read.ResolveSubscriptionTier(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("resolve subscription tier: %w", err)
	}

	return tierNameForRank(subscriptionTierRanks, rank), nil
}

// tierNameForRank maps a resolved rank back to its tier name, defaulting to
// regular for unknown ranks.
func tierNameForRank(ranks map[string]int, rank int32) string {
	for tier, r := range ranks {
		if r == int(rank) {
			return tier
		}
	}

	return "regular"
}

// IsStaffTier reports whether tier is a staff tier (staff/admin/super_admin) —
// a tier granted through group membership rather than a subscription.
func IsStaffTier(tier string) bool {
	return effectiveTierRanks[tier] > subscriptionTierRanks["business"]
}

// SetUserTier assigns the user to the tier group matching the requested tier
// and syncs users.profile_type. The user always remains in the base members
// group so they keep the member role and permissions.
func (s *Service) SetUserTier(ctx context.Context, userID pgtype.UUID, tier string) error {
	if _, ok := subscriptionTierRanks[tier]; !ok {
		return fmt.Errorf("invalid tier: %s", tier)
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tier tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txStore := NewStoreFromTx(tx)

	// Remove the user from any existing tier-bearing group so the tier
	// assignment is idempotent and cannot leave stale pro/business memberships.
	if err := txStore.write.RemoveUserFromTierGroups(ctx,

		userID,
	); err != nil {
		return fmt.Errorf("remove tier groups: %w", err)
	}

	// §03 alias amendment: if the user's public role pointed at a tier group
	// just removed, clear the dangling pointer.
	if err := txStore.write.ClearPublicGroupTierRole(ctx,
		userID,
	); err != nil {
		return fmt.Errorf("clear public tier role: %w", err)
	}

	// Always keep the user in the base members group.
	baseID, err := txStore.read.GetTierGroupID(ctx,

		"members",
	)
	if err != nil {
		return fmt.Errorf("lookup members group: %w", err)
	}

	if err := txStore.write.AddUserToTierGroup(ctx, gen.AddUserToTierGroupParams{
		UserID:  userID,
		GroupID: baseID,
	}); err != nil {
		return fmt.Errorf("add members group: %w", err)
	}

	// Add the target tier group unless the requested tier is the base tier.
	if tier != "regular" {
		targetID, err := txStore.read.GetTierGroupID(ctx,

			tier,
		)
		if err != nil {
			return fmt.Errorf("lookup %s group: %w", tier, err)
		}

		if err := txStore.write.AddUserToTierGroup(ctx, gen.AddUserToTierGroupParams{
			UserID:  userID,
			GroupID: targetID,
		}); err != nil {
			return fmt.Errorf("add %s group: %w", tier, err)
		}
	}

	if err := txStore.write.UpdateUserProfileType(ctx, gen.UpdateUserProfileTypeParams{
		ID:          userID,
		ProfileType: tier,
	}); err != nil {
		return fmt.Errorf("update profile_type: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tier tx: %w", err)
	}

	// Group membership changes must invalidate the permission cache.
	if err := rbac.InvalidatePermCache(ctx, s.valkey, userID); err != nil {
		slog.Error("tiers: failed to invalidate permission cache", "error", err, "user_id", userID.String())
	}

	return nil
}

// GetTierSetting resolves the user's tier and reads keyPrefix + "." + tier
// from the settings reader, returning fallback when the per-tier key is unset.
func (s *Service) GetTierSetting(ctx context.Context, userID pgtype.UUID, keyPrefix string, fallback int, sr SettingsReader) (int, error) {
	tier, err := s.ResolveUserTier(ctx, userID)
	if err != nil {
		return 0, err
	}

	if sr == nil {
		return 0, errors.New("settings reader not configured")
	}

	return sr.GetIntOr(ctx, fmt.Sprintf("%s.%s", keyPrefix, tier), fallback), nil
}
