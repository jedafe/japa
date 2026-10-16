package curator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	return b
}

// fieldValue returns the extracted field's value, failing the test when the
// field is absent or null.
func fieldValue(t *testing.T, ext extractor.Extraction, name string) any {
	t.Helper()

	f, ok := ext[name]
	if !ok || f.Value == nil {
		t.Fatalf("extraction field %q missing or null: %v", name, ext)
	}

	return f.Value
}

func fieldString(t *testing.T, ext extractor.Extraction, name string) string {
	t.Helper()

	s, ok := fieldValue(t, ext, name).(string)
	if !ok {
		t.Fatalf("extraction field %q is %T, want string", name, fieldValue(t, ext, name))
	}

	return s
}

// fieldStrings returns the extracted field's value as a string slice —
// Validate's array coercers emit []string, so tolerate both shapes.
func fieldStrings(t *testing.T, ext extractor.Extraction, name string) []string {
	t.Helper()

	v := fieldValue(t, ext, name)

	switch arr := v.(type) {
	case []string:
		return arr
	case []any:
		out := make([]string, 0, len(arr))

		for _, el := range arr {
			s, ok := el.(string)
			if !ok {
				t.Fatalf("field %q element is %T, want string", name, el)
			}

			out = append(out, s)
		}

		return out
	}

	t.Fatalf("field %q is %T, want string array", name, v)

	return nil
}

// fieldConfidences asserts every populated field carries confidence 1.0 —
// the deterministic-curation contract.
func fieldConfidences(t *testing.T, ext extractor.Extraction) {
	t.Helper()

	for name, f := range ext {
		if f.Value != nil && f.Confidence != 1.0 {
			t.Errorf("field %q confidence = %v, want 1.0", name, f.Confidence)
		}
	}
}

func rssChannel(entityTypes ...string) sources.Channel {
	return sources.Channel{
		CrawlMethod: "rss",
		EntityTypes: entityTypes,
		EntryUrl:    "https://news.example.com/feed",
	}
}

// TestRSSNewsCurate covers the RSS fixture: three items → three records with
// guid→link→hash external-key precedence, channel-default entity type, and
// schema-validated fields at confidence 1.0.
func TestRSSNewsCurate(t *testing.T) {
	t.Parallel()

	body := fixture(t, "feed.rss")
	ch := rssChannel(schemas.EntityTypeNews)

	recs, err := RSSNews{}.Curate(t.Context(), body, "application/rss+xml", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if len(recs) != 3 {
		t.Fatalf("records = %d, want 3", len(recs))
	}

	// External key precedence: guid → link → content hash.
	if got := recs[0].ExternalKey; got != "https://news.example.com/canada-express-entry-draw" {
		t.Errorf("item 1 external key = %q, want guid", got)
	}

	if got := recs[1].ExternalKey; got != "https://news.example.com/uk-student-funds" {
		t.Errorf("item 2 external key = %q, want link fallback", got)
	}

	if got := recs[2].ExternalKey; len(got) < 5 || got[:5] != "hash:" {
		t.Errorf("item 3 external key = %q, want hash: fallback", got)
	}

	for i, rec := range recs {
		if rec.EntityType != schemas.EntityTypeNews {
			t.Errorf("record %d entity type = %q, want news", i, rec.EntityType)
		}

		fieldConfidences(t, rec.Extraction)
	}

	first := recs[0].Extraction
	if got := fieldString(t, first, "title"); got != "Canada opens new Express Entry draw" {
		t.Errorf("title = %q", got)
	}

	if got := fieldString(t, first, "published_date"); got != "2025-09-15" {
		t.Errorf("published_date = %q, want 2025-09-15", got)
	}

	if got := fieldString(t, first, "article_url"); got != "https://news.example.com/canada-express-entry-draw" {
		t.Errorf("article_url = %q", got)
	}

	if got := fieldString(t, first, "author"); got != "Ada Obi" {
		t.Errorf("author = %q, want dc:creator name", got)
	}

	if topics := fieldStrings(t, first, "topics"); len(topics) != 2 {
		t.Errorf("topics = %v, want 2 categories", topics)
	}

	// Item 2: "email (Name)" author → display name.
	if got := fieldString(t, recs[1].Extraction, "author"); got != "Tunde Bakare" {
		t.Errorf("item 2 author = %q, want display name", got)
	}
}

// TestRSSNewsCurate_ArticleChannel verifies the same feed on a blog channel
// emits article records and keeps the content:encoded body (the article
// schema has a body field; the news schema does not).
func TestRSSNewsCurate_ArticleChannel(t *testing.T) {
	t.Parallel()

	body := fixture(t, "feed.rss")
	ch := rssChannel(schemas.EntityTypeArticle)

	recs, err := RSSNews{}.Curate(t.Context(), body, "application/rss+xml", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if len(recs) != 3 {
		t.Fatalf("records = %d, want 3", len(recs))
	}

	if recs[0].EntityType != schemas.EntityTypeArticle {
		t.Fatalf("entity type = %q, want article", recs[0].EntityType)
	}

	if got := fieldString(t, recs[0].Extraction, "body"); got == "" {
		t.Error("body empty — content:encoded should map onto the article schema")
	}
}

// TestRSSNewsCurate_Atom covers the Atom <feed>/<entry> shape.
func TestRSSNewsCurate_Atom(t *testing.T) {
	t.Parallel()

	atom := `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Atom</title>
  <entry>
    <title>Atom entry one</title>
    <id>urn:uuid:entry-1</id>
    <updated>2025-09-15T10:00:00Z</updated>
    <published>2025-09-14T09:00:00Z</published>
    <link rel="alternate" href="https://example.com/entry-1"/>
    <author><name>Atom Author</name></author>
    <summary>First atom entry.</summary>
    <category term="atom"/>
  </entry>
</feed>`

	recs, err := RSSNews{}.Curate(t.Context(), []byte(atom), "application/atom+xml", rssChannel(schemas.EntityTypeNews))
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}

	if recs[0].ExternalKey != "urn:uuid:entry-1" {
		t.Errorf("external key = %q, want atom id", recs[0].ExternalKey)
	}

	if got := fieldString(t, recs[0].Extraction, "article_url"); got != "https://example.com/entry-1" {
		t.Errorf("article_url = %q", got)
	}
}

// TestJSONLDCurate covers the newline-joined ld+json contract: JobPosting →
// job, NewsArticle → news, Event → event, BlogPosting → article, with
// unsupported @types (BreadcrumbList) skipped.
func TestJSONLDCurate(t *testing.T) {
	t.Parallel()

	body := fixture(t, "ldjson.txt")
	ch := sources.Channel{
		CrawlMethod: "jsonld",
		EntityTypes: []string{schemas.EntityTypeJob, schemas.EntityTypeNews, schemas.EntityTypeEvent, schemas.EntityTypeArticle},
		EntryUrl:    "https://example.com/page",
	}

	recs, err := JSONLD{}.Curate(t.Context(), body, "application/ld+json", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if len(recs) != 4 {
		t.Fatalf("records = %d, want 4 (BreadcrumbList skipped): %+v", len(recs), recs)
	}

	byType := map[string]Record{}
	for _, r := range recs {
		byType[r.EntityType] = r
		fieldConfidences(t, r.Extraction)
	}

	job := byType[schemas.EntityTypeJob]
	if got := fieldString(t, job.Extraction, "title"); got != "Registered Nurse — Lagos to Leeds" {
		t.Errorf("job title = %q", got)
	}

	if job.ExternalKey != "job:job-7712" {
		t.Errorf("job external key = %q, want identifier-derived", job.ExternalKey)
	}

	if got := fieldString(t, job.Extraction, "employer"); got != "Leeds Teaching Hospitals" {
		t.Errorf("employer = %q", got)
	}

	if got := fieldString(t, job.Extraction, "job_type"); got != "full_time" {
		t.Errorf("job_type = %q, want full_time", got)
	}

	if got := fieldString(t, job.Extraction, "city"); got != "Leeds" {
		t.Errorf("city = %q", got)
	}

	if got := fieldString(t, job.Extraction, "deadline"); got != "2025-12-31" {
		t.Errorf("deadline = %q, want 2025-12-31", got)
	}

	if got := fieldString(t, job.Extraction, "remote"); got != "true" {
		t.Errorf("remote = %q, want true (TELECOMMUTE)", got)
	}

	if n, ok := fieldValue(t, job.Extraction, "salary_min").(float64); !ok || n != 32000 {
		t.Errorf("salary_min = %v, want 32000", fieldValue(t, job.Extraction, "salary_min"))
	}

	news := byType[schemas.EntityTypeNews]
	if news.ExternalKey != "https://news.example.com/diaspora-bond" {
		t.Errorf("news external key = %q, want url", news.ExternalKey)
	}

	if got := fieldString(t, news.Extraction, "published_date"); got != "2025-09-12" {
		t.Errorf("news published_date = %q", got)
	}

	event := byType[schemas.EntityTypeEvent]
	if got := fieldString(t, event.Extraction, "title"); got != "Pre-departure orientation — Lagos" {
		t.Errorf("event title = %q", got)
	}

	if got := fieldString(t, event.Extraction, "starts_at"); got != "2025-10-04T10:00:00+01:00" {
		t.Errorf("starts_at = %q", got)
	}

	if got := fieldString(t, event.Extraction, "organizer"); got != "JapaConnect" {
		t.Errorf("organizer = %q", got)
	}

	article := byType[schemas.EntityTypeArticle]
	if got := fieldString(t, article.Extraction, "body"); got == "" {
		t.Error("article body empty — articleBody should map")
	}

	if got := fieldString(t, article.Extraction, "image_url"); got != "https://blog.example.com/img/sa.png" {
		t.Errorf("image_url = %q", got)
	}
}

// TestJSONLDCurate_ChannelTypeFilter verifies nodes whose mapped entity
// type is not declared on the channel are skipped.
func TestJSONLDCurate_ChannelTypeFilter(t *testing.T) {
	t.Parallel()

	body := fixture(t, "ldjson.txt")
	ch := sources.Channel{
		CrawlMethod: "jsonld",
		EntityTypes: []string{schemas.EntityTypeJob},
	}

	recs, err := JSONLD{}.Curate(t.Context(), body, "application/ld+json", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if len(recs) != 1 || recs[0].EntityType != schemas.EntityTypeJob {
		t.Fatalf("records = %+v, want the single JobPosting", recs)
	}
}

// TestICalCurate covers the VEVENT fixture: UID external keys, TZID-aware
// datetimes, line unfolding, text unescaping, and the RRULE occurrence
// window.
func TestICalCurate(t *testing.T) {
	t.Parallel()

	body := fixture(t, "events.ics")
	ch := sources.Channel{
		CrawlMethod: "ical",
		EntityTypes: []string{schemas.EntityTypeEvent},
		EntryUrl:    "https://events.example.com/feed.ics",
	}

	recs, err := ICal{}.Curate(t.Context(), body, "text/calendar", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2", len(recs))
	}

	first, second := recs[0], recs[1]

	if first.ExternalKey != "embassy-orientation-2025-10@events.example.com" {
		t.Errorf("external key = %q, want UID", first.ExternalKey)
	}

	if got := fieldString(t, first.Extraction, "title"); got != "Pre-departure orientation — Toronto" {
		t.Errorf("title = %q", got)
	}
	// DTSTART;TZID=America/Toronto:20251004T140000 → 14:00 EDT = 18:00 UTC.
	if got := fieldString(t, first.Extraction, "starts_at"); got != "2025-10-04T18:00:00Z" {
		t.Errorf("starts_at = %q, want 2025-10-04T18:00:00Z (TZID applied)", got)
	}

	if got := fieldString(t, first.Extraction, "location"); got != "North York Civic Centre, Toronto" {
		t.Errorf("location = %q (escaped comma should decode)", got)
	}

	if got := fieldString(t, first.Extraction, "organizer"); got != "JapaConnect" {
		t.Errorf("organizer = %q, want CN param", got)
	}
	// Folded description line must be joined.
	if got := fieldString(t, first.Extraction, "description"); got != "A long description that is folded acrosstwo lines per RFC 5545 continuation rules." {
		t.Errorf("description = %q", got)
	}

	// RRULE:FREQ=WEEKLY;COUNT=4 → 4 occurrence timestamps.
	if got := fieldString(t, first.Extraction, "recurrence"); got != "FREQ=WEEKLY;COUNT=4" {
		t.Errorf("recurrence = %q", got)
	}

	occ := fieldStrings(t, first.Extraction, "occurrences")
	if len(occ) != 4 {
		t.Fatalf("occurrences = %v, want 4 weekly instances", occ)
	}

	if occ[0] != "2025-10-04T18:00:00Z" || occ[3] != "2025-10-25T18:00:00Z" {
		t.Errorf("occurrences = %v, want 4 weekly steps from Oct 4", occ)
	}

	if second.ExternalKey != "visa-webinar-2025-11@events.example.com" {
		t.Errorf("second external key = %q, want UID", second.ExternalKey)
	}

	if got := fieldString(t, second.Extraction, "starts_at"); got != "2025-11-05T18:00:00Z" {
		t.Errorf("second starts_at = %q", got)
	}

	if got := fieldString(t, second.Extraction, "description"); got != "Live session with an immigration adviser; bring questions." {
		t.Errorf("second description = %q (escaped semicolon should decode)", got)
	}
}

// TestTabularCurate covers the CSV fixture: column_map fields, key_columns
// external keys, and unmapped columns preserved under `data`.
func TestTabularCurate(t *testing.T) {
	t.Parallel()

	body := fixture(t, "table.csv")
	ch := sources.Channel{
		CrawlMethod: "tabular",
		EntityTypes: []string{schemas.EntityTypeDataset},
		EntryUrl:    "https://data.example.com/fees.csv",
		ProviderConfig: []byte(`{
			"column_map": {"Program": "title", "Fee": "value", "Currency": "unit"},
			"key_columns": ["Program"]
		}`),
	}

	recs, err := Tabular{}.Curate(t.Context(), body, "text/csv", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if len(recs) != 3 {
		t.Fatalf("records = %d, want 3 data rows", len(recs))
	}

	first := recs[0]
	if first.EntityType != schemas.EntityTypeDataset {
		t.Errorf("entity type = %q, want dataset", first.EntityType)
	}

	if len(first.ExternalKey) < 4 || first.ExternalKey[:4] != "row:" {
		t.Errorf("external key = %q, want row: hash", first.ExternalKey)
	}

	if got := fieldString(t, first.Extraction, "title"); got != "Express Entry application" {
		t.Errorf("title = %q", got)
	}

	if got := fieldString(t, first.Extraction, "value"); got != "1525" {
		t.Errorf("value = %q", got)
	}

	if got := fieldString(t, first.Extraction, "unit"); got != "CAD" {
		t.Errorf("unit = %q", got)
	}
	// Unmapped "Notes" column lands under data.
	if data, ok := fieldValue(t, first.Extraction, "data").(map[string]any); !ok || data["Notes"] != "Per adult applicant" {
		t.Errorf("data = %v, want Notes preserved", fieldValue(t, first.Extraction, "data"))
	}

	// Key stability: re-curating produces identical external keys.
	recs2, err := Tabular{}.Curate(t.Context(), body, "text/csv", ch)
	if err != nil {
		t.Fatalf("Curate again: %v", err)
	}

	for i := range recs {
		if recs[i].ExternalKey != recs2[i].ExternalKey {
			t.Errorf("row %d external key unstable: %q vs %q", i, recs[i].ExternalKey, recs2[i].ExternalKey)
		}
	}
}

// TestTabularCurate_NoConfig verifies rows still produce records without a
// column_map — title falls back to the first column and the full row lands
// under data.
func TestTabularCurate_NoConfig(t *testing.T) {
	t.Parallel()

	body := fixture(t, "table.csv")
	ch := sources.Channel{
		CrawlMethod: "tabular",
		EntityTypes: []string{schemas.EntityTypeDataset},
		EntryUrl:    "https://data.example.com/fees.csv",
	}

	recs, err := Tabular{}.Curate(t.Context(), body, "text/csv", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if len(recs) != 3 {
		t.Fatalf("records = %d, want 3", len(recs))
	}

	if got := fieldString(t, recs[0].Extraction, "title"); got != "Express Entry application" {
		t.Errorf("title = %q, want first-column fallback", got)
	}
}

// TestRegistryDispatch covers Registry.Curate dispatching by crawl_method
// and the ErrNoCurator sentinel the worker uses for the extract fallback.
func TestRegistryDispatch(t *testing.T) {
	t.Parallel()

	reg := Default()

	recs, err := reg.Curate(t.Context(), fixture(t, "feed.rss"), "application/rss+xml", rssChannel(schemas.EntityTypeNews))
	if err != nil {
		t.Fatalf("registry Curate(rss): %v", err)
	}

	if len(recs) != 3 {
		t.Fatalf("records = %d, want 3", len(recs))
	}

	_, err = reg.Curate(t.Context(), []byte("https://example.com/a\t2025-01-01\n"), "text/plain", sources.Channel{CrawlMethod: "sitemap"})
	if !errors.Is(err, ErrNoCurator) {
		t.Errorf("sitemap Curate err = %v, want ErrNoCurator", err)
	}
}
