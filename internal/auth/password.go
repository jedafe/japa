// Package auth — password hashing and validation.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is the bcrypt cost factor. Cost 12 is the explicit value
// from the JapaConnect security audit (OWASP A02 row), not a default.
const BcryptCost = 12

// PasswordReusedError is returned when a password matches a recent hash
// in the user's password history.
type PasswordReusedError struct {
	Count int // How many recent hashes were checked
}

func (e *PasswordReusedError) Error() string {
	return "password cannot be the same as any of your last passwords"
}

// PasswordHistoryReader defines the interface for reading password history.
type PasswordHistoryReader interface {
	GetRecentPasswordHashes(ctx context.Context, userID pgtype.UUID, limit int) ([]string, error)
}

// PasswordPolicy defines the rules for acceptable passwords.
type PasswordPolicy struct {
	MinLength     int
	MaxLength     int
	RequireUpper  bool
	RequireNumber bool
	RequireSymbol bool
	CheckBreach   bool // When true, validate against HIBP before accepting
	HistoryCount  int  // Number of recent passwords to check against reuse (default: 5)
}

// DefaultPasswordPolicy returns the default password policy.
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{
		MinLength:     8,
		MaxLength:     128,
		RequireUpper:  true,
		RequireNumber: true,
		RequireSymbol: false,
		CheckBreach:   true,
		HistoryCount:  5,
	}
}

// SettingsReader defines the interface for reading settings from the database.
type SettingsReader interface {
	GetSettingInt(ctx context.Context, tenantID pgtype.UUID, key string) (int, error)
	GetSettingBool(ctx context.Context, tenantID pgtype.UUID, key string) (bool, error)
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// PasswordPolicyFromSettings returns a password policy configured from
// system_settings. Falls back to defaults for any unset values. All seven
// policy knobs are now settings-driven (AUDIT-BE2-SETTING-005).
func PasswordPolicyFromSettings(ctx context.Context, reader SettingsReader, tenantID pgtype.UUID) PasswordPolicy {
	policy := DefaultPasswordPolicy()

	if val := reader.GetIntOr(ctx, tenantID, "auth.password.min_length", policy.MinLength); val > 0 {
		policy.MinLength = val
	}

	if val := reader.GetIntOr(ctx, tenantID, "auth.password.max_length", policy.MaxLength); val > 0 {
		policy.MaxLength = val
	}

	if val, err := reader.GetSettingBool(ctx, tenantID, "auth.password.require_uppercase"); err == nil {
		policy.RequireUpper = val
	}

	if val, err := reader.GetSettingBool(ctx, tenantID, "auth.password.require_number"); err == nil {
		policy.RequireNumber = val
	}

	if val, err := reader.GetSettingBool(ctx, tenantID, "auth.password.require_symbol"); err == nil {
		policy.RequireSymbol = val
	}

	if val, err := reader.GetSettingBool(ctx, tenantID, "auth.password.check_breach"); err == nil {
		policy.CheckBreach = val
	}

	if val := reader.GetIntOr(ctx, tenantID, "auth.password.history_count", policy.HistoryCount); val >= 0 {
		policy.HistoryCount = val
	}

	return policy
}

// ValidatePassword checks a password against the policy.
// Returns nil if the password passes, or an error describing
// which requirement was not met.
func ValidatePassword(password string, policy PasswordPolicy) error {
	if len(password) < policy.MinLength {
		return errors.New("password is too short")
	}

	if len(password) > policy.MaxLength {
		return errors.New("password is too long")
	}

	hasUpper := false
	hasNumber := false
	hasSymbol := false

	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsDigit(ch):
			hasNumber = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSymbol = true
		}
	}

	if policy.RequireUpper && !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}

	if policy.RequireNumber && !hasNumber {
		return errors.New("password must contain at least one number")
	}

	if policy.RequireSymbol && !hasSymbol {
		return errors.New("password must contain at least one symbol")
	}

	// Breach check — only runs after structural checks pass.
	// If the HIBP API is unreachable, we degrade gracefully and allow
	// the password (BreachCheckError is non-blocking by design).
	if policy.CheckBreach {
		breached, count, err := CheckPasswordBreach(password)
		if err != nil {
			// Log but do not block — third-party failure must not prevent registration.
			// BreachCheckError means the API call failed, not that the password is breached.
			slog.Warn("password breach check unavailable, allowing password", "error", err)
		}

		if breached {
			return &BreachedError{Count: count}
		}
	}

	return nil
}

// HashPassword hashes a password using bcrypt with the configured cost.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}

	return string(hash), nil
}

// VerifyPassword compares a plaintext password against a bcrypt hash.
// Returns nil if the password matches, an error otherwise.
func VerifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// CheckPasswordHistory queries the last N password hashes for the given user
// and checks whether the new password matches any of them. Returns nil if the
// password is unique, or a PasswordReusedError if it matches a recent hash.
func CheckPasswordHistory(ctx context.Context, store PasswordHistoryReader, userID pgtype.UUID, newPassword string, policy PasswordPolicy) error {
	if policy.HistoryCount <= 0 {
		return nil
	}

	recentHashes, err := store.GetRecentPasswordHashes(ctx, userID, policy.HistoryCount)
	if err != nil {
		// Degradable: if the history table doesn't exist or query fails,
		// don't block the password change. Log and continue.
		return nil //nolint:nilerr // intentional: degradable password history check
	}

	for _, hash := range recentHashes {
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(newPassword)); err == nil {
			return &PasswordReusedError{Count: len(recentHashes)}
		}
	}

	return nil
}
