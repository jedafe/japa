package auth

import (
	"context"
	"fmt"
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

// MaxLoginAttempts is the threshold before exponential backoff kicks in.
const MaxLoginAttempts = 5

// FailWindow is how long the failure counter persists. If no further
// failures occur within this window, the counter resets.
const FailWindow = 15 * time.Minute

// CheckLoginAttempt returns whether a login is allowed and how long
// to wait if it is not. The identifier should combine both the
// account (email) and IP address so either can trigger a lockout.
func CheckLoginAttempt(ctx context.Context, store BruteForceStore, identifier string) (allowed bool, retryAfter time.Duration) {
	count, err := store.GetFailCount(ctx, identifier)
	if err != nil {
		// On store error, allow the attempt - don't lock users out
		// due to infrastructure issues
		return true, 0
	}
	if count >= MaxLoginAttempts {
		backoff := time.Duration(math.Pow(2, float64(count-MaxLoginAttempts+1))) * time.Second
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
