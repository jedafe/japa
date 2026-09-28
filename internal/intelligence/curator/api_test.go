package curator

import (
	"context"
	"errors"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

func apiChannel(entityTypes []string, providerConfig string) sources.Channel {
	return sources.Channel{
		EntryUrl:       "https://api.example.org/opportunities",
		CrawlMethod:    string(sources.CrawlMethodAPI),
		EntityTypes:    entityTypes,
		ProviderConfig: []byte(providerConfig),
	}
}

func TestAPI_FieldMapProducesRecords(t *testing.T) {
	body := []byte(`{"opportunities":[
		{"oppId":"daad-1","oppTitle":"Research Grant","closeDate":"2026-03-31","countryCode":"DEU","url":"https://x/1"},
		{"oppId":"daad-2","oppTitle":"Masters Award","closeDate":"June 30, 2026","countryCode":"DEU","url":"https://x/2"}
	]}`)

	ch := apiChannel([]string{"scholarship"}, `{
		"records_path": "opportunities",
		"field_map": {"title":"oppTitle","deadline":"closeDate","country":"countryCode","application_url":"url"},
		"key_fields": ["oppId"],
		"entity_type": "scholarship"
	}`)

	recs, err := (API{}).Curate(context.Background(), body, "application/json", ch)
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2", len(recs))
	}

	ext := recs[0].Extraction
	if got := fieldString(t, ext, "title"); got != "Research Grant" {
		t.Fatalf("title = %q", got)
	}
	if got := fieldString(t, ext, "deadline"); got != "2026-03-31" {
		t.Fatalf("deadline = %q", got)
	}
	if got := fieldStrings(t, ext, "country"); len(got) != 1 || got[0] != "DEU" {
		t.Fatalf("country = %v", got)
	}
	if recs[0].EntityType != "scholarship" {
		t.Fatalf("entity type = %q", recs[0].EntityType)
	}
	if recs[0].ExternalKey == recs[1].ExternalKey || recs[0].ExternalKey == "" {
		t.Fatalf("external keys not distinct/stable: %q vs %q", recs[0].ExternalKey, recs[1].ExternalKey)
	}
}

func TestAPI_GuessedKeysWithoutFieldMap(t *testing.T) {
	body := []byte(`[
		{"id": 42, "name": "Engineering Intern", "link": "https://x/job/42", "closeDate": "2026-01-15", "country": "NG"}
	]`)

	recs, err := (API{}).Curate(context.Background(), body, "application/json", apiChannel([]string{"job"}, "{}"))
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}

	ext := recs[0].Extraction
	if got := fieldString(t, ext, "title"); got != "Engineering Intern" {
		t.Fatalf("title = %q", got)
	}
	if got := fieldString(t, ext, "deadline"); got != "2026-01-15" {
		t.Fatalf("deadline = %q", got)
	}
}

func TestAPI_EnvelopeAutodetectsRecordsArray(t *testing.T) {
	body := []byte(`{"total": 1, "results": [{"id":"a","title":"Solo","deadline":"2030-01-01"}]}`)

	recs, err := (API{}).Curate(context.Background(), body, "application/json", apiChannel(nil, "{}"))
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	if len(recs) != 1 || fieldString(t, recs[0].Extraction, "title") != "Solo" {
		t.Fatalf("records = %+v", recs)
	}
}

func TestAPI_NoTitleFallsBackToExtract(t *testing.T) {
	body := []byte(`[{"id":1,"code":"xyz"},{"id":2,"code":"abc"}]`)

	_, err := (API{}).Curate(context.Background(), body, "application/json", apiChannel([]string{"job"}, "{}"))
	if !errors.Is(err, ErrNoCurator) {
		t.Fatalf("err = %v, want ErrNoCurator (fallback to LLM extract)", err)
	}
}

func TestAPI_NonJSONIsError(t *testing.T) {
	_, err := (API{}).Curate(context.Background(), []byte("<html>oops</html>"), "text/html", apiChannel(nil, "{}"))
	if err == nil || errors.Is(err, ErrNoCurator) {
		t.Fatalf("err = %v, want parse error (not ErrNoCurator)", err)
	}
}

func TestAPI_FieldMapOverridesGuessedKeys(t *testing.T) {
	// name exists but the declared field_map points elsewhere — config wins.
	body := []byte(`[{"oppId":"x1","name":"Wrong Field","oppTitle":"Right Field"}]`)

	ch := apiChannel(nil, `{"field_map":{"title":"oppTitle"},"key_fields":["oppId"]}`)
	recs, err := (API{}).Curate(context.Background(), body, "application/json", ch)
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	if got := fieldString(t, recs[0].Extraction, "title"); got != "Right Field" {
		t.Fatalf("title = %q, want field_map value", got)
	}
}
