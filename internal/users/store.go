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

// SetContentCountryPreference updates the user's content_country_preference.
// An empty country clears the preference (falls back to GeoIP).
func (s *Store) SetContentCountryPreference(ctx context.Context, tenantID, userID pgtype.UUID, country string) error {
	var pref *string
	if country != "" {
		c := country
		pref = &c
	}
	return s.write.SetUserContentCountryPreference(ctx, gen.SetUserContentCountryPreferenceParams{
		TenantID:                 tenantID,
		UserID:                   userID,
		ContentCountryPreference: pref,
	})
}

// GetContentCountryPreference returns the user's content_country_preference.
func (s *Store) GetContentCountryPreference(ctx context.Context, tenantID, userID pgtype.UUID) (string, error) {
	pref, err := s.read.GetUserContentCountryPreference(ctx, gen.GetUserContentCountryPreferenceParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return "", err
	}
	if pref == nil {
		return "", nil
	}
	return *pref, nil
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
func (s *Store) upsertProfile(ctx context.Context, tenantID, userID pgtype.UUID, displayName, avatarURL, coverURL, bio, timezone *string, avatarMediaID, headerMediaID pgtype.UUID) error {
	if err := s.write.UpsertUserProfile(ctx, gen.UpsertUserProfileParams{
		TenantID:      tenantID,
		UserID:        userID,
		DisplayName:   displayName,
		AvatarUrl:     avatarURL,
		AvatarMediaID: avatarMediaID,
		HeaderMediaID: headerMediaID,
		CoverUrl:      coverURL,
		Bio:           bio,
		Timezone:      timezone,
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

// listUsers returns a paginated list of users for a tenant, optionally filtered by search, status, and group.
func (s *Store) listUsers(ctx context.Context, tenantID pgtype.UUID, search, status, groupSlug string, limit, offset int32) ([]gen.ListUsersRow, error) {
	rows, err := s.read.ListUsers(ctx, gen.ListUsersParams{
		TenantID:   tenantID,
		Search:     search,
		Status:     status,
		GroupSlug:  groupSlug,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	return rows, nil
}

// countUsers returns the total count of users for a tenant, optionally filtered by search, status, and group.
func (s *Store) countUsers(ctx context.Context, tenantID pgtype.UUID, search, status, groupSlug string) (int32, error) {
	count, err := s.read.CountUsers(ctx, gen.CountUsersParams{
		TenantID:  tenantID,
		Search:    search,
		Status:    status,
		GroupSlug: groupSlug,
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

func (s *Store) updateUserPassword(ctx context.Context, tenantID, userID pgtype.UUID, passwordHash *string) error {
	return s.write.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{
		ID:           userID,
		TenantID:     tenantID,
		PasswordHash: passwordHash,
	})
}

func (s *Store) getUserPrivacySettings(ctx context.Context, tenantID, userID pgtype.UUID) (gen.GetUserPrivacySettingsRow, error) {
	return s.read.GetUserPrivacySettings(ctx, gen.GetUserPrivacySettingsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

func (s *Store) upsertUserPrivacySettings(ctx context.Context, tenantID, userID pgtype.UUID, dmPrivacy, profileVisibility string, searchIndexable, showOnlineStatus, showActivity bool) error {
	return s.write.UpsertUserPrivacySettings(ctx, gen.UpsertUserPrivacySettingsParams{
		TenantID:          tenantID,
		UserID:            userID,
		DmPrivacy:         dmPrivacy,
		ProfileVisibility: profileVisibility,
		SearchIndexable:   searchIndexable,
		ShowOnlineStatus:  showOnlineStatus,
		ShowActivity:      showActivity,
	})
}

func (s *Store) listUserSessions(ctx context.Context, tenantID, userID pgtype.UUID) ([]gen.ListUserSessionsRow, error) {
	return s.read.ListUserSessions(ctx, gen.ListUserSessionsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
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

func (s *Store) revokeUserSession(ctx context.Context, tenantID, sessionID, userID pgtype.UUID) error {
	return s.write.RevokeUserSession(ctx, gen.RevokeUserSessionParams{
		ID:       sessionID,
		TenantID: tenantID,
		UserID:   userID,
	})
}

func (s *Store) revokeAllUserSessions(ctx context.Context, tenantID, userID, keepCurrentID pgtype.UUID) error {
	return s.write.RevokeAllUserSessions(ctx, gen.RevokeAllUserSessionsParams{
		TenantID: tenantID,
		UserID:   userID,
		ID:       keepCurrentID,
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

func (s *Store) userHasPassword(ctx context.Context, tenantID, userID pgtype.UUID) (bool, error) {
	return s.read.UserHasPassword(ctx, gen.UserHasPasswordParams{
		ID:       userID,
		TenantID: tenantID,
	})
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

// getUserDraft returns the caller's own draft content by ID, scoped to the
// tenant and author and restricted to status='draft'. Returns pgx.ErrNoRows
// when the content does not exist, is not a draft, or belongs to another
// user/tenant — the handler maps that to 404.
func (s *Store) getUserDraft(ctx context.Context, tenantID, userID, contentID pgtype.UUID) (gen.GetUserDraftRow, error) {
	return s.read.GetUserDraft(ctx, gen.GetUserDraftParams{
		ID:       contentID,
		TenantID: tenantID,
		AuthorID: userID,
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

// activeUserExistsByID reports whether an ACTIVE user with this id exists in
// the tenant. Used by the public profile gate.
func (s *Store) activeUserExistsByID(ctx context.Context, tenantID, userID pgtype.UUID) (bool, error) {
	return s.read.ActiveUserExistsByID(ctx, gen.ActiveUserExistsByIDParams{
		TenantID: tenantID,
		ID:       userID,
	})
}

// getUserContentAnalytics returns per-content analytics for the user.
func (s *Store) getUserContentAnalytics(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.GetUserContentAnalyticsRow, error) {
	return s.read.GetUserContentAnalytics(ctx, gen.GetUserContentAnalyticsParams{
		TenantID: tenantID,
		AuthorID: userID,
		Limit:    limit,
		Offset:   offset,
	})
}

// getUserFollowerGrowth returns daily follower growth for the user.
func (s *Store) getUserFollowerGrowth(ctx context.Context, tenantID, userID pgtype.UUID, startDate, endDate *pgtype.Date) ([]gen.GetUserFollowerGrowthRow, error) {
	return s.read.GetUserFollowerGrowth(ctx, gen.GetUserFollowerGrowthParams{
		TenantID: tenantID,
		EntityID: userID,
		Column3:  derefDate(startDate),
		Column4:  derefDate(endDate),
	})
}

func derefDate(d *pgtype.Date) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return *d
}

// ---------------------------------------------------------------------------
// Account management (Section 50: PATCH /me/account)
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
// ---------------------------------------------------------------------------

// getUserAccountForUpdate row-locks the user and returns the fields needed
// to validate an account update (username cooldown, email uniqueness).
func (s *Store) getUserAccountForUpdate(ctx context.Context, tenantID, userID pgtype.UUID) (gen.GetUserAccountForUpdateRow, error) {
	return s.write.GetUserAccountForUpdate(ctx, gen.GetUserAccountForUpdateParams{
		ID:       userID,
		TenantID: tenantID,
	})
}

// updateUsername updates the username and stamps username_changed_at.
func (s *Store) updateUsername(ctx context.Context, tenantID, userID pgtype.UUID, username string) error {
	return s.write.UpdateUserUsername(ctx, gen.UpdateUserUsernameParams{
		ID:       userID,
		TenantID: tenantID,
		Username: username,
	})
}

// updateEmail updates the email and resets email_verified_at to NULL.
func (s *Store) updateEmail(ctx context.Context, tenantID, userID pgtype.UUID, email string) error {
	return s.write.UpdateUserEmail(ctx, gen.UpdateUserEmailParams{
		ID:       userID,
		TenantID: tenantID,
		Email:    email,
	})
}

// emailExistsForTenant returns the count of other users in the tenant with
// the given email (0 = available).
func (s *Store) emailExistsForTenant(ctx context.Context, tenantID, userID pgtype.UUID, email string) (int32, error) {
	return s.write.EmailExistsForTenant(ctx, gen.EmailExistsForTenantParams{
		TenantID: tenantID,
		Email:    email,
		ID:       userID,
	})
}

// usernameExistsForTenant returns the count of other users in the tenant with
// the given username (0 = available).
func (s *Store) usernameExistsForTenant(ctx context.Context, tenantID, userID pgtype.UUID, username string) (int32, error) {
	return s.write.UsernameExistsForTenant(ctx, gen.UsernameExistsForTenantParams{
		TenantID: tenantID,
		Username: username,
		ID:       userID,
	})
}

// ---------------------------------------------------------------------------
// Analytics counts (Section 40: BUG-012 — SQL COUNT instead of row iteration)
// ---------------------------------------------------------------------------

// countUserContent returns total/draft/published content counts for the user.
func (s *Store) countUserContent(ctx context.Context, tenantID, userID pgtype.UUID) (gen.CountUserContentRow, error) {
	return s.read.CountUserContent(ctx, gen.CountUserContentParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

// countUserBookmarks returns the total bookmark count for the user.
func (s *Store) countUserBookmarks(ctx context.Context, tenantID, userID pgtype.UUID) (int32, error) {
	return s.read.CountUserBookmarks(ctx, gen.CountUserBookmarksParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}
