// categories_test.go — unit coverage for the dynamic service_categories
// taxonomy paths that don't need a database: with an unwired store the
// hardcoded §98 enum is the fallback for both validation (categorySet)
// and the options payload (Categories).
//
// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md §2.1
package servicedir

import (
	"context"
	"testing"
)

func TestCategorySetFallsBackToStaticEnum(t *testing.T) {
	// nil store → the hardcoded §98 enum is the fallback
	s := &Service{}
	set := s.categorySet(context.Background())
	for _, slug := range []string{"immigration_legal", "relocation", "finance", "education", "health", "community", "other"} {
		if _, ok := set[slug]; !ok {
			t.Fatalf("expected %q in fallback category set", slug)
		}
	}
}

func TestCategoriesFallsBackToStaticEnum(t *testing.T) {
	// nil store → Categories serves the static enum, sorted by value.
	s := &Service{}
	opts := s.Categories(context.Background())
	if len(opts) != len(categoryLabels) {
		t.Fatalf("expected %d fallback options, got %d", len(categoryLabels), len(opts))
	}

	for _, slug := range []string{"immigration_legal", "relocation", "finance", "education", "health", "community", "other"} {
		found := false
		for _, o := range opts {
			if o.Value == slug {
				found = true

				if o.Label != categoryLabels[slug] {
					t.Fatalf("expected label %q for %q, got %q", categoryLabels[slug], slug, o.Label)
				}

				break
			}
		}
		if !found {
			t.Fatalf("expected %q in fallback Categories options", slug)
		}
	}
}
