package worker

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/fetcher"
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
		ContentType: strPtr(fetcher.ManifestContentType), Body: prevBody, ContentHash: "hash-prev", ChannelID: ch.ID,
	}); err != nil {
		t.Fatalf("create prev raw doc: %v", err)
	}
	curDoc, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID: src.ID, Url: ch.EntryUrl, HttpStatus: 200, HttpHeaders: []byte("{}"),
		ContentType: strPtr(fetcher.ManifestContentType), Body: curBody, ContentHash: "hash-cur", ChannelID: ch.ID,
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

// TestIntelExpandManifest_SitemapIndexChildren covers nested expansion: a
// manifest stamped kind=index fans out member sitemaps as sitemap_child
// derived fetches (nested manifests), not static page fetches.
func TestIntelExpandManifest_SitemapIndexChildren(t *testing.T) {
	_, srcStore, store, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "manifest-idx-src", nil)

	ch, err := srcStore.CreateChannel(t.Context(), gen.CreateIntelSourceChannelParams{
		SourceID:             src.ID,
		Label:                "sitemap-index",
		EntryUrl:             "https://example.com/sitemap_index.xml",
		CrawlMethod:          "sitemap",
		EntityTypes:          []string{"news"},
		Pipeline:             "curate",
		ProviderConfig:       []byte(`{}`),
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

	doc, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID: src.ID, Url: ch.EntryUrl, HttpStatus: 200,
		HttpHeaders: []byte(`{"` + fetcher.ManifestKindHeader + `":"` + fetcher.ManifestKindIndex + `"}`),
		ContentType: strPtr(fetcher.ManifestContentType),
		Body:        []byte("https://example.com/news-sitemap.xml\t2024-03-01\n"),
		ContentHash: "hash-idx", ChannelID: ch.ID,
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	intelExpandManifest(t.Context(), ch, doc)

	tasks := pendingTasks(t, inspector, TypeIntelFetchChannel)
	if len(tasks) != 1 {
		t.Fatalf("index manifest enqueued %d fetches, want 1", len(tasks))
	}
	var p intelChannelPayload
	if err := json.Unmarshal(tasks[0].Payload, &p); err != nil {
		t.Fatalf("payload decode: %v", err)
	}
	if !p.SitemapChild {
		t.Errorf("index manifest child missing sitemap_child flag: %+v", p)
	}
	if p.URLOverride != "https://example.com/news-sitemap.xml" {
		t.Errorf("url_override = %q, want member sitemap URL", p.URLOverride)
	}
}

// TestIntelExpandManifest_DerivedUsesURLScopedPrev covers the nested
// baseline: a derived manifest (sitemapindex member) diffs against the
// previous manifest captured at its own URL — not the channel's top-level
// manifest — so an unchanged member doesn't re-fan-out.
func TestIntelExpandManifest_DerivedUsesURLScopedPrev(t *testing.T) {
	_, srcStore, store, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "manifest-der-src", nil)

	ch, err := srcStore.CreateChannel(t.Context(), gen.CreateIntelSourceChannelParams{
		SourceID:             src.ID,
		Label:                "sitemap-index",
		EntryUrl:             "https://example.com/sitemap_index.xml",
		CrawlMethod:          "sitemap",
		EntityTypes:          []string{"news"},
		Pipeline:             "curate",
		ProviderConfig:       []byte(`{}`),
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

	parentDoc, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID: src.ID, Url: ch.EntryUrl, HttpStatus: 200, HttpHeaders: []byte("{}"),
		ContentType: strPtr(fetcher.ManifestContentType),
		Body:        []byte("https://example.com/news-sitemap.xml\t2024-03-01\n"),
		ContentHash: "hash-parent", ChannelID: ch.ID,
	})
	if err != nil {
		t.Fatalf("create parent doc: %v", err)
	}

	childURL := "https://example.com/news-sitemap.xml"
	childBody := []byte("https://example.com/a\t2024-01-01\n")

	// Previous capture of the SAME member sitemap — identical body.
	if _, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID: src.ID, Url: childURL, HttpStatus: 200, HttpHeaders: []byte("{}"),
		ContentType: strPtr(fetcher.ManifestContentType), Body: childBody,
		ContentHash: "hash-child-prev", ChannelID: ch.ID, ParentDocumentID: parentDoc.ID,
	}); err != nil {
		t.Fatalf("create prev child doc: %v", err)
	}
	curDoc, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID: src.ID, Url: childURL, HttpStatus: 200, HttpHeaders: []byte("{}"),
		ContentType: strPtr(fetcher.ManifestContentType), Body: childBody,
		ContentHash: "hash-child-cur", ChannelID: ch.ID, ParentDocumentID: parentDoc.ID,
	})
	if err != nil {
		t.Fatalf("create cur child doc: %v", err)
	}

	intelExpandManifest(t.Context(), ch, curDoc)

	if n := len(pendingTasks(t, inspector, TypeIntelFetchChannel)); n != 0 {
		t.Fatalf("unchanged derived manifest enqueued %d fetches, want 0 (URL-scoped prev)", n)
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
		{"request":{"method":"POST","url":"https://api.example.com/v1/search",
		 "postData":{"mimeType":"application/json","text":"{\"q\":\"visa\",\"limit\":10}"}},
		 "response":{"status":200,"content":{"mimeType":"application/json; charset=utf-8"}}},
		{"request":{"method":"POST","url":"https://api.example.com/v1/form",
		 "postData":{"mimeType":"application/x-www-form-urlencoded","text":"q=visa&page=2"}},
		 "response":{"status":200,"content":{"mimeType":"application/vnd.api+json"}}},
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
		{Method: "GET", URL: "https://api.example.com/v1/jobs"}, // query deduped
		// POST JSON kept, request body carried from postData.
		{Method: "POST", URL: "https://api.example.com/v1/search", Body: []byte(`{"q":"visa","limit":10}`), BodyMime: "application/json"},
		// Non-JSON postData kept too — body + mime replay into the proposal.
		{Method: "POST", URL: "https://api.example.com/v1/form", Body: []byte("q=visa&page=2"), BodyMime: "application/x-www-form-urlencoded"},
	}
	if len(got) != len(want) {
		t.Fatalf("endpoints = %v, want %v", got, want)
	}
	for i := range want {
		if got[i].Method != want[i].Method || got[i].URL != want[i].URL ||
			string(got[i].Body) != string(want[i].Body) || got[i].BodyMime != want[i].BodyMime {
			t.Fatalf("endpoints = %v, want %v", got, want)
		}
	}
}

// TestIntelSniffProposalInput covers the endpoint → channel proposal
// mapping, including the postData → provider_config.body propagation that
// keeps a discovered POST API replayable.
func TestIntelSniffProposalInput(t *testing.T) {
	t.Parallel()

	t.Run("post body propagates", func(t *testing.T) {
		in := intelSniffProposalInput(sniffEndpoint{
			Method: "POST", URL: "https://api.example.com/v1/search",
			Body: []byte(`{"q":"visa"}`),
		})

		var cfg map[string]json.RawMessage
		if err := json.Unmarshal(in.ProviderConfig, &cfg); err != nil {
			t.Fatalf("provider_config decode: %v", err)
		}
		if string(cfg["body"]) != `{"q":"visa"}` {
			t.Fatalf("body = %s, want posted JSON preserved", cfg["body"])
		}
	})

	t.Run("non-JSON body stored as string with mime", func(t *testing.T) {
		in := intelSniffProposalInput(sniffEndpoint{
			Method: "POST", URL: "https://api.example.com/v1/search",
			Body:     []byte("a=1&b=2"),
			BodyMime: "application/x-www-form-urlencoded",
		})

		var cfg map[string]json.RawMessage
		if err := json.Unmarshal(in.ProviderConfig, &cfg); err != nil {
			t.Fatalf("provider_config decode: %v", err)
		}
		// Stored as a JSON string — the api fetcher sends it unquoted.
		var body string
		if err := json.Unmarshal(cfg["body"], &body); err != nil || body != "a=1&b=2" {
			t.Fatalf("body = %s, want JSON-encoded string %q", cfg["body"], "a=1&b=2")
		}
		var headers map[string]string
		if err := json.Unmarshal(cfg["headers"], &headers); err != nil {
			t.Fatalf("headers decode: %v", err)
		}
		if headers["Content-Type"] != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type header = %q", headers["Content-Type"])
		}
	})

	t.Run("get carries no body", func(t *testing.T) {
		in := intelSniffProposalInput(sniffEndpoint{Method: "GET", URL: "https://api.example.com/v1/jobs"})

		var cfg map[string]json.RawMessage
		if err := json.Unmarshal(in.ProviderConfig, &cfg); err != nil {
			t.Fatalf("provider_config decode: %v", err)
		}
		if _, present := cfg["body"]; present {
			t.Fatal("GET proposal must not set provider_config.body")
		}
	})
}
