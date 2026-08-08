package crypto

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// All tests in this file rely on TestMain to set a default encryption key.
// Because the package caches the key via sync.OnceValues, individual tests
// cannot change the key with t.Setenv — the first call to Encrypt/Decrypt
// caches the key for the process lifetime. Tests that need to verify key
// loading logic call loadEncryptionKey directly.

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

	parts := strings.SplitN(encrypted, ":", 3)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d: %q", len(parts), encrypted)
	}

	if parts[0] != "v1" {
		t.Errorf("expected version 'v1', got %q", parts[0])
	}

	if parts[1] == "" {
		t.Error("nonce should not be empty")
	}

	if parts[2] == "" {
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

func TestDecryptInvalidFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"no colons", "justastring", false},             // legacy plaintext fallback
		{"two colons wrong version", "v2:abc:def", false}, // legacy plaintext fallback
		{"empty", "", false},                              // legacy plaintext fallback
		{"valid format bad hex nonce", "v1:zzzz:def", true},
		{"valid format bad hex ciphertext", "v1:abcd:zzzz", true},
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
				if result != tt.input {
					t.Errorf("legacy fallback should return input as-is: got %q, want %q", result, tt.input)
				}
			}
		})
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	encrypted, err := Encrypt("secret data")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	parts := strings.SplitN(encrypted, ":", 3)

	// Flip bytes in the ciphertext to break the GCM auth tag.
	ciphertext := parts[2]
	if len(ciphertext) < 4 {
		t.Fatal("ciphertext too short for tamper test")
	}
	tampered := ciphertext[:len(ciphertext)-4] + "0000"
	if tampered == ciphertext {
		tampered = ciphertext[:len(ciphertext)-4] + "ffff"
	}

	tamperedEncrypted := "v1:" + parts[1] + ":" + tampered

	_, err = Decrypt(tamperedEncrypted)
	if err == nil {
		t.Error("decrypting tampered ciphertext should fail (GCM auth tag mismatch)")
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
	// Legacy plaintext JSON (not encrypted, not a JSON string).
	legacy := json.RawMessage(`{"api_key":"sk-plaintext","endpoint":"https://old.example.com"}`)

	result, err := DecryptJSON(legacy)
	if err != nil {
		t.Fatalf("DecryptJSON on legacy plaintext should not error: %v", err)
	}

	if string(result) != string(legacy) {
		t.Errorf("legacy plaintext should be returned as-is: got %q, want %q", result, legacy)
	}
}

func TestProductionRequiresKey(t *testing.T) {
	t.Setenv("JC_ENV", "production")
	t.Setenv("JC_ENCRYPTION_KEY", "")
	t.Setenv("JC_STOCK_ENCRYPTION_KEY", "")
	t.Setenv("JC_TOTP_ENCRYPTION_KEY", "")

	// Test loadEncryptionKey directly (bypasses the OnceValues cache).
	_, err := loadEncryptionKey()
	if err == nil {
		t.Error("loadEncryptionKey should return error in production without a key")
	}
	if !strings.Contains(err.Error(), "JC_ENCRYPTION_KEY") {
		t.Errorf("error should mention JC_ENCRYPTION_KEY, got: %v", err)
	}
}

func TestKeyDerivationConsistency(t *testing.T) {
	// The same env var value should always derive the same key.
	t.Setenv("JC_ENCRYPTION_KEY", "consistent-test-key")
	t.Setenv("JC_ENV", "development")

	key1, err := loadEncryptionKey()
	if err != nil {
		t.Fatalf("first loadEncryptionKey failed: %v", err)
	}

	key2, err := loadEncryptionKey()
	if err != nil {
		t.Fatalf("second loadEncryptionKey failed: %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("derived key should be 32 bytes (AES-256), got %d", len(key1))
	}

	for i := range key1 {
		if key1[i] != key2[i] {
			t.Fatal("same env var should derive the same key")
		}
	}
}

func TestKeyPriorityOrder(t *testing.T) {
	// JC_ENCRYPTION_KEY takes priority over JC_STOCK_ENCRYPTION_KEY and
	// JC_TOTP_ENCRYPTION_KEY.
	t.Setenv("JC_ENCRYPTION_KEY", "primary-key")
	t.Setenv("JC_STOCK_ENCRYPTION_KEY", "stock-key")
	t.Setenv("JC_TOTP_ENCRYPTION_KEY", "totp-key")
	t.Setenv("JC_ENV", "development")

	key, err := loadEncryptionKey()
	if err != nil {
		t.Fatalf("loadEncryptionKey failed: %v", err)
	}

	// Verify the key was derived from "primary-key", not the fallbacks.
	expected := sha256Sum("primary-key")
	for i := range key {
		if key[i] != expected[i] {
			t.Fatal("JC_ENCRYPTION_KEY should take priority over fallback env vars")
		}
	}
}

func TestKeyFallbackToStockKey(t *testing.T) {
	t.Setenv("JC_ENCRYPTION_KEY", "")
	t.Setenv("JC_STOCK_ENCRYPTION_KEY", "stock-key")
	t.Setenv("JC_TOTP_ENCRYPTION_KEY", "")
	t.Setenv("JC_ENV", "development")

	key, err := loadEncryptionKey()
	if err != nil {
		t.Fatalf("loadEncryptionKey failed: %v", err)
	}

	expected := sha256Sum("stock-key")
	for i := range key {
		if key[i] != expected[i] {
			t.Fatal("should fall back to JC_STOCK_ENCRYPTION_KEY")
		}
	}
}

func TestKeyFallbackToTOTPKey(t *testing.T) {
	t.Setenv("JC_ENCRYPTION_KEY", "")
	t.Setenv("JC_STOCK_ENCRYPTION_KEY", "")
	t.Setenv("JC_TOTP_ENCRYPTION_KEY", "totp-key")
	t.Setenv("JC_ENV", "development")

	key, err := loadEncryptionKey()
	if err != nil {
		t.Fatalf("loadEncryptionKey failed: %v", err)
	}

	expected := sha256Sum("totp-key")
	for i := range key {
		if key[i] != expected[i] {
			t.Fatal("should fall back to JC_TOTP_ENCRYPTION_KEY")
		}
	}
}

// sha256Sum is a test helper that mirrors the key derivation in loadEncryptionKey.
func sha256Sum(s string) [32]byte {
	return sha256.Sum256([]byte(s))
}

func TestMain(m *testing.M) {
	// Ensure a default key is set for all tests that don't explicitly set one.
	// The key is cached via sync.OnceValues on first use, so this must be set
	// before any test calls Encrypt/Decrypt.
	if os.Getenv("JC_ENCRYPTION_KEY") == "" &&
		os.Getenv("JC_STOCK_ENCRYPTION_KEY") == "" &&
		os.Getenv("JC_TOTP_ENCRYPTION_KEY") == "" {
		os.Setenv("JC_ENCRYPTION_KEY", "default-test-key-for-crypto-tests")
		os.Setenv("JC_ENV", "development")
	}
	os.Exit(m.Run())
}
