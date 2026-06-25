package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestMintAndValidateAccessToken(t *testing.T) {
	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4}, Valid: true}
	tenantID := pgtype.UUID{Bytes: [16]byte{5, 6, 7, 8}, Valid: true}

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
	if claims.TenantID != tenantID.String() {
		t.Errorf("TenantID = %s, want %s", claims.TenantID, tenantID.String())
	}
}

func TestValidateAccessToken_ExpiredToken(t *testing.T) {
	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4}, Valid: true}
	tenantID := pgtype.UUID{Bytes: [16]byte{5, 6, 7, 8}, Valid: true}

	// Create an already-expired token
	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
		},
		UserID:   userID.String(),
		TenantID: tenantID.String(),
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
	kr := NewKeyRotation("key-old", []byte("old-secret-key-32-bytes-long!!!"))

	// Add new key
	kr.AddKey("key-new", []byte("new-secret-key-32-bytes-long!!!"))
	if len(kr.Keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(kr.Keys))
	}

	// Token signed with old key should still validate
	userID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	tenantID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
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
	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))
	invalidID := pgtype.UUID{Valid: false}

	_, err := kr.MintAccessToken(invalidID, invalidID)
	if err == nil {
		t.Error("expected error for invalid UUID")
	}
}
