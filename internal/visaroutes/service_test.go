package visaroutes

import "testing"

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
}
