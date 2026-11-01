package personas

import "testing"

// TestPersonaUsernamePattern covers the admin-rename validation rule —
// the same shape auth.ValidateUsername enforces for member signups.
func TestPersonaUsernamePattern(t *testing.T) {
	valid := []string{"fx_guru", "intel-visa", "ci-changelog", "AdaOra9", "abc", "a-b", strings30("a"), "a1_"}
	invalid := []string{"", "ab", "a b", "a.b", "a@b", "a/b", strings31("a"), "héllo"}

	for _, u := range valid {
		if !personaUsernamePattern.MatchString(u) {
			t.Errorf("username %q should be valid", u)
		}
	}
	for _, u := range invalid {
		if personaUsernamePattern.MatchString(u) {
			t.Errorf("username %q should be invalid", u)
		}
	}
}

func strings30(s string) string {
	for len(s) < 30 {
		s += "a"
	}
	return s
}

func strings31(s string) string {
	for len(s) < 31 {
		s += "a"
	}
	return s
}
