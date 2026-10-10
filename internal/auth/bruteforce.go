package auth

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"
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

// MaxLoginAttemptsCap caps the admin-configured 'security.max_login_attempts'
// so a huge value cannot effectively disable brute-force lockout.
const MaxLoginAttemptsCap = 100

// MaxFailWindow caps the admin-configured 'security.fail_window_minutes' so
// a huge value cannot overflow the int64 duration multiplication or pin
// counters in Valkey effectively forever.
const MaxFailWindow = 24 * time.Hour

// MaxLoginAttempts reads the threshold from system_settings (security.max_login_attempts).
func MaxLoginAttempts(ctx context.Context, reader SettingsReader) int {
	if val, err := reader.GetSettingInt(ctx, "security.max_login_attempts"); err == nil && val > 0 {
		if val > MaxLoginAttemptsCap {
			slog.Warn("max login attempts clamped to max", "requested", val, "max", MaxLoginAttemptsCap)
			return MaxLoginAttemptsCap
		}

		return val
	}

	return DefaultMaxLoginAttempts
}

// FailWindow reads the lockout window from system_settings (security.fail_window_minutes).
func FailWindow(ctx context.Context, reader SettingsReader) time.Duration {
	if val, err := reader.GetSettingInt(ctx, "security.fail_window_minutes"); err == nil && val > 0 {
		// Compare in minute units before converting: huge values would
		// overflow int64 nanoseconds during multiplication.
		if val > 1440 {
			slog.Warn("fail window clamped to max", "requested_minutes", val, "max", MaxFailWindow.String())
			return MaxFailWindow
		}

		return time.Duration(val) * time.Minute
	}

	return DefaultFailWindow
}

// CheckLoginAttempt returns whether a login is allowed and how long
// to wait if it is not. The identifier should combine both the
// account (email) and IP address so either can trigger a lockout.
// maxAttempts and failWindow are read from system_settings to
// allow admin-configurable lockout policy per platform.
//
// Fail-CLOSED: on store error the attempt is denied with a short
// retryAfter so a blind brute-force cannot proceed while the counter
// backend is down. Callers must surface the 429 + Retry-After.
func CheckLoginAttempt(ctx context.Context, store BruteForceStore, identifier string, maxAttempts int) (allowed bool, retryAfter time.Duration) {
	count, err := store.GetFailCount(ctx, identifier)
	if err != nil {
		// Store error (Valkey down) — fail closed: deny the attempt with
		// a short backoff so password guessing cannot proceed unchecked
		// while the failure counter is unavailable.
		slog.Warn("brute-force check failed, denying login attempt", "identifier", identifier, "error", err)
		return false, 30 * time.Second
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
