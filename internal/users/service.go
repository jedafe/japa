package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/bookmarkfolders"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tiers"
	"github.com/jedafe/japa/internal/worker"
	"golang.org/x/sync/errgroup"
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
	// enqueuer injects the Asynq task enqueuer for search index sync.
	// Optional — when nil, search reindex on privacy changes is skipped.
	enqueuer userEnqueuer
	// authSettings provides auth-specific system settings (password policy,
	// username cooldown). Implements auth.SettingsReader. Wired via
	// SetAuthSettings during server setup. When nil, compiled-in defaults
	// are used (BUG-005, Rule 3).
	authSettings auth.SettingsReader
	// folderResolver resolves default bookmark folders when a /me/bookmarks
	// call omits folder. Optional — when nil, "default" is used.
	folderResolver folderResolver
	// tierService resolves and updates the unified regular/pro/business tier.
	// Used by admin profile-type changes so users.profile_type stays in sync
	// with group memberships (§unified-tiers).
	tierService *tiers.Service
	sources     sourceManager
	siteURL     string
}

// userEnqueuer is the subset of worker.Enqueuer that users needs.
type userEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// sourceManager creates canonical intel_sources rows for entity profiles
// (v2 source directory, Task 5).
type sourceManager interface {
	CreateSourceFromEntity(ctx context.Context, tenantID, entityID, userID pgtype.UUID, entityType, name, slug, sourceURL, category string) (gen.IntelSource, error)
}

// folderResolver resolves a default bookmark folder for an entity/content
// type pair. Implemented by bookmarkfolders.DefaultFolderResolver.
type folderResolver interface {
	Resolve(ctx context.Context, tenantID, userID pgtype.UUID, entityType, contentType string) (bookmarkfolders.FolderResponse, error)
}

// SetEnqueuer injects the Asynq enqueuer for async user tasks (search index
// sync on privacy settings change).
func (s *Service) SetEnqueuer(e userEnqueuer) {
	s.enqueuer = e
}

// SetAuthSettings wires the auth-specific settings reader (implements
// auth.SettingsReader). Used by UpdateAccount to read the username change
// cooldown from system_settings (users.username_cooldown_days) instead of
// a hardcoded constant (BUG-005, Rule 3). Called once during server setup.
// When not wired, the compiled-in default (30 days) is used.
func (s *Service) SetAuthSettings(r auth.SettingsReader) {
	s.authSettings = r
}

// SetFolderResolver injects the default bookmark folder resolver. Optional
// — when not set, /me/bookmarks falls back to the "default" folder.
func (s *Service) SetFolderResolver(r folderResolver) { s.folderResolver = r }

// SetTierService injects the unified tier service. When set, admin profile
// type updates route through SetUserTier so group memberships and the
// denormalized users.profile_type stay in sync.
func (s *Service) SetTierService(ts *tiers.Service) { s.tierService = ts }

// SetSourceManager links the user service to the canonical source registry
// (v2 source directory, Task 5).
func (s *Service) SetSourceManager(m sourceManager) { s.sources = m }

// SetSiteURL injects the public site base URL used to build canonical entity
// source URLs. Must never be derived from the request Host (CWE-601).
func (s *Service) SetSiteURL(u string) { s.siteURL = strings.TrimRight(u, "/") }

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
	groupRows, err := s.store.read.ResolveUserGroups(ctx,

		userID,
	)
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
	originStateID, _ := s.store.getGeoCustomField(ctx, tenantID, userID, "origin_state_id")
	originCityID, _ := s.store.getGeoCustomField(ctx, tenantID, userID, "origin_city_id")
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
		OriginStateID:            originStateID,
		OriginCityID:             originCityID,
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
// // When the user has opted in via show_points_publicly (Section 28) and a
// PointsProvider is wired, the response includes the user's Points balance.
// Credits are never included.
func (s *Service) GetPublicProfileByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (*PublicProfileResponse, error) {
	p, err := s.store.getPublicProfileByUsername(ctx, tenantID, username)
	if err != nil {
		return nil, fmt.Errorf("getting public profile by username: %w", err)
	}

	resp := &PublicProfileResponse{
		ID:              p.UserID.String(),
		Username:        p.Username,
		DisplayName:     p.DisplayName,
		AvatarURL:       p.AvatarUrl,
		CoverURL:        p.CoverUrl,
		Bio:             p.Bio,
		ProfileType:     p.ProfileType,
		FollowerCount:   p.FollowerCount,
		FollowingCount:  p.FollowingCount,
		CreatedAt:       p.CreatedAt.Time.Format(time.RFC3339),
		IsSynthetic:     p.IsSynthetic,
		SyntheticSource: p.SyntheticSource,
		Location:        p.Location,
	}

	// §106 video channels: resolve the channel banner from
	// user_profiles.banner_media_id via the media resolver. Best-effort — a
	// resolution failure degrades to no banner rather than failing the
	// profile response.
	if p.BannerMediaID.Valid && s.MediaResolver != nil {
		if url, err := s.MediaResolver.ResolveMediaURL(ctx, tenantID, p.BannerMediaID); err == nil && url != "" {
			resp.BannerURL = &url
		}
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
	// BUG-002: Wrap the read-merge-upsert in a transaction with a FOR UPDATE
	// lock on the user_profiles row so two concurrent PATCH /me/profile
	// calls cannot both read the same baseline and the second write
	// overwrite the first (lost update).
	if err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		// Read current profile to preserve existing values for omitted fields.
		// The FOR UPDATE lock is held until commit.
		current, err := s.store.getProfileForUpdateTx(ctx, q, tenantID, userID)
		if err != nil {
			// First-time profile edit: no user_profiles row exists yet.
			// The upsert below will INSERT it. Use zero values for the merge.
			if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("reading current profile: %w", err)
			}
			current = gen.GetUserProfileForUpdateRow{}
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

		if err := s.store.upsertProfileTx(ctx, q, tenantID, userID, displayName, avatarURL, coverURL, bio, timezone, avatarMediaID, headerMediaID); err != nil {
			return fmt.Errorf("updating profile: %w", err)
		}

		// ADR-064a: Save geo custom fields (origin_country, origin_state_id,
		// origin_city_id, destination_country) as custom_field_values on the
		// user_profile entity. These are saved after the profile upsert to
		// ensure the profile row exists.
		if req.OriginCountry != nil {
			if err := s.store.setGeoCustomFieldTx(ctx, q, tenantID, userID, "origin_country", normalizeCountryCode(*req.OriginCountry)); err != nil {
				return fmt.Errorf("saving origin_country: %w", err)
			}
		}
		if req.OriginStateID != nil {
			if err := s.store.setGeoCustomFieldTx(ctx, q, tenantID, userID, "origin_state_id", strings.TrimSpace(*req.OriginStateID)); err != nil {
				return fmt.Errorf("saving origin_state_id: %w", err)
			}
		}
		if req.OriginCityID != nil {
			if err := s.store.setGeoCustomFieldTx(ctx, q, tenantID, userID, "origin_city_id", strings.TrimSpace(*req.OriginCityID)); err != nil {
				return fmt.Errorf("saving origin_city_id: %w", err)
			}
		}
		if req.DestinationCountry != nil {
			if err := s.store.setGeoCustomFieldTx(ctx, q, tenantID, userID, "destination_country", normalizeCountryCode(*req.DestinationCountry)); err != nil {
				return fmt.Errorf("saving destination_country: %w", err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Re-index the user in Meilisearch when display_name changes so the
	// search index reflects the new name (§13/§19). Enqueued after commit
	// so we don't index a row that might roll back.
	if req.DisplayName != nil {
		s.enqueueUserSearchIndex(tenantID, userID)
	}

	return nil
}

// UpdateJourney updates the journey-related custom field values on the user's
// profile (origin_country, destination_country, japa_status, visa_route,
// purpose, planned_travel_date). All fields are optional partial updates.
// Ref: docs/superpowers/specs/2026-08-21-journey-aware-recommendations-design.md.
func (s *Service) UpdateJourney(ctx context.Context, tenantID, userID pgtype.UUID, req *UpdateJourneyRequest) error {
	if err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if req.OriginCountry != nil {
			if err := s.store.setGeoCustomFieldTx(ctx, q, tenantID, userID, "origin_country", normalizeCountryCode(*req.OriginCountry)); err != nil {
				return fmt.Errorf("saving origin_country: %w", err)
			}
		}
		if req.DestinationCountry != nil {
			if err := s.store.setGeoCustomFieldTx(ctx, q, tenantID, userID, "destination_country", normalizeCountryCode(*req.DestinationCountry)); err != nil {
				return fmt.Errorf("saving destination_country: %w", err)
			}
		}
		if req.JapaStage != nil {
			if err := s.store.setGeoCustomFieldTx(ctx, q, tenantID, userID, "japa_status", normalizeJourneyValue(*req.JapaStage)); err != nil {
				return fmt.Errorf("saving japa_status: %w", err)
			}
		}
		if req.VisaRoute != nil {
			if err := s.store.setGeoCustomFieldTx(ctx, q, tenantID, userID, "visa_route", normalizeJourneyValue(*req.VisaRoute)); err != nil {
				return fmt.Errorf("saving visa_route: %w", err)
			}
		}
		if req.Purpose != nil {
			if err := s.store.setGeoCustomFieldTx(ctx, q, tenantID, userID, "purpose", normalizeJourneyValue(*req.Purpose)); err != nil {
				return fmt.Errorf("saving purpose: %w", err)
			}
		}
		if req.TravelDate != nil {
			if err := s.store.setGeoCustomFieldTx(ctx, q, tenantID, userID, "planned_travel_date", strings.TrimSpace(*req.TravelDate)); err != nil {
				return fmt.Errorf("saving planned_travel_date: %w", err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// normalizeCountryCode trims and upper-cases a 2-letter ISO country code.
// Values that are not 2 letters are returned as-is (preserving user intent
// while still trimming) so the custom field value remains searchable.
func normalizeCountryCode(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) == 2 {
		return s
	}

	return strings.TrimSpace(s)
}

// normalizeJourneyValue trims and lower-cases a journey enumeration value.
func normalizeJourneyValue(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
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
// // BUG-003: The read-merge-upsert is wrapped in a transaction with a FOR
// UPDATE lock on the user_preferences row so two concurrent PUT
// /me/preferences calls cannot both read the same baseline and the second
// write overwrite the first (lost update).
func (s *Service) UpdatePreferences(ctx context.Context, tenantID, userID pgtype.UUID, req *UpdatePreferencesRequest) error {
	return db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		// Read current preferences to preserve existing values for omitted
		// fields. The FOR UPDATE lock is held until commit.
		current, err := s.store.getPreferencesForUpdateTx(ctx, q, tenantID, userID)
		darkMode := "system"
		language := "en-NG"

		if err == nil {
			darkMode = current.DarkMode
			language = current.Language
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("reading current preferences: %w", err)
		}

		// Merge: use request value when provided, otherwise keep existing/default.
		if req.DarkMode != nil {
			darkMode = *req.DarkMode
		}

		if req.Language != nil {
			language = *req.Language
		}

		if err := s.store.upsertPreferencesTx(ctx, q, tenantID, userID, darkMode, language); err != nil {
			return fmt.Errorf("updating preferences: %w", err)
		}

		return nil
	})
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
// //	japaconnect-engineering/sections/46-impersonation.md (moderation)
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
		// Profile types — mirrors the unified regular/pro/business tier model
		// (§unified-tiers, BUG-019).
		ProfileTypes: []ConfigOption{
			{Value: "regular", Label: "Regular"},
			{Value: "pro", Label: "Pro"},
			{Value: "business", Label: "Business"},
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
// //	japaconnect-engineering/sections/19-privacy-gdpr.md (search_indexable opt-out)
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
			ID:              u.UserID.String(),
			Username:        u.Username,
			DisplayName:     u.DisplayName,
			AvatarURL:       u.AvatarUrl,
			CoverURL:        u.CoverUrl,
			Bio:             u.Bio,
			ProfileType:     u.ProfileType,
			FollowerCount:   u.FollowerCount,
			FollowingCount:  u.FollowingCount,
			CreatedAt:       u.CreatedAt.Time.Format(time.RFC3339),
			IsSynthetic:     u.IsSynthetic,
			SyntheticSource: u.SyntheticSource,
			Location:        u.Location,
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

// PrivacyOptions returns the allowed values for the user's privacy enum
// fields. These mirror the CHECK constraints on user_privacy_settings
// (dm_privacy and profile_visibility) and are exposed so the frontend does
// not need to hardcode them (Rule 16).
type PrivacyOptions struct {
	DmPrivacy         []string `json:"dm_privacy"`
	ProfileVisibility []string `json:"profile_visibility"`
}

// GetPrivacyOptions returns the allowed privacy option values.
func (s *Service) GetPrivacyOptions(_ context.Context) *PrivacyOptions {
	return &PrivacyOptions{
		DmPrivacy:         []string{"everyone", "followers_only", "nobody"},
		ProfileVisibility: []string{"public", "members", "followers"},
	}
}

// GetPrivacySettings returns the user's privacy settings.
func (s *Service) GetPrivacySettings(ctx context.Context, tenantID, userID pgtype.UUID) (*PrivacySettings, error) {
	settings, err := s.store.getUserPrivacySettings(ctx, tenantID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &PrivacySettings{
				DmPrivacy:          "everyone",
				ProfileVisibility:  "public",
				SearchIndexable:    true,
				ShowOnlineStatus:   true,
				ShowActivity:       true,
				ShowPointsPublicly: false, // default false — user must opt in (Section 28)
			}, nil
		}

		return nil, fmt.Errorf("getting privacy settings: %w", err)
	}

	return &PrivacySettings{
		DmPrivacy:          settings.DmPrivacy,
		ProfileVisibility:  settings.ProfileVisibility,
		SearchIndexable:    settings.SearchIndexable,
		ShowOnlineStatus:   settings.ShowOnlineStatus,
		ShowActivity:       settings.ShowActivity,
		ShowPointsPublicly: settings.ShowPointsPublicly,
	}, nil
}

// UpdatePrivacySettings updates the user's privacy settings.
// // BUG-004: The read-merge-upsert is wrapped in a transaction with a FOR
// UPDATE lock on the user_privacy_settings row so two concurrent PATCH
// /me/privacy calls cannot both read the same baseline and the second
// write overwrite the first (lost update).
func (s *Service) UpdatePrivacySettings(ctx context.Context, tenantID, userID pgtype.UUID, req *UpdatePrivacyRequest) error {
	if err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		// Read current settings with FOR UPDATE lock held until commit.
		current, err := s.store.getUserPrivacySettingsForUpdateTx(ctx, q, tenantID, userID)
		dmPrivacy := "followers"
		profileVisibility := "public"
		var searchIndexable, showOnlineStatus, showActivity, showPointsPublicly bool

		if err == nil {
			dmPrivacy = current.DmPrivacy
			profileVisibility = current.ProfileVisibility
			searchIndexable = current.SearchIndexable
			showOnlineStatus = current.ShowOnlineStatus
			showActivity = current.ShowActivity
			showPointsPublicly = current.ShowPointsPublicly
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("reading current settings: %w", err)
		}

		if req.DmPrivacy != nil {
			dmPrivacy = *req.DmPrivacy
		}
		if req.ProfileVisibility != nil {
			profileVisibility = *req.ProfileVisibility
		}
		if req.SearchIndexable != nil {
			searchIndexable = *req.SearchIndexable
		}
		if req.ShowOnlineStatus != nil {
			showOnlineStatus = *req.ShowOnlineStatus
		}
		if req.ShowActivity != nil {
			showActivity = *req.ShowActivity
		}
		if req.ShowPointsPublicly != nil {
			showPointsPublicly = *req.ShowPointsPublicly
		}

		return s.store.upsertUserPrivacySettingsTx(ctx, q, tenantID, userID, dmPrivacy, profileVisibility, searchIndexable, showOnlineStatus, showActivity, showPointsPublicly)
	}); err != nil {
		return err
	}

	// §13/§19: when search_indexable changed, re-index the user so the
	// Meilisearch users index reflects their discoverability opt-out. The
	// IndexUser method checks search_indexable and deletes the document
	// if the user opted out, or upserts it if they opted back in.
	// Enqueued after commit so we don't index a row that might roll back.
	s.enqueueUserSearchIndex(tenantID, userID)

	return nil
}

// enqueueUserSearchIndex enqueues a search:index_entity task for the user so
// the Meilisearch index reflects their current search_indexable setting.
// Best-effort — logs errors but never fails the caller.
func (s *Service) enqueueUserSearchIndex(tenantID, userID pgtype.UUID) {
	if s.enqueuer == nil {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"entity_id":   userID.String(),
		"entity_type": "user",
		"tenant_id":   tenantID.String(),
	})
	if err := s.enqueuer.Enqueue(worker.TypeSearchIndexSync, payload); err != nil {
		slog.Warn("users: failed to enqueue search:index_entity for user", "user_id", userID.String(), "error", err)
	}
}

// ListSessions returns the user's active sessions. currentSessionID is the
// refresh_tokens.id of the session making the request (resolved from the
// jc_refresh cookie); it is used to flag the current session so the frontend
// can show a "current" badge and omit the revoke button on it.
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md.
func (s *Service) ListSessions(ctx context.Context, tenantID, userID, currentSessionID pgtype.UUID) ([]SessionInfo, error) {
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
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md.
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

// ListUserContentByStatus returns the user's content filtered by status,
// content_type, and an optional free-text search over title/slug.
// sort="" defaults to newest. Used by /me/content, /me/drafts, and /me/scheduled.
func (s *Service) ListUserContentByStatus(ctx context.Context, tenantID, userID pgtype.UUID, status, contentType, q, sort string, limit, offset int32) ([]ContentListItem, error) {
	return s.listUserContentInternal(ctx, tenantID, userID, status, contentType, q, sort, limit, offset)
}

func (s *Service) listUserContentInternal(ctx context.Context, tenantID, userID pgtype.UUID, status, contentType, q, sort string, limit, offset int32) ([]ContentListItem, error) {
	content, err := s.store.listUserContent(ctx, tenantID, userID, status, contentType, q, sort, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing user content: %w", err)
	}

	result := make([]ContentListItem, len(content))
	for i, c := range content {
		result[i] = ContentListItem{
			ID:           c.ID.String(),
			Title:        c.Title,
			Slug:         c.Slug,
			ContentType:  c.ContentType,
			Status:       c.Status,
			ViewCount:    c.ViewCount,
			CommentCount: c.CommentCount,
			CreatedAt:    c.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:    c.UpdatedAt.Time.Format(time.RFC3339),
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
		VideoSourceID:   uuidPtr(row.VideoSourceID),
		MetaTitle:       row.MetaTitle,
		MetaDescription: row.MetaDescription,
		OgImageURL:      row.OgImageUrl,
		SeoIndexable:    &seoIndexable,
		OrganizationID:  uuidPtr(row.OrganizationID),
		Status:          row.Status,
		Visibility:      row.Visibility,
		SpaceID:         uuidPtr(row.SpaceID),
		GeoSpaceID:      uuidPtr(row.GeoSpaceID),
		GuideCategory:   row.GuideCategory,
		JourneyStage:    row.JourneyStage,
		VisaRoute:       row.VisaRoute,
		SourceCountry:   row.SourceCountry,
		TargetCountry:   row.TargetCountry,
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

	return draftResponseFromRow(row.ID, row.Slug, row.ContentType, row.Title, row.Body, row.BodyHtml, row.FieldValues, row.CanonicalUrl, row.HeaderImageID, row.FeaturedImageID, row.VideoSourceID, row.MetaTitle, row.MetaDescription, row.OgImageUrl, row.SeoIndexable, row.OrganizationID, row.Status, row.Visibility, row.SpaceID, row.GeoSpaceID, row.GuideCategory, row.JourneyStage, row.VisaRoute, row.SourceCountry, row.TargetCountry, row.PublishAt, row.PublishedAt, row.CreatedAt, row.UpdatedAt), nil
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

	return draftResponseFromRow(row.ID, row.Slug, row.ContentType, row.Title, row.Body, row.BodyHtml, row.FieldValues, row.CanonicalUrl, row.HeaderImageID, row.FeaturedImageID, row.VideoSourceID, row.MetaTitle, row.MetaDescription, row.OgImageUrl, row.SeoIndexable, row.OrganizationID, row.Status, row.Visibility, row.SpaceID, row.GeoSpaceID, row.GuideCategory, row.JourneyStage, row.VisaRoute, row.SourceCountry, row.TargetCountry, row.PublishAt, row.PublishedAt, row.CreatedAt, row.UpdatedAt), nil
}

// draftResponseFromRow maps the common column set shared by GetUserDraft,
// GetUserContent, and GetContentByIDForEditor into a DraftResponse. All
// three queries return the same columns, so this helper avoids duplicating
// the mapping logic.
func draftResponseFromRow(
	id pgtype.UUID, slug, contentType, title, body, bodyHtml string, fieldValues []byte,
	canonicalUrl *string, headerImageID, featuredImageID, videoSourceID pgtype.UUID, metaTitle, metaDescription, ogImageUrl *string, seoIndexable bool, organizationID pgtype.UUID, status, visibility string, spaceID, geoSpaceID pgtype.UUID, guideCategory, journeyStage, visaRoute, sourceCountry, targetCountry *string, publishAt, publishedAt, createdAt, updatedAt pgtype.Timestamptz,
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
		VideoSourceID:   uuidPtr(videoSourceID),
		MetaTitle:       metaTitle,
		MetaDescription: metaDescription,
		OgImageURL:      ogImageUrl,
		SeoIndexable:    &si,
		OrganizationID:  uuidPtr(organizationID),
		Status:          status,
		Visibility:      visibility,
		SpaceID:         uuidPtr(spaceID),
		GeoSpaceID:      uuidPtr(geoSpaceID),
		GuideCategory:   guideCategory,
		JourneyStage:    journeyStage,
		VisaRoute:       visaRoute,
		SourceCountry:   sourceCountry,
		TargetCountry:   targetCountry,
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
func (s *Service) ListUserBookmarks(ctx context.Context, tenantID, userID, folderID pgtype.UUID, entityTypes []string, limit, offset int32) ([]BookmarkListItem, error) {
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
			Audience:   nullableString(b.Audience),
		}
	}

	return result, nil
}

// CreateBookmark adds a polymorphic bookmark for the given entity.
// It is idempotent — an existing bookmark updates the folder/folder_id.
// Ref: §50 (bookmarks), DOC-013 (doc_page bookmark creation support).
func (s *Service) CreateBookmark(ctx context.Context, tenantID, userID, entityID, folderID pgtype.UUID, entityType, folder string) (gen.CreateBookmarkRow, error) {
	// Normalize legacy forum entity types to the consolidated names.
	switch entityType {
	case "thread":
		entityType = "forum_thread"
	case "reply":
		entityType = "forum_reply"
	}

	if folder == "" {
		if folderID.Valid && folderID.Bytes != [16]byte{} {
			f, err := s.store.read.GetBookmarkFolder(ctx, gen.GetBookmarkFolderParams{
				ID: folderID,

				UserID: userID,
			})

			switch {
			case err == nil:
				folder = f.Name
			case errors.Is(err, pgx.ErrNoRows):
				return gen.CreateBookmarkRow{}, fmt.Errorf("%w: folder not found", errpkg.ErrNotFound)
			default:
				return gen.CreateBookmarkRow{}, fmt.Errorf("looking up folder: %w", err)
			}
		} else if s.folderResolver != nil {
			contentType := ""

			if entityType == "content" || entityType == "blog_post" {
				c, err := s.store.read.GetContentByID(ctx,
					entityID,
				)
				if err == nil {
					contentType = c.ContentType
				} else if !errors.Is(err, pgx.ErrNoRows) {
					slog.Warn("users: failed to look up content type for bookmark folder", "error", err)
				}
			}

			f, err := s.folderResolver.Resolve(ctx, tenantID, userID, entityType, contentType)
			if err == nil && f.ID != "" {
				folder = f.Name

				var fid pgtype.UUID
				if scanErr := fid.Scan(f.ID); scanErr == nil {
					folderID = fid
				}
			} else if err != nil {
				slog.Warn("users: failed to resolve default bookmark folder", "error", err)
			}
		}

		if folder == "" {
			folder = "default"
		}
	}

	bm, err := s.store.write.CreateBookmark(ctx, gen.CreateBookmarkParams{

		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Folder:     folder,
		FolderID:   folderID,
	})
	if err != nil {
		return gen.CreateBookmarkRow{}, fmt.Errorf("creating bookmark: %w", err)
	}

	return bm, nil
}

// DeleteBookmarkByID removes a single bookmark by its row ID. Tenant + owner
// scoped — returns ErrNotFound when the bookmark doesn't exist or belongs to
// another user/tenant. Works polymorphically across all entity types in the
// unified bookmarks table (§08/§50).
func (s *Service) DeleteBookmarkByID(ctx context.Context, id, tenantID, userID pgtype.UUID) error {
	affected, err := s.store.write.DeleteBookmarkByID(ctx, gen.DeleteBookmarkByIDParams{
		ID: id,

		UserID: userID,
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
	TotalContent         int32                                  `json:"total_content"`
	DraftContent         int32                                  `json:"draft_content"`
	PublishedContent     int32                                  `json:"published_content"`
	TotalBookmarks       int32                                  `json:"total_bookmarks"`
	TotalFollowers       int32                                  `json:"total_followers"`
	TotalFollowing       int32                                  `json:"total_following"`
	TotalViews           int64                                  `json:"total_views"`
	TotalReactions       int64                                  `json:"total_reactions"`
	TotalComments        int64                                  `json:"total_comments"`
	TotalPointsEarned    int64                                  `json:"total_points_earned"`
	TotalFollowersGained int64                                  `json:"total_followers_gained"`
	DailyImpressions     []DailyImpression                      `json:"daily_impressions"`
	ReadersByCountry     []ReaderByCountry                      `json:"readers_by_country"`
	TopContent           []gen.GetUserTopContentByPeriodRow     `json:"top_content"`
	ContentCreated       gen.CountUserContentCreatedInPeriodRow `json:"content_created"`
}

// DailyImpression is a single day's view count.
type DailyImpression struct {
	Day   string `json:"day"`
	Views int64  `json:"views"`
}

// ReaderByCountry aggregates unique content views by country code.
type ReaderByCountry struct {
	CountryCode string `json:"country_code"`
	Readers     int64  `json:"readers"`
}

// GetUserAnalytics returns aggregated analytics for the user, optionally
// scoped to a date range for period-based engagement metrics.
// Uses SQL COUNT queries instead of row iteration (BUG-012).
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md.
func (s *Service) GetUserAnalytics(ctx context.Context, tenantID, userID pgtype.UUID, from, to *time.Time) (*UserAnalytics, error) {
	fromTs := pgtype.Timestamptz{}
	toTs := pgtype.Timestamptz{}

	if from != nil {
		fromTs = pgtype.Timestamptz{Time: *from, Valid: true}
	}

	if to != nil {
		toTs = pgtype.Timestamptz{Time: *to, Valid: true}
	}

	// Run independent aggregate queries concurrently so we are not blocked on
	// the sum of individual query latencies (BUG-ANALYTICS-12).
	var (
		contentCounts    gen.CountUserContentRow
		contentCreated   gen.CountUserContentCreatedInPeriodRow
		bookmarkCount    int32
		followStats      gen.CountUserFollowStatsRow
		engagement       gen.GetUserPeriodEngagementRow
		followersGained  int64
		pointsEarned     int64
		dailyImpressions []gen.GetUserDailyImpressionsRow
		readersByCountry []gen.GetUserReadersByCountryRow
		topContent       []gen.GetUserTopContentByPeriodRow
	)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		contentCounts, err = s.store.countUserContent(ctx, tenantID, userID)

		return err
	})
	g.Go(func() error {
		var err error
		contentCreated, err = s.store.countUserContentCreatedInPeriod(ctx, tenantID, userID, fromTs, toTs)

		return err
	})
	g.Go(func() error {
		var err error
		bookmarkCount, err = s.store.countUserBookmarks(ctx, tenantID, userID)

		return err
	})
	g.Go(func() error {
		var err error
		followStats, err = s.store.countUserFollowStats(ctx, tenantID, userID)

		return err
	})
	g.Go(func() error {
		var err error
		engagement, err = s.store.getUserPeriodEngagement(ctx, tenantID, userID, fromTs, toTs)

		return err
	})
	g.Go(func() error {
		var err error
		followersGained, err = s.store.countUserFollowersGained(ctx, tenantID, userID, fromTs, toTs)

		return err
	})
	g.Go(func() error {
		var err error
		pointsEarned, err = s.store.countUserPointsEarned(ctx, tenantID, userID, fromTs, toTs)

		return err
	})
	g.Go(func() error {
		var err error
		dailyImpressions, err = s.store.getUserDailyImpressions(ctx, tenantID, userID, fromTs, toTs)

		return err
	})
	g.Go(func() error {
		var err error
		readersByCountry, err = s.store.getUserReadersByCountry(ctx, tenantID, userID, fromTs, toTs)

		return err
	})
	g.Go(func() error {
		var err error
		topContent, err = s.store.getUserTopContentByPeriod(ctx, tenantID, userID, fromTs, toTs)

		return err
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("loading user analytics: %w", err)
	}

	if topContent == nil {
		topContent = []gen.GetUserTopContentByPeriodRow{}
	}

	daily := make([]DailyImpression, 0, len(dailyImpressions))

	for _, row := range dailyImpressions {
		day := ""
		if row.Day.Valid {
			day = row.Day.Time.Format(time.DateOnly)
		}

		daily = append(daily, DailyImpression{Day: day, Views: row.Views})
	}

	readers := make([]ReaderByCountry, 0, len(readersByCountry))
	for _, row := range readersByCountry {
		readers = append(readers, ReaderByCountry{CountryCode: row.CountryCode, Readers: row.Readers})
	}

	return &UserAnalytics{
		TotalContent:         contentCounts.Total,
		DraftContent:         contentCounts.DraftCount,
		PublishedContent:     contentCounts.PublishedCount,
		TotalBookmarks:       bookmarkCount,
		TotalFollowers:       followStats.FollowerCount,
		TotalFollowing:       followStats.FollowingCount,
		TotalViews:           engagement.Views,
		TotalReactions:       engagement.Reactions,
		TotalComments:        engagement.Comments,
		TotalPointsEarned:    pointsEarned,
		TotalFollowersGained: followersGained,
		DailyImpressions:     daily,
		ReadersByCountry:     readers,
		TopContent:           topContent,
		ContentCreated:       contentCreated,
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

// UpdateProfileType changes a user's unified tier and syncs the denormalized
// users.profile_type to regular/pro/business. If the tier service is wired,
// group membership is updated at the same time.
func (s *Service) UpdateProfileType(ctx context.Context, tenantID, userID pgtype.UUID, profileType string) error {
	if s.tierService != nil {
		return s.tierService.SetUserTier(ctx, tenantID, userID, profileType)
	}

	return s.store.write.UpdateUserProfileType(ctx, gen.UpdateUserProfileTypeParams{

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
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md.
func (s *Service) GetUserContentAnalytics(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.GetUserContentAnalyticsRow, error) {
	return s.store.getUserContentAnalytics(ctx, tenantID, userID, limit, offset)
}

// GetUserContentByPeriod returns a paginated, period-scored list of the user's
// content. Used by GET /me/analytics/content when a period is supplied.
func (s *Service) GetUserContentByPeriod(ctx context.Context, tenantID, userID pgtype.UUID, from, to pgtype.Timestamptz, limit, offset int32) ([]gen.GetUserContentByPeriodRow, error) {
	return s.store.getUserContentByPeriod(ctx, tenantID, userID, from, to, limit, offset)
}

// GetUserFollowerGrowth returns daily follower growth for the authenticated user.
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md.
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

// defaultUsernameCooldownDays is the fallback interval between username
// changes when system_settings.users.username_cooldown_days is not set.
// Section 50: "username (once per 30 days)".
const defaultUsernameCooldownDays = 30

// usernameCooldownDaysFor returns the configured username change cooldown
// from system_settings (users.username_cooldown_days), falling back to the
// compiled-in default when the settings reader is not wired or the key is
// unset (BUG-005, Rule 3).
func (s *Service) usernameCooldownDaysFor(ctx context.Context, tenantID pgtype.UUID) int {
	if s.authSettings == nil {
		return defaultUsernameCooldownDays
	}

	return s.authSettings.GetIntOr(ctx, pgtype.UUID{}, "users.username_cooldown_days", defaultUsernameCooldownDays)
}

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
// // BUG-001: The entire update is wrapped in a single transaction so the
// FOR UPDATE row lock acquired by getUserAccountForUpdateTx is held until
// commit. This prevents two concurrent PATCH /me/account requests from
// both passing the uniqueness check and both writing, which would create
// a duplicate username or email.
func (s *Service) UpdateAccount(ctx context.Context, tenantID, userID pgtype.UUID, req *UpdateAccountRequest) (*UpdateAccountResult, error) {
	result, err := db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (*UpdateAccountResult, error) {
		txResult := &UpdateAccountResult{}

		// Row-lock the user for the duration of the transaction to prevent
		// concurrent account edits from racing on uniqueness checks.
		acct, err := s.store.getUserAccountForUpdateTx(ctx, q, tenantID, userID)
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
				// Enforce cooldown (configurable via system_settings,
				// users.username_cooldown_days; default 30 per §50).
				if acct.UsernameChangedAt.Valid {
					elapsed := time.Since(acct.UsernameChangedAt.Time)
					cooldown := time.Duration(s.usernameCooldownDaysFor(ctx, tenantID)) * 24 * time.Hour

					if elapsed < cooldown {
						return nil, ErrUsernameCooldown
					}
				}
				// Check uniqueness within the tenant.
				count, err := s.store.usernameExistsForTenantTx(ctx, q, tenantID, userID, newUsername)
				if err != nil {
					return nil, fmt.Errorf("checking username uniqueness: %w", err)
				}

				if count > 0 {
					return nil, ErrUsernameTaken
				}

				if err := s.store.updateUsernameTx(ctx, q, tenantID, userID, newUsername); err != nil {
					return nil, fmt.Errorf("updating username: %w", err)
				}

				txResult.UsernameChanged = true
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
				count, err := s.store.emailExistsForTenantTx(ctx, q, tenantID, userID, newEmail)
				if err != nil {
					return nil, fmt.Errorf("checking email uniqueness: %w", err)
				}

				if count > 0 {
					return nil, ErrEmailTaken
				}

				if err := s.store.updateEmailTx(ctx, q, tenantID, userID, newEmail); err != nil {
					return nil, fmt.Errorf("updating email: %w", err)
				}

				txResult.EmailChanged = true
				txResult.EmailVerificationNeeded = true
			}
		}

		// --- Display name change (upserts user_profiles within the same
		// transaction so the FOR UPDATE lock covers this write too) ---
		if req.DisplayName != nil {
			newDisplayName := strings.TrimSpace(*req.DisplayName)
			if err := s.store.upsertProfileTx(ctx, q, tenantID, userID, &newDisplayName, nil, nil, nil, nil, pgtype.UUID{}, pgtype.UUID{}); err != nil {
				return nil, fmt.Errorf("updating display name: %w", err)
			}

			txResult.DisplayNameChanged = true
		}

		return txResult, nil
	})
	if err != nil {
		return nil, err
	}

	// Re-index the user in Meilisearch when username or display_name
	// changes so the search index reflects the new values (§13/§19).
	// Enqueued after commit so we don't index a row that might roll back.
	if result.UsernameChanged || result.DisplayNameChanged {
		s.enqueueUserSearchIndex(tenantID, userID)
	}

	return result, nil
}

// EnsureSyntheticUser returns an existing synthetic user by username or creates
// one with the supplied channel metadata. It is used by the video import flow
// to represent external channels as tenant-scoped users.
func (s *Service) EnsureSyntheticUser(ctx context.Context, tenantID pgtype.UUID, input SyntheticUserInput) (pgtype.UUID, error) {
	var userID pgtype.UUID

	err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		existing, err := q.GetSyntheticUserByUsername(ctx,

			input.Username,
		)
		if err == nil {
			userID = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("looking up synthetic user: %w", err)
		}

		newID, err := q.CreateSyntheticUser(ctx, gen.CreateSyntheticUserParams{

			Email:           input.Email,
			Username:        input.Username,
			SyntheticSource: &input.SourceType,
		})
		if err != nil {
			return fmt.Errorf("creating synthetic user: %w", err)
		}

		displayName := stringOrNil(input.DisplayName)
		bio := stringOrNil(input.Bio)
		avatarURL := stringOrNil(input.AvatarURL)
		coverURL := stringOrNil(input.CoverURL)
		location := stringOrNil(input.Location)

		if err := q.UpsertUserProfile(ctx, gen.UpsertUserProfileParams{

			UserID:        newID,
			DisplayName:   displayName,
			Bio:           bio,
			AvatarUrl:     avatarURL,
			CoverUrl:      coverURL,
			Location:      location,
			BannerMediaID: input.BannerMediaID,
		}); err != nil {
			return fmt.Errorf("upserting synthetic user profile: %w", err)
		}

		userID = newID
		return nil
	})
	if err != nil {
		return pgtype.UUID{}, err
	}

	return userID, nil
}

func stringOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
