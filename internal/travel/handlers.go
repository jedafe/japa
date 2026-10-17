// Package travel — handlers.go provides the HTTP handlers for the Travel
// Search & Compare routes (Section 67, ADR-085).
//
// Public routes (no auth — travel.search permission covers all users
// including anonymous):
//
//	GET    /api/v1/travel/search/flights   — ?origin=&dest=&date=&adults=&return_date=&cabin_class=
//	GET    /api/v1/travel/search/hotels    — ?dest=&checkin=&checkout=&guests=&rooms=
//
// Admin routes (travel.manage permission):
//
//	GET    /api/v1/admin/travel/providers       — list configured providers
//	POST   /api/v1/admin/travel/providers       — add provider
//	PATCH  /api/v1/admin/travel/providers/:id   — update provider config
//	DELETE /api/v1/admin/travel/providers/:id   — remove provider
//
// Ref: japaconnect-engineering/sections/67-travel-search.md
//
//	japaconnect-engineering/adr/ADR-085-travel-search.md
package travel

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

// Handlers exposes the travel search and admin management endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	lit      literalResolver // nil disables literal resolution (§14)
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// SetLiteralResolver injects the template-literal catalog so user-facing
// titles/descriptions render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a response DTO or slice.
// Best-effort — failures are logged inside the resolver.
func (h *Handlers) resolveLiterals(r *http.Request, v any) {
	if literals.IsRawRequest(r) {
		return
	}

	if h.lit == nil {
		return
	}

	h.lit.ResolveFields(r.Context(), v, literals.NewRequestContext(r, ""))
}

// NewHandlers creates travel handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as rag/handlers.go, remittance/handlers.go)
// ---------------------------------------------------------------------------

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}

	return id, true
}

// atoiDefault parses a query param as int, returning fallback on error/empty.
func atoiDefault(s string, fallback int) int {
	if s == "" {
		return fallback
	}

	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}

	return v
}

// floatDefault parses a query param as float64, returning fallback on
// error/empty.
func floatDefault(s string, fallback float64) float64 {
	if s == "" {
		return fallback
	}

	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}

	return v
}

// maxPageSize caps page_size so a client can't request unbounded pages.
const maxPageSize = 100

// validateDate checks that a date string is in YYYY-MM-DD format and is a
// valid calendar date. Returns true when valid.
func validateDate(s string) bool {
	if s == "" {
		return false
	}

	if _, err := time.Parse("2006-01-02", s); err != nil {
		return false
	}

	return true
}

// validatePositiveInt returns the int if it is >= min, otherwise fallback.
func validatePositiveInt(v, fallback, min int) int {
	if v < min {
		return fallback
	}

	return v
}

// writeConflictIfDuplicate maps a pgx unique-violation (SQLSTATE 23505) to a
// 409 duplicate_provider error envelope (Rule 5). Returns true when the error
// was handled (response already written); false when the caller should handle
// the error itself.
func writeConflictIfDuplicate(w http.ResponseWriter, err error, code, message string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		errpkg.WriteError(w, http.StatusConflict, code, message)
		return true
	}

	return false
}

// ---------------------------------------------------------------------------
// Public search routes
// ---------------------------------------------------------------------------

// HandleSearchFlights handles GET /api/v1/travel/search/flights — proxy a
// flight search to the configured provider.
// Query: origin, dest, date, adults, return_date, cabin_class.
func (h *Handlers) HandleSearchFlights(w http.ResponseWriter, r *http.Request) {
	origin := r.URL.Query().Get("origin")
	dest := r.URL.Query().Get("dest")
	date := r.URL.Query().Get("date")

	if origin == "" || dest == "" || date == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "origin, dest, and date are required")
		return
	}

	if !validateDate(date) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_date", "date must be in YYYY-MM-DD format")
		return
	}

	returnDate := r.URL.Query().Get("return_date")
	if returnDate != "" {
		if !validateDate(returnDate) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_return_date", "return_date must be in YYYY-MM-DD format")
			return
		}

		if returnDate < date {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_return_date", "return_date must be on or after date")
			return
		}
	}

	params := FlightSearchParams{
		Origin:      origin,
		Destination: dest,
		Date:        date,
		Adults:      validatePositiveInt(atoiDefault(r.URL.Query().Get("adults"), 1), 1, 1),
		Children:    validatePositiveInt(atoiDefault(r.URL.Query().Get("children"), 0), 0, 0),
		ReturnDate:  returnDate,
		CabinClass:  r.URL.Query().Get("cabin_class"),
	}

	opts := SearchOptions{
		Sort:     r.URL.Query().Get("sort"),
		Airline:  r.URL.Query().Get("airline"),
		MaxStops: atoiDefault(r.URL.Query().Get("max_stops"), -1),
		PriceMax: floatDefault(r.URL.Query().Get("price_max"), 0),
		Page:     validatePositiveInt(atoiDefault(r.URL.Query().Get("page"), 1), 1, 1),
		PageSize: validatePositiveInt(atoiDefault(r.URL.Query().Get("page_size"), 0), 0, 0),
	}
	if opts.PageSize > maxPageSize {
		opts.PageSize = maxPageSize
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	resp, err := h.service.SearchFlights(r.Context(), params, opts)
	if err != nil {
		if errors.Is(err, ErrTravelDisabled) {
			errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_disabled", "Travel search is disabled")
			return
		}

		if errors.Is(err, ErrNoProvider) {
			errpkg.WriteError(w, http.StatusServiceUnavailable, "provider_not_configured", "No flight provider is configured yet")
			return
		}

		if errors.Is(err, ErrQuotaExceeded) {
			errpkg.WriteError(w, http.StatusTooManyRequests, "travel_quota_exceeded", "Travel search is temporarily rate limited")
			return
		}

		slog.Error("travel: flight search failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "search_failed", "Failed to search flights")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}

// HandleSearchHotels handles GET /api/v1/travel/search/hotels — proxy a hotel
// search to the configured provider.
// Query: dest, checkin, checkout, guests, rooms.
func (h *Handlers) HandleSearchHotels(w http.ResponseWriter, r *http.Request) {
	dest := r.URL.Query().Get("dest")
	checkin := r.URL.Query().Get("checkin")
	checkout := r.URL.Query().Get("checkout")

	if dest == "" || checkin == "" || checkout == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "dest, checkin, and checkout are required")
		return
	}

	if !validateDate(checkin) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_checkin", "checkin must be in YYYY-MM-DD format")
		return
	}

	if !validateDate(checkout) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_checkout", "checkout must be in YYYY-MM-DD format")
		return
	}

	if checkout <= checkin {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_checkout", "checkout must be after checkin")
		return
	}

	params := HotelSearchParams{
		Destination: dest,
		CheckIn:     checkin,
		CheckOut:    checkout,
		Guests:      validatePositiveInt(atoiDefault(r.URL.Query().Get("guests"), 1), 1, 1),
		Rooms:       validatePositiveInt(atoiDefault(r.URL.Query().Get("rooms"), 1), 1, 1),
	}

	opts := SearchOptions{
		Sort:     r.URL.Query().Get("sort"),
		PriceMax: floatDefault(r.URL.Query().Get("price_max"), 0),
		MinStars: atoiDefault(r.URL.Query().Get("min_stars"), 0),
		Page:     validatePositiveInt(atoiDefault(r.URL.Query().Get("page"), 1), 1, 1),
		PageSize: validatePositiveInt(atoiDefault(r.URL.Query().Get("page_size"), 0), 0, 0),
	}
	if opts.PageSize > maxPageSize {
		opts.PageSize = maxPageSize
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	resp, err := h.service.SearchHotels(r.Context(), params, opts)
	if err != nil {
		if errors.Is(err, ErrTravelDisabled) {
			errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_disabled", "Travel search is disabled")
			return
		}

		if errors.Is(err, ErrNoProvider) {
			errpkg.WriteError(w, http.StatusServiceUnavailable, "provider_not_configured", "No hotel provider is configured yet")
			return
		}

		if errors.Is(err, ErrQuotaExceeded) {
			errpkg.WriteError(w, http.StatusTooManyRequests, "travel_quota_exceeded", "Travel search is temporarily rate limited")
			return
		}

		slog.Error("travel: hotel search failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "search_failed", "Failed to search hotels")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}

// requireTravelEnabled maps the service-level travel.enabled check to the
// same 503 travel_disabled contract as the search handlers. Locations and
// directory routes share it — they are part of the travel surface, so they
// must not stay reachable when the feature is switched off. (The
// route-level features.Gate 404s first; this covers direct callers and
// flag races.)
func (h *Handlers) requireTravelEnabled(w http.ResponseWriter, r *http.Request) bool {
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return false
	}

	if !h.service.IsEnabled(r.Context()) {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_disabled", "Travel search is disabled")
		return false
	}

	return true
}

// HandleSearchLocations handles GET /api/v1/travel/locations — airport
// autocomplete backed by the seeded IATA catalog, merged with the provider's
// locations API when configured (§67 amendment).
// Query: q (required), limit (default 15, max 50).
func (h *Handlers) HandleSearchLocations(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	if q == "" {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	if !h.requireTravelEnabled(w, r) {
		return
	}

	locs, err := h.service.SearchLocations(r.Context(), q, toInt32(int64(atoiDefault(r.URL.Query().Get("limit"), 15))))
	if err != nil {
		slog.Error("travel: locations search failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "locations_failed", "Failed to search locations")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": locs,
	})
}

// HandleHotelDirectory handles GET /api/v1/travel/hotels — browse the hotel
// directory (travel_hotels). Pure local read; rows accumulate from hotel
// searches and the travel:sync_hotel_directory feed task (§67, ADR-124).
// Query: city, q (name substring), min_stars, page, page_size (default 50).
func (h *Handlers) HandleHotelDirectory(w http.ResponseWriter, r *http.Request) {
	if !h.requireTravelEnabled(w, r) {
		return
	}

	q := r.URL.Query()

	items, total, err := h.service.HotelDirectory(
		r.Context(),
		q.Get("city"),
		q.Get("q"),
		atoiDefault(q.Get("min_stars"), 0),
		atoiDefault(q.Get("page"), 1),
		atoiDefault(q.Get("page_size"), 50),
	)
	if err != nil {
		slog.Error("travel: hotel directory failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "directory_failed", "Failed to load hotel directory")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"results":   items,
			"total":     total,
			"page":      atoiDefault(q.Get("page"), 1),
			"page_size": min(max(atoiDefault(q.Get("page_size"), 50), 1), 200),
		},
	})
}

// HandleAirlineDirectory handles GET /api/v1/travel/airlines — browse the
// airline catalog joined to observed provider coverage. Pure local read.
func (h *Handlers) HandleAirlineDirectory(w http.ResponseWriter, r *http.Request) {
	if !h.requireTravelEnabled(w, r) {
		return
	}

	q := r.URL.Query()

	items, total, err := h.service.AirlineDirectory(
		r.Context(),
		q.Get("q"),
		q.Get("country"),
		q.Get("provider"),
		atoiDefault(q.Get("page"), 1),
		atoiDefault(q.Get("page_size"), 50),
	)
	if err != nil {
		slog.Error("travel: airline directory failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "directory_failed", "Failed to load airline directory")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"results":   items,
			"total":     total,
			"page":      atoiDefault(q.Get("page"), 1),
			"page_size": min(max(atoiDefault(q.Get("page_size"), 50), 1), 200),
		},
	})
}

// HandlePublicListProviders handles GET /api/v1/travel/providers — list active
// travel providers for the current platform. Public, no auth required.
func (h *Handlers) HandlePublicListProviders(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	providers, err := h.service.ListPublicProviders(r.Context())
	if err != nil {
		slog.Error("travel: public list providers failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "providers_failed", "Failed to list travel providers")

		return
	}

	h.resolveLiterals(r, &providers)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": providers,
	})
}

// ---------------------------------------------------------------------------
// Admin routes
// ---------------------------------------------------------------------------

// HandleAdminListProviders handles GET /api/v1/admin/travel/providers — list
// configured providers. Query: is_active (tri-state: true/false/absent=all),
// provider_type, q (ILIKE search on name/slug).
func (h *Handlers) HandleAdminListProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	// Tri-state is_active filter: "true" = only active, "false" = only
	// inactive, absent = all statuses. Also accept the legacy "active_only"
	// param for backward compatibility (active_only=true → is_active=true).
	isActiveFilter := r.URL.Query().Get("is_active")
	if isActiveFilter == "" {
		if a := r.URL.Query().Get("active_only"); a == "true" {
			isActiveFilter = "true"
		}
	}

	providerType := r.URL.Query().Get("provider_type")
	q := r.URL.Query().Get("q")

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	providers, err := h.service.ListProviders(r.Context(), isActiveFilter, providerType, q)
	if err != nil {
		slog.Error("travel: admin list providers failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "providers_failed", "Failed to list providers")

		return
	}

	h.resolveLiterals(r, &providers)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": providers,
	})
}

// HandleAdminCreateProvider handles POST /api/v1/admin/travel/providers —
// create a travel provider.
// Body: {name, slug, provider_type, api_config?, affiliate_config?, is_active}.
func (h *Handlers) HandleAdminCreateProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	var body struct {
		Name            string          `json:"name"`
		Slug            string          `json:"slug"`
		ProviderType    string          `json:"provider_type"`
		APIConfig       json.RawMessage `json:"api_config"`
		AffiliateConfig json.RawMessage `json:"affiliate_config"`
		IsActive        bool            `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if body.Name == "" || body.Slug == "" || body.ProviderType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "name, slug, and provider_type are required")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	var apiConfig []byte

	if len(body.APIConfig) > 0 {
		enc, err := crypto.EncryptJSON(body.APIConfig)
		if err != nil {
			slog.Error("travel: failed to encrypt api_config", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "encryption_failed", "Failed to encrypt provider credentials")

			return
		}

		apiConfig = enc
	}

	var affiliateConfig []byte
	if len(body.AffiliateConfig) > 0 {
		affiliateConfig = body.AffiliateConfig
	}

	provider, err := h.service.CreateProvider(r.Context(), gen.CreateTravelProviderParams{
		Name:            body.Name,
		Slug:            body.Slug,
		ProviderType:    body.ProviderType,
		ApiConfig:       apiConfig,
		AffiliateConfig: affiliateConfig,
		IsActive:        body.IsActive,
	})
	if err != nil {
		if writeConflictIfDuplicate(w, err, "duplicate_provider", "A travel provider with this slug already exists") {
			return
		}

		slog.Error("travel: admin create provider failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_provider_failed", "Failed to create provider")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": provider,
	})
}

// HandleAdminUpdateProvider handles PATCH /api/v1/admin/travel/providers/:id
// — update a travel provider's config.
// Body: {name?, provider_type?, api_config?, affiliate_config?, is_active?}.
func (h *Handlers) HandleAdminUpdateProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	providerID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Name            *string         `json:"name"`
		ProviderType    *string         `json:"provider_type"`
		APIConfig       json.RawMessage `json:"api_config"`
		AffiliateConfig json.RawMessage `json:"affiliate_config"`
		IsActive        *bool           `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	var apiConfig []byte

	if len(body.APIConfig) > 0 {
		// Extra-only configs merge into the stored credentials rather than
		// replacing them (§67 admin api_config PATCH semantics).
		merged, err := h.service.MergeAPIConfigPatch(r.Context(), providerID, body.APIConfig)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "provider_not_found", "Travel provider not found")
				return
			}

			slog.Error("travel: failed to merge api_config", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "update_provider_failed", "Failed to update provider")

			return
		}

		enc, err := crypto.EncryptJSON(merged)
		if err != nil {
			slog.Error("travel: failed to encrypt api_config", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "encryption_failed", "Failed to encrypt provider credentials")

			return
		}

		apiConfig = enc
	}

	var affiliateConfig []byte
	if len(body.AffiliateConfig) > 0 {
		affiliateConfig = body.AffiliateConfig
	}

	provider, err := h.service.UpdateProvider(r.Context(), gen.UpdateTravelProviderParams{
		ID:              providerID,
		Name:            body.Name,
		ProviderType:    body.ProviderType,
		ApiConfig:       apiConfig,
		AffiliateConfig: affiliateConfig,
		IsActive:        body.IsActive,
	})
	if err != nil {
		if writeConflictIfDuplicate(w, err, "duplicate_provider", "A travel provider with this slug already exists") {
			return
		}

		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "provider_not_found", "Travel provider not found")
			return
		}

		slog.Error("travel: admin update provider failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_provider_failed", "Failed to update provider")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": provider,
	})
}

// HandleAdminDeleteProvider handles DELETE /api/v1/admin/travel/providers/:id
// — permanently remove a travel provider. Provider rows are admin-managed
// configuration, not user content, so a hard delete is correct here (Rule 11
// only covers user content).
func (h *Handlers) HandleAdminDeleteProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	providerID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	if err := h.service.DeleteProvider(r.Context(), providerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "provider_not_found", "Travel provider not found")
			return
		}

		slog.Error("travel: admin delete provider failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_provider_failed", "Failed to delete provider")

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
