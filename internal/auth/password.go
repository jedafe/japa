// Package auth — password hashing and validation.
package auth

import (
	"context"
	"errors"
	"fmt"
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

// PasswordHistoryQuerier defines the database operations needed for
// password history checks. Implementations live in store.go.
type PasswordHistoryQuerier interface {
	GetRecentPasswordHashes(ctx context.Context, userID pgtype.UUID, limit int) ([]string, error)
	InsertPasswordHistory(ctx context.Context, tenantID, userID pgtype.UUID, passwordHash string) error
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
			// Allow the password through — logged upstream by the breach checker.
			_ = err //nolint:staticcheck // SA9003: intentional empty branch — error is non-blocking
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
func CheckPasswordHistory(ctx context.Context, store PasswordHistoryQuerier, userID pgtype.UUID, newPassword string, policy PasswordPolicy) error {
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
