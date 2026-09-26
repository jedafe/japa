package fetcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// spiderPage returns an HTML page mixing same-host, subdomain, external,
// asset, tracking-param, and fragment links against the fixture server.
func spiderPage(srvURL string) string {
	host := strings.TrimPrefix(srvURL, "http://")
	return fmt.Sprintf(`<html><body>
		<a href="/scholarships/list">Scholarships</a>
		<a href="/admissions/apply">Apply</a>
		<a href="%s/scholarships/detail?utm_source=x&fbclid=abc">Detail</a>
		<a href="/logo.png">Logo</a>
		<a href="#top">Top</a>
		<a href="https://sub.%s/scholarships">Subdomain link</a>
		<a href="https://external.example.com/visa-guide">External A</a>
		<a href="https://external.example.com/other">External A dup</a>
		<a href="https://other.example.net/jobs">External B</a>
		<a href="https://10.9.9.9/internal">Private literal</a>
		<a href="mailto:hi@example.com">Mail</a>
		<a href="javascript:void(0)">JS</a>
	</body></html>`, srvURL, host)
}

// TestCompositeFetch_Spider verifies the spider adapter produces a
// url<TAB>stamp manifest scoped to the entry host plus the outbound-link
// discovery header.
func TestCompositeFetch_Spider(t *testing.T) {
	t.Parallel()

	srv, c := channelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(spiderPage(srvURL(r))))
	}))

	src := sources.Source{SourceUrl: "https://source.example/ignored"}
	ch := sources.Channel{EntryUrl: srv.URL + "/", CrawlMethod: string(sources.CrawlMethodSpider)}

	res, err := c.Fetch(t.Context(), src, ch)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if res.ContentType != ManifestContentType {
		t.Fatalf("ContentType = %q, want manifest marker", res.ContentType)
	}
	if res.Headers[ManifestKindHeader] != ManifestKindSpider {
		t.Errorf("manifest kind = %q, want spider", res.Headers[ManifestKindHeader])
	}
	if res.Headers[SpiderDepthHeader] != "0" {
		t.Errorf("spider depth = %q, want 0", res.Headers[SpiderDepthHeader])
	}

	lines := ParseManifest(res.Body)
	if len(lines) == 0 {
		t.Fatal("manifest empty — same-host links missing")
	}

	for u := range lines {
		if !strings.HasPrefix(u, srv.URL) {
			t.Errorf("manifest contains non-entry-host URL %q", u)
		}
		if strings.Contains(u, "utm_") || strings.Contains(u, "fbclid") || strings.Contains(u, "#") {
			t.Errorf("manifest URL not normalized: %q", u)
		}
		if strings.HasSuffix(u, ".png") {
			t.Errorf("asset link in manifest: %q", u)
		}
	}
	if _, ok := lines[srv.URL+"/scholarships/list"]; !ok {
		t.Errorf("manifest missing /scholarships/list: %v", lines)
	}

	var outbound []string
	if err := json.Unmarshal([]byte(res.Headers[OutboundLinksHeader]), &outbound); err != nil {
		t.Fatalf("outbound header: %v", err)
	}

	hosts := map[string]int{}
	for _, u := range outbound {
		if strings.Contains(u, "10.9.9.9") || strings.HasPrefix(u, "mailto:") {
			t.Errorf("outbound list carried a non-crawlable URL: %q", u)
		}
		if i := strings.Index(u, "://"); i >= 0 {
			hosts[u[i+3:strings.Index(u[i+3:], "/")+i+3]]++
		}
	}
	if hosts["external.example.com"] != 1 {
		t.Errorf("external.example.com deduped by host = %d entries, want 1 (%v)", hosts["external.example.com"], outbound)
	}
	if hosts["other.example.net"] != 1 {
		t.Errorf("other.example.net missing from outbound list: %v", outbound)
	}
}

// TestCompositeFetch_SpiderDepthStamp covers the worker-injected
// spider_depth provider_config key — a derived spider fetch must stamp
// the depth of the page it crawled so expansion knows where it sat.
func TestCompositeFetch_SpiderDepthStamp(t *testing.T) {
	t.Parallel()

	srv, c := channelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<a href="/a">A</a>`))
	}))

	src := sources.Source{SourceUrl: srv.URL}
	ch := sources.Channel{
		EntryUrl:       srv.URL + "/",
		CrawlMethod:    string(sources.CrawlMethodSpider),
		ProviderConfig: []byte(`{"spider_depth":1}`),
	}

	res, err := c.Fetch(t.Context(), src, ch)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Headers[SpiderDepthHeader] != "1" {
		t.Errorf("depth header = %q, want 1", res.Headers[SpiderDepthHeader])
	}
}

// TestCompositeFetch_SpiderNonHTML rejects a non-HTML entry — the method
// can only manifest anchor graphs.
func TestCompositeFetch_SpiderNonHTML(t *testing.T) {
	t.Parallel()

	srv, c := channelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	ch := sources.Channel{EntryUrl: srv.URL, CrawlMethod: string(sources.CrawlMethodSpider)}
	if _, err := c.Fetch(t.Context(), sources.Source{}, ch); err == nil || !strings.Contains(err.Error(), "expected HTML") {
		t.Fatalf("Fetch = %v, want HTML rejection", err)
	}
}

// TestSpiderMaxDepth covers provider_config.max_depth clamping: 1–2 is
// honored, out-of-range values clamp to the ceiling/floor.
func TestSpiderMaxDepth(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]int{
		`{}`:                1,
		`{"max_depth":2}`:   2,
		`{"max_depth":9}`:   2,
		`{"max_depth":0}`:   1,
		`{"max_depth":"x"}`: 1,
		`not-json`:          1,
	} {
		if got := SpiderMaxDepth([]byte(in)); got != want {
			t.Errorf("SpiderMaxDepth(%s) = %d, want %d", in, got, want)
		}
	}
}

// TestSpiderManifestDepth covers the header read-back used by expansion.
func TestSpiderManifestDepth(t *testing.T) {
	t.Parallel()

	if got := SpiderManifestDepth(map[string]string{SpiderDepthHeader: "2"}); got != 2 {
		t.Errorf("depth = %d, want 2", got)
	}
	if got := SpiderManifestDepth(map[string]string{SpiderDepthHeader: "bogus"}); got != 0 {
		t.Errorf("depth = %d, want 0 on malformed header", got)
	}
	if got := SpiderManifestDepth(nil); got != 0 {
		t.Errorf("depth = %d, want 0 on absent header", got)
	}
}

// TestExtractOutboundLinks covers the body-parse path the harvest task
// uses for non-spider raw docs.
func TestExtractOutboundLinks(t *testing.T) {
	t.Parallel()

	body := []byte(`<html><body>
		<a href="https://a.example.com/x">A</a>
		<a href="https://a.example.com/y">A2</a>
		<a href="/internal">self</a>
		<a href="https://b.example.org/">B</a>
	</body></html>`)

	out := ExtractOutboundLinks(body, "https://site.example/", 10)
	if len(out) != 2 {
		t.Fatalf("outbound = %v, want 2 (one per host)", out)
	}
	for _, u := range out {
		if strings.HasPrefix(u, "https://site.example") {
			t.Errorf("same-host link leaked into outbound list: %q", u)
		}
	}

	if capped := ExtractOutboundLinks(body, "https://site.example/", 1); len(capped) != 1 {
		t.Fatalf("cap = %v, want 1", capped)
	}
}

// srvURL is a small helper so the page template can embed the fixture's
// base URL inside a handler closure.
func srvURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
