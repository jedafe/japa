package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestValidatePassword(t *testing.T) {
	t.Parallel()
	// Use a policy without breach checking — these tests verify
	// structural password requirements (length, uppercase, etc.),
	// not breach detection.
	policy := PasswordPolicy{
		MinLength:     8,
		MaxLength:     128,
		RequireUpper:  true,
		RequireNumber: true,
		RequireSymbol: false,
		CheckBreach:   false,
	}

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid password", "Password123", false},
		{"too short", "Ab1", true},
		{"no uppercase", "password123", true},
		{"no number", "PasswordABC", true},
		{"empty password", "", true},
		{"valid with symbol", "Password123!", false},
		{"max length boundary", string(make([]byte, 128)), true}, // all zeros, fails upper/number
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePassword(tt.password, policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()

	password := "TestPassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	if hash == password {
		t.Fatal("hash should not equal plaintext")
	}

	// Correct password should verify
	if err := VerifyPassword(hash, password); err != nil {
		t.Errorf("VerifyPassword with correct password failed: %v", err)
	}

	// Wrong password should fail
	if err := VerifyPassword(hash, "wrongpassword"); err == nil {
		t.Error("VerifyPassword with wrong password should fail")
	}
}

func TestBcryptCost(t *testing.T) {
	t.Parallel()

	if BcryptCost != 12 {
		t.Errorf("BcryptCost = %d, want 12", BcryptCost)
	}
}

// ---------------------------------------------------------------------------
// Password History
// ---------------------------------------------------------------------------

// mockHistoryStore is a minimal in-memory implementation of PasswordHistoryReader
// used to test CheckPasswordHistory without a database.
type mockHistoryStore struct {
	hashes []string // most recent first
}

func (m *mockHistoryStore) GetRecentPasswordHashes(_ context.Context, _ pgtype.UUID, limit int) ([]string, error) {
	if limit > len(m.hashes) {
		limit = len(m.hashes)
	}

	out := make([]string, limit)
	copy(out, m.hashes[:limit])

	return out, nil
}

// mustTestUUID scans a UUID string into pgtype.UUID, failing the test on error.
func mustTestUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

func TestCheckPasswordHistory_ReusedPasswordRejected(t *testing.T) {
	t.Parallel()
	// Hash a password and put it in history.
	hash, err := HashPassword("OldPassword1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	store := &mockHistoryStore{hashes: []string{hash}}
	policy := PasswordPolicy{HistoryCount: 5}
	ctx := t.Context()
	userID := mustTestUUID(t, "550e8400-e29b-41d4-a716-446655440000")

	err = CheckPasswordHistory(ctx, store, userID, "OldPassword1", policy)
	if err == nil {
		t.Fatal("expected error for reused password, got nil")
	}

	var reused *PasswordReusedError
	if !is(err, &reused) {
		t.Errorf("expected PasswordReusedError, got %T: %v", err, err)
	}
}

func TestCheckPasswordHistory_UniquePasswordAccepted(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("OldPassword1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	store := &mockHistoryStore{hashes: []string{hash}}
	policy := PasswordPolicy{HistoryCount: 5}
	ctx := t.Context()
	userID := mustTestUUID(t, "550e8400-e29b-41d4-a716-446655440000")

	err = CheckPasswordHistory(ctx, store, userID, "BrandNewPassword99", policy)
	if err != nil {
		t.Errorf("expected nil for unique password, got: %v", err)
	}
}

func TestCheckPasswordHistory_HistoryLimitRespected(t *testing.T) {
	t.Parallel()
	// Put 3 hashes in history, set limit to 2 — the oldest hash should be ignored.
	hashes := make([]string, 3)
	for i := range hashes {
		h, err := HashPassword("Password" + string(rune('A'+i)) + "123")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}

		hashes[i] = h
	}

	store := &mockHistoryStore{hashes: hashes}
	policy := PasswordPolicy{HistoryCount: 2} // Only check last 2
	ctx := t.Context()
	userID := mustTestUUID(t, "550e8400-e29b-41d4-a716-446655440000")

	// The oldest password (index 2, "PasswordC123") should NOT be in the
	// checked window, so reusing it should be accepted.
	err := CheckPasswordHistory(ctx, store, userID, "PasswordC123", policy)
	if err != nil {
		t.Errorf("expected nil when oldest hash is outside history window, got: %v", err)
	}

	// The newest password (index 0, "PasswordA123") IS in the window, so
	// reusing it should be rejected.
	err = CheckPasswordHistory(ctx, store, userID, "PasswordA123", policy)
	if err == nil {
		t.Error("expected error when newest hash is inside history window, got nil")
	}
}

func TestCheckPasswordHistory_ZeroHistoryCountSkipsCheck(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("OldPassword1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	store := &mockHistoryStore{hashes: []string{hash}}
	policy := PasswordPolicy{HistoryCount: 0} // History check disabled
	ctx := t.Context()
	userID := mustTestUUID(t, "550e8400-e29b-41d4-a716-446655440000")

	err = CheckPasswordHistory(ctx, store, userID, "OldPassword1", policy)
	if err != nil {
		t.Errorf("expected nil when history count is 0, got: %v", err)
	}
}

func TestCheckPasswordHistory_EmptyHistoryAccepted(t *testing.T) {
	t.Parallel()

	store := &mockHistoryStore{hashes: nil}
	policy := PasswordPolicy{HistoryCount: 5}
	ctx := t.Context()
	userID := mustTestUUID(t, "550e8400-e29b-41d4-a716-446655440000")

	err := CheckPasswordHistory(ctx, store, userID, "AnyPassword1", policy)
	if err != nil {
		t.Errorf("expected nil with empty history, got: %v", err)
	}
}

// is checks whether err wraps a target type (simplified errors.As).
func is(err error, target interface{}) bool {
	if err == nil {
		return false
	}

	switch t := target.(type) {
	case **PasswordReusedError:
		e := &PasswordReusedError{}
		if errors.As(err, &e) {
			*t = e
			return true
		}
	}

	return false
}
