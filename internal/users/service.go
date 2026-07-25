package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service provides user profile and preference operations with reader/writer
// pool separation. Business logic lives here; the Store handles raw queries.
type Service struct {
	store *Store
	db    *db.DB
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db: database,
	}
}

// GetProfile returns the user profile for the given user within a tenant.
func (s *Service) GetProfile(ctx context.Context, tenantID, userID pgtype.UUID) (*UserProfileResponse, error) {
	p, err := s.store.getProfile(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("getting profile: %w", err)
	}

	groupName := ""
	if p.UserGroupName != nil {
		groupName = *p.UserGroupName
	}

	// Fetch all groups for this user.
	groupRows, err := s.store.read.ResolveUserGroups(ctx, gen.ResolveUserGroupsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	groups := make([]string, 0, len(groupRows))
	if err == nil {
		for _, g := range groupRows {
			groups = append(groups, g.Name)
		}
	}

	var lastLogin *string
	if p.LastLoginAt.Valid {
		s := p.LastLoginAt.Time.Format(time.RFC3339)
		lastLogin = &s
	}

	return &UserProfileResponse{
		ID:            p.UserID.String(),
		Email:         p.Email,
		Username:      p.Username,
		DisplayName:   p.DisplayName,
		AvatarURL:     p.AvatarUrl,
		Bio:           p.Bio,
		Timezone:      p.Timezone,
		ProfileType:   p.ProfileType,
		Status:        p.Status,
		FollowerCount: p.FollowerCount,
		UserGroupName: groupName,
		Groups:        groups,
		LastLoginAt:   lastLogin,
		CreatedAt:     p.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:     p.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

// GetProfileByUsername returns the user profile for the given username within a tenant.
func (s *Service) GetProfileByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (*UserProfileResponse, error) {
	p, err := s.store.getUserByUsername(ctx, tenantID, username)
	if err != nil {
		return nil, fmt.Errorf("getting profile by username: %w", err)
	}

	groupName := ""
	if p.UserGroupName != nil {
		groupName = *p.UserGroupName
	}

	return &UserProfileResponse{
		ID:            p.UserID.String(),
		Email:         p.Email,
		Username:      p.Username,
		DisplayName:   p.DisplayName,
		AvatarURL:     p.AvatarUrl,
		Bio:           p.Bio,
		Timezone:      p.Timezone,
		ProfileType:   p.ProfileType,
		Status:        p.Status,
		FollowerCount: p.FollowerCount,
		UserGroupName: groupName,
		CreatedAt:     p.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:     p.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

// GetPublicProfileByUsername returns the lean, PII-safe public profile for
// the given username within a tenant, restricted to active users. It never
// exposes email, status, last_login_at, or groups (Section 19 / ADR-014).
func (s *Service) GetPublicProfileByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (*PublicProfileResponse, error) {
	p, err := s.store.getPublicProfileByUsername(ctx, tenantID, username)
	if err != nil {
		return nil, fmt.Errorf("getting public profile by username: %w", err)
	}

	return &PublicProfileResponse{
		ID:             p.UserID.String(),
		Username:       p.Username,
		DisplayName:    p.DisplayName,
		AvatarURL:      p.AvatarUrl,
		Bio:            p.Bio,
		ProfileType:    p.ProfileType,
		FollowerCount:  p.FollowerCount,
		FollowingCount: p.FollowingCount,
		CreatedAt:      p.CreatedAt.Time.Format(time.RFC3339),
	}, nil
}

// UpdateProfile updates the user profile fields. Only non-nil fields are
// overwritten; nil fields are left unchanged via a read-then-merge strategy.
func (s *Service) UpdateProfile(ctx context.Context, tenantID, userID pgtype.UUID, req *UpdateProfileRequest) error {
	// Read current profile to preserve existing values for omitted fields.
	current, err := s.store.getProfile(ctx, tenantID, userID)
	if err != nil {
		return fmt.Errorf("reading current profile: %w", err)
	}

	// Merge: use request value when provided, otherwise keep existing.
	displayName := current.DisplayName
	if req.DisplayName != nil {
		displayName = req.DisplayName
	}

	avatarURL := current.AvatarUrl
	if req.AvatarURL != nil {
		avatarURL = req.AvatarURL
	}

	bio := current.Bio
	if req.Bio != nil {
		bio = req.Bio
	}

	timezone := current.Timezone
	if req.Timezone != nil {
		timezone = req.Timezone
	}

	if err := s.store.upsertProfile(ctx, tenantID, userID, displayName, avatarURL, bio, timezone); err != nil {
		return fmt.Errorf("updating profile: %w", err)
	}

	return nil
}

// GetPreferences returns user preferences. If no preferences exist yet,
// returns the system defaults (dark_mode='system', language='en-NG').
func (s *Service) GetPreferences(ctx context.Context, tenantID, userID pgtype.UUID) (*UserPreferencesResponse, error) {
	pref, err := s.store.getPreferences(ctx, tenantID, userID)
	if err != nil {
		// If no preferences exist yet, return defaults.
		return &UserPreferencesResponse{ //nolint:nilerr // intentional: missing prefs return defaults
			DarkMode: "system",
			Language: "en-NG",
		}, nil
	}

	return &UserPreferencesResponse{
		DarkMode: pref.DarkMode,
		Language: pref.Language,
	}, nil
}

// UpdatePreferences upserts user preferences. Only non-nil fields are
// overwritten; nil fields are left unchanged via a read-then-merge strategy.
func (s *Service) UpdatePreferences(ctx context.Context, tenantID, userID pgtype.UUID, req *UpdatePreferencesRequest) error {
	// Read current preferences to preserve existing values for omitted fields.
	current, err := s.store.getPreferences(ctx, tenantID, userID)
	darkMode := "system"
	language := "en-NG"

	if err == nil {
		darkMode = current.DarkMode
		language = current.Language
	}

	// Merge: use request value when provided, otherwise keep existing/default.
	if req.DarkMode != nil {
		darkMode = *req.DarkMode
	}

	if req.Language != nil {
		language = *req.Language
	}

	if err := s.store.upsertPreferences(ctx, tenantID, userID, darkMode, language); err != nil {
		return fmt.Errorf("updating preferences: %w", err)
	}

	return nil
}

// ListUsers returns a paginated list of users for admin use, with optional
// search filtering by username/email, status filter, and group slug filter.
func (s *Service) ListUsers(ctx context.Context, tenantID pgtype.UUID, search, status, groupSlug string, limit, offset int32) (*ListUsersResponse, error) {
	if limit <= 0 {
		limit = 20
	}

	if offset < 0 {
		offset = 0
	}

	users, err := s.store.listUsers(ctx, tenantID, search, status, groupSlug, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	total, err := s.store.countUsers(ctx, tenantID, search, status, groupSlug)
	if err != nil {
		return nil, fmt.Errorf("counting users: %w", err)
	}

	items := make([]UserListItem, len(users))
	for i, u := range users {
		groups := parseGroups(u.Groups)
		items[i] = UserListItem{
			ID:            u.ID.String(),
			Email:         u.Email,
			Username:      u.Username,
			ProfileType:   u.ProfileType,
			Status:        parseString(u.Status),
			FollowerCount: u.FollowerCount,
			Groups:        groups,
			CreatedAt:     u.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:     u.UpdatedAt.Time.Format(time.RFC3339),
		}
	}

	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}

	return &ListUsersResponse{
		Items:      items,
		Page:       offset / limit,
		TotalPages: totalPages,
		Total:      total,
	}, nil
}

// parseGroups extracts group names from the string_agg result.
// The SQL COALESCE returns "" for users with no groups.
func parseGroups(v interface{}) []string {
	if v == nil {
		return []string{}
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return []string{}
	}
	return strings.Split(s, ", ")
}

// parseString converts an interface{} to string (for CASE expressions).
func parseString(v interface{}) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// GetPrivacySettings returns the user's privacy settings.
func (s *Service) GetPrivacySettings(ctx context.Context, userID pgtype.UUID) (*PrivacySettings, error) {
	settings, err := s.store.getUserPrivacySettings(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &PrivacySettings{
				DmPrivacy:         "everyone",
				ProfileVisibility: "public",
				SearchIndexable:   true,
				ShowOnlineStatus:  true,
				ShowActivity:      true,
			}, nil
		}
		return nil, fmt.Errorf("getting privacy settings: %w", err)
	}

	return &PrivacySettings{
		DmPrivacy:         settings.DmPrivacy,
		ProfileVisibility: settings.ProfileVisibility,
		SearchIndexable:   settings.SearchIndexable,
		ShowOnlineStatus:  settings.ShowOnlineStatus,
		ShowActivity:      settings.ShowActivity,
	}, nil
}

// UpdatePrivacySettings updates the user's privacy settings.
func (s *Service) UpdatePrivacySettings(ctx context.Context, userID pgtype.UUID, req *UpdatePrivacyRequest) error {
	current, err := s.GetPrivacySettings(ctx, userID)
	if err != nil {
		return fmt.Errorf("reading current settings: %w", err)
	}

	dmPrivacy := current.DmPrivacy
	if req.DmPrivacy != nil {
		dmPrivacy = *req.DmPrivacy
	}

	profileVisibility := current.ProfileVisibility
	if req.ProfileVisibility != nil {
		profileVisibility = *req.ProfileVisibility
	}

	searchIndexable := current.SearchIndexable
	if req.SearchIndexable != nil {
		searchIndexable = *req.SearchIndexable
	}

	showOnlineStatus := current.ShowOnlineStatus
	if req.ShowOnlineStatus != nil {
		showOnlineStatus = *req.ShowOnlineStatus
	}

	showActivity := current.ShowActivity
	if req.ShowActivity != nil {
		showActivity = *req.ShowActivity
	}

	return s.store.upsertUserPrivacySettings(ctx, userID, dmPrivacy, profileVisibility, searchIndexable, showOnlineStatus, showActivity)
}

// ListSessions returns the user's active sessions.
func (s *Service) ListSessions(ctx context.Context, userID pgtype.UUID) ([]SessionInfo, error) {
	sessions, err := s.store.listUserSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}

	result := make([]SessionInfo, len(sessions))
	for i, sess := range sessions {
		result[i] = SessionInfo{
			ID:        sess.ID.String(),
			CreatedAt: sess.CreatedAt.Time.Format(time.RFC3339),
			ExpiresAt: sess.ExpiresAt.Time.Format(time.RFC3339),
		}
	}

	return result, nil
}

// ListUserActivity returns a merged feed of the user's authored content and
// comments, newest first, plus the total count across both sources.
func (s *Service) ListUserActivity(ctx context.Context, tenantID, userID pgtype.UUID, limit int32) (*UserActivityResponse, error) {
	rows, err := s.store.listUserActivity(ctx, tenantID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing user activity: %w", err)
	}

	total, err := s.store.countUserActivity(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("counting user activity: %w", err)
	}

	items := make([]UserActivityItem, len(rows))
	for i, row := range rows {
		items[i] = UserActivityItem{
			Type:      row.Type,
			Content:   row.Content,
			CreatedAt: row.CreatedAt.Time.Format(time.RFC3339),
		}
	}

	return &UserActivityResponse{Activities: items, Total: total}, nil
}

// RevokeSession revokes a specific session.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID pgtype.UUID) error {
	return s.store.revokeUserSession(ctx, sessionID, userID)
}

// RevokeAllSessions revokes all sessions, optionally keeping the current one.
func (s *Service) RevokeAllSessions(ctx context.Context, userID pgtype.UUID, keepCurrent bool) error {
	if keepCurrent {
		// Find the most recently created session to keep.
		sessions, err := s.store.listUserSessions(ctx, userID)
		if err != nil {
			return fmt.Errorf("listing sessions: %w", err)
		}
		if len(sessions) > 0 {
			return s.store.revokeAllUserSessions(ctx, userID, sessions[0].ID)
		}
	}
	// Revoke all by passing a zero UUID (won't match any real session).
	return s.store.revokeAllUserSessions(ctx, userID, pgtype.UUID{})
}

// ListConnectedAccounts returns the user's linked OAuth accounts.
func (s *Service) ListConnectedAccounts(ctx context.Context, tenantID, userID pgtype.UUID) ([]ConnectedAccountInfo, error) {
	accounts, err := s.store.listOAuthAccounts(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("listing connected accounts: %w", err)
	}

	result := make([]ConnectedAccountInfo, len(accounts))
	for i, acc := range accounts {
		result[i] = ConnectedAccountInfo{
			Provider:    acc.Provider,
			ConnectedAt: acc.CreatedAt.Time.Format(time.RFC3339),
		}
	}

	return result, nil
}

// DisconnectAccount removes a linked OAuth provider.
func (s *Service) DisconnectAccount(ctx context.Context, tenantID, userID pgtype.UUID, provider string) error {
	return s.store.disconnectOAuthAccount(ctx, tenantID, userID, provider)
}

// ListUserContent returns the user's content, optionally filtered by status.
func (s *Service) ListUserContent(ctx context.Context, tenantID, userID pgtype.UUID, status string, limit, offset int32) ([]ContentListItem, error) {
	content, err := s.store.listUserContent(ctx, tenantID, userID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing user content: %w", err)
	}

	result := make([]ContentListItem, len(content))
	for i, c := range content {
		result[i] = ContentListItem{
			ID:          c.ID.String(),
			Title:       c.Title,
			ContentType: c.ContentType,
			Status:      c.Status,
			CreatedAt:   c.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:   c.UpdatedAt.Time.Format(time.RFC3339),
		}
	}

	return result, nil
}

// GetUserDraft returns the caller's own draft content by ID for the /create
// composer to restore. Returns pgx.ErrNoRows (→ 404) when the content is
// missing, not a draft, or owned by another user/tenant. FieldValues is
// emitted as a json.RawMessage so the JSONB column round-trips as a JSON
// object.
func (s *Service) GetUserDraft(ctx context.Context, tenantID, userID, contentID pgtype.UUID) (*DraftResponse, error) {
	row, err := s.store.getUserDraft(ctx, tenantID, userID, contentID)
	if err != nil {
		return nil, fmt.Errorf("getting user draft: %w", err)
	}

	var fieldValues json.RawMessage
	if len(row.FieldValues) > 0 {
		fieldValues = json.RawMessage(row.FieldValues)
	}

	return &DraftResponse{
		ID:           row.ID.String(),
		Slug:         row.Slug,
		ContentType:  row.ContentType,
		Title:        row.Title,
		Body:         row.Body,
		BodyHTML:     row.BodyHtml,
		FieldValues:  fieldValues,
		CanonicalURL: row.CanonicalUrl,
		Status:       row.Status,
		Visibility:   row.Visibility,
		SpaceID:      uuidPtr(row.SpaceID),
		GeoSpaceID:   uuidPtr(row.GeoSpaceID),
		PublishAt:    tsPtr(row.PublishAt),
		PublishedAt:  tsPtr(row.PublishedAt),
		CreatedAt:    row.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:    row.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

// uuidPtr converts a nullable pgtype.UUID to a *string (nil when invalid).
func uuidPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := u.String()
	return &s
}

// tsPtr converts a nullable pgtype.Timestamptz to a *string RFC3339 (nil when invalid).
func tsPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.Format(time.RFC3339)
	return &s
}

// ListUserBookmarks returns the user's bookmarks.
func (s *Service) ListUserBookmarks(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]BookmarkListItem, error) {
	bookmarks, err := s.store.listUserBookmarks(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing user bookmarks: %w", err)
	}

	result := make([]BookmarkListItem, len(bookmarks))
	for i, b := range bookmarks {
		result[i] = BookmarkListItem{
			ID:         b.ID.String(),
			EntityType: b.EntityType,
			EntityID:   b.EntityID.String(),
			Folder:     b.Folder,
			CreatedAt:  b.CreatedAt.Time.Format(time.RFC3339),
		}
	}

	return result, nil
}

// UserAnalytics represents aggregated user analytics.
type UserAnalytics struct {
	TotalContent     int32 `json:"total_content"`
	DraftContent     int32 `json:"draft_content"`
	PublishedContent int32 `json:"published_content"`
	TotalBookmarks   int32 `json:"total_bookmarks"`
}

// GetUserAnalytics returns aggregated analytics for the user.
func (s *Service) GetUserAnalytics(ctx context.Context, tenantID, userID pgtype.UUID) (*UserAnalytics, error) {
	all, err := s.store.listUserContent(ctx, tenantID, userID, "", 1000, 0)
	if err != nil {
		return nil, fmt.Errorf("listing content for analytics: %w", err)
	}

	bookmarks, err := s.store.listUserBookmarks(ctx, userID, 1000, 0)
	if err != nil {
		return nil, fmt.Errorf("listing bookmarks for analytics: %w", err)
	}

	analytics := &UserAnalytics{
		TotalContent:   int32(len(all)),       //nolint:gosec // bounded by query LIMIT
		TotalBookmarks: int32(len(bookmarks)), //nolint:gosec // bounded by query LIMIT
	}

	for _, c := range all {
		switch c.Status {
		case "draft":
			analytics.DraftContent++
		case "published":
			analytics.PublishedContent++
		}
	}

	return analytics, nil
}

// DeactivateAccount soft-deactivates the user account.
func (s *Service) DeactivateAccount(ctx context.Context, tenantID, userID pgtype.UUID) error {
	return s.store.deactivateUser(ctx, tenantID, userID)
}

// SoftDeleteUser soft-deletes a user account (status = 'deleted').
func (s *Service) SoftDeleteUser(ctx context.Context, tenantID, userID pgtype.UUID) error {
	return s.store.SoftDeleteUser(ctx, tenantID, userID)
}

// UpdateProfileType changes a user's profile_type (personal, business, organisation).
func (s *Service) UpdateProfileType(ctx context.Context, tenantID, userID pgtype.UUID, profileType string) error {
	return s.store.read.UpdateUserProfileType(ctx, gen.UpdateUserProfileTypeParams{
		TenantID:    tenantID,
		ID:          userID,
		ProfileType: profileType,
	})
}

// ActiveUserExists reports whether an active user with this id exists in the
// tenant. Used by the public profile gate to 404 non-active users.
func (s *Service) ActiveUserExists(ctx context.Context, tenantID, userID pgtype.UUID) (bool, error) {
	return s.store.activeUserExistsByID(ctx, tenantID, userID)
}
