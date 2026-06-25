// Package auth — password hashing and validation.
package auth

import (
	"errors"
	"fmt"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is the bcrypt cost factor. Cost 12 is the explicit value
// from the JapaConnect security audit (OWASP A02 row), not a default.
const BcryptCost = 12

// PasswordPolicy defines the rules for acceptable passwords.
// These defaults can be overridden by system_settings at runtime.
type PasswordPolicy struct {
	MinLength     int
	MaxLength     int
	RequireUpper  bool
	RequireNumber bool
	RequireSymbol bool
}

// DefaultPasswordPolicy returns the default password policy.
// Admin can override these via system_settings keys:
//   auth.password.min_length (default: 8)
//   auth.password.require_uppercase (default: true)
//   auth.password.require_number (default: true)
//   auth.password.require_symbol (default: false)
//   auth.password.max_length (default: 128)
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{
		MinLength:     8,
		MaxLength:     128,
		RequireUpper:  true,
		RequireNumber: true,
		RequireSymbol: false,
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
