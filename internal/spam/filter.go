package spam

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// CensorResult holds the outcome of a content filter check.
type CensorResult struct {
	// Blocked is true when content must be rejected entirely (action=block).
	Blocked bool
	// Flagged is true when content should be held for moderation review (action=flag).
	Flagged bool
	// FlagReason explains why the content was flagged or blocked.
	FlagReason string
	// CleanText is the content after replacements have been applied.
	// When action=block, this is the original text (caller should not persist).
	// When action=flag, this is the original text (caller should persist with flag).
	// When action=replace, this is the text with censored words replaced.
	CleanText string
}

// CensoredWord represents a single censored word pattern and its action.
type CensoredWord struct {
	ID          string
	Pattern     string
	Action      string
	Replacement string
}

// CheckCensoredWords checks text against a list of censored word patterns.
// Each pattern is tested against the lowercased text. The function returns
// the first blocking match immediately. If no blocking match is found, it
// returns any flagged match. If no matches are found, CleanText equals the
// original text.
func CheckCensoredWords(patterns []CensoredWord, text string) CensorResult {
	lowerText := strings.ToLower(text)
	result := CensorResult{CleanText: text}

	for _, pw := range patterns {
		matched, err := matchPattern(pw.Pattern, lowerText)
		if err != nil || !matched {
			continue
		}

		switch pw.Action {
		case "block":
			result.Blocked = true
			result.FlagReason = fmt.Sprintf("content contains blocked pattern: %s", pw.Pattern)
			return result
		case "flag":
			result.Flagged = true
			result.FlagReason = fmt.Sprintf("content contains flagged pattern: %s", pw.Pattern)
			// Continue scanning — a later block action takes precedence
		case "replace":
			result.CleanText = applyReplacement(pw.Pattern, pw.Replacement, result.CleanText)
		}
	}

	return result
}

// matchPattern tests whether pattern matches text. The pattern is treated
// as a Go regular expression. If it fails to compile, it falls back to
// literal substring matching.
func matchPattern(pattern, text string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		// Fall back to literal substring match
		return strings.Contains(text, strings.ToLower(pattern)), nil
	}
	return re.MatchString(text), nil
}

// applyReplacement replaces all occurrences of pattern in text with the
// given replacement string. Uses Go regex for pattern matching.
func applyReplacement(pattern, replacement, text string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		// Fall back to literal replacement
		return strings.ReplaceAll(text, strings.ToLower(pattern), replacement)
	}
	return re.ReplaceAllString(text, replacement)
}

// ComputeContentHash computes the SHA-256 hex digest of normalized content.
// Normalization strips leading/trailing whitespace and collapses internal
// whitespace to single spaces, then lowercases the result. This ensures
// that cosmetic differences (extra spaces, mixed case) do not produce
// different hashes for semantically identical content.
func ComputeContentHash(body string) string {
	normalized := normalizeForHash(body)
	h := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", h)
}

// normalizeForHash prepares content for hashing by trimming whitespace,
// collapsing internal runs of whitespace to a single space, and lowercasing.
func normalizeForHash(s string) string {
	s = strings.TrimSpace(s)
	// Collapse whitespace runs
	fields := strings.Fields(s)
	return strings.ToLower(strings.Join(fields, " "))
}

// BlockType constants for IP blocking.
const (
	BlockTypeFull        = "full"
	BlockTypeRegistration = "registration"
	BlockTypePosting     = "posting"
	BlockTypeAPI         = "api"
)

// CensorAction constants for censored word actions.
const (
	CensorActionBlock   = "block"
	CensorActionReplace = "replace"
	CensorActionFlag    = "flag"
)

// ValidBlockTypes returns the set of valid IP block types.
func ValidBlockTypes() []string {
	return []string{BlockTypeFull, BlockTypeRegistration, BlockTypePosting, BlockTypeAPI}
}

// ValidCensorActions returns the set of valid censored word actions.
func ValidCensorActions() []string {
	return []string{CensorActionBlock, CensorActionReplace, CensorActionFlag}
}

// IsValidBlockType checks if a block type string is valid.
func IsValidBlockType(bt string) bool {
	for _, v := range ValidBlockTypes() {
		if bt == v {
			return true
		}
	}
	return false
}

// IsValidCensorAction checks if a censor action string is valid.
func IsValidCensorAction(a string) bool {
	for _, v := range ValidCensorActions() {
		if a == v {
			return true
		}
	}
	return false
}
