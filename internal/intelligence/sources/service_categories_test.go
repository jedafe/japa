package sources

import (
	"os"
	"testing"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// TestNormalizeCategories covers the shared normalization Propose and the
// admin create/update paths use: trim, lowercase, drop empties, dedupe —
// order preserved.
func TestNormalizeCategories(t *testing.T) {
	t.Parallel()

	got := NormalizeCategories([]string{" News ", "", "JOB", "news", "  "})
	want := []string{"news", "job"}
	if len(got) != len(want) {
		t.Fatalf("NormalizeCategories = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if got := NormalizeCategories(nil); len(got) != 0 {
		t.Errorf("NormalizeCategories(nil) = %v, want empty", got)
	}
}

// TestServiceCategories_LiveDB exercises the service-level multi-category
// behavior against a live Postgres (JC_TEST_DATABASE_URL): Create
// normalizes the array and dual-writes the legacy scalar when it is empty,
// while a nil array keeps the INSERT's ARRAY[category] fallback;
// UpdateCategories replaces by direct assignment and clears on empty.
func TestServiceCategories_LiveDB(t *testing.T) {
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping live Postgres test")
	}

	ctx := t.Context()

	database, err := db.New(ctx, dsn)
	if err != nil {
		t.Skipf("test database unreachable: %v", err)
	}
	defer database.Close()

	svc := NewService(NewStore(database))

	base := gen.CreateIntelSourceParams{
		Name:               "Categories Service Test Source",
		SourceUrl:          "https://svc-categories-test.example.com/feed",
		SourceType:         "static",
		TrustLevel:         3,
		CrawlMethod:        "manual",
		CrawlFrequency:     "manual",
		CrawlFrequencyMins: 0,
		Enabled:            false,
	}

	// --- Create: non-nil categories normalize + dual-write empty category ---
	arg := base
	arg.Categories = []string{" News ", "job", "NEWS"}

	src, err := svc.Create(ctx, arg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = svc.Delete(ctx, src.ID) }()

	if len(src.Categories) != 2 || src.Categories[0] != "news" || src.Categories[1] != "job" {
		t.Errorf("categories = %v, want [news job]", src.Categories)
	}
	if src.Category != "news" {
		t.Errorf("category = %q, want news (dual-write of categories[0])", src.Category)
	}

	// --- UpdateCategories replaces by direct assignment ---
	updated, err := svc.UpdateCategories(ctx, gen.UpdateIntelSourceCategoriesParams{
		ID:         src.ID,
		Categories: []string{" Visa ", "travel_alert"},
	})
	if err != nil {
		t.Fatalf("update categories: %v", err)
	}
	if len(updated.Categories) != 2 || updated.Categories[0] != "visa" || updated.Categories[1] != "travel_alert" {
		t.Errorf("categories = %v, want [visa travel_alert]", updated.Categories)
	}
	// The dedicated update does not dual-write category — the caller owns
	// that decision (the admin handler writes it through the scalar update).
	if updated.Category != "news" {
		t.Errorf("category = %q, want news (unchanged by UpdateCategories)", updated.Category)
	}

	// --- UpdateCategories with empty array is an explicit clear ---
	updated, err = svc.UpdateCategories(ctx, gen.UpdateIntelSourceCategoriesParams{
		ID:         src.ID,
		Categories: []string{},
	})
	if err != nil {
		t.Fatalf("clear categories: %v", err)
	}
	if len(updated.Categories) != 0 {
		t.Errorf("categories = %v, want [] (explicit clear)", updated.Categories)
	}

	// --- Create: nil categories keeps the ARRAY[category] SQL fallback ---
	arg2 := base
	arg2.Name = "Categories Fallback Source"
	arg2.SourceUrl = "https://svc-categories-fallback.example.com/feed"
	arg2.Category = "news"

	src2, err := svc.Create(ctx, arg2)
	if err != nil {
		t.Fatalf("create fallback: %v", err)
	}
	defer func() { _ = svc.Delete(ctx, src2.ID) }()

	if len(src2.Categories) != 1 || src2.Categories[0] != "news" {
		t.Errorf("categories = %v, want [news] (ARRAY[category] fallback)", src2.Categories)
	}
}
