package fetcher

import (
	"context"
	"encoding/json"
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

// TestAPIFetcher_Fetch exercises the full fetch path against a test server
// that returns the Grants.gov fixture, verifying the body + hash are set.
func TestAPIFetcher_Fetch(t *testing.T) {
	fixture, err := os.ReadFile("testdata/scholarship_grantsgov.json")
	if err != nil {
		t.Fatal(err)
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
		SourceUrl:       srv.URL,
		CrawlMethod:     "api",
		ProviderConfig:  cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if res.HTTPStatus != http.StatusOK {
		t.Errorf("status = %d, want %d", res.HTTPStatus, http.StatusOK)
	}
	if string(res.Body) != string(fixture) {
		t.Error("body does not match fixture")
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
