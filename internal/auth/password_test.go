package auth

import "testing"

func TestValidatePassword(t *testing.T) {
	policy := DefaultPasswordPolicy()

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
			err := ValidatePassword(tt.password, policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
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
	if BcryptCost != 12 {
		t.Errorf("BcryptCost = %d, want 12", BcryptCost)
	}
}
