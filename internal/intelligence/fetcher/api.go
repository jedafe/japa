package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
}

// NewAPIFetcher returns an APIFetcher with a 30s timeout and SSRF-safe
// transport (redirects to private IPs are blocked; response bodies are
// capped at defaultMaxBodySize by the Fetch method).
func NewAPIFetcher() *APIFetcher {
	return &APIFetcher{client: NewSafeClient(30*time.Second, defaultMaxBodySize)}
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

	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodGet
	}

	var bodyReader io.Reader
	if len(cfg.Body) > 0 {
		bodyReader = bytes.NewReader(cfg.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, cfg.Endpoint, bodyReader)
	if err != nil {
		return FetchResult{}, fmt.Errorf("api: new request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect-Intelligence/1.0 (+https://japaconnect.com)")
	req.Header.Set("Accept", "application/json")

	if len(cfg.Body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("api: do: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

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
