// Package referral — referral_v2_test.go tests the V2 Referral System.
//
// Ref: japaconnect-engineering/sections/60-referral-system.md
package referral

import (
	"strings"
	"testing"
)

func TestGenerateReferralCode(t *testing.T) {
	code, err := GenerateReferralCode()
	if err != nil {
		t.Fatalf("GenerateReferralCode error: %v", err)
	}
	if len(code) != 8 {
		t.Errorf("code length = %d, want 8", len(code))
	}

	// Generate multiple codes and verify uniqueness.
	seen := map[string]bool{code: true}
	for i := 0; i < 100; i++ {
		c, err := GenerateReferralCode()
		if err != nil {
			t.Fatalf("GenerateReferralCode error: %v", err)
		}
		if len(c) != 8 {
			t.Errorf("code length = %d, want 8", len(c))
		}
		if seen[c] {
			t.Errorf("duplicate code generated: %s", c)
		}
		seen[c] = true
	}
}

func TestGenerateReferralCode_Charset(t *testing.T) {
	// The charset should only contain valid characters (no ambiguous ones).
	const validChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code, err := GenerateReferralCode()
	if err != nil {
		t.Fatalf("GenerateReferralCode error: %v", err)
	}
	for _, c := range code {
		if !strings.ContainsRune(validChars, c) {
			t.Errorf("code contains invalid character: %c", c)
		}
	}
}
