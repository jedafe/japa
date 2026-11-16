// Package newsletter — newsletter_test.go tests the Newsletter system.
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
package newsletter

import (
	"context"
	"testing"
)

func TestGenerateUnsubscribeToken(t *testing.T) {
	token, err := GenerateUnsubscribeToken()
	if err != nil {
		t.Fatalf("GenerateUnsubscribeToken error: %v", err)
	}

	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("token length = %d, want 64", len(token))
	}

	// Generate multiple tokens and verify uniqueness.
	seen := map[string]bool{token: true}

	for range 100 {
		tk, err := GenerateUnsubscribeToken()
		if err != nil {
			t.Fatalf("GenerateUnsubscribeToken error: %v", err)
		}

		if len(tk) != 64 {
			t.Errorf("token length = %d, want 64", len(tk))
		}

		if seen[tk] {
			t.Errorf("duplicate token generated: %s", tk)
		}

		seen[tk] = true
	}
}

// Double opt-in must fail closed: with no settings provider wired the
// feature is off, preserving the §62 single opt-in contract (migration
// 001170, audit NL-I1).
func TestDoubleOptInDefaultsOff(t *testing.T) {
	s := &Service{}

	if s.DoubleOptIn(context.Background()) {
		t.Error("DoubleOptIn() = true with no settings provider, want false (fail closed)")
	}
}
