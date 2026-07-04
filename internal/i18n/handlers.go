// Package i18n — HTTP handlers for the translation system.
//
// Two route groups:
//   - Admin: CRUD operations behind auth + RBAC permission "i18n.manage"
//   - Public: locale-scoped translation map for frontend apps (no auth)
//
// Responses follow the JapaConnect API envelope: {"data": ...} for success,
// {"error":{"code":"...","message":"..."}} for errors.
package i18n

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for translation HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, errors.New("invalid " + param)
	}

	return id, nil
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "not found"):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case strings.Contains(err.Error(), "required"):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			slog.Warn("json encode error", "error", err)
		}
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// ---------------------------------------------------------------------------
// Admin handlers (require auth + RBAC "i18n.manage")
// ---------------------------------------------------------------------------

// ListByLocale returns all translations for a given locale.
// GET /admin/translations?locale=en.
func (h *Handlers) ListByLocale(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	locale := r.URL.Query().Get("locale")
	if locale == "" {
		writeError(w, http.StatusBadRequest, "missing_locale", "locale query parameter is required")
		return
	}

	rows, err := h.Service.store.ListByLocale(r.Context(), tenantID, locale)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	type translationItem struct {
		ID        string  `json:"id"`
		Locale    string  `json:"locale"`
		Key       string  `json:"key"`
		Value     string  `json:"value"`
		Context   *string `json:"context"`
		CreatedAt string  `json:"created_at"`
		UpdatedAt string  `json:"updated_at"`
	}

	items := make([]translationItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, translationItem{
			ID:        r.ID.String(),
			Locale:    r.Locale,
			Key:       r.Key,
			Value:     r.Value,
			Context:   r.Context,
			CreatedAt: r.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: r.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ListLocales returns distinct locales with counts.
// GET /admin/translations/locales.
func (h *Handlers) ListLocales(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	counts, err := h.Service.Counts(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": counts})
}

// Upsert handles create/update of a single translation.
// POST /admin/translations.
func (h *Handlers) Upsert(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Locale  string  `json:"locale"`
		Key     string  `json:"key"`
		Value   string  `json:"value"`
		Context *string `json:"context"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Locale == "" {
		writeError(w, http.StatusBadRequest, "missing_locale", "locale is required")
		return
	}

	if input.Key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "key is required")
		return
	}

	if input.Value == "" {
		writeError(w, http.StatusBadRequest, "missing_value", "value is required")
		return
	}

	if err := h.Service.Upsert(r.Context(), tenantID, input.Locale, input.Key, input.Value, input.Context); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{
		"locale": input.Locale,
		"key":    input.Key,
		"value":  input.Value,
	}})
}

// UpsertBatch handles batch creation/update of translations.
// POST /admin/translations/batch.
func (h *Handlers) UpsertBatch(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Translations []struct {
			Locale  string  `json:"locale"`
			Key     string  `json:"key"`
			Value   string  `json:"value"`
			Context *string `json:"context"`
		} `json:"translations"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Translations) == 0 {
		writeError(w, http.StatusBadRequest, "missing_translations", "translations array is required")
		return
	}

	items := make([]UpsertBatchItem, 0, len(input.Translations))
	for _, t := range input.Translations {
		items = append(items, UpsertBatchItem{
			Locale:  t.Locale,
			Key:     t.Key,
			Value:   t.Value,
			Context: t.Context,
		})
	}

	if err := h.Service.UpsertBatch(r.Context(), tenantID, items); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int{"upserted": len(items)},
	})
}

// Delete removes a translation.
// DELETE /admin/translations/{id}?locale=en.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid translation ID")
		return
	}

	// We need the locale to invalidate the correct cache entry.
	// The client must pass it as a query parameter since we don't store
	// the locale alongside the ID in the request path.
	locale := r.URL.Query().Get("locale")
	if locale == "" {
		writeError(w, http.StatusBadRequest, "missing_locale", "locale query parameter is required")
		return
	}

	if err := h.Service.Delete(r.Context(), tenantID, id, locale); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Public handlers (no auth)
// ---------------------------------------------------------------------------

// GetLocaleTranslations returns all translations for a locale as a flat map.
// GET /api/v1/translations/{locale}.
func (h *Handlers) GetLocaleTranslations(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	locale := chi.URLParam(r, "locale")
	if locale == "" {
		writeError(w, http.StatusBadRequest, "missing_locale", "locale is required")
		return
	}

	data, err := h.Service.GetTranslationsByLocale(r.Context(), tenantID, locale)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"locale": locale,
		"data":   data,
	})
}
