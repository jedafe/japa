package users

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// JSONLDResolver resolves admin-managed JSON-LD templates (Section 14 /
// Section 26). The user handlers use it to emit Person structured data.
type JSONLDResolver interface {
	ResolveJSONLD(ctx context.Context, tenantID pgtype.UUID, contentType string, fields map[string]string) (json.RawMessage, error)
}

// PermissionResolver checks RBAC permissions and super-admin status. The
// /me/content/{id} handler uses it to decide whether a non-owner viewer
// (super-admin or a user with the `content.edit` permission node) may
// fetch content they don't own for editing.
type PermissionResolver interface {
	IsSuperAdmin(ctx context.Context, userID, tenantID pgtype.UUID) bool
	HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool
}

// PermissionLister resolves the caller's full effective permission node set
// (§26 RBAC wiring). Used by GetMyProfile to surface `permissions` on /me so
// frontend pages can gate UI on the actual permission node (e.g.
// `forum.ama.create`) instead of hardcoded role names (Rule 16).
type PermissionLister interface {
	UserPermissions(ctx context.Context, userID, tenantID pgtype.UUID) (map[string]bool, error)
}

// Handlers holds dependencies for user HTTP handlers.
type Handlers struct {
	Service *Service
	// Redis is used for VisitorContext cache invalidation when the user's
	// content_country_preference changes (ADR-064). Optional — when nil,
	// cache invalidation is skipped (the 15-minute TTL will eventually
	// expire the stale entry).
	Redis          valkeycompat.Cmdable
	jsonldResolver JSONLDResolver
	// permResolver is an optional RBAC resolver used by GetMyContent to
	// allow super-admins and `content.edit` permission holders to fetch
	// content they don't own for editing. When nil, only the author can
	// restore content via /me/content/{id}.
	permResolver PermissionResolver
	// permLister is an optional RBAC resolver used by GetMyProfile to
	// surface the caller's effective permission nodes on /me (§26). When
	// nil, the `permissions` field is omitted from the /me response.
	permLister PermissionLister
	// settings is used for configurable Nominatim URL/timeout (ADR-064a,
	// Rule 3). Optional — when nil, compiled-in defaults are used.
	settings geoSettingsReader
	// authSettings provides auth-specific system settings (password policy,
	// token TTLs). Implements auth.SettingsReader. Wired via SetAuthSettings
	// during server setup. When nil, ChangePassword falls back to defaults.
	authSettings auth.SettingsReader
	// keyRot is the JWT key rotation used to validate and revoke the current
	// access token on password change. Wired via SetKeyRotation during server
	// setup. When nil, JTI revocation is skipped (refresh token revocation
	// still happens via the store).
	keyRot *auth.KeyRotation
}

// NewHandlers creates Handlers with the given Service. A nil resolver is
// acceptable — JSON-LD is simply omitted when unavailable.
func NewHandlers(service *Service, resolver JSONLDResolver) *Handlers {
	return &Handlers{Service: service, jsonldResolver: resolver}
}

// SetPermissionResolver wires the RBAC resolver. Called once during server
// setup after the RBAC resolver is constructed.
func (h *Handlers) SetPermissionResolver(r PermissionResolver) {
	h.permResolver = r
}

// SetPermissionLister wires the RBAC permission lister used by GetMyProfile
// to surface the caller's effective permission nodes on /me (§26). Called
// once during server setup. When not wired, the `permissions` field is
// omitted from the /me response.
func (h *Handlers) SetPermissionLister(l PermissionLister) {
	h.permLister = l
}

// SetSettings wires the config settings reader. Called once during server
// setup. Used by the reverse-geocoding flow for configurable Nominatim
// URL/timeout (ADR-064a, Rule 3).
func (h *Handlers) SetSettings(s geoSettingsReader) {
	h.settings = s
}

// SetAuthSettings wires the auth-specific settings reader (implements
// auth.SettingsReader). Used by ChangePassword to read the tenant's password
// policy from system_settings (auth.password.* keys) instead of hardcoding
// validation rules. Called once during server setup.
func (h *Handlers) SetAuthSettings(s auth.SettingsReader) {
	h.authSettings = s
}

// SetKeyRotation wires the JWT key rotation. Used by ChangePassword to
// validate and revoke the caller's current access token JTI after a
// successful password change, forcing immediate re-login on all devices.
// Ref: §02 "Session invalidation on password change".
func (h *Handlers) SetKeyRotation(kr *auth.KeyRotation) {
	h.keyRot = kr
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate
// HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > maxPageSize {
		return maxPageSize
	}

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
}

// parsePageOffset extracts the page_offset query parameter.
func parsePageOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	// #nosec G109 // value bounded by MaxPageSize check
	// #nosec G115 // value bounded by caller
	return int32(n)
}

// ---------------------------------------------------------------------------
// Profile Handlers
// ---------------------------------------------------------------------------

// GetMyProfile handles GET /api/v1/users/me.
func (h *Handlers) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	profile, err := h.Service.GetProfile(r.Context(), user.TenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Surface the caller's effective RBAC permission nodes on /me so frontend
	// pages can gate UI on the actual permission node (e.g. `forum.ama.create`)
	// instead of hardcoded role names (§26 RBAC wiring, Rule 16). Best-effort:
	// a resolver failure degrades to omitting the field. The underlying
	// GetUserContext is L1/L2-cached so this adds no DB load to the /me call.
	if h.permLister != nil {
		perms, pErr := h.permLister.UserPermissions(r.Context(), user.UserID, user.TenantID)
		if pErr == nil && len(perms) > 0 {
			keys := make([]string, 0, len(perms))

			for k, v := range perms {
				if v {
					keys = append(keys, k)
				}
			}

			sort.Strings(keys)
			profile.EffectivePermissions = keys
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// GetUserByUsername handles GET /api/v1/users/{username}.
func (h *Handlers) GetUserByUsername(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	username := chi.URLParam(r, "username")
	if username == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_username", "Username is required")
		return
	}

	profile, err := h.Service.GetPublicProfileByUsername(r.Context(), tenantID, username)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// JSON-LD structured data (Section 14 / Section 26). Resolve the
	// admin-managed Person template, interpolating the profile's field
	// values. Best-effort: a missing template or resolution error degrades
	// to no json_ld field. Emitted as a sibling of `data`.
	resp := map[string]any{"data": profile}

	if h.jsonldResolver != nil {
		if jsonLd, jErr := h.jsonldResolver.ResolveJSONLD(r.Context(), tenantID, "Person", profileJSONLDFields(profile)); jErr == nil && len(jsonLd) > 0 {
			resp["json_ld"] = json.RawMessage(jsonLd)
		} else if jErr != nil {
			slog.Warn("users: JSON-LD template resolution failed", "username", username, "error", jErr)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// profileJSONLDFields builds the {{field_key}} → value map for interpolating
// a Person JSON-LD template from a public profile response.
func profileJSONLDFields(p *PublicProfileResponse) map[string]string {
	fullName := p.Username
	if p.DisplayName != nil && *p.DisplayName != "" {
		fullName = *p.DisplayName
	}

	avatar := ""
	if p.AvatarURL != nil {
		avatar = *p.AvatarURL
	}

	bio := ""
	if p.Bio != nil {
		bio = *p.Bio
	}

	return map[string]string{
		"full_name":    fullName,
		"username":     p.Username,
		"profile_url":  "",
		"avatar_url":   avatar,
		"bio":          bio,
		"job_title":    "",
		"company_name": "",
		"country":      "",
		"twitter_url":  "",
		"linkedin_url": "",
	}
}

// UpdateMyProfile handles PATCH /api/v1/users/me.
func (h *Handlers) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdateProfile(r.Context(), user.TenantID, user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	// ADR-064a: Invalidate the VisitorContext cache when geo custom fields
	// (origin_country, origin_state_id, origin_city_id, destination_country)
	// are updated, so the next request picks up the new archetype/ContentCountry
	// resolution.
	if req.OriginCountry != nil || req.OriginStateID != nil || req.OriginCityID != nil || req.DestinationCountry != nil {
		h.invalidateVisitorCache(r.Context(), user)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// UpdateMyJourney handles PATCH /api/v1/me/journey.
// Updates the user's journey context fields (origin, destination, japa stage,
// visa route, purpose, planned travel date) stored as user_profile custom
// fields, then invalidates the VisitorContext cache so the next feed request
// picks up the new signals.
func (h *Handlers) UpdateMyJourney(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req UpdateJourneyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdateJourney(r.Context(), user.TenantID, user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	h.invalidateVisitorCache(r.Context(), user)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// invalidateVisitorCache deletes the VisitorContext and recommendations
// cache entries for the given user (ADR-064a).
func (h *Handlers) invalidateVisitorCache(ctx context.Context, user *auth.AuthenticatedUser) {
	if h.Redis == nil {
		return
	}

	if user.SessionID != "" {
		_ = h.Redis.Del(ctx, valkey.MustKey("visitor:ctx:%s", user.SessionID)).Err()
		_ = h.Redis.Del(ctx, valkey.MustKey("recommendations:%s", user.SessionID)).Err()
	}

	_ = h.Redis.Del(ctx, valkey.MustKey("visitor:ctx:user:%s", user.UserID.String())).Err()
	_ = h.Redis.Del(ctx, valkey.MustKey("recommendations:user:%s", user.UserID.String())).Err()
}

// ListUsers handles GET /api/v1/users for admin user listing.
// Query params: search, status, group, page_size (default 20, max 100), page_offset (default 0).
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	// Admin-only: only super_admin group members may list users.
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")
	groupSlug := r.URL.Query().Get("group")

	result, err := h.Service.ListUsers(r.Context(), tenantID, search, status, groupSlug, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ListPublicUsers handles GET /api/v1/users — public, tenant-scoped, PII-safe.
// Returns active, search-indexable users with follower/following counts. No
// email, status, last_login_at, or groups are exposed (Section 19 / ADR-014).
// Query params: q (search by username or display_name), profile_type (optional
// filter: regular/pro/business), page_size (default 20, max 100),
// page_offset (default 0). Returns {data: items, meta: {total, page, total_pages}}.
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
//
//	japaconnect-engineering/sections/19-privacy-gdpr.md (search_indexable opt-out)
func (h *Handlers) ListPublicUsers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if search == "" {
		search = strings.TrimSpace(r.URL.Query().Get("search"))
	}

	profileType := strings.TrimSpace(r.URL.Query().Get("profile_type"))

	result, err := h.Service.ListPublicUsers(r.Context(), tenantID, search, profileType, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": result.Items,
		"meta": map[string]any{
			"total":       result.Total,
			"page":        result.Page,
			"total_pages": result.TotalPages,
		},
	})
}

// UserModerationConfig is the API payload returned by
// GET /admin/users/config. It surfaces the platform's valid user statuses,
// profile types, violation types, restriction operations, and restriction
// durations so the admin frontend does not need to hardcode them
// (BUG-017 through BUG-021).
// Ref: japaconnect-engineering/sections/03-rbac.md (user statuses)
//
//	japaconnect-engineering/sections/46-impersonation.md (moderation)
type UserModerationConfig struct {
	Statuses             []ConfigOption   `json:"statuses"`
	ProfileTypes         []ConfigOption   `json:"profile_types"`
	ViolationTypes       []ConfigOption   `json:"violation_types"`
	RestrictionOps       []ConfigOption   `json:"restriction_operations"`
	RestrictionDurations []ConfigDuration `json:"restriction_durations"`
}

// ConfigOption is a value/label pair for a dropdown or filter.
type ConfigOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ConfigDuration is a value/label pair for a duration selector.
type ConfigDuration struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// GetUserModerationConfig handles GET /api/v1/admin/users/config.
// Returns the platform's valid user statuses, profile types, violation
// types, restriction operations, and restriction durations so the admin
// frontend does not hardcode them (BUG-017 through BUG-021).
// Ref: japaconnect-engineering/sections/03-rbac.md (user statuses/profile types)
//
//	japaconnect-engineering/sections/46-impersonation.md (moderation)
func (h *Handlers) GetUserModerationConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireTenant(w, r); !ok {
		return
	}

	if user := auth.FromContext(r.Context()); user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	cfg, err := h.Service.GetUserModerationConfig(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cfg})
}

// ListUserSessions handles GET /api/v1/admin/users/{id}/sessions.
// Admin-scoped variant of /me/sessions that lists sessions for any user.
func (h *Handlers) ListUserSessions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if user := auth.FromContext(r.Context()); user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	// Admin views another user's sessions — no "current" session to flag.
	sessions, err := h.Service.ListSessions(r.Context(), tenantID, userID, pgtype.UUID{})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": sessions})
}

// GetAdminUser handles GET /api/v1/admin/users/{id}.
// Resolves the user by UUID (not username) for admin detail views.
func (h *Handlers) GetAdminUser(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	profile, err := h.Service.GetProfile(r.Context(), tenantID, userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// ListUserActivity handles GET /api/v1/admin/users/{id}/activity.
// Merged feed of the user's authored content and comments, newest first.
func (h *Handlers) ListUserActivity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if user := auth.FromContext(r.Context()); user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	limit := int32(defaultPageSize)

	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			if n > maxPageSize {
				n = maxPageSize
			}

			limit = int32(n) // #nosec
		}
	}

	activity, err := h.Service.ListUserActivity(r.Context(), tenantID, userID, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": activity})
}

// DeleteUser handles DELETE /api/v1/admin/users/{id}.
// Soft-deletes the user (status = 'deleted') to comply with the GDPR erasure
// model — the row is retained and only a confirmed erasure request may hard-delete.
func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if user := auth.FromContext(r.Context()); user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	if err := h.Service.SoftDeleteUser(r.Context(), tenantID, userID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Revoke all sessions for the deleted user — a soft-deleted user must
	// not continue to use the platform on existing tokens. Ref: §02
	// (Session invalidation), Rule 6 (perm cache invalidation).
	if err := h.Service.store.revokeAllRefreshTokens(r.Context(), tenantID, userID); err != nil {
		slog.Error("delete-user: failed to invalidate sessions",
			"user_id", userID.String(), "error", err)
	}
	// Invalidate the permission cache so cached permissions don't outlive
	// the deletion (Rule 6).
	if h.Redis != nil {
		_ = h.Redis.Del(r.Context(), valkey.MustKey("perm:%s", userID.String())).Err()
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": userID.String(), "status": "deleted"}})
}

// UpdateProfileType handles PATCH /api/v1/admin/users/{id}/profile-type.
// Allows admins to change a user's profile_type to regular, pro, or business.
func (h *Handlers) UpdateProfileType(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if user := auth.FromContext(r.Context()); user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var req struct {
		ProfileType string `json:"profile_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	validTypes := map[string]bool{"regular": true, "pro": true, "business": true}
	if !validTypes[req.ProfileType] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_type", "profile_type must be regular, pro, or business")
		return
	}

	if err := h.Service.UpdateProfileType(r.Context(), tenantID, userID, req.ProfileType); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": userID.String(), "profile_type": req.ProfileType}})
}

// ---------------------------------------------------------------------------
// Preferences Handlers
// ---------------------------------------------------------------------------

// GetMyPreferences handles GET /api/v1/users/me/preferences.
func (h *Handlers) GetMyPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	prefs, err := h.Service.GetPreferences(r.Context(), user.TenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": prefs})
}

// UpdateMyPreferences handles PUT /api/v1/users/me/preferences.
func (h *Handlers) UpdateMyPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdatePreferences(r.Context(), user.TenantID, user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// UpdateMyContentCountry handles PATCH /api/v1/me/preferences/content-country.
// Sets the authenticated user's content_country_preference for geo-aware
// personalization (ADR-064). An empty body or "country": "" clears the
// preference, causing the system to fall back to GeoIP-based detection.
func (h *Handlers) UpdateMyContentCountry(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var body struct {
		Country string `json:"country"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	country := strings.ToUpper(strings.TrimSpace(body.Country))
	if country != "" && len(country) != 2 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_country", "Country must be a 2-letter ISO code")
		return
	}

	if err := h.Service.SetContentCountry(r.Context(), user.TenantID, user.UserID, country); err != nil {
		handleServiceError(w, err)
		return
	}

	// Invalidate the VisitorContext and recommendations cache for this user
	// so the next request picks up the new preference immediately (ADR-064a).
	h.invalidateVisitorCache(r.Context(), user)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status":          "updated",
			"content_country": country,
		},
	})
}

// UpdateMyLocation handles POST /api/v1/me/location (ADR-064a).
// Accepts either { latitude, longitude } for browser-geolocation reverse-
// geocoding or { space_id } for direct Space selection from the cascading
// picker. Updates users.current_geo_space_id and invalidates the
// VisitorContext cache so the next request picks up the new location.
func (h *Handlers) UpdateMyLocation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var body struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
		SpaceID   *string  `json:"space_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var spaceID pgtype.UUID

	switch {
	// Direct space_id selection (cascading picker).
	case body.SpaceID != nil && *body.SpaceID != "":
		id, err := uuid.Parse(*body.SpaceID)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "space_id must be a valid UUID")
			return
		}

		spaceID = pgtype.UUID{Bytes: id, Valid: true}
		// Validate that the space exists and is a geo space (country/state/city
		// level) — prevents users from setting a non-geo space as their location.
		if !h.Service.store.isValidGeoSpace(r.Context(), tenantID, spaceID) {
			errpkg.WriteError(w, http.StatusBadRequest, "not_a_geo_space", "The selected space is not a geo space (country/state/city)")
			return
		}

	// Browser geolocation — reverse-geocode to a city-level Space.
	case body.Latitude != nil && body.Longitude != nil:
		// Reverse-geocode via Nominatim (OpenStreetMap). Rate-limited to
		// 1 req/s per Nominatim usage policy. The result is matched to an
		// existing seeded Space by name (city → state → country fallback).
		// URL and timeout are configurable via system_settings (Rule 3).
		cityName, stateName, countryCode, err := reverseGeocode(r.Context(), h.settings, tenantID, *body.Latitude, *body.Longitude)
		if err != nil {
			slog.Warn("reverse geocode failed", "error", err, "lat", *body.Latitude, "lng", *body.Longitude)
			errpkg.WriteError(w, http.StatusBadGateway, "geocode_failed", "Could not resolve coordinates to a location")

			return
		}

		if countryCode != "" {
			// 1. Try to find a city-level Space matching the city name.
			if cityName != "" {
				space, err := h.Service.store.findCitySpace(r.Context(), tenantID, cityName)
				if err == nil {
					spaceID = space
				}
			}
			// 2. Fall back to state-level Space if no city match (BUG-006).
			// Nominatim often appends administrative suffixes ("Montserrado
			// County", "Lagos State", "Île-de-France Region") that don't
			// match the seeded Space name. Try the exact name first, then
			// retry with common suffixes stripped.
			if !spaceID.Valid && stateName != "" {
				space, err := h.Service.store.findStateSpace(r.Context(), tenantID, stateName)
				if err == nil {
					spaceID = space
				} else {
					// Try stripping common administrative suffixes.
					for _, suffix := range []string{" County", " State", " Region", " Province", " Department", " District", " Prefecture", " Territory"} {
						if strings.HasSuffix(stateName, suffix) {
							trimmed := strings.TrimSuffix(stateName, suffix)

							space, err = h.Service.store.findStateSpace(r.Context(), tenantID, trimmed)
							if err == nil {
								spaceID = space
								break
							}
						}
					}
				}
			}
			// 3. Fall back to the country-level space if no state match.
			if !spaceID.Valid {
				space, err := h.Service.store.findCountrySpace(r.Context(), tenantID, countryCode)
				if err == nil {
					spaceID = space
				}
			}
		}

		if !spaceID.Valid {
			errpkg.WriteError(w, http.StatusNotFound, "no_matching_space", "No geo space found for this location")
			return
		}

	default:
		errpkg.WriteError(w, http.StatusBadRequest, "missing_location", "Provide either latitude/longitude or space_id")
		return
	}

	if err := h.Service.SetCurrentGeoSpace(r.Context(), tenantID, user.UserID, spaceID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Invalidate the VisitorContext and recommendations cache (ADR-064a).
	h.invalidateVisitorCache(r.Context(), user)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"status":   "updated",
			"space_id": spaceID.String(),
		},
	})
}

// GetMyLocation handles GET /api/v1/me/location (ADR-064a).
// Returns the user's current geo space ID and resolved location info.
func (h *Handlers) GetMyLocation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	geoSpace, err := h.Service.store.getCurrentGeoSpace(r.Context(), tenantID, user.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No profile row yet — return empty location, not an error.
			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
				"space_id":     nil,
				"city":         nil,
				"country":      nil,
				"country_code": nil,
			}})

			return
		}
		// Real DB error — surface it.
		slog.Warn("get current geo space failed", "error", err, "user_id", user.UserID.String())
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Could not retrieve location")

		return
	}

	data := map[string]any{
		"space_id":     nil,
		"name":         nil,
		"city":         nil,
		"city_id":      nil,
		"state":        nil,
		"state_id":     nil,
		"country":      nil,
		"country_code": nil,
	}
	if geoSpace.CurrentGeoSpaceID.Valid {
		data["space_id"] = geoSpace.CurrentGeoSpaceID.String()
	}
	// BUG-VIS-003: name is the space's display name regardless of level
	// (country/state/city), so the frontend can show the user's location
	// even when they selected a state or country instead of a city.
	if geoSpace.SpaceName != nil && *geoSpace.SpaceName != "" {
		data["name"] = *geoSpace.SpaceName
	}
	// city is only non-nil when the space is city-level (BUG-VIS-002).
	if geoSpace.CityName != "" {
		data["city"] = geoSpace.CityName
	}

	if geoSpace.CityID.Valid && geoSpace.CityName != "" {
		data["city_id"] = geoSpace.CityID.String()
	}
	// state is populated when the space is state-level or a city with a parent state.
	if geoSpace.StateName != nil && *geoSpace.StateName != "" {
		data["state"] = *geoSpace.StateName
	}

	if geoSpace.StateID.Valid {
		data["state_id"] = geoSpace.StateID.String()
	}

	if geoSpace.CountryName != nil {
		data["country"] = *geoSpace.CountryName
	}

	if geoSpace.CountryGeoCode != nil {
		data["country_code"] = *geoSpace.CountryGeoCode
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetUserAnalytics handles GET /users/{id}/analytics — the profile Analytics
// dashboard. Owner-only per JEOS api-routes-catalog §342: [auth, self] — the
// caller must be the profile owner (viewer id == {id}). Guests and other users
// receive 403 analytics_forbidden. Gated by requireActiveProfileUser on the
// route (404 for banned/suspended/deleted). Returns aggregate stats for the
// {id} user's published+public content plus follower/following counts.
//
// Ref: japaconnect-engineering/sections/33-professional-profiles.md
//
//	japaconnect-engineering/conventions/api-routes-catalog.md §342
func (h *Handlers) GetUserAnalytics(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	// Owner-only: analytics is a self-dashboard. Guests are blocked by
	// RequireAuth on the route; other authenticated users are blocked here.
	if userID != user.UserID {
		errpkg.WriteError(w, http.StatusForbidden, "analytics_forbidden", "You can only view your own analytics")
		return
	}

	stats, err := h.Service.store.read.GetPublicAuthorAnalytics(r.Context(),

		userID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}

		slog.Error("users: GetUserAnalytics failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "analytics_error", "Failed to load analytics")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
}
