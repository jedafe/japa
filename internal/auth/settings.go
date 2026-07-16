package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// DefaultRefreshTokenTTL is the fallback refresh token TTL when the system setting
// 'auth.session.refresh_days' is not configured. Matches the catalog default.
const DefaultRefreshTokenTTL = 30 * 24 * time.Hour // 30 days

// DefaultAccessTokenTTL is the fallback access token TTL when the system setting
// 'auth.session.access_token_ttl_minutes' is not configured. Matches the catalog default.
const DefaultAccessTokenTTL = 15 * time.Minute

// getRefreshTokenTTL reads the refresh token TTL from system_settings with a
// fallback to the 30-day default. The setting key is 'auth.session.refresh_days'
// and the value is an integer number of days.
func getRefreshTokenTTL(ctx context.Context, settings SettingsReader, tenantID pgtype.UUID) time.Duration {
	if settings == nil {
		return DefaultRefreshTokenTTL
	}
	days := settings.GetIntOr(ctx, tenantID, "auth.session.refresh_days", 30)
	if days <= 0 {
		return DefaultRefreshTokenTTL
	}
	return time.Duration(days) * 24 * time.Hour
}

// getAccessTokenTTL reads the access token TTL from system_settings with a
// fallback to the 15-minute default. The setting key is
// 'auth.session.access_token_ttl_minutes' and the value is an integer number
// of minutes.
func getAccessTokenTTL(ctx context.Context, settings SettingsReader, tenantID pgtype.UUID) time.Duration {
	if settings == nil {
		return DefaultAccessTokenTTL
	}
	minutes := settings.GetIntOr(ctx, tenantID, "auth.session.access_token_ttl_minutes", 15)
	if minutes <= 0 {
		return DefaultAccessTokenTTL
	}
	return time.Duration(minutes) * time.Minute
}
