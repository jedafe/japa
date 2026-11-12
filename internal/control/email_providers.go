package control

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/email"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Email Provider Management Handlers (Control Panel — super-admin)
// ADR-015: multi-provider email abstraction with category routing.
// ---------------------------------------------------------------------------

// validEmailProviderTypes is derived from the email package factory so the
// control panel and the gateway stay in sync.
var validEmailProviderTypes = buildValidSet(email.SupportedProviderTypes())

// validEmailCategories is the set of dispatcher categories providers can be
// registered for (transactional, marketing).
var validEmailCategories = buildValidSet(email.SupportedEmailCategories())

// ListEmailProviders handles GET /control/email-providers.
// Returns all email provider configs. Secrets are masked.
func (h *Handlers) ListEmailProviders(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.EmailProviderStore == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	providers, err := h.EmailProviderStore.ListProviders(r.Context())
	if err != nil {
		slog.Error("control: list email providers", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list email providers")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// GetEmailProvider handles GET /control/email-providers/{id}.
func (h *Handlers) GetEmailProvider(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.EmailProviderStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_provider_store", "Provider store not configured")
		return
	}

	provider, err := h.EmailProviderStore.GetProvider(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Provider not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// CreateEmailProvider handles POST /control/email-providers.
func (h *Handlers) CreateEmailProvider(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.EmailProviderStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_provider_store", "Provider store not configured")
		return
	}

	var input struct {
		Name         string            `json:"name"`
		ProviderType string            `json:"provider_type"`
		Secret       string            `json:"secret"`
		BaseURL      string            `json:"base_url"`
		Config       map[string]string `json:"config"`
		Categories   []string          `json:"categories"`
		Priority     int               `json:"priority"`
		Enabled      *bool             `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	if !validEmailProviderTypes[input.ProviderType] {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid provider_type")
		return
	}

	if len(input.Categories) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "at least one category is required")
		return
	}

	for _, cat := range input.Categories {
		if !validEmailCategories[cat] {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid category: "+cat)
			return
		}
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	provider, err := h.EmailProviderStore.CreateProvider(r.Context(), email.CreateProviderInput{
		Name:         input.Name,
		ProviderType: input.ProviderType,
		Secret:       input.Secret,
		BaseURL:      input.BaseURL,
		Config:       input.Config,
		Categories:   input.Categories,
		Priority:     input.Priority,
		Enabled:      enabled,
	})
	if err != nil {
		slog.Error("control: create email provider", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create email provider")

		return
	}

	// Hot-reload the dispatcher so the new provider is immediately available.
	h.reloadEmailProviders(r.Context())

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": provider})
}

// UpdateEmailProvider handles PATCH /control/email-providers/{id}.
func (h *Handlers) UpdateEmailProvider(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.EmailProviderStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_provider_store", "Provider store not configured")
		return
	}

	var input struct {
		Name         *string            `json:"name"`
		ProviderType *string            `json:"provider_type"`
		Secret       *string            `json:"secret"`
		BaseURL      *string            `json:"base_url"`
		Config       *map[string]string `json:"config"`
		Categories   []string           `json:"categories"`
		Priority     *int               `json:"priority"`
		Enabled      *bool              `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ProviderType != nil && !validEmailProviderTypes[*input.ProviderType] {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid provider_type")
		return
	}

	if input.Categories != nil {
		if len(input.Categories) == 0 {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "at least one category is required")
			return
		}

		for _, cat := range input.Categories {
			if !validEmailCategories[cat] {
				errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid category: "+cat)
				return
			}
		}
	}

	var categoriesPtr *[]string
	if input.Categories != nil {
		categoriesPtr = &input.Categories
	}

	provider, err := h.EmailProviderStore.UpdateProvider(r.Context(), email.UpdateProviderInput{
		ID:           id,
		Name:         input.Name,
		ProviderType: input.ProviderType,
		Secret:       input.Secret,
		BaseURL:      input.BaseURL,
		Config:       input.Config,
		Categories:   categoriesPtr,
		Priority:     input.Priority,
		Enabled:      input.Enabled,
	})
	if err != nil {
		slog.Error("control: update email provider", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update email provider")

		return
	}

	// Hot-reload the dispatcher so the updated config takes effect immediately.
	h.reloadEmailProviders(r.Context())

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": provider})
}

// DeleteEmailProvider handles DELETE /control/email-providers/{id}.
func (h *Handlers) DeleteEmailProvider(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.EmailProviderStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_provider_store", "Provider store not configured")
		return
	}

	if err := h.EmailProviderStore.DeleteProvider(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not_found") {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Provider not found")
			return
		}

		slog.Error("control: delete email provider", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete email provider")

		return
	}

	// Hot-reload the dispatcher so the removed provider is unregistered.
	h.reloadEmailProviders(r.Context())

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// TestEmailProvider handles POST /control/email-providers/{id}/test.
// Sends a test email through the provider to verify connectivity. Unlike
// the AI test endpoint, this dispatches a real email to the admin-supplied
// address — the only meaningful connectivity check for a mail provider.
func (h *Handlers) TestEmailProvider(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.EmailProviderStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_provider_store", "Provider store not configured")
		return
	}

	var input struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if strings.TrimSpace(input.Email) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}

	provider, err := h.EmailProviderStore.GetProvider(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Provider not found")
		return
	}

	secret, err := h.EmailProviderStore.GetDecryptedSecret(r.Context(), id)
	if err != nil {
		slog.Error("control: test email provider: decrypt secret", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to decrypt provider secret")

		return
	}

	p, err := email.NewProviderFromConfig(provider, secret)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}

	resp, err := p.Send(r.Context(), email.EmailRequest{
		To:       input.Email,
		Subject:  "JapaConnect email provider test",
		Body:     "This is a connectivity test from the JapaConnect control panel.",
		IsHTML:   false,
		Category: "transactional",
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
			"message":      "Test email sent via " + p.Name(),
		},
	})
}

// GetEmailProviderTypes handles GET /control/email-providers/types.
// Returns the valid provider_type values, dispatcher categories, and the
// per-type config fields so the control panel frontend can populate its
// selectors and forms from the API (Rule 16) instead of hardcoding.
func (h *Handlers) GetEmailProviderTypes(w http.ResponseWriter, _ *http.Request) {
	types := make([]string, 0, len(validEmailProviderTypes))
	for t := range validEmailProviderTypes {
		types = append(types, t)
	}

	cats := make([]string, 0, len(validEmailCategories))
	for c := range validEmailCategories {
		cats = append(cats, c)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"provider_types": types,
			"categories":     cats,
			"config_fields":  emailProviderConfigFields(),
		},
	})
}

// emailProviderConfigFields documents the non-secret config keys each
// provider type accepts — the frontend renders its form from this list.
func emailProviderConfigFields() map[string][]string {
	return map[string][]string{
		"smtp":      {"host", "port", "username", "from_address"},
		"resend":    {"from_address"},
		"postmark":  {"from_address", "message_stream"},
		"ses":       {"region", "access_key_id", "from_address"},
		"sendgrid":  {"from_address"},
		"sendpulse": {"client_id", "from_address"},
	}
}

// reloadEmailProviders re-reads email provider configs and re-registers
// them with the dispatcher so create/update/delete operations take effect
// immediately without a server restart.
func (h *Handlers) reloadEmailProviders(ctx context.Context) {
	if h.EmailProviderStore == nil || h.EmailDispatcher == nil {
		return
	}

	if err := email.LoadAndRegisterProviders(ctx, h.EmailProviderStore, h.EmailDispatcher); err != nil {
		slog.Error("control: hot-reload email providers", "error", err)
	}
}
