package fetcher

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// TestHTTPFailureError covers the >=400 classifier: 429 → throttled (with
// Retry-After carried), challenge bodies on error statuses → walled, bare
// 401/403 → denied, other 4xx → gone, 5xx → unclassified transient.
func TestHTTPFailureError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		headers     http.Header
		body        string
		contentType string
		wantIs      error
		wantClass   FailureClass
	}{
		{"429", 429, nil, "slow down", "text/plain", ErrUpstreamThrottled, FailureThrottled},
		{"403 with cf challenge body", 403, nil,
			`<html><script>window._cf_chl_opt={};</script></html>`, "text/html", ErrBotWalled, FailureBlocked},
		{"200-style wall body on 503", 503, nil,
			`<html><body>Pardon Our Interruption</body></html>`, "text/html", ErrBotWalled, FailureBlocked},
		{"bare 403", 403, nil, "denied", "text/plain", ErrAccessDenied, FailureBlocked},
		{"bare 401", 401, nil, "unauthorized", "text/plain", ErrAccessDenied, FailureBlocked},
		{"404", 404, nil, "not found", "text/plain", ErrContentGone, FailureGone},
		{"410", 410, nil, "gone", "text/plain", ErrContentGone, FailureGone},
		{"400", 400, nil, "bad request", "text/plain", ErrContentGone, FailureGone},
		{"451 legal block", 451, nil, "unavailable", "text/plain", ErrContentGone, FailureGone},
		{"500", 500, nil, "server error", "text/plain", nil, FailureTransient},
		{"502", 502, nil, "bad gateway", "text/plain", nil, FailureTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := &http.Response{StatusCode: tt.status, Header: tt.headers}
			err := httpFailureError("test", resp, []byte(tt.body), tt.contentType)

			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("err = %v, want errors.Is %v", err, tt.wantIs)
			}

			if got := ClassifyFetchError(err); got != tt.wantClass {
				t.Errorf("class = %v, want %v (err %v)", got, tt.wantClass, err)
			}
		})
	}
}

// TestThrottledError_RetryAfter: the site's Retry-After hint is carried on
// the error so the worker sizes the domain cool-off from it.
func TestThrottledError_RetryAfter(t *testing.T) {
	t.Parallel()

	resp := &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"120"}}}
	err := httpFailureError("static", resp, nil, "")

	var te *ThrottledError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *ThrottledError", err)
	}

	if te.RetryAfter != 120*time.Second {
		t.Errorf("RetryAfter = %s, want 120s", te.RetryAfter)
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	if got := parseRetryAfter("90"); got != 90*time.Second {
		t.Errorf("delta-seconds = %s, want 90s", got)
	}

	if got := parseRetryAfter(""); got != 0 {
		t.Errorf("empty = %s, want 0", got)
	}

	if got := parseRetryAfter("garbage"); got != 0 {
		t.Errorf("garbage = %s, want 0", got)
	}

	future := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 || got > 30*time.Second {
		t.Errorf("http date = %s, want in (0, 30s]", got)
	}
}

// TestClassifyFetchError covers sentinel routing plus the DNS NXDOMAIN →
// gone rule; plain transport errors stay transient.
func TestClassifyFetchError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want FailureClass
	}{
		{"wall", &WallError{Method: "static", Signature: "cloudflare"}, FailureBlocked},
		{"access denied", fmt.Errorf("x: %w", ErrAccessDenied), FailureBlocked},
		{"collapse", fmt.Errorf("x: %w", ErrContentCollapsed), FailureBlocked},
		{"throttled", &ThrottledError{Method: "api"}, FailureThrottled},
		{"gone", fmt.Errorf("x: %w", ErrContentGone), FailureGone},
		{"dns nxdomain", &url.Error{Op: "Get", Err: &net.DNSError{IsNotFound: true}}, FailureGone},
		{"dns other", &url.Error{Op: "Get", Err: &net.DNSError{}}, FailureTransient},
		{"timeout", &url.Error{Op: "Get", Err: errors.New("i/o timeout")}, FailureTransient},
		{"plain", errors.New("boom"), FailureTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ClassifyFetchError(tt.err); got != tt.want {
				t.Errorf("ClassifyFetchError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// cfWall is a minimal Cloudflare interstitial — enough for the signature
// scan without shipping a real challenge fixture.
const cfWall = `<!DOCTYPE html><html><head><title>Just a moment...</title></head>` +
	`<body>Checking your connection — cloudflare</body></html>`

// TestCompositeFetch_WallEscalation covers the implicit render+wayback
// ladder extension on blocked-class primary failures: no fallback_methods
// configured, yet a walled static fetch still escalates.
func TestCompositeFetch_WallEscalation(t *testing.T) {
	t.Parallel()

	// entryURL uses the hostname form — validateRenderTarget rejects
	// private-IP literals (127.0.0.1) but "localhost" is a name, not a
	// literal, and still dials the loopback test server.
	localhostURL := func(srv *httptest.Server, p string) string {
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatalf("srv.URL parse: %v", err)
		}

		return "http://localhost:" + u.Port() + p
	}

	t.Run("render hop rescues walled static", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(cfWall))
		})
		mux.HandleFunc("/render", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"html":"<html><body><h1>real dom</h1></body></html>","final_url":"http://x/final"}`))
		})

		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		c := NewCompositeWithClient(srv.Client(), mockAdapterSettings{
			"intel.render.service_url": srv.URL,
		}, nil)

		ch := sources.Channel{EntryUrl: localhostURL(srv, "/page"), CrawlMethod: "static"}

		res, err := c.Fetch(t.Context(), sources.Source{}, ch)
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}

		if res.ViaMethod != "render" {
			t.Errorf("ViaMethod = %q, want render", res.ViaMethod)
		}

		if !strings.Contains(string(res.Body), "real dom") {
			t.Errorf("body = %q, want rendered content", res.Body)
		}
	})

	t.Run("wayback hop rescues when render unconfigured", func(t *testing.T) {
		t.Parallel()

		var gotSince atomic.Value

		mux := http.NewServeMux()
		mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(cfWall))
		})
		mux.HandleFunc("/cdx", func(w http.ResponseWriter, r *http.Request) {
			gotSince.Store(r.URL.Query().Get("since"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[["timestamp","original","statuscode"],["20260101000000","http://example.com/x","200"]]`))
		})

		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		// No intel.render.service_url → the render hop answers
		// ErrAdapterDisabled and the ladder falls through to wayback.
		c := NewCompositeWithClient(srv.Client(), mockAdapterSettings{}, nil)

		ch := sources.Channel{
			EntryUrl:       localhostURL(srv, "/page"),
			CrawlMethod:    "static",
			ProviderConfig: []byte(`{"cdx_endpoint":"` + srv.URL + `/cdx"}`),
			LastSuccessAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		}

		res, err := c.Fetch(t.Context(), sources.Source{}, ch)
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}

		if res.ViaMethod != "wayback" {
			t.Errorf("ViaMethod = %q, want wayback", res.ViaMethod)
		}

		if !strings.Contains(string(res.Body), "web.archive.org/web/20260101000000id_/") {
			t.Errorf("manifest = %q, want snapshot URL", res.Body)
		}

		// last_success_at is injected as the CDX `since` window even
		// though wayback is only an implicit hop.
		if s, _ := gotSince.Load().(string); s == "" {
			t.Error("cdx request missing since= — last_success_at was not injected")
		}
	})

	t.Run("deceptive 200 wall still classifies blocked", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(cfWall)) // status 200 — the poison case
		})
		mux.HandleFunc("/cdx", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError) // wayback hop fails too
		})

		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		c := NewCompositeWithClient(srv.Client(), mockAdapterSettings{}, nil)

		ch := sources.Channel{
			EntryUrl:       localhostURL(srv, "/page"),
			CrawlMethod:    "static",
			ProviderConfig: []byte(`{"cdx_endpoint":"` + srv.URL + `/cdx"}`),
		}

		_, err := c.Fetch(t.Context(), sources.Source{}, ch)
		if !errors.Is(err, ErrBotWalled) {
			t.Fatalf("err = %v, want ErrBotWalled (200 challenge must not pass as content)", err)
		}
	})

	t.Run("transient failure gets no escalation", func(t *testing.T) {
		t.Parallel()

		var cdxHits atomic.Int32

		mux := http.NewServeMux()
		mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		})
		mux.HandleFunc("/cdx", func(w http.ResponseWriter, _ *http.Request) {
			cdxHits.Add(1)
			w.WriteHeader(http.StatusOK)
		})

		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		c := NewCompositeWithClient(srv.Client(), mockAdapterSettings{}, nil)

		ch := sources.Channel{
			EntryUrl:       localhostURL(srv, "/page"),
			CrawlMethod:    "static",
			ProviderConfig: []byte(`{"cdx_endpoint":"` + srv.URL + `/cdx"}`),
		}

		_, err := c.Fetch(t.Context(), sources.Source{}, ch)
		if err == nil {
			t.Fatal("want error on 502")
		}

		if errors.Is(err, ErrBotWalled) {
			t.Error("502 must not classify as walled")
		}

		if n := cdxHits.Load(); n != 0 {
			t.Errorf("cdx hits = %d, want 0 — escalation is for blocked-class only", n)
		}
	})

	t.Run("gone without archive_on_gone gets no wayback hop", func(t *testing.T) {
		t.Parallel()

		var cdxHits atomic.Int32

		mux := http.NewServeMux()
		mux.HandleFunc("/feed", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		mux.HandleFunc("/cdx", func(w http.ResponseWriter, _ *http.Request) {
			cdxHits.Add(1)
			w.WriteHeader(http.StatusOK)
		})

		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		c := NewCompositeWithClient(srv.Client(), mockAdapterSettings{}, nil)

		ch := sources.Channel{
			EntryUrl:       localhostURL(srv, "/feed"),
			CrawlMethod:    "static",
			ProviderConfig: []byte(`{"cdx_endpoint":"` + srv.URL + `/cdx"}`),
		}

		_, err := c.Fetch(t.Context(), sources.Source{}, ch)
		if !errors.Is(err, ErrContentGone) {
			t.Fatalf("err = %v, want ErrContentGone", err)
		}

		if n := cdxHits.Load(); n != 0 {
			t.Errorf("cdx hits = %d, want 0 — gone-class escalation is opt-in only", n)
		}
	})

	t.Run("archive_on_gone opts into the wayback rescue", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.HandleFunc("/feed", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		mux.HandleFunc("/cdx", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[["timestamp","original","statuscode"],["20260101000000","http://example.com/x","200"]]`))
		})

		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		c := NewCompositeWithClient(srv.Client(), mockAdapterSettings{}, nil)

		ch := sources.Channel{
			EntryUrl:    localhostURL(srv, "/feed"),
			CrawlMethod: "static",
			ProviderConfig: []byte(`{"cdx_endpoint":"` + srv.URL + `/cdx",` +
				`"archive_on_gone":true}`),
		}

		res, err := c.Fetch(t.Context(), sources.Source{}, ch)
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}

		if res.ViaMethod != "wayback" {
			t.Errorf("ViaMethod = %q, want wayback", res.ViaMethod)
		}
	})
}

// TestCompositeFetch_ConditionalGET: the worker's stored validators ride
// the context into If-None-Match / If-Modified-Since, and a 304 answer
// comes back as a successful no-document result.
func TestCompositeFetch_ConditionalGET(t *testing.T) {
	t.Parallel()

	var gotETag, gotLastMod atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotETag.Store(r.Header.Get("If-None-Match"))
		gotLastMod.Store(r.Header.Get("If-Modified-Since"))

		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>page</body></html>"))
	}))
	t.Cleanup(srv.Close)

	ch := sources.Channel{EntryUrl: srv.URL + "/page", CrawlMethod: "static"}

	// No validators on context → unconditional request → 200 body.
	res, err := testComposite(srv.Client()).Fetch(t.Context(), sources.Source{}, ch)
	if err != nil || res.NotModified {
		t.Fatalf("unconditional fetch: res=%+v err=%v", res, err)
	}

	// Validators on context → headers sent → 304 → NotModified result.
	ctx := WithConditionals(t.Context(), `"v1"`, "Wed, 01 Oct 2025 00:00:00 GMT")

	res, err = testComposite(srv.Client()).Fetch(ctx, sources.Source{}, ch)
	if err != nil {
		t.Fatalf("conditional fetch: %v", err)
	}

	if e, _ := gotETag.Load().(string); e != `"v1"` {
		t.Errorf("If-None-Match sent = %q, want \"v1\"", e)
	}

	if m, _ := gotLastMod.Load().(string); m != "Wed, 01 Oct 2025 00:00:00 GMT" {
		t.Errorf("If-Modified-Since sent = %q", m)
	}

	if !res.NotModified {
		t.Errorf("NotModified = false, want true on 304 (body %d bytes)", len(res.Body))
	}
}

// TestCompositeFetch_304ThroughManifestWrappers: a 304 answer must survive
// the sitemap/jsonld/spider wrappers as NotModified — an empty body can
// never be rewritten into a document or reported as a fetch failure.
func TestCompositeFetch_304ThroughManifestWrappers(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	t.Cleanup(srv.Close)

	ctx := WithConditionals(t.Context(), `"v1"`, "")

	for _, method := range []string{"sitemap", "jsonld", "spider"} {
		ch := sources.Channel{EntryUrl: srv.URL + "/doc", CrawlMethod: method}

		res, err := testComposite(srv.Client()).Fetch(ctx, sources.Source{}, ch)
		if err != nil {
			t.Fatalf("%s: 304 fetch: %v — the wrapper must pass NotModified through", method, err)
		}

		if !res.NotModified {
			t.Errorf("%s: NotModified = false, want true", method)
		}
	}
}

// TestPreflightDomainCooloff: an upstream-imposed cool-off rejects
// preflight with ErrRateLimited (the existing upstream-backoff path) and
// does not leak across domains.
func TestPreflightDomainCooloff(t *testing.T) {
	t.Parallel()

	p := newTestChecker(t, 3600, 10)

	src := sources.Source{Domain: "example.com", TrustLevel: 1}
	ch := sources.Channel{ID: validUUID(), EntryUrl: "https://example.com/page"}

	p.SetDomainCooloff(t.Context(), "example.com", time.Minute)

	if err := p.CheckChannel(t.Context(), src, ch); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("cooled-off domain: err = %v, want ErrRateLimited", err)
	}

	// A sibling domain is unaffected.
	other := sources.Channel{ID: validUUID(), EntryUrl: "https://other.example.org/page"}
	if err := p.CheckChannel(t.Context(), src, other); err != nil {
		t.Fatalf("other domain: err = %v, want nil", err)
	}
}
