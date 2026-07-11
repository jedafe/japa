package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// DBStore implements the Querier interface using pgxpool.
type DBStore struct {
	db      *db.DB
	queries *gen.Queries
}

// NewDBStore creates a DBStore backed by the given database.
func NewDBStore(database *db.DB) *DBStore {
	return &DBStore{
		db:      database,
		queries: gen.New(database.Reader),
	}
}

// Queries returns the sqlc-generated queries for direct access.
func (s *DBStore) Queries() *gen.Queries {
	return s.queries
}

// GetUserByEmail looks up a user by email within a tenant.
func (s *DBStore) GetUserByEmail(ctx context.Context, tenantID pgtype.UUID, email string) (*User, error) {
	row, err := s.queries.GetUserByEmail(ctx, gen.GetUserByEmailParams{
		TenantID: tenantID,
		Email:    email,
	})
	if err != nil {
		return nil, fmt.Errorf("querying user by email: %w", err)
	}

	var passwordHash pgtype.Text
	if row.PasswordHash != nil {
		passwordHash = pgtype.Text{String: *row.PasswordHash, Valid: true}
	}

	return &User{
		ID:           row.ID,
		TenantID:     row.TenantID,
		Email:        row.Email,
		PasswordHash: passwordHash,
		Username:     row.Username,
		ProfileType:  row.ProfileType,
		Status:       row.Status,
	}, nil
}

// GetRefreshToken retrieves a refresh token by its hash.
func (s *DBStore) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	row, err := s.queries.GetRefreshTokenByHashGlobal(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("querying refresh token: %w", err)
	}

	return &RefreshToken{
		ID:        row.ID,
		UserID:    row.UserID,
		TenantID:  row.TenantID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt.Time,
		RevokedAt: row.RevokedAt,
	}, nil
}

// RevokeRefreshToken marks a refresh token as revoked.
func (s *DBStore) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	return s.queries.RevokeRefreshTokenByHash(ctx, tokenHash)
}

// GetValidResetToken looks up a reset token by hash and returns the record
// if it exists, is not expired, and has not been used.
func (s *DBStore) GetValidResetToken(ctx context.Context, tokenHash string) (*ResetTokenRecord, error) {
	row, err := s.queries.GetPasswordResetTokenByHashGlobal(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("querying reset token: %w", err)
	}

	return &ResetTokenRecord{
		UserID:    row.UserID,
		TenantID:  row.TenantID,
		ExpiresAt: row.ExpiresAt.Time,
		UsedAt:    row.UsedAt,
	}, nil
}

// MarkResetTokenUsed sets the used_at timestamp on a reset token.
func (s *DBStore) MarkResetTokenUsed(ctx context.Context, tokenHash string) error {
	return s.queries.UsePasswordResetTokenByHash(ctx, tokenHash)
}

// UpdateUserPassword updates the password hash for a user.
func (s *DBStore) UpdateUserPassword(ctx context.Context, userID pgtype.UUID, passwordHash string) error {
	return s.queries.UpdateUserPasswordByUserID(ctx, gen.UpdateUserPasswordByUserIDParams{
		UserID:       userID,
		PasswordHash: &passwordHash,
	})
}

// GetValidVerificationToken looks up an email verification token by hash.
func (s *DBStore) GetValidVerificationToken(ctx context.Context, tokenHash string) (*VerificationTokenRecord, error) {
	row, err := s.queries.GetEmailVerificationByHashGlobal(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("querying verification token: %w", err)
	}

	return &VerificationTokenRecord{
		UserID:     row.UserID,
		TenantID:   row.TenantID,
		ExpiresAt:  row.ExpiresAt.Time,
		VerifiedAt: row.VerifiedAt,
	}, nil
}

// MarkEmailVerified sets email_verified_at on the users table.
func (s *DBStore) MarkEmailVerified(ctx context.Context, userID pgtype.UUID) error {
	return s.queries.MarkEmailVerifiedByUserID(ctx, userID)
}

// GetRecentPasswordHashes returns the most recent N password hashes for a user,
// ordered by creation time descending (newest first).
func (s *DBStore) GetRecentPasswordHashes(ctx context.Context, userID pgtype.UUID, limit int) ([]string, error) {
	rows, err := s.queries.GetRecentPasswordHashes(ctx, gen.GetRecentPasswordHashesParams{
		UserID:    userID,
		HashLimit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("querying password history: %w", err)
	}

	var hashes []string
	for _, row := range rows {
		hashes = append(hashes, row)
	}

	return hashes, nil
}
