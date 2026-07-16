package spam

import (
	"testing"
)

func TestComputeContentHash_Consistent(t *testing.T) {
	t.Parallel()

	body := "This is a test post about immigration to the UK."
	hash1 := ComputeContentHash(body)
	hash2 := ComputeContentHash(body)

	if hash1 == "" {
		t.Fatal("ComputeContentHash returned empty string")
	}

	if hash1 != hash2 {
		t.Errorf("same input produced different hashes: %s != %s", hash1, hash2)
	}
}

func TestComputeContentHash_Normalization(t *testing.T) {
	t.Parallel()
	// Different whitespace/casing should produce the same hash
	body1 := "  Hello   World  "
	body2 := "hello world"
	hash1 := ComputeContentHash(body1)
	hash2 := ComputeContentHash(body2)

	if hash1 != hash2 {
		t.Errorf("normalized inputs should produce same hash: %s != %s", hash1, hash2)
	}
}

func TestComputeContentHash_DifferentContent(t *testing.T) {
	t.Parallel()

	hash1 := ComputeContentHash("cats are great")
	hash2 := ComputeContentHash("dogs are great")

	if hash1 == hash2 {
		t.Error("different content produced same hash")
	}
}

func TestCheckCensoredWords_Block(t *testing.T) {
	t.Parallel()

	patterns := []CensoredWord{
		{Pattern: "spam", Action: "block"},
	}
	result := CheckCensoredWords(patterns, "This is spam content")

	if !result.Blocked {
		t.Error("expected Blocked=true for block action")
	}

	if result.CleanText == "" {
		t.Error("CleanText should be set even when blocked")
	}
}

func TestCheckCensoredWords_Replace(t *testing.T) {
	t.Parallel()

	patterns := []CensoredWord{
		{Pattern: "badword", Action: CensorActionReplace, Replacement: "****"},
	}
	result := CheckCensoredWords(patterns, "This contains badword here")

	if result.Blocked {
		t.Error("expected Blocked=false for replace action")
	}

	if result.CleanText != "This contains **** here" {
		t.Errorf("CleanText = %q, want %q", result.CleanText, "This contains **** here")
	}
}

func TestCheckCensoredWords_Flag(t *testing.T) {
	t.Parallel()

	patterns := []CensoredWord{
		{Pattern: "suspicious", Action: "flag"},
	}
	result := CheckCensoredWords(patterns, "This is suspicious content")

	if result.Blocked {
		t.Error("expected Blocked=false for flag action")
	}

	if !result.Flagged {
		t.Error("expected Flagged=true for flag action")
	}

	if result.FlagReason == "" {
		t.Error("FlagReason should be set")
	}
}

func TestCheckCensoredWords_CaseInsensitive(t *testing.T) {
	t.Parallel()

	patterns := []CensoredWord{
		{Pattern: "spam", Action: "block"},
	}
	result := CheckCensoredWords(patterns, "This is SPAM content")

	if !result.Blocked {
		t.Error("expected case-insensitive match to block")
	}
}

func TestCheckCensoredWords_NoMatch(t *testing.T) {
	t.Parallel()

	patterns := []CensoredWord{
		{Pattern: "spam", Action: "block"},
	}
	result := CheckCensoredWords(patterns, "This is clean content")

	if result.Blocked {
		t.Error("expected Blocked=false when no pattern matches")
	}

	if result.Flagged {
		t.Error("expected Flagged=false when no pattern matches")
	}

	if result.CleanText != "This is clean content" {
		t.Errorf("CleanText = %q, want original text", result.CleanText)
	}
}

func TestCheckCensoredWords_RegexPattern(t *testing.T) {
	t.Parallel()

	patterns := []CensoredWord{
		{Pattern: `buy\s+now`, Action: "block"},
	}
	result := CheckCensoredWords(patterns, "Please buy now today")

	if !result.Blocked {
		t.Error("expected regex pattern to match")
	}
}

func TestCheckCensoredWords_MultipleActions(t *testing.T) {
	t.Parallel()
	// Block takes precedence over flag
	patterns := []CensoredWord{
		{Pattern: "suspicious", Action: "flag"},
		{Pattern: "spam", Action: "block"},
	}
	result := CheckCensoredWords(patterns, "This is spam content")

	if !result.Blocked {
		t.Error("expected block to take precedence over flag")
	}
}

func TestCheckCensoredWords_EmptyPatterns(t *testing.T) {
	t.Parallel()

	result := CheckCensoredWords([]CensoredWord{}, "Any text")

	if result.Blocked {
		t.Error("expected Blocked=false with empty patterns")
	}

	if result.CleanText != "Any text" {
		t.Errorf("CleanText = %q, want original text", result.CleanText)
	}
}

func TestBlockTypeConstants(t *testing.T) {
	t.Parallel()

	if BlockTypeFull != "full" {
		t.Errorf("BlockTypeFull = %q, want %q", BlockTypeFull, "full")
	}

	if BlockTypeRegistration != "registration" {
		t.Errorf("BlockTypeRegistration = %q", BlockTypeRegistration)
	}

	if BlockTypePosting != "posting" {
		t.Errorf("BlockTypePosting = %q", BlockTypePosting)
	}

	if BlockTypeAPI != "api" {
		t.Errorf("BlockTypeAPI = %q", BlockTypeAPI)
	}
}

func TestCensorActionConstants(t *testing.T) {
	t.Parallel()

	if CensorActionBlock != "block" {
		t.Errorf("CensorActionBlock = %q", CensorActionBlock)
	}

	if CensorActionReplace != "replace" {
		t.Errorf("CensorActionReplace = %q", CensorActionReplace)
	}

	if CensorActionFlag != "flag" {
		t.Errorf("CensorActionFlag = %q", CensorActionFlag)
	}
}

func TestIsValidBlockType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"full", true},
		{"registration", true},
		{"posting", true},
		{"api", true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsValidBlockType(tt.input)
		if got != tt.want {
			t.Errorf("IsValidBlockType(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsValidCensorAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"block", true},
		{"replace", true},
		{"flag", true},
		{"delete", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsValidCensorAction(tt.input)
		if got != tt.want {
			t.Errorf("IsValidCensorAction(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeForHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"  Hello   World  ", "hello world"},
		{"no change", "no change"},
		{"UPPERCASE", "uppercase"},
		{"  multiple   spaces   between  ", "multiple spaces between"},
	}

	for _, tt := range tests {
		got := normalizeForHash(tt.input)
		if got != tt.want {
			t.Errorf("normalizeForHash(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
