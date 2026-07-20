package auth

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// BruteForceStore is the interface for tracking failed login attempts.
// Implemented by a Valkey/Redis-backed store in production and a mock in tests.
type BruteForceStore interface {
	GetFailCount(ctx context.Context, key string) (int, error)
	IncrFailCount(ctx context.Context, key string) error
	ResetFailCount(ctx context.Context, key string) error
}

// DefaultMaxLoginAttempts is the fallback when system_settings key is unavailable.
const DefaultMaxLoginAttempts = 5

// DefaultFailWindow is the fallback when system_settings key is unavailable.
const DefaultFailWindow = 15 * time.Minute

// MaxLoginAttempts reads the threshold from system_settings (security.max_login_attempts).
func MaxLoginAttempts(ctx context.Context, reader SettingsReader, tenantID pgtype.UUID) int {
	if val, err := reader.GetSettingInt(ctx, tenantID, "security.max_login_attempts"); err == nil && val > 0 {
		return val
	}
	return DefaultMaxLoginAttempts
}

// FailWindow reads the lockout window from system_settings (security.fail_window_minutes).
func FailWindow(ctx context.Context, reader SettingsReader, tenantID pgtype.UUID) time.Duration {
	if val, err := reader.GetSettingInt(ctx, tenantID, "security.fail_window_minutes"); err == nil && val > 0 {
		return time.Duration(val) * time.Minute
	}
	return DefaultFailWindow
}

// CheckLoginAttempt returns whether a login is allowed and how long
// to wait if it is not. The identifier should combine both the
// account (email) and IP address so either can trigger a lockout.
// maxAttempts and failWindow are read from system_settings to
// allow admin-configurable lockout policy per tenant.
func CheckLoginAttempt(ctx context.Context, store BruteForceStore, identifier string, maxAttempts int) (allowed bool, retryAfter time.Duration) {
	count, err := store.GetFailCount(ctx, identifier)
	if err != nil {
		// On store error, allow the attempt - don't lock users out
		// due to infrastructure issues
		return true, 0
	}

	if count >= maxAttempts {
		backoff := time.Duration(math.Pow(2, float64(count-maxAttempts+1))) * time.Second
		if backoff > 30*time.Minute {
			backoff = 30 * time.Minute
		}

		return false, backoff
	}

	return true, 0
}

// RecordFailedLogin increments the failure counter for the identifier.
func RecordFailedLogin(ctx context.Context, store BruteForceStore, identifier string) error {
	if err := store.IncrFailCount(ctx, identifier); err != nil {
		return fmt.Errorf("incrementing fail count: %w", err)
	}

	return nil
}

// ResetLoginAttempts clears the failure counter after a successful login.
func ResetLoginAttempts(ctx context.Context, store BruteForceStore, identifier string) error {
	if err := store.ResetFailCount(ctx, identifier); err != nil {
		return fmt.Errorf("resetting fail count: %w", err)
	}

	return nil
}

// LoginIdentifier combines account and IP into a single key so that
// either can independently trigger a lockout.
func LoginIdentifier(email, ip string) string {
	return fmt.Sprintf("account:%s:ip:%s", email, ip)
}
