package auth

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// DBStore implements the Querier interface using pgxpool.
// Reads are routed through the reader pool; writes through the writer pool.
type DBStore struct {
	db    *db.DB
	read  *gen.Queries
	write *gen.Queries
}

// NewDBStore creates a DBStore backed by the given database.
func NewDBStore(database *db.DB) *DBStore {
	return &DBStore{
		db:    database,
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// Queries returns the sqlc-generated queries for direct access.
// This returns the writer pool because auth handlers use it for both
// reads and writes (e.g. UpdateLastLoginAt, CreateRefreshToken).
func (s *DBStore) Queries() *gen.Queries {
	return s.write
}

// GetUserByEmail looks up a user by email within a platform.
func (s *DBStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row, err := s.read.GetUserByEmail(ctx,

		email,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user by email: %w", err)
	}

	var passwordHash pgtype.Text
	if row.PasswordHash != nil {
		passwordHash = pgtype.Text{String: *row.PasswordHash, Valid: true}
	}

	return &User{
		ID: row.ID,

		Email:           row.Email,
		PasswordHash:    passwordHash,
		Username:        row.Username,
		ProfileType:     row.ProfileType,
		Status:          row.Status,
		EmailVerifiedAt: row.EmailVerifiedAt,
		IsBot:           row.IsBot,
	}, nil
}

// GetRefreshToken retrieves a refresh token by its hash.
func (s *DBStore) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	row, err := s.read.GetRefreshTokenByHashGlobal(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("querying refresh token: %w", err)
	}

	return &RefreshToken{
		ID:     row.ID,
		UserID: row.UserID,

		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt.Time,
		RevokedAt: row.RevokedAt,
	}, nil
}

// RevokeRefreshToken marks a refresh token as revoked.
func (s *DBStore) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	return s.write.RevokeRefreshTokenByHash(ctx, tokenHash)
}

// GetValidResetToken looks up a reset token by hash and returns the record
// if it exists, is not expired, and has not been used.
func (s *DBStore) GetValidResetToken(ctx context.Context, tokenHash string) (*ResetTokenRecord, error) {
	row, err := s.read.GetPasswordResetTokenByHashGlobal(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("querying reset token: %w", err)
	}

	return &ResetTokenRecord{
		UserID: row.UserID,

		ExpiresAt: row.ExpiresAt.Time,
		UsedAt:    row.UsedAt,
	}, nil
}

// MarkResetTokenUsed sets the used_at timestamp on a reset token.
func (s *DBStore) MarkResetTokenUsed(ctx context.Context, tokenHash string) error {
	return s.write.UsePasswordResetTokenByHash(ctx, tokenHash)
}

// UpdateUserPassword updates the password hash for a user.
func (s *DBStore) UpdateUserPassword(ctx context.Context, userID pgtype.UUID, passwordHash string) error {
	return s.write.UpdateUserPasswordByUserID(ctx, gen.UpdateUserPasswordByUserIDParams{
		UserID:       userID,
		PasswordHash: &passwordHash,
	})
}

// GetValidVerificationToken looks up an email verification token by hash.
func (s *DBStore) GetValidVerificationToken(ctx context.Context, tokenHash string) (*VerificationTokenRecord, error) {
	row, err := s.read.GetEmailVerificationByHashGlobal(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("querying verification token: %w", err)
	}

	return &VerificationTokenRecord{
		UserID: row.UserID,

		ExpiresAt:  row.ExpiresAt.Time,
		VerifiedAt: row.VerifiedAt,
	}, nil
}

// MarkEmailVerified sets email_verified_at on the users table.
func (s *DBStore) MarkEmailVerified(ctx context.Context, userID pgtype.UUID) error {
	return s.write.MarkEmailVerifiedByUserID(ctx, userID)
}

// GetRecentPasswordHashes returns the most recent N password hashes for a user,
// ordered by creation time descending (newest first).
func (s *DBStore) GetRecentPasswordHashes(ctx context.Context, userID pgtype.UUID, limit int) ([]string, error) {
	if limit < 0 {
		limit = 0
	} else if limit > math.MaxInt32 {
		limit = math.MaxInt32
	}

	rows, err := s.read.GetRecentPasswordHashes(ctx, gen.GetRecentPasswordHashesParams{
		UserID:    userID,
		HashLimit: int32(limit), // #nosec G115 -- bounded by the MaxInt32 check above
	})
	if err != nil {
		return nil, fmt.Errorf("querying password history: %w", err)
	}

	var hashes []string
	hashes = append(hashes, rows...)

	return hashes, nil
}

// IsUserInGroup checks if a user belongs to a group identified by slug.
func (s *DBStore) IsUserInGroup(ctx context.Context, arg gen.IsUserInGroupParams) (bool, error) {
	return s.read.IsUserInGroup(ctx, arg)
}
