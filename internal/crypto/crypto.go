// Package crypto provides shared AES-256-GCM encryption helpers.
// Used by auth (TOTP secrets), stock (provider API keys), AI providers,
// remittance (provider api_config), and travel (provider api_config).
//
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
// Ref: japaconnect-engineering/sections/17-payment-core.md
// Ref: japaconnect-engineering/adr/ADR-075-remittance-rate-tracker.md
// Ref: japaconnect-engineering/adr/ADR-085-travel-search.md
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// cipherVersion is the format version prefix for encrypted output.
const cipherVersion = "v1"

// cachedKey holds the derived encryption key, computed once on first use.
var cachedKey = sync.OnceValues(func() ([]byte, error) {
	return loadEncryptionKey()
})

// Encrypt encrypts a plaintext string using AES-256-GCM.
// The key is derived from JC_ENCRYPTION_KEY (falling back to
// JC_STOCK_ENCRYPTION_KEY, then JC_TOTP_ENCRYPTION_KEY) via SHA-256.
// Returns "v1:<hex nonce>:<hex ciphertext>".
// In production, returns an error if no encryption key is configured.
func Encrypt(plaintext string) (string, error) {
	key, err := cachedKey()
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

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return fmt.Sprintf("%s:%s:%s", cipherVersion, hex.EncodeToString(nonce), hex.EncodeToString(ciphertext)), nil
}

// Decrypt reverses Encrypt. Parses "v1:<nonce>:<ciphertext>" format and
// decrypts using AES-256-GCM. Returns plaintext or an error.
// If the input does not match the encrypted format, it is returned as-is
// for backward compatibility with legacy plaintext data, with a warning log.
func Decrypt(encrypted string) (string, error) {
	key, err := cachedKey()
	if err != nil {
		return "", err
	}

	parts := strings.SplitN(encrypted, ":", 3)
	if len(parts) != 3 || parts[0] != cipherVersion {
		// Legacy plaintext — return as-is for backward compatibility.
		slog.Warn("crypto: decrypting legacy plaintext (not encrypted format)", "len", len(encrypted))
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

// EncryptJSON encrypts a raw JSON byte slice and wraps the encrypted string
// as a JSON string value, suitable for storage in a jsonb column. The
// returned []byte is valid JSON (a quoted string containing the encrypted
// blob). Use DecryptJSON to reverse.
//
// This is used by features that store api_config as jsonb but need the
// contents encrypted at rest (remittance §74/ADR-075, travel §67/ADR-085).
func EncryptJSON(rawJSON []byte) ([]byte, error) {
	if len(rawJSON) == 0 {
		return nil, nil
	}
	encrypted, err := Encrypt(string(rawJSON))
	if err != nil {
		return nil, fmt.Errorf("encrypting JSON: %w", err)
	}
	// Wrap as a JSON string so the result is valid JSONB.
	return json.Marshal(encrypted)
}

// DecryptJSON reverses EncryptJSON. It unmarshals the stored JSON string
// to obtain the encrypted blob, decrypts it, and returns the original raw
// JSON byte slice. Returns nil (no error) for empty input.
func DecryptJSON(storedJSON []byte) ([]byte, error) {
	if len(storedJSON) == 0 {
		return nil, nil
	}
	var encrypted string
	if err := json.Unmarshal(storedJSON, &encrypted); err != nil {
		// The stored value is not a JSON string — it may be legacy
		// plaintext JSON (pre-encryption). Return as-is.
		slog.Warn("crypto: stored JSON is not an encrypted string, returning as-is (legacy)", "len", len(storedJSON))
		return storedJSON, nil
	}
	plaintext, err := Decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypting JSON: %w", err)
	}
	return []byte(plaintext), nil
}

// loadEncryptionKey reads the encryption key from environment variables.
// Priority: JC_ENCRYPTION_KEY (JEOS canonical) → JC_STOCK_ENCRYPTION_KEY
// → JC_TOTP_ENCRYPTION_KEY (legacy aliases).
// In production, returns an error if no key is configured.
func loadEncryptionKey() ([]byte, error) {
	raw := os.Getenv("JC_ENCRYPTION_KEY")
	if raw == "" {
		raw = os.Getenv("JC_STOCK_ENCRYPTION_KEY")
	}
	if raw == "" {
		raw = os.Getenv("JC_TOTP_ENCRYPTION_KEY")
	}
	if raw == "" {
		env := os.Getenv("JC_ENV")
		if strings.EqualFold(env, "production") {
			return nil, errors.New("encryption key not configured (set JC_ENCRYPTION_KEY)")
		}
		slog.Warn("encryption key not set — secrets stored without encryption (dev only). Set JC_ENCRYPTION_KEY for production.")
		devKey := sha256.Sum256([]byte("japaconnect-dev-totp-key"))
		return devKey[:], nil
	}
	hash := sha256.Sum256([]byte(raw))
	return hash[:], nil
}
