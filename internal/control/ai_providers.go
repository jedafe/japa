package control

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// AI Provider Management Handlers (Control Panel — super-admin)
// ---------------------------------------------------------------------------

// validProviderTypes is the set of provider_type values accepted by the
// create/update handlers. Must match the CHECK constraint in migration 000269.
var validProviderTypes = map[string]bool{
	"xiaomi":    true,
	"openai":    true,
	"ollama":    true,
	"anthropic": true,
	"devin":     true,
	"cursor":    true,
	"custom":    true,
}

// validAICategories is the set of dispatcher categories that providers can
// be registered for. This prevents typos from creating orphan categories.
var validAICategories = map[string]bool{
	"content_generation": true,
	"chat":               true,
	"embedding":          true,
	"moderation":         true,
}

// ListAIProviders handles GET /control/ai-providers.
// Returns all AI provider configs for the tenant. API keys are masked.
func (h *Handlers) ListAIProviders(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.ProviderStore == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	providers, err := h.ProviderStore.ListProviders(r.Context(), tenantID)
	if err != nil {
		slog.Error("control: list ai providers", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list AI providers")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// GetAIProvider handles GET /control/ai-providers/{id}.
func (h *Handlers) GetAIProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.ProviderStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_provider_store", "Provider store not configured")
		return
	}

	provider, err := h.ProviderStore.GetProvider(r.Context(), tenantID, id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Provider not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// CreateAIProvider handles POST /control/ai-providers.
func (h *Handlers) CreateAIProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.ProviderStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_provider_store", "Provider store not configured")
		return
	}

	var input struct {
		Name         string   `json:"name"`
		ProviderType string   `json:"provider_type"`
		APIKey       string   `json:"api_key"`
		BaseURL      string   `json:"base_url"`
		Model        string   `json:"model"`
		Categories   []string `json:"categories"`
		Priority     int      `json:"priority"`
		Enabled      *bool    `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	if !validProviderTypes[input.ProviderType] {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid provider_type")
		return
	}

	if len(input.Categories) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "at least one category is required")
		return
	}

	for _, cat := range input.Categories {
		if !validAICategories[cat] {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid category: "+cat)
			return
		}
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	provider, err := h.ProviderStore.CreateProvider(r.Context(), ai.CreateProviderInput{
		TenantID:     tenantID,
		Name:         input.Name,
		ProviderType: input.ProviderType,
		APIKey:       input.APIKey,
		BaseURL:      input.BaseURL,
		Model:        input.Model,
		Categories:   input.Categories,
		Priority:     input.Priority,
		Enabled:      enabled,
	})
	if err != nil {
		slog.Error("control: create ai provider", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create AI provider")
		return
	}

	// Hot-reload the dispatcher so the new provider is immediately available.
	h.reloadAIProviders(r.Context(), tenantID)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": provider})
}

// UpdateAIProvider handles PATCH /control/ai-providers/{id}.
func (h *Handlers) UpdateAIProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.ProviderStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_provider_store", "Provider store not configured")
		return
	}

	var input struct {
		Name         *string  `json:"name"`
		ProviderType *string  `json:"provider_type"`
		APIKey       *string  `json:"api_key"`
		BaseURL      *string  `json:"base_url"`
		Model        *string  `json:"model"`
		Categories   []string `json:"categories"`
		Priority     *int     `json:"priority"`
		Enabled      *bool    `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ProviderType != nil && !validProviderTypes[*input.ProviderType] {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid provider_type")
		return
	}

	// Categories: if the field is present in the JSON (even if empty), validate.
	// We detect presence by checking if the raw body contains the key.
	if input.Categories != nil {
		if len(input.Categories) == 0 {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "at least one category is required")
			return
		}
		for _, cat := range input.Categories {
			if !validAICategories[cat] {
				errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid category: "+cat)
				return
			}
		}
	}

	var categoriesPtr *[]string
	if input.Categories != nil {
		categoriesPtr = &input.Categories
	}

	provider, err := h.ProviderStore.UpdateProvider(r.Context(), ai.UpdateProviderInput{
		TenantID:     tenantID,
		ID:           id,
		Name:         input.Name,
		ProviderType: input.ProviderType,
		APIKey:       input.APIKey,
		BaseURL:      input.BaseURL,
		Model:        input.Model,
		Categories:   categoriesPtr,
		Priority:     input.Priority,
		Enabled:      input.Enabled,
	})
	if err != nil {
		slog.Error("control: update ai provider", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update AI provider")
		return
	}

	// Hot-reload the dispatcher so the updated config takes effect immediately.
	h.reloadAIProviders(r.Context(), tenantID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// DeleteAIProvider handles DELETE /control/ai-providers/{id}.
func (h *Handlers) DeleteAIProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.ProviderStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_provider_store", "Provider store not configured")
		return
	}

	if err := h.ProviderStore.DeleteProvider(r.Context(), tenantID, id); err != nil {
		if strings.Contains(err.Error(), "not_found") {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Provider not found")
			return
		}
		slog.Error("control: delete ai provider", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete AI provider")
		return
	}

	// Hot-reload the dispatcher so the removed provider is unregistered.
	h.reloadAIProviders(r.Context(), tenantID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// TestAIProvider handles POST /control/ai-providers/{id}/test.
// Sends a minimal test prompt through the provider to verify connectivity.
func (h *Handlers) TestAIProvider(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.ProviderStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_provider_store", "Provider store not configured")
		return
	}

	provider, err := h.ProviderStore.GetProvider(r.Context(), tenantID, id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Provider not found")
		return
	}

	apiKey, err := h.ProviderStore.GetDecryptedAPIKey(r.Context(), tenantID, id)
	if err != nil {
		slog.Error("control: test ai provider: decrypt key", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to decrypt API key")
		return
	}

	if apiKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "no_api_key", "This provider has no API key configured")
		return
	}

	p, err := ai.NewProviderFromConfig(provider, apiKey)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_type", err.Error())
		return
	}

	// Send a minimal test request through the provider.
	resp, err := p.Send(r.Context(), ai.Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt": "Hello, respond with 'OK' to confirm connectivity.",
			"task":   "test",
		},
	})
	if err != nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"status":  "failed",
				"message": err.Error(),
			},
		})
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"status":       "success",
			"provider_ref": resp.ProviderRef,
			"message":      "Provider responded successfully",
		},
	})
}

// FetchAIModels handles POST /control/ai-providers/fetch-models.
// Takes provider type, base URL, and API key inline (before the provider
// is saved) and returns the list of available models from the provider's
// /v1/models endpoint. This lets the admin pick from a dropdown instead
// of typing model names manually.
func (h *Handlers) FetchAIModels(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ProviderType string `json:"provider_type"`
		BaseURL      string `json:"base_url"`
		APIKey       string `json:"api_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if !validProviderTypes[input.ProviderType] {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid provider_type")
		return
	}

	// For existing providers (edit mode), the API key may be empty —
	// meaning "use the stored key". But for this inline fetch, we need
	// a key. If empty, return an error prompting the user to enter one.
	if input.APIKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "no_api_key", "Enter an API key to fetch available models")
		return
	}

	// Construct a temporary provider to call FetchModels.
	cfg := ai.ProviderConfig{
		Name:         "temp",
		ProviderType: input.ProviderType,
		BaseURL:      input.BaseURL,
	}
	p, err := ai.NewProviderFromConfig(cfg, input.APIKey)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_type", err.Error())
		return
	}

	models, err := p.FetchModels(r.Context())
	if err != nil {
		slog.Warn("control: fetch ai models", "error", err, "type", input.ProviderType)
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data":    []any{},
			"error":   err.Error(),
			"warning": "Could not fetch models — you can still type the model name manually.",
		})
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": models})
}

// GetAIProviderTypes handles GET /control/ai-providers/types.
// Returns the valid provider_type values and dispatcher categories so the
// control panel frontend can populate its selectors from the API (Rule 16)
// instead of hardcoding enum values that must match the DB CHECK constraint.
func (h *Handlers) GetAIProviderTypes(w http.ResponseWriter, _ *http.Request) {
	types := make([]string, 0, len(validProviderTypes))
	for t := range validProviderTypes {
		types = append(types, t)
	}
	cats := make([]string, 0, len(validAICategories))
	for c := range validAICategories {
		cats = append(cats, c)
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"provider_types": types,
			"categories":     cats,
		},
	})
}

// create/update/delete operations so changes take effect immediately without
// a server restart.
func (h *Handlers) reloadAIProviders(ctx context.Context, tenantID pgtype.UUID) {
	if h.ProviderStore == nil || h.AIDispatcher == nil {
		return
	}

	if err := ai.LoadAndRegisterProviders(ctx, h.ProviderStore, h.AIDispatcher, tenantID); err != nil {
		slog.Error("control: hot-reload ai providers", "error", err)
	}
}
