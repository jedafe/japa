package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/intelligence/stores"
)

// ---------------------------------------------------------------------------
// Phase-4 spider fan-out + outbound source discovery
// ---------------------------------------------------------------------------

// mkSpiderManifestDoc inserts a spider-kind manifest raw doc over body
// (url<TAB>stamp lines) stamped at the given crawl depth.
func mkSpiderManifestDoc(t *testing.T, store *stores.Store, src gen.IntelSource, ch gen.IntelSourceChannel, body string, depth int) gen.IntelRawDocument {
	t.Helper()

	headers, err := json.Marshal(map[string]string{
		fetcher.ManifestKindHeader: fetcher.ManifestKindSpider,
		fetcher.SpiderDepthHeader:  fmt.Sprintf("%d", depth),
	})
	if err != nil {
		t.Fatalf("marshal headers: %v", err)
	}

	doc, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		Url:         ch.EntryUrl,
		HttpStatus:  200,
		HttpHeaders: headers,
		ContentType: strPtr(fetcher.ManifestContentType),
		Body:        []byte(body),
		ContentHash: "hash-spider-" + fmt.Sprint(depth),
		ChannelID:   ch.ID,
	})
	if err != nil {
		t.Fatalf("create spider manifest doc: %v", err)
	}

	return doc
}

// mkSpiderChannel inserts a spider channel over src with the given
// provider_config.
func mkSpiderChannel(t *testing.T, srcStore *sources.Store, src gen.IntelSource, providerConfig string) gen.IntelSourceChannel {
	t.Helper()

	return mkIntelTestChannel(t, srcStore, src, func(arg *gen.CreateIntelSourceChannelParams) {
		arg.CrawlMethod = "spider"
		arg.ProviderConfig = []byte(providerConfig)
	})
}

// decodeSpiderPayloads returns the spider-flagged fetch payloads pending
// across all queues.
func decodeSpiderPayloads(t *testing.T, inspector *asynq.Inspector) []intelChannelPayload {
	t.Helper()

	var out []intelChannelPayload
	for _, ti := range pendingTasks(t, inspector, TypeIntelFetchChannel) {
		var p intelChannelPayload
		if err := json.Unmarshal(ti.Payload, &p); err != nil {
			t.Fatalf("payload decode: %v", err)
		}
		out = append(out, p)
	}

	return out
}

// TestIntelExpandManifest_SpiderFansOutNested covers depth 0→1 under
// max_depth=2: children are derived spider fetches (nested manifests),
// not static page fetches.
func TestIntelExpandManifest_SpiderFansOutNested(t *testing.T) {
	_, srcStore, store, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "spider-nest", nil)
	ch := mkSpiderChannel(t, srcStore, src, `{"max_depth":2}`)

	doc := mkSpiderManifestDoc(t, store, src, ch,
		"https://example.com/a\t2024-01-01\nhttps://example.com/b\t2024-01-01\n", 0)

	intelExpandManifest(t.Context(), ch, doc)

	payloads := decodeSpiderPayloads(t, inspector)
	if len(payloads) != 2 {
		t.Fatalf("derived fetches = %d, want 2", len(payloads))
	}
	for _, p := range payloads {
		if !p.SpiderChild {
			t.Errorf("depth-1 child missing spider_child (max_depth=2): %+v", p)
		}
		if p.SpiderDepth != 1 {
			t.Errorf("spider_depth = %d, want 1: %+v", p.SpiderDepth, p)
		}
		if p.ParentDocID != doc.ID.String() {
			t.Errorf("parent_document_id = %q, want %q", p.ParentDocID, doc.ID.String())
		}
	}
}

// TestIntelExpandManifest_SpiderCeiling covers the depth ceiling: under
// max_depth=2 a depth-1 manifest's children are content pages — fetched
// via the static adapter (spider_child=false) at depth 2.
func TestIntelExpandManifest_SpiderCeiling(t *testing.T) {
	_, srcStore, store, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "spider-ceil", nil)
	ch := mkSpiderChannel(t, srcStore, src, `{"max_depth":2}`)

	doc := mkSpiderManifestDoc(t, store, src, ch,
		"https://example.com/deep\t2024-01-01\n", 1)

	intelExpandManifest(t.Context(), ch, doc)

	payloads := decodeSpiderPayloads(t, inspector)
	if len(payloads) != 1 {
		t.Fatalf("derived fetches = %d, want 1", len(payloads))
	}
	if payloads[0].SpiderChild {
		t.Errorf("ceiling child must be a content fetch (spider_child=false): %+v", payloads[0])
	}
	if payloads[0].SpiderDepth != 2 {
		t.Errorf("spider_depth = %d, want 2", payloads[0].SpiderDepth)
	}
}

// TestIntelExpandManifest_SpiderCrossHost covers the defense-in-depth
// host check: a manifest body carrying a foreign-host URL must not fan a
// fetch out to it even though the fetcher scopes extraction already.
func TestIntelExpandManifest_SpiderCrossHost(t *testing.T) {
	_, srcStore, store, inspector := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "spider-xhost", nil)
	ch := mkSpiderChannel(t, srcStore, src, `{"max_depth":1}`)

	doc := mkSpiderManifestDoc(t, store, src, ch,
		"https://example.com/ok\t2024-01-01\nhttps://evil.example.net/x\t2024-01-01\nhttps://127.0.0.1/y\t2024-01-01\n", 0)

	intelExpandManifest(t.Context(), ch, doc)

	payloads := decodeSpiderPayloads(t, inspector)
	if len(payloads) != 1 {
		t.Fatalf("derived fetches = %d, want 1 (same-host only)", len(payloads))
	}
	if payloads[0].URLOverride != "https://example.com/ok" {
		t.Errorf("url_override = %q, want https://example.com/ok", payloads[0].URLOverride)
	}
}

// TestIntelOutboundCandidates covers the harvest task's link-source
// selection: the spider header wins when present, HTML bodies are parsed
// otherwise, and non-HTML docs yield nothing.
func TestIntelOutboundCandidates(t *testing.T) {
	t.Parallel()

	headerJSON, _ := json.Marshal([]string{"https://a.example.com/", "https://b.example.org/x"})
	// http_headers is a map[string]string — the array rides as a
	// JSON-encoded string value, matching what the adapter stores.
	headerMap, _ := json.Marshal(map[string]string{fetcher.OutboundLinksHeader: string(headerJSON)})

	headerDoc := gen.IntelRawDocument{
		Url:         "https://site.example/",
		HttpHeaders: headerMap,
		ContentType: strPtr(fetcher.ManifestContentType),
	}
	if got := intelOutboundCandidates(headerDoc); len(got) != 2 {
		t.Fatalf("header-path candidates = %v, want 2", got)
	}

	htmlDoc := gen.IntelRawDocument{
		Url:         "https://site.example/",
		HttpHeaders: []byte("{}"),
		ContentType: strPtr("text/html"),
		Body:        []byte(`<a href="https://c.example.net/">C</a><a href="/self">S</a>`),
	}
	got := intelOutboundCandidates(htmlDoc)
	if len(got) != 1 || got[0] != "https://c.example.net/" {
		t.Fatalf("html-path candidates = %v, want [https://c.example.net/]", got)
	}

	feedDoc := gen.IntelRawDocument{
		Url:         "https://site.example/feed",
		HttpHeaders: []byte("{}"),
		ContentType: strPtr("application/rss+xml"),
		Body:        []byte(`<rss><link>https://c.example.net/</link></rss>`),
	}
	if got := intelOutboundCandidates(feedDoc); len(got) != 0 {
		t.Fatalf("non-HTML candidates = %v, want none", got)
	}
}

// TestIntelDiscoveryCandidateOK covers the per-candidate gates:
// scheme/host validity, private literals, self-links, parent-domain
// exclusion, and built-in/configured blocklists.
func TestIntelDiscoveryCandidateOK(t *testing.T) {
	oldSettings := appSettings
	t.Cleanup(func() { appSettings = oldSettings })
	appSettings = fakeSettingsReader{strings: map[string]string{
		"intel.discovery.domain_blocklist": `["blocked.example.com"]`,
	}}

	src := gen.IntelSource{Domain: "example.com"}
	doc := gen.IntelRawDocument{Url: "https://example.com/scholarships"}

	cases := []struct {
		name      string
		candidate string
		want      bool
	}{
		{"plain external host", "https://ext.example.org/page", true},
		{"external subdomain", "https://sub.ext.example.org/", true},
		{"non-http scheme", "ftp://ext.example.org/x", false},
		{"missing scheme", "ext.example.org/page", false},
		{"no dotted host", "https://localhost/x", false},
		{"private IP literal", "https://10.0.0.1/x", false},
		{"link-local literal", "https://169.254.169.254/latest", false},
		{"same host as doc", "https://example.com/other-page", false},
		{"parent domain", "https://example.com/", false},
		{"parent subdomain", "https://www.example.com/x", false},
		{"built-in denylist", "https://www.facebook.com/share", false},
		{"configured blocklist", "https://blocked.example.com/x", false},
		{"blocklist subdomain", "https://sub.blocked.example.com/x", false},
		{"domain-suffix spoof", "https://notexample.com/", true},
	}
	for _, tc := range cases {
		if got := intelDiscoveryCandidateOK(t.Context(), tc.candidate, src, doc); got != tc.want {
			t.Errorf("%s: candidateOK(%q) = %v, want %v", tc.name, tc.candidate, got, tc.want)
		}
	}
}

// TestHandleIntelHarvestOutbound_Proposes covers the end-to-end path: a
// spider manifest's outbound header becomes proposed, disabled,
// trust-level-5 source rows; re-runs dedupe; low-trust sources propose
// nothing.
func TestHandleIntelHarvestOutbound_Proposes(t *testing.T) {
	database, srcStore, store, _ := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "harvest-src", nil)
	ch := mkSpiderChannel(t, srcStore, src, `{}`)

	outbound, _ := json.Marshal([]string{
		"https://new-a.example.org/page",
		"https://new-b.example.net/",
		"https://facebook.com/share", // built-in denylist
		"https://example.com/self",   // parent domain
	})
	headers, _ := json.Marshal(map[string]string{
		fetcher.ManifestKindHeader:  fetcher.ManifestKindSpider,
		fetcher.SpiderDepthHeader:   "0",
		fetcher.OutboundLinksHeader: string(outbound),
	})

	doc, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		Url:         src.SourceUrl,
		HttpStatus:  200,
		HttpHeaders: headers,
		ContentType: strPtr(fetcher.ManifestContentType),
		Body:        []byte("https://example.com/a\t2024-01-01\n"),
		ContentHash: "hash-harvest",
		ChannelID:   ch.ID,
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	payload := marshalPayload(intelHarvestOutboundPayload{
		Version:  intelPayloadVersion,
		SourceID: src.ID.String(),
		RawDocID: doc.ID.String(),
	})
	task := asynq.NewTask(TypeIntelHarvestOutbound, payload)

	if err := HandleIntelHarvestOutbound(t.Context(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	for _, domain := range []string{"new-a.example.org", "new-b.example.net"} {
		ok, err := srcStore.DomainExists(t.Context(), domain)
		if err != nil || !ok {
			t.Fatalf("domain %s proposed: exists=%v err=%v", domain, ok, err)
		}
	}
	if ok, _ := srcStore.DomainExists(t.Context(), "facebook.com"); ok {
		t.Error("denylisted facebook.com was proposed")
	}

	proposed, err := srcStore.GetByURL(t.Context(), "https://new-a.example.org/page")
	if err != nil {
		t.Fatalf("lookup proposed source: %v", err)
	}
	t.Cleanup(func() { _ = srcStore.Delete(context.Background(), proposed.ID) })
	if proposed.Status != "proposed" || proposed.Enabled {
		t.Errorf("proposed source status=%q enabled=%v, want proposed/disabled", proposed.Status, proposed.Enabled)
	}
	if proposed.TrustLevel != 5 {
		t.Errorf("trust_level = %d, want 5", proposed.TrustLevel)
	}
	if proposed.CrawlMethod != "manual" {
		t.Errorf("crawl_method = %q, want manual", proposed.CrawlMethod)
	}
	if second, err := srcStore.GetByURL(t.Context(), "https://new-b.example.net/"); err == nil {
		t.Cleanup(func() { _ = srcStore.Delete(context.Background(), second.ID) })
	}

	// Re-run over the same doc must be a no-op (domain dedupe).
	if err := HandleIntelHarvestOutbound(t.Context(), task); err != nil {
		t.Fatalf("handler rerun: %v", err)
	}

	var count int
	if err := database.Writer.QueryRow(t.Context(),
		`SELECT count(*) FROM intel_sources WHERE domain = 'new-a.example.org'`).Scan(&count); err != nil {
		t.Fatalf("count proposed: %v", err)
	}
	if count != 1 {
		t.Errorf("new-a.example.org rows = %d after rerun, want 1", count)
	}
}

// TestHandleIntelHarvestOutbound_TrustGate covers the trust-level gate:
// a community-tier source's outbound links are never proposed.
func TestHandleIntelHarvestOutbound_TrustGate(t *testing.T) {
	_, srcStore, store, _ := wireIntelChannelDeps(t)

	src := mkIntelTestSource(t, srcStore, "harvest-lowtrust", func(p *gen.CreateIntelSourceParams) {
		p.TrustLevel = 5
	})
	ch := mkSpiderChannel(t, srcStore, src, `{}`)

	outbound, _ := json.Marshal([]string{"https://gated.example.org/"})
	headers, _ := json.Marshal(map[string]string{
		fetcher.ManifestKindHeader:  fetcher.ManifestKindSpider,
		fetcher.OutboundLinksHeader: string(outbound),
	})

	doc, err := store.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
		SourceID:    src.ID,
		Url:         src.SourceUrl,
		HttpStatus:  200,
		HttpHeaders: headers,
		ContentType: strPtr(fetcher.ManifestContentType),
		Body:        []byte("https://example.com/a\t2024-01-01\n"),
		ContentHash: "hash-gated",
		ChannelID:   ch.ID,
	})
	if err != nil {
		t.Fatalf("create raw doc: %v", err)
	}

	task := asynq.NewTask(TypeIntelHarvestOutbound, marshalPayload(intelHarvestOutboundPayload{
		Version: intelPayloadVersion, SourceID: src.ID.String(), RawDocID: doc.ID.String(),
	}))

	if err := HandleIntelHarvestOutbound(t.Context(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if ok, _ := srcStore.DomainExists(t.Context(), "gated.example.org"); ok {
		t.Error("trust-5 source's outbound link was proposed despite the trust gate")
	}
}
