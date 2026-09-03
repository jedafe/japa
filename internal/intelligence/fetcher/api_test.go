package fetcher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// TestParseProviderConfig_Valid decodes a well-formed provider_config and
// asserts the endpoint + path are populated.
func TestParseProviderConfig_Valid(t *testing.T) {
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
	if _, err := ParseProviderConfig(nil); err == nil {
		t.Fatal("expected error for empty provider_config, got nil")
	}
}

// TestParseProviderConfig_NoEndpoint ensures a config without an endpoint
// is rejected.
func TestParseProviderConfig_NoEndpoint(t *testing.T) {
	raw := []byte(`{"path":"data"}`)
	if _, err := ParseProviderConfig(raw); err == nil {
		t.Fatal("expected error for missing endpoint, got nil")
	}
}

// TestParseProviderConfig_InvalidJSON ensures malformed JSON is rejected.
func TestParseProviderConfig_InvalidJSON(t *testing.T) {
	if _, err := ParseProviderConfig([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestParseProviderConfig_MethodAndBody decodes method and JSON body from
// provider_config.
func TestParseProviderConfig_MethodAndBody(t *testing.T) {
	raw := []byte(`{"endpoint":"https://api.example.com/grants","method":"POST","body":{"status":"posted"},"path":"data.opportunities"}`)
	cfg, err := ParseProviderConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Method != "POST" {
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
	fixture, err := os.ReadFile("testdata/scholarship_grantsgov.json")
	if err != nil {
		t.Fatal(err)
	}

	expectedBody, err := selectByPath(fixture, "data.opportunityData.opportunityDetails")
	if err != nil {
		t.Fatalf("selectByPath: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(fixture)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(APIProviderConfig{Endpoint: srv.URL, Path: "data.opportunityData.opportunityDetails"})
	f := NewAPIFetcher()
	res, err := f.Fetch(context.Background(), sources.Source{
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
	res2, err := f.Fetch(context.Background(), sources.Source{
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
	f := NewAPIFetcher()
	_, err := f.Fetch(context.Background(), sources.Source{
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
		w.Write(fixture)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(APIProviderConfig{
		Endpoint: srv.URL,
		Method:   "POST",
		Body:     postBody,
		Path:     "data.opportunityData.opportunityDetails",
	})
	f := NewAPIFetcher()
	res, err := f.Fetch(context.Background(), sources.Source{
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

	res2, err := f.Fetch(context.Background(), sources.Source{
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

// TestAPIFetcher_FetchAuthHeaders verifies that auth headers from the
// provider_config are attached to the outgoing request.
func TestAPIFetcher_FetchAuthHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(APIProviderConfig{
		Endpoint: srv.URL,
		Headers:  map[string]string{"Authorization": "Bearer test-token"},
	})
	f := NewAPIFetcher()
	res, err := f.Fetch(context.Background(), sources.Source{
		SourceUrl:      srv.URL,
		CrawlMethod:    "api",
		ProviderConfig: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if res.HTTPStatus != http.StatusOK {
		t.Errorf("status = %d, want 200 (auth header not sent)", res.HTTPStatus)
	}
}
