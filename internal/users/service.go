package users

import (
	"context"
	"fmt"

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
