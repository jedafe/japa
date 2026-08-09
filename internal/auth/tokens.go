package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// generateRefreshToken creates a cryptographically random refresh token.
// Uses crypto/rand for unpredictability - never math/rand for security tokens.
func generateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}

	return hex.EncodeToString(bytes), nil
}

// HashRefreshToken creates a SHA-256 hash of the refresh token for storage.
// We store the hash (not the raw token) so a database leak does not
// give an attacker valid refresh tokens.
func HashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// hashRefreshToken is the internal alias kept for backward compatibility.
func hashRefreshToken(token string) string { return HashRefreshToken(token) }
