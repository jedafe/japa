// Package auth — TOTP (Time-based One-Time Password) two-factor authentication.
//
// Provides secret generation, verification, and enable/disable lifecycle
// for the totp_credentials table. Secrets are stored encrypted at rest;
// only the raw secret is returned to the caller during setup for QR code
// generation — it is never persisted in plaintext.
package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pquerna/otp/totp"
)

// TOTPIssuer is the issuer label shown in authenticator apps.
const TOTPIssuer = "JapaConnect"

// TOTPStore defines the database operations required for TOTP lifecycle.
type TOTPStore interface {
	GetTOTPSecret(ctx context.Context, userID pgtype.UUID) (*TOTPRecord, error)
	UpsertTOTPSecret(ctx context.Context, userID pgtype.UUID, secretEncrypted string) error
	EnableTOTP(ctx context.Context, userID pgtype.UUID) error
	DisableTOTP(ctx context.Context, userID pgtype.UUID) error
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
func GenerateTOTPSecret(userID, email string) (secret, qrCodeURL string, err error) {
	if userID == "" || email == "" {
		return "", "", errors.New("user ID and email are required")
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      TOTPIssuer,
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

// encryptSecret encrypts a TOTP secret using AES-256-GCM with the key from
// JC_TOTP_ENCRYPTION_KEY. The key is derived via SHA-256 so it can be provided
// as any-length passphrase. Returns "v1:<hex nonce>:<hex ciphertext>".
//
// If the encryption key is not configured, the function logs a warning and
// returns an error — in production, missing encryption key must fail closed.
func encryptSecret(secret string) (string, error) {
	key, err := loadEncryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(secret), nil)

	return fmt.Sprintf("v1:%s:%s", hex.EncodeToString(nonce), hex.EncodeToString(ciphertext)), nil
}

// decryptSecret reverses encryptSecret. It parses the "v1:<nonce>:<ciphertext>"
// format and decrypts using AES-256-GCM. Returns an error if the format is
// invalid or the key does not match.
func decryptSecret(encrypted string) (string, error) {
	key, err := loadEncryptionKey()
	if err != nil {
		return "", err
	}

	parts := strings.SplitN(encrypted, ":", 3)
	if len(parts) != 3 || parts[0] != "v1" {
		// Legacy plaintext secret — return as-is for backward compatibility
		// during migration. Once all secrets are re-encrypted, remove this.
		return encrypted, nil
	}

	nonce, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decoding nonce: %w", err)
	}

	ciphertext, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting: %w", err)
	}

	return string(plaintext), nil
}

// loadEncryptionKey reads JC_TOTP_ENCRYPTION_KEY from the environment and
// derives a 32-byte AES-256 key via SHA-256. Returns an error in production
// when the key is missing — we fail closed rather than store plaintext.
func loadEncryptionKey() ([]byte, error) {
	raw := os.Getenv("JC_TOTP_ENCRYPTION_KEY")
	if raw == "" {
		env := os.Getenv("JC_ENV")
		if strings.EqualFold(env, "production") {
			return nil, errors.New("JC_TOTP_ENCRYPTION_KEY is required in production")
		}

		slog.Warn("JC_TOTP_ENCRYPTION_KEY not set — TOTP secrets stored without encryption (dev only)")
		// Return a deterministic dev-only key so encrypt/decrypt round-trip works.
		devKey := sha256.Sum256([]byte("japaconnect-dev-totp-key"))

		return devKey[:], nil
	}

	hash := sha256.Sum256([]byte(raw))

	return hash[:], nil
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
