package sources

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// rssProposal returns a typical verify-probe rss proposal; each test
// mutates fields to exercise one materialize rule.
func rssProposal() ChannelProposalInput {
	return ChannelProposalInput{
		Label:           "News feed",
		EntryURL:        "https://example.com/feed",
		CrawlMethod:     "rss",
		EntityTypes:     []string{"news"},
		Pipeline:        "curate",
		Confidence:      0.95,
		RelevanceTopics: []string{"migration"},
	}
}

// TestMaterializeProposal exercises the pure proposal→channel decision
// rules: channel_sync routing, unknown methods, entity-type fill/skip,
// pipeline resolution, origin/relevance stamping, and topic fallbacks.
func TestMaterializeProposal(t *testing.T) {
	t.Parallel()

	sourceID := mustUUID("22222222-2222-4222-8222-222222222222")
	resolved := ResolveCategoryMap(nil, []string{"job", "news"})

	t.Run("channel_sync is routed away", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal()
		prop.CrawlMethod = "channel_sync"

		if _, reason := materializeProposal(sourceID, prop, resolved); reason == "" {
			t.Fatal("channel_sync proposal must be skipped")
		}
	})

	t.Run("unknown method skipped", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal()
		prop.CrawlMethod = "telepathy"

		_, reason := materializeProposal(sourceID, prop, resolved)
		if !strings.Contains(reason, "telepathy") {
			t.Fatalf("skip reason = %q, want mention of the bad method", reason)
		}
	})

	t.Run("happy path stamps probe origin + filtered relevance", func(t *testing.T) {
		t.Parallel()

		arg, reason := materializeProposal(sourceID, rssProposal(), resolved)
		if reason != "" {
			t.Fatalf("skip reason = %q, want none", reason)
		}

		if arg.Origin != "probe" {
			t.Errorf("origin = %q, want probe", arg.Origin)
		}

		if arg.RelevanceMode != "filtered" {
			t.Errorf("relevance_mode = %q, want filtered", arg.RelevanceMode)
		}

		if !arg.Enabled {
			t.Error("enabled = false, want true")
		}

		if arg.SourceID != sourceID {
			t.Errorf("source_id = %v, want %v", arg.SourceID, sourceID)
		}

		if !slices.Equal(arg.RelevanceTopics, []string{"migration"}) {
			t.Errorf("relevance_topics = %v, want proposal's topics", arg.RelevanceTopics)
		}
	})

	t.Run("empty entity_types fill from nominating categories", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal()
		prop.EntityTypes = nil

		arg, reason := materializeProposal(sourceID, prop, resolved)
		if reason != "" {
			t.Fatalf("skip reason = %q, want none", reason)
		}
		// rss is nominated by 'news' alone on a [job, news] source — the
		// channel carries the nominating category's vocabulary, not the
		// source-wide union.
		if !slices.Equal(arg.EntityTypes, []string{"news"}) {
			t.Errorf("entity_types = %v, want [news]", arg.EntityTypes)
		}
	})

	t.Run("unfillable entity_types skip record pipelines", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal() // pipeline=curate requires entity types
		prop.EntityTypes = nil

		empty := ResolveCategoryMap(nil, nil)

		if _, reason := materializeProposal(sourceID, prop, empty); reason == "" {
			t.Fatal("extract/curate proposal with no entity types must be skipped")
		}
	})

	t.Run("empty pipeline resolves from nominating category", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal()
		prop.Pipeline = ""

		arg, _ := materializeProposal(sourceID, prop, resolved)
		// rss is nominated by 'news' (curate) — the merged source-level
		// extract pipeline ('job' forces it) does NOT leak onto a channel
		// no extract category nominated.
		if arg.Pipeline != "curate" {
			t.Errorf("pipeline = %q, want curate (news nominates rss)", arg.Pipeline)
		}
	})

	t.Run("method nominated by an extract category extracts", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal()
		prop.CrawlMethod = "api" // nominated by job (extract) on [job, news]
		prop.EntityTypes = nil
		prop.Pipeline = ""

		arg, _ := materializeProposal(sourceID, prop, resolved)
		if arg.Pipeline != "extract" {
			t.Errorf("pipeline = %q, want extract (job nominates api)", arg.Pipeline)
		}

		if !slices.Equal(arg.EntityTypes, []string{"job"}) {
			t.Errorf("entity_types = %v, want [job]", arg.EntityTypes)
		}
	})

	t.Run("method nominated by both categories merges", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal()
		prop.CrawlMethod = "jsonld" // nominated by job AND news on [job, news]
		prop.EntityTypes = nil
		prop.Pipeline = ""

		arg, _ := materializeProposal(sourceID, prop, resolved)
		if arg.Pipeline != "extract" {
			t.Errorf("pipeline = %q, want extract (any extract nominator wins)", arg.Pipeline)
		}

		if !slices.Equal(arg.EntityTypes, []string{"job", "news"}) {
			t.Errorf("entity_types = %v, want [job news]", arg.EntityTypes)
		}
	})

	t.Run("platform method uses method default pipeline + no topics", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal()
		prop.CrawlMethod = "social"
		prop.Pipeline = ""
		prop.RelevanceTopics = nil
		prop.EntryURL = "https://www.reddit.com/r/nigeria/.json"

		arg, _ := materializeProposal(sourceID, prop, resolved)
		if arg.Pipeline != "curate" {
			t.Errorf("pipeline = %q, want curate (method default, not resolved extract)", arg.Pipeline)
		}

		if len(arg.RelevanceTopics) != 0 {
			t.Errorf("relevance_topics = %v, want empty for platform method", arg.RelevanceTopics)
		}
	})

	t.Run("category method falls back to resolved topics", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal()
		prop.RelevanceTopics = nil

		arg, _ := materializeProposal(sourceID, prop, resolved)
		if !slices.Equal(arg.RelevanceTopics, resolved.Topics) {
			t.Errorf("relevance_topics = %v, want resolved %v", arg.RelevanceTopics, resolved.Topics)
		}
	})

	t.Run("empty label falls back to method name", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal()
		prop.Label = ""

		arg, _ := materializeProposal(sourceID, prop, resolved)
		if arg.Label != "rss channel" {
			t.Errorf("label = %q, want \"rss channel\"", arg.Label)
		}
	})

	t.Run("inbound webhook created enabled", func(t *testing.T) {
		t.Parallel()

		prop := rssProposal()
		prop.CrawlMethod = "webhook"
		prop.EntryURL = "https://example.com/topic"
		prop.Pipeline = "curate"

		arg, reason := materializeProposal(sourceID, prop, resolved)
		if reason != "" {
			t.Fatalf("skip reason = %q, want none", reason)
		}

		if !arg.Enabled {
			t.Error("inbound channel must be created enabled (never scheduled)")
		}
	})
}

// TestStampProbeConfidence verifies the confidence merge into
// provider_config preserves existing keys and tolerates bad input.
func TestStampProbeConfidence(t *testing.T) {
	t.Parallel()

	got := stampProbeConfidence([]byte(`{"hub":"https://hub.example"}`), 0.8)

	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("stamped config unparseable: %v", err)
	}

	if m["hub"] != "https://hub.example" {
		t.Errorf("existing key lost: %v", m)
	}

	if m["probe_confidence"] != 0.8 {
		t.Errorf("probe_confidence = %v, want 0.8", m["probe_confidence"])
	}

	if out := stampProbeConfidence(nil, 0.5); !strings.Contains(string(out), `"probe_confidence":0.5`) {
		t.Errorf("empty config stamped = %s", out)
	}

	bad := []byte(`[not json`)
	if out := stampProbeConfidence(bad, 0.5); string(out) != string(bad) {
		t.Errorf("unparseable config mutated: %s", out)
	}
}

// TestMaterializeChannels_Dedupe exercises the service method against a
// live Postgres: the first run creates rows, an identical re-run skips
// them all via the UNIQUE(source_id, entry_url, entity_types) constraint.
// Gated on JC_TEST_DATABASE_URL like the channel-store test.
func TestMaterializeChannels_Dedupe(t *testing.T) {
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

	src, err := store.Create(ctx, gen.CreateIntelSourceParams{
		Name:               "Materialize Test Source",
		Domain:             "example.com",
		SourceUrl:          "https://example.com",
		Category:           "news",
		Categories:         []string{"news"},
		SourceType:         "auto",
		SourceKind:         "publisher",
		TrustLevel:         3,
		CrawlMethod:        "manual",
		CrawlFrequency:     "manual",
		CrawlFrequencyMins: 0,
		RobotsPolicy:       []byte("{}"),
		ProviderConfig:     []byte("{}"),
		Enabled:            true,
		Status:             "active",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	defer func() { _ = store.Delete(ctx, src.ID) }()

	proposals := []ChannelProposalInput{
		{
			Label:       "News feed",
			EntryURL:    "https://example.com/feed",
			CrawlMethod: "rss",
			EntityTypes: []string{"news"},
			Pipeline:    "curate",
			Confidence:  0.95,
		},
		{
			Label:       "Jobs JSON-LD",
			EntryURL:    "https://example.com/jobs",
			CrawlMethod: "jsonld",
			EntityTypes: []string{"job"},
			Pipeline:    "extract",
			Confidence:  0.9,
		},
		{
			// channel_sync never materializes.
			Label:       "YouTube channel",
			EntryURL:    "https://youtube.com/@x",
			CrawlMethod: "channel_sync",
			Pipeline:    "curate",
		},
	}

	created, skipped, err := svc.MaterializeChannels(ctx, src.ID, proposals)
	if err != nil {
		t.Fatalf("MaterializeChannels: %v", err)
	}

	if created != 2 || skipped != 1 {
		t.Fatalf("created=%d skipped=%d, want 2/1", created, skipped)
	}

	channels, err := store.ListChannels(ctx, src.ID)
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}

	if len(channels) != 2 {
		t.Fatalf("channels = %d, want 2", len(channels))
	}

	for _, ch := range channels {
		if ch.Origin != "probe" {
			t.Errorf("channel %s origin = %q, want probe", ch.Label, ch.Origin)
		}

		if ch.RelevanceMode != "filtered" {
			t.Errorf("channel %s relevance_mode = %q, want filtered", ch.Label, ch.RelevanceMode)
		}
	}

	// Re-run is idempotent: every proposal collides with the UNIQUE
	// constraint and counts as skipped.
	created, skipped, err = svc.MaterializeChannels(ctx, src.ID, proposals)
	if err != nil {
		t.Fatalf("MaterializeChannels re-run: %v", err)
	}

	if created != 0 || skipped != 3 {
		t.Fatalf("re-run created=%d skipped=%d, want 0/3", created, skipped)
	}
}
