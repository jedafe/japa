// Package travel — service.go holds the business logic for the Travel Search
// & Compare surface. It proxies flight and hotel search queries to a
// configured affiliate travel provider API, injects affiliate tracking
// parameters at render time (ADR-009), and caches identical queries in Valkey
// (jc:travel:search:{hash}, configurable TTL, default 5 min).
//
// Ref: japaconnect-engineering/sections/67-travel-search.md
//      japaconnect-engineering/adr/ADR-085-travel-search.md
//      japaconnect-engineering/adr/ADR-009-render-time-affiliate-processing.md
package travel

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
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
)

// Provider type constants — match the CHECK constraint on
// travel_providers.provider_type.
const (
	ProviderTypeFlight = "flight"
	ProviderTypeHotel  = "hotel"
	ProviderTypeBoth   = "both"
)

// ErrTravelDisabled is returned when the travel surface is globally disabled
// via the travel.enabled system_setting (default false until a provider is
// configured).
var ErrTravelDisabled = errors.New("travel: feature disabled")

// defaultCacheTTL is the fallback TTL for jc:travel:search:{hash} when the
// travel.cache_ttl_seconds system_setting is unset (ADR-085: 5 min).
const defaultCacheTTL = 5 * time.Minute

// ---------------------------------------------------------------------------
// Search parameter and result DTOs
// ---------------------------------------------------------------------------

// FlightSearchParams is the user-facing flight search query. All timestamps
// are RFC 3339 UTC date strings.
type FlightSearchParams struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	Date        string `json:"date"`
	Adults      int    `json:"adults"`
	Children    int    `json:"children"`
	ReturnDate  string `json:"return_date"`
	CabinClass  string `json:"cabin_class"`
}

// HotelSearchParams is the user-facing hotel search query. All timestamps are
// RFC 3339 UTC date strings.
type HotelSearchParams struct {
	Destination string `json:"destination"`
	CheckIn     string `json:"check_in"`
	CheckOut    string `json:"check_out"`
	Guests      int    `json:"guests"`
	Rooms       int    `json:"rooms"`
}

// FlightResult is a single flight offer returned by the provider API, with
// affiliate tracking params injected into BookingURL at render time.
type FlightResult struct {
	Provider      string  `json:"provider"`
	Price         float64 `json:"price"`
	Currency      string  `json:"currency"`
	DepartureTime string  `json:"departure_time"`
	ArrivalTime   string  `json:"arrival_time"`
	Airline       string  `json:"airline"`
	FlightNumber  string  `json:"flight_number"`
	Stops         int     `json:"stops"`
	Duration      string  `json:"duration"`
	BookingURL    string  `json:"booking_url"`
}

// HotelResult is a single hotel offer returned by the provider API, with
// affiliate tracking params injected into BookingURL at render time.
type HotelResult struct {
	Provider    string  `json:"provider"`
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	Currency    string  `json:"currency"`
	CheckIn     string  `json:"check_in"`
	CheckOut    string  `json:"check_out"`
	StarRating  int     `json:"star_rating"`
	Address     string  `json:"address"`
	ImageURL    string  `json:"image_url"`
	BookingURL  string  `json:"booking_url"`
}

// SearchResponse is the envelope returned to the client. Results is a slice
// of FlightResult or HotelResult (typed as []interface{} so the same envelope
// serves both search types).
type SearchResponse struct {
	Results      []interface{} `json:"results"`
	CacheHit     bool          `json:"cache_hit"`
	SearchTimeMs int64         `json:"search_time_ms"`
}

// ProviderInfo is the JSON-serialisable representation of a travel provider.
// APIConfig is intentionally excluded — encrypted credentials must not be
// returned to the API caller (§5.4 Secrets & Config, ADR-085).
type ProviderInfo struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Slug            string          `json:"slug"`
	ProviderType    string          `json:"provider_type"`
	AffiliateConfig json.RawMessage `json:"affiliate_config,omitempty"`
	IsActive        bool            `json:"is_active"`
	CreatedAt       string          `json:"created_at"`
}

// ---------------------------------------------------------------------------
// TravelAPIClient — provider API abstraction
// ---------------------------------------------------------------------------

// TravelAPIClient is the interface for calling a travel provider's search API.
// The default implementation (DefaultAPIClient) makes real HTTP calls via
// net/http; tests inject a fake implementation so the service can be exercised
// without network access.
type TravelAPIClient interface {
	SearchFlights(ctx context.Context, apiConfig json.RawMessage, params FlightSearchParams) ([]FlightResult, error)
	SearchHotels(ctx context.Context, apiConfig json.RawMessage, params HotelSearchParams) ([]HotelResult, error)
}

// DefaultAPIClient is the production TravelAPIClient. It reads the provider
// endpoint and credentials from apiConfig (stored encrypted in the DB) and
// performs HTTP GET requests against the provider's search endpoint.
type DefaultAPIClient struct {
	HTTPClient *http.Client
}

// NewDefaultAPIClient returns a DefaultAPIClient with a sensible timeout.
func NewDefaultAPIClient() *DefaultAPIClient {
	return &DefaultAPIClient{
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// apiEndpointConfig is the shape of the apiConfig JSON stored on a provider.
type apiEndpointConfig struct {
	Endpoint string          `json:"endpoint"`
	APIKey   string          `json:"api_key"`
	Extra    json.RawMessage `json:"extra,omitempty"`
}

// providerSearchResponse is a minimal envelope common to affiliate travel
// APIs (Kiwi/Tequila, Aviasales, Booking.com affiliate). Provider-specific
// mapping is intentionally kept loose — the admin configures the endpoint and
// the response shape is expected to be a JSON array of offer objects under
// "data" or "results".
type providerSearchResponse struct {
	Data    []json.RawMessage `json:"data"`
	Results []json.RawMessage `json:"results"`
}

// doSearch performs an HTTP GET against the provider endpoint, attaching the
// API key as a bearer token when present. The raw offer objects are returned
// for the caller to map into FlightResult/HotelResult.
func (c *DefaultAPIClient) doSearch(ctx context.Context, cfg apiEndpointConfig, query url.Values) ([]json.RawMessage, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("travel: provider endpoint not configured")
	}

	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("travel: invalid provider endpoint: %w", err)
	}
	q := u.Query()
	for k, vs := range query {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("travel: build request: %w", err)
	}
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("travel: provider request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("travel: provider returned %d: %s", resp.StatusCode, string(body))
	}

	var env providerSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("travel: decode provider response: %w", err)
	}

	if len(env.Data) > 0 {
		return env.Data, nil
	}
	return env.Results, nil
}

// SearchFlights calls the provider flight search endpoint and maps the raw
// offer objects into FlightResult. The mapping is best-effort: missing fields
// default to zero values.
func (c *DefaultAPIClient) SearchFlights(ctx context.Context, apiConfig json.RawMessage, params FlightSearchParams) ([]FlightResult, error) {
	var cfg apiEndpointConfig
	if len(apiConfig) > 0 {
		if err := json.Unmarshal(apiConfig, &cfg); err != nil {
			return nil, fmt.Errorf("travel: parse api_config: %w", err)
		}
	}

	q := url.Values{}
	q.Set("origin", params.Origin)
	q.Set("destination", params.Destination)
	q.Set("date", params.Date)
	q.Set("adults", fmt.Sprintf("%d", params.Adults))
	q.Set("children", fmt.Sprintf("%d", params.Children))
	if params.ReturnDate != "" {
		q.Set("return_date", params.ReturnDate)
	}
	if params.CabinClass != "" {
		q.Set("cabin_class", params.CabinClass)
	}

	rawOffers, err := c.doSearch(ctx, cfg, q)
	if err != nil {
		return nil, err
	}

	results := make([]FlightResult, 0, len(rawOffers))
	for _, raw := range rawOffers {
		var f FlightResult
		_ = json.Unmarshal(raw, &f)
		if f.Provider == "" {
			f.Provider = cfg.Endpoint
		}
		results = append(results, f)
	}
	return results, nil
}

// SearchHotels calls the provider hotel search endpoint and maps the raw
// offer objects into HotelResult.
func (c *DefaultAPIClient) SearchHotels(ctx context.Context, apiConfig json.RawMessage, params HotelSearchParams) ([]HotelResult, error) {
	var cfg apiEndpointConfig
	if len(apiConfig) > 0 {
		if err := json.Unmarshal(apiConfig, &cfg); err != nil {
			return nil, fmt.Errorf("travel: parse api_config: %w", err)
		}
	}

	q := url.Values{}
	q.Set("destination", params.Destination)
	q.Set("checkin", params.CheckIn)
	q.Set("checkout", params.CheckOut)
	q.Set("guests", fmt.Sprintf("%d", params.Guests))
	q.Set("rooms", fmt.Sprintf("%d", params.Rooms))

	rawOffers, err := c.doSearch(ctx, cfg, q)
	if err != nil {
		return nil, err
	}

	results := make([]HotelResult, 0, len(rawOffers))
	for _, raw := range rawOffers {
		var h HotelResult
		_ = json.Unmarshal(raw, &h)
		if h.Provider == "" {
			h.Provider = cfg.Endpoint
		}
		results = append(results, h)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// Service holds the business logic for the Travel Search surface.
type Service struct {
	store    *Store
	settings *config.Settings
	cache    valkeycompat.Cmdable // nil = caching disabled
	api      TravelAPIClient
}

// NewService creates a travel service. The default HTTP-based TravelAPIClient
// is wired in automatically; tests can override it with SetAPIClient.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
		api:      NewDefaultAPIClient(),
	}
}

// SetCache injects the Valkey client for search result caching.
func (s *Service) SetCache(c valkeycompat.Cmdable) { s.cache = c }

// SetAPIClient injects the travel provider API client. Tests use this to
// substitute a fake implementation.
func (s *Service) SetAPIClient(c TravelAPIClient) { s.api = c }

// IsEnabled returns whether the travel surface is globally enabled. Reads
// the travel.enabled system_setting (default false — off until a provider is
// configured, per ADR-085 §4).
func (s *Service) IsEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "travel.enabled", false)
}

// cacheTTL returns the configured cache TTL, falling back to 5 min.
func (s *Service) cacheTTL(ctx context.Context, tenantID pgtype.UUID) time.Duration {
	if s.settings == nil {
		return defaultCacheTTL
	}
	secs := s.settings.GetIntOr(ctx, tenantID, "travel.cache_ttl_seconds", 300)
	if secs <= 0 {
		return defaultCacheTTL
	}
	return time.Duration(secs) * time.Second
}

// ---------------------------------------------------------------------------
// Cache key + hash helpers
// ---------------------------------------------------------------------------

// searchCacheKey builds the Valkey key for a cached search result. The hash
// is over the canonical JSON of the search params plus the tenant ID, so
// identical queries hit the cache (ADR-085 §3, jc:travel:search:{hash}).
func searchCacheKey(tenantID pgtype.UUID, params interface{}) string {
	return valkey.MustKey("travel:search:%s", HashSearchQuery(params))
}

// HashSearchQuery generates a SHA-256 hash of the canonical JSON encoding of
// params. The same params always produce the same hash, so identical queries
// resolve to the same cache key. Struct fields are serialized in a
// deterministic order by sorting the JSON object keys.
func HashSearchQuery(params interface{}) string {
	raw, err := json.Marshal(params)
	if err != nil {
		return ""
	}
	canonical := canonicalizeJSON(raw)
	h := sha256.Sum256(canonical)
	return hex.EncodeToString(h[:])
}

// canonicalizeJSON re-serializes a JSON byte slice with object keys sorted
// alphabetically so that semantically identical payloads produce identical
// bytes regardless of field declaration order.
func canonicalizeJSON(raw []byte) []byte {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	return sortJSONValue(v)
}

func sortJSONValue(v interface{}) []byte {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		sb.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				sb.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			sb.Write(kb)
			sb.WriteByte(':')
			sb.Write(sortJSONValue(val[k]))
		}
		sb.WriteByte('}')
		return []byte(sb.String())
	case []interface{}:
		var sb strings.Builder
		sb.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.Write(sortJSONValue(item))
		}
		sb.WriteByte(']')
		return []byte(sb.String())
	default:
		b, _ := json.Marshal(val)
		return b
	}
}

// ---------------------------------------------------------------------------
// Affiliate param injection (ADR-009 — render-time processing)
// ---------------------------------------------------------------------------

// InjectAffiliateParams injects affiliate tracking parameters into a booking
// URL at render time. affiliateConfig is the JSON stored on the provider's
// affiliate_config column — an object of key/value query params (e.g.
// {"aid": "12345", "utm_source": "japaconnect"}). The params are appended to
// the booking URL's query string; existing params are preserved.
//
// Per ADR-009, affiliate processing happens at render time, never at storage
// time — the provider API returns a clean booking URL and JapaConnect stamps
// its tracking params on the way out to the client.
func InjectAffiliateParams(bookingURL string, affiliateConfig json.RawMessage) string {
	if bookingURL == "" || len(affiliateConfig) == 0 {
		return bookingURL
	}

	var params map[string]string
	if err := json.Unmarshal(affiliateConfig, &params); err != nil {
		return bookingURL
	}
	if len(params) == 0 {
		return bookingURL
	}

	u, err := url.Parse(bookingURL)
	if err != nil {
		return bookingURL
	}
	q := u.Query()
	for k, v := range params {
		if k == "" {
			continue
		}
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// injectAffiliateIntoFlights stamps affiliate params onto every flight
// result's BookingURL in place.
func injectAffiliateIntoFlights(results []FlightResult, affiliateConfig json.RawMessage) {
	for i := range results {
		results[i].BookingURL = InjectAffiliateParams(results[i].BookingURL, affiliateConfig)
	}
}

// injectAffiliateIntoHotels stamps affiliate params onto every hotel result's
// BookingURL in place.
func injectAffiliateIntoHotels(results []HotelResult, affiliateConfig json.RawMessage) {
	for i := range results {
		results[i].BookingURL = InjectAffiliateParams(results[i].BookingURL, affiliateConfig)
	}
}

// ---------------------------------------------------------------------------
// Search operations
// ---------------------------------------------------------------------------

// SearchFlights proxies a flight search query to the configured flight
// provider API. The flow: (1) check cache, (2) on miss, get the active
// provider, (3) call the provider API, (4) inject affiliate params into
// booking URLs, (5) cache the result, (6) return. If travel.enabled is false,
// returns ErrTravelDisabled (the handler maps this to 503).
func (s *Service) SearchFlights(ctx context.Context, tenantID pgtype.UUID, params FlightSearchParams) (*SearchResponse, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return nil, ErrTravelDisabled
	}

	start := time.Now()
	cacheKey := searchCacheKey(tenantID, params)

	// (1) Check cache.
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var results []FlightResult
			if err := json.Unmarshal([]byte(cached), &results); err == nil {
				out := make([]interface{}, len(results))
				for i, r := range results {
					out[i] = r
				}
				return &SearchResponse{
					Results:      out,
					CacheHit:     true,
					SearchTimeMs: time.Since(start).Milliseconds(),
				}, nil
			}
		}
	}

	// (2) Get the active provider.
	provider, err := s.store.GetActiveProvider(ctx, tenantID, ProviderTypeFlight)
	if err != nil {
		return nil, fmt.Errorf("travel: get active flight provider: %w", err)
	}

	// (3) Decrypt the provider's api_config (ADR-085 encryption at rest).
	apiCfg, err := decryptAPIConfig(provider)
	if err != nil {
		return nil, fmt.Errorf("travel: flight search: %w", err)
	}

	// (4) Call the provider API.
	results, err := s.api.SearchFlights(ctx, apiCfg, params)
	if err != nil {
		return nil, fmt.Errorf("travel: flight search: %w", err)
	}

	// (4) Inject affiliate params into booking URLs (ADR-009).
	injectAffiliateIntoFlights(results, provider.AffiliateConfig)

	// (5) Cache the result.
	if s.cache != nil && len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			if err := s.cache.Set(ctx, cacheKey, data, s.cacheTTL(ctx, tenantID)).Err(); err != nil {
				slog.Warn("travel: flight search: cache set failed", "error", err)
			}
		}
	}

	// (6) Return.
	out := make([]interface{}, len(results))
	for i, r := range results {
		out[i] = r
	}
	return &SearchResponse{
		Results:      out,
		CacheHit:     false,
		SearchTimeMs: time.Since(start).Milliseconds(),
	}, nil
}

// SearchHotels proxies a hotel search query to the configured hotel provider
// API. Same flow as SearchFlights.
func (s *Service) SearchHotels(ctx context.Context, tenantID pgtype.UUID, params HotelSearchParams) (*SearchResponse, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return nil, ErrTravelDisabled
	}

	start := time.Now()
	cacheKey := searchCacheKey(tenantID, params)

	// (1) Check cache.
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var results []HotelResult
			if err := json.Unmarshal([]byte(cached), &results); err == nil {
				out := make([]interface{}, len(results))
				for i, r := range results {
					out[i] = r
				}
				return &SearchResponse{
					Results:      out,
					CacheHit:     true,
					SearchTimeMs: time.Since(start).Milliseconds(),
				}, nil
			}
		}
	}

	// (2) Get the active provider.
	provider, err := s.store.GetActiveProvider(ctx, tenantID, ProviderTypeHotel)
	if err != nil {
		return nil, fmt.Errorf("travel: get active hotel provider: %w", err)
	}

	// (3) Decrypt the provider's api_config (ADR-085 encryption at rest).
	apiCfg, err := decryptAPIConfig(provider)
	if err != nil {
		return nil, fmt.Errorf("travel: hotel search: %w", err)
	}

	// (4) Call the provider API.
	results, err := s.api.SearchHotels(ctx, apiCfg, params)
	if err != nil {
		return nil, fmt.Errorf("travel: hotel search: %w", err)
	}

	// (4) Inject affiliate params into booking URLs (ADR-009).
	injectAffiliateIntoHotels(results, provider.AffiliateConfig)

	// (5) Cache the result.
	if s.cache != nil && len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			if err := s.cache.Set(ctx, cacheKey, data, s.cacheTTL(ctx, tenantID)).Err(); err != nil {
				slog.Warn("travel: hotel search: cache set failed", "error", err)
			}
		}
	}

	// (6) Return.
	out := make([]interface{}, len(results))
	for i, r := range results {
		out[i] = r
	}
	return &SearchResponse{
		Results:      out,
		CacheHit:     false,
		SearchTimeMs: time.Since(start).Milliseconds(),
	}, nil
}

// ---------------------------------------------------------------------------
// Admin: provider management
// ---------------------------------------------------------------------------

// GetActiveProvider returns the active provider for a given type. Used by the
// search proxy and exposed for admin introspection.
func (s *Service) GetActiveProvider(ctx context.Context, tenantID pgtype.UUID, providerType string) (ProviderInfo, error) {
	p, err := s.store.GetActiveProvider(ctx, tenantID, providerType)
	if err != nil {
		return ProviderInfo{}, fmt.Errorf("travel: get active provider: %w", err)
	}
	return providerToInfo(p), nil
}

// CreateProvider creates a new travel provider (admin).
func (s *Service) CreateProvider(ctx context.Context, arg gen.CreateTravelProviderParams) (ProviderInfo, error) {
	if arg.ProviderType != ProviderTypeFlight && arg.ProviderType != ProviderTypeHotel && arg.ProviderType != ProviderTypeBoth {
		return ProviderInfo{}, fmt.Errorf("travel: invalid provider_type %q", arg.ProviderType)
	}
	p, err := s.store.CreateProvider(ctx, arg)
	if err != nil {
		return ProviderInfo{}, fmt.Errorf("travel: create provider: %w", err)
	}
	return providerToInfo(p), nil
}

// UpdateProvider updates a travel provider's mutable fields (admin).
func (s *Service) UpdateProvider(ctx context.Context, arg gen.UpdateTravelProviderParams) (ProviderInfo, error) {
	if arg.ProviderType != nil && *arg.ProviderType != ProviderTypeFlight && *arg.ProviderType != ProviderTypeHotel && *arg.ProviderType != ProviderTypeBoth {
		return ProviderInfo{}, fmt.Errorf("travel: invalid provider_type %q", *arg.ProviderType)
	}
	p, err := s.store.UpdateProvider(ctx, arg)
	if err != nil {
		return ProviderInfo{}, fmt.Errorf("travel: update provider: %w", err)
	}
	return providerToInfo(p), nil
}

// ListProviders lists travel providers for a tenant (admin).
func (s *Service) ListProviders(ctx context.Context, tenantID pgtype.UUID, activeOnly bool, providerType string) ([]ProviderInfo, error) {
	rows, err := s.store.ListProviders(ctx, tenantID, activeOnly, providerType)
	if err != nil {
		return nil, err
	}
	providers := make([]ProviderInfo, 0, len(rows))
	for _, p := range rows {
		providers = append(providers, providerToInfo(p))
	}
	return providers, nil
}

// providerToInfo maps a gen.TravelProvider to the JSON-serialisable ProviderInfo
// DTO. IDs are rendered as strings; timestamps as RFC 3339 UTC. APIConfig is
// deliberately omitted — encrypted credentials are never returned to the API
// caller (ADR-085, §5.4).
func providerToInfo(p gen.TravelProvider) ProviderInfo {
	var affCfg json.RawMessage
	if len(p.AffiliateConfig) > 0 {
		affCfg = json.RawMessage(p.AffiliateConfig)
	}
	created := ""
	if p.CreatedAt.Valid {
		created = p.CreatedAt.Time.UTC().Format(time.RFC3339)
	}
	return ProviderInfo{
		ID:              p.ID.String(),
		Name:            p.Name,
		Slug:            p.Slug,
		ProviderType:    p.ProviderType,
		AffiliateConfig: affCfg,
		IsActive:        p.IsActive,
		CreatedAt:       created,
	}
}

// decryptAPIConfig decrypts the encrypted api_config stored on a travel
// provider, returning the original raw JSON for use by the search API client.
func decryptAPIConfig(p gen.TravelProvider) (json.RawMessage, error) {
	decrypted, err := crypto.DecryptJSON(p.ApiConfig)
	if err != nil {
		return nil, fmt.Errorf("travel: decrypt api_config: %w", err)
	}
	return json.RawMessage(decrypted), nil
}


