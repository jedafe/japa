package sources

import (
	"os"
	"testing"

	"github.com/jedafe/japa/internal/db"
)

// TestSubmitOneshot exercises the Phase-2 oneshot contract against a live
// Postgres: first submission creates the shared system source + a oneshot
// channel, an identical resubmission dedupes to the existing row, and
// resubmitting a channel the failure breaker disabled re-arms it.
// Gated on JC_TEST_DATABASE_URL like the channel-store tests.
func TestSubmitOneshot(t *testing.T) {
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

	store := NewStore(database)
	svc := NewService(store)

	// The test deletes the shared system source on cleanup — refuse to
	// run against a database that already owns one (it would carry real
	// user-submitted channels).
	if _, err := store.GetBySlug(ctx, oneshotSourceSlug); err == nil {
		t.Skip("oneshot system source already exists in this database")
	}

	entryURL := "https://example.com/oneshot-dedupe-test"

	ch1, created, err := svc.SubmitOneshot(ctx, entryURL, []string{"news"}, "")
	if err != nil {
		t.Fatalf("SubmitOneshot: %v", err)
	}

	if !created {
		t.Fatal("first submission: created = false, want true")
	}

	if ch1.CrawlMethod != string(CrawlMethodOneshot) || ch1.Origin != string(ChannelOriginUser) || !ch1.Enabled {
		t.Errorf("channel = method %q origin %q enabled %v, want oneshot/user/enabled",
			ch1.CrawlMethod, ch1.Origin, ch1.Enabled)
	}

	src, err := store.GetBySlug(ctx, oneshotSourceSlug)
	if err != nil {
		t.Fatalf("get oneshot source after submit: %v", err)
	}

	defer func() { _ = store.Delete(ctx, src.ID) }()

	if ch1.SourceID != src.ID {
		t.Errorf("channel source = %s, want shared source %s", ch1.SourceID, src.ID)
	}

	if src.SourceType != "system" || src.Slug == nil || *src.Slug != oneshotSourceSlug {
		t.Errorf("system source = type %q slug %v, want system/%s", src.SourceType, src.Slug, oneshotSourceSlug)
	}

	// Identical resubmission dedupes — same row, created=false.
	ch2, created, err := svc.SubmitOneshot(ctx, entryURL, []string{"news"}, "")
	if err != nil {
		t.Fatalf("SubmitOneshot resubmit: %v", err)
	}

	if created || ch2.ID != ch1.ID {
		t.Fatalf("resubmit created=%v id=%s, want dedupe to %s", created, ch2.ID, ch1.ID)
	}

	// Scheme-less input normalizes to https and dedupes to the same row.
	ch3, created, err := svc.SubmitOneshot(ctx, "example.com/oneshot-dedupe-test", []string{"news"}, "")
	if err != nil {
		t.Fatalf("SubmitOneshot scheme-less: %v", err)
	}

	if created || ch3.ID != ch1.ID {
		t.Fatalf("scheme-less resubmit created=%v id=%s, want dedupe to %s", created, ch3.ID, ch1.ID)
	}

	// A channel the consecutive-failures breaker disabled is re-armed by
	// an explicit resubmission — otherwise the queued fetch would skip
	// silently while the caller saw 202.
	if err := store.UpdateChannelEnabled(ctx, ch1.ID, false); err != nil {
		t.Fatalf("disable channel: %v", err)
	}

	ch4, created, err := svc.SubmitOneshot(ctx, entryURL, []string{"news"}, "")
	if err != nil {
		t.Fatalf("SubmitOneshot after disable: %v", err)
	}

	if created || ch4.ID != ch1.ID || !ch4.Enabled {
		t.Fatalf("resubmit-after-disable created=%v enabled=%v id=%s, want re-armed %s",
			created, ch4.Enabled, ch4.ID, ch1.ID)
	}

	// entity_hint prepends the caller-asserted entity type — and because
	// entity_types participates in the dedupe constraint, a hinted
	// submission is a distinct channel from the unhinted one.
	ch5, created, err := svc.SubmitOneshot(ctx, entryURL, []string{"news"}, "article")
	if err != nil {
		t.Fatalf("SubmitOneshot entity_hint: %v", err)
	}

	if !created {
		t.Fatal("hinted submission: created = false, want true (distinct entity_types → distinct channel)")
	}

	if len(ch5.EntityTypes) == 0 || ch5.EntityTypes[0] != "article" {
		t.Errorf("entity_types = %v, want article first", ch5.EntityTypes)
	}

	// Invalid inputs are rejected before any row is created.
	for _, bad := range []string{"", "ftp://x", "http://127.0.0.1/internal"} {
		if _, _, err := svc.SubmitOneshot(ctx, bad, nil, ""); err == nil {
			t.Errorf("SubmitOneshot(%q): expected error", bad)
		}
	}
}
