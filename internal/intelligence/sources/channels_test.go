package sources

import (
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// mustUUID parses a literal UUID into pgtype.UUID for test fixtures.
func mustUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		panic(err)
	}
	return u
}

// TestRelevanceMode_IsValid covers the relevance_mode CHECK set from
// migration 000681 (open|filtered|custom).
func TestRelevanceMode_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode RelevanceMode
		want bool
	}{
		{RelevanceOpen, true},
		{RelevanceFiltered, true},
		{RelevanceCustom, true},
		{RelevanceMode(""), false},
		{RelevanceMode("strict"), false},
		{RelevanceMode("FILTERED"), false},
	}

	for _, tt := range tests {
		if got := tt.mode.IsValid(); got != tt.want {
			t.Errorf("RelevanceMode(%q).IsValid() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

// TestPipeline_IsValid covers the pipeline CHECK set (extract|curate|hybrid).
func TestPipeline_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pipeline Pipeline
		want     bool
	}{
		{PipelineExtract, true},
		{PipelineCurate, true},
		{PipelineHybrid, true},
		{Pipeline(""), false},
		{Pipeline("llm"), false},
	}

	for _, tt := range tests {
		if got := tt.pipeline.IsValid(); got != tt.want {
			t.Errorf("Pipeline(%q).IsValid() = %v, want %v", tt.pipeline, got, tt.want)
		}
	}
}

// TestCrawlMethod_IsValid covers the full adapter set: the pre-existing
// methods plus the intel-channels phase-1 additions, including the two
// inbound methods.
func TestCrawlMethod_IsValid(t *testing.T) {
	t.Parallel()

	known := []CrawlMethod{
		CrawlMethodStatic, CrawlMethodRSS, CrawlMethodAPI, CrawlMethodPDF,
		CrawlMethodOCR, CrawlMethodManual,
		CrawlMethodRender, CrawlMethodJSONLD, CrawlMethodTabular,
		CrawlMethodSitemap, CrawlMethodICal, CrawlMethodGit, CrawlMethodSocial,
		CrawlMethodSearch, CrawlMethodWayback, CrawlMethodXHRSniff,
		CrawlMethodOneshot, CrawlMethodEmail, CrawlMethodWebhook,
	}
	for _, m := range known {
		if !m.IsValid() {
			t.Errorf("CrawlMethod(%q).IsValid() = false, want true", m)
		}
	}

	for _, m := range []CrawlMethod{"", "ftp", "screenshot", CrawlMethod("RSS")} {
		if m.IsValid() {
			t.Errorf("CrawlMethod(%q).IsValid() = true, want false", m)
		}
	}
}

// TestInboundMethod verifies that email/webhook are push methods excluded
// from scheduling, while every pull method reports false.
func TestInboundMethod(t *testing.T) {
	t.Parallel()

	for _, m := range []CrawlMethod{CrawlMethodEmail, CrawlMethodWebhook} {
		if !InboundMethod(m) {
			t.Errorf("InboundMethod(%q) = false, want true", m)
		}
	}
	for _, m := range []CrawlMethod{
		CrawlMethodStatic, CrawlMethodRSS, CrawlMethodAPI, CrawlMethodRender,
		CrawlMethodManual, CrawlMethodOneshot, CrawlMethod(""),
	} {
		if InboundMethod(m) {
			t.Errorf("InboundMethod(%q) = true, want false", m)
		}
	}
}

// TestSourceKindConstants covers the expanded source_kind CHECK set from
// migration 000681.
func TestSourceKindConstants(t *testing.T) {
	t.Parallel()

	cases := map[SourceKind]string{
		SourceKindPublisher:       "publisher",
		SourceKindInstitution:     "institution",
		SourceKindIndividual:      "individual",
		SourceKindAggregator:      "aggregator",
		SourceKindChannelProfile:  "channel_profile",
		SourceKindInstitutionSite: "institution_site",
		SourceKindPersonalBlog:    "personal_blog",
		SourceKindJobBoard:        "job_board",
		SourceKindGovPortal:       "gov_portal",
		SourceKindCommunity:       "community",
	}
	for k, want := range cases {
		if string(k) != want {
			t.Errorf("got %q, want %q", k, want)
		}
		if !k.IsValid() {
			t.Errorf("SourceKind(%q).IsValid() = false, want true", k)
		}
	}
	if SourceKind("bot").IsValid() {
		t.Error("SourceKind(bot).IsValid() = true, want false")
	}
}

// validChannelParams returns a fully-populated, valid create params struct;
// each test case mutates one field to exercise a single validation rule.
func validChannelParams() gen.CreateIntelSourceChannelParams {
	return gen.CreateIntelSourceChannelParams{
		SourceID:           mustUUID("11111111-1111-4111-8111-111111111111"),
		Label:              "Jobs feed",
		EntryUrl:           "https://example.com/jobs",
		CrawlMethod:        string(CrawlMethodJSONLD),
		EntityTypes:        []string{"job"},
		Pipeline:           string(PipelineExtract),
		ProviderConfig:     []byte("{}"),
		CrawlFrequencyMins: 1440,
		Enabled:            true,
		Origin:             string(ChannelOriginAdmin),
		RelevanceMode:      string(RelevanceFiltered),
	}
}

// TestValidateChannelCreate exercises the domain rules CreateChannel applies
// before delegating to the store.
func TestValidateChannelCreate(t *testing.T) {
	t.Parallel()

	t.Run("valid params pass", func(t *testing.T) {
		t.Parallel()
		if err := validateChannelCreate(validChannelParams()); err != nil {
			t.Fatalf("validateChannelCreate(valid) = %v, want nil", err)
		}
	})

	t.Run("inbound channel accepts non-http entry", func(t *testing.T) {
		t.Parallel()
		arg := validChannelParams()
		arg.CrawlMethod = string(CrawlMethodEmail)
		arg.EntryUrl = "mailto:intake@example.com"
		if err := validateChannelCreate(arg); err != nil {
			t.Fatalf("validateChannelCreate(email channel) = %v, want nil", err)
		}
	})

	cases := []struct {
		name   string
		mutate func(*gen.CreateIntelSourceChannelParams)
		sent   error
	}{
		{"missing source_id", func(a *gen.CreateIntelSourceChannelParams) { a.SourceID = ChannelID{} }, errpkg.ErrRequired},
		{"missing label", func(a *gen.CreateIntelSourceChannelParams) { a.Label = "" }, errpkg.ErrRequired},
		{"missing entry_url", func(a *gen.CreateIntelSourceChannelParams) { a.EntryUrl = "" }, errpkg.ErrRequired},
		{"non-http entry_url", func(a *gen.CreateIntelSourceChannelParams) { a.EntryUrl = "ftp://example.com/x" }, errpkg.ErrInvalid},
		{"hostless entry_url", func(a *gen.CreateIntelSourceChannelParams) { a.EntryUrl = "https://" }, errpkg.ErrInvalid},
		{"unknown crawl_method", func(a *gen.CreateIntelSourceChannelParams) { a.CrawlMethod = "telepathy" }, errpkg.ErrInvalid},
		{"unknown pipeline", func(a *gen.CreateIntelSourceChannelParams) { a.Pipeline = "llm" }, errpkg.ErrInvalid},
		{"extract without entity_types", func(a *gen.CreateIntelSourceChannelParams) { a.EntityTypes = nil }, errpkg.ErrInvalid},
		{"curate without entity_types", func(a *gen.CreateIntelSourceChannelParams) {
			a.Pipeline = string(PipelineCurate)
			a.EntityTypes = nil
		}, errpkg.ErrInvalid},
		{"unknown relevance_mode", func(a *gen.CreateIntelSourceChannelParams) { a.RelevanceMode = "strict" }, errpkg.ErrInvalid},
		{"unknown origin", func(a *gen.CreateIntelSourceChannelParams) { a.Origin = "robot" }, errpkg.ErrInvalid},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			arg := validChannelParams()
			tt.mutate(&arg)

			err := validateChannelCreate(arg)
			if err == nil {
				t.Fatalf("validateChannelCreate(%s) = nil, want %v", tt.name, tt.sent)
			}
			if !errors.Is(err, tt.sent) {
				t.Fatalf("validateChannelCreate(%s) error = %v, want sentinel %v", tt.name, err, tt.sent)
			}
		})
	}
}

// TestValidateChannelUpdate exercises the per-field rules UpdateChannel
// applies to COALESCE params: only non-nil fields are checked.
func TestValidateChannelUpdate(t *testing.T) {
	t.Parallel()

	strPtr := func(s string) *string { return &s }

	t.Run("empty patch passes", func(t *testing.T) {
		t.Parallel()
		arg := gen.UpdateIntelSourceChannelParams{ID: mustUUID("11111111-1111-4111-8111-111111111111")}
		if err := validateChannelUpdate(arg); err != nil {
			t.Fatalf("validateChannelUpdate(empty patch) = %v, want nil", err)
		}
	})

	cases := []struct {
		name   string
		mutate func(*gen.UpdateIntelSourceChannelParams)
	}{
		{"blank label", func(a *gen.UpdateIntelSourceChannelParams) { a.Label = strPtr("  ") }},
		{"non-http entry_url", func(a *gen.UpdateIntelSourceChannelParams) { a.EntryUrl = strPtr("ftp://x") }},
		{"unknown crawl_method", func(a *gen.UpdateIntelSourceChannelParams) { a.CrawlMethod = strPtr("nope") }},
		{"unknown pipeline", func(a *gen.UpdateIntelSourceChannelParams) { a.Pipeline = strPtr("nope") }},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			arg := gen.UpdateIntelSourceChannelParams{ID: mustUUID("11111111-1111-4111-8111-111111111111")}
			tt.mutate(&arg)

			if err := validateChannelUpdate(arg); err == nil {
				t.Fatalf("validateChannelUpdate(%s) = nil, want error", tt.name)
			}
		})
	}

	t.Run("valid fields pass and are trimmed", func(t *testing.T) {
		t.Parallel()
		arg := gen.UpdateIntelSourceChannelParams{
			ID:          mustUUID("11111111-1111-4111-8111-111111111111"),
			Label:       strPtr("  New label  "),
			EntryUrl:    strPtr("https://example.com/feed"),
			CrawlMethod: strPtr(string(CrawlMethodSitemap)),
			Pipeline:    strPtr(string(PipelineCurate)),
		}
		if err := validateChannelUpdate(arg); err != nil {
			t.Fatalf("validateChannelUpdate(valid patch) = %v, want nil", err)
		}
		if *arg.Label != "New label" {
			t.Errorf("label not trimmed: %q", *arg.Label)
		}
	})
}

// TestChannelStore_CreateAndList exercises the channel store against a live
// Postgres. This repo has no shared test-DB harness, so the test is gated on
// JC_TEST_DATABASE_URL and skips when unset or unreachable.
func TestChannelStore_CreateAndList(t *testing.T) {
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

	src, err := store.Create(ctx, gen.CreateIntelSourceParams{
		Name:               "Channel Test Source",
		Domain:             "example.com",
		SourceUrl:          "https://example.com",
		Category:           "job",
		SourceType:         "static",
		SourceKind:         "publisher",
		TrustLevel:         3,
		CrawlMethod:        "static",
		CrawlFrequency:     "daily",
		CrawlFrequencyMins: 1440,
		RobotsPolicy:       []byte("{}"),
		ProviderConfig:     []byte("{}"),
		Enabled:            true,
		Status:             "active",
	})
	if err != nil {
		t.Fatalf("create parent source: %v", err)
	}
	defer func() { _ = store.Delete(ctx, src.ID) }()

	ch, err := store.CreateChannel(ctx, gen.CreateIntelSourceChannelParams{
		SourceID:           src.ID,
		Label:              "Jobs JSON-LD",
		EntryUrl:           "https://example.com/jobs",
		CrawlMethod:        string(CrawlMethodJSONLD),
		EntityTypes:        []string{"job"},
		Pipeline:           string(PipelineExtract),
		ProviderConfig:     []byte("{}"),
		CrawlFrequencyMins: 60,
		Enabled:            true,
		Origin:             string(ChannelOriginAdmin),
		RelevanceMode:      string(RelevanceFiltered),
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if ch.SourceID != src.ID {
		t.Errorf("channel source_id = %v, want %v", ch.SourceID, src.ID)
	}

	got, err := store.GetChannel(ctx, ch.ID)
	if err != nil {
		t.Fatalf("get channel: %v", err)
	}
	if got.Label != "Jobs JSON-LD" {
		t.Errorf("channel label = %q, want %q", got.Label, "Jobs JSON-LD")
	}

	channels, err := store.ListChannels(ctx, src.ID)
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	found := false
	for _, c := range channels {
		if c.ID == ch.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("ListChannels(%v) did not return created channel %v", src.ID, ch.ID)
	}

	failMsg := "boom"
	if err := store.UpdateChannelCrawlStatus(ctx, gen.UpdateIntelChannelCrawlStatusParams{
		ID:               ch.ID,
		Success:          false,
		LastErrorMessage: &failMsg,
	}); err != nil {
		t.Fatalf("update channel crawl status: %v", err)
	}

	got, err = store.GetChannel(ctx, ch.ID)
	if err != nil {
		t.Fatalf("get channel after status update: %v", err)
	}
	if got.ConsecutiveFailures != 1 {
		t.Errorf("consecutive_failures = %d, want 1", got.ConsecutiveFailures)
	}
	if got.LastErrorMessage == nil || *got.LastErrorMessage != "boom" {
		t.Errorf("last_error_message = %v, want %q", got.LastErrorMessage, "boom")
	}

	if err := store.DeleteChannel(ctx, ch.ID); err != nil {
		t.Fatalf("delete channel: %v", err)
	}
}
