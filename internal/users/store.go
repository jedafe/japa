package users

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for user profiles and preferences.
// The Service layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// getProfile returns the user profile joined with users table data.
func (s *Store) getProfile(ctx context.Context, tenantID, userID pgtype.UUID) (gen.GetUserProfileRow, error) {
	row, err := s.read.GetUserProfile(ctx, gen.GetUserProfileParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return gen.GetUserProfileRow{}, fmt.Errorf("getting user profile: %w", err)
	}
	return row, nil
}

// getUser returns core user data from the users table.
func (s *Store) getUser(ctx context.Context, tenantID, userID pgtype.UUID) (gen.GetUserByIDRow, error) {
	row, err := s.read.GetUserByID(ctx, gen.GetUserByIDParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return gen.GetUserByIDRow{}, fmt.Errorf("getting user: %w", err)
	}
	return row, nil
}

// upsertProfile inserts or updates the user profile.
func (s *Store) upsertProfile(ctx context.Context, tenantID, userID pgtype.UUID, displayName, avatarURL, bio, timezone *string) error {
	if err := s.write.UpsertUserProfile(ctx, gen.UpsertUserProfileParams{
		TenantID:   tenantID,
		UserID:     userID,
		DisplayName: displayName,
		AvatarUrl:  avatarURL,
		Bio:        bio,
		Timezone:   timezone,
	}); err != nil {
		return fmt.Errorf("upserting user profile: %w", err)
	}
	return nil
}

// getPreferences returns user preferences.
func (s *Store) getPreferences(ctx context.Context, tenantID, userID pgtype.UUID) (gen.UserPreference, error) {
	pref, err := s.read.GetUserPreferences(ctx, gen.GetUserPreferencesParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return gen.UserPreference{}, fmt.Errorf("getting user preferences: %w", err)
	}
	return pref, nil
}

// upsertPreferences inserts or updates user preferences.
func (s *Store) upsertPreferences(ctx context.Context, tenantID, userID pgtype.UUID, darkMode, language string) error {
	if err := s.write.UpsertUserPreferences(ctx, gen.UpsertUserPreferencesParams{
		TenantID: tenantID,
		UserID:   userID,
		DarkMode: darkMode,
		Language: language,
	}); err != nil {
		return fmt.Errorf("upserting user preferences: %w", err)
	}
	return nil
}
