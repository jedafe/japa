package auth

import (
	"context"
	"log/slog"
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

// MaxRefreshTokenTTL caps the admin-configured refresh token TTL so a
// misconfigured 'auth.session.refresh_days' (e.g. 1000000) cannot create
// everlasting tokens.
const MaxRefreshTokenTTL = 90 * 24 * time.Hour // 90 days

// MaxAccessTokenTTL caps the admin-configured access token TTL so a huge
// 'auth.session.access_token_ttl_minutes' cannot create everlasting JWTs or
// overflow the int64 duration multiplication.
const MaxAccessTokenTTL = 24 * time.Hour

// GetRefreshTokenTTL reads the refresh token TTL from system_settings with a
// fallback to the 30-day default. The setting key is 'auth.session.refresh_days'
// and the value is an integer number of days.
func GetRefreshTokenTTL(ctx context.Context, settings SettingsReader) time.Duration {
	if settings == nil {
		return DefaultRefreshTokenTTL
	}

	days := settings.GetIntOr(ctx, "auth.session.refresh_days", 30)
	if days <= 0 {
		return DefaultRefreshTokenTTL
	}

	// Compare in day units before converting to time.Duration: a huge
	// days value would overflow int64 nanoseconds during multiplication
	// and wrap negative, defeating a post-conversion clamp.
	if days > 90 {
		slog.Warn("refresh token TTL clamped to max", "requested_days", days, "max", MaxRefreshTokenTTL.String())
		return MaxRefreshTokenTTL
	}

	return time.Duration(days) * 24 * time.Hour
}

// GetAccessTokenTTL reads the access token TTL from system_settings with a
// fallback to the 15-minute default. The setting key is
// 'auth.session.access_token_ttl_minutes' and the value is an integer number
// of minutes.
func GetAccessTokenTTL(ctx context.Context, settings SettingsReader) time.Duration {
	if settings == nil {
		return DefaultAccessTokenTTL
	}

	minutes := settings.GetIntOr(ctx, "auth.session.access_token_ttl_minutes", 15)
	if minutes <= 0 {
		return DefaultAccessTokenTTL
	}

	// Compare in minute units before converting: huge values would
	// overflow int64 nanoseconds during multiplication.
	if minutes > 1440 {
		slog.Warn("access token TTL clamped to max", "requested_minutes", minutes, "max", MaxAccessTokenTTL.String())
		return MaxAccessTokenTTL
	}

	return time.Duration(minutes) * time.Minute
}

// TOTPRequiredForAdmin reads the 'auth.totp.required_for_admin' setting.
// When true, admin/control panel access requires TOTP enrollment.
func TOTPRequiredForAdmin(ctx context.Context, settings SettingsReader) bool {
	if settings == nil {
		return false
	}

	val, err := settings.GetSettingBool(ctx, "auth.totp.required_for_admin")
	if err != nil {
		return false
	}

	return val
}

// EmailVerifyRequired reads the 'auth.email.verify_required' setting.
// When true, users whose email_verified_at is NULL are blocked at login.
func EmailVerifyRequired(ctx context.Context, settings SettingsReader) bool {
	if settings == nil {
		return false
	}

	val, err := settings.GetSettingBool(ctx, "auth.email.verify_required")
	if err != nil {
		return false
	}

	return val
}

// AdminCapabilityChecker reports whether a user holds admin-level capabilities.
// Implemented by *rbac.RBAC.HasAdminCapability. Used by RequireTOTPForAdmin
// to decide whether the TOTP enrollment requirement applies.
type AdminCapabilityChecker interface {
	HasAdminCapability(ctx context.Context, userID pgtype.UUID) bool
}

// TOTPEnrollmentChecker reports whether a user has an active TOTP enrollment
// (enabled_at IS NOT NULL in totp_credentials). Implemented by a closure
// over the DB reader pool to avoid importing gen.Queries here.
type TOTPEnrollmentChecker interface {
	HasTOTPEnrolled(ctx context.Context, userID pgtype.UUID) (bool, error)
}

// RequireTOTPForAdmin is middleware that enforces the 'auth.totp.required_for_admin'
// system setting (Section 02). When the setting is enabled, any admin-level
// user accessing an admin route (path starting with /admin/) must have an
// active TOTP enrollment; otherwise the request is rejected with 403
// 'totp_required'. Non-admin users and non-admin routes pass through
// unchanged.
//
// Fail-closed: when the setting is enabled but the resolver (rbac) or the
// TOTP store (totp) is nil — or the enrollment lookup errors — an admin-level
// request is DENIED with 503 (dependency unavailable), never passed through.
// A nil settings reader is treated as "setting disabled" (fail-open) because
// there is no affirmative signal that enforcement was requested.
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
			if !TOTPRequiredForAdmin(r.Context(), settings) {
				next.ServeHTTP(w, r)
				return
			}

			user := FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
				// Unauthenticated — let RequireAuth handle the 401.
				next.ServeHTTP(w, r)
				return
			}

			// Only enforce for admin-level users. A nil resolver cannot prove
			// the caller is NOT admin-level, so fail closed for every
			// authenticated caller on admin routes.
			if rbac == nil {
				slog.Error("totp-for-admin: RBAC resolver unavailable while enforcement is enabled — denying admin access",
					"path", r.URL.Path, "user_id", user.UserID.String())
				errpkg.WriteError(w, http.StatusServiceUnavailable, "totp_unavailable", "Two-factor enforcement is temporarily unavailable")
				return
			}
			if !rbac.HasAdminCapability(r.Context(), user.UserID) {
				next.ServeHTTP(w, r)
				return
			}

			// Check TOTP enrollment.
			if totp == nil {
				slog.Error("totp-for-admin: TOTP store unavailable while enforcement is enabled — denying admin access",
					"path", r.URL.Path, "user_id", user.UserID.String())
				errpkg.WriteError(w, http.StatusServiceUnavailable, "totp_unavailable", "Two-factor enforcement is temporarily unavailable")
				return
			}

			enrolled, err := totp.HasTOTPEnrolled(r.Context(), user.UserID)
			if err != nil {
				// DB error — fail closed while enforcement is enabled.
				slog.Error("totp-for-admin: enrollment check failed — denying admin access",
					"path", r.URL.Path, "user_id", user.UserID.String(), "error", err)
				errpkg.WriteError(w, http.StatusServiceUnavailable, "totp_unavailable", "Two-factor enforcement is temporarily unavailable")
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

// ErrNoRows — Sentinel: pgx.ErrNoRows re-exported for convenience.
var ErrNoRows = pgx.ErrNoRows
