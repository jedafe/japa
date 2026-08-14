package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

// All tests in this file rely on TestMain to set a default encryption key.
// Because the package caches the key set via sync.OnceValues, individual
// tests cannot change the key with t.Setenv — the first call to
// Encrypt/Decrypt caches the keys for the process lifetime. Tests that
// need to verify key loading logic call loadCurrentKey / loadKeys /
// deriveKey directly.

func TestEncryptDecryptRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty", ""},
		{"short", "hello"},
		{"long", strings.Repeat("a", 10000)},
		{"unicode", "日本語テスト🔐"},
		{"json", `{"api_key":"sk-12345","endpoint":"https://api.example.com"}`},
		{"special chars", `!@#$%^&*()_+-=[]{}|;':",./<>?`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			if encrypted == tt.plaintext && tt.plaintext != "" {
				t.Error("encrypted output should differ from plaintext")
			}

			decrypted, err := Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("round-trip mismatch: got %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptFormat(t *testing.T) {
	encrypted, err := Encrypt("test data")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	parts := strings.Split(encrypted, ":")
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts (v1:keyid:nonce:ciphertext), got %d: %q", len(parts), encrypted)
	}

	if parts[0] != "v1" {
		t.Errorf("expected version 'v1', got %q", parts[0])
	}
	if len(parts[1]) != 8 {
		t.Errorf("key ID should be 8 hex chars, got %q (len %d)", parts[1], len(parts[1]))
	}
	if parts[2] == "" {
		t.Error("nonce should not be empty")
	}
	if parts[3] == "" {
		t.Error("ciphertext should not be empty")
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	plaintext := "same plaintext"

	enc1, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("first Encrypt failed: %v", err)
	}

	enc2, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("second Encrypt failed: %v", err)
	}

	if enc1 == enc2 {
		t.Error("encrypting the same plaintext should produce different ciphertexts (random nonce)")
	}
}

func TestEncryptEmbedsKeyID(t *testing.T) {
	// The key ID in the ciphertext must match the current key's ID.
	encrypted, err := Encrypt("test data")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	parts := strings.Split(encrypted, ":")
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(parts))
	}

	cur, err := CurrentKey()
	if err != nil {
		t.Fatalf("CurrentKey failed: %v", err)
	}
	if parts[1] != cur.keyID {
		t.Errorf("ciphertext key ID %q does not match current key ID %q", parts[1], cur.keyID)
	}
}

func TestDecryptInvalidFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"no colons", "justastring", true},               // fail closed
		{"two colons wrong version", "v2:abc:def", true}, // fail closed
		{"empty", "", true},                              // fail closed
		{"four colons wrong version", "v2:k1:abc:def", true},
		{"keyid format bad hex nonce", "v1:aaaaaaaa:zzzz:def", true},
		{"keyid format bad hex ciphertext", "v1:aaaaaaaa:abcd:zzzz", true},
		{"keyid format unknown key id", "v1:deadbeef:abcd:abcd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Decrypt(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (result: %q)", result)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestDecryptRejectsPlaintext(t *testing.T) {
	// Non-encrypted input must always error — the legacy plaintext hatch
	// has been removed. This is the downgrade-protection guarantee.
	if _, err := Decrypt("plain-secret-value"); err == nil {
		t.Error("Decrypt should reject plaintext input (fail closed)")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	encrypted, err := Encrypt("secret data")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	parts := strings.Split(encrypted, ":")
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(parts))
	}

	// Flip bytes in the ciphertext to break the GCM auth tag.
	ciphertext := parts[3]
	if len(ciphertext) < 4 {
		t.Fatal("ciphertext too short for tamper test")
	}
	tampered := ciphertext[:len(ciphertext)-4] + "0000"
	if tampered == ciphertext {
		tampered = ciphertext[:len(ciphertext)-4] + "ffff"
	}

	tamperedEncrypted := "v1:" + parts[1] + ":" + parts[2] + ":" + tampered

	_, err = Decrypt(tamperedEncrypted)
	if err == nil {
		t.Error("decrypting tampered ciphertext should fail (GCM auth tag mismatch)")
	}
}

func TestDecryptLegacyThreePartFormat(t *testing.T) {
	// Ciphertext written by the pre-key-ID format (v1:nonce:ciphertext)
	// must still decrypt with the current key.
	ks, err := KeysForMigration()
	if err != nil {
		t.Fatalf("KeysForMigration failed: %v", err)
	}

	// Hand-build a 3-part ciphertext using the current key.
	block := newGCM(t, ks.current.key)
	nonce := make([]byte, block.NonceSize())
	if _, err := readRand(nonce); err != nil {
		t.Fatalf("rand: %v", err)
	}
	ciphertext := block.Seal(nil, nonce, []byte("legacy-format-payload"), nil)
	legacy := "v1:" + hex.EncodeToString(nonce) + ":" + hex.EncodeToString(ciphertext)

	pt, err := Decrypt(legacy)
	if err != nil {
		t.Fatalf("legacy 3-part format should decrypt: %v", err)
	}
	if pt != "legacy-format-payload" {
		t.Errorf("legacy decrypt mismatch: got %q", pt)
	}
}

func TestKeyRotationPreviousKey(t *testing.T) {
	// Build a key set with a current and a previous key. Ciphertext written
	// with the previous key (key-ID format) must decrypt via the previous key.
	oldKey := deriveTestKey(t, "old-rotation-passphrase")
	newKey := deriveTestKey(t, "new-rotation-passphrase")

	ks := &keySet{
		current:  keyMaterial{key: newKey, keyID: keyIDFor(newKey), source: "new"},
		previous: &keyMaterial{key: oldKey, keyID: keyIDFor(oldKey), source: "old"},
	}

	// Encrypt with the OLD key (simulating pre-rotation data).
	oldCT, err := EncryptWith(keyMaterial{key: oldKey, keyID: keyIDFor(oldKey), source: "old"}, "pre-rotation-secret")
	if err != nil {
		t.Fatalf("EncryptWith old key failed: %v", err)
	}

	// Decrypt via the key set — should select the previous key by ID.
	pt, err := DecryptWith(ks, oldCT)
	if err != nil {
		t.Fatalf("DecryptWith should use previous key: %v", err)
	}
	if pt != "pre-rotation-secret" {
		t.Errorf("rotation decrypt mismatch: got %q", pt)
	}

	// New writes use the current key and decrypt with it.
	newCT, err := EncryptWith(ks.current, "post-rotation-secret")
	if err != nil {
		t.Fatalf("EncryptWith new key failed: %v", err)
	}
	pt2, err := DecryptWith(ks, newCT)
	if err != nil {
		t.Fatalf("DecryptWith new key failed: %v", err)
	}
	if pt2 != "post-rotation-secret" {
		t.Errorf("new decrypt mismatch: got %q", pt2)
	}

	// Ciphertext with an unknown key ID must error.
	unknown := keyMaterial{key: deriveTestKey(t, "unknown"), keyID: keyIDFor(deriveTestKey(t, "unknown")), source: "unknown"}
	badCT, err := EncryptWith(unknown, "x")
	if err != nil {
		t.Fatalf("EncryptWith unknown key failed: %v", err)
	}
	if _, err := DecryptWith(ks, badCT); err == nil {
		t.Error("decrypt with unknown key ID should error")
	}
}

func TestEncryptJSONDecryptJSONRoundTrip(t *testing.T) {
	original := json.RawMessage(`{"endpoint":"https://api.example.com","api_key":"sk-secret123"}`)

	encrypted, err := EncryptJSON(original)
	if err != nil {
		t.Fatalf("EncryptJSON failed: %v", err)
	}

	if len(encrypted) == 0 {
		t.Fatal("encrypted output should not be empty")
	}

	// The encrypted output should be valid JSON (a JSON string).
	var asString string
	if err := json.Unmarshal(encrypted, &asString); err != nil {
		t.Fatalf("encrypted output should be a valid JSON string: %v", err)
	}

	if !strings.HasPrefix(asString, "v1:") {
		t.Errorf("encrypted JSON string should start with 'v1:', got %q", asString[:min(10, len(asString))])
	}

	decrypted, err := DecryptJSON(encrypted)
	if err != nil {
		t.Fatalf("DecryptJSON failed: %v", err)
	}

	if string(decrypted) != string(original) {
		t.Errorf("JSON round-trip mismatch: got %q, want %q", decrypted, original)
	}
}

func TestEncryptJSONEmptyInput(t *testing.T) {
	result, err := EncryptJSON(nil)
	if err != nil {
		t.Fatalf("EncryptJSON(nil) failed: %v", err)
	}
	if result != nil {
		t.Errorf("EncryptJSON(nil) should return nil, got %v", result)
	}

	result, err = EncryptJSON([]byte{})
	if err != nil {
		t.Fatalf("EncryptJSON([]byte{}) failed: %v", err)
	}
	if result != nil {
		t.Errorf("EncryptJSON([]byte{}) should return nil, got %v", result)
	}
}

func TestDecryptJSONEmptyInput(t *testing.T) {
	result, err := DecryptJSON(nil)
	if err != nil {
		t.Fatalf("DecryptJSON(nil) failed: %v", err)
	}
	if result != nil {
		t.Errorf("DecryptJSON(nil) should return nil, got %v", result)
	}
}

func TestDecryptJSONLegacyPlaintext(t *testing.T) {
	// Legacy plaintext JSON (not encrypted, not a JSON string) must always
	// error — the legacy plaintext hatch has been removed. This is the
	// downgrade-protection guarantee for api_config columns.
	legacy := json.RawMessage(`{"api_key":"sk-plaintext","endpoint":"https://old.example.com"}`)

	if _, err := DecryptJSON(legacy); err == nil {
		t.Error("DecryptJSON on legacy plaintext should error (fail closed)")
	}
}

func TestProductionRequiresKey(t *testing.T) {
	t.Setenv("JC_ENV", "production")
	t.Setenv("JC_ENCRYPTION_KEY", "")
	t.Setenv("JC_STOCK_ENCRYPTION_KEY", "")
	t.Setenv("JC_TOTP_ENCRYPTION_KEY", "")

	_, err := loadCurrentKey()
	if err == nil {
		t.Error("loadCurrentKey should return error in production without a key")
	}
	if !strings.Contains(err.Error(), "JC_ENCRYPTION_KEY") {
		t.Errorf("error should mention JC_ENCRYPTION_KEY, got: %v", err)
	}
}

func TestNonDevEnvRequiresKey(t *testing.T) {
	// Any environment that is not explicitly development/test must fail
	// closed when no key is configured — staging/QA/preview/unset must not
	// silently fall back to the public hardcoded dev key.
	for _, env := range []string{"staging", "qa", "preview", "", "PRODUCTION"} {
		t.Run("JC_ENV="+env, func(t *testing.T) {
			t.Setenv("JC_ENV", env)
			t.Setenv("JC_ENCRYPTION_KEY", "")
			t.Setenv("JC_STOCK_ENCRYPTION_KEY", "")
			t.Setenv("JC_TOTP_ENCRYPTION_KEY", "")
			if _, err := loadCurrentKey(); err == nil {
				t.Errorf("loadCurrentKey should error when JC_ENV=%q and no key is set", env)
			}
		})
	}
}

func TestDevEnvAllowsFallbackKey(t *testing.T) {
	// Explicit development/test environments may use the hardcoded dev key.
	for _, env := range []string{"development", "test", "testing", "Development"} {
		t.Run("JC_ENV="+env, func(t *testing.T) {
			t.Setenv("JC_ENV", env)
			t.Setenv("JC_ENCRYPTION_KEY", "")
			t.Setenv("JC_STOCK_ENCRYPTION_KEY", "")
			t.Setenv("JC_TOTP_ENCRYPTION_KEY", "")
			km, err := loadCurrentKey()
			if err != nil {
				t.Fatalf("loadCurrentKey should allow dev fallback for JC_ENV=%q: %v", env, err)
			}
			if len(km.key) != 32 {
				t.Errorf("dev fallback key should be 32 bytes, got %d", len(km.key))
			}
		})
	}
}

func TestKeyDerivationConsistency(t *testing.T) {
	// The same passphrase should always derive the same key via Argon2id.
	key1, err := deriveKey("consistent-test-passphrase")
	if err != nil {
		t.Fatalf("first deriveKey failed: %v", err)
	}
	key2, err := deriveKey("consistent-test-passphrase")
	if err != nil {
		t.Fatalf("second deriveKey failed: %v", err)
	}
	if len(key1) != 32 {
		t.Errorf("derived key should be 32 bytes (AES-256), got %d", len(key1))
	}
	for i := range key1 {
		if key1[i] != key2[i] {
			t.Fatal("same passphrase should derive the same key")
		}
	}
}

func TestKeyPriorityOrder(t *testing.T) {
	// JC_ENCRYPTION_KEY takes priority over JC_STOCK_ENCRYPTION_KEY and
	// JC_TOTP_ENCRYPTION_KEY.
	t.Setenv("JC_ENCRYPTION_KEY", "primary-passphrase")
	t.Setenv("JC_STOCK_ENCRYPTION_KEY", "stock-passphrase")
	t.Setenv("JC_TOTP_ENCRYPTION_KEY", "totp-passphrase")
	t.Setenv("JC_ENV", "development")

	km, err := loadCurrentKey()
	if err != nil {
		t.Fatalf("loadCurrentKey failed: %v", err)
	}

	expected := argon2idKey("primary-passphrase")
	if !bytesEqual(km.key, expected) {
		t.Fatal("JC_ENCRYPTION_KEY should take priority over fallback env vars")
	}
}

func TestKeyFallbackToStockKey(t *testing.T) {
	t.Setenv("JC_ENCRYPTION_KEY", "")
	t.Setenv("JC_STOCK_ENCRYPTION_KEY", "stock-passphrase")
	t.Setenv("JC_TOTP_ENCRYPTION_KEY", "")
	t.Setenv("JC_ENV", "development")

	km, err := loadCurrentKey()
	if err != nil {
		t.Fatalf("loadCurrentKey failed: %v", err)
	}

	expected := argon2idKey("stock-passphrase")
	if !bytesEqual(km.key, expected) {
		t.Fatal("should fall back to JC_STOCK_ENCRYPTION_KEY")
	}
}

func TestKeyFallbackToTOTPKey(t *testing.T) {
	t.Setenv("JC_ENCRYPTION_KEY", "")
	t.Setenv("JC_STOCK_ENCRYPTION_KEY", "")
	t.Setenv("JC_TOTP_ENCRYPTION_KEY", "totp-passphrase")
	t.Setenv("JC_ENV", "development")

	km, err := loadCurrentKey()
	if err != nil {
		t.Fatalf("loadCurrentKey failed: %v", err)
	}

	expected := argon2idKey("totp-passphrase")
	if !bytesEqual(km.key, expected) {
		t.Fatal("should fall back to JC_TOTP_ENCRYPTION_KEY")
	}
}

func TestDeriveKeyRawHex(t *testing.T) {
	// A 64-char hex string decoding to 32 bytes is used directly (no KDF).
	raw := hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef")) // 32 bytes → 64 hex
	if len(raw) != 64 {
		t.Fatalf("setup: expected 64 hex chars, got %d", len(raw))
	}
	key, err := deriveKey(raw)
	if err != nil {
		t.Fatalf("deriveKey raw hex failed: %v", err)
	}
	want, _ := hex.DecodeString(raw)
	if !bytesEqual(key, want) {
		t.Error("raw 32-byte hex key should be used directly, not derived")
	}
}

func TestDeriveKeyRawBase64(t *testing.T) {
	// A 44-char standard base64 string decoding to 32 bytes is used directly.
	raw := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if len(raw) != 44 {
		t.Fatalf("setup: expected 44 base64 chars, got %d", len(raw))
	}
	key, err := deriveKey(raw)
	if err != nil {
		t.Fatalf("deriveKey raw base64 failed: %v", err)
	}
	want, _ := base64.StdEncoding.DecodeString(raw)
	if !bytesEqual(key, want) {
		t.Error("raw 32-byte base64 key should be used directly, not derived")
	}
}

func TestDeriveKeyPassphraseUsesArgon2id(t *testing.T) {
	// A non-raw-value passphrase is stretched via Argon2id, not SHA-256.
	key, err := deriveKey("some-passphrase")
	if err != nil {
		t.Fatalf("deriveKey passphrase failed: %v", err)
	}
	expected := argon2idKey("some-passphrase")
	if !bytesEqual(key, expected) {
		t.Error("passphrase should be derived via Argon2id")
	}
	// And it must NOT equal the old single-SHA-256 derivation.
	oldSHA := sha256.Sum256([]byte("some-passphrase"))
	if bytesEqual(key, oldSHA[:]) {
		t.Error("passphrase must not use the retired single-SHA-256 derivation")
	}
}

func TestLoadKeysWithPrevious(t *testing.T) {
	// JC_ENCRYPTION_KEY_PREVIOUS configures the previous key for rotation.
	t.Setenv("JC_ENCRYPTION_KEY", "new-passphrase")
	t.Setenv("JC_ENCRYPTION_KEY_PREVIOUS", "old-passphrase")
	t.Setenv("JC_ENV", "development")

	ks, err := loadKeys()
	if err != nil {
		t.Fatalf("loadKeys failed: %v", err)
	}
	if ks.previous == nil {
		t.Fatal("previous key should be configured when JC_ENCRYPTION_KEY_PREVIOUS is set")
	}
	if ks.current.keyID == ks.previous.keyID {
		t.Fatal("current and previous key IDs must differ for distinct passphrases")
	}
	// The current key must derive from "new-passphrase", previous from "old-passphrase".
	if !bytesEqual(ks.current.key, argon2idKey("new-passphrase")) {
		t.Error("current key should derive from JC_ENCRYPTION_KEY")
	}
	if !bytesEqual(ks.previous.key, argon2idKey("old-passphrase")) {
		t.Error("previous key should derive from JC_ENCRYPTION_KEY_PREVIOUS")
	}
}

func TestLoadKeysWithoutPrevious(t *testing.T) {
	t.Setenv("JC_ENCRYPTION_KEY", "solo-passphrase")
	t.Setenv("JC_ENCRYPTION_KEY_PREVIOUS", "")
	t.Setenv("JC_ENV", "development")

	ks, err := loadKeys()
	if err != nil {
		t.Fatalf("loadKeys failed: %v", err)
	}
	if ks.previous != nil {
		t.Fatal("previous key should be nil when JC_ENCRYPTION_KEY_PREVIOUS is unset")
	}
}

// --- test helpers ---

// argon2idKey mirrors the KDF in deriveKey for passphrases.
func argon2idKey(passphrase string) []byte {
	return argon2.IDKey([]byte(passphrase), argon2Salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
}

// newGCM builds an AES-256-GCM aead for a key (test helper).
func newGCM(t *testing.T, key []byte) cipher.AEAD {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("cipher.NewGCM: %v", err)
	}
	return gcm
}

// readRand fills b with crypto/rand bytes (test helper).
func readRand(b []byte) (int, error) {
	return io.ReadFull(rand.Reader, b)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// deriveTestKey derives a key from a passphrase (Argon2id path).
func deriveTestKey(t *testing.T, passphrase string) []byte {
	t.Helper()
	k, err := deriveKey(passphrase)
	if err != nil {
		t.Fatalf("deriveTestKey: %v", err)
	}
	return k
}

func TestMain(m *testing.M) {
	// Ensure a default key is set for all tests that don't explicitly set one.
	// The key set is cached via sync.OnceValues on first use, so this must be
	// set before any test calls Encrypt/Decrypt.
	if os.Getenv("JC_ENCRYPTION_KEY") == "" &&
		os.Getenv("JC_STOCK_ENCRYPTION_KEY") == "" &&
		os.Getenv("JC_TOTP_ENCRYPTION_KEY") == "" {
		os.Setenv("JC_ENCRYPTION_KEY", "default-test-key-for-crypto-tests")
		os.Setenv("JC_ENV", "development")
	}
	os.Exit(m.Run())
}
