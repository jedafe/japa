package sources

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// TestSourceCrawlable covers the crawlability predicate behind the
// Default-channel materialization (final review I1): a source needs a
// schedulable pull method, a source_url, and must not be a §106
// video_channel subscription. The enabled flag does NOT decide
// crawlability — a disabled source still gets a (disabled) Default channel.
func TestSourceCrawlable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  Source
		want bool
	}{
		{"static source", Source{SourceUrl: "https://example.com", CrawlMethod: "static", SourceType: "auto"}, true},
		{"rss source", Source{SourceUrl: "https://example.com/feed", CrawlMethod: "rss", SourceType: "auto"}, true},
		{"disabled static source", Source{SourceUrl: "https://example.com", CrawlMethod: "static", SourceType: "auto", Enabled: false}, true},
		{"manual source", Source{SourceUrl: "https://example.com", CrawlMethod: "manual", SourceType: "auto"}, false},
		{"email inbound", Source{SourceUrl: "https://example.com", CrawlMethod: "email", SourceType: "auto"}, false},
		{"webhook inbound", Source{SourceUrl: "https://example.com", CrawlMethod: "webhook", SourceType: "auto"}, false},
		{"video channel", Source{SourceUrl: "https://youtube.com/c/x", CrawlMethod: "manual", SourceType: "video_channel"}, false},
		{"missing url", Source{CrawlMethod: "static", SourceType: "auto"}, false},
		{"invalid method", Source{SourceUrl: "https://example.com", CrawlMethod: "telepathy", SourceType: "auto"}, false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sourceCrawlable(tt.src); got != tt.want {
				t.Errorf("sourceCrawlable(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestServiceCreate_DefaultChannel_LiveDB exercises the I1 fix against a
// live Postgres (JC_TEST_DATABASE_URL): an admin-created crawlable source
// gets a Default channel mirroring the 000681 backfill plus an
// intel:probe_verify enqueue; a disabled source gets a disabled channel
// that never appears in ListDueChannels and no probe enqueue; manual and
// video_channel sources get neither.
func TestServiceCreate_DefaultChannel_LiveDB(t *testing.T) {
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping live Postgres test")
	}

	ctx := context.Background()

	database, err := db.New(ctx, dsn)
	if err != nil {
		t.Skipf("test database unreachable: %v", err)
	}
	defer database.Close()

	store := NewStore(database)
	svc := NewService(store)

	var (
		probed []string
		metas  []string
	)

	svc.SetProbeQueue(func(_ context.Context, id SourceID) error {
		probed = append(probed, id.String())
		return nil
	})
	svc.SetMetadataQueue(func(_ context.Context, id SourceID) error {
		metas = append(metas, id.String())
		return nil
	})

	base := gen.CreateIntelSourceParams{
		Name:               "Default Channel Test Source",
		Domain:             "dc-test.example.com",
		SourceUrl:          "https://dc-test.example.com/feed",
		SourceType:         "static",
		SourceKind:         "publisher",
		TrustLevel:         3,
		CrawlMethod:        "static",
		CrawlFrequency:     "daily",
		CrawlFrequencyMins: 1440,
		Enabled:            true,
		Status:             "active",
	}

	// --- Active crawlable source → Default channel + verify enqueued ---
	arg := base
	arg.Categories = []string{"job", "news"}

	src, err := svc.Create(ctx, arg)
	if err != nil {
		t.Fatalf("create active source: %v", err)
	}
	defer func() { _ = svc.Delete(ctx, src.ID) }()

	channels, err := store.ListChannels(ctx, src.ID)
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("channels = %d, want exactly the Default channel", len(channels))
	}

	ch := channels[0]
	if ch.Label != "Default" || ch.EntryUrl != src.SourceUrl || ch.CrawlMethod != "static" {
		t.Errorf("default channel shape wrong: %+v", ch)
	}
	if !ch.Enabled {
		t.Error("default channel disabled for an enabled source")
	}
	if ch.Origin != "admin" {
		t.Errorf("origin = %q, want admin", ch.Origin)
	}
	if ch.Pipeline != "extract" {
		t.Errorf("pipeline = %q, want extract (job/news map default)", ch.Pipeline)
	}
	if ch.RelevanceMode != "filtered" {
		t.Errorf("relevance_mode = %q, want filtered", ch.RelevanceMode)
	}
	if !slices.Contains(ch.EntityTypes, "job") || !slices.Contains(ch.EntityTypes, "news") {
		t.Errorf("entity_types = %v, want job+news from ResolveCategoryMap", ch.EntityTypes)
	}
	if len(ch.RelevanceTopics) == 0 {
		t.Error("relevance_topics empty, want map topics")
	}

	if !slices.Contains(probed, src.ID.String()) {
		t.Errorf("probe queue not called for %s (calls: %v)", src.ID, probed)
	}
	if !slices.Contains(metas, src.ID.String()) {
		t.Errorf("metadata queue not called for %s", src.ID)
	}

	// The Default channel is schedulable.
	due, err := store.ListDueChannels(ctx, 100)
	if err != nil {
		t.Fatalf("list due channels: %v", err)
	}
	foundDue := false
	for _, d := range due {
		if d.ID == ch.ID {
			foundDue = true
		}
	}
	if !foundDue {
		t.Error("enabled Default channel not due — scheduler would never crawl the source")
	}

	// --- Disabled source → channel disabled, nothing due, no verify ---
	probed = nil
	arg2 := base
	arg2.Name = "Default Channel Disabled Source"
	arg2.SourceUrl = "https://dc-disabled.example.com/feed"
	arg2.Categories = []string{"news"}
	arg2.Enabled = false

	src2, err := svc.Create(ctx, arg2)
	if err != nil {
		t.Fatalf("create disabled source: %v", err)
	}
	defer func() { _ = svc.Delete(ctx, src2.ID) }()

	channels2, err := store.ListChannels(ctx, src2.ID)
	if err != nil {
		t.Fatalf("list channels (disabled): %v", err)
	}
	for _, c := range channels2 {
		if c.Enabled {
			t.Errorf("disabled source produced enabled channel %s", c.ID)
		}
	}
	if slices.Contains(probed, src2.ID.String()) {
		t.Error("probe queue fired for a disabled source")
	}

	// --- Manual source → no channel, no verify ---
	probed = nil
	arg3 := base
	arg3.Name = "Manual Source"
	arg3.SourceUrl = "https://manual.example.com/"
	arg3.Categories = []string{"news"}
	arg3.CrawlMethod = "manual"
	arg3.Enabled = true

	src3, err := svc.Create(ctx, arg3)
	if err != nil {
		t.Fatalf("create manual source: %v", err)
	}
	defer func() { _ = svc.Delete(ctx, src3.ID) }()

	channels3, err := store.ListChannels(ctx, src3.ID)
	if err != nil {
		t.Fatalf("list channels (manual): %v", err)
	}
	if len(channels3) != 0 {
		t.Errorf("manual source got %d channels, want none (probe materializes)", len(channels3))
	}
	if slices.Contains(probed, src3.ID.String()) {
		t.Error("probe queue fired for a manual source")
	}

	// --- Private-literal source_url → source persists, no channel ---
	arg4 := base
	arg4.Name = "SSRF Literal Source"
	arg4.SourceUrl = "http://169.254.169.254/latest/meta-data"
	arg4.Categories = []string{"news"}

	src4, err := svc.Create(ctx, arg4)
	if err != nil {
		t.Fatalf("create private-literal source: %v", err)
	}
	defer func() { _ = svc.Delete(ctx, src4.ID) }()

	channels4, err := store.ListChannels(ctx, src4.ID)
	if err != nil {
		t.Fatalf("list channels (ssrf): %v", err)
	}
	if len(channels4) != 0 {
		t.Errorf("private-literal source_url produced %d channels — C1c guard must reject", len(channels4))
	}

	// --- Default channel dedupe: re-running materialization for a source
	// whose channel already exists is a silent no-op (UNIQUE constraint) —
	// the channel count stays at 1.
	svc.ensureDefaultChannel(ctx, src)

	after, err := store.ListChannels(ctx, src.ID)
	if err != nil {
		t.Fatalf("list channels after dedupe re-run: %v", err)
	}
	if len(after) != 1 {
		t.Errorf("dedupe re-run changed channel count to %d, want 1", len(after))
	}
}
