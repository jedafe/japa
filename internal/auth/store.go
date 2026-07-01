package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// DBStore implements the Querier interface using pgxpool.
type DBStore struct {
	db *db.DB
}

// NewDBStore creates a DBStore backed by the given database.
func NewDBStore(database *db.DB) *DBStore {
	return &DBStore{db: database}
}

// CreateUser inserts a new user and returns the generated ID.
func (s *DBStore) CreateUser(ctx context.Context, tenantID pgtype.UUID, email, username, passwordHash string) (pgtype.UUID, error) {
	var id pgtype.UUID
	query := `
		INSERT INTO users (tenant_id, email, username, password_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`
	err := s.db.Writer.QueryRow(ctx, query, tenantID, email, username, passwordHash).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating user: %w", err)
	}
	return id, nil
}

// GetUserByEmail looks up a user by email within a tenant.
func (s *DBStore) GetUserByEmail(ctx context.Context, tenantID pgtype.UUID, email string) (*User, error) {
	query := `
		SELECT id, tenant_id, email, password_hash, username, profile_type, status
		FROM users
		WHERE tenant_id = $1 AND email = $2
		LIMIT 1
	`
	var u User
	var passwordHash pgtype.Text
	err := s.db.Reader.QueryRow(ctx, query, tenantID, email).Scan(
		&u.ID, &u.TenantID, &u.Email, &passwordHash, &u.Username, &u.ProfileType, &u.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user by email: %w", err)
	}
	u.PasswordHash = passwordHash
	return &u, nil
}

// StoreRefreshToken stores a hashed refresh token for a user.
func (s *DBStore) StoreRefreshToken(ctx context.Context, tenantID, userID pgtype.UUID, tokenHash string, expiresAt time.Time) error {
	query := `
		INSERT INTO refresh_tokens (tenant_id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := s.db.Writer.Exec(ctx, query, tenantID, userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("storing refresh token: %w", err)
	}
	return nil
}

// GetRefreshToken retrieves a refresh token by its hash.
func (s *DBStore) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	query := `
		SELECT id, user_id, tenant_id, token_hash, expires_at, revoked_at
		FROM refresh_tokens
		WHERE token_hash = $1
		LIMIT 1
	`
	var rt RefreshToken
	var revokedAt pgtype.Timestamptz
	err := s.db.Reader.QueryRow(ctx, query, tokenHash).Scan(
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
	query := `
		UPDATE refresh_tokens
		SET revoked_at = now(), updated_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`
	_, err := s.db.Writer.Exec(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}
	return nil
}

// RevokeAllRefreshTokens marks all active refresh tokens for a user as revoked.
func (s *DBStore) RevokeAllRefreshTokens(ctx context.Context, tenantID, userID pgtype.UUID) error {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = now(), updated_at = now()
		WHERE tenant_id = $1 AND user_id = $2 AND revoked_at IS NULL
	`
	_, err := s.db.Writer.Exec(ctx, query, tenantID, userID)
	if err != nil {
		return fmt.Errorf("revoking all refresh tokens: %w", err)
	}
	return nil
}

// StoreResetToken persists a password reset token hash in the database.
func (s *DBStore) StoreResetToken(ctx context.Context, tenantID, userID pgtype.UUID, tokenHash string, expiresAt time.Time) error {
	query := `
		INSERT INTO password_reset_tokens (tenant_id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := s.db.Writer.Exec(ctx, query, tenantID, userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("storing reset token: %w", err)
	}
	return nil
}

// GetValidResetToken looks up a reset token by hash and returns the record
// if it exists, is not expired, and has not been used.
func (s *DBStore) GetValidResetToken(ctx context.Context, tokenHash string) (*ResetTokenRecord, error) {
	query := `
		SELECT user_id, tenant_id, expires_at, used_at
		FROM password_reset_tokens
		WHERE token_hash = $1
		LIMIT 1
	`
	var rec ResetTokenRecord
	var usedAt pgtype.Timestamptz
	err := s.db.Reader.QueryRow(ctx, query, tokenHash).Scan(
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
	query := `
		UPDATE password_reset_tokens
		SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL
	`
	_, err := s.db.Writer.Exec(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("marking reset token used: %w", err)
	}
	return nil
}

// UpdateUserPassword updates the password hash for a user.
func (s *DBStore) UpdateUserPassword(ctx context.Context, userID pgtype.UUID, passwordHash string) error {
	query := `
		UPDATE users
		SET password_hash = $2, updated_at = now()
		WHERE id = $1
	`
	_, err := s.db.Writer.Exec(ctx, query, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("updating user password: %w", err)
	}
	return nil
}

// StoreEmailVerification persists an email verification token hash.
func (s *DBStore) StoreEmailVerification(ctx context.Context, tenantID, userID pgtype.UUID, email, tokenHash string, expiresAt time.Time) error {
	query := `
		INSERT INTO email_verifications (tenant_id, user_id, email, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := s.db.Writer.Exec(ctx, query, tenantID, userID, email, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("storing email verification: %w", err)
	}
	return nil
}

// GetValidVerificationToken looks up an email verification token by hash.
func (s *DBStore) GetValidVerificationToken(ctx context.Context, tokenHash string) (*VerificationTokenRecord, error) {
	query := `
		SELECT user_id, tenant_id, expires_at, verified_at
		FROM email_verifications
		WHERE token_hash = $1
		LIMIT 1
	`
	var rec VerificationTokenRecord
	var verifiedAt pgtype.Timestamptz
	err := s.db.Reader.QueryRow(ctx, query, tokenHash).Scan(
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
	query := `
		UPDATE users
		SET email_verified_at = now(), updated_at = now()
		WHERE id = $1 AND email_verified_at IS NULL
	`
	_, err := s.db.Writer.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("marking email verified: %w", err)
	}
	return nil
}

// InsertPasswordHistory records a password hash in the user's history.
func (s *DBStore) InsertPasswordHistory(ctx context.Context, tenantID, userID pgtype.UUID, passwordHash string) error {
	query := `
		INSERT INTO password_history (tenant_id, user_id, password_hash)
		VALUES ($1, $2, $3)
	`
	_, err := s.db.Writer.Exec(ctx, query, tenantID, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("inserting password history: %w", err)
	}
	return nil
}

// GetRecentPasswordHashes returns the most recent N password hashes for a user,
// ordered by creation time descending (newest first).
func (s *DBStore) GetRecentPasswordHashes(ctx context.Context, userID pgtype.UUID, limit int) ([]string, error) {
	query := `
		SELECT password_hash
		FROM password_history
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := s.db.Reader.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying password history: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("scanning password history: %w", err)
		}
		hashes = append(hashes, hash)
	}
	return hashes, rows.Err()
}
