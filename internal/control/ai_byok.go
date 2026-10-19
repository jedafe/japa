// Package control — ai_byok.go implements the Control Panel CRUD for the
// BYOK provider allowlist (ai_byok_providers). These rows are the
// "storefront" users attach their own API keys against: provider type,
// display name, model catalog, and the affiliate signup link. No secrets
// live here — user keys are in user_ai_keys.
//
// Ref: docs/superpowers/specs/2026-10-19-ai-byok-design.md
package control

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// validByokProviderTypes mirrors ai.SupportedByokProviderTypes for request
// validation (the DB CHECK constraint is the backstop).
var validByokProviderTypes = func() map[string]bool {
	m := make(map[string]bool)
	for _, t := range ai.SupportedByokProviderTypes() {
		m[t] = true
	}
	return m
}()

func validateByokModels(models []ai.ModelInfo) bool {
	for _, m := range models {
		if strings.TrimSpace(m.ID) == "" {
			return false
		}
	}
	return true
}

func validSignupURL(u string) bool {
	return u == "" || strings.HasPrefix(u, "https://")
}

// ListByokProviders handles GET /control/ai-byok-providers.
func (h *Handlers) ListByokProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.ByokStore == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	providers, err := h.ByokStore.List(r.Context())
	if err != nil {
		slog.Error("control: list byok providers", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list BYOK providers")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// GetByokProvider handles GET /control/ai-byok-providers/{id}.
func (h *Handlers) GetByokProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.ByokStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_store", "BYOK store not configured")
		return
	}

	bp, err := h.ByokStore.Get(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "BYOK provider not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": bp})
}

// CreateByokProvider handles POST /control/ai-byok-providers.
func (h *Handlers) CreateByokProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.ByokStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_store", "BYOK store not configured")
		return
	}

	var input struct {
		ProviderType string         `json:"provider_type"`
		DisplayName  string         `json:"display_name"`
		Models       []ai.ModelInfo `json:"models"`
		SignupURL    string         `json:"signup_url"`
		Enabled      *bool          `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if strings.TrimSpace(input.DisplayName) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "display_name is required")
		return
	}

	if !validByokProviderTypes[input.ProviderType] {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid provider_type for BYOK")
		return
	}

	if !validateByokModels(input.Models) {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "models entries require a non-empty id")
		return
	}

	if !validSignupURL(input.SignupURL) {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "signup_url must be https:// or empty")
		return
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	bp, err := h.ByokStore.Create(r.Context(), ai.CreateByokProviderInput{
		ProviderType: input.ProviderType,
		DisplayName:  strings.TrimSpace(input.DisplayName),
		Models:       input.Models,
		SignupURL:    strings.TrimSpace(input.SignupURL),
		Enabled:      enabled,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			errpkg.WriteError(w, http.StatusConflict, "duplicate_provider_type", "A BYOK provider for this type already exists")
			return
		}

		slog.Error("control: create byok provider", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create BYOK provider")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": bp})
}

// UpdateByokProvider handles PATCH /control/ai-byok-providers/{id}.
func (h *Handlers) UpdateByokProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.ByokStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_store", "BYOK store not configured")
		return
	}

	var input struct {
		ProviderType *string         `json:"provider_type"`
		DisplayName  *string         `json:"display_name"`
		Models       *[]ai.ModelInfo `json:"models"`
		SignupURL    *string         `json:"signup_url"`
		Enabled      *bool           `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ProviderType != nil && !validByokProviderTypes[*input.ProviderType] {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid provider_type for BYOK")
		return
	}

	if input.Models != nil && !validateByokModels(*input.Models) {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "models entries require a non-empty id")
		return
	}

	if input.SignupURL != nil && !validSignupURL(*input.SignupURL) {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "signup_url must be https:// or empty")
		return
	}

	bp, err := h.ByokStore.Update(r.Context(), ai.UpdateByokProviderInput{
		ID:           id,
		ProviderType: input.ProviderType,
		DisplayName:  input.DisplayName,
		Models:       input.Models,
		SignupURL:    input.SignupURL,
		Enabled:      input.Enabled,
	})
	if err != nil {
		if errors.Is(err, ai.ErrByokProviderNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "BYOK provider not found")
			return
		}

		slog.Error("control: update byok provider", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update BYOK provider")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": bp})
}

// DeleteByokProvider handles DELETE /control/ai-byok-providers/{id}.
// user_ai_keys referencing the row cascade-delete.
func (h *Handlers) DeleteByokProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.ByokStore == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_store", "BYOK store not configured")
		return
	}

	if err := h.ByokStore.Delete(r.Context(), id); err != nil {
		if errors.Is(err, ai.ErrByokProviderNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "BYOK provider not found")
			return
		}

		slog.Error("control: delete byok provider", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete BYOK provider")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
