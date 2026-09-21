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

// validProposeInput returns a fully-valid proposal; each test mutates one
// field to exercise a single rule of buildProposeParams.
func validProposeInput() ProposeInput {
	return ProposeInput{
		Name:       "Example News",
		SourceURL:  "https://example.com/feed",
		Categories: []string{"news", "job"},
		SourceKind: "publisher",
	}
}

// TestBuildProposeParams_Categories covers the intel-channels multi-category
// payload: the array persists verbatim (normalized) and the legacy scalar
// category dual-writes categories[0].
func TestBuildProposeParams_Categories(t *testing.T) {
	t.Parallel()

	arg, err := buildProposeParams(validProposeInput())
	if err != nil {
		t.Fatalf("buildProposeParams(valid) = %v, want nil", err)
	}

	if len(arg.Categories) != 2 || arg.Categories[0] != "news" || arg.Categories[1] != "job" {
		t.Errorf("categories = %v, want [news job]", arg.Categories)
	}
	if arg.Category != "news" {
		t.Errorf("category = %q, want %q (dual-write of categories[0])", arg.Category, "news")
	}
}

// TestBuildProposeParams_LegacyCategory verifies the back-compat fold: a
// request carrying only the legacy `category` field produces
// categories=[category] and the same dual-write.
func TestBuildProposeParams_LegacyCategory(t *testing.T) {
	t.Parallel()

	input := validProposeInput()
	input.Categories = nil
	input.Category = " News "

	arg, err := buildProposeParams(input)
	if err != nil {
		t.Fatalf("buildProposeParams(legacy category) = %v, want nil", err)
	}

	if len(arg.Categories) != 1 || arg.Categories[0] != "news" {
		t.Errorf("categories = %v, want [news]", arg.Categories)
	}
	if arg.Category != "news" {
		t.Errorf("category = %q, want news", arg.Category)
	}
}

// TestBuildProposeParams_RequiresCategory covers the ≥1-category rule: no
// categories and no legacy category → ErrRequired (400 at the handler).
func TestBuildProposeParams_RequiresCategory(t *testing.T) {
	t.Parallel()

	input := validProposeInput()
	input.Categories = nil
	input.Category = ""

	_, err := buildProposeParams(input)
	if !errors.Is(err, errpkg.ErrRequired) {
		t.Fatalf("buildProposeParams(no categories) = %v, want ErrRequired", err)
	}
}

// TestBuildProposeParams_CategoriesNormalized covers normalization: trim,
// lowercase, drop empties, dedupe — order preserved.
func TestBuildProposeParams_CategoriesNormalized(t *testing.T) {
	t.Parallel()

	input := validProposeInput()
	input.Categories = []string{" News ", "", "JOB", "news", "  "}

	arg, err := buildProposeParams(input)
	if err != nil {
		t.Fatalf("buildProposeParams = %v, want nil", err)
	}

	want := []string{"news", "job"}
	if len(arg.Categories) != len(want) {
		t.Fatalf("categories = %v, want %v", arg.Categories, want)
	}
	for i := range want {
		if arg.Categories[i] != want[i] {
			t.Errorf("categories[%d] = %q, want %q", i, arg.Categories[i], want[i])
		}
	}
}

// TestBuildProposeParams_SourceKind covers kind defaulting and the 10-kind
// enum validation.
func TestBuildProposeParams_SourceKind(t *testing.T) {
	t.Parallel()

	t.Run("defaults to publisher", func(t *testing.T) {
		t.Parallel()

		input := validProposeInput()
		input.SourceKind = ""

		arg, err := buildProposeParams(input)
		if err != nil {
			t.Fatalf("buildProposeParams = %v, want nil", err)
		}
		if arg.SourceKind != string(SourceKindPublisher) {
			t.Errorf("source_kind = %v, want publisher", arg.SourceKind)
		}
	})

	t.Run("accepts every kind in the enum", func(t *testing.T) {
		t.Parallel()

		for _, kind := range []string{
			"publisher", "institution", "individual", "aggregator",
			"institution_site", "personal_blog", "job_board",
			"gov_portal", "community",
		} {
			input := validProposeInput()
			input.SourceKind = kind

			arg, err := buildProposeParams(input)
			if err != nil {
				t.Fatalf("buildProposeParams(kind=%q) = %v, want nil", kind, err)
			}
			if arg.SourceKind != kind {
				t.Errorf("source_kind = %v, want %q", arg.SourceKind, kind)
			}
		}
	})

	t.Run("rejects unknown kind", func(t *testing.T) {
		t.Parallel()

		input := validProposeInput()
		input.SourceKind = "robot"

		_, err := buildProposeParams(input)
		if !errors.Is(err, errpkg.ErrInvalid) {
			t.Fatalf("buildProposeParams(kind=robot) = %v, want ErrInvalid", err)
		}
	})
}

// TestBuildProposeParams_BareDomain covers the scheme-less URL
// normalization: the probe endpoint accepts bare domains, so the submit
// step must too — "jobberman.com" normalizes to https://jobberman.com for
// both validation and the stored source_url.
func TestBuildProposeParams_BareDomain(t *testing.T) {
	t.Parallel()

	input := validProposeInput()
	input.SourceURL = "jobberman.com"

	arg, err := buildProposeParams(input)
	if err != nil {
		t.Fatalf("buildProposeParams(bare domain) = %v, want nil", err)
	}
	if arg.SourceUrl != "https://jobberman.com" {
		t.Errorf("source_url = %q, want %q", arg.SourceUrl, "https://jobberman.com")
	}
	if arg.Domain != "jobberman.com" {
		t.Errorf("domain = %q, want %q", arg.Domain, "jobberman.com")
	}
}

// TestBuildProposeParams_ManualCrawl covers the spec change: proposed rows
// are always crawl_method/crawl_frequency 'manual' because channels own
// crawling after approval — a legacy 'rss' source_type no longer maps to an
// rss crawl.
func TestBuildProposeParams_ManualCrawl(t *testing.T) {
	t.Parallel()

	input := validProposeInput()
	input.SourceType = "rss"

	arg, err := buildProposeParams(input)
	if err != nil {
		t.Fatalf("buildProposeParams = %v, want nil", err)
	}

	if arg.CrawlMethod != string(CrawlMethodManual) {
		t.Errorf("crawl_method = %q, want manual", arg.CrawlMethod)
	}
	if arg.CrawlFrequency != string(CrawlFrequencyManual) {
		t.Errorf("crawl_frequency = %q, want manual", arg.CrawlFrequency)
	}
	if arg.CrawlFrequencyMins != 0 {
		t.Errorf("crawl_frequency_mins = %d, want 0", arg.CrawlFrequencyMins)
	}
	if arg.SourceType != "auto" {
		t.Errorf("source_type = %q, want auto", arg.SourceType)
	}
	if arg.Status != "proposed" {
		t.Errorf("status = %v, want proposed", arg.Status)
	}
	if arg.Enabled {
		t.Error("enabled = true, want false for proposed rows")
	}
}

// TestBuildProposeParams_VideoChannelRouting covers the §106 video path now
// keyed on source_kind: channel_profile + a supported video platform URL →
// video_channel/manual so video:sync_channels picks it up.
func TestBuildProposeParams_VideoChannelRouting(t *testing.T) {
	t.Parallel()

	t.Run("channel_profile on youtube becomes video_channel", func(t *testing.T) {
		t.Parallel()

		input := validProposeInput()
		input.SourceKind = "channel_profile"
		input.SourceURL = "https://www.youtube.com/@example"
		input.Categories = []string{"news"}

		arg, err := buildProposeParams(input)
		if err != nil {
			t.Fatalf("buildProposeParams = %v, want nil", err)
		}
		if arg.SourceType != string(SourceTypeVideoChannel) {
			t.Errorf("source_type = %q, want video_channel", arg.SourceType)
		}
		if arg.CrawlMethod != string(CrawlMethodManual) || arg.CrawlFrequency != string(CrawlFrequencyManual) {
			t.Errorf("crawl = %q/%q, want manual/manual", arg.CrawlMethod, arg.CrawlFrequency)
		}
	})

	t.Run("channel_profile on non-video host stays a manual source", func(t *testing.T) {
		t.Parallel()

		input := validProposeInput()
		input.SourceKind = "channel_profile"
		input.SourceURL = "https://t.me/example"
		input.Categories = []string{"news"}

		arg, err := buildProposeParams(input)
		if err != nil {
			t.Fatalf("buildProposeParams = %v, want nil", err)
		}
		if arg.SourceType != "auto" {
			t.Errorf("source_type = %q, want auto", arg.SourceType)
		}
	})

	t.Run("legacy video type on youtube still routes", func(t *testing.T) {
		t.Parallel()

		input := validProposeInput()
		input.SourceKind = ""
		input.SourceType = "video"
		input.SourceURL = "https://youtube.com/@example"
		input.Categories = []string{"news"}

		arg, err := buildProposeParams(input)
		if err != nil {
			t.Fatalf("buildProposeParams = %v, want nil", err)
		}
		if arg.SourceKind != string(SourceKindChannelProfile) {
			t.Errorf("source_kind = %v, want channel_profile", arg.SourceKind)
		}
		if arg.SourceType != string(SourceTypeVideoChannel) {
			t.Errorf("source_type = %q, want video_channel", arg.SourceType)
		}
	})

	t.Run("legacy video type on non-platform host is rejected", func(t *testing.T) {
		t.Parallel()

		input := validProposeInput()
		input.SourceKind = ""
		input.SourceType = "video"
		input.SourceURL = "https://example.com/video"
		input.Categories = []string{"news"}

		_, err := buildProposeParams(input)
		if !errors.Is(err, errpkg.ErrInvalid) {
			t.Fatalf("buildProposeParams(video on example.com) = %v, want ErrInvalid", err)
		}
	})
}

// TestBuildProposeParams_ProbeSnapshot covers probe_report storage: a JSON
// object snapshot is carried verbatim; non-object payloads are rejected;
// absent snapshots leave ProbeReport nil (the query COALESCEs '{}').
func TestBuildProposeParams_ProbeSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("valid object is stored", func(t *testing.T) {
		t.Parallel()

		input := validProposeInput()
		input.ProbeSnapshot = []byte(`{"final_url":"https://example.com","channels":[{"method":"rss"}]}`)

		arg, err := buildProposeParams(input)
		if err != nil {
			t.Fatalf("buildProposeParams = %v, want nil", err)
		}
		if string(arg.ProbeReport) != string(input.ProbeSnapshot) {
			t.Errorf("probe_report = %s, want %s", arg.ProbeReport, input.ProbeSnapshot)
		}
	})

	t.Run("empty object is stored", func(t *testing.T) {
		t.Parallel()

		input := validProposeInput()
		input.ProbeSnapshot = []byte(`{}`)

		arg, err := buildProposeParams(input)
		if err != nil {
			t.Fatalf("buildProposeParams = %v, want nil", err)
		}
		if string(arg.ProbeReport) != `{}` {
			t.Errorf("probe_report = %s, want {}", arg.ProbeReport)
		}
	})

	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"array", `[{"method":"rss"}]`},
		{"string", `"rss"`},
		{"number", `42`},
		{"null", `null`},
		{"malformed", `{`},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			t.Parallel()

			input := validProposeInput()
			input.ProbeSnapshot = []byte(tc.raw)

			_, err := buildProposeParams(input)
			if !errors.Is(err, errpkg.ErrInvalid) {
				t.Fatalf("buildProposeParams(snapshot=%s) = %v, want ErrInvalid", tc.raw, err)
			}
		})
	}

	t.Run("absent leaves probe_report unset", func(t *testing.T) {
		t.Parallel()

		arg, err := buildProposeParams(validProposeInput())
		if err != nil {
			t.Fatalf("buildProposeParams = %v, want nil", err)
		}
		if arg.ProbeReport != nil {
			t.Errorf("probe_report = %s, want nil (query COALESCEs '{}')", arg.ProbeReport)
		}
	})
}

// TestPropose_ValidationBeforeStore exercises Propose's error paths with a
// nil store — every validation failure must return before the uniqueness
// lookup touches the database.
func TestPropose_ValidationBeforeStore(t *testing.T) {
	t.Parallel()

	svc := NewService(nil)
	userID := mustUUID("550e8400-e29b-41d4-a716-446655440000")

	for _, tc := range []struct {
		name  string
		input ProposeInput
		sent  error
	}{
		{"missing name", ProposeInput{SourceURL: "https://example.com", Categories: []string{"news"}}, errpkg.ErrRequired},
		{"missing url", ProposeInput{Name: "X", Categories: []string{"news"}}, errpkg.ErrRequired},
		{"invalid url", ProposeInput{Name: "X", SourceURL: "ftp://x", Categories: []string{"news"}}, errpkg.ErrInvalid},
		{"missing categories", ProposeInput{Name: "X", SourceURL: "https://example.com"}, errpkg.ErrRequired},
		{"unknown kind", ProposeInput{Name: "X", SourceURL: "https://example.com", Categories: []string{"news"}, SourceKind: "bot"}, errpkg.ErrInvalid},
		{"bad snapshot", ProposeInput{Name: "X", SourceURL: "https://example.com", Categories: []string{"news"}, ProbeSnapshot: []byte(`[1]`)}, errpkg.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := svc.Propose(t.Context(), userID, tc.input)
			if !errors.Is(err, tc.sent) {
				t.Fatalf("Propose(%s) = %v, want sentinel %v", tc.name, err, tc.sent)
			}
		})
	}
}

// TestPropose_PersistsPayloadFields exercises the full propose path against
// a live Postgres — categories array, dual-written category, source_kind,
// probe_report, manual crawl flags. Gated on JC_TEST_DATABASE_URL like the
// channel store test (this repo has no shared test-DB harness).
func TestPropose_PersistsPayloadFields(t *testing.T) {
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

	// proposed_by has an FK to users(id) — use a real user row rather than
	// a fabricated UUID.
	var userID pgtype.UUID
	if err := database.Writer.QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID); err != nil {
		t.Skipf("no users row available for proposed_by FK: %v", err)
	}

	src, err := svc.Propose(ctx, userID, ProposeInput{
		Name:          "Propose Test Source",
		SourceURL:     "https://propose-test.example.com/feed",
		Categories:    []string{"news", "job"},
		SourceKind:    "aggregator",
		ProbeSnapshot: []byte(`{"final_url":"https://propose-test.example.com/feed"}`),
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	defer func() { _ = svc.Delete(ctx, src.ID) }()

	if src.Status != "proposed" {
		t.Errorf("status = %q, want proposed", src.Status)
	}
	if len(src.Categories) != 2 || src.Categories[0] != "news" || src.Categories[1] != "job" {
		t.Errorf("categories = %v, want [news job]", src.Categories)
	}
	if src.Category != "news" {
		t.Errorf("category = %q, want news (dual-write)", src.Category)
	}
	if src.SourceKind != "aggregator" {
		t.Errorf("source_kind = %q, want aggregator", src.SourceKind)
	}
	if string(src.ProbeReport) == "" || string(src.ProbeReport) == "{}" {
		t.Errorf("probe_report = %s, want the submitted snapshot", src.ProbeReport)
	}
	if src.CrawlMethod != "manual" || src.CrawlFrequency != "manual" {
		t.Errorf("crawl = %q/%q, want manual/manual", src.CrawlMethod, src.CrawlFrequency)
	}
	if src.SourceType != "auto" {
		t.Errorf("source_type = %q, want auto", src.SourceType)
	}
}

// Compile-time check that CreateIntelSourceParams still exposes the fields
// the propose path populates (regresses if the query shape changes).
var _ = gen.CreateIntelSourceParams{Categories: []string{}, ProbeReport: []byte{}}
