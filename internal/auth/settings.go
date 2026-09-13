package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// DefaultRefreshTokenTTL is the fallback refresh token TTL when the system setting
// 'auth.session.refresh_days' is not configured. Matches the catalog default.
const DefaultRefreshTokenTTL = 30 * 24 * time.Hour // 30 days

// DefaultAccessTokenTTL is the fallback access token TTL when the system setting
// 'auth.session.access_token_ttl_minutes' is not configured. Matches the catalog default.
const DefaultAccessTokenTTL = 15 * time.Minute

// GetRefreshTokenTTL reads the refresh token TTL from system_settings with a
// fallback to the 30-day default. The setting key is 'auth.session.refresh_days'
// and the value is an integer number of days.
func GetRefreshTokenTTL(ctx context.Context, settings SettingsReader, tenantID pgtype.UUID) time.Duration {
	if settings == nil {
		return DefaultRefreshTokenTTL
	}

	days := settings.GetIntOr(ctx, pgtype.UUID{}, "auth.session.refresh_days", 30)
	if days <= 0 {
		return DefaultRefreshTokenTTL
	}

	return time.Duration(days) * 24 * time.Hour
}

// GetAccessTokenTTL reads the access token TTL from system_settings with a
// fallback to the 15-minute default. The setting key is
// 'auth.session.access_token_ttl_minutes' and the value is an integer number
// of minutes.
func GetAccessTokenTTL(ctx context.Context, settings SettingsReader, tenantID pgtype.UUID) time.Duration {
	if settings == nil {
		return DefaultAccessTokenTTL
	}

	minutes := settings.GetIntOr(ctx, pgtype.UUID{}, "auth.session.access_token_ttl_minutes", 15)
	if minutes <= 0 {
		return DefaultAccessTokenTTL
	}

	return time.Duration(minutes) * time.Minute
}

// TOTPRequiredForAdmin reads the 'auth.totp.required_for_admin' setting.
// When true, admin/control panel access requires TOTP enrollment.
func TOTPRequiredForAdmin(ctx context.Context, settings SettingsReader, tenantID pgtype.UUID) bool {
	if settings == nil {
		return false
	}

	val, err := settings.GetSettingBool(ctx, tenantID, "auth.totp.required_for_admin")
	if err != nil {
		return false
	}

	return val
}

// EmailVerifyRequired reads the 'auth.email.verify_required' setting.
// When true, users whose email_verified_at is NULL are blocked at login.
func EmailVerifyRequired(ctx context.Context, settings SettingsReader, tenantID pgtype.UUID) bool {
	if settings == nil {
		return false
	}

	val, err := settings.GetSettingBool(ctx, tenantID, "auth.email.verify_required")
	if err != nil {
		return false
	}

	return val
}

// AdminCapabilityChecker reports whether a user holds admin-level capabilities.
// Implemented by *rbac.RBAC.HasAdminCapability. Used by RequireTOTPForAdmin
// to decide whether the TOTP enrollment requirement applies.
type AdminCapabilityChecker interface {
	HasAdminCapability(ctx context.Context, userID, tenantID pgtype.UUID) bool
}

// TOTPEnrollmentChecker reports whether a user has an active TOTP enrollment
// (enabled_at IS NOT NULL in totp_credentials). Implemented by a closure
// over the DB reader pool to avoid importing gen.Queries here.
type TOTPEnrollmentChecker interface {
	HasTOTPEnrolled(ctx context.Context, userID, tenantID pgtype.UUID) (bool, error)
}

// RequireTOTPForAdmin is middleware that enforces the 'auth.totp.required_for_admin'
// system setting (Section 02). When the setting is enabled, any admin-level
// user accessing an admin route (path starting with /admin/) must have an
// active TOTP enrollment; otherwise the request is rejected with 403
// 'totp_required'. Non-admin users and non-admin routes pass through
// unchanged. The middleware is a no-op when the setting is disabled or when
// any dependency is nil (graceful degradation).
// // Ref: japaconnect-engineering/sections/02-authentication-sessions.md.
func RequireTOTPForAdmin(
	settings SettingsReader,
	rbac AdminCapabilityChecker,
	totp TOTPEnrollmentChecker,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only applies to admin routes.
			if !strings.HasPrefix(r.URL.Path, "/api/v1/admin/") {
				next.ServeHTTP(w, r)
				return
			}

			// If the setting is disabled, no enforcement.
			if !TOTPRequiredForAdmin(r.Context(), settings, tenantIDFromRequest(r)) {
				next.ServeHTTP(w, r)
				return
			}

			user := FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
				// Unauthenticated — let RequireAuth handle the 401.
				next.ServeHTTP(w, r)
				return
			}

			// Only enforce for admin-level users.
			if rbac == nil || !rbac.HasAdminCapability(r.Context(), user.UserID, user.TenantID) {
				next.ServeHTTP(w, r)
				return
			}

			// Check TOTP enrollment.
			if totp == nil {
				next.ServeHTTP(w, r)
				return
			}

			enrolled, err := totp.HasTOTPEnrolled(r.Context(), user.UserID, user.TenantID)
			if err != nil {
				// DB error — fail open (let the request through) but log.
				next.ServeHTTP(w, r)
				return
			}

			if !enrolled {
				errpkg.WriteError(w, http.StatusForbidden, "totp_required", "Admin access requires two-factor authentication enrollment")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// tenantIDFromRequest extracts the tenant ID from the request context,
// falling back to the AuthenticatedUser's TenantID when the tenant
// middleware hasn't set it (e.g., during early bootstrap).
func tenantIDFromRequest(r *http.Request) pgtype.UUID {
	if user := FromContext(r.Context()); user != nil && user.TenantID.Valid {
		return user.TenantID
	}

	return pgtype.UUID{}
}

// ErrNoRows — Sentinel: pgx.ErrNoRows re-exported for convenience.
var ErrNoRows = pgx.ErrNoRows
