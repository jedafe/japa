package fetcher

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

func pickerReq(t *testing.T, rawURL string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	return req
}

// Unmapped domains and unconfigured pools must return a nil proxy —
// direct egress identical to before the hook existed.
func TestEgressProxyPicker_UnmappedIsDirect(t *testing.T) {
	t.Parallel()

	settings := mockAdapterSettings{
		"intel.egress.proxy_domains": `{"walled.example":"residential"}`,
		"intel.egress.proxy_pools":   `{"residential":["http://proxy.example:8080"]}`,
	}

	p := NewEgressProxyPicker(settings, nil)
	if p == nil {
		t.Fatal("expected picker with settings")
	}

	for _, u := range []string{
		"https://other.example/page",
		"https://notwalled.example/",
	} {
		proxy, err := p.ProxyFor(pickerReq(t, u))
		if err != nil || proxy != nil {
			t.Fatalf("%s: proxy=%v err=%v, want direct", u, proxy, err)
		}
	}
}

// A mapped domain resolves to a pool member — exact and suffix matches —
// and the pick is stable across calls.
func TestEgressProxyPicker_MappedDomain(t *testing.T) {
	t.Parallel()

	settings := mockAdapterSettings{
		"intel.egress.proxy_domains": `{"walled.example":"residential","other.example":"datacenter"}`,
		"intel.egress.proxy_pools": `{
			"residential": ["http://u:p@res1.example:8080", "http://u:p@res2.example:8080"],
			"datacenter":  ["http://dc1.example:9000"]
		}`,
	}

	p := NewEgressProxyPicker(settings, startTestValkey(t))

	for _, u := range []string{"https://walled.example/x", "https://www.walled.example/y", "https://a.b.walled.example/"} {
		proxy, err := p.ProxyFor(pickerReq(t, u))
		if err != nil || proxy == nil {
			t.Fatalf("%s: proxy=%v err=%v, want residential pool member", u, proxy, err)
		}

		if !strings.HasPrefix(proxy.Host, "res") {
			t.Fatalf("%s: proxy host %q not in residential pool", u, proxy.Host)
		}
	}

	proxy, err := p.ProxyFor(pickerReq(t, "https://other.example/"))
	if err != nil || proxy == nil || proxy.Host != "dc1.example:9000" {
		t.Fatalf("other.example: proxy=%v err=%v, want dc1", proxy, err)
	}

	// Stickiness: repeated picks for one domain must agree — a rotating
	// egress IP mid-session is what makes vendors suspicious.
	first, _ := p.ProxyFor(pickerReq(t, "https://walled.example/"))

	for i := 0; i < 5; i++ {
		got, _ := p.ProxyFor(pickerReq(t, "https://walled.example/"))
		if got.String() != first.String() {
			t.Fatalf("pick %d = %q, want sticky %q", i, got, first)
		}
	}
}

// A sticky assignment whose URL left the pool is re-picked from the
// current pool (pool membership wins over the cached pin).
func TestEgressProxyPicker_StaleStickyRepicked(t *testing.T) {
	t.Parallel()

	settings := mockAdapterSettings{
		"intel.egress.proxy_domains":     `{"walled.example":"g"}`,
		"intel.egress.proxy_pools":       `{"g":["http://new.example:8080"]}`,
		"intel.egress.proxy_sticky_secs": "60",
	}

	rdb := startTestValkey(t)
	p := NewEgressProxyPicker(settings, rdb)

	// Pre-seed a stale assignment.
	if err := rdb.Set(t.Context(), egressProxyKey("walled.example"), "http://removed.example:1", 0).Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	proxy, err := p.ProxyFor(pickerReq(t, "https://walled.example/"))
	if err != nil || proxy == nil || proxy.Host != "new.example:8080" {
		t.Fatalf("proxy=%v err=%v, want re-pick to new.example:8080", proxy, err)
	}
}

// A private-IP literal target must never be proxied — with a proxy
// engaged the transport's dial guard only sees the proxy address, so the
// picker is the last boundary. Numeric IPv4 spellings (decimal, hex,
// octal) are covered the same way: net.ParseIP rejects them but an
// upstream proxy would dial them as loopback.
func TestEgressProxyPicker_PrivateLiteralRejected(t *testing.T) {
	t.Parallel()

	settings := mockAdapterSettings{
		"intel.egress.proxy_domains": `{"169.254.169.254":"g","2130706433":"g","0x7f000001":"g","0177.0.0.1":"g"}`,
		"intel.egress.proxy_pools":   `{"g":["http://proxy.example:8080"]}`,
	}

	p := NewEgressProxyPicker(settings, nil)

	for _, u := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://2130706433/", // decimal 127.0.0.1
		"http://0x7f000001/", // hex 127.0.0.1
		"http://0177.0.0.1/", // octal 127.0.0.1
	} {
		if _, err := p.ProxyFor(pickerReq(t, u)); err == nil {
			t.Fatalf("%s proxied — expected refusal", u)
		}
	}
}

// Overlapping suffix keys must resolve deterministically to the LONGEST
// match — map iteration order is random, so a.b.walled.example matching
// both ".walled.example" and ".example" must always pick the specific
// entry regardless of iteration order.
func TestEgressProxyPicker_LongestSuffixWins(t *testing.T) {
	t.Parallel()

	settings := mockAdapterSettings{
		"intel.egress.proxy_domains": `{"walled.example":"specific","example":"general"}`,
		"intel.egress.proxy_pools":   `{"specific":["http://specific.example:1"],"general":["http://general.example:2"]}`,
	}

	p := NewEgressProxyPicker(settings, nil)

	for i := 0; i < 20; i++ {
		proxy, err := p.ProxyFor(pickerReq(t, "https://a.b.walled.example/"))
		if err != nil || proxy == nil || proxy.Host != "specific.example:1" {
			t.Fatalf("iter %d: proxy=%v err=%v, want longest-match group", i, proxy, err)
		}
	}

	// A domain matching only the general key still resolves to it.
	proxy, err := p.ProxyFor(pickerReq(t, "https://other.example/"))
	if err != nil || proxy == nil || proxy.Host != "general.example:2" {
		t.Fatalf("proxy=%v err=%v, want general group", proxy, err)
	}
}

// Empty/invalid pool entries degrade to direct, not a fetch failure.
func TestEgressProxyPicker_BadPoolDegrades(t *testing.T) {
	t.Parallel()

	settings := mockAdapterSettings{
		"intel.egress.proxy_domains": `{"walled.example":"missing-group"}`,
		"intel.egress.proxy_pools":   `{"g":["not a url",""]}`,
	}

	p := NewEgressProxyPicker(settings, nil)

	proxy, err := p.ProxyFor(pickerReq(t, "https://walled.example/"))
	if err != nil || proxy != nil {
		t.Fatalf("proxy=%v err=%v, want direct on bad pool", proxy, err)
	}
}

// End-to-end: a client built with the picker routes a mapped domain's
// request through the proxy (absolute-URI form reaches the proxy server).
func TestSafeClient_EgressProxyEndToEnd(t *testing.T) {
	t.Parallel()

	var proxiedFor string

	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedFor = r.URL.String()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("via proxy"))
	}))
	defer proxySrv.Close()

	target := "http://walled.example/page" // never dialed — proxy answers

	p := NewEgressProxyPicker(mockAdapterSettings{
		"intel.egress.proxy_domains": `{"walled.example":"g"}`,
		"intel.egress.proxy_pools":   `{"g":["` + proxySrv.URL + `"]}`,
	}, nil)

	client := NewSafeClientOpts(10*time.Second, 0, SafeClientOpts{
		AllowPrivate: true, // proxy + test hosts are loopback
		Proxy:        p.ProxyFor,
	})

	resp, err := client.Get(target)
	if err != nil {
		t.Fatalf("proxied fetch: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if proxiedFor != target {
		t.Fatalf("proxy saw %q, want absolute URI %q", proxiedFor, target)
	}

	if string(body) != "via proxy" {
		t.Fatalf("body = %q, want proxy response", body)
	}
}

// Nil picker → egressProxyFunc nil → transport Proxy unset (no-op wiring).
func TestEgressProxyFunc_NilSettings(t *testing.T) {
	t.Parallel()

	if fn := egressProxyFunc(nil, nil); fn != nil {
		t.Fatal("nil settings must yield nil proxy func")
	}
}

// Composite wiring smoke test: NewComposite with a mapped domain still
// fetches through the proxy pool for the static adapter.
func TestComposite_EgressPoolUsedByStatic(t *testing.T) {
	t.Parallel()

	var sawAbsoluteURI bool

	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAbsoluteURI = strings.HasPrefix(r.RequestURI, "http")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer proxySrv.Close()

	settings := mockAdapterSettings{
		"intel.egress.proxy_domains": `{"walled.example":"g"}`,
		"intel.egress.proxy_pools":   `{"g":["` + proxySrv.URL + `"]}`,
	}

	// Rebuild the static client with the egress hook + AllowPrivate so the
	// loopback proxy is dialable — mirrors NewComposite's wiring minus the
	// dial guard the test can't satisfy.
	client := NewSafeClientOpts(0, 0, SafeClientOpts{
		AllowPrivate: true,
		Proxy:        egressProxyFunc(settings, nil),
	})

	c := &Composite{static: &StaticFetcher{client: client}}

	res, err := c.Fetch(t.Context(), sources.Source{}, sources.Channel{
		EntryUrl:    "http://walled.example/",
		CrawlMethod: "static",
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if !sawAbsoluteURI {
		t.Fatal("request did not take the proxy path")
	}

	if res.ViaMethod != "static" || len(res.Body) == 0 {
		t.Fatalf("result = %+v", res)
	}
}
