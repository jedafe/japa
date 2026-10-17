package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// APIProviderConfig is the JSON schema for source.provider_config when
// crawl_method = "api". It carries the endpoint URL, request method/body,
// an optional JSON path pointing at the list of records within the response,
// and optional auth headers to attach to the request.
type APIProviderConfig struct {
	Endpoint string            `json:"endpoint"`
	Method   string            `json:"method,omitempty"`
	Body     json.RawMessage   `json:"body,omitempty"`
	Path     string            `json:"path,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// selectByPath extracts a nested JSON value at dot-separated path.
// An empty path returns the original data unchanged.
func selectByPath(data []byte, path string) ([]byte, error) {
	if path == "" {
		return data, nil
	}

	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("api: unmarshal body for path selection: %w", err)
	}

	parts := strings.Split(path, ".")
	for _, p := range parts {
		switch v := root.(type) {
		case map[string]any:
			next, ok := v[p]
			if !ok {
				return nil, fmt.Errorf("api: path %q not found at segment %q", path, p)
			}

			root = next
		default:
			return nil, fmt.Errorf("api: cannot traverse path %q at segment %q", path, p)
		}
	}

	return json.Marshal(root)
}

// APIFetcher is a generic JSON API client. It reads the endpoint + response
// path from source.provider_config, issues an HTTP GET/POST request as
// configured, and returns the raw JSON body with a content hash. Downstream
// stages (extractor) walk Path to pull individual records.
type APIFetcher struct {
	client *http.Client
	// allowPrivate disables the insert-time private-IP guard for
	// httptest loopback servers. Production wiring never sets it —
	// the SafeClient dial guard stays on regardless.
	allowPrivate bool
}

// NewAPIFetcher returns an APIFetcher with a 30s timeout and SSRF-safe
// transport (redirects to private IPs are blocked; response bodies are
// capped at defaultMaxBodySize by the Fetch method).
func NewAPIFetcher() *APIFetcher {
	return &APIFetcher{client: NewSafeClient(30*time.Second, defaultMaxBodySize)}
}

// NewAPIFetcherWithClient injects a custom client (tests). Passing a
// non-nil client also relaxes the insert-time private-IP guard so
// httptest loopback URLs validate — the dial guard belongs to the
// injected client in that case.
func NewAPIFetcherWithClient(client *http.Client) *APIFetcher {
	if client == nil {
		return NewAPIFetcher()
	}

	return &APIFetcher{client: client, allowPrivate: true}
}

// ParseProviderConfig decodes source.provider_config into APIProviderConfig.
// An empty config is an error — an API source must specify an endpoint.
func ParseProviderConfig(raw []byte) (APIProviderConfig, error) {
	var cfg APIProviderConfig
	if len(raw) == 0 {
		return cfg, errors.New("api: provider_config is empty")
	}

	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("api: parse provider_config: %w", err)
	}

	if cfg.Endpoint == "" {
		return cfg, errors.New("api: provider_config.endpoint is empty")
	}

	return cfg, nil
}

// Fetch implements the Fetcher interface for JSON API sources.
func (f *APIFetcher) Fetch(ctx context.Context, source sources.Source) (FetchResult, error) {
	cfg, err := ParseProviderConfig(source.ProviderConfig)
	if err != nil {
		return FetchResult{}, err
	}

	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method == "" {
		method = http.MethodGet
	}

	// CONNECT/TRACE/TRACK let provider_config turn the fetcher into an
	// arbitrary proxy primitive — restrict to data-retrieval verbs.
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
	default:
		return FetchResult{}, fmt.Errorf("api: method %q not allowed", method)
	}

	if err := f.validateAPIEndpoint(cfg.Endpoint); err != nil {
		return FetchResult{}, err
	}

	// provider_config.body is a JSON-encoded payload: a JSON string body is
	// sent as its raw content (unquoted — lets a form-encoded or text POST
	// be expressed), while objects/arrays/numbers are sent verbatim.
	var bodyBytes []byte
	if len(cfg.Body) > 0 {
		bodyBytes = cfg.Body

		var asString string

		if err := json.Unmarshal(cfg.Body, &asString); err == nil {
			bodyBytes = []byte(asString)
		}
	}

	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, cfg.Endpoint, bodyReader)
	if err != nil {
		return FetchResult{}, fmt.Errorf("api: new request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect-Intelligence/1.0 (+https://japa.to)")
	req.Header.Set("Accept", "application/json")

	if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range cfg.Headers {
		// Host/Authorization/proxy headers from provider_config would let
		// a channel override smuggle credentials or reroute the request —
		// the fetcher sets its own User-Agent/Accept, nothing else.
		lowered := strings.ToLower(strings.TrimSpace(k))
		switch lowered {
		case "host", "authorization", "proxy-authorization", "x-forwarded-for", "x-forwarded-host", "x-real-ip", "forwarded":
			continue
		}

		if lowered == "" || strings.ContainsAny(lowered, " :\r\n") {
			continue
		}

		req.Header.Set(k, v)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("api: do: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	// Error responses are fetch failures, not documents — matching the
	// rss/social/wayback/render convention. Without the check an endpoint
	// that 500s stores the error body as content, resets
	// consecutive_failures, and never enters the fallback ladder.
	if resp.StatusCode >= 400 {
		return FetchResult{}, fmt.Errorf("api: unexpected HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(NewMaxBodyReader(resp.Body, defaultMaxBodySize))
	if err != nil {
		return FetchResult{}, fmt.Errorf("api: read body: %w", err)
	}

	if cfg.Path != "" {
		body, err = selectByPath(body, cfg.Path)
		if err != nil {
			return FetchResult{}, fmt.Errorf("api: select path %q: %w", cfg.Path, err)
		}
	}

	contentType := resp.Header.Get("Content-Type")

	return FetchResult{
		HTTPStatus:  resp.StatusCode,
		Headers:     flattenHeaders(resp.Header),
		ContentType: contentType,
		Body:        body,
		ContentHash: NormalizeForHash(body, contentType),
	}, nil
}

// validateAPIEndpoint constrains provider_config.endpoint: http(s) only,
// no userinfo — on top of the SafeClient dial guard. Private-literal and
// numeric-IP hosts are rejected unless the fetcher runs with a
// test-injected client (allowPrivate, httptest loopback). Operator
// endpoints are trusted past these insert-time checks (DNS rebinding
// residual, same as git).
func (f *APIFetcher) validateAPIEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("api: endpoint must be an http(s) URL: %q", endpoint)
	}

	if u.User != nil {
		return errors.New("api: endpoint must not embed credentials")
	}

	host := u.Hostname()
	if sources.IsPrivateIPLiteral(host) || isNumericIPPrivate(host) {
		if !f.allowPrivate || !isLoopbackOrPrivateLiteral(host) {
			return fmt.Errorf("api: endpoint targets a private address: %q", endpoint)
		}
	}

	return nil
}
