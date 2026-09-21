package fetcher

import (
	"slices"
	"testing"
)

// TestParseRobotsTXT_WildcardMultipleDisallows pins the collect fix: every
// Disallow in the wildcard block must be recorded, not just the first —
// an early len(disallowed)==0 gate used to drop all but the first rule.
func TestParseRobotsTXT_WildcardMultipleDisallows(t *testing.T) {
	t.Parallel()

	policy := parseRobotsTXT([]byte(
		"User-agent: *\n" +
			"Disallow: /admin\n" +
			"Disallow: /private\n" +
			"Disallow: /staging\n"))

	for _, want := range []string{"/admin", "/private", "/staging"} {
		if !slices.Contains(policy.DisallowedPaths, want) {
			t.Errorf("disallowed paths missing %q: %v", want, policy.DisallowedPaths)
		}
	}
	if !policy.IsPathDisallowed("/private/x") {
		t.Error("IsPathDisallowed(/private/x) = false, want true")
	}
}

// TestParseRobotsTXT_ForeignBlockDoesNotClear pins the empty-Disallow fix:
// a bare "Disallow:" inside another crawler's block means "allow all FOR
// THAT CRAWLER" and must not wipe the rules collected for our agent.
func TestParseRobotsTXT_ForeignBlockDoesNotClear(t *testing.T) {
	t.Parallel()

	policy := parseRobotsTXT([]byte(
		"User-agent: *\n" +
			"Disallow: /admin\n" +
			"User-agent: googlebot\n" +
			"Disallow:\n"))

	if !slices.Contains(policy.DisallowedPaths, "/admin") {
		t.Errorf("foreign Disallow cleared our rules: %v", policy.DisallowedPaths)
	}
	if !policy.IsPathDisallowed("/admin") {
		t.Error("IsPathDisallowed(/admin) = false, want true")
	}
}

// TestParseRobotsTXT_JapaBlockSupersedesWildcard covers UA-group
// precedence: once a dedicated JapaConnect block applies a rule, wildcard
// rules collected earlier are discarded and later wildcard rules ignored.
func TestParseRobotsTXT_JapaBlockSupersedesWildcard(t *testing.T) {
	t.Parallel()

	policy := parseRobotsTXT([]byte(
		"User-agent: *\n" +
			"Disallow: /wildcard-only\n" +
			"User-agent: japaconnect\n" +
			"Disallow: /japa-only\n" +
			"User-agent: *\n" +
			"Disallow: /also-ignored\n"))

	if !slices.Contains(policy.DisallowedPaths, "/japa-only") {
		t.Errorf("japa block rule missing: %v", policy.DisallowedPaths)
	}
	if slices.Contains(policy.DisallowedPaths, "/wildcard-only") {
		t.Error("wildcard rule survived the dedicated block — should be superseded")
	}
	if slices.Contains(policy.DisallowedPaths, "/also-ignored") {
		t.Error("post-block wildcard rule collected — should be ignored")
	}
}

// TestParseRobotsTXT_BareDisallowInWildcardAllowsAll — an empty Disallow
// inside the wildcard block still means allow-all for us.
func TestParseRobotsTXT_BareDisallowInWildcardAllowsAll(t *testing.T) {
	t.Parallel()

	policy := parseRobotsTXT([]byte(
		"User-agent: *\n" +
			"Disallow: /admin\n" +
			"Disallow:\n"))

	if len(policy.DisallowedPaths) != 0 {
		t.Errorf("bare wildcard Disallow must clear collected paths: %v", policy.DisallowedPaths)
	}
	if !policy.Allowed {
		t.Error("Allowed = false after bare Disallow, want true")
	}
}
