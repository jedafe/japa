package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// ResetTokenTTL is the validity period for password reset tokens.
	ResetTokenTTL = 1 * time.Hour
	// VerificationTokenTTL is the validity period for email verification tokens.
	VerificationTokenTTL = 24 * time.Hour
)

// ResetTokenRecord represents a password reset token from the database.
type ResetTokenRecord struct {
	UserID    pgtype.UUID
	TenantID  pgtype.UUID
	ExpiresAt time.Time
	UsedAt    pgtype.Timestamptz
}

// VerificationTokenRecord represents an email verification token from the database.
type VerificationTokenRecord struct {
	UserID     pgtype.UUID
	TenantID   pgtype.UUID
	ExpiresAt  time.Time
	VerifiedAt pgtype.Timestamptz
}

// GenerateResetToken creates a cryptographically random token and its SHA-256 hash.
// The raw token is sent to the user; the hash is stored in the database.
func GenerateResetToken() (token, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generating reset token: %w", err)
	}

	token = hex.EncodeToString(bytes)
	h := sha256.Sum256(bytes)
	hash = hex.EncodeToString(h[:])

	return token, hash, nil
}

// GenerateVerificationToken creates a cryptographically random token and its SHA-256 hash.
func GenerateVerificationToken() (token, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generating verification token: %w", err)
	}

	token = hex.EncodeToString(bytes)
	h := sha256.Sum256(bytes)
	hash = hex.EncodeToString(h[:])

	return token, hash, nil
}

// InvalidateAllUserSessions revokes all active refresh tokens for a user.
// This forces the user to re-authenticate on all devices after a password reset.
func InvalidateAllUserSessions(ctx context.Context, store Querier, userID, tenantID pgtype.UUID) error {
	// Revoke all non-revoked refresh tokens for this user.
	// The DBStore.RevokeRefreshToken method only revokes a single token by hash,
	// so we need a bulk operation here.
	return store.RevokeAllRefreshTokens(ctx, tenantID, userID)
}
