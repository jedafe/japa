package housingguide

import (
	"encoding/json"
	"testing"
)

func TestRentRangesWithCFVOverlay(t *testing.T) {
	base := json.RawMessage(`{"rent_monthly":800,"deposit_months":2}`)
	cfv := map[string]any{"rent_monthly": 950.0, "utilities_monthly": 120.0}

	merged := rentRangesWithCFV(base, cfv)

	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatalf("merged rent_ranges not valid JSON: %v", err)
	}

	if m["rent_monthly"] != 950.0 {
		t.Fatalf("expected cfv to shadow per key, got %v", m["rent_monthly"])
	}

	if m["deposit_months"] != 2.0 {
		t.Fatalf("expected stored key preserved, got %v", m["deposit_months"])
	}

	if m["utilities_monthly"] != 120.0 {
		t.Fatalf("expected cfv key added, got %v", m["utilities_monthly"])
	}
}

func TestRentRangesWithCFVEmptyBase(t *testing.T) {
	merged := rentRangesWithCFV(nil, map[string]any{"rent_monthly": 950.0})

	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatalf("merged rent_ranges not valid JSON: %v", err)
	}

	if m["rent_monthly"] != 950.0 {
		t.Fatalf("expected cfv key present, got %v", m["rent_monthly"])
	}
}
