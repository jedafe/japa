package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// TOTPStoreImpl implements TOTPStore using the application database.
type TOTPStoreImpl struct {
	db *db.DB
}

// NewTOTPStore creates a TOTPStoreImpl backed by the given database.
func NewTOTPStore(database *db.DB) *TOTPStoreImpl {
	return &TOTPStoreImpl{db: database}
}

// GetTOTPSecret retrieves the TOTP credential for a user.
func (s *TOTPStoreImpl) GetTOTPSecret(ctx context.Context, userID pgtype.UUID) (*TOTPRecord, error) {
	query := `
		SELECT secret_encrypted, (enabled_at IS NOT NULL) AS enabled
		FROM totp_credentials
		WHERE user_id = $1
		LIMIT 1
	`
	var record TOTPRecord
	err := s.db.Reader.QueryRow(ctx, query, userID).Scan(&record.SecretEncrypted, &record.Enabled)
	if err != nil {
		return nil, fmt.Errorf("querying TOTP secret: %w", err)
	}
	return &record, nil
}

// UpsertTOTPSecret inserts or updates the TOTP secret for a user.
// The secret is stored encrypted; enabled_at is left NULL until EnableTOTP.
func (s *TOTPStoreImpl) UpsertTOTPSecret(ctx context.Context, userID pgtype.UUID, secretEncrypted string) error {
	query := `
		INSERT INTO totp_credentials (tenant_id, user_id, secret_encrypted)
		SELECT tenant_id, $1, $2
		FROM users
		WHERE id = $1
		ON CONFLICT (tenant_id, user_id) DO UPDATE
		SET secret_encrypted = EXCLUDED.secret_encrypted,
		    updated_at = now()
	`
	_, err := s.db.Writer.Exec(ctx, query, userID, secretEncrypted)
	if err != nil {
		return fmt.Errorf("upserting TOTP secret: %w", err)
	}
	return nil
}

// EnableTOTP marks 2FA as enabled for a user by setting enabled_at.
func (s *TOTPStoreImpl) EnableTOTP(ctx context.Context, userID pgtype.UUID) error {
	query := `
		UPDATE totp_credentials
		SET enabled_at = now(), updated_at = now()
		WHERE user_id = $1 AND enabled_at IS NULL
	`
	tag, err := s.db.Writer.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("enabling TOTP: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("TOTP credential not found or already enabled")
	}
	return nil
}

// DisableTOTP marks 2FA as disabled for a user by clearing enabled_at.
func (s *TOTPStoreImpl) DisableTOTP(ctx context.Context, userID pgtype.UUID) error {
	query := `
		UPDATE totp_credentials
		SET enabled_at = NULL, updated_at = now()
		WHERE user_id = $1 AND enabled_at IS NOT NULL
	`
	tag, err := s.db.Writer.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("disabling TOTP: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("TOTP is not enabled for this user")
	}
	return nil
}

// OAuthStoreImpl implements OAuthStore using the application database.
type OAuthStoreImpl struct {
	db *db.DB
}

// NewOAuthStore creates an OAuthStoreImpl backed by the given database.
func NewOAuthStore(database *db.DB) *OAuthStoreImpl {
	return &OAuthStoreImpl{db: database}
}

// GetOAuthAccount retrieves a linked OAuth account by provider and provider user ID.
func (s *OAuthStoreImpl) GetOAuthAccount(ctx context.Context, tenantID pgtype.UUID, provider, providerUserID string) (*OAuthAccount, error) {
	query := `
		SELECT id, user_id, provider, provider_user_id
		FROM oauth_accounts
		WHERE tenant_id = $1 AND provider = $2 AND provider_user_id = $3
		LIMIT 1
	`
	var acct OAuthAccount
	err := s.db.Reader.QueryRow(ctx, query, tenantID, provider, providerUserID).Scan(
		&acct.ID, &acct.UserID, &acct.Provider, &acct.ProviderUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying OAuth account: %w", err)
	}
	return &acct, nil
}

// GetOAuthAccountByUser retrieves a linked OAuth account by user ID and provider.
func (s *OAuthStoreImpl) GetOAuthAccountByUser(ctx context.Context, tenantID pgtype.UUID, userID pgtype.UUID, provider string) (*OAuthAccount, error) {
	query := `
		SELECT id, user_id, provider, provider_user_id
		FROM oauth_accounts
		WHERE tenant_id = $1 AND user_id = $2 AND provider = $3
		LIMIT 1
	`
	var acct OAuthAccount
	err := s.db.Reader.QueryRow(ctx, query, tenantID, userID, provider).Scan(
		&acct.ID, &acct.UserID, &acct.Provider, &acct.ProviderUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying OAuth account by user: %w", err)
	}
	return &acct, nil
}

// LinkOAuthAccount creates a new OAuth account linkage.
func (s *OAuthStoreImpl) LinkOAuthAccount(ctx context.Context, tenantID, userID pgtype.UUID, provider, providerUserID string) error {
	query := `
		INSERT INTO oauth_accounts (tenant_id, user_id, provider, provider_user_id)
		VALUES ($1, $2, $3, $4)
	`
	_, err := s.db.Writer.Exec(ctx, query, tenantID, userID, provider, providerUserID)
	if err != nil {
		return fmt.Errorf("linking OAuth account: %w", err)
	}
	return nil
}

// GetUserByEmail looks up a user by email within a tenant.
// Delegates to the auth DBStore query pattern.
func (s *OAuthStoreImpl) GetUserByEmail(ctx context.Context, tenantID pgtype.UUID, email string) (*User, error) {
	query := `
		SELECT id, tenant_id, email, password_hash, username, profile_type, status
		FROM users
		WHERE tenant_id = $1 AND email = $2
		LIMIT 1
	`
	var u User
	err := s.db.Reader.QueryRow(ctx, query, tenantID, email).Scan(
		&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Username, &u.ProfileType, &u.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user by email: %w", err)
	}
	return &u, nil
}
