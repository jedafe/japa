package fetcher

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// TestParseProviderConfig_Valid decodes a well-formed provider_config and
// asserts the endpoint + path are populated.
func TestParseProviderConfig_Valid(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"endpoint":"https://api.example.com/grants","path":"data.opportunities"}`)

	cfg, err := ParseProviderConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Endpoint != "https://api.example.com/grants" {
		t.Errorf("endpoint = %q, want https://api.example.com/grants", cfg.Endpoint)
	}

	if cfg.Path != "data.opportunities" {
		t.Errorf("path = %q, want data.opportunities", cfg.Path)
	}
}

// TestParseProviderConfig_Empty ensures an empty config is rejected.
func TestParseProviderConfig_Empty(t *testing.T) {
	t.Parallel()

	if _, err := ParseProviderConfig(nil); err == nil {
		t.Fatal("expected error for empty provider_config, got nil")
	}
}

// TestParseProviderConfig_NoEndpoint ensures a config without an endpoint
// is rejected.
func TestParseProviderConfig_NoEndpoint(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"path":"data"}`)
	if _, err := ParseProviderConfig(raw); err == nil {
		t.Fatal("expected error for missing endpoint, got nil")
	}
}

// TestParseProviderConfig_InvalidJSON ensures malformed JSON is rejected.
func TestParseProviderConfig_InvalidJSON(t *testing.T) {
	t.Parallel()

	if _, err := ParseProviderConfig([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestParseProviderConfig_MethodAndBody decodes method and JSON body from
// provider_config.
func TestParseProviderConfig_MethodAndBody(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"endpoint":"https://api.example.com/grants","method":"POST","body":{"status":"posted"},"path":"data.opportunities"}`)

	cfg, err := ParseProviderConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", cfg.Method)
	}

	if string(cfg.Body) != `{"status":"posted"}` {
		t.Errorf("body = %q, want {\"status\":\"posted\"}", cfg.Body)
	}

	if cfg.Path != "data.opportunities" {
		t.Errorf("path = %q, want data.opportunities", cfg.Path)
	}
}

// TestAPIFetcher_Fetch exercises the full fetch path against a test server
// that returns the Grants.gov fixture, extracting the opportunityDetails
// array and verifying the body + content hash are stable.
func TestAPIFetcher_Fetch(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/scholarship_grantsgov.json")
	if err != nil {
		t.Fatal(err)
	}

	expectedBody, err := selectByPath(fixture, "data.opportunityData.opportunityDetails")
	if err != nil {
		t.Fatalf("selectByPath: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(APIProviderConfig{Endpoint: srv.URL, Path: "data.opportunityData.opportunityDetails"})
	// httptest binds 127.0.0.1 — inject its client (mirroring rss_test.go)
	// since NewAPIFetcher's SSRF dial guard would reject the loopback dial.
	f := &APIFetcher{client: srv.Client(), allowPrivate: true}

	res, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      srv.URL,
		CrawlMethod:    "api",
		ProviderConfig: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if res.HTTPStatus != http.StatusOK {
		t.Errorf("status = %d, want %d", res.HTTPStatus, http.StatusOK)
	}

	if string(res.Body) != string(expectedBody) {
		t.Errorf("body does not match extracted path\ngot:  %s\nwant: %s", res.Body, expectedBody)
	}

	if res.ContentHash == "" {
		t.Error("content hash is empty")
	}

	// Hash should be stable for the same body.
	res2, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      srv.URL,
		CrawlMethod:    "api",
		ProviderConfig: cfg,
	})
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	if res.ContentHash != res2.ContentHash {
		t.Errorf("hash not stable: %q != %q", res.ContentHash, res2.ContentHash)
	}
}

// TestAPIFetcher_FetchBadConfig ensures a missing provider_config surfaces
// an error rather than panicking.
func TestAPIFetcher_FetchBadConfig(t *testing.T) {
	t.Parallel()

	f := NewAPIFetcher()

	_, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      "https://example.com",
		CrawlMethod:    "api",
		ProviderConfig: nil,
	})
	if err == nil {
		t.Fatal("expected error for missing provider_config, got nil")
	}
}

// TestAPIFetcher_FetchPOST verifies POST requests with a JSON body, path
// extraction, and a stable content hash.
func TestAPIFetcher_FetchPOST(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/scholarship_grantsgov.json")
	if err != nil {
		t.Fatal(err)
	}

	expectedBody, err := selectByPath(fixture, "data.opportunityData.opportunityDetails")
	if err != nil {
		t.Fatalf("selectByPath: %v", err)
	}

	postBody := json.RawMessage(`{"searchFields":{"oppStatuses":"posted"}}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}

		gotBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}

		if string(gotBody) != string(postBody) {
			t.Errorf("request body = %q, want %q", gotBody, postBody)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(APIProviderConfig{
		Endpoint: srv.URL,
		Method:   "POST",
		Body:     postBody,
		Path:     "data.opportunityData.opportunityDetails",
	})
	f := &APIFetcher{client: srv.Client(), allowPrivate: true}

	res, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      srv.URL,
		CrawlMethod:    "api",
		ProviderConfig: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if res.HTTPStatus != http.StatusOK {
		t.Errorf("status = %d, want %d", res.HTTPStatus, http.StatusOK)
	}

	if string(res.Body) != string(expectedBody) {
		t.Errorf("body does not match extracted path\ngot:  %s\nwant: %s", res.Body, expectedBody)
	}

	if res.ContentHash == "" {
		t.Error("content hash is empty")
	}

	res2, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      srv.URL,
		CrawlMethod:    "api",
		ProviderConfig: cfg,
	})
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	if res.ContentHash != res2.ContentHash {
		t.Errorf("hash not stable: %q != %q", res.ContentHash, res2.ContentHash)
	}
}

// TestAPIFetcher_FetchPOST_StringBody covers the xhr_sniff replay contract:
// provider_config.body stored as a JSON string is sent UNQUOTED (the raw
// bytes), a configured Content-Type header overrides the JSON default, and
// a non-JSON response body is preserved verbatim with its content type.
func TestAPIFetcher_FetchPOST_StringBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}

		if string(got) != "q=visa&page=2" {
			t.Errorf("request body = %q, want unquoted q=visa&page=2", got)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("plain result, not json"))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(APIProviderConfig{
		Endpoint: srv.URL,
		Method:   "POST",
		Body:     json.RawMessage(`"q=visa&page=2"`),
		Headers:  map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
	})
	f := &APIFetcher{client: srv.Client(), allowPrivate: true}

	res, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      srv.URL,
		CrawlMethod:    "api",
		ProviderConfig: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if string(res.Body) != "plain result, not json" {
		t.Errorf("body = %q, want verbatim non-JSON response", res.Body)
	}

	if res.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", res.ContentType)
	}
}

// TestAPIFetcher_FetchPOST_JSONBodyUnchanged pins the object-body path — a
// JSON object body is sent verbatim with the application/json default.
func TestAPIFetcher_FetchPOST_JSONBodyUnchanged(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		if string(got) != `{"status":"posted"}` {
			t.Errorf("request body = %q, want verbatim JSON", got)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json default", ct)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(APIProviderConfig{
		Endpoint: srv.URL,
		Method:   "POST",
		Body:     json.RawMessage(`{"status":"posted"}`),
	})
	f := &APIFetcher{client: srv.Client(), allowPrivate: true}

	if _, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      srv.URL,
		CrawlMethod:    "api",
		ProviderConfig: cfg,
	}); err != nil {
		t.Fatalf("fetch: %v", err)
	}
}

// TestAPIFetcher_FetchAuthHeaders verifies that sensitive headers
// (Authorization, Host, proxy/forwarding headers) from provider_config are
// STRIPPED and never attached to the outgoing request — a channel override
// must not smuggle credentials or reroute the fetch. Benign custom headers
// (X-Source) still pass through.
func TestAPIFetcher_FetchAuthHeaders(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Header.Get("X-Source") != "intel" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(APIProviderConfig{
		Endpoint: srv.URL,
		Headers:  map[string]string{"Authorization": "Bearer test-token", "X-Source": "intel"},
	})
	f := &APIFetcher{client: srv.Client(), allowPrivate: true}

	res, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      srv.URL,
		CrawlMethod:    "api",
		ProviderConfig: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if res.HTTPStatus != http.StatusOK {
		t.Errorf("status = %d, want 200 (Authorization stripped, X-Source passed)", res.HTTPStatus)
	}
}

// TestAPIFetcher_ErrorStatus: non-2xx responses are fetch failures — an
// endpoint that 500s must not have its error body stored as content, and
// the failure must surface so consecutive_failures and the fallback ladder
// engage.
func TestAPIFetcher_ErrorStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	t.Cleanup(srv.Close)

	cfg, _ := json.Marshal(APIProviderConfig{Endpoint: srv.URL})
	f := &APIFetcher{client: srv.Client(), allowPrivate: true}

	_, err := f.Fetch(t.Context(), sources.Source{
		SourceUrl:      srv.URL,
		CrawlMethod:    "api",
		ProviderConfig: cfg,
	})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, want status-500 fetch error", err)
	}
}
