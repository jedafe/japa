// Package travel — service.go holds the business logic for the Travel Search
// & Compare surface. Searches resolve through a layered result store — Valkey
// cache (jc:travel:search:{hash}), then fresh/stale travel_search_results
// rows, then a live provider call — with affiliate tracking params injected
// at render time on every path (ADR-009).
// // Ref: japaconnect-engineering/sections/67-travel-search.md
// //	japaconnect-engineering/adr/ADR-085-travel-search.md
// japaconnect-engineering/adr/ADR-009-render-time-affiliate-processing.md
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
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"
	"golang.org/x/sync/singleflight"

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
// via the travel.enabled feature flag (default false until a provider is
// configured). The public search handlers surface this before the provider
// lookup, so the status/code contract is:
//
//	ErrTravelDisabled → 503 travel_disabled (surface switched off)
//	ErrNoProvider     → 503 provider_not_configured (surface on, no usable provider)
//
// The route-level features.Gate(travel.enabled) 404s first (ADR-042); the
// service check covers direct service callers and flag races between the
// gate and the search.
var ErrTravelDisabled = errors.New("travel: feature disabled")

// ErrNoProvider is returned when a search is attempted but no active provider
// exists for the requested type (flight or hotel). The handler maps this to a
// 503 provider_not_configured so the UI can show a friendly "not configured
// yet" state instead of a generic failure. Unlike ErrTravelDisabled it means
// the surface is enabled but has no usable upstream — never returned when
// the feature flag itself is off.
var ErrNoProvider = errors.New("travel: no provider configured")

// errProviderUnconfigured marks a provider whose api_config carries no
// endpoint — an active row that isn't actually usable yet. Search maps it to
// ErrNoProvider after attempting stale-fallback.
var errProviderUnconfigured = errors.New("travel: provider endpoint not configured")

// defaultCacheTTL is the fallback TTL for jc:travel:search:{hash} when the
// travel.cache_ttl_seconds system_setting is unset (ADR-085: 5 min).
const defaultCacheTTL = 5 * time.Minute

// ---------------------------------------------------------------------------
// Search parameter and result DTOs
// ---------------------------------------------------------------------------

// normalizeCoverageString lowercases and trims a value for coverage hashing.
// Every coverage hash funnels through this so route/city/cabin keys share
// one normalization instead of drifting per call site.
func normalizeCoverageString(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normalizeCoverageRoute uppercases and trims an IATA code pair for coverage
// hashing.
func normalizeCoverageCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

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
// OfferID carries the provider's offer identifier when present — it becomes
// the inventory offer_key (ADR-124). RawOffer preserves the original provider
// payload for forensics; it is never serialized to clients.
type FlightResult struct {
	Provider      string  `json:"provider"`
	Price         float64 `json:"price"`
	Currency      string  `json:"currency"`
	DepartureTime string  `json:"departure_time"`
	ArrivalTime   string  `json:"arrival_time"`
	Airline       string  `json:"airline"`
	// AirlineKey is the canonical travel_airlines identity (e.g. "iata:BA")
	// resolved during offer persistence — enables canonical airline filtering
	// regardless of the raw name/code a provider sent.
	AirlineKey   string          `json:"airline_key,omitempty"`
	FlightNumber string          `json:"flight_number"`
	Stops        int             `json:"stops"`
	Duration     string          `json:"duration"`
	BookingURL   string          `json:"booking_url"`
	OfferID      string          `json:"offer_id,omitempty"`
	RawOffer     json.RawMessage `json:"-"`
}

// HotelResult is a single hotel offer returned by the provider API, with
// affiliate tracking params injected into BookingURL at render time.
type HotelResult struct {
	Provider   string          `json:"provider"`
	Name       string          `json:"name"`
	Price      float64         `json:"price"`
	Currency   string          `json:"currency"`
	CheckIn    string          `json:"check_in"`
	CheckOut   string          `json:"check_out"`
	StarRating int             `json:"star_rating"`
	Address    string          `json:"address"`
	ImageURL   string          `json:"image_url"`
	BookingURL string          `json:"booking_url"`
	OfferID    string          `json:"offer_id,omitempty"`
	RawOffer   json.RawMessage `json:"-"`
}

// SearchOptions are the presentation params (ADR-124): sorting, offer
// filters, and pagination applied at serve time over a coverage's offer
// set. They are deliberately excluded from the coverage hash — every
// presentation shape of a covered query serves from the same stored offers
// with zero upstream calls.
//
// PageSize 0 (absent) returns the full filtered set — backward compatible
// with the original client-side pagination contract.
type SearchOptions struct {
	Sort     string  `json:"sort,omitempty"`
	Airline  string  `json:"airline,omitempty"`
	MaxStops int     `json:"max_stops,omitempty"` // flights; <0 = unset
	PriceMax float64 `json:"price_max,omitempty"` // <=0 = unset
	MinStars int     `json:"min_stars,omitempty"` // hotels; <=0 = unset
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`

	// airlineKey is the canonical travel_airlines key resolved from Airline
	// by the service before serving — not an API param.
	airlineKey string
}

// SearchResponse is the envelope returned to the client. Results is a slice
// of FlightResult or HotelResult (typed as []interface{} so the same envelope
// serves both search types). Source records which layer served the query:
// "cache" (Valkey), "inventory" (ADR-124 offer rows under fresh coverage),
// "stored" (travel_search_results row, fresh), "stale" (expired data served
// because the provider call failed), or "provider" (live provider call,
// freshly persisted). Total is the filtered-set size before pagination.
type SearchResponse struct {
	Results      []interface{} `json:"results"`
	CacheHit     bool          `json:"cache_hit"`
	Source       string        `json:"source"`
	SearchTimeMs int64         `json:"search_time_ms"`
	Total        int           `json:"total"`
	Page         int           `json:"page"`
	PageSize     int           `json:"page_size"`
}

// LocationResult is one airport suggestion returned by /travel/locations
// (§67 amendment — real flight locations, not generic geo spaces).
type LocationResult struct {
	IATA        string `json:"iata"`
	Name        string `json:"name"`
	City        string `json:"city"`
	CountryCode string `json:"country_code"`
}

// HotelDirectoryItem is one property row in the hotel directory served by
// /travel/hotels (ADR-124 extension). Populated by hotel search upserts and
// the travel:sync_hotel_directory feed task; never by a live call.
type HotelDirectoryItem struct {
	HotelID      string   `json:"hotel_id,omitempty"`
	Name         string   `json:"name"`
	City         string   `json:"city"`
	StarRating   int      `json:"star_rating,omitempty"`
	Address      string   `json:"address,omitempty"`
	ImageURL     string   `json:"image_url,omitempty"`
	LastPrice    *float64 `json:"last_price,omitempty"`
	LastCurrency string   `json:"last_currency,omitempty"`
}

// AirlineDirectoryItem is one carrier row in the airline directory served
// by /travel/airlines (ADR-124 extension). Providers lists the configured
// integrations whose offers have been observed carrying this airline —
// the catalog ↔ travel_providers coverage join.
type AirlineDirectoryItem struct {
	AirlineKey string   `json:"airline_key"`
	IATACode   string   `json:"iata_code,omitempty"`
	ICAOCode   string   `json:"icao_code,omitempty"`
	Name       string   `json:"name"`
	Country    string   `json:"country,omitempty"`
	Observed   bool     `json:"observed"`
	Providers  []string `json:"providers"`
	LastSeenAt string   `json:"last_seen_at,omitempty"`
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

// PublicProviderInfo is the minimal public-safe representation of a travel
// provider. It omits APIConfig and AffiliateConfig so anonymous users can
// browse and compare providers without accessing tracking credentials.
type PublicProviderInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	ProviderType string `json:"provider_type"`
	IsActive     bool   `json:"is_active"`
	CreatedAt    string `json:"created_at"`
}

// ---------------------------------------------------------------------------
// TravelAPIClient — provider API abstraction
// ---------------------------------------------------------------------------

// TravelAPIClient is the interface for calling a travel provider's search API.
// The default implementation (DefaultAPIClient) makes real HTTP calls via
// net/http; tests inject a fake implementation so the service can be exercised
// without network access.
// //nolint:revive // exported: stutter naming is intentional.
type TravelAPIClient interface {
	SearchFlights(ctx context.Context, apiConfig json.RawMessage, params FlightSearchParams) ([]FlightResult, error)
	SearchHotels(ctx context.Context, apiConfig json.RawMessage, params HotelSearchParams) ([]HotelResult, error)
	// SearchLocations queries the provider's own locations/autocomplete API
	// when api_config.extra.locations_endpoint is configured. Returns nil,
	// nil when the provider has no locations endpoint — the seeded
	// travel_airports catalog is the primary source.
	SearchLocations(ctx context.Context, apiConfig json.RawMessage, q string) ([]LocationResult, error)
	// FetchHotelDirectory pulls the provider's static hotel-content feed
	// (api_config.extra.hotels_endpoint) for the travel_hotels directory.
	// Returns nil, nil when no feed is configured.
	FetchHotelDirectory(ctx context.Context, apiConfig json.RawMessage) ([]HotelDirectoryItem, error)
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
		return nil, errProviderUnconfigured
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
	q.Set("adults", strconv.Itoa(params.Adults))
	q.Set("children", strconv.Itoa(params.Children))

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

		f.OfferID = extractOfferID(raw, f.OfferID)
		f.RawOffer = append(json.RawMessage(nil), raw...)

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
	q.Set("guests", strconv.Itoa(params.Guests))
	q.Set("rooms", strconv.Itoa(params.Rooms))

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

		h.OfferID = extractOfferID(raw, h.OfferID)
		h.RawOffer = append(json.RawMessage(nil), raw...)

		results = append(results, h)
	}

	return results, nil
}

// extractOfferID pulls the provider's offer identifier out of the raw
// payload — offer_id wins, falling back to a bare id field. Empty when the
// provider doesn't carry one (the caller hashes offer fields instead).
func extractOfferID(raw json.RawMessage, alreadyMapped string) string {
	if alreadyMapped != "" {
		return alreadyMapped
	}

	var probe struct {
		ID      string `json:"id"`
		OfferID string `json:"offer_id"`
	}

	if json.Unmarshal(raw, &probe) != nil {
		return ""
	}

	if probe.OfferID != "" {
		return probe.OfferID
	}

	return probe.ID
}

// SearchLocations queries the provider's locations/autocomplete endpoint
// (api_config.extra.locations_endpoint) and normalises whatever field names
// the provider uses into LocationResult. Returns nil, nil when no locations
// endpoint is configured so the caller degrades to the seeded airport
// catalog only (§67 locations endpoint).
func (c *DefaultAPIClient) SearchLocations(ctx context.Context, apiConfig json.RawMessage, q string) ([]LocationResult, error) {
	var cfg apiEndpointConfig
	if len(apiConfig) > 0 {
		if err := json.Unmarshal(apiConfig, &cfg); err != nil {
			return nil, fmt.Errorf("travel: parse api_config: %w", err)
		}
	}

	var extra struct {
		LocationsEndpoint string `json:"locations_endpoint"`
	}

	if len(cfg.Extra) > 0 {
		_ = json.Unmarshal(cfg.Extra, &extra)
	}

	if extra.LocationsEndpoint == "" {
		return nil, nil
	}

	cfg.Endpoint = extra.LocationsEndpoint

	query := url.Values{}
	query.Set("q", q)

	raw, err := c.doSearch(ctx, cfg, query)
	if err != nil {
		return nil, err
	}

	out := make([]LocationResult, 0, len(raw))

	for _, r := range raw {
		var loc struct {
			IATA         string `json:"iata"`
			Code         string `json:"code"`
			Name         string `json:"name"`
			City         string `json:"city"`
			CityName     string `json:"city_name"`
			Municipality string `json:"municipality"`
			Country      string `json:"country"`
			CountryCode  string `json:"country_code"`
			ISOCountry   string `json:"iso_country"`
		}

		if json.Unmarshal(r, &loc) != nil {
			continue
		}

		iata := loc.IATA
		if iata == "" {
			iata = loc.Code
		}

		city := loc.City
		if city == "" {
			city = loc.CityName
		}

		if city == "" {
			city = loc.Municipality
		}

		country := loc.CountryCode
		if country == "" {
			country = loc.ISOCountry
		}

		if country == "" {
			country = loc.Country
		}

		out = append(out, LocationResult{IATA: iata, Name: loc.Name, City: city, CountryCode: country})
	}

	return out, nil
}

// FetchHotelDirectory pulls the provider's static hotel-content feed
// (api_config.extra.hotels_endpoint) and normalises field-name variants into
// HotelDirectoryItem rows. Returns nil, nil when the provider has no feed
// configured — the directory stays search-populated in that case.
func (c *DefaultAPIClient) FetchHotelDirectory(ctx context.Context, apiConfig json.RawMessage) ([]HotelDirectoryItem, error) {
	var cfg apiEndpointConfig
	if len(apiConfig) > 0 {
		if err := json.Unmarshal(apiConfig, &cfg); err != nil {
			return nil, fmt.Errorf("travel: parse api_config: %w", err)
		}
	}

	var extra struct {
		HotelsEndpoint string `json:"hotels_endpoint"`
	}

	if len(cfg.Extra) > 0 {
		_ = json.Unmarshal(cfg.Extra, &extra)
	}

	if extra.HotelsEndpoint == "" {
		return nil, nil
	}

	cfg.Endpoint = extra.HotelsEndpoint

	raw, err := c.doSearch(ctx, cfg, url.Values{})
	if err != nil {
		return nil, err
	}

	out := make([]HotelDirectoryItem, 0, len(raw))

	for _, r := range raw {
		var h struct {
			HotelID     string  `json:"hotel_id"`
			PropertyID  string  `json:"property_id"`
			HotelCode   string  `json:"hotel_code"`
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			City        string  `json:"city"`
			CityName    string  `json:"city_name"`
			Destination string  `json:"destination"`
			StarRating  float64 `json:"star_rating"`
			Stars       float64 `json:"stars"`
			Rating      float64 `json:"rating"`
			Address     string  `json:"address"`
			ImageURL    string  `json:"image_url"`
			Image       string  `json:"image"`
			Thumbnail   string  `json:"thumbnail"`
		}

		if json.Unmarshal(r, &h) != nil {
			continue
		}

		id := h.HotelID
		if id == "" {
			id = h.PropertyID
		}

		if id == "" {
			id = h.HotelCode
		}

		if id == "" {
			id = h.ID
		}

		city := h.City
		if city == "" {
			city = h.CityName
		}

		if city == "" {
			city = h.Destination
		}

		stars := h.StarRating
		if stars == 0 {
			stars = h.Stars
		}

		if stars == 0 {
			stars = h.Rating
		}

		image := h.ImageURL
		if image == "" {
			image = h.Image
		}

		if image == "" {
			image = h.Thumbnail
		}

		out = append(out, HotelDirectoryItem{
			HotelID:    id,
			Name:       h.Name,
			City:       city,
			StarRating: int(stars),
			Address:    h.Address,
			ImageURL:   image,
		})
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for feature reachability.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Service holds the business logic for the Travel Search surface.
type Service struct {
	store    *Store
	settings *config.Settings
	checker  FeatureChecker
	cache    valkeycompat.Cmdable // nil = caching disabled
	api      TravelAPIClient
	// searchGroup dedupes concurrent identical provider calls (§67 quota
	// controls) — a burst of parallel users with the same canonical query
	// costs one upstream request.
	searchGroup singleflight.Group
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

// SetFeatureChecker injects the feature flag checker so IsEnabled reads the
// feature_flags table (the single source of truth for feature reachability)
// instead of system_settings. Falls back to system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// IsEnabled returns whether the travel surface is globally enabled. Reads the
// travel.enabled feature flag (default false — off until a provider is
// configured, per ADR-085 §4); system_settings is consulted only as a
// fallback when no checker is injected.
func (s *Service) IsEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "travel.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "travel.enabled", false)
}

// cacheTTL returns the configured cache TTL, falling back to 5 min.
func (s *Service) cacheTTL(ctx context.Context) time.Duration {
	if s.settings == nil {
		return defaultCacheTTL
	}

	secs := s.settings.GetIntOr(ctx, "travel.cache_ttl_seconds", 300)
	if secs <= 0 {
		return defaultCacheTTL
	}

	return time.Duration(secs) * time.Second
}

// resultsTTL returns the stored-result freshness window, falling back to 1h
// (travel.results_ttl_seconds, §67).
func (s *Service) resultsTTL(ctx context.Context) time.Duration {
	const fallback = time.Hour
	if s.settings == nil {
		return fallback
	}

	secs := s.settings.GetIntOr(ctx, "travel.results_ttl_seconds", 3600)
	if secs <= 0 {
		return fallback
	}

	return time.Duration(secs) * time.Second
}

// locationsCacheTTL returns the Valkey TTL for cached provider location
// responses, falling back to 24h (travel.locations_cache_ttl_seconds, §67).
func (s *Service) locationsCacheTTL(ctx context.Context) time.Duration {
	const fallback = 24 * time.Hour
	if s.settings == nil {
		return fallback
	}

	secs := s.settings.GetIntOr(ctx, "travel.locations_cache_ttl_seconds", 86400)
	if secs <= 0 {
		return fallback
	}

	return time.Duration(secs) * time.Second
}

// locationsNegativeTTL is the short negative-cache window for failed
// provider location calls — keeps a dead endpoint from being re-hit on
// every autocomplete keystroke.
const locationsNegativeTTL = 60 * time.Second

// persistSearchResult upserts the provider's raw (pre-affiliate) results into
// travel_search_results for the canonical query. Best-effort — a failed write
// must not fail the search.
func (s *Service) persistSearchResult(ctx context.Context, searchType, queryHash string, params, results interface{}, providerID pgtype.UUID) {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return
	}

	resultsJSON, err := json.Marshal(results)
	if err != nil {
		return
	}

	if _, err := s.store.UpsertSearchResult(ctx, gen.UpsertTravelSearchResultParams{
		SearchType: searchType,
		QueryHash:  queryHash,
		Params:     paramsJSON,
		Results:    resultsJSON,
		ProviderID: providerID,
		ExpiresAt:  pgtype.Timestamptz{Time: time.Now().Add(s.resultsTTL(ctx)), Valid: true},
	}); err != nil {
		slog.Warn("travel: persist search result failed", "search_type", searchType, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Cache key + hash helpers
// ---------------------------------------------------------------------------

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
// // Per ADR-009, affiliate processing happens at render time, never at storage
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

// cacheSearchResults writes results to the Valkey query cache. Best-effort —
// a failed write must not fail the search.
func (s *Service) cacheSearchResults(ctx context.Context, cacheKey string, results interface{}) {
	if s.cache == nil {
		return
	}

	data, err := json.Marshal(results)
	if err != nil {
		return
	}

	if err := s.cache.Set(ctx, cacheKey, data, s.cacheTTL(ctx)).Err(); err != nil {
		slog.Warn("travel: cache set failed", "error", err)
	}
}

// storedSearchRow fetches the persisted result row for a canonical query.
// Returns nil on miss or error (logged) — a broken store never blocks search.
func (s *Service) storedSearchRow(ctx context.Context, searchType, queryHash string) *gen.TravelSearchResult {
	row, err := s.store.GetSearchResult(ctx, searchType, queryHash)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("travel: stored result lookup failed", "search_type", searchType, "error", err)
		}

		return nil
	}

	return &row
}

// staleFlightResponse serves expired data for a coverage when the provider
// path failed. ADR-124: expired inventory offers are tried first (they are
// offer-level data, at least as fresh as the replay blob), then the expired
// travel_search_results row. Returns nil when neither yields offers.
// Affiliate injection is skipped when the provider could not be loaded
// (nil config is a no-op).
func (s *Service) staleFlightResponse(ctx context.Context, stored *gen.TravelSearchResult, affiliate json.RawMessage, params FlightSearchParams, coverageKey string, provider *gen.TravelProvider, opts SearchOptions, start time.Time) *SearchResponse {
	if provider != nil {
		if offers := s.inventoryFlights(ctx, *provider, params, coverageKey, true); len(offers) > 0 {
			injectAffiliateIntoFlights(offers, affiliate)
			return serveFlights(offers, opts, "stale", true, start)
		}
	}

	if stored == nil {
		return nil
	}

	var stale []FlightResult
	if err := json.Unmarshal(stored.Results, &stale); err != nil || len(stale) == 0 {
		return nil
	}

	injectAffiliateIntoFlights(stale, affiliate)

	return serveFlights(stale, opts, "stale", true, start)
}

// staleHotelResponse mirrors staleFlightResponse for hotel offers.
func (s *Service) staleHotelResponse(ctx context.Context, stored *gen.TravelSearchResult, affiliate json.RawMessage, params HotelSearchParams, coverageKey string, provider *gen.TravelProvider, opts SearchOptions, start time.Time) *SearchResponse {
	if provider != nil {
		if offers := s.inventoryHotels(ctx, *provider, params, coverageKey, true); len(offers) > 0 {
			injectAffiliateIntoHotels(offers, affiliate)
			return serveHotels(offers, opts, "stale", true, start)
		}
	}

	if stored == nil {
		return nil
	}

	var stale []HotelResult
	if err := json.Unmarshal(stored.Results, &stale); err != nil || len(stale) == 0 {
		return nil
	}

	injectAffiliateIntoHotels(stale, affiliate)

	return serveHotels(stale, opts, "stale", true, start)
}

// ---------------------------------------------------------------------------
// Offer inventory (ADR-124) — coverage keys, TTL ladder, quota, mapping
// ---------------------------------------------------------------------------

// ErrQuotaExceeded is returned when the daily upstream-call ceiling
// (travel.max_provider_calls_per_day) is exhausted — handlers map it to
// 429 travel_quota_exceeded.
var ErrQuotaExceeded = errors.New("travel: daily provider call quota exhausted")

// flightCoverageHash produces the coverage key for a flight query — the
// canonical COVERAGE params (route/date/return/pax/cabin), normalized.
// Presentation params never reach this hash (§67/ADR-124).
func flightCoverageHash(p FlightSearchParams) string {
	return HashSearchQuery(struct {
		Type       string `json:"type"`
		Origin     string `json:"origin"`
		Dest       string `json:"dest"`
		Date       string `json:"date"`
		ReturnDate string `json:"return_date"`
		Cabin      string `json:"cabin"`
		Adults     int    `json:"adults"`
		Children   int    `json:"children"`
	}{
		Type:       ProviderTypeFlight,
		Origin:     normalizeCoverageCode(p.Origin),
		Dest:       normalizeCoverageCode(p.Destination),
		Date:       p.Date,
		ReturnDate: p.ReturnDate,
		Cabin:      normalizeCoverageString(p.CabinClass),
		Adults:     p.Adults,
		Children:   p.Children,
	})
}

// hotelCoverageHash mirrors flightCoverageHash for hotel queries.
func hotelCoverageHash(p HotelSearchParams) string {
	return HashSearchQuery(struct {
		Type     string `json:"type"`
		Dest     string `json:"dest"`
		CheckIn  string `json:"checkin"`
		CheckOut string `json:"checkout"`
		Guests   int    `json:"guests"`
		Rooms    int    `json:"rooms"`
	}{
		Type:     ProviderTypeHotel,
		Dest:     normalizeCoverageString(p.Destination),
		CheckIn:  p.CheckIn,
		CheckOut: p.CheckOut,
		Guests:   p.Guests,
		Rooms:    p.Rooms,
	})
}

// flightRouteKey is the coverage dimension stored on each flight offer row.
func flightRouteKey(p FlightSearchParams) string {
	return normalizeCoverageCode(p.Origin) + "-" +
		normalizeCoverageCode(p.Destination)
}

// hotelCityKey is the coverage dimension stored on each hotel offer row.
func hotelCityKey(p HotelSearchParams) string {
	return normalizeCoverageString(p.Destination)
}

// flightOfferKey derives the dedup identity of a provider offer, scoped to
// its coverage — the coverage dims are part of what an offer *is* (a price
// for a route/date/pax tuple), so a provider that reuses offer IDs across
// queries must produce distinct rows rather than collapsing into one
// coverage's row. Identity is the provider's own offer_id when present,
// else a hash over the fields that make an offer unique.
func flightOfferKey(r FlightResult, coverageKey string) string {
	if r.OfferID != "" {
		return coverageKey + "|id:" + r.OfferID
	}

	return coverageKey + "|h:" + HashSearchQuery(struct {
		Airline, FlightNumber, Depart, Arrive string
		Price                                 float64
	}{r.Airline, r.FlightNumber, r.DepartureTime, r.ArrivalTime, r.Price})
}

// hotelOfferKey mirrors flightOfferKey for hotel offers.
func hotelOfferKey(r HotelResult, coverageKey string) string {
	if r.OfferID != "" {
		return coverageKey + "|id:" + r.OfferID
	}

	return coverageKey + "|h:" + HashSearchQuery(struct {
		Name, Address, CheckIn string
		Price                  float64
	}{r.Name, r.Address, r.CheckIn, r.Price})
}

// providerOfferID strips the coverage scope back off an offer_key when
// surfacing the upstream offer identifier to clients.
func providerOfferID(offerKey string) string {
	if _, k, ok := strings.Cut(offerKey, "|"); ok {
		return strings.TrimPrefix(k, "id:")
	}

	return offerKey
}

// hotelEntityKey derives the directory identity of a hotel PROPERTY — the
// provider's hotel_id/property_id when the payload carries one, else its
// offer_id, else a content hash. Unlike offer keys this is NOT
// coverage-scoped: a hotel is the same entity across dates and rates.
func hotelEntityKey(r HotelResult, cityKey string) string {
	var raw struct {
		HotelID    string `json:"hotel_id"`
		PropertyID string `json:"property_id"`
		HotelCode  string `json:"hotel_code"`
	}

	if len(r.RawOffer) > 0 {
		_ = json.Unmarshal(r.RawOffer, &raw)
	}

	switch {
	case raw.HotelID != "":
		return "id:" + raw.HotelID
	case raw.PropertyID != "":
		return "id:" + raw.PropertyID
	case raw.HotelCode != "":
		return "id:" + raw.HotelCode
	case r.OfferID != "":
		return "id:" + r.OfferID
	default:
		return "h:" + HashSearchQuery(struct {
			Name, Address, City string
		}{r.Name, r.Address, cityKey})
	}
}

// upsertHotelDirectoryRow refreshes a property's travel_hotels row from a
// seen offer. Best-effort — a directory write must never sink a live search.
func (s *Service) upsertHotelDirectoryRow(ctx context.Context, providerID pgtype.UUID, cityKey string, r HotelResult) {
	if strings.TrimSpace(r.Name) == "" {
		return
	}

	err := s.store.UpsertTravelHotel(ctx, gen.UpsertTravelHotelParams{
		ProviderID:   providerID,
		HotelKey:     hotelEntityKey(r, cityKey),
		CityKey:      cityKey,
		Name:         r.Name,
		StarRating:   int16PtrOrNil(r.StarRating, r.StarRating > 0),
		Address:      strPtrOrNil(r.Address),
		ImageUrl:     strPtrOrNil(r.ImageURL),
		LastPrice:    floatToNumeric(r.Price),
		LastCurrency: strPtrOrNil(offerCurrency(r.Currency)),
	})
	if err != nil {
		slog.Warn("travel: upsert hotel directory row failed", "hotel", r.Name, "error", err)
	}
}

// HotelDirectory serves GET /travel/hotels — a pure local read over
// travel_hotels. It never calls a provider; the table is filled by search
// upserts and the sync task. Disabled surfaces return ErrTravelDisabled so
// the directory honors the same travel.enabled gate as search.
func (s *Service) HotelDirectory(ctx context.Context, city, q string, minStars, page, pageSize int) ([]HotelDirectoryItem, int64, error) {
	if !s.IsEnabled(ctx) {
		return nil, 0, ErrTravelDisabled
	}

	if pageSize <= 0 {
		pageSize = 50
	}

	if pageSize > 200 {
		pageSize = 200
	}

	if page <= 0 {
		page = 1
	}

	cityKey := normalizeCoverageString(city)
	nameQ := strings.TrimSpace(q)

	filter := gen.ListTravelHotelsParams{
		CityKey:  cityKey,
		NameQ:    nameQ,
		MinStars: toInt32(int64(minStars)),
		Off:      toInt32(int64(page-1) * int64(pageSize)),
		Lim:      toInt32(int64(pageSize)),
	}

	rows, err := s.store.ListTravelHotels(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountTravelHotels(ctx, gen.CountTravelHotelsParams{
		CityKey:  cityKey,
		NameQ:    nameQ,
		MinStars: toInt32(int64(minStars)),
	})
	if err != nil {
		return nil, 0, err
	}

	out := make([]HotelDirectoryItem, 0, len(rows))
	for _, h := range rows {
		out = append(out, hotelDirectoryItemFromRow(h))
	}

	return out, total, nil
}

// lookupAirline resolves an airline value (IATA code or name) to a catalog
// row. Read-only — used by the search filter path where writing observed
// rows would mutate the directory on a mere query.
func (s *Service) lookupAirline(ctx context.Context, raw string) (gen.TravelAirline, error) {
	var row gen.TravelAirline

	var err error

	if airlineIATARe.MatchString(raw) {
		row, err = s.store.FindTravelAirlineByIATA(ctx, strings.ToUpper(raw))
	}

	if err != nil || row.AirlineKey == "" {
		row, err = s.store.FindTravelAirlineByName(ctx, raw)
	}

	return row, err
}

// airlineKeyFor resolves an offer's airline value to a travel_airlines
// row: exact IATA code first, then case-insensitive name, then an upserted
// 'observed' row keyed by the raw value. On success it also records the
// provider coverage join — which integration actually carries the airline.
// Best-effort: returns "" on any failure so offer persistence is never
// sunk by directory bookkeeping.
func (s *Service) airlineKeyFor(ctx context.Context, providerID pgtype.UUID, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	row, err := s.lookupAirline(ctx, raw)

	if err == nil && row.AirlineKey != "" {
		// Existing catalog row — refresh last_seen_at so the timestamp
		// reflects the latest observation, then record coverage below.
		_ = s.store.UpsertTravelAirline(ctx, gen.UpsertTravelAirlineParams{
			AirlineKey: row.AirlineKey,
			Name:       row.Name,
			Country:    row.Country,
			Source:     row.Source,
		})
	} else {
		// Unseen airline — self-maintaining directory row. Key on the IATA
		// code when the provider sent one (it may fill a seed gap), else a
		// name slug.
		key := "name:" + airlineSlug(raw)

		var iata *string

		if airlineIATARe.MatchString(raw) {
			code := strings.ToUpper(raw)
			iata = &code
			key = "iata:" + code
		}

		if uerr := s.store.UpsertTravelAirline(ctx, gen.UpsertTravelAirlineParams{
			AirlineKey: key,
			IataCode:   iata,
			Name:       raw,
			Source:     "observed",
		}); uerr != nil {
			slog.Warn("travel: upsert airline failed", "airline", raw, "error", uerr)
			return ""
		}

		row.AirlineKey = key
	}

	if err := s.store.UpsertTravelProviderAirline(ctx, gen.UpsertTravelProviderAirlineParams{
		ProviderID: providerID,
		AirlineKey: row.AirlineKey,
	}); err != nil {
		slog.Warn("travel: upsert provider airline failed", "airline_key", row.AirlineKey, "error", err)
	}

	return row.AirlineKey
}

// airlineSlug normalizes a carrier name to its name-key suffix.
func airlineSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = airlineSlugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	if s == "" {
		s = "unknown"
	}

	return s
}

// AirlineDirectory serves GET /travel/airlines — a pure local read over
// travel_airlines joined to provider coverage. It never calls a provider;
// rows come from the OpenFlights seed and offer-persistence observations.
// Disabled surfaces return ErrTravelDisabled so the directory honors the
// same travel.enabled gate as search.
func (s *Service) AirlineDirectory(ctx context.Context, q, country, providerSlug string, page, pageSize int) ([]AirlineDirectoryItem, int64, error) {
	if !s.IsEnabled(ctx) {
		return nil, 0, ErrTravelDisabled
	}

	if pageSize <= 0 {
		pageSize = 50
	}

	if pageSize > 200 {
		pageSize = 200
	}

	if page <= 0 {
		page = 1
	}

	query := strings.TrimSpace(q)
	countryFilter := strings.TrimSpace(country)
	providerFilter := normalizeCoverageString(providerSlug)

	filter := gen.ListTravelAirlinesParams{
		Q:            query,
		Country:      countryFilter,
		ProviderSlug: providerFilter,
		Off:          toInt32(int64(page-1) * int64(pageSize)),
		Lim:          toInt32(int64(pageSize)),
	}

	rows, err := s.store.ListTravelAirlines(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountTravelAirlines(ctx, gen.CountTravelAirlinesParams{
		Q:            query,
		Country:      countryFilter,
		ProviderSlug: providerFilter,
	})
	if err != nil {
		return nil, 0, err
	}

	out := make([]AirlineDirectoryItem, 0, len(rows))
	for _, a := range rows {
		out = append(out, airlineDirectoryItemFromRow(a))
	}

	return out, total, nil
}

// airlineDirectoryItemFromRow maps a directory row to the public DTO.
func airlineDirectoryItemFromRow(a gen.ListTravelAirlinesRow) AirlineDirectoryItem {
	item := AirlineDirectoryItem{
		AirlineKey: a.AirlineKey,
		Name:       a.Name,
		Country:    a.Country,
		Observed:   a.Observed,
		Providers:  a.ProviderNames,
	}

	if a.IataCode != nil {
		item.IATACode = *a.IataCode
	}

	if a.IcaoCode != nil {
		item.ICAOCode = *a.IcaoCode
	}

	if a.CoverageLastSeenAt.Valid {
		item.LastSeenAt = a.CoverageLastSeenAt.Time.UTC().Format(time.RFC3339)
	}

	return item
}

// SyncHotelDirectory is the travel:sync_hotel_directory task body — pulls each
// active hotel provider's extra.hotels_endpoint content feed and upserts the
// properties. Providers without a feed are skipped; per-provider failures are
// best-effort so one bad feed can't sink the sweep.
func (s *Service) SyncHotelDirectory(ctx context.Context) error {
	providers, err := s.store.ListProviders(ctx, "true", ProviderTypeHotel, "")
	if err != nil {
		return fmt.Errorf("travel: list hotel providers: %w", err)
	}

	for _, p := range providers {
		apiCfg, err := decryptAPIConfig(p)
		if err != nil {
			slog.Warn("travel: hotel directory sync skipped — config undecryptable", "provider", p.Name, "error", err)
			continue
		}

		feed, err := s.api.FetchHotelDirectory(ctx, apiCfg)
		if err != nil {
			slog.Warn("travel: hotel directory feed failed", "provider", p.Name, "error", err)
			continue
		}

		for _, item := range feed {
			if strings.TrimSpace(item.Name) == "" {
				continue
			}

			cityKey := normalizeCoverageString(item.City)
			if cityKey == "" {
				continue
			}

			hotelKey := "h:" + HashSearchQuery(struct{ Name, Address, City string }{item.Name, item.Address, cityKey})
			if item.HotelID != "" {
				hotelKey = "id:" + item.HotelID
			}

			if err := s.store.UpsertTravelHotel(ctx, gen.UpsertTravelHotelParams{
				ProviderID: p.ID,
				HotelKey:   hotelKey,
				CityKey:    cityKey,
				Name:       item.Name,
				StarRating: int16PtrOrNil(item.StarRating, item.StarRating > 0),
				Address:    strPtrOrNil(item.Address),
				ImageUrl:   strPtrOrNil(item.ImageURL),
			}); err != nil {
				slog.Warn("travel: sync hotel upsert failed", "provider", p.Name, "hotel", item.Name, "error", err)
			}
		}

		slog.Info("travel: hotel directory synced", "provider", p.Name, "hotels", len(feed))
	}

	return nil
}

// inventoryEnabled reads the ADR-124 rollout flag.
func (s *Service) inventoryEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "travel.inventory_enabled", false)
}

// offerTTL returns the freshness window for an offer per the §67 TTL ladder:
// departures under 7 days out expire fastest, hotels use a flat TTL. The same
// duration stamps the coverage row that describes the offers.
func (s *Service) offerTTL(ctx context.Context, searchType, dateStr string) time.Duration {
	get := func(key string, fallback int) time.Duration {
		secs := fallback
		if s.settings != nil {
			secs = s.settings.GetIntOr(ctx, key, fallback)
		}

		if secs <= 0 {
			secs = fallback
		}

		return time.Duration(secs) * time.Second
	}

	if searchType == ProviderTypeHotel {
		return get("travel.offer_ttl_hotel_seconds", 21600)
	}

	depart, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return get("travel.offer_ttl_7_30d_seconds", 7200)
	}

	days := time.Until(depart).Hours() / 24

	switch {
	case days < 7:
		return get("travel.offer_ttl_lt7d_seconds", 1800)
	case days < 30:
		return get("travel.offer_ttl_7_30d_seconds", 7200)
	default:
		return get("travel.offer_ttl_gt30d_seconds", 21600)
	}
}

// acquireProviderQuota increments the daily upstream-call counter
// (jc:travel:quota:{YYYYMMDD}) and returns ErrQuotaExceeded once past
// travel.max_provider_calls_per_day. Called inside singleflight so it counts
// real provider calls, not requests. Fails open on Valkey outage — a cache
// problem must not kill search.
func (s *Service) acquireProviderQuota(ctx context.Context) error {
	if s.cache == nil || s.settings == nil {
		return nil
	}

	limit := s.settings.GetIntOr(ctx, "travel.max_provider_calls_per_day", 1000)
	if limit <= 0 {
		return nil
	}

	key := valkey.MustKey("travel:quota:%s", time.Now().UTC().Format("20060102"))

	pipe := s.cache.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 48*time.Hour)

	if _, err := pipe.Exec(ctx); err != nil {
		slog.Warn("travel: quota counter unavailable — allowing call", "error", err)
		return nil
	}

	if incr.Val() > int64(limit) {
		return ErrQuotaExceeded
	}

	return nil
}

// persistFlightOffers upserts each provider offer into the inventory and
// records the coverage row. Best-effort per offer — one malformed row must
// not sink the batch; a coverage row is written regardless (offer_count may
// be 0 for a legitimately empty response).
func (s *Service) persistFlightOffers(ctx context.Context, provider gen.TravelProvider, params FlightSearchParams, results []FlightResult, coverageKey string) {
	expires := time.Now().Add(s.offerTTL(ctx, ProviderTypeFlight, params.Date))

	departDate, _ := time.Parse("2006-01-02", params.Date)

	var returnDate pgtype.Date

	if params.ReturnDate != "" {
		if rd, err := time.Parse("2006-01-02", params.ReturnDate); err == nil {
			returnDate = pgtype.Date{Time: rd, Valid: true}
		}
	}

	// Airline resolution is memoized per coverage — a response carries a
	// handful of distinct carriers, and each resolution is a couple of
	// indexed reads (plus the provider-coverage join upsert).
	airlineKeys := make(map[string]string)
	airlineKey := func(raw string) string {
		k := strings.TrimSpace(raw)
		if v, ok := airlineKeys[k]; ok {
			return v
		}

		v := s.airlineKeyFor(ctx, provider.ID, raw)
		airlineKeys[k] = v

		return v
	}

	for i := range results {
		r := &results[i]
		ak := airlineKey(r.Airline)
		// Write the canonical key back onto the result so every source
		// (provider response, Valkey payload, replay row) carries it and
		// the airline filter can match canonically, not just literally.
		r.AirlineKey = ak

		arg := gen.UpsertFlightOfferParams{
			ProviderID:   provider.ID,
			RouteKey:     flightRouteKey(params),
			DepartDate:   pgtype.Date{Time: departDate, Valid: true},
			ReturnDate:   returnDate,
			Adults:       toInt16(params.Adults),
			Children:     toInt16(params.Children),
			CabinClass:   strings.ToLower(strings.TrimSpace(params.CabinClass)),
			OfferKey:     flightOfferKey(*r, coverageKey),
			Price:        floatToNumeric(r.Price),
			Currency:     offerCurrency(r.Currency),
			Airline:      strPtrOrNil(r.Airline),
			AirlineKey:   strPtrOrNil(ak),
			FlightNumber: strPtrOrNil(r.FlightNumber),
			Stops:        int16PtrOrNil(r.Stops, true),
			DepartTs:     tsFromString(r.DepartureTime),
			ArriveTs:     tsFromString(r.ArrivalTime),
			Duration:     strPtrOrNil(r.Duration),
			BookingUrl:   r.BookingURL,
			Raw:          r.RawOffer,
			ExpiresAt:    pgtype.Timestamptz{Time: expires, Valid: true},
		}

		if err := s.store.UpsertFlightOffer(ctx, arg); err != nil {
			slog.Warn("travel: upsert flight offer failed", "offer_key", arg.OfferKey, "error", err)
		}
	}

	if err := s.store.UpsertCoverage(ctx, gen.UpsertTravelSearchCoverageParams{
		SearchType:  ProviderTypeFlight,
		CoverageKey: coverageKey,
		ProviderID:  provider.ID,
		ExpiresAt:   pgtype.Timestamptz{Time: expires, Valid: true},
		OfferCount:  toInt32(int64(len(results))),
	}); err != nil {
		slog.Warn("travel: upsert coverage failed", "error", err)
	}
}

// persistHotelOffers mirrors persistFlightOffers for hotel offers. Each offer
// also upserts its property into the travel_hotels directory so property
// names/stars persist after price offers expire (ADR-124 extension).
func (s *Service) persistHotelOffers(ctx context.Context, provider gen.TravelProvider, params HotelSearchParams, results []HotelResult, coverageKey string) {
	expires := time.Now().Add(s.offerTTL(ctx, ProviderTypeHotel, params.CheckIn))

	checkin, _ := time.Parse("2006-01-02", params.CheckIn)
	checkout, _ := time.Parse("2006-01-02", params.CheckOut)

	cityKey := hotelCityKey(params)

	for _, r := range results {
		arg := gen.UpsertHotelOfferParams{
			ProviderID: provider.ID,
			CityKey:    cityKey,
			Checkin:    pgtype.Date{Time: checkin, Valid: true},
			Checkout:   pgtype.Date{Time: checkout, Valid: true},
			Guests:     toInt16(params.Guests),
			Rooms:      toInt16(params.Rooms),
			OfferKey:   hotelOfferKey(r, coverageKey),
			Name:       r.Name,
			Price:      floatToNumeric(r.Price),
			Currency:   offerCurrency(r.Currency),
			StarRating: int16PtrOrNil(r.StarRating, r.StarRating > 0),
			Address:    strPtrOrNil(r.Address),
			ImageUrl:   strPtrOrNil(r.ImageURL),
			BookingUrl: r.BookingURL,
			Raw:        r.RawOffer,
			ExpiresAt:  pgtype.Timestamptz{Time: expires, Valid: true},
		}

		if err := s.store.UpsertHotelOffer(ctx, arg); err != nil {
			slog.Warn("travel: upsert hotel offer failed", "offer_key", arg.OfferKey, "error", err)
		}

		s.upsertHotelDirectoryRow(ctx, provider.ID, cityKey, r)
	}

	if err := s.store.UpsertCoverage(ctx, gen.UpsertTravelSearchCoverageParams{
		SearchType:  ProviderTypeHotel,
		CoverageKey: coverageKey,
		ProviderID:  provider.ID,
		ExpiresAt:   pgtype.Timestamptz{Time: expires, Valid: true},
		OfferCount:  toInt32(int64(len(results))),
	}); err != nil {
		slog.Warn("travel: upsert coverage failed", "error", err)
	}
}

// inventoryFlights returns the offer rows for a flight coverage when a fresh
// coverage row exists (or stale rows when includeExpired). Returns nil on
// miss/error — callers fall through to the next layer.
func (s *Service) inventoryFlights(ctx context.Context, provider gen.TravelProvider, params FlightSearchParams, coverageKey string, includeExpired bool) []FlightResult {
	if !includeExpired {
		cov, err := s.store.GetCoverage(ctx, ProviderTypeFlight, coverageKey, provider.ID)
		if err != nil || !cov.ExpiresAt.Valid || !cov.ExpiresAt.Time.After(time.Now()) || cov.OfferCount == 0 {
			return nil
		}
	}

	departDate, _ := time.Parse("2006-01-02", params.Date)

	var returnDate pgtype.Date

	if params.ReturnDate != "" {
		if rd, err := time.Parse("2006-01-02", params.ReturnDate); err == nil {
			returnDate = pgtype.Date{Time: rd, Valid: true}
		}
	}

	rows, err := s.store.ListFlightOffersByCoverage(ctx, gen.ListFlightOffersByCoverageParams{
		ProviderID:     provider.ID,
		RouteKey:       flightRouteKey(params),
		DepartDate:     pgtype.Date{Time: departDate, Valid: true},
		ReturnDate:     returnDate,
		Adults:         toInt16(params.Adults),
		Children:       toInt16(params.Children),
		CabinClass:     strings.ToLower(strings.TrimSpace(params.CabinClass)),
		IncludeExpired: includeExpired,
	})
	if err != nil || len(rows) == 0 {
		return nil
	}

	out := make([]FlightResult, 0, len(rows))
	for _, o := range rows {
		out = append(out, flightResultFromOffer(provider, o))
	}

	return out
}

// inventoryHotels mirrors inventoryFlights for hotel offers.
func (s *Service) inventoryHotels(ctx context.Context, provider gen.TravelProvider, params HotelSearchParams, coverageKey string, includeExpired bool) []HotelResult {
	if !includeExpired {
		cov, err := s.store.GetCoverage(ctx, ProviderTypeHotel, coverageKey, provider.ID)
		if err != nil || !cov.ExpiresAt.Valid || !cov.ExpiresAt.Time.After(time.Now()) || cov.OfferCount == 0 {
			return nil
		}
	}

	checkin, _ := time.Parse("2006-01-02", params.CheckIn)
	checkout, _ := time.Parse("2006-01-02", params.CheckOut)

	rows, err := s.store.ListHotelOffersByCoverage(ctx, gen.ListHotelOffersByCoverageParams{
		ProviderID:     provider.ID,
		CityKey:        hotelCityKey(params),
		Checkin:        pgtype.Date{Time: checkin, Valid: true},
		Checkout:       pgtype.Date{Time: checkout, Valid: true},
		Guests:         toInt16(params.Guests),
		Rooms:          toInt16(params.Rooms),
		IncludeExpired: includeExpired,
	})
	if err != nil || len(rows) == 0 {
		return nil
	}

	out := make([]HotelResult, 0, len(rows))
	for _, o := range rows {
		out = append(out, hotelResultFromOffer(provider, o))
	}

	return out
}

// flightResultFromOffer maps an inventory row back to the client DTO.
func flightResultFromOffer(provider gen.TravelProvider, o gen.TravelFlightOffer) FlightResult {
	r := FlightResult{
		Provider:   provider.Name,
		Price:      numericToFloat(o.Price),
		Currency:   o.Currency,
		BookingURL: o.BookingUrl,
		OfferID:    providerOfferID(o.OfferKey),
	}

	if o.Airline != nil {
		r.Airline = *o.Airline
	}

	if o.AirlineKey != nil {
		r.AirlineKey = *o.AirlineKey
	}

	if o.FlightNumber != nil {
		r.FlightNumber = *o.FlightNumber
	}

	if o.Stops != nil {
		r.Stops = int(*o.Stops)
	}

	if o.Duration != nil {
		r.Duration = *o.Duration
	}

	if o.DepartTs.Valid {
		r.DepartureTime = o.DepartTs.Time.UTC().Format(time.RFC3339)
	}

	if o.ArriveTs.Valid {
		r.ArrivalTime = o.ArriveTs.Time.UTC().Format(time.RFC3339)
	}

	return r
}

// hotelResultFromOffer mirrors flightResultFromOffer for hotel offers.
func hotelResultFromOffer(provider gen.TravelProvider, o gen.TravelHotelOffer) HotelResult {
	r := HotelResult{
		Provider:   provider.Name,
		Name:       o.Name,
		Price:      numericToFloat(o.Price),
		Currency:   o.Currency,
		BookingURL: o.BookingUrl,
		OfferID:    providerOfferID(o.OfferKey),
	}

	if o.StarRating != nil {
		r.StarRating = int(*o.StarRating)
	}

	if o.Address != nil {
		r.Address = *o.Address
	}

	if o.ImageUrl != nil {
		r.ImageURL = *o.ImageUrl
	}

	if o.Checkin.Valid {
		r.CheckIn = o.Checkin.Time.Format("2006-01-02")
	}

	if o.Checkout.Valid {
		r.CheckOut = o.Checkout.Time.Format("2006-01-02")
	}

	return r
}

// hotelDirectoryItemFromRow maps a travel_hotels row to the public DTO.
func hotelDirectoryItemFromRow(h gen.TravelHotel) HotelDirectoryItem {
	item := HotelDirectoryItem{
		Name:    h.Name,
		City:    h.CityKey,
		HotelID: strings.TrimPrefix(h.HotelKey, "id:"),
	}

	if strings.HasPrefix(h.HotelKey, "h:") {
		item.HotelID = ""
	}

	if h.StarRating != nil {
		item.StarRating = int(*h.StarRating)
	}

	if h.Address != nil {
		item.Address = *h.Address
	}

	if h.ImageUrl != nil {
		item.ImageURL = *h.ImageUrl
	}

	if h.LastCurrency != nil {
		item.LastCurrency = strings.TrimSpace(*h.LastCurrency)
	}

	if h.LastPrice.Valid {
		p := numericToFloat(h.LastPrice)
		item.LastPrice = &p
	}

	return item
}

// ---------------------------------------------------------------------------
// Presentation — sort/filter/page applied at serve time (ADR-124)
// ---------------------------------------------------------------------------

// applyFlightPresentation filters, sorts, and paginates a flight offer set.
// Returns the page and the pre-pagination total.
func applyFlightPresentation(in []FlightResult, opts SearchOptions) ([]FlightResult, int) {
	filtered := make([]FlightResult, 0, len(in))

	for _, r := range in {
		if opts.Airline != "" &&
			!strings.EqualFold(r.Airline, opts.Airline) &&
			(opts.airlineKey == "" || r.AirlineKey != opts.airlineKey) {
			continue
		}

		if opts.MaxStops >= 0 && r.Stops > opts.MaxStops {
			continue
		}

		if opts.PriceMax > 0 && r.Price > opts.PriceMax {
			continue
		}

		filtered = append(filtered, r)
	}

	switch opts.Sort {
	case "price_asc":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Price < filtered[j].Price })
	case "price_desc":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Price > filtered[j].Price })
	case "duration":
		sort.SliceStable(filtered, func(i, j int) bool {
			return durationMinutes(filtered[i].Duration) < durationMinutes(filtered[j].Duration)
		})
	case "stops":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Stops < filtered[j].Stops })
	case "departure":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].DepartureTime < filtered[j].DepartureTime })
	}

	return pageSlice(filtered, opts.Page, opts.PageSize)
}

// applyHotelPresentation mirrors applyFlightPresentation for hotel offers.
func applyHotelPresentation(in []HotelResult, opts SearchOptions) ([]HotelResult, int) {
	filtered := make([]HotelResult, 0, len(in))

	for _, r := range in {
		if opts.PriceMax > 0 && r.Price > opts.PriceMax {
			continue
		}

		if opts.MinStars > 0 && r.StarRating < opts.MinStars {
			continue
		}

		filtered = append(filtered, r)
	}

	switch opts.Sort {
	case "price_asc":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Price < filtered[j].Price })
	case "price_desc":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Price > filtered[j].Price })
	case "rating":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].StarRating > filtered[j].StarRating })
	case "name_asc":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })
	case "name_desc":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Name > filtered[j].Name })
	}

	return pageSlice(filtered, opts.Page, opts.PageSize)
}

// pageSlice applies page/page_size pagination. PageSize <= 0 returns the
// full set — the legacy contract for clients that paginate themselves.
func pageSlice[T any](in []T, page, pageSize int) ([]T, int) {
	total := len(in)
	if pageSize <= 0 {
		return in, total
	}

	if page < 1 {
		page = 1
	}

	lo := (page - 1) * pageSize
	if lo > total {
		lo = total
	}

	hi := lo + pageSize
	if hi > total {
		hi = total
	}

	return in[lo:hi], total
}

// durationRe parses provider duration strings: "6h30m", "6h 30m", "PT6H30M".
var durationRe = regexp.MustCompile(`(?i)(\d+)\s*h(?:ours?)?\s*(\d+)?`)

var (
	airlineIATARe = regexp.MustCompile(`^[A-Za-z0-9]{2}$`)
	airlineSlugRe = regexp.MustCompile(`[^a-z0-9]+`)
)

// durationMinutes converts a duration string to minutes for sorting.
// Unparseable values sort last.
func durationMinutes(s string) int {
	const unparseable = 1 << 30

	s = strings.TrimSpace(s)
	if s == "" {
		return unparseable
	}

	if m := durationRe.FindStringSubmatch(s); m != nil {
		hours, _ := strconv.Atoi(m[1])

		mins := 0
		if len(m) > 2 && m[2] != "" {
			mins, _ = strconv.Atoi(m[2])
		}

		return hours*60 + mins
	}

	if i := strings.Index(s, ":"); i > 0 { // "6:30"
		hours, err1 := strconv.Atoi(s[:i])
		mins, err2 := strconv.Atoi(s[i+1:])

		if err1 == nil && err2 == nil {
			return hours*60 + mins
		}
	}

	if n, err := strconv.Atoi(s); err == nil { // bare minutes
		return n
	}

	return unparseable
}

// ---------------------------------------------------------------------------
// Small conversion helpers
// ---------------------------------------------------------------------------

// floatToNumeric converts a float64 to pgtype.Numeric (same pattern as
// wallet.float64ToNumeric).
func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(fmt.Sprintf("%f", f)); err != nil {
		return pgtype.Numeric{}
	}

	return n
}

// numericToFloat converts a pgtype.Numeric back to float64.
func numericToFloat(n pgtype.Numeric) float64 {
	f, err := n.Float64Value()
	if err != nil {
		return 0
	}

	return f.Float64
}

// offerCurrency normalizes a currency code for the char(3) column.
func offerCurrency(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	if len(c) > 3 {
		c = c[:3]
	}

	return c
}

// strPtrOrNil returns nil for empty strings (nullable columns).
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

// int16PtrOrNil returns nil when the value is meaningless; keep=true stores
// the value (including a legitimate 0 like non-stop flights).
func int16PtrOrNil(v int, keep bool) *int16 {
	if !keep {
		return nil
	}

	i := toInt16(v)

	return &i
}

// toInt16 clamps v to the int16 range so request-derived or provider-derived
// counts can never wrap negative on conversion (gosec G109/G115).
func toInt16(v int) int16 {
	if v > math.MaxInt16 {
		return math.MaxInt16
	}

	if v < math.MinInt16 {
		return math.MinInt16
	}

	return int16(v)
}

// toInt32 clamps v to the int32 range (gosec G109/G115).
func toInt32(v int64) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}

	if v < math.MinInt32 {
		return math.MinInt32
	}

	return int32(v)
}

// tsFromString parses an RFC3339-ish timestamp into a nullable timestamptz.
func tsFromString(s string) pgtype.Timestamptz {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return pgtype.Timestamptz{Time: t, Valid: true}
		}
	}

	return pgtype.Timestamptz{}
}

// serveFlights applies presentation (filter/sort/page) over an offer set and
// builds the response envelope. Affiliate injection is the caller's job —
// cached payloads arrive already stamped, stored/inventory payloads must be
// injected first.
func serveFlights(results []FlightResult, opts SearchOptions, source string, cacheHit bool, start time.Time) *SearchResponse {
	page, total := applyFlightPresentation(results, opts)

	out := make([]interface{}, len(page))
	for i, r := range page {
		out[i] = r
	}

	return &SearchResponse{
		Results:      out,
		CacheHit:     cacheHit,
		Source:       source,
		SearchTimeMs: time.Since(start).Milliseconds(),
		Total:        total,
		Page:         opts.Page,
		PageSize:     opts.PageSize,
	}
}

// serveHotels mirrors serveFlights for hotel offers.
func serveHotels(results []HotelResult, opts SearchOptions, source string, cacheHit bool, start time.Time) *SearchResponse {
	page, total := applyHotelPresentation(results, opts)

	out := make([]interface{}, len(page))
	for i, r := range page {
		out[i] = r
	}

	return &SearchResponse{
		Results:      out,
		CacheHit:     cacheHit,
		Source:       source,
		SearchTimeMs: time.Since(start).Milliseconds(),
		Total:        total,
		Page:         opts.Page,
		PageSize:     opts.PageSize,
	}
}

// SearchFlights serves a flight search through the layered result store (§67,
// ADR-124): (1) Valkey cache keyed on the coverage hash, (2) offer inventory
// under fresh coverage (when travel.inventory_enabled), (3) fresh
// travel_search_results replay row, (4) quota-gated singleflight provider
// call persisted to BOTH the inventory and the replay row. Presentation
// params are excluded from every key and applied at serve time. Stored data
// (fresh or stale) is served even when no provider is currently active.
func (s *Service) SearchFlights(ctx context.Context, params FlightSearchParams, opts SearchOptions) (*SearchResponse, error) {
	if !s.IsEnabled(ctx) {
		return nil, ErrTravelDisabled
	}

	start := time.Now()
	coverageKey := flightCoverageHash(params)
	cacheKey := valkey.MustKey("travel:search:%s", coverageKey)
	inventoryOn := s.inventoryEnabled(ctx)

	// Canonical airline identity for the ?airline= filter: providers send
	// inconsistent raw values ("BA" vs "British Airways"), so resolve the
	// param to a travel_airlines key and match offer airline_key too.
	// Read-only lookup — a filter must not mutate the directory.
	if opts.Airline != "" {
		if row, err := s.lookupAirline(ctx, opts.Airline); err == nil {
			opts.airlineKey = row.AirlineKey
		}
	}

	// (1) Check Valkey cache — payload is the full pre-presentation offer
	// set (post-affiliate), so every sort/filter/page shape hits it.
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var results []FlightResult
			if err := json.Unmarshal([]byte(cached), &results); err == nil {
				return serveFlights(results, opts, "cache", true, start), nil
			}
		}
	}

	// (2) Load the replay row and the active provider. The provider lookup
	// soft-fails — it is only required for a live fetch and the inventory
	// lookup; stored rows are served regardless.
	stored := s.storedSearchRow(ctx, ProviderTypeFlight, coverageKey)

	provider, err := s.store.GetActiveProvider(ctx, ProviderTypeFlight)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("travel: get active flight provider: %w", err)
	}

	hasProvider := err == nil

	var affiliate json.RawMessage

	var providerPtr *gen.TravelProvider

	if hasProvider {
		affiliate = provider.AffiliateConfig
		providerPtr = &provider
	}

	// (3) Offer inventory — fresh coverage row means this query shape was
	// fetched recently; serve the indexed offers with zero upstream calls.
	// Served offers never touched the provider, so cache_hit stays false.
	if inventoryOn && hasProvider {
		if offers := s.inventoryFlights(ctx, provider, params, coverageKey, false); len(offers) > 0 {
			injectAffiliateIntoFlights(offers, affiliate)
			s.cacheSearchResults(ctx, cacheKey, offers)

			return serveFlights(offers, opts, "inventory", false, start), nil
		}
	}

	// (4) Fresh replay row — the exact-query fallback (also coverage-keyed
	// now, so presentation variants share it). Stored payloads are
	// pre-affiliate, so inject before serving. Like inventory, a replay row
	// never touched the provider on this request, so cache_hit stays false.
	if stored != nil && stored.ExpiresAt.Valid && stored.ExpiresAt.Time.After(time.Now()) {
		var results []FlightResult
		if err := json.Unmarshal(stored.Results, &results); err == nil {
			injectAffiliateIntoFlights(results, affiliate)

			if len(results) > 0 {
				s.cacheSearchResults(ctx, cacheKey, results)
			}

			s.store.BumpSearchResultHit(ctx, stored.ID)

			return serveFlights(results, opts, "stored", false, start), nil
		}
	}

	// (5) Live provider call — the only path that requires a provider. When
	// none is configured, fall back to the stale row before erroring.
	if !hasProvider {
		if resp := s.staleFlightResponse(ctx, stored, affiliate, params, coverageKey, providerPtr, opts, start); resp != nil {
			s.store.BumpSearchResultHit(ctx, stored.ID)
			return resp, nil
		}

		return nil, ErrNoProvider
	}

	apiCfg, err := decryptAPIConfig(provider)
	if err != nil {
		if resp := s.staleFlightResponse(ctx, stored, affiliate, params, coverageKey, providerPtr, opts, start); resp != nil {
			slog.Warn("travel: provider config unusable, serving stale results", "error", err)
			s.store.BumpSearchResultHit(ctx, stored.ID)

			return resp, nil
		}

		return nil, fmt.Errorf("travel: flight search: %w", err)
	}

	// Concurrent identical-coverages share this provider call — the shared
	// function also persists results (replay row + inventory offers +
	// coverage) exactly once. The quota counter ticks inside the shared
	// function so it measures real upstream calls, not requests. The fetch
	// runs detached from any single caller's context so a leader disconnect
	// can't fail every waiter; the HTTP client's own timeout still bounds it.
	shared, err, _ := s.searchGroup.Do("flight:"+coverageKey, func() (interface{}, error) {
		fctx := context.WithoutCancel(ctx)

		if err := s.acquireProviderQuota(fctx); err != nil {
			return nil, err
		}

		results, err := s.api.SearchFlights(fctx, apiCfg, params)
		if err != nil {
			return nil, err
		}

		s.persistSearchResult(fctx, ProviderTypeFlight, coverageKey, params, results, provider.ID)

		if inventoryOn {
			s.persistFlightOffers(fctx, provider, params, results, coverageKey)
		}

		return results, nil
	})
	if err != nil {
		// Provider down/quota exhausted — serve stale inventory offers first,
		// then the stale replay row. Stale offers beat an error.
		if resp := s.staleFlightResponse(ctx, stored, affiliate, params, coverageKey, providerPtr, opts, start); resp != nil {
			slog.Warn("travel: provider failed, serving stale results", "error", err)
			s.store.BumpSearchResultHit(ctx, stored.ID)

			return resp, nil
		}

		if errors.Is(err, ErrQuotaExceeded) {
			return nil, ErrQuotaExceeded
		}

		if errors.Is(err, errProviderUnconfigured) {
			return nil, ErrNoProvider
		}

		return nil, fmt.Errorf("travel: flight search: %w", err)
	}

	// Copy the shared slice before mutating — another singleflight caller may
	// be marshaling it concurrently.
	results := append([]FlightResult(nil), shared.([]FlightResult)...)

	// (6) Stamp affiliate params, present, cache, return.
	injectAffiliateIntoFlights(results, affiliate)

	if len(results) > 0 {
		s.cacheSearchResults(ctx, cacheKey, results)
	}

	return serveFlights(results, opts, "provider", false, start), nil
}

// SearchHotels serves a hotel search through the same layered result store
// as SearchFlights (§67, ADR-124).
func (s *Service) SearchHotels(ctx context.Context, params HotelSearchParams, opts SearchOptions) (*SearchResponse, error) {
	if !s.IsEnabled(ctx) {
		return nil, ErrTravelDisabled
	}

	start := time.Now()
	coverageKey := hotelCoverageHash(params)
	cacheKey := valkey.MustKey("travel:search:%s", coverageKey)
	inventoryOn := s.inventoryEnabled(ctx)

	// (1) Check Valkey cache — pre-presentation offer set per coverage.
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var results []HotelResult
			if err := json.Unmarshal([]byte(cached), &results); err == nil {
				return serveHotels(results, opts, "cache", true, start), nil
			}
		}
	}

	// (2) Load the replay row and the active provider — same soft-fail
	// semantics as SearchFlights.
	stored := s.storedSearchRow(ctx, ProviderTypeHotel, coverageKey)

	provider, err := s.store.GetActiveProvider(ctx, ProviderTypeHotel)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("travel: get active hotel provider: %w", err)
	}

	hasProvider := err == nil

	var affiliate json.RawMessage

	var providerPtr *gen.TravelProvider

	if hasProvider {
		affiliate = provider.AffiliateConfig
		providerPtr = &provider
	}

	// (3) Offer inventory — fresh coverage serves the indexed offers with
	// zero upstream calls (ADR-124). cache_hit stays false: only the Valkey
	// layer counts as a cache hit.
	if inventoryOn && hasProvider {
		if offers := s.inventoryHotels(ctx, provider, params, coverageKey, false); len(offers) > 0 {
			injectAffiliateIntoHotels(offers, affiliate)
			s.cacheSearchResults(ctx, cacheKey, offers)

			return serveHotels(offers, opts, "inventory", false, start), nil
		}
	}

	// (4) Fresh replay row. cache_hit stays false — same reasoning as flights.
	if stored != nil && stored.ExpiresAt.Valid && stored.ExpiresAt.Time.After(time.Now()) {
		var results []HotelResult
		if err := json.Unmarshal(stored.Results, &results); err == nil {
			injectAffiliateIntoHotels(results, affiliate)

			if len(results) > 0 {
				s.cacheSearchResults(ctx, cacheKey, results)
			}

			s.store.BumpSearchResultHit(ctx, stored.ID)

			return serveHotels(results, opts, "stored", false, start), nil
		}
	}

	// (5) Provider call — the only path that requires a provider.
	if !hasProvider {
		if resp := s.staleHotelResponse(ctx, stored, affiliate, params, coverageKey, providerPtr, opts, start); resp != nil {
			s.store.BumpSearchResultHit(ctx, stored.ID)
			return resp, nil
		}

		return nil, ErrNoProvider
	}

	apiCfg, err := decryptAPIConfig(provider)
	if err != nil {
		if resp := s.staleHotelResponse(ctx, stored, affiliate, params, coverageKey, providerPtr, opts, start); resp != nil {
			slog.Warn("travel: provider config unusable, serving stale results", "error", err)
			s.store.BumpSearchResultHit(ctx, stored.ID)

			return resp, nil
		}

		return nil, fmt.Errorf("travel: hotel search: %w", err)
	}

	// Concurrent identical-coverages share this provider call — same dedup
	// semantics as SearchFlights, plus the daily quota ceiling.
	shared, err, _ := s.searchGroup.Do("hotel:"+coverageKey, func() (interface{}, error) {
		fctx := context.WithoutCancel(ctx)

		if err := s.acquireProviderQuota(fctx); err != nil {
			return nil, err
		}

		results, err := s.api.SearchHotels(fctx, apiCfg, params)
		if err != nil {
			return nil, err
		}

		s.persistSearchResult(fctx, ProviderTypeHotel, coverageKey, params, results, provider.ID)

		if inventoryOn {
			s.persistHotelOffers(fctx, provider, params, results, coverageKey)
		}

		return results, nil
	})
	if err != nil {
		if resp := s.staleHotelResponse(ctx, stored, affiliate, params, coverageKey, providerPtr, opts, start); resp != nil {
			slog.Warn("travel: provider failed, serving stale results", "error", err)
			s.store.BumpSearchResultHit(ctx, stored.ID)

			return resp, nil
		}

		if errors.Is(err, ErrQuotaExceeded) {
			return nil, ErrQuotaExceeded
		}

		if errors.Is(err, errProviderUnconfigured) {
			return nil, ErrNoProvider
		}

		return nil, fmt.Errorf("travel: hotel search: %w", err)
	}

	results := append([]HotelResult(nil), shared.([]HotelResult)...)

	// (6) Inject affiliate params, present, cache, return.
	injectAffiliateIntoHotels(results, affiliate)

	if len(results) > 0 {
		s.cacheSearchResults(ctx, cacheKey, results)
	}

	return serveHotels(results, opts, "provider", false, start), nil
}

// SearchLocations serves the /travel/locations autocomplete. The seeded
// travel_airports catalog is the primary source; when the active flight
// provider exposes a locations endpoint (api_config.extra.locations_endpoint)
// its matches are merged in (deduped by IATA). Provider failures degrade to
// local results only (§67). Disabled surfaces return ErrTravelDisabled — the
// same gate as the search paths — so the endpoint is never reachable while
// travel.enabled is off.
func (s *Service) SearchLocations(ctx context.Context, q string, limit int32) ([]LocationResult, error) {
	if !s.IsEnabled(ctx) {
		return nil, ErrTravelDisabled
	}

	q = normalizeCoverageString(q)

	if q == "" {
		return []LocationResult{}, nil
	}

	if limit <= 0 {
		limit = 15
	}

	if limit > 50 {
		limit = 50
	}

	rows, err := s.store.SearchAirports(ctx, q, limit)
	if err != nil {
		return nil, err
	}

	out := make([]LocationResult, 0, len(rows))
	seen := make(map[string]bool, len(rows))

	for _, r := range rows {
		seen[r.IataCode] = true

		out = append(out, LocationResult{
			IATA:        r.IataCode,
			Name:        r.Name,
			City:        r.City,
			CountryCode: r.CountryCode,
		})
	}

	// Provider locations merge — provider responses are cached in Valkey
	// (§67 quota controls) so autocomplete keystrokes don't each cost an
	// upstream call.
	if len(out) < int(limit) {
		if provider, err := s.store.GetActiveProvider(ctx, ProviderTypeFlight); err == nil {
			if apiCfg, derr := decryptAPIConfig(provider); derr == nil {
				for _, loc := range s.providerLocations(ctx, apiCfg, q) {
					code := strings.ToUpper(loc.IATA)
					if code == "" || seen[code] {
						continue
					}

					seen[code] = true
					loc.IATA = code

					out = append(out, loc)

					if len(out) >= int(limit) {
						break
					}
				}
			}
		}
	}

	if len(out) > int(limit) {
		out = out[:limit]
	}

	return out, nil
}

// providerLocations returns the provider's location matches for q through a
// Valkey read-through cache (jc:travel:locations:{sha256(q)}, §67). Airport
// catalogs change rarely, so successful responses are cached for
// locationsCacheTTL (default 24h); failures are negative-cached for 60s so
// a dead endpoint isn't retried per keystroke. The upstream call itself is
// bounded to 3s so a slow endpoint can't stall the autocomplete path.
func (s *Service) providerLocations(ctx context.Context, apiCfg json.RawMessage, q string) []LocationResult {
	var key string
	if s.cache != nil {
		key = valkey.MustKey("travel:locations:%s", HashSearchQuery(q))

		if cached, err := s.cache.Get(ctx, key).Result(); err == nil && cached != "" {
			var locs []LocationResult
			if json.Unmarshal([]byte(cached), &locs) == nil {
				return locs
			}
		}
	}

	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	remote, err := s.api.SearchLocations(pctx, apiCfg, q)
	if err != nil {
		if key != "" {
			_ = s.cache.Set(ctx, key, "[]", locationsNegativeTTL).Err()
		}

		return nil
	}

	if key != "" {
		if data, merr := json.Marshal(remote); merr == nil {
			_ = s.cache.Set(ctx, key, data, s.locationsCacheTTL(ctx)).Err()
		}
	}

	return remote
}

// ---------------------------------------------------------------------------
// Admin: provider management
// ---------------------------------------------------------------------------

// GetActiveProvider returns the active provider for a given type. Used by the
// search proxy and exposed for admin introspection.
func (s *Service) GetActiveProvider(ctx context.Context, providerType string) (ProviderInfo, error) {
	p, err := s.store.GetActiveProvider(ctx, providerType)
	if err != nil {
		return ProviderInfo{}, fmt.Errorf("travel: get active provider: %w", err)
	}

	return providerToInfo(p), nil
}

// CreateProvider creates a new travel provider (admin). Invalidates the
// platform's search result cache so new searches use the new provider.
func (s *Service) CreateProvider(ctx context.Context, arg gen.CreateTravelProviderParams) (ProviderInfo, error) {
	if arg.ProviderType != ProviderTypeFlight && arg.ProviderType != ProviderTypeHotel && arg.ProviderType != ProviderTypeBoth {
		return ProviderInfo{}, fmt.Errorf("travel: invalid provider_type %q", arg.ProviderType)
	}

	p, err := s.store.CreateProvider(ctx, arg)
	if err != nil {
		return ProviderInfo{}, fmt.Errorf("travel: create provider: %w", err)
	}

	s.invalidateSearchCache(ctx)

	return providerToInfo(p), nil
}

// UpdateProvider updates a travel provider's mutable fields (admin).
// Invalidates the platform's search result cache so new searches reflect the
// updated provider config (affiliate params, api_config, is_active).
func (s *Service) UpdateProvider(ctx context.Context, arg gen.UpdateTravelProviderParams) (ProviderInfo, error) {
	if arg.ProviderType != nil && *arg.ProviderType != ProviderTypeFlight && *arg.ProviderType != ProviderTypeHotel && *arg.ProviderType != ProviderTypeBoth {
		return ProviderInfo{}, fmt.Errorf("travel: invalid provider_type %q", *arg.ProviderType)
	}

	p, err := s.store.UpdateProvider(ctx, arg)
	if err != nil {
		return ProviderInfo{}, fmt.Errorf("travel: update provider: %w", err)
	}

	s.invalidateSearchCache(ctx)

	return providerToInfo(p), nil
}

// MergeAPIConfigPatch resolves the api_config JSON to persist for a PATCH
// (§67 admin api_config PATCH semantics). When the incoming config carries a
// blank endpoint AND api_key but a non-empty extra object, extra is merged
// into the provider's existing decrypted config — an "edit only Extra
// Settings" update (e.g. adding locations_endpoint) preserves credentials
// instead of wiping them. In every other case the incoming config replaces
// the stored one wholesale. Unknown top-level keys in the stored config are
// preserved.
func (s *Service) MergeAPIConfigPatch(ctx context.Context, providerID pgtype.UUID, incoming json.RawMessage) (json.RawMessage, error) {
	var inc struct {
		Endpoint string          `json:"endpoint"`
		APIKey   string          `json:"api_key"`
		Extra    json.RawMessage `json:"extra"`
	}

	if err := json.Unmarshal(incoming, &inc); err != nil {
		return nil, fmt.Errorf("travel: parse api_config: %w", err)
	}

	if inc.Endpoint != "" || inc.APIKey != "" || len(inc.Extra) == 0 {
		return incoming, nil
	}

	provider, err := s.store.GetProviderByID(ctx, providerID)
	if err != nil {
		return nil, err
	}

	existing := map[string]json.RawMessage{}

	if len(provider.ApiConfig) > 0 {
		dec, err := crypto.DecryptJSON(provider.ApiConfig)
		if err != nil {
			return nil, fmt.Errorf("travel: decrypt existing api_config: %w", err)
		}

		_ = json.Unmarshal(dec, &existing)
	}

	extra := map[string]json.RawMessage{}
	if raw, ok := existing["extra"]; ok {
		_ = json.Unmarshal(raw, &extra)
	}

	incExtra := map[string]json.RawMessage{}
	_ = json.Unmarshal(inc.Extra, &incExtra)

	for k, v := range incExtra {
		extra[k] = v
	}

	mergedExtra, err := json.Marshal(extra)
	if err != nil {
		return nil, fmt.Errorf("travel: merge api_config extra: %w", err)
	}

	existing["extra"] = mergedExtra

	out, err := json.Marshal(existing)
	if err != nil {
		return nil, fmt.Errorf("travel: merge api_config: %w", err)
	}

	return out, nil
}

// DeleteProvider deletes a travel provider by id (admin).
// Invalidates the platform's search result cache so searches no longer use a
// deleted provider.
func (s *Service) DeleteProvider(ctx context.Context, id pgtype.UUID) error {
	if err := s.store.DeleteProvider(ctx, id); err != nil {
		return fmt.Errorf("travel: delete provider: %w", err)
	}

	s.invalidateSearchCache(ctx)

	return nil
}

// invalidateSearchCache flushes all cached travel search results for a platform.
// Uses SCAN (not KEYS) to find jc:travel:search:* keys and batch-deletes them.
// Best-effort: errors are logged but do not fail the caller's operation.
func (s *Service) invalidateSearchCache(ctx context.Context) {
	if s.cache == nil {
		return
	}

	pattern := valkey.MustKey("travel:search:*")

	var cursor uint64

	var keys []string

	for {
		scanKeys, nextCursor, err := s.cache.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Warn("travel: cache invalidation scan failed", "error", err)
			return
		}

		keys = append(keys, scanKeys...)

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if len(keys) > 0 {
		if err := s.cache.Del(ctx, keys...).Err(); err != nil {
			slog.Warn("travel: cache invalidation delete failed", "error", err)
		}
	}
}

// ListProviders lists travel providers for a platform (admin). isActiveFilter
// is tri-state: "true" = only active, "false" = only inactive, "" = all. q
// is an ILIKE search on name/slug (empty = no filter).
func (s *Service) ListProviders(ctx context.Context, isActiveFilter, providerType, q string) ([]ProviderInfo, error) {
	rows, err := s.store.ListProviders(ctx, isActiveFilter, providerType, q)
	if err != nil {
		return nil, err
	}

	providers := make([]ProviderInfo, 0, len(rows))
	for _, p := range rows {
		providers = append(providers, providerToInfo(p))
	}

	return providers, nil
}

// ListPublicProviders lists active travel providers for a platform in a
// public-safe shape. Only active providers are returned, and affiliate/api
// config are omitted.
func (s *Service) ListPublicProviders(ctx context.Context) ([]PublicProviderInfo, error) {
	rows, err := s.store.ListProviders(ctx, "true", "", "")
	if err != nil {
		return nil, err
	}

	providers := make([]PublicProviderInfo, 0, len(rows))

	for _, p := range rows {
		if !p.IsActive {
			continue
		}

		providers = append(providers, providerToPublicInfo(p))
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

// providerToPublicInfo maps a gen.TravelProvider to the public-safe
// PublicProviderInfo DTO.
func providerToPublicInfo(p gen.TravelProvider) PublicProviderInfo {
	created := ""
	if p.CreatedAt.Valid {
		created = p.CreatedAt.Time.UTC().Format(time.RFC3339)
	}

	return PublicProviderInfo{
		ID:           p.ID.String(),
		Name:         p.Name,
		Slug:         p.Slug,
		ProviderType: p.ProviderType,
		IsActive:     p.IsActive,
		CreatedAt:    created,
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
