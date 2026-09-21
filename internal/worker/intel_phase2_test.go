package worker

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Phase-2 manifest fan-out — sitemap/wayback manifest diffs enqueue derived
// intel:fetch_channel tasks (url_override + parent_document_id); manifests
// never reach curate/extract.
// ---------------------------------------------------------------------------

func TestIntelExpandManifest_FansOut(t *testing.T) {
	_, srcStore, store, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "manifest-src", nil)

	ch, err := srcStore.CreateChannel(t.Context(), gen.CreateIntelSourceChannelParams{
		SourceID:             src.ID,
		Label:                "sitemap",
		EntryUrl:             "https://example.com/sitemap.xml",
		CrawlMethod:          "sitemap",
		EntityTypes:          []string{"news"},
		Pipeline:             "curate",
		ProviderConfig:       []byte(`{"max_urls":10}`),
		CrawlFrequencyMins:   1440,
		Enabled:              true,
		Origin:               "probe",
		RelevanceMode:        "filtered",
		RelevanceTopics:      []string{},
		RelevanceUrlPatterns: []string{},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	prevBody := []byte("https://example.com/a\t2024-01-01\nhttps://example.com/b\t2024-01-02\n")
	curBody := []byte("https://example.com/a\t2024-01-01\nhttps://example.com/b\t2024-03-01\nhttps://example.com/c\t2024-03-02\n")

	if _, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID: src.ID, Url: ch.EntryUrl, HttpStatus: 200, HttpHeaders: []byte("{}"),
		ContentType: strPtr("text/plain"), Body: prevBody, ContentHash: "hash-prev", ChannelID: ch.ID,
	}); err != nil {
		t.Fatalf("create prev raw doc: %v", err)
	}
	curDoc, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID: src.ID, Url: ch.EntryUrl, HttpStatus: 200, HttpHeaders: []byte("{}"),
		ContentType: strPtr("text/plain"), Body: curBody, ContentHash: "hash-cur", ChannelID: ch.ID,
	})
	if err != nil {
		t.Fatalf("create cur raw doc: %v", err)
	}

	intelExpandManifest(t.Context(), ch, curDoc)

	tasks, err := inspector.ListPendingTasks("default")
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}

	var derived []intelChannelPayload
	for _, ti := range tasks {
		if ti.Type != TypeIntelFetchChannel {
			continue
		}
		var p intelChannelPayload
		if err := json.Unmarshal(ti.Payload, &p); err != nil {
			t.Fatalf("payload decode: %v", err)
		}
		if p.ParentDocID != "" {
			derived = append(derived, p)
		}
	}

	if len(derived) != 2 {
		t.Fatalf("derived fetch tasks = %d, want 2 (changed /b, new /c)", len(derived))
	}
	for _, p := range derived {
		if p.URLOverride == "" {
			t.Errorf("derived payload missing url_override: %+v", p)
		}
		if p.ParentDocID != curDoc.ID.String() {
			t.Errorf("parent_document_id = %q, want %q", p.ParentDocID, curDoc.ID.String())
		}
		if p.ChannelID != ch.ID.String() {
			t.Errorf("channel_id = %q, want %q", p.ChannelID, ch.ID.String())
		}
	}
}

// TestIntelExpandManifest_Cap checks the per-crawl fan-out bound.
func TestIntelExpandManifest_Cap(t *testing.T) {
	_, srcStore, store, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "manifest-cap-src", nil)

	ch, err := srcStore.CreateChannel(t.Context(), gen.CreateIntelSourceChannelParams{
		SourceID:             src.ID,
		Label:                "sitemap",
		EntryUrl:             "https://example.com/sitemap.xml",
		CrawlMethod:          "sitemap",
		EntityTypes:          []string{"news"},
		Pipeline:             "curate",
		ProviderConfig:       []byte(`{"max_urls":3}`),
		CrawlFrequencyMins:   1440,
		Enabled:              true,
		Origin:               "probe",
		RelevanceMode:        "filtered",
		RelevanceTopics:      []string{},
		RelevanceUrlPatterns: []string{},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	var b []byte
	for i := 0; i < 10; i++ {
		b = append(b, []byte(fmt.Sprintf("https://example.com/p%d\t2024-01-01\n", i))...)
	}
	doc, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID: src.ID, Url: ch.EntryUrl, HttpStatus: 200, HttpHeaders: []byte("{}"),
		ContentType: strPtr("text/plain"), Body: b, ContentHash: "hash-x", ChannelID: ch.ID,
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	intelExpandManifest(t.Context(), ch, doc)

	tasks, err := inspector.ListPendingTasks("default")
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	count := 0
	for _, ti := range tasks {
		if ti.Type == TypeIntelFetchChannel {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("fan-out = %d, want provider_config.max_urls cap of 3", count)
	}
}

// TestIntelSniffEndpoints covers the pure HAR → endpoint extraction.
func TestIntelSniffEndpoints(t *testing.T) {
	t.Parallel()

	har := `{"log":{"entries":[
		{"request":{"method":"GET","url":"https://api.example.com/v1/jobs?page=1"},
		 "response":{"status":200,"content":{"mimeType":"application/json"}}},
		{"request":{"method":"GET","url":"https://api.example.com/v1/jobs?page=2"},
		 "response":{"status":200,"content":{"mimeType":"application/json"}}},
		{"request":{"method":"GET","url":"https://cdn.example.com/app.js"},
		 "response":{"status":200,"content":{"mimeType":"text/javascript"}}},
		{"request":{"method":"POST","url":"https://api.example.com/v1/search"},
		 "response":{"status":200,"content":{"mimeType":"application/json; charset=utf-8"}}},
		{"request":{"method":"GET","url":"https://api.example.com/v1/err"},
		 "response":{"status":500,"content":{"mimeType":"application/json"}}},
		{"request":{"method":"GET","url":"http://169.254.169.254/latest/meta"},
		 "response":{"status":200,"content":{"mimeType":"application/json"}}}
	]}}`

	var env harEnvelope
	if err := json.Unmarshal([]byte(har), &env); err != nil {
		t.Fatalf("har decode: %v", err)
	}

	got := intelSniffEndpoints(env)
	want := []sniffEndpoint{
		{Method: "GET", URL: "https://api.example.com/v1/jobs"},   // query deduped
		{Method: "POST", URL: "https://api.example.com/v1/search"}, // POST JSON kept
	}
	if len(got) != len(want) {
		t.Fatalf("endpoints = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("endpoints = %v, want %v", got, want)
		}
	}
}
