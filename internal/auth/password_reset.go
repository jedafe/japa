package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

const (
	// ResetTokenTTL is the validity period for password reset tokens.
	ResetTokenTTL = 1 * time.Hour
	// VerificationTokenTTL is the validity period for email verification tokens.
	VerificationTokenTTL = 24 * time.Hour
)

// ResetTokenRecord represents a password reset token from the database.
type ResetTokenRecord struct {
	UserID pgtype.UUID

	ExpiresAt time.Time
	UsedAt    pgtype.Timestamptz
}

// VerificationTokenRecord represents an email verification token from the database.
type VerificationTokenRecord struct {
	UserID pgtype.UUID

	ExpiresAt  time.Time
	VerifiedAt pgtype.Timestamptz
}

// GenerateResetToken creates a cryptographically random token and its SHA-256 hash.
// The raw token is sent to the user; the hash is stored in the database.
//
// The hash MUST be computed over the hex-encoded token string (not the raw
// bytes) because every lookup site hashes the user-supplied token string
// with HashRefreshToken (SHA256 of the string). Hashing raw bytes here
// while lookups hash the hex string would make every stored token
// unfindable — a total password-reset/email-verify DoS.
func GenerateResetToken() (token, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generating reset token: %w", err)
	}

	token = hex.EncodeToString(bytes)
	hash = HashRefreshToken(token)

	return token, hash, nil
}

// GenerateVerificationToken creates a cryptographically random token and its SHA-256 hash.
// Same hash-domain contract as GenerateResetToken: hash over the hex string.
func GenerateVerificationToken() (token, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generating verification token: %w", err)
	}

	token = hex.EncodeToString(bytes)
	hash = HashRefreshToken(token)

	return token, hash, nil
}

// InvalidateAllUserSessions revokes all active refresh tokens for a user.
// This forces the user to re-authenticate on all devices after a password reset.
func InvalidateAllUserSessions(ctx context.Context, queries *gen.Queries, userID pgtype.UUID) error {
	// Revoke all non-revoked refresh tokens for this user.
	return queries.RevokeAllUserRefreshTokens(ctx,
		userID,
	)
}
