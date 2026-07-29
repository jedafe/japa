// Package crypto provides shared AES-256-GCM encryption helpers.
// Used by auth (TOTP secrets) and stock (provider API keys).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Encrypt encrypts a plaintext string using AES-256-GCM.
// The key is derived from JC_STOCK_ENCRYPTION_KEY (falling back to
// JC_TOTP_ENCRYPTION_KEY) via SHA-256. Returns "v1:<hex nonce>:<hex ciphertext>".
// In production, returns an error if no encryption key is configured.
func Encrypt(plaintext string) (string, error) {
	key, err := LoadEncryptionKey()
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
	return fmt.Sprintf("v1:%s:%s", hex.EncodeToString(nonce), hex.EncodeToString(ciphertext)), nil
}

// Decrypt reverses Encrypt. Parses "v1:<nonce>:<ciphertext>" format and
// decrypts using AES-256-GCM. Returns plaintext or an error.
func Decrypt(encrypted string) (string, error) {
	key, err := LoadEncryptionKey()
	if err != nil {
		return "", err
	}

	parts := strings.SplitN(encrypted, ":", 3)
	if len(parts) != 3 || parts[0] != "v1" {
		// Legacy plaintext — return as-is for backward compatibility.
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

// LoadEncryptionKey reads the encryption key from environment variables.
// Priority: JC_STOCK_ENCRYPTION_KEY → JC_TOTP_ENCRYPTION_KEY.
// In production, returns an error if no key is configured.
func LoadEncryptionKey() ([]byte, error) {
	raw := os.Getenv("JC_STOCK_ENCRYPTION_KEY")
	if raw == "" {
		raw = os.Getenv("JC_TOTP_ENCRYPTION_KEY")
	}
	if raw == "" {
		env := os.Getenv("JC_ENV")
		if strings.EqualFold(env, "production") {
			return nil, errors.New("encryption key not configured (set JC_STOCK_ENCRYPTION_KEY or JC_TOTP_ENCRYPTION_KEY)")
		}
		slog.Warn("encryption key not set — secrets stored without encryption (dev only)")
		devKey := sha256.Sum256([]byte("japaconnect-dev-encryption-key"))
		return devKey[:], nil
	}
	hash := sha256.Sum256([]byte(raw))
	return hash[:], nil
}
