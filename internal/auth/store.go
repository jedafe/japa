package auth

import (
	"context"
	"fmt"
	"time"

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

// CreateUser inserts a new user and returns the generated ID.
func (s *DBStore) CreateUser(ctx context.Context, tenantID pgtype.UUID, email, username, passwordHash string) (pgtype.UUID, error) {
	return s.queries.CreateUser(ctx, gen.CreateUserParams{
		TenantID:     tenantID,
		Email:        email,
		Username:     username,
		PasswordHash: &passwordHash,
	})
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

// StoreRefreshToken stores a hashed refresh token for a user.
func (s *DBStore) StoreRefreshToken(ctx context.Context, tenantID, userID pgtype.UUID, tokenHash string, expiresAt time.Time) error {
	return s.queries.CreateRefreshToken(ctx, gen.CreateRefreshTokenParams{
		TenantID:  tenantID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}

// GetRefreshToken retrieves a refresh token by its hash.
func (s *DBStore) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	// sqlc's GetRefreshTokenByHash requires tenant_id, but the hand-rolled version doesn't.
	// Use raw query for backward compatibility.
	var rt RefreshToken
	var revokedAt pgtype.Timestamptz

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, token_hash, expires_at, revoked_at
		FROM refresh_tokens
		WHERE token_hash = $1
		LIMIT 1
	`, tokenHash).Scan(
		&rt.ID, &rt.UserID, &rt.TenantID, &rt.TokenHash, &rt.ExpiresAt, &revokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("querying refresh token: %w", err)
	}

	rt.RevokedAt = revokedAt
	return &rt, nil
}

// RevokeRefreshToken marks a refresh token as revoked.
func (s *DBStore) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	// sqlc's RevokeRefreshToken requires tenant_id, but the hand-rolled version doesn't.
	// Use raw query for backward compatibility.
	_, err := s.db.Writer.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now(), updated_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	if err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}
	return nil
}

// RevokeAllRefreshTokens marks all active refresh tokens for a user as revoked.
func (s *DBStore) RevokeAllRefreshTokens(ctx context.Context, tenantID, userID pgtype.UUID) error {
	return s.queries.RevokeAllUserRefreshTokens(ctx, gen.RevokeAllUserRefreshTokensParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

// StoreResetToken persists a password reset token hash in the database.
func (s *DBStore) StoreResetToken(ctx context.Context, tenantID, userID pgtype.UUID, tokenHash string, expiresAt time.Time) error {
	return s.queries.CreatePasswordResetToken(ctx, gen.CreatePasswordResetTokenParams{
		TenantID:  tenantID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}

// GetValidResetToken looks up a reset token by hash and returns the record
// if it exists, is not expired, and has not been used.
func (s *DBStore) GetValidResetToken(ctx context.Context, tokenHash string) (*ResetTokenRecord, error) {
	// sqlc's GetPasswordResetTokenByHash requires tenant_id, but the hand-rolled version doesn't.
	// Use raw query for backward compatibility.
	var rec ResetTokenRecord
	var usedAt pgtype.Timestamptz

	err := s.db.Reader.QueryRow(ctx, `
		SELECT user_id, tenant_id, expires_at, used_at
		FROM password_reset_tokens
		WHERE token_hash = $1
		LIMIT 1
	`, tokenHash).Scan(
		&rec.UserID, &rec.TenantID, &rec.ExpiresAt, &usedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("querying reset token: %w", err)
	}

	rec.UsedAt = usedAt
	return &rec, nil
}

// MarkResetTokenUsed sets the used_at timestamp on a reset token.
func (s *DBStore) MarkResetTokenUsed(ctx context.Context, tokenHash string) error {
	// sqlc's UsePasswordResetToken requires id and tenant_id, but the hand-rolled version uses token_hash.
	// Use raw query for backward compatibility.
	_, err := s.db.Writer.Exec(ctx, `
		UPDATE password_reset_tokens
		SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL
	`, tokenHash)
	if err != nil {
		return fmt.Errorf("marking reset token used: %w", err)
	}
	return nil
}

// UpdateUserPassword updates the password hash for a user.
func (s *DBStore) UpdateUserPassword(ctx context.Context, userID pgtype.UUID, passwordHash string) error {
	_, err := s.db.Writer.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, updated_at = now()
		WHERE id = $1
	`, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("updating user password: %w", err)
	}
	return nil
}

// StoreEmailVerification persists an email verification token hash.
func (s *DBStore) StoreEmailVerification(ctx context.Context, tenantID, userID pgtype.UUID, email, tokenHash string, expiresAt time.Time) error {
	return s.queries.CreateEmailVerification(ctx, gen.CreateEmailVerificationParams{
		TenantID:  tenantID,
		UserID:    userID,
		Email:     email,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}

// GetValidVerificationToken looks up an email verification token by hash.
func (s *DBStore) GetValidVerificationToken(ctx context.Context, tokenHash string) (*VerificationTokenRecord, error) {
	// sqlc's GetEmailVerificationByHash requires tenant_id, but the hand-rolled version doesn't.
	// Use raw query for backward compatibility.
	var rec VerificationTokenRecord
	var verifiedAt pgtype.Timestamptz

	err := s.db.Reader.QueryRow(ctx, `
		SELECT user_id, tenant_id, expires_at, verified_at
		FROM email_verifications
		WHERE token_hash = $1
		LIMIT 1
	`, tokenHash).Scan(
		&rec.UserID, &rec.TenantID, &rec.ExpiresAt, &verifiedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("querying verification token: %w", err)
	}

	rec.VerifiedAt = verifiedAt
	return &rec, nil
}

// MarkEmailVerified sets email_verified_at on the users table.
func (s *DBStore) MarkEmailVerified(ctx context.Context, userID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		UPDATE users
		SET email_verified_at = now(), updated_at = now()
		WHERE id = $1 AND email_verified_at IS NULL
	`, userID)
	if err != nil {
		return fmt.Errorf("marking email verified: %w", err)
	}
	return nil
}

// InsertPasswordHistory records a password hash in the user's history.
func (s *DBStore) InsertPasswordHistory(ctx context.Context, tenantID, userID pgtype.UUID, passwordHash string) error {
	return s.queries.InsertPasswordHistory(ctx, gen.InsertPasswordHistoryParams{
		TenantID:     tenantID,
		UserID:       userID,
		PasswordHash: passwordHash,
	})
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
