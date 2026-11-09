package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// external.fetch — handler dispatch, argument validation, caching
// ---------------------------------------------------------------------------

func TestExternalFetchHandler_NoDeps(t *testing.T) {
	t.Parallel()

	handler := ExternalFetchHandler(nil)

	tr, _ := handler(t.Context(), json.RawMessage(`{"url":"https://example.com"}`), pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "external_fetch_unavailable" {
		t.Errorf("expected code = external_fetch_unavailable, got %v", m["code"])
	}
}

func TestExternalFetchHandler_MissingURL(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Settings: &stubSettingsReader{}}
	handler := ExternalFetchHandler(deps)

	tr, _ := handler(t.Context(), json.RawMessage(`{}`), pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "missing_url" {
		t.Errorf("expected code = missing_url, got %v", m["code"])
	}
}

func TestExternalFetchHandler_FetchWebFuncResult(t *testing.T) {
	t.Parallel()

	var gotURL string
	var gotMaxChars int
	deps := &ToolDeps{
		Settings: &stubSettingsReader{},
		FetchWebFunc: func(ctx context.Context, client *http.Client, settings SettingsReader, rawURL string, maxChars int) (*FetchWebResult, error) {
			gotURL = rawURL
			gotMaxChars = maxChars

			return &FetchWebResult{
				URL:         rawURL,
				FinalURL:    rawURL,
				Status:      200,
				ContentType: "text/html; charset=utf-8",
				Title:       "Example",
				Content:     "Hello world",
			}, nil
		},
	}
	handler := ExternalFetchHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"url":" https://unilag.edu.ng/ "}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if gotURL != "https://unilag.edu.ng/" {
		t.Errorf("url was not trimmed: %q", gotURL)
	}
	if gotMaxChars != defaultFetchMaxChars {
		t.Errorf("maxChars = %d, want default %d", gotMaxChars, defaultFetchMaxChars)
	}

	var res FetchWebResult
	if err := json.Unmarshal([]byte(tr.Content), &res); err != nil {
		t.Fatalf("parsing result content: %v", err)
	}
	if res.Title != "Example" || res.Content != "Hello world" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestExternalFetchHandler_MaxCharsArg(t *testing.T) {
	t.Parallel()

	var gotMaxChars int
	deps := &ToolDeps{
		Settings: &stubSettingsReader{},
		FetchWebFunc: func(ctx context.Context, client *http.Client, settings SettingsReader, rawURL string, maxChars int) (*FetchWebResult, error) {
			gotMaxChars = maxChars
			return &FetchWebResult{URL: rawURL, Status: 200, Content: "x"}, nil
		},
	}
	handler := ExternalFetchHandler(deps)

	// Over-ceiling max_chars is clamped.
	if _, err := handler(t.Context(), json.RawMessage(`{"url":"https://example.com","max_chars":999999}`), pgtype.UUID{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMaxChars != fetchToolMaxCharsCeiling {
		t.Errorf("maxChars = %d, want ceiling %d", gotMaxChars, fetchToolMaxCharsCeiling)
	}
}

func TestExternalFetchHandler_FetchError(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		Settings: &stubSettingsReader{},
		FetchWebFunc: func(ctx context.Context, client *http.Client, settings SettingsReader, rawURL string, maxChars int) (*FetchWebResult, error) {
			return nil, errors.New("connection refused")
		},
	}
	handler := ExternalFetchHandler(deps)

	tr, _ := handler(t.Context(), json.RawMessage(`{"url":"https://example.com"}`), pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "external_fetch_failed" {
		t.Errorf("expected code = external_fetch_failed, got %v", m["code"])
	}
}

func TestExternalFetchHandler_InvalidURLError(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		Settings: &stubSettingsReader{},
		FetchWebFunc: func(ctx context.Context, client *http.Client, settings SettingsReader, rawURL string, maxChars int) (*FetchWebResult, error) {
			return nil, errExternalFetchInvalidURL
		},
	}
	handler := ExternalFetchHandler(deps)

	tr, _ := handler(t.Context(), json.RawMessage(`{"url":"http://169.254.169.254/"}`), pgtype.UUID{})

	m := parseToolResult(t, tr)
	if m["code"] != "invalid_url" {
		t.Errorf("expected code = invalid_url, got %v", m["code"])
	}
}

func TestExternalFetchHandler_CacheHit(t *testing.T) {
	t.Parallel()

	cache := newStubCacheClient()
	cachedJSON := `{"url":"https://example.com","final_url":"https://example.com","status":200,"content_type":"text/html","content":"cached","truncated":false,"fetched_at":"2026-01-01T00:00:00Z"}`
	cache.store[externalFetchCacheKey("https://example.com", defaultFetchMaxChars)] = cachedJSON

	called := false
	deps := &ToolDeps{
		Settings: &stubSettingsReader{},
		Cache:    cache,
		FetchWebFunc: func(ctx context.Context, client *http.Client, settings SettingsReader, rawURL string, maxChars int) (*FetchWebResult, error) {
			called = true
			return nil, errors.New("should not be called")
		},
	}
	handler := ExternalFetchHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"url":"https://example.com"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("FetchWebFunc should not be called on cache hit")
	}
	if tr.Content != cachedJSON {
		t.Error("expected cached content to be returned directly")
	}
}

func TestExternalFetchHandler_CacheWrite(t *testing.T) {
	t.Parallel()

	cache := newStubCacheClient()
	deps := &ToolDeps{
		Settings: &stubSettingsReader{},
		Cache:    cache,
		FetchWebFunc: func(ctx context.Context, client *http.Client, settings SettingsReader, rawURL string, maxChars int) (*FetchWebResult, error) {
			return &FetchWebResult{URL: rawURL, Status: 200, Content: "fresh"}, nil
		},
	}
	handler := ExternalFetchHandler(deps)

	if _, err := handler(t.Context(), json.RawMessage(`{"url":"https://example.com"}`), pgtype.UUID{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cache.store[externalFetchCacheKey("https://example.com", defaultFetchMaxChars)]; !ok {
		t.Error("expected result to be written to cache")
	}
}

// ---------------------------------------------------------------------------
// FetchWeb — validation paths (no network needed; private IPs and bad
// schemes are rejected before any request goes out)
// ---------------------------------------------------------------------------

func TestFetchWeb_RejectsPrivateIP(t *testing.T) {
	t.Parallel()

	for _, u := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://127.0.0.1:8080/admin",
		"http://10.0.0.1/internal",
		"http://192.168.1.1/",
	} {
		_, err := FetchWeb(t.Context(), nil, nil, u, 1000)
		if err == nil {
			t.Errorf("expected %q to be rejected", u)
			continue
		}
		if !errors.Is(err, errExternalFetchInvalidURL) {
			t.Errorf("%q: expected errExternalFetchInvalidURL, got %v", u, err)
		}
	}
}

func TestFetchWeb_RejectsUserinfoURL(t *testing.T) {
	t.Parallel()

	// Embedded credentials must be rejected before any DNS/network activity —
	// .invalid never resolves, so a pass here proves the check runs first.
	for _, u := range []string{
		"https://user:pass@unresolvable.invalid/",
		"https://token@169.254.169.254/",
		"http://admin:hunter2@10.0.0.1/internal",
	} {
		_, err := FetchWeb(t.Context(), nil, nil, u, 1000)
		if err == nil {
			t.Errorf("expected %q to be rejected", u)
			continue
		}
		if !errors.Is(err, errExternalFetchInvalidURL) {
			t.Errorf("%q: expected errExternalFetchInvalidURL, got %v", u, err)
		}
	}
}

func TestFetchWeb_RejectsNonHTTPScheme(t *testing.T) {
	t.Parallel()

	for _, u := range []string{
		"ftp://example.com/file",
		"file:///etc/passwd",
		"gopher://example.com",
	} {
		_, err := FetchWeb(t.Context(), nil, nil, u, 1000)
		if err == nil {
			t.Errorf("expected %q to be rejected", u)
			continue
		}
		if !errors.Is(err, errExternalFetchInvalidURL) {
			t.Errorf("%q: expected errExternalFetchInvalidURL, got %v", u, err)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func TestExternalFetchCacheKey(t *testing.T) {
	t.Parallel()

	key1 := externalFetchCacheKey("https://example.com/page", 16000)
	if externalFetchCacheKey("https://example.com/page", 16000) != key1 {
		t.Error("same URL+maxChars should produce same key")
	}
	if externalFetchCacheKey("https://example.com/other", 16000) == key1 {
		t.Error("different URL should produce different key")
	}
	if externalFetchCacheKey("https://example.com/page", 60000) == key1 {
		t.Error("different maxChars should produce different key")
	}
	// Scheme/host case differences normalize to the same key.
	if externalFetchCacheKey("HTTPS://EXAMPLE.COM/page", 16000) != key1 {
		t.Error("scheme/host case should be normalized")
	}
	// Fragments never reach the server — they must not fragment the cache.
	if externalFetchCacheKey("https://example.com/page#section", 16000) != key1 {
		t.Error("URL fragment should be ignored in the cache key")
	}
}

func TestFetchableContentType(t *testing.T) {
	t.Parallel()

	for _, ct := range []string{
		"text/html; charset=utf-8",
		"application/xhtml+xml",
		"application/json",
		"application/rss+xml",
		"text/plain",
		"", // no header → allowed through textprep sniffing
	} {
		if !fetchableContentType(ct) {
			t.Errorf("expected %q to be fetchable", ct)
		}
	}

	for _, ct := range []string{
		"application/pdf",
		"image/png",
		"application/octet-stream",
		"application/zip",
	} {
		if fetchableContentType(ct) {
			t.Errorf("expected %q to be rejected", ct)
		}
	}
}
