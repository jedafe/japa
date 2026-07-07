package users

import (
	"context"
	"errors"
	"fmt"

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
		CreatedAt:     p.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     p.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// GetProfileByUsername returns the user profile for the given username within a tenant.
func (s *Service) GetProfileByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (*UserProfileResponse, error) {
	p, err := s.store.getUserByUsername(ctx, tenantID, username)
	if err != nil {
		return nil, fmt.Errorf("getting profile by username: %w", err)
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
		CreatedAt:     p.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     p.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
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
// search filtering by username or email.
func (s *Service) ListUsers(ctx context.Context, tenantID pgtype.UUID, search string, limit, offset int32) (*ListUsersResponse, error) {
	if limit <= 0 {
		limit = 20
	}

	if offset < 0 {
		offset = 0
	}

	users, err := s.store.listUsers(ctx, tenantID, search, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	total, err := s.store.countUsers(ctx, tenantID, search)
	if err != nil {
		return nil, fmt.Errorf("counting users: %w", err)
	}

	items := make([]UserListItem, len(users))
	for i, u := range users {
		items[i] = UserListItem{
			ID:            u.ID.String(),
			Email:         u.Email,
			Username:      u.Username,
			ProfileType:   u.ProfileType,
			Status:        u.Status,
			FollowerCount: u.FollowerCount,
			CreatedAt:     u.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:     u.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
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
	}, nil
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
			CreatedAt: sess.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			ExpiresAt: sess.ExpiresAt.Time.Format("2006-01-02T15:04:05Z"),
		}
	}

	return result, nil
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
			ConnectedAt: acc.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
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
			CreatedAt:   c.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   c.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
		}
	}

	return result, nil
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
			CreatedAt:  b.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
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
