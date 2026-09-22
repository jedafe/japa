package curator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

func mustUUIDCurator(t *testing.T, raw string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}

	return id
}

// TestEmailCurate covers the inbound-email envelope → single record:
// subject→title, text→summary+body, first link→article_url, and the
// {channel_id}:{message-id} external key.
func TestEmailCurate(t *testing.T) {
	t.Parallel()

	ch := sources.Channel{
		ID:          mustUUIDCurator(t, "550e8400-e29b-41d4-a716-446655440000"),
		CrawlMethod: "email",
		EntityTypes: []string{"article"},
	}

	body := []byte(`{
		"from": "Embassy Alerts <alerts@embassy.example>",
		"to": "intel@inbound.japa.example",
		"subject": "New student visa slots open",
		"message_id": "<msg-42@embassy.example>",
		"text": "Appointments open Friday.\nBook here: https://embassy.example/slots/42 and prepare documents."
	}`)

	recs, err := (Email{}).Curate(context.Background(), body, "application/x-intel-email", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}

	rec := recs[0]
	if fieldString(t, rec.Extraction, "title") != "New student visa slots open" {
		t.Errorf("title = %v", rec.Extraction["title"])
	}
	if got := fieldString(t, rec.Extraction, "article_url"); got != "https://embassy.example/slots/42" {
		t.Errorf("article_url = %q", got)
	}
	if got := fieldString(t, rec.Extraction, "author"); got != "Embassy Alerts" && got == "" {
		t.Errorf("author = %q", got)
	}
	if rec.ExternalKey != "550e8400-e29b-41d4-a716-446655440000:<msg-42@embassy.example>" {
		t.Errorf("ExternalKey = %q", rec.ExternalKey)
	}
	if rec.EntityType != "article" {
		t.Errorf("EntityType = %q", rec.EntityType)
	}
}

// TestEmailCurate_NoMessageID falls back to a content-hash key component
// when the delivery lacks Message-Id — still channel-scoped.
func TestEmailCurate_NoMessageID(t *testing.T) {
	t.Parallel()

	ch := sources.Channel{ID: mustUUIDCurator(t, "550e8400-e29b-41d4-a716-446655440000"), CrawlMethod: "email"}

	recs, err := (Email{}).Curate(context.Background(),
		[]byte(`{"subject":"s","text":"body"}`), "application/x-intel-email", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if len(recs) != 1 || !strings.HasPrefix(recs[0].ExternalKey, ch.ID.String()+":hash:") {
		t.Fatalf("ExternalKey = %+v", recs)
	}
}

// TestEmailCurate_Empty rejects an envelope with neither subject nor text.
func TestEmailCurate_Empty(t *testing.T) {
	t.Parallel()

	ch := sources.Channel{CrawlMethod: "email"}
	if _, err := (Email{}).Curate(context.Background(), []byte(`{"from":"a@b"}`), "", ch); err == nil {
		t.Fatal("expected error for empty delivery")
	}
}

// TestWebSubCurate verifies WebSub deliveries parse through the shared
// feed pipeline — an Atom push produces the same records the rss curator
// would.
func TestWebSubCurate(t *testing.T) {
	t.Parallel()

	ch := sources.Channel{
		CrawlMethod: "webhook",
		EntityTypes: []string{"news"},
	}

	body := []byte(`<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
	<title>Hub Feed</title>
	<entry>
		<title>Slot opened</title>
		<id>urn:entry:1</id>
		<published>2026-09-22T10:00:00Z</published>
		<link rel="alternate" href="https://hub.example/items/1"/>
		<summary>First pushed item</summary>
	</entry>
</feed>`)

	recs, err := (WebSub{}).Curate(context.Background(), body, "application/atom+xml", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if fieldString(t, recs[0].Extraction, "title") != "Slot opened" {
		t.Errorf("title = %v", recs[0].Extraction["title"])
	}
	if recs[0].ExternalKey != "urn:entry:1" {
		t.Errorf("ExternalKey = %q", recs[0].ExternalKey)
	}
	if recs[0].EntityType != "news" {
		t.Errorf("EntityType = %q", recs[0].EntityType)
	}
}

// TestSearchCurate covers NDJSON → records: one per line, external_key =
// result URL, malformed lines skipped, empty body → error.
func TestSearchCurate(t *testing.T) {
	t.Parallel()

	ch := sources.Channel{
		CrawlMethod: "search",
		EntityTypes: []string{"news"},
	}

	body := []byte(strings.Join([]string{
		`{"title":"Hit A","url":"https://a.example/1","snippet":"first","published_at":"2026-09-20"}`,
		`{"title":"Hit B","url":"https://b.example/2","snippet":"second"}`,
		`not json at all`,
		`{"title":"no url"}`,
	}, "\n"))

	recs, err := (Search{}).Curate(context.Background(), body, "application/x-ndjson", ch)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2", len(recs))
	}
	if recs[0].ExternalKey != "https://a.example/1" || recs[1].ExternalKey != "https://b.example/2" {
		t.Errorf("ExternalKeys = %q, %q", recs[0].ExternalKey, recs[1].ExternalKey)
	}
	if fieldString(t, recs[0].Extraction, "title") != "Hit A" {
		t.Errorf("title = %v", recs[0].Extraction["title"])
	}
	if fieldString(t, recs[0].Extraction, "summary") != "first" {
		t.Errorf("summary = %v", recs[0].Extraction["summary"])
	}
	if recs[0].EntityType != "news" {
		t.Errorf("EntityType = %q", recs[0].EntityType)
	}
}

// TestSearchCurate_Empty errors when the page carries no usable lines.
func TestSearchCurate_Empty(t *testing.T) {
	t.Parallel()

	ch := sources.Channel{CrawlMethod: "search"}
	if _, err := (Search{}).Curate(context.Background(), []byte("garbage\n"), "", ch); err == nil {
		t.Fatal("expected error for no usable lines")
	}
}

// TestDefaultRegistry_Phase3 pins the Phase-3 curator registrations so a
// future refactor can't silently drop inbound/search curation back to the
// extract path.
func TestDefaultRegistry_Phase3(t *testing.T) {
	t.Parallel()

	reg := Default()
	for _, m := range []string{"email", "webhook", "search"} {
		ch := sources.Channel{CrawlMethod: m}
		_, err := reg.Curate(context.Background(), []byte("x"), "", ch)
		if errors.Is(err, ErrNoCurator) {
			t.Errorf("no curator registered for %q", m)
		}
	}
}
