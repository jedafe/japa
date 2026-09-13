package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestMintAndValidateAccessToken(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4}, Valid: true}
	tenantID := pgtype.UUID{Valid: true}

	token, err := kr.MintAccessToken(userID, tenantID)
	if err != nil {
		t.Fatalf("MintAccessToken failed: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := kr.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}

	if claims.UserID != userID.String() {
		t.Errorf("UserID = %s, want %s", claims.UserID, userID.String())
	}

}

func TestMintAccessToken_JTIIsPresent(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4}, Valid: true}
	tenantID := pgtype.UUID{Valid: true}

	token, err := kr.MintAccessToken(userID, tenantID)
	if err != nil {
		t.Fatalf("MintAccessToken failed: %v", err)
	}

	claims, err := kr.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}

	if claims.JTI == "" {
		t.Error("expected non-empty JTI claim")
	}

	if len(claims.JTI) < 36 {
		t.Errorf("JTI looks too short: %q", claims.JTI)
	}
}

func TestMintAccessToken_UniqueJTIs(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4}, Valid: true}
	tenantID := pgtype.UUID{Valid: true}

	token1, _ := kr.MintAccessToken(userID, tenantID)
	token2, _ := kr.MintAccessToken(userID, tenantID)

	claims1, _ := kr.ValidateAccessToken(token1)
	claims2, _ := kr.ValidateAccessToken(token2)

	if claims1.JTI == claims2.JTI {
		t.Errorf("JTIs should be unique, got same JTI: %s", claims1.JTI)
	}
}

func TestValidateAccessToken_MissingJTI(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))

	// Create a token manually without JTI
	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		UserID: "user-123",

		// JTI intentionally empty
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = "key-test"
	signed, _ := token.SignedString([]byte("test-secret-key-32-bytes-long!!!"))

	_, err := kr.ValidateAccessToken(signed)
	if err == nil {
		t.Error("expected error for token missing JTI, got nil")
	}
}

func TestValidateAccessToken_ExpiredToken(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4}, Valid: true}

	// Create an already-expired token
	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
		},
		UserID: userID.String(),

		JTI: "test-jti",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = "key-test"
	signed, _ := token.SignedString([]byte("test-secret-key-32-bytes-long!!!"))

	_, err := kr.ValidateAccessToken(signed)
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

func TestKeyRotation_AddAndRemoveKey(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-old", []byte("old-secret-key-32-bytes-long!!!"))

	// Add new key
	kr.AddKey("key-new", []byte("new-secret-key-32-bytes-long!!!"))

	if len(kr.Keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(kr.Keys))
	}

	// Token signed with old key should still validate
	userID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	tenantID := pgtype.UUID{Valid: true}
	token, _ := kr.MintAccessToken(userID, tenantID)

	// Switch to new key
	if err := kr.SetCurrent("key-new"); err != nil {
		t.Fatalf("SetCurrent failed: %v", err)
	}

	// Old token should still validate (old key still in rotation)
	_, err := kr.ValidateAccessToken(token)
	if err != nil {
		t.Errorf("old token should still validate: %v", err)
	}

	// Remove old key
	kr.RemoveKey("key-old")

	_, err = kr.ValidateAccessToken(token)
	if err == nil {
		t.Error("expected error after removing old key, got nil")
	}
}

func TestValidateAccessToken_WrongSigningMethod(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))
	// Create token with wrong method
	token := jwt.NewWithClaims(jwt.SigningMethodNone, &JWTClaims{})
	token.Header["kid"] = "key-test"
	signed, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	_, err := kr.ValidateAccessToken(signed)
	if err == nil {
		t.Error("expected error for wrong signing method")
	}
}

func TestMintAccessToken_InvalidUUID(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))
	invalidID := pgtype.UUID{Valid: false}

	_, err := kr.MintAccessToken(invalidID, pgtype.UUID{})
	if err == nil {
		t.Error("expected error for invalid UUID")
	}
}

func TestIsTokenRevoked_NilClient(t *testing.T) {
	t.Parallel()
	// With nil client, token should NOT be considered revoked (fail-open).
	revoked := IsTokenRevoked(t.Context(), "any-jti", nil)
	if revoked {
		t.Error("expected false when Valkey client is nil")
	}
}

func TestValidateAccessToken_RejectsTOTPChallengeToken(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4}, Valid: true}
	tenantID := pgtype.UUID{Valid: true}

	challenge, err := kr.MintTOTPChallengeToken(userID, tenantID)
	if err != nil {
		t.Fatalf("MintTOTPChallengeToken failed: %v", err)
	}

	if _, err := kr.ValidateAccessToken(challenge); err == nil {
		t.Fatal("challenge token must not be accepted as a session token")
	}

	if _, err := kr.ValidateChallengeToken(challenge); err != nil {
		t.Fatalf("ValidateChallengeToken failed: %v", err)
	}
}

func TestValidateChallengeToken_RejectsAccessToken(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4}, Valid: true}
	tenantID := pgtype.UUID{Valid: true}

	access, err := kr.MintAccessToken(userID, tenantID)
	if err != nil {
		t.Fatalf("MintAccessToken failed: %v", err)
	}

	if _, err := kr.ValidateChallengeToken(access); err == nil {
		t.Fatal("session token must not satisfy the TOTP challenge")
	}
}
