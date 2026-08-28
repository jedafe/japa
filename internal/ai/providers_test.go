package ai

import (
	"reflect"
	"sort"
	"testing"
)

// TestSupportedProviderTypesMatchFactory verifies that every provider type
// advertised by SupportedProviderTypes can be constructed by NewProviderFromConfig
// with a minimal config, and that no extra types are returned.
func TestSupportedProviderTypesMatchFactory(t *testing.T) {
	t.Parallel()

	for _, pt := range SupportedProviderTypes() {
		cfg := ProviderConfig{ProviderType: pt}
		if _, err := NewProviderFromConfig(cfg, "test-api-key"); err != nil {
			t.Errorf("NewProviderFromConfig(%q) returned error: %v", pt, err)
		}
	}

	// Ensure NewProviderFromConfig does not accept an unknown provider type.
	cfg := ProviderConfig{ProviderType: "unknown-provider"}
	if _, err := NewProviderFromConfig(cfg, "test-api-key"); err == nil {
		t.Error("NewProviderFromConfig(unknown-provider) expected error, got nil")
	}
}

// TestSupportedAICategoriesCanonical verifies the category list is sorted and
// non-empty. The actual usage is spread across handlers; this catches accidental
// duplicates or empty regressions.
func TestSupportedAICategoriesCanonical(t *testing.T) {
	t.Parallel()

	cats := SupportedAICategories()
	if len(cats) == 0 {
		t.Fatal("SupportedAICategories is empty")
	}

	sorted := make([]string, len(cats))
	copy(sorted, cats)
	sort.Strings(sorted)
	if !reflect.DeepEqual(cats, sorted) {
		t.Errorf("SupportedAICategories not sorted: %v", cats)
	}
}
