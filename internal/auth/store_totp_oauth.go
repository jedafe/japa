package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
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
		INSERT INTO totp_credentials (user_id, secret_encrypted)
		SELECT $1, $2
		FROM users
		WHERE id = $1
		ON CONFLICT (user_id) DO UPDATE
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
		return errors.New("TOTP credential not found or already enabled")
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
		return errors.New("TOTP is not enabled for this user")
	}

	return nil
}

// HasTOTPEnrolled reports whether the user has an active (enabled) TOTP
// enrollment. Implements auth.TOTPEnrollmentChecker for the
// RequireTOTPForAdmin middleware (Section 02 — auth.totp.required_for_admin).
// Returns false when no credential row exists or when enabled_at IS NULL.
func (s *TOTPStoreImpl) HasTOTPEnrolled(ctx context.Context, userID pgtype.UUID) (bool, error) {
	var enabled bool

	err := s.db.Reader.QueryRow(ctx, `SELECT (enabled_at IS NOT NULL) FROM totp_credentials
		 WHERE user_id = $1`, userID).Scan(&enabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("querying TOTP enrollment: %w", err)
	}

	return enabled, nil
}

// SaveBackupCodes stores hashed backup codes for a user.
func (s *TOTPStoreImpl) SaveBackupCodes(ctx context.Context, userID pgtype.UUID, hashedCodes []string) error {
	encoded, err := encodeBackupHashes(hashedCodes)
	if err != nil {
		return fmt.Errorf("encoding backup codes: %w", err)
	}

	_, err = s.db.Writer.Exec(ctx, `UPDATE totp_credentials SET backup_codes = $1, updated_at = now() WHERE user_id = $2`, encoded, userID)
	if err != nil {
		return fmt.Errorf("saving backup codes: %w", err)
	}

	return nil
}

// GetUserEmail retrieves the email for a user, used as the account label
// in the TOTP QR code so authenticator apps display the correct identity.
func (s *TOTPStoreImpl) GetUserEmail(ctx context.Context, userID pgtype.UUID) (string, error) {
	var email string

	err := s.db.Reader.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if err != nil {
		return "", fmt.Errorf("querying user email: %w", err)
	}

	return email, nil
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
func (s *OAuthStoreImpl) GetOAuthAccount(ctx context.Context, provider, providerUserID string) (*OAuthAccount, error) {
	query := `
		SELECT id, user_id, provider, provider_user_id
		FROM oauth_accounts
		WHERE provider = $1 AND provider_user_id = $2
		LIMIT 1
	`

	var acct OAuthAccount

	err := s.db.Reader.QueryRow(ctx, query, provider, providerUserID).Scan(
		&acct.ID, &acct.UserID, &acct.Provider, &acct.ProviderUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying OAuth account: %w", err)
	}

	return &acct, nil
}

// LinkOAuthAccount creates a new OAuth account linkage. ON CONFLICT DO
// NOTHING keeps the call idempotent — callers verify the resolved owner via
// GetOAuthAccount afterwards so a (provider, provider_user_id) link pointing
// at a different user surfaces as a conflict, not a silent session grant.
func (s *OAuthStoreImpl) LinkOAuthAccount(ctx context.Context, userID pgtype.UUID, provider, providerUserID string) error {
	query := `
		INSERT INTO oauth_accounts (user_id, provider, provider_user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`

	_, err := s.db.Writer.Exec(ctx, query, userID, provider, providerUserID)
	if err != nil {
		return fmt.Errorf("linking OAuth account: %w", err)
	}

	return nil
}

// GetUserByEmail looks up a user by email within a platform.
// Delegates to the auth DBStore query pattern.
func (s *OAuthStoreImpl) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, password_hash, username, profile_type, status, email_verified_at, is_bot
		FROM users
		WHERE email = $1
		LIMIT 1
	`

	var u User

	err := s.db.Reader.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Username, &u.ProfileType, &u.Status, &u.EmailVerifiedAt, &u.IsBot,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user by email: %w", err)
	}

	return &u, nil
}

// GetUserByID looks up a user by primary key — the OAuth callback needs the
// full status/bot/email-verification fields to enforce the same login gates
// as the password path (§02, ADR-068).
func (s *OAuthStoreImpl) GetUserByID(ctx context.Context, userID pgtype.UUID) (*User, error) {
	query := `
		SELECT id, email, password_hash, username, profile_type, status, email_verified_at, is_bot
		FROM users
		WHERE id = $1
		LIMIT 1
	`

	var u User

	err := s.db.Reader.QueryRow(ctx, query, userID).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Username, &u.ProfileType, &u.Status, &u.EmailVerifiedAt, &u.IsBot,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user by id: %w", err)
	}

	return &u, nil
}
