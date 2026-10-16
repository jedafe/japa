// adapter.go implements remittance.RateProvider for mid-market FX engines
// (Section 112, ADR-125). One upstream call per remittance:fetch_rates tick
// writes the full fx_rates matrix AND returns per-corridor RateSnapshots the
// remittance insert loop persists under the "mid-market" benchmark provider.
//
// Ref: japaconnect-engineering/sections/112-fx-converter.md
//
//	japaconnect-engineering/adr/ADR-125-fx-converter.md
package fx

import (
	"errors"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/remittance"
)

// defaultOXREndpoint is the compiled-in endpoint for the oxr engine;
// overridable per-provider via api_config.endpoint (§112 — the field exists
// so a self-hosted/proxy feed or paid-tier base override needs config only).
const defaultOXREndpoint = "https://openexchangerates.org/api/latest.json"

// defaultHTTPTimeout bounds the upstream engine call when the caller passes a
// nil http.Client — http.DefaultClient has no timeout and a hung upstream
// would stall the remittance:fetch_rates sweep until the Asynq deadline.
const defaultHTTPTimeout = 30 * time.Second

// attributionURL is rendered on benchmark rows and in the UI — the OXR free
// tier terms require attribution.
const attributionURL = "https://openexchangerates.org"

// providerConfig is the decrypted api_config shape for fx engines (§112).
type providerConfig struct {
	Engine             string   `json:"engine"`
	AppID              string   `json:"app_id"`
	BaseCurrency       string   `json:"base_currency"`
	MinIntervalMinutes int      `json:"min_interval_minutes"`
	Endpoint           string   `json:"endpoint"`
	Corridors          []string `json:"corridors"`
}

// Adapter implements remittance.RateProvider for mid-market FX engines.
type Adapter struct {
	svc  *Service
	http *http.Client
	// allowPrivate relaxes the insert-time endpoint guard for httptest
	// loopback servers. Set only when a custom client is injected
	// (tests); production wiring passes nil and the guard stays on.
	allowPrivate bool
}

// NewAdapter creates the adapter. httpClient may be nil → a client with the
// default upstream timeout.
func NewAdapter(svc *Service, httpClient *http.Client) *Adapter {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
		return &Adapter{svc: svc, http: httpClient}
	}

	return &Adapter{svc: svc, http: httpClient, allowPrivate: true}
}

// FetchRates satisfies remittance.RateProvider (ADR-075 §2).
// apiConfig is the decrypted remittance_providers.api_config JSON; corridors
// are the platform's existing corridor pairs (may be empty on fresh installs,
// where config.corridors is the seed set).
func (a *Adapter) FetchRates(ctx context.Context, apiConfig []byte, corridors []remittance.Corridor) ([]remittance.RateSnapshot, error) {
	var cfg providerConfig
	if len(apiConfig) > 0 {
		if err := json.Unmarshal(apiConfig, &cfg); err != nil {
			return nil, fmt.Errorf("fx: parse api_config: %w", err)
		}
	}

	if cfg.Engine == "" {
		cfg.Engine = "oxr"
	}

	if cfg.BaseCurrency == "" {
		cfg.BaseCurrency = "USD"
	}

	if cfg.MinIntervalMinutes <= 0 {
		cfg.MinIntervalMinutes = 60
	}

	if cfg.AppID == "" {
		return nil, errors.New("fx: provider api_config missing app_id")
	}

	// Quota guard: skip when the last matrix is fresher than min_interval.
	// Returning nil,nil leaves the existing remittance_rates rows untouched —
	// the sweep logs "no snapshots" and stays cheap (§112).
	if a.svc != nil {
		last, err := a.svc.LastFetchTime(ctx, cfg.Engine)
		if err != nil {
			slog.Warn("fx: last fetch probe failed, proceeding", "error", err)
		} else if throttled(last, cfg.MinIntervalMinutes) {
			slog.Info("fx: fetch throttled inside min_interval", "engine", cfg.Engine)
			return nil, nil
		}
	}

	rates, err := a.fetchMatrix(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if len(rates) == 0 {
		return nil, fmt.Errorf("fx: engine %q returned empty matrix", cfg.Engine)
	}

	if a.svc != nil {
		if err := a.svc.InsertMatrix(ctx, cfg.BaseCurrency, cfg.Engine, rates); err != nil {
			return nil, err
		}
	}

	m := &Matrix{Base: cfg.BaseCurrency, Rates: rates, FetchedAt: time.Now()}

	return emitSnapshots(m, cfg.Corridors, corridors, attributionURL), nil
}

// throttled reports whether lastFetch is inside minInterval minutes.
// A zero lastFetch (never fetched) never throttles.
func throttled(lastFetch time.Time, minIntervalMinutes int) bool {
	if lastFetch.IsZero() {
		return false
	}

	return time.Since(lastFetch) < time.Duration(minIntervalMinutes)*time.Minute
}

// fetchMatrix calls the engine endpoint and returns base→quote rates.
// The base→base = 1.0 cell is injected when the engine omits it so every
// matrix uniformly contains its own base.
func (a *Adapter) fetchMatrix(ctx context.Context, cfg providerConfig) (map[string]float64, error) {
	var base string

	var rates map[string]float64

	var err error

	switch cfg.Engine {
	case "oxr":
		base, rates, err = a.fetchOXR(ctx, cfg)
	default:
		return nil, fmt.Errorf("fx: unsupported engine %q", cfg.Engine)
	}

	if err != nil {
		return nil, err
	}
	// A configured base that disagrees with the engine's response base means
	// the matrix would be labelled wrong — every downstream rate off by a
	// ratio. Fail loudly rather than persist mislabeled data.
	if base != "" && base != cfg.BaseCurrency {
		return nil, fmt.Errorf("fx: engine %q returned base %q, api_config expects %q", cfg.Engine, base, cfg.BaseCurrency)
	}
	// Canonical base→base cell — refill on missing OR corrupt (≤0) values so
	// cross-rate math can never divide by a zero base.
	if rates[cfg.BaseCurrency] <= 0 {
		rates[cfg.BaseCurrency] = 1
	}

	return rates, nil
}

func (a *Adapter) fetchOXR(ctx context.Context, cfg providerConfig) (string, map[string]float64, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = defaultOXREndpoint
	} else if err := a.validateEngineEndpoint(endpoint); err != nil {
		return "", nil, err
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", nil, fmt.Errorf("fx: parse oxr endpoint: %w", err)
	}

	q := u.Query()
	q.Set("app_id", cfg.AppID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", nil, fmt.Errorf("fx: build oxr request: %w", err)
	}

	resp, err := a.http.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("fx: oxr request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("fx: oxr status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", nil, fmt.Errorf("fx: read oxr body: %w", err)
	}

	return parseOXRResponse(body)
}

// parseOXRResponse extracts the base currency and rates map from a
// latest.json payload.
func parseOXRResponse(body []byte) (string, map[string]float64, error) {
	var parsed struct {
		Base  string             `json:"base"`
		Rates map[string]float64 `json:"rates"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", nil, fmt.Errorf("fx: parse oxr response: %w", err)
	}

	if parsed.Rates == nil {
		return "", nil, errors.New("fx: oxr response has no rates")
	}

	return parsed.Base, parsed.Rates, nil
}

// emitSnapshots converts a matrix into per-corridor RateSnapshots for the
// union of config corridors ("FROM->TO" strings) and requested corridors,
// deduplicated and limited to pairs the matrix covers. Mid-market rows carry
// no fees or transfer time — only the cross-rate and the attribution URL.
func emitSnapshots(m *Matrix, configCorridors []string, requested []remittance.Corridor, sourceURL string) []remittance.RateSnapshot {
	seen := map[string]bool{}

	var pairs []remittance.Corridor

	for _, c := range configCorridors {
		parts := strings.SplitN(c, "->", 2)
		if len(parts) != 2 {
			continue
		}

		pairs = append(pairs, remittance.Corridor{
			SendCurrency:    strings.ToUpper(strings.TrimSpace(parts[0])),
			ReceiveCurrency: strings.ToUpper(strings.TrimSpace(parts[1])),
		})
	}

	pairs = append(pairs, requested...)

	var snaps []remittance.RateSnapshot

	for _, p := range pairs {
		key := p.SendCurrency + "->" + p.ReceiveCurrency
		if seen[key] {
			continue
		}

		seen[key] = true

		fromRate, ok1 := m.Rates[p.SendCurrency]
		toRate, ok2 := m.Rates[p.ReceiveCurrency]

		if !ok1 || !ok2 || fromRate <= 0 || toRate <= 0 {
			continue
		}

		snaps = append(snaps, remittance.RateSnapshot{
			SendCurrency:    p.SendCurrency,
			ReceiveCurrency: p.ReceiveCurrency,
			Rate:            toRate / fromRate,
			SourceURL:       sourceURL,
		})
	}

	return snaps
}

// validateEngineEndpoint constrains api_config.endpoint overrides: https
// only (the app_id query credential would leak over plaintext http), no
// userinfo, no private-literal hosts. The compiled-in default bypasses
// validation (constant, not operator input). Test-injected clients
// (allowPrivate) accept loopback http for httptest servers. No dial-time
// guard exists on the adapter client — DNS-rebinding past insert-time
// checks is an accepted residual (admin-only precondition, per-tick fetch
// with 30s timeout).
func (a *Adapter) validateEngineEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return fmt.Errorf("fx: engine endpoint must be a valid URL: %q", endpoint)
	}

	if u.Scheme != "https" {
		if !a.allowPrivate || u.Scheme != "http" {
			return fmt.Errorf("fx: engine endpoint must be https: %q", endpoint)
		}

		host := u.Hostname()
		if ip := net.ParseIP(host); ip != nil {
			if !isReservedIP(ip) {
				return fmt.Errorf("fx: engine endpoint must be https for non-local hosts: %q", endpoint)
			}
		} else if !strings.EqualFold(host, "localhost") && !strings.HasSuffix(strings.ToLower(host), ".localhost") {
			return fmt.Errorf("fx: engine endpoint must be https for non-local hosts: %q", endpoint)
		}

		return nil
	}

	if u.User != nil {
		return errors.New("fx: engine endpoint must not embed credentials")
	}

	if ip := net.ParseIP(u.Hostname()); ip != nil && isReservedIP(ip) {
		return fmt.Errorf("fx: engine endpoint targets a private address: %q", endpoint)
	}

	return nil
}

// isReservedIP mirrors the webhook/ai SSRF literal blocklist for the fx
// adapter (stdlib-only: fx cannot import intelligence/sources without a
// cycle risk through remittance).
func isReservedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}

	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}

		if v4[0] == 0 || v4[0] >= 240 {
			return true
		}

		if (v4[0] == 192 && v4[1] == 0 && (v4[2] == 0 || v4[2] == 2)) ||
			(v4[0] == 198 && v4[1] == 51 && v4[2] == 100) ||
			(v4[0] == 203 && v4[1] == 0 && v4[2] == 113) ||
			(v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)) {
			return true
		}
	}

	return false
}
