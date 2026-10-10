package auth

import "testing"

// TestValidateUsername covers the §50 username contract enforced on every
// user-supplied username write (register, PATCH /me/account,
// POST /admin/users): ^[a-zA-Z0-9_]{3,30}$.
func TestValidateUsername(t *testing.T) {
	valid := []string{
		"abc",                            // minimum length
		"japa_user",                      // underscore
		"User123",                        // mixed case + digits
		"_leading",                       // leading underscore is allowed by the charset
		"trailing_",                      // trailing underscore
		"012345678901234567890123456789", // 30 chars — maximum
	}
	for _, u := range valid {
		if !ValidateUsername(u) {
			t.Errorf("ValidateUsername(%q) = false, want true", u)
		}
	}

	invalid := []string{
		"",                                // empty
		"ab",                              // too short
		"a-b",                             // hyphen not in charset
		"user name",                       // space
		"user.name",                       // dot
		"user@name",                       // symbol
		"usér",                            // non-ASCII
		"0123456789012345678901234567890", // 31 chars — over maximum
		"   ",                             // whitespace only
	}
	for _, u := range invalid {
		if ValidateUsername(u) {
			t.Errorf("ValidateUsername(%q) = true, want false", u)
		}
	}
}
