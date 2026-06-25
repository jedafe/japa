package auth

import (
	"testing"
)

func TestGenerateRefreshToken(t *testing.T) {
	token, err := generateRefreshToken()
	if err != nil {
		t.Fatalf("generateRefreshToken failed: %v", err)
	}
	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("token length = %d, want 64", len(token))
	}

	// Two calls should produce different tokens
	token2, _ := generateRefreshToken()
	if token == token2 {
		t.Error("expected different tokens on subsequent calls")
	}
}

func TestHashRefreshToken(t *testing.T) {
	token := "test-token-value"
	hash := hashRefreshToken(token)
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if hash == token {
		t.Fatal("hash should not equal input")
	}

	// Same input should produce same hash
	hash2 := hashRefreshToken(token)
	if hash != hash2 {
		t.Error("hash should be deterministic")
	}

	// Different input should produce different hash
	hash3 := hashRefreshToken("different-token")
	if hash == hash3 {
		t.Error("different inputs should produce different hashes")
	}
}
