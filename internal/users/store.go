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

// getPublicProfileByUsername returns the lean, PII-safe public profile row
// (with following_count) for an active user, looked up by username.
func (s *Store) getPublicProfileByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (gen.GetPublicProfileByUsernameRow, error) {
	row, err := s.read.GetPublicProfileByUsername(ctx, gen.GetPublicProfileByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
	if err != nil {
		return gen.GetPublicProfileByUsernameRow{}, fmt.Errorf("getting public profile by username: %w", err)
	}

	return row, nil
}

// upsertProfile inserts or updates the user profile.
func (s *Store) upsertProfile(ctx context.Context, tenantID, userID pgtype.UUID, displayName, avatarURL, bio, timezone *string) error {
	if err := s.write.UpsertUserProfile(ctx, gen.UpsertUserProfileParams{
		TenantID:    tenantID,
		UserID:      userID,
		DisplayName: displayName,
		AvatarUrl:   avatarURL,
		Bio:         bio,
		Timezone:    timezone,
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

func (s *Store) getUserPasswordHash(ctx context.Context, userID, tenantID pgtype.UUID) (gen.GetUserPasswordHashRow, error) {
	return s.read.GetUserPasswordHash(ctx, gen.GetUserPasswordHashParams{
		ID:       userID,
		TenantID: tenantID,
	})
}

func (s *Store) updateUserPassword(ctx context.Context, userID pgtype.UUID, passwordHash *string) error {
	return s.write.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{
		ID:           userID,
		PasswordHash: passwordHash,
	})
}

func (s *Store) getUserPrivacySettings(ctx context.Context, userID pgtype.UUID) (gen.UserPrivacySetting, error) {
	return s.read.GetUserPrivacySettings(ctx, userID)
}

func (s *Store) upsertUserPrivacySettings(ctx context.Context, userID pgtype.UUID, dmPrivacy, profileVisibility string, searchIndexable, showOnlineStatus, showActivity bool) error {
	return s.write.UpsertUserPrivacySettings(ctx, gen.UpsertUserPrivacySettingsParams{
		UserID:            userID,
		DmPrivacy:         dmPrivacy,
		ProfileVisibility: profileVisibility,
		SearchIndexable:   searchIndexable,
		ShowOnlineStatus:  showOnlineStatus,
		ShowActivity:      showActivity,
	})
}

func (s *Store) listUserSessions(ctx context.Context, userID pgtype.UUID) ([]gen.ListUserSessionsRow, error) {
	return s.read.ListUserSessions(ctx, userID)
}

func (s *Store) listUserActivity(ctx context.Context, tenantID, userID pgtype.UUID, limit int32) ([]gen.ListUserActivityRow, error) {
	return s.read.ListUserActivity(ctx, gen.ListUserActivityParams{
		TenantID: tenantID,
		AuthorID: userID,
		Limit:    limit,
	})
}

func (s *Store) countUserActivity(ctx context.Context, tenantID, userID pgtype.UUID) (int32, error) {
	return s.read.CountUserActivity(ctx, gen.CountUserActivityParams{
		TenantID: tenantID,
		AuthorID: userID,
	})
}

func (s *Store) revokeUserSession(ctx context.Context, sessionID, userID pgtype.UUID) error {
	return s.write.RevokeUserSession(ctx, gen.RevokeUserSessionParams{
		ID:     sessionID,
		UserID: userID,
	})
}

func (s *Store) revokeAllUserSessions(ctx context.Context, userID, keepCurrentID pgtype.UUID) error {
	return s.write.RevokeAllUserSessions(ctx, gen.RevokeAllUserSessionsParams{
		UserID: userID,
		ID:     keepCurrentID,
	})
}

func (s *Store) listOAuthAccounts(ctx context.Context, tenantID, userID pgtype.UUID) ([]gen.ListOAuthAccountsRow, error) {
	return s.read.ListOAuthAccounts(ctx, gen.ListOAuthAccountsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

func (s *Store) disconnectOAuthAccount(ctx context.Context, tenantID, userID pgtype.UUID, provider string) error {
	return s.write.DisconnectOAuthAccount(ctx, gen.DisconnectOAuthAccountParams{
		TenantID: tenantID,
		UserID:   userID,
		Provider: provider,
	})
}

func (s *Store) userHasPassword(ctx context.Context, userID pgtype.UUID) (bool, error) {
	return s.read.UserHasPassword(ctx, userID)
}

func (s *Store) listUserContent(ctx context.Context, tenantID, userID pgtype.UUID, status string, limit, offset int32) ([]gen.ListUserContentRow, error) {
	return s.read.ListUserContent(ctx, gen.ListUserContentParams{
		TenantID: tenantID,
		AuthorID: userID,
		Column3:  status,
		Limit:    limit,
		Offset:   offset,
	})
}

func (s *Store) listUserBookmarks(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.ListUserBookmarksRow, error) {
	return s.read.ListUserBookmarks(ctx, gen.ListUserBookmarksParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Store) deactivateUser(ctx context.Context, tenantID, userID pgtype.UUID) error {
	return s.write.DeactivateUser(ctx, gen.DeactivateUserParams{
		TenantID: tenantID,
		ID:       userID,
	})
}

// SoftDeleteUser marks a user as deleted (soft delete) without removing the row.
func (s *Store) SoftDeleteUser(ctx context.Context, tenantID, userID pgtype.UUID) error {
	return s.write.SoftDeleteUser(ctx, gen.SoftDeleteUserParams{
		TenantID: tenantID,
		ID:       userID,
	})
}
