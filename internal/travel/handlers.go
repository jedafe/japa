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
//
// Ref: japaconnect-engineering/sections/67-travel-search.md
//      japaconnect-engineering/adr/ADR-085-travel-search.md
package travel

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the travel search and admin management endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates travel handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as rag/handlers.go, remittance/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("travel: encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
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

// ---------------------------------------------------------------------------
// Public search routes
// ---------------------------------------------------------------------------

// HandleSearchFlights handles GET /api/v1/travel/search/flights — proxy a
// flight search to the configured provider.
// Query: origin, dest, date, adults, return_date, cabin_class.
func (h *Handlers) HandleSearchFlights(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	origin := r.URL.Query().Get("origin")
	dest := r.URL.Query().Get("dest")
	date := r.URL.Query().Get("date")
	if origin == "" || dest == "" || date == "" {
		writeError(w, http.StatusBadRequest, "missing_params", "origin, dest, and date are required")
		return
	}

	params := FlightSearchParams{
		Origin:      origin,
		Destination: dest,
		Date:        date,
		Adults:      atoiDefault(r.URL.Query().Get("adults"), 1),
		Children:    atoiDefault(r.URL.Query().Get("children"), 0),
		ReturnDate:  r.URL.Query().Get("return_date"),
		CabinClass:  r.URL.Query().Get("cabin_class"),
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	resp, err := h.service.SearchFlights(r.Context(), tenantID, params)
	if err != nil {
		if errors.Is(err, ErrTravelDisabled) {
			writeError(w, http.StatusServiceUnavailable, "travel_disabled", "Travel search is disabled")
			return
		}
		slog.Error("travel: flight search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "search_failed", "Failed to search flights")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}

// HandleSearchHotels handles GET /api/v1/travel/search/hotels — proxy a hotel
// search to the configured provider.
// Query: dest, checkin, checkout, guests, rooms.
func (h *Handlers) HandleSearchHotels(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	dest := r.URL.Query().Get("dest")
	checkin := r.URL.Query().Get("checkin")
	checkout := r.URL.Query().Get("checkout")
	if dest == "" || checkin == "" || checkout == "" {
		writeError(w, http.StatusBadRequest, "missing_params", "dest, checkin, and checkout are required")
		return
	}

	params := HotelSearchParams{
		Destination: dest,
		CheckIn:     checkin,
		CheckOut:    checkout,
		Guests:      atoiDefault(r.URL.Query().Get("guests"), 1),
		Rooms:       atoiDefault(r.URL.Query().Get("rooms"), 1),
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	resp, err := h.service.SearchHotels(r.Context(), tenantID, params)
	if err != nil {
		if errors.Is(err, ErrTravelDisabled) {
			writeError(w, http.StatusServiceUnavailable, "travel_disabled", "Travel search is disabled")
			return
		}
		slog.Error("travel: hotel search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "search_failed", "Failed to search hotels")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}

// ---------------------------------------------------------------------------
// Admin routes
// ---------------------------------------------------------------------------

// HandleAdminListProviders handles GET /api/v1/admin/travel/providers — list
// configured providers. Query: active_only, provider_type.
func (h *Handlers) HandleAdminListProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	activeOnly := false
	if a := r.URL.Query().Get("active_only"); a == "true" {
		activeOnly = true
	}
	providerType := r.URL.Query().Get("provider_type")

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	providers, err := h.service.ListProviders(r.Context(), tenantID, activeOnly, providerType)
	if err != nil {
		slog.Error("travel: admin list providers failed", "error", err)
		writeError(w, http.StatusInternalServerError, "providers_failed", "Failed to list providers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": providers,
	})
}

// HandleAdminCreateProvider handles POST /api/v1/admin/travel/providers —
// create a travel provider.
// Body: {name, slug, provider_type, api_config?, affiliate_config?, is_active}
func (h *Handlers) HandleAdminCreateProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if body.Name == "" || body.Slug == "" || body.ProviderType == "" {
		writeError(w, http.StatusBadRequest, "missing_params", "name, slug, and provider_type are required")
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	var apiConfig []byte
	if len(body.APIConfig) > 0 {
		enc, err := crypto.EncryptJSON(body.APIConfig)
		if err != nil {
			slog.Error("travel: failed to encrypt api_config", "error", err)
			writeError(w, http.StatusInternalServerError, "encryption_failed", "Failed to encrypt provider credentials")
			return
		}
		apiConfig = enc
	}
	var affiliateConfig []byte
	if len(body.AffiliateConfig) > 0 {
		affiliateConfig = body.AffiliateConfig
	}

	provider, err := h.service.CreateProvider(r.Context(), gen.CreateTravelProviderParams{
		TenantID:        tenantID,
		Name:            body.Name,
		Slug:            body.Slug,
		ProviderType:    body.ProviderType,
		ApiConfig:       apiConfig,
		AffiliateConfig: affiliateConfig,
		IsActive:        body.IsActive,
	})
	if err != nil {
		slog.Error("travel: admin create provider failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_provider_failed", "Failed to create provider")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": provider,
	})
}

// HandleAdminUpdateProvider handles PATCH /api/v1/admin/travel/providers/:id
// — update a travel provider's config.
// Body: {name?, provider_type?, api_config?, affiliate_config?, is_active?}
func (h *Handlers) HandleAdminUpdateProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "travel_unavailable", "Travel service not configured")
		return
	}

	var apiConfig []byte
	if len(body.APIConfig) > 0 {
		enc, err := crypto.EncryptJSON(body.APIConfig)
		if err != nil {
			slog.Error("travel: failed to encrypt api_config", "error", err)
			writeError(w, http.StatusInternalServerError, "encryption_failed", "Failed to encrypt provider credentials")
			return
		}
		apiConfig = enc
	}
	var affiliateConfig []byte
	if len(body.AffiliateConfig) > 0 {
		affiliateConfig = body.AffiliateConfig
	}

	provider, err := h.service.UpdateProvider(r.Context(), gen.UpdateTravelProviderParams{
		TenantID:        tenantID,
		ID:              providerID,
		Name:            body.Name,
		ProviderType:    body.ProviderType,
		ApiConfig:       apiConfig,
		AffiliateConfig: affiliateConfig,
		IsActive:        body.IsActive,
	})
	if err != nil {
		slog.Error("travel: admin update provider failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_provider_failed", "Failed to update provider")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": provider,
	})
}
