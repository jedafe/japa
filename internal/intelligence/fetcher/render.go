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
//	{"url": "...", "wait_selector": "...", "timeout_ms": 30000, "a11y": false,
//	 "stealth": true, "solve": "auto", "proxy_group": "...", "session_id": "..."}
//	→ 200 {"html": "...", "final_url": "...", "wall_detected": false}
//	                                         (a11y → {"a11y_text": "..."})
//	→ 4xx = bad request (fetch failure, does not count toward the circuit)
//	→ 5xx/timeout = service failure (fetch failure; renderCircuitThreshold
//	  consecutive failures open the breaker)
//
// provider_config keys: wait_selector, timeout_ms (overrides
// intel.render.timeout_ms), a11y, auth_token (Bearer auth to the service).
//
// Anti-bot escalation keys — stealth, solve, proxy_group, session_id — are
// policy-gated: they are forwarded to the render service ONLY when
// provider_config.bypass_policy = "auto". The default ("never" or unset)
// strips them, so an operator cannot smuggle CAPTCHA solving or stealth
// flags into a render request without an explicit per-source opt-in.
// The Go side never solves CAPTCHAs itself — it only relays the opt-in;
// whether the service honors solve:"auto" is the service's policy. A
// wall_detected response flag is surfaced as a WallError so repeated
// walls route through the blocked failure class, not the baseline.
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
	Stealth      bool   `json:"stealth,omitempty"`
	Solve        string `json:"solve,omitempty"`
	ProxyGroup   string `json:"proxy_group,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

type renderResponse struct {
	HTML         string `json:"html"`
	A11yText     string `json:"a11y_text"`
	FinalURL     string `json:"final_url"`
	WallDetected bool   `json:"wall_detected"`
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

	// The render target is attacker-influenced (proposed source entry_url)
	// and the fetch is delegated to an internal service that skips its own
	// dial guard (AllowPrivate client). Validate the target here so a
	// proposed http://169.254.169.254/ source cannot SSRF through the
	// renderer. Service URL itself is admin-configured — scheme + userinfo
	// checked, host trusted past literals (internal infra by design).
	if err := validateRenderTarget(src.SourceUrl); err != nil {
		return FetchResult{}, err
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

	renderReq := renderRequest{
		URL:          src.SourceUrl,
		WaitSelector: stringFromCfg(cfg, "wait_selector"),
		TimeoutMs:    timeoutMs,
		A11y:         a11y,
	}

	// Anti-bot escalation fields forward only under an explicit
	// bypass_policy:"auto" opt-in — "never" (default) and every other
	// value strip them. JapaConnect is a polite self-identifying crawler;
	// CAPTCHA solving and stealth flags are a per-source posture decision,
	// not something a config can sneak into the request.
	if bypassPolicyAuto(cfg) {
		renderReq.Stealth, _ = cfg["stealth"].(bool)
		renderReq.Solve = stringFromCfg(cfg, "solve")
		renderReq.ProxyGroup = stringFromCfg(cfg, "proxy_group")
		renderReq.SessionID = stringFromCfg(cfg, "session_id")
	}

	reqBody, err := json.Marshal(renderReq)
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

	// The service flagged the page as a wall even though it produced
	// content — solver declined or challenge only partially passed.
	// Surface it as a wall so the channel records the sighting instead of
	// baselining challenge HTML.
	if out.WallDetected {
		return FetchResult{}, &WallError{Method: "render", Status: http.StatusOK, Signature: "render:wall_detected"}
	}

	// A rendered bot-wall challenge is still a wall — the service passed a
	// challenge page back as if it were content. Catch it before the body
	// poisons the channel's change-detection baseline.
	if sig, ok := DetectWall(http.StatusOK, nil, []byte(content), contentType); ok {
		return FetchResult{}, &WallError{Method: "render", Status: http.StatusOK, Signature: sig}
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

// bypassPolicyAuto reports whether provider_config.bypass_policy is the
// explicit "auto" opt-in that unlocks anti-bot escalation fields
// (stealth/solve/proxy_group/session_id) on render requests. Anything
// else — unset, "never", or a typo — is treated as never.
func bypassPolicyAuto(cfg map[string]any) bool {
	return stringFromCfg(cfg, "bypass_policy") == "auto"
}

// validateRenderTarget constrains the delegated render target: http(s)
// only, no userinfo, no private-literal or numeric-IP hosts. Attacker
// proposes the URL via source entry_url; without this the internal render
// service (AllowPrivate) fetches it unchecked.
func validateRenderTarget(target string) error {
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("fetcher: render target must be an http(s) URL")
	}

	if u.User != nil {
		return errors.New("fetcher: render target must not embed credentials")
	}

	host := u.Hostname()
	if sources.IsPrivateIPLiteral(host) || isNumericIPPrivate(host) {
		return errors.New("fetcher: render target must not be a private address")
	}

	return nil
}
