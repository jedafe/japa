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
