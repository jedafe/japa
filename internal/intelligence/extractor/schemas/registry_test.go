package schemas

import (
	"slices"
	"testing"
)

// TestCurateOnlyEntityTypes covers the intel-channels Phase 1 contract:
// article/event/dataset have registered schemas (so curated records
// validate) but are curate-only — IsCurateOnly reports them so the extract
// dispatch can refuse to burn an LLM call on a template-less type.
func TestCurateOnlyEntityTypes(t *testing.T) {
	t.Parallel()

	for _, et := range []string{EntityTypeArticle, EntityTypeEvent, EntityTypeDataset} {
		if _, ok := ForEntityType(et); !ok {
			t.Errorf("entity type %q not registered", et)
		}
		if !IsCurateOnly(et) {
			t.Errorf("IsCurateOnly(%q) = false, want true", et)
		}
	}

	for _, et := range []string{EntityTypeNews, EntityTypeJob, EntityTypeScholarship} {
		if IsCurateOnly(et) {
			t.Errorf("IsCurateOnly(%q) = true, want false (LLM-extractable)", et)
		}
	}

	if IsCurateOnly("bogus") {
		t.Error("IsCurateOnly(bogus) = true, want false for unregistered type")
	}
}

// TestCurateCategoryMappings covers the intel-channels category → entity
// type additions: blog/event/dataset map to the curate-only schemas,
// announcement reuses news, and scam_alert stays unmapped (no schema) so
// its sources land in the review queue.
func TestCurateCategoryMappings(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"blog":         EntityTypeArticle,
		"event":        EntityTypeEvent,
		"dataset":      EntityTypeDataset,
		"announcement": EntityTypeNews,
	}
	for cat, et := range want {
		if got := CategoryToEntityType(cat); got != et {
			t.Errorf("CategoryToEntityType(%q) = %q, want %q", cat, got, et)
		}
	}

	if got := CategoryToEntityType("scam_alert"); got != "" {
		t.Errorf("CategoryToEntityType(scam_alert) = %q, want unmapped", got)
	}

	for _, cat := range []string{"blog", "event", "dataset", "announcement", "scam_alert"} {
		if !slices.Contains(RegisteredCategories(), cat) {
			t.Errorf("RegisteredCategories missing %q", cat)
		}
	}
}
