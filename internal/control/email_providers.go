package control

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
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

	// Validate the full config before persisting — a row that can't be
	// constructed (missing host/from/secret, bad base_url) would silently
	// degrade sends at runtime.
	if err := email.ValidateProviderConfig(email.ProviderConfig{
		Name:         input.Name,
		ProviderType: input.ProviderType,
		BaseURL:      input.BaseURL,
		Config:       input.Config,
		Categories:   input.Categories,
	}, input.Secret); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
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
	h.reloadEmailProviders(context.WithoutCancel(r.Context()))

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

	// Merge the patch onto the stored row and validate the resulting config
	// before writing — catches type changes that leave stale/mismatched
	// config keys (e.g. smtp→ses without region) which would otherwise only
	// fail when a send is attempted. The row is locked FOR UPDATE inside a
	// transaction so a concurrent PATCH can't clobber this merge.
	var validationMsg string

	provider, err := h.EmailProviderStore.UpdateProviderWithLock(r.Context(), id,
		func(existing email.ProviderConfig, storedSecret string) (email.UpdateProviderInput, error) {
			merged := mergedProviderConfig(existing, input.Name, input.ProviderType, input.BaseURL, input.Config, categoriesPtr)

			mergedSecret := storedSecret
			if input.Secret != nil {
				mergedSecret = *input.Secret
			}

			if err := email.ValidateProviderConfig(merged, mergedSecret); err != nil {
				validationMsg = err.Error()

				return email.UpdateProviderInput{}, errProviderValidation
			}

			return email.UpdateProviderInput{
				Name:         input.Name,
				ProviderType: input.ProviderType,
				Secret:       input.Secret,
				BaseURL:      input.BaseURL,
				Config:       input.Config,
				Categories:   categoriesPtr,
				Priority:     input.Priority,
				Enabled:      input.Enabled,
			}, nil
		})
	if err != nil {
		if errors.Is(err, errProviderValidation) {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", validationMsg)
			return
		}

		if strings.Contains(err.Error(), "not_found") {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Provider not found")
			return
		}

		slog.Error("control: update email provider", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update email provider")

		return
	}

	// Hot-reload the dispatcher so the updated config takes effect immediately.
	h.reloadEmailProviders(context.WithoutCancel(r.Context()))

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
	h.reloadEmailProviders(context.WithoutCancel(r.Context()))

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

	if _, err := mail.ParseAddress(input.Email); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "email must be a valid address")
		return
	}

	// ForUpdate reads: a test issued right after create/update must see
	// the row it just wrote even when the Reader pool is a lagged replica.
	provider, err := h.EmailProviderStore.GetProviderForUpdate(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Provider not found")
		return
	}

	secret, err := h.EmailProviderStore.GetDecryptedSecretForUpdate(r.Context(), id)
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
// per-type config field schema (accepted keys + required keys) so the
// control panel frontend can populate its selectors and forms from the API
// (Rule 16) instead of hardcoding.
func (h *Handlers) GetEmailProviderTypes(w http.ResponseWriter, _ *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"provider_types": email.SupportedProviderTypes(),
			"categories":     email.SupportedEmailCategories(),
			"config_fields":  emailProviderConfigFields(),
		},
	})
}

// emailProviderConfigFields documents the non-secret config keys each
// provider type accepts, and which are required — the frontend renders its
// form from this schema.
func emailProviderConfigFields() map[string]map[string][]string {
	out := make(map[string]map[string][]string, len(validEmailProviderTypes))

	optional := map[string][]string{
		"smtp":      {"host", "port", "username", "from_address"},
		"resend":    {"from_address"},
		"postmark":  {"from_address", "message_stream"},
		"ses":       {"region", "access_key_id", "from_address"},
		"sendgrid":  {"from_address"},
		"sendpulse": {"client_id", "from_address"},
	}

	for _, t := range email.SupportedProviderTypes() {
		out[t] = map[string][]string{
			"accepted": optional[t],
			"required": email.RequiredConfigFields(t),
		}
	}

	return out
}

// mergedProviderConfig applies an update's non-nil fields onto the stored
// row so the resulting config can be validated before the write.
func mergedProviderConfig(base email.ProviderConfig, name, providerType, baseURL *string, config *map[string]string, categories *[]string) email.ProviderConfig {
	if name != nil {
		base.Name = *name
	}

	if providerType != nil {
		base.ProviderType = *providerType
	}

	if baseURL != nil {
		base.BaseURL = *baseURL
	}

	if config != nil {
		base.Config = *config
	}

	if categories != nil {
		base.Categories = *categories
	}

	return base
}

// errProviderValidation marks callback errors from UpdateProviderWithLock
// that should surface as a 400 rather than a 500.
var errProviderValidation = errors.New("provider_validation")

// reloadEmailProviders re-reads email provider configs and re-registers
// them with the dispatcher so create/update/delete operations take effect
// immediately without a server restart.
func (h *Handlers) reloadEmailProviders(ctx context.Context) {
	if h.EmailProviderStore == nil || h.EmailDispatcher == nil {
		return
	}

	if err := email.LoadAndRegisterProviders(ctx, h.EmailProviderStore, h.EmailDispatcher, h.EmailFallbackProvider); err != nil {
		slog.Error("control: hot-reload email providers", "error", err)
	}
}
