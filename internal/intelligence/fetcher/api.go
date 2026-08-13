package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// APIProviderConfig is the JSON schema for source.provider_config when
// crawl_method = "api". It carries the endpoint URL, an optional JSON path
// pointing at the list of records within the response, and optional auth
// headers to attach to the request.
type APIProviderConfig struct {
	Endpoint string            `json:"endpoint"`
	Path     string            `json:"path,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// APIFetcher is a generic JSON API client. It reads the endpoint + response
// path from source.provider_config, issues an HTTP GET, and returns the raw
// JSON body with a content hash. Downstream stages (extractor) walk Path to
// pull individual records.
type APIFetcher struct {
	client *http.Client
}

// NewAPIFetcher returns an APIFetcher with a 30s timeout.
func NewAPIFetcher() *APIFetcher {
	return &APIFetcher{client: &http.Client{Timeout: 30 * time.Second}}
}

// ParseProviderConfig decodes source.provider_config into APIProviderConfig.
// An empty config is an error — an API source must specify an endpoint.
func ParseProviderConfig(raw []byte) (APIProviderConfig, error) {
	var cfg APIProviderConfig
	if len(raw) == 0 {
		return cfg, fmt.Errorf("api: provider_config is empty")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("api: parse provider_config: %w", err)
	}
	if cfg.Endpoint == "" {
		return cfg, fmt.Errorf("api: provider_config.endpoint is empty")
	}
	return cfg, nil
}

// Fetch implements the Fetcher interface for JSON API sources.
func (f *APIFetcher) Fetch(ctx context.Context, source sources.Source) (FetchResult, error) {
	cfg, err := ParseProviderConfig(source.ProviderConfig)
	if err != nil {
		return FetchResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", cfg.Endpoint, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("api: new request: %w", err)
	}
	req.Header.Set("User-Agent", "JapaConnect-Intelligence/1.0 (+https://japaconnect.com)")
	req.Header.Set("Accept", "application/json")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("api: do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return FetchResult{}, fmt.Errorf("api: read body: %w", err)
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
