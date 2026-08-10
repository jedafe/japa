package users

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
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

// updateGeoSpace sets the user's current_geo_space_id (ADR-064a).
func (s *Store) updateGeoSpace(ctx context.Context, tenantID, userID, spaceID pgtype.UUID) error {
	return s.write.UpdateUserGeoSpace(ctx, gen.UpdateUserGeoSpaceParams{
		CurrentGeoSpaceID: spaceID,
		ID:                userID,
		TenantID:          tenantID,
	})
}

// getCurrentGeoSpace resolves the user's current geo space hierarchy
// (city → country) for the GET /me/location endpoint (ADR-064a).
func (s *Store) getCurrentGeoSpace(ctx context.Context, tenantID, userID pgtype.UUID) (gen.GetUserCurrentGeoSpaceRow, error) {
	return s.read.GetUserCurrentGeoSpace(ctx, gen.GetUserCurrentGeoSpaceParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}

// findCitySpace finds a city-level Space by name (case-insensitive) for
// the reverse-geocoding flow (ADR-064a).
func (s *Store) findCitySpace(ctx context.Context, tenantID pgtype.UUID, cityName string) (pgtype.UUID, error) {
	space, err := s.read.FindCitySpaceByName(ctx, gen.FindCitySpaceByNameParams{
		CityName: cityName, TenantID: tenantID,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return space.ID, nil
}

// findCountrySpace finds a country-level Space by geo_code (ISO 3166-1
// alpha-2) for the reverse-geocoding fallback (ADR-064a).

// findStateSpace finds a state-level Space by name (case-insensitive) for
// the reverse-geocoding fallback (ADR-064a, BUG-006). Used when no city-
// level Space matches but a state/county name is available from Nominatim.
func (s *Store) findStateSpace(ctx context.Context, tenantID pgtype.UUID, stateName string) (pgtype.UUID, error) {
	space, err := s.read.FindStateSpaceByName(ctx, gen.FindStateSpaceByNameParams{
		StateName: stateName, TenantID: tenantID,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return space.ID, nil
}

// findCountrySpace finds a country-level Space by geo_code (ISO 3166-1
// alpha-2) for the reverse-geocoding fallback (ADR-064a).
func (s *Store) findCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error) {
	code := countryCode
	space, err := s.read.GetSpaceByGeoCode(ctx, gen.GetSpaceByGeoCodeParams{
		GeoCode:  &code,
		TenantID: tenantID,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return space.ID, nil
}

// isValidGeoSpace checks that a space exists, belongs to the tenant, and is
// a geo space (level = country/state/city). Used by the POST /me/location
// handler to validate direct space_id selections (ADR-064a security check).
func (s *Store) isValidGeoSpace(ctx context.Context, tenantID, spaceID pgtype.UUID) bool {
	space, err := s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{
		ID:       spaceID,
		TenantID: tenantID,
	})
	if err != nil {
		return false
	}
	switch space.Level {
	case "country", "state", "city":
		return true
	default:
		return false
	}
}

// setGeoCustomField upserts a geo-related custom field value (origin_country
// or destination_country) on the user's profile (ADR-064a). It resolves the
// custom field ID by key, fetches the user_profiles.id, and upserts the value.
// An empty value clears the field by setting value_text to NULL.
func (s *Store) setGeoCustomField(ctx context.Context, tenantID, userID pgtype.UUID, fieldKey, value string) error {
	field, err := s.read.GetCustomFieldByKey(ctx, gen.GetCustomFieldByKeyParams{
		Key:      fieldKey,
		TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("looking up custom field %q: %w", fieldKey, err)
	}

	profileID, err := s.read.GetUserProfileID(ctx, gen.GetUserProfileIDParams{
		TenantID: tenantID, UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("getting user profile id: %w", err)
	}

	var valueText *string
	if value != "" {
		v := value
		valueText = &v
	}

	_, err = s.write.UpsertCustomFieldValue(ctx, gen.UpsertCustomFieldValueParams{
		FieldID:    field.ID,
		EntityType: "user_profile",
		EntityID:   profileID,
		ValueText:  valueText,
	})
	if err != nil {
		return fmt.Errorf("upserting custom field value for %q: %w", fieldKey, err)
	}

	return nil
}

// getGeoCustomField reads a geo-related custom field value (origin_country
// or destination_country) from the user's profile (ADR-064a). Returns an
// empty string when the field has no value set (no row / NULL value_text).
func (s *Store) getGeoCustomField(ctx context.Context, tenantID, userID pgtype.UUID, fieldKey string) (string, error) {
	field, err := s.read.GetCustomFieldByKey(ctx, gen.GetCustomFieldByKeyParams{
		Key:      fieldKey,
		TenantID: tenantID,
	})
	if err != nil {
		return "", fmt.Errorf("looking up custom field %q: %w", fieldKey, err)
	}

	profileID, err := s.read.GetUserProfileID(ctx, gen.GetUserProfileIDParams{
		TenantID: tenantID, UserID: userID,
	})
	if err != nil {
		return "", fmt.Errorf("getting user profile id: %w", err)
	}

	val, err := s.read.GetCustomFieldValue(ctx, gen.GetCustomFieldValueParams{
		FieldID:    field.ID,
		EntityType: "user_profile",
		EntityID:   profileID,
		TenantID:   tenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil // no value set yet
		}
		return "", fmt.Errorf("getting custom field value for %q: %w", fieldKey, err)
	}
	if val.ValueText != nil {
		return *val.ValueText, nil
	}
	return "", nil
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

// listPublicUsers returns a paginated, PII-safe list of active, search-indexable
// users for the public directory at GET /users. Filters by username or display_name
// and optionally by profile_type. Ref: japaconnect-engineering/sections/02-authentication-sessions.md
//      japaconnect-engineering/sections/19-privacy-gdpr.md (search_indexable opt-out)
func (s *Store) listPublicUsers(ctx context.Context, tenantID pgtype.UUID, search, profileType string, limit, offset int32) ([]gen.ListPublicUsersRow, error) {
	rows, err := s.read.ListPublicUsers(ctx, gen.ListPublicUsersParams{
		TenantID:    tenantID,
		Search:      search,
		ProfileType: profileType,
		PageLimit:   limit,
		PageOffset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing public users: %w", err)
	}
	return rows, nil
}

// countPublicUsers returns the total count of active, search-indexable users
// matching the search term and profile_type filter, for pagination of the
// public directory.
func (s *Store) countPublicUsers(ctx context.Context, tenantID pgtype.UUID, search, profileType string) (int32, error) {
	count, err := s.read.CountPublicUsers(ctx, gen.CountPublicUsersParams{
		TenantID:    tenantID,
		Search:      search,
		ProfileType: profileType,
	})
	if err != nil {
		return 0, fmt.Errorf("counting public users: %w", err)
	}
	return count, nil
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

// insertPasswordHistory records a password hash in the password_history table
// so it can be checked against future password changes (auth.password.history_count).
func (s *Store) insertPasswordHistory(ctx context.Context, tenantID, userID pgtype.UUID, passwordHash string) error {
	return s.write.InsertPasswordHistory(ctx, gen.InsertPasswordHistoryParams{
		TenantID:     tenantID,
		UserID:       userID,
		PasswordHash: passwordHash,
	})
}

// GetRecentPasswordHashes returns the most recent N password hashes for a user,
// ordered by creation time descending. Used to prevent password reuse.
// Implements auth.PasswordHistoryReader so ChangePassword can reuse the auth
// package's CheckPasswordHistory helper.
func (s *Store) GetRecentPasswordHashes(ctx context.Context, userID pgtype.UUID, limit int) ([]string, error) {
	return s.read.GetRecentPasswordHashes(ctx, gen.GetRecentPasswordHashesParams{
		UserID:    userID,
		HashLimit: int32(limit),
	})
}

// revokeAllRefreshTokens revokes all non-revoked refresh tokens for a user,
// forcing re-authentication on all devices. Used after password change/reset.
func (s *Store) revokeAllRefreshTokens(ctx context.Context, tenantID, userID pgtype.UUID) error {
	return s.write.RevokeAllUserRefreshTokens(ctx, gen.RevokeAllUserRefreshTokensParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

func (s *Store) getUserPrivacySettings(ctx context.Context, tenantID, userID pgtype.UUID) (gen.GetUserPrivacySettingsRow, error) {
	return s.read.GetUserPrivacySettings(ctx, gen.GetUserPrivacySettingsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

func (s *Store) upsertUserPrivacySettings(ctx context.Context, tenantID, userID pgtype.UUID, dmPrivacy, profileVisibility string, searchIndexable, showOnlineStatus, showActivity, showPointsPublicly bool) error {
	return s.write.UpsertUserPrivacySettings(ctx, gen.UpsertUserPrivacySettingsParams{
		TenantID:           tenantID,
		UserID:             userID,
		DmPrivacy:          dmPrivacy,
		ProfileVisibility:  profileVisibility,
		SearchIndexable:    searchIndexable,
		ShowOnlineStatus:   showOnlineStatus,
		ShowActivity:       showActivity,
		ShowPointsPublicly: showPointsPublicly,
	})
}

func (s *Store) listUserSessions(ctx context.Context, tenantID, userID pgtype.UUID) ([]gen.ListUserSessionsRow, error) {
	return s.read.ListUserSessions(ctx, gen.ListUserSessionsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

// getSessionIDByHash resolves the refresh_tokens.id for the current session
// from the jc_refresh cookie hash. Returns pgtype.UUID{} (zero) when the
// token is not found so the caller can degrade gracefully.
func (s *Store) getSessionIDByHash(ctx context.Context, tenantID, userID pgtype.UUID, tokenHash string) (pgtype.UUID, error) {
	return s.read.GetSessionIDByHash(ctx, gen.GetSessionIDByHashParams{
		TenantID:  tenantID,
		UserID:    userID,
		TokenHash: tokenHash,
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

func (s *Store) listUserContent(ctx context.Context, tenantID, userID pgtype.UUID, status, contentType, q string, limit, offset int32) ([]gen.ListUserContentRow, error) {
	return s.read.ListUserContent(ctx, gen.ListUserContentParams{
		TenantID:    tenantID,
		AuthorID:    userID,
		Status:      status,
		ContentType: contentType,
		Q:           q,
		PageLimit:   limit,
		PageOffset:  offset,
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

// getUserContent returns the caller's own content by ID, scoped to the
// tenant and author with no status filter (covers published, scheduled, and
// draft). Used by GET /me/content/{id} so the /create composer can restore
// any of the caller's own content for editing. Returns pgx.ErrNoRows when
// the content is missing or owned by another user/tenant → 404.
func (s *Store) getUserContent(ctx context.Context, tenantID, userID, contentID pgtype.UUID) (gen.GetUserContentRow, error) {
	return s.read.GetUserContent(ctx, gen.GetUserContentParams{
		ID:       contentID,
		TenantID: tenantID,
		AuthorID: userID,
	})
}

// getContentByIDForEditor returns any content by ID within the tenant (no
// author filter). Used by GET /me/content/{id} as a fallback when the caller
// is a super-admin or holds the RBAC `content.edit` permission but is not
// the content owner. Returns pgx.ErrNoRows when the content is missing or
// belongs to a different tenant → 404.
func (s *Store) getContentByIDForEditor(ctx context.Context, tenantID, contentID pgtype.UUID) (gen.GetContentByIDForEditorRow, error) {
	return s.read.GetContentByIDForEditor(ctx, gen.GetContentByIDForEditorParams{
		ID:       contentID,
		TenantID: tenantID,
	})
}

func (s *Store) listUserBookmarks(ctx context.Context, tenantID, userID pgtype.UUID, folderID pgtype.UUID, entityTypes []string, limit, offset int32) ([]gen.ListUserBookmarksRow, error) {
	return s.read.ListUserBookmarks(ctx, gen.ListUserBookmarksParams{
		UserID:      userID,
		TenantID:    tenantID,
		FolderID:    folderID,
		EntityTypes: entityTypes,
		Limit:       limit,
		Offset:      offset,
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

// countUserFollowStats returns the follower and following counts for the user.
// Ref: japaconnect-engineering/sections/31-follow-system.md (denormalized counts)
func (s *Store) countUserFollowStats(ctx context.Context, tenantID, userID pgtype.UUID) (gen.CountUserFollowStatsRow, error) {
	return s.read.CountUserFollowStats(ctx, gen.CountUserFollowStatsParams{
		UserID:   userID,
		TenantID: tenantID,
	})
}
