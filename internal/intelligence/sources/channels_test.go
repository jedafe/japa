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

		if err := validateChannelCreate(validChannelParams(), ""); err != nil {
			t.Fatalf("validateChannelCreate(valid) = %v, want nil", err)
		}
	})

	t.Run("inbound channel accepts non-http entry", func(t *testing.T) {
		t.Parallel()

		arg := validChannelParams()
		arg.CrawlMethod = string(CrawlMethodEmail)
		arg.EntryUrl = "mailto:intake@example.com"

		if err := validateChannelCreate(arg, ""); err != nil {
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
		// SSRF (final review C1c): pull-channel entry URLs must not point at
		// private/reserved IP literals — a scheduled crawl would fetch them
		// and persist the body into intel_raw_documents.
		{"loopback literal entry_url", func(a *gen.CreateIntelSourceChannelParams) { a.EntryUrl = "http://127.0.0.1:8080/x" }, errpkg.ErrInvalid},
		{"metadata literal entry_url", func(a *gen.CreateIntelSourceChannelParams) { a.EntryUrl = "http://169.254.169.254/latest/meta-data" }, errpkg.ErrInvalid},
		{"rfc1918 literal entry_url", func(a *gen.CreateIntelSourceChannelParams) { a.EntryUrl = "http://10.0.0.5/internal" }, errpkg.ErrInvalid},
		{"cgnat literal entry_url", func(a *gen.CreateIntelSourceChannelParams) { a.EntryUrl = "http://100.64.1.2/x" }, errpkg.ErrInvalid},
		{"ipv6 loopback literal entry_url", func(a *gen.CreateIntelSourceChannelParams) { a.EntryUrl = "http://[::1]/x" }, errpkg.ErrInvalid},
		{"ipv6 ula literal entry_url", func(a *gen.CreateIntelSourceChannelParams) { a.EntryUrl = "http://[fd00::7]/x" }, errpkg.ErrInvalid},
		{"ipv4-mapped ipv6 literal entry_url", func(a *gen.CreateIntelSourceChannelParams) { a.EntryUrl = "http://[::ffff:10.0.0.1]/x" }, errpkg.ErrInvalid},
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

			err := validateChannelCreate(arg, "")
			if err == nil {
				t.Fatalf("validateChannelCreate(%s) = nil, want %v", tt.name, tt.sent)
			}

			if !errors.Is(err, tt.sent) {
				t.Fatalf("validateChannelCreate(%s) error = %v, want sentinel %v", tt.name, err, tt.sent)
			}
		})
	}

	t.Run("public literal and hostname entry_urls pass", func(t *testing.T) {
		t.Parallel()

		for _, u := range []string{"http://8.8.8.8/feed", "https://example.com/feed", "https://internal.corp.example/feed"} {
			arg := validChannelParams()
			arg.EntryUrl = u

			if err := validateChannelCreate(arg, ""); err != nil {
				t.Errorf("validateChannelCreate(%s) = %v, want nil (literal check must not reject names)", u, err)
			}
		}
	})
}

// TestValidateChannelUpdate exercises the per-field rules UpdateChannel
// applies to COALESCE params: only non-nil fields are checked.
func TestValidateChannelUpdate(t *testing.T) {
	t.Parallel()

	strPtr := func(s string) *string { return &s }

	t.Run("empty patch passes", func(t *testing.T) {
		t.Parallel()

		arg := gen.UpdateIntelSourceChannelParams{ID: mustUUID("11111111-1111-4111-8111-111111111111")}
		if err := validateChannelUpdate(arg, CrawlMethodRSS); err != nil {
			t.Fatalf("validateChannelUpdate(empty patch) = %v, want nil", err)
		}
	})

	cases := []struct {
		name   string
		mutate func(*gen.UpdateIntelSourceChannelParams)
	}{
		{"blank label", func(a *gen.UpdateIntelSourceChannelParams) { a.Label = strPtr("  ") }},
		{"non-http entry_url", func(a *gen.UpdateIntelSourceChannelParams) { a.EntryUrl = strPtr("ftp://x") }},
		{"private literal entry_url", func(a *gen.UpdateIntelSourceChannelParams) { a.EntryUrl = strPtr("http://169.254.169.254/x") }},
		{"loopback literal entry_url", func(a *gen.UpdateIntelSourceChannelParams) { a.EntryUrl = strPtr("http://127.0.0.1/x") }},
		{"unknown crawl_method", func(a *gen.UpdateIntelSourceChannelParams) { a.CrawlMethod = strPtr("nope") }},
		{"unknown pipeline", func(a *gen.UpdateIntelSourceChannelParams) { a.Pipeline = strPtr("nope") }},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			arg := gen.UpdateIntelSourceChannelParams{ID: mustUUID("11111111-1111-4111-8111-111111111111")}
			tt.mutate(&arg)

			if err := validateChannelUpdate(arg, CrawlMethodRSS); err == nil {
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
		if err := validateChannelUpdate(arg, CrawlMethodRSS); err != nil {
			t.Fatalf("validateChannelUpdate(valid patch) = %v, want nil", err)
		}

		if *arg.Label != "New label" {
			t.Errorf("label not trimmed: %q", *arg.Label)
		}
	})

	t.Run("inbound channel accepts non-URL entry_url", func(t *testing.T) {
		t.Parallel()
		// Email/webhook channels are created with non-URL entry points
		// (mailto:, provider address); a PATCH touching entry_url must not
		// strand them behind the pull-method http(s) rule.
		arg := gen.UpdateIntelSourceChannelParams{
			ID:       mustUUID("11111111-1111-4111-8111-111111111111"),
			EntryUrl: strPtr("mailto:intel@inbound.japa.example"),
		}
		if err := validateChannelUpdate(arg, CrawlMethodEmail); err != nil {
			t.Fatalf("validateChannelUpdate(inbound entry_url) = %v, want nil", err)
		}
	})

	t.Run("repurpose to pull re-checks entry_url strictly", func(t *testing.T) {
		t.Parallel()
		// Same patch switches email → rss: the relaxed inbound rule must
		// not carry the non-URL entry point into a fetchable method.
		arg := gen.UpdateIntelSourceChannelParams{
			ID:          mustUUID("11111111-1111-4111-8111-111111111111"),
			EntryUrl:    strPtr("mailto:intel@inbound.japa.example"),
			CrawlMethod: strPtr(string(CrawlMethodRSS)),
		}
		if err := validateChannelUpdate(arg, CrawlMethodEmail); err == nil {
			t.Fatal("validateChannelUpdate(repurposed entry_url) = nil, want error")
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

// TestChannelStore_PurgeInboundEvents exercises the retention sweep query
// against a live Postgres — gated on JC_TEST_DATABASE_URL like
// TestChannelStore_CreateAndList.
func TestChannelStore_PurgeInboundEvents(t *testing.T) {
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
		Name:               "Purge Test Source",
		Domain:             "example.com",
		SourceUrl:          "https://example.com",
		Category:           "news",
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
		Label:              "Inbound Email",
		EntryUrl:           "intel+purge-test@inbound.example",
		CrawlMethod:        string(CrawlMethodEmail),
		Pipeline:           string(PipelineCurate),
		ProviderConfig:     []byte("{}"),
		CrawlFrequencyMins: 1440,
		Enabled:            true,
		Origin:             string(ChannelOriginAdmin),
		RelevanceMode:      string(RelevanceFiltered),
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	defer func() {
		_, _ = database.Writer.Exec(ctx, "DELETE FROM intel_inbound_events WHERE channel_id=$1", ch.ID)
	}()

	mkEvent := func(hash string) gen.IntelInboundEvent {
		ev, err := store.CreateInboundEvent(ctx, gen.CreateIntelInboundEventParams{
			ChannelID:   ch.ID,
			Payload:     []byte("test delivery"),
			ContentHash: hash,
		})
		if err != nil {
			t.Fatalf("create inbound event: %v", err)
		}

		return ev
	}

	old := mkEvent("purge-old")
	fresh := mkEvent("purge-fresh")

	// Backdate one event past the 30-day retention window.
	if _, err := database.Writer.Exec(ctx,
		"UPDATE intel_inbound_events SET received_at = now() - interval '40 days' WHERE id=$1",
		old.ID); err != nil {
		t.Fatalf("backdate event: %v", err)
	}

	deleted, err := store.PurgeInboundEvents(ctx, 30)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}

	if deleted < 1 {
		t.Fatalf("purge deleted %d rows, want >= 1", deleted)
	}

	if _, err := store.GetInboundEvent(ctx, old.ID); err == nil {
		t.Error("old event still present after purge")
	}

	if _, err := store.GetInboundEvent(ctx, fresh.ID); err != nil {
		t.Errorf("fresh event purged: %v", err)
	}

	// retention_days <= 0 is the cataloged keep-forever sentinel — nothing
	// may be deleted even for the backdated row.
	if _, err := database.Writer.Exec(ctx,
		"UPDATE intel_inbound_events SET received_at = now() - interval '40 days' WHERE id=$1",
		fresh.ID); err != nil {
		t.Fatalf("re-backdate event: %v", err)
	}

	if deleted, err := store.PurgeInboundEvents(ctx, 0); err != nil || deleted != 0 {
		t.Errorf("purge(0) deleted %d rows (err=%v), want 0", deleted, err)
	}
}
