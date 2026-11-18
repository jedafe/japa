package visaroutes

import (
	"errors"
	"strings"
	"testing"
)

func TestRequirementsWithCFVShadowsPerKey(t *testing.T) {
	req := map[string]any{"minimum_funds": 5000.0, "occupations": []any{"nurse"}}
	cfv := map[string]any{"minimum_funds": 7500.0, "fee_usd": 1200.0}

	merged, ok := requirementsWithCFV(req, cfv).(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", merged)
	}

	if merged["minimum_funds"] != 7500.0 {
		t.Fatalf("expected cfv to shadow per key, got %v", merged["minimum_funds"])
	}

	if merged["fee_usd"] != 1200.0 {
		t.Fatalf("expected cfv key added, got %v", merged["fee_usd"])
	}

	if _, ok := merged["occupations"]; !ok {
		t.Fatal("expected stored requirements key preserved")
	}
}

func TestRequirementsWithCFVNonMapBase(t *testing.T) {
	cfv := map[string]any{"fee_usd": 1200.0}

	merged, ok := requirementsWithCFV(nil, cfv).(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", merged)
	}

	if merged["fee_usd"] != 1200.0 {
		t.Fatalf("expected cfv key present, got %v", merged["fee_usd"])
	}
}

func TestRequirementsWithCFVPreservesNonMapPayload(t *testing.T) {
	req := []any{"degree", "job_offer"}
	cfv := map[string]any{"fee_usd": 1200.0}

	merged, ok := requirementsWithCFV(req, cfv).(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", merged)
	}

	raw, ok := merged["_raw"].([]any)
	if !ok || len(raw) != 2 {
		t.Fatalf("expected original payload preserved under _raw, got %v", merged["_raw"])
	}

	if merged["fee_usd"] != 1200.0 {
		t.Fatalf("expected cfv key present, got %v", merged["fee_usd"])
	}
}

// sanitizeVisaGovURL accepts only absolute http(s) URLs with a host (V1).
func TestSanitizeVisaGovURL(t *testing.T) {
	valid := []string{"https://www.canada.ca/visa", "http://example.com/a?b=c"}
	for _, v := range valid {
		if _, ok := sanitizeVisaGovURL(v); !ok {
			t.Errorf("sanitizeVisaGovURL(%q) rejected a valid URL", v)
		}
	}

	invalid := []string{"", "javascript:alert(1)", "data:text/html,x", "//evil.example.com/x", "example.com", "ftp://example.com/x", "https://", "https:///path"}
	for _, v := range invalid {
		if _, ok := sanitizeVisaGovURL(v); ok {
			t.Errorf("sanitizeVisaGovURL(%q) accepted an unsafe URL", v)
		}
	}

	// Literal-IP hosts in private/loopback ranges must be rejected at write
	// time — the weekly check_links sweep HEAD-fetches whatever is stored.
	for _, v := range []string{"https://127.0.0.1/x", "http://10.0.0.5/internal", "http://[::1]/", "http://169.254.169.254/latest/meta-data"} {
		if _, ok := sanitizeVisaGovURL(v); ok {
			t.Errorf("sanitizeVisaGovURL(%q) accepted a private/loopback IP host", v)
		}
	}
}

// sanitizeVisaGovURL must return the trimmed value so callers store the
// sanitized URL, not the raw input.
func TestSanitizeVisaGovURLTrims(t *testing.T) {
	got, ok := sanitizeVisaGovURL("  https://www.canada.ca/visa \n")
	if !ok {
		t.Fatal("rejected a padded valid URL")
	}

	if got != "https://www.canada.ca/visa" {
		t.Fatalf("expected trimmed URL stored, got %q", got)
	}
}

// ADR-088 HITL gate: PATCH may only carry reviewable statuses —
// 'published' is approve-only, 'ai_draft' is generate-only.
func TestValidatePatchStatus(t *testing.T) {
	for _, s := range []string{StatusDraft, StatusPending, StatusRejected} {
		if err := validatePatchStatus(s); err != nil {
			t.Errorf("validatePatchStatus(%q) rejected a reviewable status: %v", s, err)
		}
	}

	for _, s := range []string{StatusPublished, StatusAIDraft, "bogus"} {
		if err := validatePatchStatus(s); !errors.Is(err, ErrInvalidStatus) {
			t.Errorf("validatePatchStatus(%q) = %v, want ErrInvalidStatus", s, err)
		}
	}
}

// A repeat Generate of the same (category, prompt) must not collide on
// the (destination, slug) unique index, and category text must be
// slug-safe.
func TestAIDraftSlugUniqueAndSafe(t *testing.T) {
	a := aiDraftSlug("Intra Company Transfer!", "deadbeefcafe")
	b := aiDraftSlug("Intra Company Transfer!", "deadbeefcafe")

	if a == b {
		t.Fatal("identical inputs produced identical slugs — repeat Generate would 409")
	}

	for _, slug := range []string{a, b} {
		if !strings.HasPrefix(slug, "ai-draft-intra-company-transfer-deadbe-") {
			t.Errorf("slug %q lost the category prefix or prompt hash", slug)
		}
	}
}

// originTerms order matters: [code, name] — the store query treats
// term[0] as the exact-match geo code and term[1] as the ILIKE display
// name.
func TestOriginTerms(t *testing.T) {
	code, name := "US", "United States"

	terms := originTerms(&code, &name)
	if len(terms) != 2 || terms[0] != "US" || terms[1] != "United States" {
		t.Fatalf("expected [code, name], got %v", terms)
	}

	if got := originTerms(&code, nil); len(got) != 1 || got[0] != "US" {
		t.Fatalf("expected [code] with nil name, got %v", got)
	}

	dup := "CA"
	if got := originTerms(&dup, &dup); len(got) != 1 {
		t.Fatalf("expected dedup of identical code/name, got %v", got)
	}
}
