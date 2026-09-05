// Package crypto provides shared AES-256-GCM encryption helpers.
// Used by auth (TOTP secrets, JWT signing keys), stock (provider API keys),
// AI providers, remittance (provider api_config), travel (provider api_config),
// webhooks (signing secrets), and advertising (ad-server keys).
//
// Ciphertext format:  v1:<keyid>:<hex nonce>:<hex ciphertext>
// Legacy format still readable: v1:<hex nonce>:<hex ciphertext>
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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// cipherVersion is the format version prefix for encrypted output.
const cipherVersion = "v1"

// argon2Salt is the fixed package salt used when deriving a key from a
// passphrase via Argon2id. The passphrase itself is the per-deployment
// secret; this salt prevents cross-package rainbow tables. Per the §02
// spec the salt is fixed.
var argon2Salt = []byte("japaconnect-encryption-key-v1")

// argon2id parameters per §02 spec: time=2, memory=64 MiB, parallelism=2.
const (
	argon2Time    = 2
	argon2Memory  = 64 * 1024 // KiB → 64 MiB
	argon2Threads = 2
	argon2KeyLen  = 32 // AES-256
)

// keyMaterial holds a derived AES-256 key and its short identifier.
// The key ID is the first 4 bytes of SHA-256(key), hex-encoded (8 chars).
// It is NOT secret — its only purpose is to let Decrypt select the correct
// key during a rotation window.
type keyMaterial struct {
	key    []byte
	keyID  string
	source string // human-readable origin for error messages
}

// cachedKeys holds the current and (optionally) previous encryption keys,
// computed once on first use. The previous key supports the rotation
// procedure documented in §02: during a key rotation, JC_ENCRYPTION_KEY
// (or a legacy alias) holds the new key and JC_ENCRYPTION_KEY_PREVIOUS
// holds the old one, so Decrypt can read ciphertext written before the
// rotation while Encrypt writes with the new key.
var cachedKeys = sync.OnceValues(func() (*keySet, error) {
	return loadKeys()
})

// keySet is the resolved set of keys available to Decrypt.
type keySet struct {
	current  keyMaterial
	previous *keyMaterial // nil when no previous key is configured
}

// byID returns the key material whose keyID matches id, or nil.
func (ks *keySet) byID(id string) *keyMaterial {
	if ks.current.keyID == id {
		return &ks.current
	}

	if ks.previous != nil && ks.previous.keyID == id {
		return ks.previous
	}

	return nil
}

// all returns every configured key (current first, then previous) for
// legacy-format fallback decryption that must try each key in turn.
func (ks *keySet) all() []keyMaterial {
	out := []keyMaterial{ks.current}
	if ks.previous != nil {
		out = append(out, *ks.previous)
	}

	return out
}

// Encrypt encrypts a plaintext string using AES-256-GCM with the current
// key. Returns "v1:<keyid>:<hex nonce>:<hex ciphertext>".
//
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md.
func Encrypt(plaintext string) (string, error) {
	ks, err := cachedKeys()
	if err != nil {
		return "", err
	}

	return encryptWith(ks.current, plaintext)
}

// EncryptWith encrypts plaintext with an explicit key material. It is
// intended for one-off re-encryption migration jobs that need to write
// with a specific (e.g. newly rotated) key. Steady-state callers should
// use Encrypt, which picks the current key.
func EncryptWith(km keyMaterial, plaintext string) (string, error) {
	return encryptWith(km, plaintext)
}

// CurrentKey returns the current key material. It is intended for
// migration jobs that need to know which key ID they are writing with.
// The returned key bytes are sensitive — callers must not log them.
func CurrentKey() (keyMaterial, error) {
	ks, err := cachedKeys()
	if err != nil {
		return keyMaterial{}, err
	}

	return ks.current, nil
}

func encryptWith(km keyMaterial, plaintext string) (string, error) {
	block, err := aes.NewCipher(km.key)
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

	return fmt.Sprintf("%s:%s:%s:%s", cipherVersion, km.keyID, hex.EncodeToString(nonce), hex.EncodeToString(ciphertext)), nil
}

// Decrypt reverses Encrypt. It accepts both the current key-ID format
// "v1:<keyid>:<nonce>:<ciphertext>" and the legacy 3-part format
// "v1:<nonce>:<ciphertext>" (written before key IDs were introduced).
//
// For the key-ID format, the key is selected by ID (current or previous).
// For the legacy 3-part format, each configured key is tried in turn
// (current first, then previous) until one decrypts successfully.
//
// If the input does not match either encrypted format, it is treated as
// legacy plaintext and returned as-is ONLY when the operator has opted in
// via JC_CRYPTO_ALLOW_LEGACY_PLAINTEXT=1; otherwise an error is returned
// (fail closed — prevents a downgrade/JWT-forgery vector).
//
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md.
func Decrypt(encrypted string) (string, error) {
	ks, err := cachedKeys()
	if err != nil {
		return "", err
	}

	return decryptWith(ks, encrypted)
}

// DecryptWith decrypts encrypted using an explicit key set. Intended for
// migration jobs and tests. Steady-state callers should use Decrypt.
func DecryptWith(ks *keySet, encrypted string) (string, error) {
	return decryptWith(ks, encrypted)
}

// KeysForMigration loads the current and previous key material for a
// re-encryption migration job. The previous key may be nil.
func KeysForMigration() (*keySet, error) {
	return cachedKeys()
}

func decryptWith(ks *keySet, encrypted string) (string, error) {
	parts := strings.Split(encrypted, ":")

	switch {
	case len(parts) == 4 && parts[0] == cipherVersion:
		// Current format: v1:<keyid>:<nonce>:<ciphertext>
		return decryptKeyed(ks, parts[1], parts[2], parts[3])
	case len(parts) == 3 && parts[0] == cipherVersion:
		// Legacy format (pre key-ID): v1:<nonce>:<ciphertext>. Try every
		// configured key — the ciphertext could have been written with the
		// current or the previous key.
		return decryptLegacyFormat(ks, parts[1], parts[2])
	default:
		// Not an encrypted value. Fail closed — never return plaintext.
		// This prevents a downgrade vector where an attacker with DB write
		// access replaces an encrypted secret with a known plaintext and
		// the app silently accepts it (enabling JWT forgery / 2FA bypass).
		return "", errors.New("crypto: input is not encrypted")
	}
}

func decryptKeyed(ks *keySet, keyID, nonceHex, ciphertextHex string) (string, error) {
	km := ks.byID(keyID)
	if km == nil {
		return "", fmt.Errorf("crypto: ciphertext key ID %q does not match any configured key (current=%q)", keyID, ks.current.keyID)
	}

	return openGCM(km, nonceHex, ciphertextHex)
}

func decryptLegacyFormat(ks *keySet, nonceHex, ciphertextHex string) (string, error) {
	var lastErr error

	for _, km := range ks.all() {
		pt, err := openGCM(&km, nonceHex, ciphertextHex)
		if err == nil {
			return pt, nil
		}

		lastErr = err
	}

	return "", fmt.Errorf("crypto: legacy-format ciphertext did not decrypt with any configured key: %w", lastErr)
}

func openGCM(km *keyMaterial, nonceHex, ciphertextHex string) (string, error) {
	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		return "", fmt.Errorf("decoding nonce: %w", err)
	}

	ciphertext, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}

	block, err := aes.NewCipher(km.key)
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
//
// If the stored value is not a JSON string, it is treated as legacy
// plaintext and returned as-is ONLY when JC_CRYPTO_ALLOW_LEGACY_PLAINTEXT=1;
// otherwise an error is returned (fail closed — prevents a credential
// downgrade vector on api_config columns).
func DecryptJSON(storedJSON []byte) ([]byte, error) {
	if len(storedJSON) == 0 {
		return nil, nil
	}

	var encrypted string
	if err := json.Unmarshal(storedJSON, &encrypted); err != nil {
		// The stored value is not a JSON string. Fail closed — never return
		// raw bytes as-is. This prevents a downgrade vector where an attacker
		// with DB write access replaces an encrypted api_config with plaintext
		// credentials and the app silently accepts it.
		return nil, fmt.Errorf("crypto: stored JSON is not an encrypted string: %w", err)
	}

	plaintext, err := Decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypting JSON: %w", err)
	}

	return []byte(plaintext), nil
}

// loadKeys resolves the current and (optionally) previous encryption keys
// from environment variables.
//
// Current key priority: JC_ENCRYPTION_KEY → JC_STOCK_ENCRYPTION_KEY →
// JC_TOTP_ENCRYPTION_KEY → hardcoded dev key (only when JC_ENV is
// explicitly development|test|testing).
//
// Previous key (for rotation): JC_ENCRYPTION_KEY_PREVIOUS → then the same
// alias fallback chain is NOT applied to the previous key (the operator
// sets JC_ENCRYPTION_KEY_PREVIOUS explicitly to the old raw value).
//
// Key material may be a raw 32-byte key (64 hex chars or 44 base64-std
// chars) used directly, or a passphrase derived via Argon2id.
//
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md.
func loadKeys() (*keySet, error) {
	current, err := loadCurrentKey()
	if err != nil {
		return nil, err
	}

	ks := &keySet{current: current}

	if prevRaw := os.Getenv("JC_ENCRYPTION_KEY_PREVIOUS"); prevRaw != "" {
		prevKey, err := deriveKey(prevRaw)
		if err != nil {
			return nil, fmt.Errorf("JC_ENCRYPTION_KEY_PREVIOUS: %w", err)
		}

		ks.previous = &keyMaterial{
			key:    prevKey,
			keyID:  keyIDFor(prevKey),
			source: "JC_ENCRYPTION_KEY_PREVIOUS",
		}
	}

	return ks, nil
}

func loadCurrentKey() (keyMaterial, error) {
	raw := os.Getenv("JC_ENCRYPTION_KEY")
	source := "JC_ENCRYPTION_KEY"

	if raw == "" {
		raw = os.Getenv("JC_STOCK_ENCRYPTION_KEY")
		source = "JC_STOCK_ENCRYPTION_KEY"
	}

	if raw == "" {
		raw = os.Getenv("JC_TOTP_ENCRYPTION_KEY")
		source = "JC_TOTP_ENCRYPTION_KEY"
	}

	if raw == "" {
		env := strings.ToLower(os.Getenv("JC_ENV"))
		if env == "development" || env == "test" || env == "testing" {
			slog.Warn("encryption key not set — secrets stored with hardcoded dev key (dev/test only). Set JC_ENCRYPTION_KEY for any other environment.")

			devKey := sha256.Sum256([]byte("japaconnect-dev-totp-key"))

			return keyMaterial{
				key:    devKey[:],
				keyID:  keyIDFor(devKey[:]),
				source: "dev-fallback",
			}, nil
		}

		return keyMaterial{}, errors.New("encryption key not configured (set JC_ENCRYPTION_KEY; a dev fallback is only permitted when JC_ENV=development|test)")
	}

	key, err := deriveKey(raw)
	if err != nil {
		return keyMaterial{}, fmt.Errorf("%s: %w", source, err)
	}

	return keyMaterial{
		key:    key,
		keyID:  keyIDFor(key),
		source: source,
	}, nil
}

// deriveKey converts a raw env value into a 32-byte AES-256 key.
//
// If the value is a decodable 32-byte hex (64 chars) or base64-std (44
// chars) string, it is used directly — the recommended production form.
// Otherwise it is treated as a passphrase and stretched via Argon2id
// (time=2, memory=64 MiB, parallelism=2) with a fixed package salt.
//
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md.
func deriveKey(raw string) ([]byte, error) {
	if key, ok := tryRaw32Hex(raw); ok {
		return key, nil
	}

	if key, ok := tryRaw32Base64(raw); ok {
		return key, nil
	}
	// Passphrase — derive via Argon2id. This is intentionally not cached
	// per-call: loadKeys() is itself wrapped in sync.OnceValues, so the
	// derivation runs once per process.
	return argon2.IDKey([]byte(raw), argon2Salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen), nil
}

// tryRaw32Hex returns the 32-byte key if raw is 64 hex chars decoding to
// 32 bytes; ok=false otherwise.
func tryRaw32Hex(raw string) ([]byte, bool) {
	if len(raw) != 64 {
		return nil, false
	}

	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != 32 {
		return nil, false
	}

	return b, true
}

// tryRaw32Base64 returns the 32-byte key if raw is standard base64
// decoding to 32 bytes; ok=false otherwise.
func tryRaw32Base64(raw string) ([]byte, bool) {
	if len(raw) != 44 {
		return nil, false
	}

	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(b) != 32 {
		return nil, false
	}

	return b, true
}

// keyIDFor returns the 8-char hex key ID for a 32-byte key (first 4 bytes
// of SHA-256(key)). The ID is not secret.
func keyIDFor(key []byte) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:4])
}
