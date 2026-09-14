package stock

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/crypto"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
)

// MaxStockBody is the maximum request body size for stock POST/PATCH endpoints.
const MaxStockBody int64 = 1 << 20 // 1MB — only small JSON payloads expected

// StockHandlers holds the stock service for HTTP handler methods.
// //nolint:revive // exported: stutter naming is intentional
type StockHandlers struct {
	svc *Service
}

// NewStockHandlers creates a StockHandlers with the given service.
func NewStockHandlers(svc *Service) *StockHandlers {
	return &StockHandlers{svc: svc}
}

// requireUser resolves the authenticated user from the request context.
func (h *StockHandlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return nil, false
	}

	return user, true
}

// requireTenant resolves the tenant from the request context.
func (h *StockHandlers) requireTenant(w http.ResponseWriter, r *http.Request) (*struct{ ID pgtype.UUID }, bool) {
	return &struct{ ID pgtype.UUID }{}, true
}

// hasPermission checks a single permission node via the RBAC resolver in context.
func hasPermission(r *http.Request, permKey string) bool {
	resolver := rbac.FromContext(r.Context())
	user := auth.FromContext(r.Context())

	if resolver == nil || user == nil {
		return false
	}

	return resolver.HasPermission(r.Context(), user.UserID, permKey)
}

// ---------------------------------------------------------------------------
// User-facing handlers
// ---------------------------------------------------------------------------

// ListProviders returns providers available to the authenticated user.
// GET /api/v1/stock/providers.
func (h *StockHandlers) ListProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	hasSearch := hasPermission(r, "media.stock.search")
	hasProviders := hasPermission(r, "media.stock.providers")
	names := h.svc.AvailableProvidersForPerms(hasSearch, hasProviders)

	result := make([]map[string]string, 0, len(names))
	for _, name := range names {
		result = append(result, map[string]string{
			"name":         name,
			"display_name": displayName(name),
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"providers": result}})
}

// Search searches stock photos across available providers.
// GET /api/v1/stock/search?q=...&provider=...&page=...&per_page=...
func (h *StockHandlers) Search(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	if !hasPermission(r, "media.stock.search") {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "media.stock.search permission required")
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_query", "query parameter 'q' is required")
		return
	}

	provider := r.URL.Query().Get("provider")

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}

	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage <= 0 || perPage > 80 {
		perPage = 20
	}

	// RBAC filter: if user doesn't have media.stock.providers, only allow openverse.
	hasProviders := hasPermission(r, "media.stock.providers")
	if !hasProviders && provider != "" && provider != "openverse" {
		errpkg.WriteError(w, http.StatusForbidden, "provider_unavailable", "Provider not available without media.stock.providers permission")
		return
	}

	if !hasProviders && provider == "" {
		provider = "openverse"
	}

	result, err := h.svc.Search(r.Context(), q, provider, page, perPage)
	if err != nil {
		slog.Error("stock: search failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "search_error", "Failed to search stock")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// GetPhoto returns metadata for a single stock photo.
// GET /api/v1/stock/photo/{provider}/{id}.
func (h *StockHandlers) GetPhoto(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	if !hasPermission(r, "media.stock.search") {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "media.stock.search permission required")
		return
	}

	providerName := chi.URLParam(r, "provider")
	photoID := chi.URLParam(r, "id")

	if providerName == "" || photoID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "provider and id are required")
		return
	}

	// RBAC: non-openverse providers require media.stock.providers.
	if providerName != "openverse" && !hasPermission(r, "media.stock.providers") {
		errpkg.WriteError(w, http.StatusForbidden, "provider_unavailable", "Provider not available without media.stock.providers permission")
		return
	}

	photo, err := h.svc.GetPhoto(r.Context(), providerName, photoID)
	if err != nil {
		if errors.Is(err, ErrProviderUnavailable) {
			errpkg.WriteError(w, http.StatusNotFound, "provider_unavailable", err.Error())
			return
		}

		errpkg.WriteError(w, http.StatusNotFound, "photo_not_found", err.Error())

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": photo})
}

// ImportPhoto downloads a stock photo and stores it in the media library.
// POST /api/v1/stock/import body: { "provider": "...", "id": "..." }.
func (h *StockHandlers) ImportPhoto(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxStockBody)

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	if !hasPermission(r, "media.stock.import") {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "media.stock.import permission required")
		return
	}

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	var body struct {
		Provider string `json:"provider"`
		ID       string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if body.Provider == "" || body.ID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "provider and id are required")
		return
	}

	// RBAC: provider != openverse requires media.stock.providers.
	// (media.stock.import is enforced at the route level by RequirePermission.)
	if body.Provider != "openverse" && !hasPermission(r, "media.stock.providers") {
		errpkg.WriteError(w, http.StatusForbidden, "provider_unavailable", "Provider requires media.stock.providers permission")
		return
	}

	mediaID, err := h.svc.ImportPhoto(r.Context(), user.UserID, body.Provider, body.ID)
	if err != nil {
		slog.Error("stock: import failed", "error", err, "provider", body.Provider, "photo_id", body.ID)
		errpkg.WriteError(w, http.StatusInternalServerError, "import_error", "Failed to import photo")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]string{"media_id": mediaID.String()},
	})
}

// ---------------------------------------------------------------------------
// Admin handlers
// ---------------------------------------------------------------------------

// AdminListProviders returns all providers with enabled/disabled status.
// GET /api/v1/admin/stock/providers.
func (h *StockHandlers) AdminListProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListAllProviders(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_error", "Failed to list providers")
		return
	}

	result := make([]map[string]any, 0, len(rows))

	for _, row := range rows {
		hasKey := row.APIKeyEncrypted != ""
		result = append(result, map[string]any{
			"provider_name": row.ProviderName,
			"display_name":  displayName(row.ProviderName),
			"enabled":       row.Enabled,
			"has_api_key":   hasKey,
			"updated_at":    row.UpdatedAt,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"providers": result}})
}

// AdminPatchProvider toggles enabled status or updates the API key.
// PATCH /api/v1/admin/stock/providers/{name} body: { "enabled": bool, "api_key": "..." }.
func (h *StockHandlers) AdminPatchProvider(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxStockBody)

	providerName := chi.URLParam(r, "name")
	if providerName == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_provider", "provider name is required")
		return
	}

	var body struct {
		Enabled *bool   `json:"enabled,omitempty"`
		APIKey  *string `json:"api_key,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if body.Enabled != nil {
		if err := h.svc.store.ToggleProvider(r.Context(), providerName, *body.Enabled); err != nil {
			if errors.Is(err, ErrProviderNotFound) {
				errpkg.WriteError(w, http.StatusNotFound, "provider_not_found", "Stock provider not found")
				return
			}

			errpkg.WriteError(w, http.StatusInternalServerError, "toggle_error", "Failed to toggle provider")

			return
		}
	}

	if body.APIKey != nil && *body.APIKey != "" {
		encrypted, err := crypto.Encrypt(*body.APIKey)
		if err != nil {
			slog.Error("stock: failed to encrypt API key", "provider", providerName, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "encrypt_error", "Failed to encrypt API key")

			return
		}

		if err := h.svc.store.SetAPIKey(r.Context(), providerName, encrypted); err != nil {
			if errors.Is(err, ErrProviderNotFound) {
				errpkg.WriteError(w, http.StatusNotFound, "provider_not_found", "Stock provider not found")
				return
			}

			errpkg.WriteError(w, http.StatusInternalServerError, "update_error", "Failed to update provider")

			return
		}
	}

	// Reload providers to pick up the change.
	if err := h.svc.ReloadProviders(r.Context()); err != nil {
		slog.Warn("stock: failed to reload providers after admin update", "error", err)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}

// displayName maps a provider identifier to a human-readable display name.
func displayName(name string) string {
	switch name {
	case "unsplash":
		return "Unsplash"
	case "pexels":
		return "Pexels"
	case "pixabay":
		return "Pixabay"
	case "openverse":
		return "Openverse"
	default:
		return name
	}
}
