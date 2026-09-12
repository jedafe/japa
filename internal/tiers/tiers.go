// Package tiers resolves and manages a user's effective tier from user_groups.
//
// The tier system replaces the legacy Professional Profiles feature with a
// unified regular / pro / business model. A user's effective tier is the
// highest tier among their group memberships, synced to users.profile_type
// for fast display and filtering.
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
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
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

var tierRanks = map[string]int{
	"regular":  1,
	"pro":      2,
	"business": 3,
}

// ResolveUserTier returns the user's effective tier (regular/pro/business).
func (s *Service) ResolveUserTier(ctx context.Context, tenantID, userID pgtype.UUID) (string, error) {
	rank, err := s.store.read.ResolveUserTier(ctx, gen.ResolveUserTierParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return "", fmt.Errorf("resolve user tier: %w", err)
	}

	for tier, r := range tierRanks {
		if r == int(rank) {
			return tier, nil
		}
	}

	return "regular", nil
}

// SetUserTier assigns the user to the tier group matching the requested tier
// and syncs users.profile_type. The user always remains in the base members
// group so they keep the member role and permissions.
func (s *Service) SetUserTier(ctx context.Context, tenantID, userID pgtype.UUID, tier string) error {
	if _, ok := tierRanks[tier]; !ok {
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
	if err := txStore.write.RemoveUserFromTierGroups(ctx, gen.RemoveUserFromTierGroupsParams{
		TenantID: tenantID,
		UserID:   userID,
	}); err != nil {
		return fmt.Errorf("remove tier groups: %w", err)
	}

	// Always keep the user in the base members group.
	baseID, err := txStore.read.GetTierGroupID(ctx, gen.GetTierGroupIDParams{
		TenantID: tenantID,
		Slug:     "members",
	})
	if err != nil {
		return fmt.Errorf("lookup members group: %w", err)
	}

	if err := txStore.write.AddUserToTierGroup(ctx, gen.AddUserToTierGroupParams{
		TenantID: tenantID,
		UserID:   userID,
		GroupID:  baseID,
	}); err != nil {
		return fmt.Errorf("add members group: %w", err)
	}

	// Add the target tier group unless the requested tier is the base tier.
	if tier != "regular" {
		targetID, err := txStore.read.GetTierGroupID(ctx, gen.GetTierGroupIDParams{
			TenantID: tenantID,
			Slug:     tier,
		})
		if err != nil {
			return fmt.Errorf("lookup %s group: %w", tier, err)
		}

		if err := txStore.write.AddUserToTierGroup(ctx, gen.AddUserToTierGroupParams{
			TenantID: tenantID,
			UserID:   userID,
			GroupID:  targetID,
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
func (s *Service) GetTierSetting(ctx context.Context, tenantID, userID pgtype.UUID, keyPrefix string, fallback int, sr SettingsReader) (int, error) {
	tier, err := s.ResolveUserTier(ctx, tenantID, userID)
	if err != nil {
		return 0, err
	}

	if sr == nil {
		return 0, errors.New("settings reader not configured")
	}

	return sr.GetIntOr(ctx, tenantID, fmt.Sprintf("%s.%s", keyPrefix, tier), fallback), nil
}
