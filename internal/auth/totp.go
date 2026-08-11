// Package auth — TOTP (Time-based One-Time Password) two-factor authentication.
//
// Provides secret generation, verification, and enable/disable lifecycle
// for the totp_credentials table. Secrets are stored encrypted at rest;
// only the raw secret is returned to the caller during setup for QR code
// generation — it is never persisted in plaintext.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pquerna/otp/totp"
	"github.com/jedafe/japa/internal/crypto"
)

// defaultTOTPIssuer is the fallback issuer label shown in authenticator apps.
const defaultTOTPIssuer = "App"

// TOTPStore defines the database operations required for TOTP lifecycle.
type TOTPStore interface {
	GetTOTPSecret(ctx context.Context, userID pgtype.UUID) (*TOTPRecord, error)
	UpsertTOTPSecret(ctx context.Context, userID pgtype.UUID, secretEncrypted string) error
	EnableTOTP(ctx context.Context, userID pgtype.UUID) error
	DisableTOTP(ctx context.Context, userID pgtype.UUID) error
	SaveBackupCodes(ctx context.Context, userID pgtype.UUID, hashedCodes []string) error
	GetUserEmail(ctx context.Context, userID pgtype.UUID) (string, error)
}

// TOTPRecord holds the stored TOTP credential for a user.
type TOTPRecord struct {
	SecretEncrypted string
	Enabled         bool
}

// GenerateTOTPSecret creates a new TOTP secret for the given user.
// Returns the raw secret (for QR code generation) and an otpauth:// URL
// that authenticator apps can scan. The secret is NOT stored — call
// EnableTOTP after the user verifies a code.
func GenerateTOTPSecret(userID, email, issuer string) (secret, qrCodeURL string, err error) {
	if userID == "" || email == "" {
		return "", "", errors.New("user ID and email are required")
	}

	if issuer == "" {
		issuer = defaultTOTPIssuer
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: email,
	})
	if err != nil {
		return "", "", fmt.Errorf("generating TOTP key: %w", err)
	}

	return key.Secret(), key.URL(), nil
}

// GenerateBackupCodes creates a set of one-time backup codes for account
// recovery when the user loses access to their authenticator device.
func GenerateBackupCodes() ([]string, error) {
	codes := make([]string, 10)
	for i := range codes {
		buf := make([]byte, 4)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("generating backup code: %w", err)
		}
		// Format: XXXX-XXXX (8 hex characters with a dash)
		codes[i] = fmt.Sprintf(
			"%s-%s",
			base32.StdEncoding.EncodeToString(buf[:2])[:4],
			base32.StdEncoding.EncodeToString(buf[2:])[:4],
		)
	}

	return codes, nil
}

// VerifyTOTP validates a 6-digit TOTP code against the given secret.
// Uses a 30-second period and 1 window step (±30 seconds tolerance)
// to handle minor clock skew between the server and the user's device.
func VerifyTOTP(secret, code string) bool {
	secret = strings.TrimSpace(strings.ToUpper(secret))
	return totp.Validate(code, secret)
}

// EnableTOTP verifies the code, then marks 2FA as enabled for the user.
// The secret must have been previously stored via UpsertTOTPSecret.
// Returns an error if the code is invalid or the database operation fails.
func EnableTOTP(ctx context.Context, store TOTPStore, userID pgtype.UUID, code string) error {
	if !userID.Valid {
		return errors.New("invalid user ID")
	}

	record, err := store.GetTOTPSecret(ctx, userID)
	if err != nil || record == nil {
		return errors.New("TOTP not set up — run setup first")
	}

	if record.Enabled {
		return errors.New("2FA is already enabled")
	}

	plaintext, err := decryptSecret(record.SecretEncrypted)
	if err != nil {
		return fmt.Errorf("decrypting TOTP secret: %w", err)
	}

	if !VerifyTOTP(plaintext, code) {
		return errors.New("invalid verification code")
	}

	return store.EnableTOTP(ctx, userID)
}

// DisableTOTP verifies the code, then disables 2FA for the user.
// The user must provide a valid code to confirm the disable action,
// preventing an attacker who gains session access from turning off 2FA.
func DisableTOTP(ctx context.Context, store TOTPStore, userID pgtype.UUID, code string) error {
	if !userID.Valid {
		return errors.New("invalid user ID")
	}

	record, err := store.GetTOTPSecret(ctx, userID)
	if err != nil || record == nil {
		return errors.New("TOTP not set up")
	}

	if !record.Enabled {
		return errors.New("2FA is not enabled")
	}

	plaintext, err := decryptSecret(record.SecretEncrypted)
	if err != nil {
		return fmt.Errorf("decrypting TOTP secret: %w", err)
	}

	if !VerifyTOTP(plaintext, code) {
		return errors.New("invalid verification code")
	}

	return store.DisableTOTP(ctx, userID)
}

func encryptSecret(secret string) (string, error) {
	enc, err := crypto.Encrypt(secret)
	if err != nil {
		return "", fmt.Errorf("auth: encrypt totp secret: %w", err)
	}
	return enc, nil
}

func decryptSecret(encrypted string) (string, error) {
	plaintext, err := crypto.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("auth: decrypt totp secret: %w", err)
	}
	return plaintext, nil
}

// FormatBackupCodes formats backup codes as a JSON-compatible string slice.
func FormatBackupCodes(codes []string) string {
	quoted := make([]string, len(codes))
	for i, c := range codes {
		quoted[i] = fmt.Sprintf("%q", c)
	}

	return fmt.Sprintf("[%s]", strings.Join(quoted, ","))
}

// Now returns the current time. Extracted for testability.
var Now = func() time.Time { return time.Now() }
