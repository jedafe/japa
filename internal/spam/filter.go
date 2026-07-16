package spam

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// --- Regex safety: ReDoS protection ---

// regexCacheEntry stores a compiled regex with its expiration time.
type regexCacheEntry struct {
	re        *regexp.Regexp
	expiresAt time.Time
}

var (
	regexCacheMu sync.RWMutex
	regexCache   = make(map[string]regexCacheEntry)
)

// regexCacheTTL controls how long compiled regexps stay in the in-memory
// cache. A short TTL limits memory use while still avoiding recompilation
// during burst traffic.
const regexCacheTTL = 10 * time.Minute

// defaultRegexTimeout is the maximum time allowed for regexp.Compile.
// Patterns that exceed this are assumed to be catastrophic backtracking
// (ReDoS) and are rejected.
const defaultRegexTimeout = 100 * time.Millisecond

// safeCompileRegex compiles a regular expression with a timeout to prevent
// ReDoS attacks from catastrophic backtracking patterns. Compiled regexps
// are cached to avoid repeated compilation overhead.
//
// If the pattern fails to compile within the timeout, an error is returned
// and the caller should fall back to literal matching.
func safeCompileRegex(pattern string, timeout time.Duration) (*regexp.Regexp, error) {
	// Fast path: check the cache first.
	regexCacheMu.RLock()
	entry, ok := regexCache[pattern]
	regexCacheMu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		return entry.re, nil
	}

	// Slow path: compile with timeout.
	type compileResult struct {
		re  *regexp.Regexp
		err error
	}

	ch := make(chan compileResult, 1)

	go func() {
		re, err := regexp.Compile(pattern)
		select {
		case ch <- compileResult{re: re, err: err}:
		default:
		}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil, fmt.Errorf("regex compilation timed out after %v (possible ReDoS pattern)", timeout)
	case result := <-ch:
		if result.err != nil {
			return nil, result.err
		}

		// Cache the successful compilation.
		regexCacheMu.Lock()
		regexCache[pattern] = regexCacheEntry{
			re:        result.re,
			expiresAt: time.Now().Add(regexCacheTTL),
		}
		regexCacheMu.Unlock()

		return result.re, nil
	}
}

// EvictExpiredRegexCache removes entries whose TTL has expired. Call this
// periodically (or on startup) to reclaim memory. It is safe to call from
// any goroutine.
func EvictExpiredRegexCache() {
	now := time.Now()

	regexCacheMu.Lock()
	defer regexCacheMu.Unlock()

	for key, entry := range regexCache {
		if now.After(entry.expiresAt) {
			delete(regexCache, key)
		}
	}
}

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
			result.FlagReason = "content contains blocked pattern: " + pw.Pattern

			return result
		case "flag":
			result.Flagged = true
			result.FlagReason = "content contains flagged pattern: " + pw.Pattern
			// Continue scanning — a later block action takes precedence
		case CensorActionReplace:
			result.CleanText = applyReplacement(pw.Pattern, pw.Replacement, result.CleanText)
		}
	}

	return result
}

// matchPattern tests whether pattern matches text. The pattern is treated
// as a Go regular expression. If it fails to compile or exceeds the ReDoS
// timeout, it falls back to literal substring matching.
func matchPattern(pattern, text string) (bool, error) {
	re, err := safeCompileRegex(pattern, defaultRegexTimeout)
	if err != nil {
		// Fall back to literal substring match
		return strings.Contains(text, strings.ToLower(pattern)), nil //nolint:nilerr // intentional: fallback to literal match
	}

	return re.MatchString(text), nil
}

// applyReplacement replaces all occurrences of pattern in text with the
// given replacement string. Uses Go regex for pattern matching. Falls back
// to literal replacement if the pattern exceeds the ReDoS timeout.
func applyReplacement(pattern, replacement, text string) string {
	re, err := safeCompileRegex(pattern, defaultRegexTimeout)
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

	return hex.EncodeToString(h[:])
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
	BlockTypeFull         = "full"
	BlockTypeRegistration = "registration"
	BlockTypePosting      = "posting"
	BlockTypeAPI          = "api"
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
