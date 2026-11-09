// Package ai — tools_fetch.go implements external.fetch: an on-demand,
// SSRF-safe web page fetch tool for the agent catalog (§103). The LLM calls
// it to read a specific URL — a page the user pasted, an official website,
// or a result URL returned by external.search — so answers are grounded in
// the fetched document rather than model memory.
//
// Security: the URL is validated through linkpreview.ValidateURL (scheme
// allowlist, private/reserved IP rejection) and fetched through
// linkpreview's SSRF-safe HTTP client, whose dialer re-checks the resolved
// IP at TCP connect time (TOCTOU / DNS-rebinding protection) and
// re-validates every redirect hop. Response bodies are size-capped and
// reduced to visible text via textprep.SourceText before being returned to
// the model.
//
// Config (system_settings, Rule 3 — hot-reloadable):
//
//	agent.external_fetch_timeout_seconds   — per-fetch deadline (default 15s)
//	agent.external_fetch_max_bytes         — response body cap (default 2 MB)
//	agent.external_fetch_max_chars         — output text cap (default 16000)
//	agent.external_fetch_cache_ttl_seconds — jc:agent:fetch:{hash} TTL (default 600, 0 disables)
//
// Ref: .jeos/sections/103-ai-agent-executor.md
package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/intelligence/textprep"
	"github.com/jedafe/japa/internal/linkpreview"
	"github.com/jedafe/japa/internal/valkey"
)

// externalFetchArgs is the LLM-produced argument shape for external.fetch.
type externalFetchArgs struct {
	URL      string `json:"url"`
	MaxChars int    `json:"max_chars,omitempty"`
}

// FetchWebResult is what external.fetch returns to the model: the page's
// final URL (after redirects), HTTP status, content type, extracted title,
// visible-text content, and whether the output was truncated.
type FetchWebResult struct {
	URL         string `json:"url"`
	FinalURL    string `json:"final_url"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Title       string `json:"title,omitempty"`
	Content     string `json:"content"`
	Truncated   bool   `json:"truncated"`
	FetchedAt   string `json:"fetched_at"`
}

// Compiled-in defaults applied when the settings reader is nil or a key is
// unset (Rule 3 — the system_settings values are the operator knobs).
const (
	defaultFetchTimeoutSeconds  = 15
	defaultFetchMaxBytes        = 2 << 20 // 2 MB
	defaultFetchMaxChars        = 16000   // ~4k tokens of page text
	defaultFetchCacheTTLSeconds = 600
	fetchToolMaxCharsCeiling    = 60000
	fetchToolUserAgent          = "Mozilla/5.0 (compatible; JapaConnect-Agent/1.0; +https://japa.to)"
)

var (
	errExternalFetchInvalidURL   = errors.New("invalid or disallowed URL")
	errUnsupportedFetchMediaType = errors.New("unsupported content type")
)

// ExternalFetchHandler returns a ToolHandler that fetches a single web page
// on demand and returns its visible text to the model. It complements
// external.search (which returns snippets) for the "go read this URL" case —
// e.g. pulling current data from an official site before updating a page.
//
// The handler requires the agent.external_fetch RBAC permission, checked by
// the executor's RBAC gate before dispatch; the handler itself performs the
// SSRF/URL validation and fetch. Results are cached in jc:agent:fetch:{hash}
// per the Valkey keys catalog (Rule 4).
func ExternalFetchHandler(deps *ToolDeps) ToolHandler {
	// The SSRF-safe client is constructed once per handler and shared across
	// calls — its dialer blocks private IPs at connect time and its redirect
	// checker re-validates every hop.
	client := linkpreview.NewSafeHTTPClient()

	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil {
			return errorToolResult("external_fetch_unavailable", "tool deps not configured"), nil
		}

		var a externalFetchArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		a.URL = strings.TrimSpace(a.URL)
		if a.URL == "" {
			return errorToolResult("missing_url", "url is required"), nil
		}

		maxChars := a.MaxChars
		if maxChars <= 0 {
			maxChars = fetchIntSetting(ctx, deps.Settings, "agent.external_fetch_max_chars", defaultFetchMaxChars)
		}
		if maxChars > fetchToolMaxCharsCeiling {
			maxChars = fetchToolMaxCharsCeiling
		}

		cacheTTL := fetchIntSetting(ctx, deps.Settings, "agent.external_fetch_cache_ttl_seconds", defaultFetchCacheTTLSeconds)
		cacheKey := externalFetchCacheKey(a.URL)

		if deps.Cache != nil && cacheTTL > 0 {
			if cached, err := deps.Cache.Get(ctx, cacheKey); err == nil && cached != "" {
				return &ToolResult{Status: ToolCallStatusSuccess, Content: cached}, nil
			}
		}

		fetchFn := FetchWeb
		if deps.FetchWebFunc != nil {
			fetchFn = deps.FetchWebFunc
		}

		result, err := fetchFn(ctx, client, deps.Settings, a.URL, maxChars)
		if err != nil {
			slog.Warn("ai: external fetch failed", "error", err, "url", a.URL)

			switch {
			case errors.Is(err, errExternalFetchInvalidURL):
				return errorToolResult("invalid_url", err.Error()), nil
			case errors.Is(err, errUnsupportedFetchMediaType):
				return errorToolResult("unsupported_content_type", err.Error()), nil
			default:
				return errorToolResult("external_fetch_failed", err.Error()), nil
			}
		}

		respJSON, err := json.Marshal(result)
		if err != nil {
			return errorToolResult("external_fetch_failed", fmt.Sprintf("marshal result: %v", err)), nil
		}

		if deps.Cache != nil && cacheTTL > 0 {
			if cerr := deps.Cache.Set(ctx, cacheKey, string(respJSON), cacheTTL); cerr != nil {
				slog.Warn("ai: failed to cache external fetch result", "error", cerr, "url", a.URL)
			}
		}

		return &ToolResult{Status: ToolCallStatusSuccess, Content: string(respJSON)}, nil
	}
}

// FetchWeb fetches rawURL and returns its visible-text content. Exported so
// non-agent callers (and tests via ToolDeps.FetchWebFunc) can reuse the same
// validated fetch path — parallel to SearchWeb.
//
// The caller must pass an SSRF-safe client (linkpreview.NewSafeHTTPClient);
// the function additionally re-runs linkpreview.ValidateURL up front so a
// misconfigured caller still cannot reach internal addresses.
func FetchWeb(ctx context.Context, client *http.Client, settings SettingsReader, rawURL string, maxChars int) (*FetchWebResult, error) {
	if client == nil {
		client = linkpreview.NewSafeHTTPClient()
	}

	if maxChars <= 0 {
		maxChars = fetchIntSetting(ctx, settings, "agent.external_fetch_max_chars", defaultFetchMaxChars)
	}
	if maxChars > fetchToolMaxCharsCeiling {
		maxChars = fetchToolMaxCharsCeiling
	}

	// Up-front validation: scheme allowlist + private/reserved IP rejection.
	// The safe client's dialer repeats the check at connect time to close
	// the DNS-rebinding window.
	if err := linkpreview.ValidateURL(rawURL); err != nil {
		return nil, fmt.Errorf("%w: %v", errExternalFetchInvalidURL, err)
	}

	timeout := fetchIntSetting(ctx, settings, "agent.external_fetch_timeout_seconds", defaultFetchTimeoutSeconds)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// #nosec G704 -- rawURL validated by linkpreview.ValidateURL above and
	// the safe client's dialer/redirect checks guard connect-time egress.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errExternalFetchInvalidURL, err)
	}
	req.Header.Set("User-Agent", fetchToolUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,application/xml,text/plain;q=0.9,*/*;q=0.1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}

	contentType := resp.Header.Get("Content-Type")
	if !fetchableContentType(contentType) {
		return nil, fmt.Errorf("%w: %s", errUnsupportedFetchMediaType, contentType)
	}

	maxBytes := int64(fetchIntSetting(ctx, settings, "agent.external_fetch_max_bytes", defaultFetchMaxBytes))
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	// Ask for maxChars+1 runes so truncation is detected exactly rather than
	// guessed from output length.
	text := textprep.SourceText(string(body), contentType, maxChars+1)
	truncated := false
	if runes := []rune(text); len(runes) > maxChars {
		text = string(runes[:maxChars])
		truncated = true
	}

	return &FetchWebResult{
		URL:         rawURL,
		FinalURL:    finalURL,
		Status:      resp.StatusCode,
		ContentType: contentType,
		Title:       htmlTitle(string(body), contentType),
		Content:     text,
		Truncated:   truncated,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// fetchableContentType reports whether a response body can be reduced to
// text for the model. HTML, plain text, JSON, XML/RSS/Atom, and CSV pass;
// binary media (PDF, images, archives, octet-stream) is rejected — the model
// cannot read them and they only waste the body cap.
func fetchableContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	if ct == "" {
		return true // no header — sniff through textprep instead of rejecting
	}

	return strings.Contains(ct, "html") ||
		strings.Contains(ct, "text/") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "xml") ||
		strings.Contains(ct, "csv") ||
		strings.Contains(ct, "javascript")
}

// htmlTitle extracts the <title> text from an HTML body; empty for non-HTML.
func htmlTitle(body, contentType string) string {
	if !strings.Contains(strings.ToLower(contentType), "html") &&
		!strings.HasPrefix(strings.TrimSpace(body), "<") {
		return ""
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return ""
	}

	return strings.TrimSpace(doc.Find("title").First().Text())
}

// fetchIntSetting resolves an int setting with a fallback; a nil settings
// reader yields the compiled-in default.
func fetchIntSetting(ctx context.Context, settings SettingsReader, key string, fallback int) int {
	if settings == nil {
		return fallback
	}

	return settings.GetIntOr(ctx, key, fallback)
}

// externalFetchCacheKey builds jc:agent:fetch:{hash} where hash is the
// SHA-256 of the platform ID + normalized URL (Rule 4 — catalog key
// jc:agent:fetch:{hash}).
func externalFetchCacheKey(rawURL string) string {
	h := sha256.Sum256([]byte(":" + normalizeFetchURL(rawURL)))
	return valkey.MustKey("agent:fetch:%s", hex.EncodeToString(h[:16]))
}

// normalizeFetchURL lowercases the scheme+host so equivalent URLs share one
// cache entry; the path and query are left case-sensitive.
func normalizeFetchURL(rawURL string) string {
	rest := rawURL
	scheme := ""
	if i := strings.Index(rawURL, "://"); i >= 0 {
		scheme = strings.ToLower(rawURL[:i]) + "://"
		rest = rawURL[i+3:]
	}

	host := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		host = rest[:i]
		rest = rest[i:]
	} else {
		rest = ""
	}

	return scheme + strings.ToLower(host) + rest
}
