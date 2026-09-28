package curator

import (
	"context"
	"errors"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

func staticChannel(entityTypes []string, providerConfig string) sources.Channel {
	return sources.Channel{
		EntryUrl:       "https://example.org/scholarships/daad-123",
		CrawlMethod:    string(sources.CrawlMethodStatic),
		EntityTypes:    entityTypes,
		ProviderConfig: []byte(providerConfig),
	}
}

func TestStaticHTML_SelelectorsPopulateRecord(t *testing.T) {
	body := []byte(`<html><head><title>Fallback title</title></head><body>
		<h1 class="program-title">DAAD Regional Scholarship</h1>
		<span class="app-deadline">2026-12-31</span>
		<p class="host">Germany</p>
	</body></html>`)

	ch := staticChannel([]string{"scholarship"}, `{"selectors": {
		"title": "h1.program-title",
		"deadline": ".app-deadline",
		"country": ".host"
	}}`)

	recs, err := (StaticHTML{method: string(sources.CrawlMethodStatic)}).Curate(context.Background(), body, "text/html", ch)
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}

	ext := recs[0].Extraction
	if got := fieldString(t, ext, "title"); got != "DAAD Regional Scholarship" {
		t.Fatalf("title = %q", got)
	}
	if got := fieldString(t, ext, "deadline"); got != "2026-12-31" {
		t.Fatalf("deadline = %q", got)
	}
	if got := fieldStrings(t, ext, "country"); len(got) != 1 || got[0] != "Germany" {
		t.Fatalf("country = %v", got)
	}
	if recs[0].EntityType != "scholarship" {
		t.Fatalf("entity type = %q", recs[0].EntityType)
	}
}

func TestStaticHTML_LdjsonNodeProducesRecord(t *testing.T) {
	body := []byte(`<html><head>
		<script type="application/ld+json">
		{"@type":"JobPosting","title":"Junior DevOps Engineer","hiringOrganization":{"name":"ACME"},
		 "validThrough":"2026-06-30","jobLocation":{"address":{"addressLocality":"Lagos","addressCountry":"Nigeria"}}}
		</script>
		</head><body><p>Some page</p></body></html>`)

	recs, err := (StaticHTML{method: "static"}).Curate(context.Background(), body, "text/html", staticChannel([]string{"job"}, "{}"))
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if recs[0].EntityType != "job" {
		t.Fatalf("entity type = %q", recs[0].EntityType)
	}
	if got := fieldString(t, recs[0].Extraction, "title"); got != "Junior DevOps Engineer" {
		t.Fatalf("title = %q", got)
	}
	if got := fieldString(t, recs[0].Extraction, "employer"); got != "ACME" {
		t.Fatalf("employer = %q", got)
	}
}

func TestStaticHTML_MetaAndHeuristicDeadline(t *testing.T) {
	body := []byte(`<html><head>
		<meta property="og:title" content="Swiss Excellence Scholarship 2026"/>
		<meta property="og:description" content="Fully funded research scholarship"/>
		<link rel="canonical" href="https://example.org/swiss-2026"/>
	</head><body><p>Applications close on December 15, 2026. Open to all fields.</p></body></html>`)

	recs, err := (StaticHTML{method: "static"}).Curate(context.Background(), body, "text/html", staticChannel([]string{"scholarship"}, "{}"))
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}

	ext := recs[0].Extraction
	if got := fieldString(t, ext, "title"); got != "Swiss Excellence Scholarship 2026" {
		t.Fatalf("title = %q", got)
	}
	if got := fieldString(t, ext, "description_summary"); got != "Fully funded research scholarship" {
		t.Fatalf("description_summary = %q", got)
	}
	if got := fieldString(t, ext, "application_url"); got != "https://example.org/swiss-2026" {
		t.Fatalf("application_url = %q", got)
	}
	if got := fieldString(t, ext, "deadline"); got != "2026-12-15" {
		t.Fatalf("deadline = %q", got)
	}
	if f := ext["deadline"]; f.Confidence != heuristicFieldConfidence {
		t.Fatalf("deadline confidence = %v, want %v", f.Confidence, heuristicFieldConfidence)
	}
}

func TestStaticHTML_ThinPageFallsBackToExtract(t *testing.T) {
	// A nav-heavy index page with no usable structure — the curator must
	// defer to the LLM extract pipeline rather than persist a title-only
	// record.
	body := []byte(`<html><head><title>Index</title></head><body><nav>Home | About</nav></body></html>`)

	_, err := (StaticHTML{method: "static"}).Curate(context.Background(), body, "text/html", staticChannel([]string{"scholarship"}, "{}"))
	if !errors.Is(err, ErrNoCurator) {
		t.Fatalf("err = %v, want ErrNoCurator", err)
	}
}

func TestStaticHTML_NonHTMLFallsBackToExtract(t *testing.T) {
	_, err := (StaticHTML{method: "static"}).Curate(context.Background(), []byte(`{"a":1}`), "application/json", staticChannel(nil, "{}"))
	if !errors.Is(err, ErrNoCurator) {
		t.Fatalf("err = %v, want ErrNoCurator", err)
	}
}

func TestStaticHTML_LdjsonFilteredByChannelEntityTypes(t *testing.T) {
	// A JobPosting node on a scholarship channel must be skipped — the
	// channel's declared entity types are the contract.
	body := []byte(`<html><head>
		<script type="application/ld+json">{"@type":"JobPosting","title":"Engineer","hiringOrganization":{"name":"X"}}</script>
		<meta property="og:title" content="Scholarship listing"/>
		<meta property="og:description" content="Scholarships for engineers"/>
	</head><body></body></html>`)

	recs, err := (StaticHTML{method: "static"}).Curate(context.Background(), body, "text/html", staticChannel([]string{"scholarship"}, "{}"))
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if recs[0].EntityType != "scholarship" {
		t.Fatalf("entity type = %q, want scholarship (job node must be skipped)", recs[0].EntityType)
	}
	if got := fieldString(t, recs[0].Extraction, "title"); got != "Scholarship listing" {
		t.Fatalf("title = %q", got)
	}
}
