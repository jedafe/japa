package atproto

// A6-4 proxy tests — the app.bsky read proxy's security contract:
// GET-only, fixed upstream, no credential forwarding, no redirect
// following, no path escape; repo.* is always 501.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (A6-4)

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func proxyHandlers(t *testing.T, upstreamURL string) *Handlers {
	t.Helper()
	d := testDB(t)
	svc := newAtprotoService(t, d)
	svc.cfg.AppViewURL = upstreamURL
	return NewHandlers(svc)
}

func TestProxyRejectsNonGET(t *testing.T) {
	h := proxyHandlers(t, "http://upstream.invalid")
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ProxyAppView(rec, httptest.NewRequest(m, "/xrpc/app.bsky.actor.putPreferences", nil))
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("%s status = %d, want 501", m, rec.Code)
		}
	}
}

func TestProxyRepoAlways501(t *testing.T) {
	h := proxyHandlers(t, "http://upstream.invalid")
	// GET repo.getRecord is a query, not a write — still 501: the
	// namespace is PDS-local, never proxied upstream.
	for _, m := range []string{http.MethodGet, http.MethodPost} {
		rec := httptest.NewRecorder()
		h.ProxyRepo(rec, httptest.NewRequest(m, "/xrpc/com.atproto.repo.getRecord", nil))
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("%s repo.* status = %d, want 501", m, rec.Code)
		}
	}
}

func TestProxyGetForwardsAndStripsCredentials(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	h := proxyHandlers(t, upstream.URL)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/xrpc/app.bsky.actor.getProfile?actor=alice.test", nil)
	req.Header.Set("Authorization", "Bearer member-secret-jwt")
	h.ProxyAppView(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if gotAuth != "" {
		t.Fatalf("member credential leaked upstream: %q", gotAuth)
	}
	if gotPath != "/xrpc/app.bsky.actor.getProfile" || gotQuery != "actor=alice.test" {
		t.Fatalf("upstream got %s?%s", gotPath, gotQuery)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestProxyDoesNotFollowRedirects(t *testing.T) {
	// A 302 to an arbitrary origin must come back as a mirrored 302 —
	// never fetched. The redirect target deliberately fails if hit.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect target was fetched — SSRF")
	}))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/evil", http.StatusFound)
	}))
	defer upstream.Close()

	h := proxyHandlers(t, upstream.URL)
	rec := httptest.NewRecorder()
	h.ProxyAppView(rec, httptest.NewRequest(http.MethodGet, "/xrpc/app.bsky.actor.getProfile", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want mirrored 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != target.URL+"/evil" {
		t.Fatalf("location = %q", loc)
	}
}

func TestProxyRejectsPathTraversal(t *testing.T) {
	h := proxyHandlers(t, "http://upstream.invalid")
	rec := httptest.NewRecorder()
	h.ProxyAppView(rec, httptest.NewRequest(http.MethodGet, "/xrpc/app.bsky.foo/../admin", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestProxyUpstreamDown502(t *testing.T) {
	h := proxyHandlers(t, "http://127.0.0.1:1") // nothing listens
	rec := httptest.NewRecorder()
	h.ProxyAppView(rec, httptest.NewRequest(http.MethodGet, "/xrpc/app.bsky.actor.getProfile", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}
