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

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// RenderFetcher is the Phase-2 headless-render adapter (channel design §3):
// entry_url is rendered by the operator-configured headless service
// (intel.render.service_url — empty = adapter disabled) and the post-JS DOM
// (or accessibility tree when provider_config.a11y) becomes the body.
//
// Render-service contract (internal):
//
//	POST {service_url}/render
//	{"url": "...", "wait_selector": "...", "timeout_ms": 30000, "a11y": false}
//	→ 200 {"html": "...", "final_url": "..."}        (a11y → {"a11y_text": "..."})
//	→ 4xx = bad request (fetch failure, does not count toward the circuit)
//	→ 5xx/timeout = service failure (fetch failure; renderCircuitThreshold
//	  consecutive failures open the breaker)
//
// provider_config keys: wait_selector, timeout_ms (overrides
// intel.render.timeout_ms), a11y, auth_token (Bearer auth to the service).
//
// jc:intel:render:circuit (5m) is set after renderCircuitThreshold
// consecutive transport/5xx failures — while present the adapter reports
// ErrAdapterDisabled so a flapping render service does not churn every
// render channel's failure counter. The consecutive count lives in
// jc:intel:render:circuit_failures (reset on the first success or when the
// breaker trips).
type RenderFetcher struct {
	client   *http.Client
	settings AdapterSettings
	rdb      valkeycompat.Cmdable
}

// NewRenderFetcher returns a RenderFetcher. settings/rdb may be nil — the
// adapter then reports ErrAdapterDisabled for every fetch (unconfigured).
func NewRenderFetcher(client *http.Client, settings AdapterSettings, rdb valkeycompat.Cmdable) *RenderFetcher {
	return &RenderFetcher{client: client, settings: settings, rdb: rdb}
}

const (
	renderCircuitKey      = "jc:intel:render:circuit"
	renderCircuitFailures = "jc:intel:render:circuit_failures"
	renderCircuitTTL      = 5 * time.Minute
	// renderCircuitThreshold is the number of consecutive service failures
	// that opens the breaker (channel design §3: N=3 — a single transient
	// timeout must not blackout every render channel for 5 minutes).
	renderCircuitThreshold = 3
	renderPath             = "/render"
)

type renderRequest struct {
	URL          string `json:"url"`
	WaitSelector string `json:"wait_selector,omitempty"`
	TimeoutMs    int    `json:"timeout_ms,omitempty"`
	A11y         bool   `json:"a11y,omitempty"`
}

type renderResponse struct {
	HTML     string `json:"html"`
	A11yText string `json:"a11y_text"`
	FinalURL string `json:"final_url"`
}

// Fetch implements Fetcher.
func (f *RenderFetcher) Fetch(ctx context.Context, src sources.Source) (FetchResult, error) {
	serviceURL := ""
	if f.settings != nil {
		serviceURL = strings.TrimRight(f.settings.GetStringOr(ctx, "intel.render.service_url", ""), "/")
	}
	if serviceURL == "" || f.client == nil {
		return FetchResult{}, fmt.Errorf("%w: intel.render.service_url unset", ErrAdapterDisabled)
	}

	if f.circuitOpen(ctx) {
		return FetchResult{}, fmt.Errorf("%w: render circuit open", ErrAdapterDisabled)
	}

	var cfg map[string]any
	_ = json.Unmarshal(src.ProviderConfig, &cfg)
	a11y, _ := cfg["a11y"].(bool)
	timeoutMs := intFromCfg(cfg, "timeout_ms", 0)
	if timeoutMs <= 0 && f.settings != nil {
		timeoutMs = f.settings.GetIntOr(ctx, "intel.render.timeout_ms", 30000)
	}
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	reqBody, err := json.Marshal(renderRequest{
		URL:          src.SourceUrl,
		WaitSelector: stringFromCfg(cfg, "wait_selector"),
		TimeoutMs:    timeoutMs,
		A11y:         a11y,
	})
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: render request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond+5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, serviceURL+renderPath, bytes.NewReader(reqBody))
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: render request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token := stringFromCfg(cfg, "auth_token"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		f.recordServiceFailure(ctx)
		return FetchResult{}, fmt.Errorf("fetcher: render: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(NewMaxBodyReader(resp.Body, defaultMaxBodySize))
	if err != nil {
		f.recordServiceFailure(ctx)
		return FetchResult{}, fmt.Errorf("fetcher: render read: %w", err)
	}

	if resp.StatusCode >= 500 {
		f.recordServiceFailure(ctx)
		return FetchResult{}, fmt.Errorf("fetcher: render service: status %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("fetcher: render service: status %d", resp.StatusCode)
	}

	var out renderResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: render decode: %w", err)
	}

	content := out.HTML
	contentType := "text/html"
	if a11y && out.A11yText != "" {
		content = out.A11yText
		contentType = "text/plain"
	}
	if content == "" {
		return FetchResult{}, errors.New("fetcher: render service returned empty body")
	}

	f.resetServiceFailures(ctx)

	return FetchResult{
		HTTPStatus:  http.StatusOK,
		Headers:     map[string]string{"X-Render-Final-URL": out.FinalURL},
		ContentType: contentType,
		Body:        []byte(content),
		ContentHash: NormalizeForHash([]byte(content), contentType),
	}, nil
}

// circuitOpen reports whether the shared render circuit key is set — all
// render channels share one breaker since the failure domain is the service
// itself, not any channel.
func (f *RenderFetcher) circuitOpen(ctx context.Context) bool {
	if f.rdb == nil {
		return false
	}
	ok, err := f.rdb.Exists(ctx, renderCircuitKey).Result()
	return err == nil && ok > 0
}

// renderFailureScript atomically increments the consecutive-failure counter
// and sets its TTL only on the first increment — a counter without expiry
// would count failures spread arbitrarily far apart as "consecutive" (and a
// process crash between INCR and EXPIRE would leave it permanently stale).
const renderFailureScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`

// recordServiceFailure counts one consecutive transport/5xx failure; at
// renderCircuitThreshold it opens the breaker for renderCircuitTTL and
// clears the counter. Valkey errors are swallowed — the breaker is
// best-effort and must not fail the fetch.
func (f *RenderFetcher) recordServiceFailure(ctx context.Context) {
	if f.rdb == nil {
		return
	}

	count, err := f.rdb.Eval(ctx, renderFailureScript, []string{renderCircuitFailures}, int64(renderCircuitTTL/time.Second)).Result()
	if err != nil {
		return
	}
	if n, ok := count.(int64); ok && n >= renderCircuitThreshold {
		_ = f.rdb.Set(ctx, renderCircuitKey, "1", renderCircuitTTL).Err()
		_ = f.rdb.Del(ctx, renderCircuitFailures).Err()
	}
}

// resetServiceFailures clears the consecutive-failure counter on a
// successful render — only consecutive failures count toward the breaker.
func (f *RenderFetcher) resetServiceFailures(ctx context.Context) {
	if f.rdb == nil {
		return
	}
	_ = f.rdb.Del(ctx, renderCircuitFailures).Err()
}

// intFromCfg / stringFromCfg read loosely-typed provider_config values.
func intFromCfg(cfg map[string]any, key string, fallback int) int {
	switch v := cfg[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

func stringFromCfg(cfg map[string]any, key string) string {
	if s, ok := cfg[key].(string); ok {
		return s
	}
	return ""
}
