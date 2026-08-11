package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Service provides user profile and preference operations with reader/writer
// pool separation. Business logic lives here; the Store handles raw queries.
type Service struct {
	store *Store
	db    *db.DB
	// MediaResolver resolves a media_id to a presigned URL. Optional —
	// when nil, avatar_media_id/header_media_id fields are stored as-is
	// without URL resolution (BUG-004).
	MediaResolver MediaResolver
	// PointsProvider returns a user's Points balance for public display.
	// Optional — when nil, the public profile omits the points field even
	// if the user has consented (Section 28). Implemented by wallet.Service.
	PointsProvider PointsProvider
}

// MediaResolver resolves a media UUID to a presigned URL.
// Implemented by the media package; injected at server wiring time.
type MediaResolver interface {
	ResolveMediaURL(ctx context.Context, tenantID, mediaID pgtype.UUID) (string, error)
}

// PointsProvider returns a user's Points balance. Implemented by wallet.Service.
// Used by GetPublicProfileByUsername to surface points on a public profile when
// the user has opted in via show_points_publicly (Section 28).
type PointsProvider interface {
	GetPointsBalance(ctx context.Context, tenantID, userID pgtype.UUID) (int64, error)
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

	// Best-effort fetch of the user's content_country_preference (ADR-064)
	// so the feed header can render the inline geo control without an extra
	// round-trip. A failure degrades to an empty string (frontend falls back
	// to the GeoIP-resolved country).
	contentCountry, _ := s.GetContentCountry(ctx, tenantID, userID)

	// ADR-064a: best-effort fetch of origin/destination country custom field
	// values so the settings/location page can pre-populate. A failure
	// degrades to empty strings (the dropdowns show "Not set").
	originCountry, _ := s.store.getGeoCustomField(ctx, tenantID, userID, "origin_country")
	destCountry, _ := s.store.getGeoCustomField(ctx, tenantID, userID, "destination_country")

	// ADR-064 Option A: best-effort fetch of the user's current_geo_space_id
	// so sidebar widgets can pass it to geo-aware content endpoints without
	// an extra round-trip. A failure degrades to an empty string (widgets
	// fetch global content).
	currentGeoSpaceID := ""
	if geoSpace, gErr := s.store.getCurrentGeoSpace(ctx, tenantID, userID); gErr == nil && geoSpace.CurrentGeoSpaceID.Valid {
		currentGeoSpaceID = geoSpace.CurrentGeoSpaceID.String()
	}

	// Map media IDs to strings for the response (BUG-004).
	var avatarMediaID, headerMediaID *string
	if p.AvatarMediaID.Valid {
		s := uuid.UUID(p.AvatarMediaID.Bytes).String()
		avatarMediaID = &s
	}
	if p.HeaderMediaID.Valid {
		s := uuid.UUID(p.HeaderMediaID.Bytes).String()
		headerMediaID = &s
	}

	return &UserProfileResponse{
		ID:                       p.UserID.String(),
		Email:                    p.Email,
		Username:                 p.Username,
		DisplayName:              p.DisplayName,
		AvatarURL:                p.AvatarUrl,
		AvatarMediaID:            avatarMediaID,
		CoverURL:                 p.CoverUrl,
		HeaderMediaID:            headerMediaID,
		Bio:                      p.Bio,
		Timezone:                 p.Timezone,
		ProfileType:              p.ProfileType,
		Status:                   p.Status,
		FollowerCount:            p.FollowerCount,
		UserGroupName:            groupName,
		Groups:                   groups,
		LastLoginAt:              lastLogin,
		CreatedAt:                p.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:                p.UpdatedAt.Time.Format(time.RFC3339),
		ContentCountryPreference: contentCountry,
		OriginCountry:            originCountry,
		DestinationCountry:       destCountry,
		CurrentGeoSpaceID:        currentGeoSpaceID,
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
		CoverURL:      p.CoverUrl,
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
//
// When the user has opted in via show_points_publicly (Section 28) and a
// PointsProvider is wired, the response includes the user's Points balance.
// Credits are never included.
func (s *Service) GetPublicProfileByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (*PublicProfileResponse, error) {
	p, err := s.store.getPublicProfileByUsername(ctx, tenantID, username)
	if err != nil {
		return nil, fmt.Errorf("getting public profile by username: %w", err)
	}

	resp := &PublicProfileResponse{
		ID:             p.UserID.String(),
		Username:       p.Username,
		DisplayName:    p.DisplayName,
		AvatarURL:      p.AvatarUrl,
		CoverURL:       p.CoverUrl,
		Bio:            p.Bio,
		ProfileType:    p.ProfileType,
		FollowerCount:  p.FollowerCount,
		FollowingCount: p.FollowingCount,
		CreatedAt:      p.CreatedAt.Time.Format(time.RFC3339),
	}

	// Conditionally include Points if the user has consented (Section 28).
	// Credits are never exposed publicly.
	if p.ShowPointsPublicly && s.PointsProvider != nil {
		points, perr := s.PointsProvider.GetPointsBalance(ctx, tenantID, p.UserID)
		if perr == nil {
			resp.Points = &points
		}
		// If the balance lookup fails, omit the field rather than failing
		// the entire profile response — points display is non-critical.
	}

	return resp, nil
}

// UpdateProfile updates the user profile fields. Only non-nil fields are
// overwritten; nil fields are left unchanged via a read-then-merge strategy.
// Supports both URL-based (avatar_url/cover_url) and media-system
// (avatar_media_id/header_media_id) fields per Section 50 (BUG-004).
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

	coverURL := current.CoverUrl
	if req.CoverURL != nil {
		coverURL = req.CoverURL
	}

	bio := current.Bio
	if req.Bio != nil {
		bio = req.Bio
	}

	timezone := current.Timezone
	if req.Timezone != nil {
		timezone = req.Timezone
	}

	// Media ID fields (BUG-004): when provided, resolve to URLs via the
	// MediaResolver (if wired) and store both the media_id and the URL.
	avatarMediaID := current.AvatarMediaID
	if req.AvatarMediaID != nil {
		if *req.AvatarMediaID == "" {
			avatarMediaID = pgtype.UUID{}
		} else {
			var mid pgtype.UUID
			if err := mid.Scan(*req.AvatarMediaID); err != nil {
				return fmt.Errorf("invalid avatar_media_id: %w", err)
			}
			avatarMediaID = mid
			// Resolve to URL if a MediaResolver is wired.
			if s.MediaResolver != nil {
				url, err := s.MediaResolver.ResolveMediaURL(ctx, tenantID, mid)
				if err == nil {
					avatarURL = &url
				}
			}
		}
	}

	headerMediaID := current.HeaderMediaID
	if req.HeaderMediaID != nil {
		if *req.HeaderMediaID == "" {
			headerMediaID = pgtype.UUID{}
		} else {
			var mid pgtype.UUID
			if err := mid.Scan(*req.HeaderMediaID); err != nil {
				return fmt.Errorf("invalid header_media_id: %w", err)
			}
			headerMediaID = mid
			// Resolve to URL if a MediaResolver is wired.
			if s.MediaResolver != nil {
				url, err := s.MediaResolver.ResolveMediaURL(ctx, tenantID, mid)
				if err == nil {
					coverURL = &url
				}
			}
		}
	}

	if err := s.store.upsertProfile(ctx, tenantID, userID, displayName, avatarURL, coverURL, bio, timezone, avatarMediaID, headerMediaID); err != nil {
		return fmt.Errorf("updating profile: %w", err)
	}

	// ADR-064a: Save geo custom fields (origin_country, destination_country)
	// as custom_field_values on the user_profile entity. These are saved
	// after the profile upsert to ensure the profile row exists.
	if req.OriginCountry != nil {
		if err := s.store.setGeoCustomField(ctx, tenantID, userID, "origin_country", strings.ToUpper(strings.TrimSpace(*req.OriginCountry))); err != nil {
			return fmt.Errorf("saving origin_country: %w", err)
		}
	}
	if req.DestinationCountry != nil {
		if err := s.store.setGeoCustomField(ctx, tenantID, userID, "destination_country", strings.ToUpper(strings.TrimSpace(*req.DestinationCountry))); err != nil {
			return fmt.Errorf("saving destination_country: %w", err)
		}
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

// SetContentCountry updates the user's content_country_preference.
// An empty country clears the preference (falls back to GeoIP).
func (s *Service) SetContentCountry(ctx context.Context, tenantID, userID pgtype.UUID, country string) error {
	return s.store.SetContentCountryPreference(ctx, tenantID, userID, country)
}

// GetContentCountry returns the user's content_country_preference.
func (s *Service) GetContentCountry(ctx context.Context, tenantID, userID pgtype.UUID) (string, error) {
	return s.store.GetContentCountryPreference(ctx, tenantID, userID)
}

// SetCurrentGeoSpace updates the user's current_geo_space_id (ADR-064a).
// This is the user's declared current location (city/state/country Space).
// The VisitorContext resolver reads this to override GeoIP-based CurrentCountry.
func (s *Service) SetCurrentGeoSpace(ctx context.Context, tenantID, userID, spaceID pgtype.UUID) error {
	return s.store.updateGeoSpace(ctx, tenantID, userID, spaceID)
}

// GetUserModerationConfig returns the platform's valid user statuses, profile
// types, violation types, restriction operations, and restriction durations
// so the admin frontend does not need to hardcode them (BUG-017 through
// BUG-021). These are platform constants sourced from the schema CHECK
// constraints and the moderation subsystem's canonical value sets.
// Ref: japaconnect-engineering/sections/03-rbac.md (user statuses/profile types)
//
//	japaconnect-engineering/sections/46-impersonation.md (moderation)
func (s *Service) GetUserModerationConfig(ctx context.Context) (*UserModerationConfig, error) {
	return &UserModerationConfig{
		// User statuses — mirrors the values used by the users.status column
		// and surfaced in the admin users filter (BUG-017).
		Statuses: []ConfigOption{
			{Value: "active", Label: "Active"},
			{Value: "pending", Label: "Pending"},
			{Value: "restricted", Label: "Restricted"},
			{Value: "suspended", Label: "Suspended"},
			{Value: "banned", Label: "Banned"},
			{Value: "deleted", Label: "Deleted"},
		},
		// Profile types — mirrors users.profile_type CHECK constraint
		// (BUG-019).
		ProfileTypes: []ConfigOption{
			{Value: "personal", Label: "Personal"},
			{Value: "business", Label: "Business"},
			{Value: "organisation", Label: "Organisation"},
		},
		// Violation types — canonical moderation violation categories
		// (BUG-020).
		ViolationTypes: []ConfigOption{
			{Value: "spam", Label: "Spam"},
			{Value: "harassment", Label: "Harassment"},
			{Value: "misinformation", Label: "Misinformation"},
			{Value: "hate_speech", Label: "Hate Speech"},
			{Value: "other", Label: "Other"},
		},
		// Restriction operations — the moderation actions that can be
		// applied to a user (BUG-021).
		RestrictionOps: []ConfigOption{
			{Value: "post", Label: "Post"},
			{Value: "comment", Label: "Comment"},
			{Value: "discussion", Label: "Discussion"},
			{Value: "blog", Label: "Blog"},
			{Value: "message", Label: "Message"},
		},
		// Restriction durations — the time-boxed enforcement windows
		// (BUG-021).
		RestrictionDurations: []ConfigDuration{
			{Value: "1h", Label: "1 Hour"},
			{Value: "2h", Label: "2 Hours"},
			{Value: "6h", Label: "6 Hours"},
			{Value: "12h", Label: "12 Hours"},
			{Value: "1d", Label: "1 Day"},
			{Value: "7d", Label: "7 Days"},
			{Value: "30d", Label: "30 Days"},
			{Value: "permanent", Label: "Permanent"},
		},
	}, nil
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

// ListPublicUsers returns a paginated, PII-safe list of active, search-indexable
// users for the public directory at GET /users. Never exposes email, status,
// last_login_at, or groups (Section 19 / ADR-014). Search filters by username
// or display_name. profileType filters by profile_type (empty = all). Results
// are ordered by follower_count DESC so the directory surfaces prominent
// users first. Returns items + pagination metadata (total, page, total_pages).
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
//      japaconnect-engineering/sections/19-privacy-gdpr.md (search_indexable opt-out)
func (s *Service) ListPublicUsers(ctx context.Context, tenantID pgtype.UUID, search, profileType string, limit, offset int32) (*PublicUserListResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.store.listPublicUsers(ctx, tenantID, search, profileType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing public users: %w", err)
	}

	items := make([]PublicProfileResponse, len(rows))
	for i, u := range rows {
		items[i] = PublicProfileResponse{
			ID:             u.UserID.String(),
			Username:       u.Username,
			DisplayName:    u.DisplayName,
			AvatarURL:      u.AvatarUrl,
			CoverURL:       u.CoverUrl,
			Bio:            u.Bio,
			ProfileType:    u.ProfileType,
			FollowerCount:  u.FollowerCount,
			FollowingCount: u.FollowingCount,
			CreatedAt:      u.CreatedAt.Time.Format(time.RFC3339),
		}
	}

	total, err := s.store.countPublicUsers(ctx, tenantID, search, profileType)
	if err != nil {
		return nil, fmt.Errorf("counting public users: %w", err)
	}

	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}

	return &PublicUserListResponse{
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
func (s *Service) GetPrivacySettings(ctx context.Context, tenantID, userID pgtype.UUID) (*PrivacySettings, error) {
	settings, err := s.store.getUserPrivacySettings(ctx, tenantID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &PrivacySettings{
				DmPrivacy:         "everyone",
				ProfileVisibility: "public",
				SearchIndexable:   true,
				ShowOnlineStatus:  true,
				ShowActivity:      true,
				ShowPointsPublicly: false, // default false — user must opt in (Section 28)
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
		ShowPointsPublicly: settings.ShowPointsPublicly,
	}, nil
}

// UpdatePrivacySettings updates the user's privacy settings.
func (s *Service) UpdatePrivacySettings(ctx context.Context, tenantID, userID pgtype.UUID, req *UpdatePrivacyRequest) error {
	current, err := s.GetPrivacySettings(ctx, tenantID, userID)
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

	showPointsPublicly := current.ShowPointsPublicly
	if req.ShowPointsPublicly != nil {
		showPointsPublicly = *req.ShowPointsPublicly
	}

	return s.store.upsertUserPrivacySettings(ctx, tenantID, userID, dmPrivacy, profileVisibility, searchIndexable, showOnlineStatus, showActivity, showPointsPublicly)
}

// ListSessions returns the user's active sessions. currentSessionID is the
// refresh_tokens.id of the session making the request (resolved from the
// jc_refresh cookie); it is used to flag the current session so the frontend
// can show a "current" badge and omit the revoke button on it.
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
func (s *Service) ListSessions(ctx context.Context, tenantID, userID pgtype.UUID, currentSessionID pgtype.UUID) ([]SessionInfo, error) {
	sessions, err := s.store.listUserSessions(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}

	result := make([]SessionInfo, len(sessions))
	for i, sess := range sessions {
		si := SessionInfo{
			ID:        sess.ID.String(),
			CreatedAt: sess.CreatedAt.Time.Format(time.RFC3339),
			ExpiresAt: sess.ExpiresAt.Time.Format(time.RFC3339),
			Current:   sess.ID == currentSessionID,
		}
		if sess.DeviceName != nil {
			si.DeviceName = *sess.DeviceName
		}
		if sess.IpAddress != nil {
			si.IpAddress = *sess.IpAddress
		}
		if sess.LastUsedAt.Valid {
			si.LastUsedAt = sess.LastUsedAt.Time.Format(time.RFC3339)
		}
		result[i] = si
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
func (s *Service) RevokeSession(ctx context.Context, tenantID, userID, sessionID pgtype.UUID) error {
	return s.store.revokeUserSession(ctx, tenantID, sessionID, userID)
}

// RevokeAllSessions revokes all sessions except the current one. The current
// session's refresh_tokens.id is resolved from the jc_refresh cookie by the
// handler and passed here — NOT inferred from created_at DESC, which would
// keep the wrong session if the user has multiple concurrent sessions.
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
func (s *Service) RevokeAllSessions(ctx context.Context, tenantID, userID, currentSessionID pgtype.UUID) error {
	return s.store.revokeAllUserSessions(ctx, tenantID, userID, currentSessionID)
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
// ListUserContent returns the user's content, optionally filtered by status
// and content_type. An empty status/contentType string means "no filter".
func (s *Service) ListUserContent(ctx context.Context, tenantID, userID pgtype.UUID, status, q string, limit, offset int32) ([]ContentListItem, error) {
	return s.listUserContentInternal(ctx, tenantID, userID, status, "", q, limit, offset)
}

// ListUserContentByStatus returns the user's content filtered by status,
// content_type, and an optional free-text search over title/slug. Used by
// /me/content (published) and /me/drafts (draft).
func (s *Service) ListUserContentByStatus(ctx context.Context, tenantID, userID pgtype.UUID, status, contentType, q string, limit, offset int32) ([]ContentListItem, error) {
	return s.listUserContentInternal(ctx, tenantID, userID, status, contentType, q, limit, offset)
}

func (s *Service) listUserContentInternal(ctx context.Context, tenantID, userID pgtype.UUID, status, contentType, q string, limit, offset int32) ([]ContentListItem, error) {
	content, err := s.store.listUserContent(ctx, tenantID, userID, status, contentType, q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing user content: %w", err)
	}

	result := make([]ContentListItem, len(content))
	for i, c := range content {
		result[i] = ContentListItem{
			ID:          c.ID.String(),
			Title:       c.Title,
			Slug:        c.Slug,
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

	seoIndexable := row.SeoIndexable
	return &DraftResponse{
		ID:              row.ID.String(),
		Slug:            row.Slug,
		ContentType:     row.ContentType,
		Title:           row.Title,
		Body:            row.Body,
		BodyHTML:        row.BodyHtml,
		FieldValues:     fieldValues,
		CanonicalURL:    row.CanonicalUrl,
		HeaderImageID:   uuidPtr(row.HeaderImageID),
		FeaturedImageID: uuidPtr(row.FeaturedImageID),
		MetaTitle:       row.MetaTitle,
		MetaDescription: row.MetaDescription,
		OgImageURL:      row.OgImageUrl,
		SeoIndexable:    &seoIndexable,
		OrganizationID:  uuidPtr(row.OrganizationID),
		Status:          row.Status,
		Visibility:      row.Visibility,
		SpaceID:         uuidPtr(row.SpaceID),
		GeoSpaceID:      uuidPtr(row.GeoSpaceID),
		PublishAt:       tsPtr(row.PublishAt),
		PublishedAt:     tsPtr(row.PublishedAt),
		CreatedAt:       row.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:       row.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

// GetUserContent returns the caller's own content by ID (any status) for the
// /create composer to restore published, scheduled, or draft content for
// editing. Returns pgx.ErrNoRows (→ 404) when the content is missing or owned
// by another user/tenant. The response shape is identical to GetUserDraft so
// the composer's ContentRecord consumes it without branching.
func (s *Service) GetUserContent(ctx context.Context, tenantID, userID, contentID pgtype.UUID) (*DraftResponse, error) {
	row, err := s.store.getUserContent(ctx, tenantID, userID, contentID)
	if err != nil {
		return nil, fmt.Errorf("getting user content: %w", err)
	}
	return draftResponseFromRow(row.ID, row.Slug, row.ContentType, row.Title, row.Body, row.BodyHtml, row.FieldValues, row.CanonicalUrl, row.HeaderImageID, row.FeaturedImageID, row.MetaTitle, row.MetaDescription, row.OgImageUrl, row.SeoIndexable, row.OrganizationID, row.Status, row.Visibility, row.SpaceID, row.GeoSpaceID, row.PublishAt, row.PublishedAt, row.CreatedAt, row.UpdatedAt), nil
}

// GetContentForEditor returns any content by ID within the tenant (no author
// filter) for super-admins or users with the RBAC `content.edit` permission
// who need to edit content they don't own. The handler is responsible for
// checking permissions before calling this — it is NOT author-scoped.
// Returns pgx.ErrNoRows (→ 404) when the content is missing or belongs to a
// different tenant. The response shape is identical to GetUserContent.
func (s *Service) GetContentForEditor(ctx context.Context, tenantID, contentID pgtype.UUID) (*DraftResponse, error) {
	row, err := s.store.getContentByIDForEditor(ctx, tenantID, contentID)
	if err != nil {
		return nil, fmt.Errorf("getting content for editor: %w", err)
	}
	return draftResponseFromRow(row.ID, row.Slug, row.ContentType, row.Title, row.Body, row.BodyHtml, row.FieldValues, row.CanonicalUrl, row.HeaderImageID, row.FeaturedImageID, row.MetaTitle, row.MetaDescription, row.OgImageUrl, row.SeoIndexable, row.OrganizationID, row.Status, row.Visibility, row.SpaceID, row.GeoSpaceID, row.PublishAt, row.PublishedAt, row.CreatedAt, row.UpdatedAt), nil
}

// draftResponseFromRow maps the common column set shared by GetUserDraft,
// GetUserContent, and GetContentByIDForEditor into a DraftResponse. All
// three queries return the same columns, so this helper avoids duplicating
// the mapping logic.
func draftResponseFromRow(
	id pgtype.UUID, slug, contentType, title, body, bodyHtml string, fieldValues []byte,
	canonicalUrl *string, headerImageID, featuredImageID pgtype.UUID, metaTitle, metaDescription, ogImageUrl *string, seoIndexable bool, organizationID pgtype.UUID, status, visibility string, spaceID, geoSpaceID pgtype.UUID, publishAt, publishedAt, createdAt, updatedAt pgtype.Timestamptz,
) *DraftResponse {
	var fv json.RawMessage
	if len(fieldValues) > 0 {
		fv = json.RawMessage(fieldValues)
	}
	si := seoIndexable
	return &DraftResponse{
		ID:              id.String(),
		Slug:            slug,
		ContentType:     contentType,
		Title:           title,
		Body:            body,
		BodyHTML:        bodyHtml,
		FieldValues:     fv,
		CanonicalURL:    canonicalUrl,
		HeaderImageID:   uuidPtr(headerImageID),
		FeaturedImageID: uuidPtr(featuredImageID),
		MetaTitle:       metaTitle,
		MetaDescription: metaDescription,
		OgImageURL:      ogImageUrl,
		SeoIndexable:    &si,
		OrganizationID:  uuidPtr(organizationID),
		Status:          status,
		Visibility:      visibility,
		SpaceID:         uuidPtr(spaceID),
		GeoSpaceID:      uuidPtr(geoSpaceID),
		PublishAt:       tsPtr(publishAt),
		PublishedAt:     tsPtr(publishedAt),
		CreatedAt:       createdAt.Time.Format(time.RFC3339),
		UpdatedAt:       updatedAt.Time.Format(time.RFC3339),
	}
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

// ListUserBookmarks returns the user's bookmarks enriched with the
// bookmarked entity's title + slug (joined server-side per entity_type).
// When folderID is valid, results are filtered to that folder (BL-002).
// When entityTypes is non-empty, results are filtered to those entity types
// (e.g. the learn app requests only 'learning_track' bookmarks).
func (s *Service) ListUserBookmarks(ctx context.Context, tenantID, userID pgtype.UUID, folderID pgtype.UUID, entityTypes []string, limit, offset int32) ([]BookmarkListItem, error) {
	bookmarks, err := s.store.listUserBookmarks(ctx, tenantID, userID, folderID, entityTypes, limit, offset)
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
			FolderID:   nullableUUIDString(b.FolderID),
			CreatedAt:  b.CreatedAt.Time.Format(time.RFC3339),
			Title:      nullableString(b.Title),
			Slug:       nullableString(b.Slug),
		}
	}

	return result, nil
}

// DeleteBookmarkByID removes a single bookmark by its row ID. Tenant + owner
// scoped — returns ErrNotFound when the bookmark doesn't exist or belongs to
// another user/tenant. Works polymorphically across all entity types.
func (s *Service) DeleteBookmarkByID(ctx context.Context, id, tenantID, userID pgtype.UUID) error {
	affected, err := s.store.write.DeleteBookmarkByID(ctx, gen.DeleteBookmarkByIDParams{
		ID:       id,
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return fmt.Errorf("deleting bookmark by id: %w", err)
	}
	if affected == 0 {
		return errpkg.ErrNotFound
	}
	return nil
}

// nullableString coerces a sqlc interface{} column (from a LEFT JOIN CASE)
// into a *string. Returns nil for NULL/empty/non-string values.
func nullableString(v interface{}) *string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case string:
		if s == "" {
			return nil
		}
		return &s
	case *string:
		return s
	default:
		return nil
	}
}

// nullableUUIDString converts a pgtype.UUID to a *string. Returns nil when
// the UUID is zero/invalid (no folder assigned).
func nullableUUIDString(u pgtype.UUID) *string {
	if !u.Valid || u.Bytes == [16]byte{} {
		return nil
	}
	s := u.String()
	return &s
}

// UserAnalytics represents aggregated user analytics.
type UserAnalytics struct {
	TotalContent     int32 `json:"total_content"`
	DraftContent     int32 `json:"draft_content"`
	PublishedContent int32 `json:"published_content"`
	TotalBookmarks   int32 `json:"total_bookmarks"`
	TotalFollowers   int32 `json:"total_followers"`
	TotalFollowing   int32 `json:"total_following"`
}

// GetUserAnalytics returns aggregated analytics for the user.
// Uses SQL COUNT queries instead of row iteration (BUG-012).
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
func (s *Service) GetUserAnalytics(ctx context.Context, tenantID, userID pgtype.UUID) (*UserAnalytics, error) {
	contentCounts, err := s.store.countUserContent(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("counting user content: %w", err)
	}

	bookmarkCount, err := s.store.countUserBookmarks(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("counting user bookmarks: %w", err)
	}

	followStats, err := s.store.countUserFollowStats(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("counting user follow stats: %w", err)
	}

	return &UserAnalytics{
		TotalContent:     contentCounts.Total,
		DraftContent:     contentCounts.DraftCount,
		PublishedContent: contentCounts.PublishedCount,
		TotalBookmarks:   bookmarkCount,
		TotalFollowers:   followStats.FollowerCount,
		TotalFollowing:   followStats.FollowingCount,
	}, nil
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
	return s.store.write.UpdateUserProfileType(ctx, gen.UpdateUserProfileTypeParams{
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

// GetUserContentAnalytics returns per-content analytics for the authenticated user.
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
func (s *Service) GetUserContentAnalytics(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.GetUserContentAnalyticsRow, error) {
	return s.store.getUserContentAnalytics(ctx, tenantID, userID, limit, offset)
}

// GetUserFollowerGrowth returns daily follower growth for the authenticated user.
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
func (s *Service) GetUserFollowerGrowth(ctx context.Context, tenantID, userID pgtype.UUID, days int) ([]gen.GetUserFollowerGrowthRow, error) {
	if days <= 0 {
		days = 30
	}
	from := pgtype.Date{Time: time.Now().AddDate(0, 0, -days), Valid: true}
	return s.store.getUserFollowerGrowth(ctx, tenantID, userID, &from, nil)
}

// ---------------------------------------------------------------------------
// Account management (Section 50: PATCH /me/account)
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
// ---------------------------------------------------------------------------

// usernameCooldownDays is the minimum interval between username changes.
// Section 50: "username (once per 30 days)".
const usernameCooldownDays = 30

// ErrUsernameCooldown is returned when the user attempts to change their
// username before the cooldown window has elapsed.
var ErrUsernameCooldown = errors.New("username change cooldown not elapsed")

// ErrUsernameTaken is returned when the requested username is already in use.
var ErrUsernameTaken = errors.New("username already taken")

// ErrEmailTaken is returned when the requested email is already in use.
var ErrEmailTaken = errors.New("email already taken")

// UpdateAccountResult describes the outcome of an account update so the
// handler can inform the frontend whether email re-verification is needed.
type UpdateAccountResult struct {
	UsernameChanged         bool `json:"username_changed"`
	EmailChanged            bool `json:"email_changed"`
	EmailVerificationNeeded bool `json:"email_verification_needed"`
	DisplayNameChanged      bool `json:"display_name_changed"`
}

// UpdateAccount updates the user's account-level fields (username, email,
// display_name) per Section 50. Username changes are rate-limited to once
// per 30 days; email changes reset email_verified_at to NULL (triggering
// re-verification). All fields are optional (partial update).
func (s *Service) UpdateAccount(ctx context.Context, tenantID, userID pgtype.UUID, req *UpdateAccountRequest) (*UpdateAccountResult, error) {
	result := &UpdateAccountResult{}

	// Row-lock the user for the duration of the update to prevent concurrent
	// account edits from racing on uniqueness checks.
	acct, err := s.store.getUserAccountForUpdate(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("getting account for update: %w", err)
	}

	// --- Username change (with 30-day cooldown) ---
	if req.Username != nil {
		newUsername := strings.TrimSpace(*req.Username)
		if newUsername == "" {
			return nil, errors.New("username cannot be empty")
		}
		if newUsername != acct.Username {
			// Enforce 30-day cooldown.
			if acct.UsernameChangedAt.Valid {
				elapsed := time.Since(acct.UsernameChangedAt.Time)
				if elapsed < time.Duration(usernameCooldownDays)*24*time.Hour {
					return nil, ErrUsernameCooldown
				}
			}
			// Check uniqueness within the tenant.
			count, err := s.store.usernameExistsForTenant(ctx, tenantID, userID, newUsername)
			if err != nil {
				return nil, fmt.Errorf("checking username uniqueness: %w", err)
			}
			if count > 0 {
				return nil, ErrUsernameTaken
			}
			if err := s.store.updateUsername(ctx, tenantID, userID, newUsername); err != nil {
				return nil, fmt.Errorf("updating username: %w", err)
			}
			result.UsernameChanged = true
		}
	}

	// --- Email change (triggers re-verification) ---
	if req.Email != nil {
		newEmail := strings.TrimSpace(strings.ToLower(*req.Email))
		if newEmail == "" {
			return nil, errors.New("email cannot be empty")
		}
		if newEmail != acct.Email {
			// Check uniqueness within the tenant.
			count, err := s.store.emailExistsForTenant(ctx, tenantID, userID, newEmail)
			if err != nil {
				return nil, fmt.Errorf("checking email uniqueness: %w", err)
			}
			if count > 0 {
				return nil, ErrEmailTaken
			}
			if err := s.store.updateEmail(ctx, tenantID, userID, newEmail); err != nil {
				return nil, fmt.Errorf("updating email: %w", err)
			}
			result.EmailChanged = true
			result.EmailVerificationNeeded = true
		}
	}

	// --- Display name change (delegates to profile upsert) ---
	if req.DisplayName != nil {
		newDisplayName := strings.TrimSpace(*req.DisplayName)
		// Update the display_name on the user_profiles row via the existing
		// UpdateProfile path (which upserts user_profiles).
		profileReq := &UpdateProfileRequest{DisplayName: &newDisplayName}
		if err := s.UpdateProfile(ctx, tenantID, userID, profileReq); err != nil {
			return nil, fmt.Errorf("updating display name: %w", err)
		}
		result.DisplayNameChanged = true
	}

	return result, nil
}
