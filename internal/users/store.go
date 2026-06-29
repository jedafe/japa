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

// getUserByUsername returns the user profile joined with users table data, looked up by username.
func (s *Store) getUserByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (gen.GetUserByUsernameRow, error) {
	row, err := s.read.GetUserByUsername(ctx, gen.GetUserByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
	if err != nil {
		return gen.GetUserByUsernameRow{}, fmt.Errorf("getting user by username: %w", err)
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

// listUsers returns a paginated list of users for a tenant, optionally filtered by search.
func (s *Store) listUsers(ctx context.Context, tenantID pgtype.UUID, search string, limit, offset int32) ([]gen.ListUsersRow, error) {
	rows, err := s.read.ListUsers(ctx, gen.ListUsersParams{
		TenantID:   tenantID,
		Search:     search,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	return rows, nil
}

// countUsers returns the total count of users for a tenant, optionally filtered by search.
func (s *Store) countUsers(ctx context.Context, tenantID pgtype.UUID, search string) (int32, error) {
	count, err := s.read.CountUsers(ctx, gen.CountUsersParams{
		TenantID: tenantID,
		Search:   search,
	})
	if err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}
