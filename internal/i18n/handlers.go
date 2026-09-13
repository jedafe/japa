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
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// TaskEnqueueClient is the interface for enqueueing background tasks.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// AIDispatcher is the contract for synchronous AI translation of a single
// string. The wiring layer passes the real ai.Dispatcher; tests can pass a
// stub.
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// Handlers holds dependencies for translation HTTP handlers.
type Handlers struct {
	Service      *Service
	AsynqClient  TaskEnqueueClient
	AIDispatcher AIDispatcher
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// enqueueTranslatePack enqueues an async AI translation task for a language pack.
// Fire-and-forget: errors are logged but never propagated.
func (h *Handlers) enqueueTranslatePack(packID, sourceLang, targetLang string, tenantID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"pack_id":     packID,
		"source_lang": sourceLang,
		"target_lang": targetLang,
		"tenant_id":   tenantID.String(),
	})
	if err != nil {
		slog.Warn("i18n: failed to marshal translate pack payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeLanguagePackAITranslate, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeLanguagePackAITranslate)),
	); err != nil {
		slog.Warn("i18n: failed to enqueue translate pack task", "pack_id", packID, "error", err)
	}
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
	return pgtype.UUID{}, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
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
		errpkg.WriteError(w, http.StatusBadRequest, "missing_locale", "locale query parameter is required")
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
			CreatedAt: r.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt: r.UpdatedAt.Time.Format(time.RFC3339),
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": counts})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Locale == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_locale", "locale is required")
		return
	}

	if input.Key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "key is required")
		return
	}

	if input.Value == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_value", "value is required")
		return
	}

	if err := h.Service.Upsert(r.Context(), tenantID, input.Locale, input.Key, input.Value, input.Context); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Translations) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_translations", "translations array is required")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid translation ID")
		return
	}

	// We need the locale to invalidate the correct cache entry.
	// The client must pass it as a query parameter since we don't store
	// the locale alongside the ID in the request path.
	locale := r.URL.Query().Get("locale")
	if locale == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_locale", "locale query parameter is required")
		return
	}

	if err := h.Service.Delete(r.Context(), tenantID, id, locale); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteByLocale removes all translations for a locale.
// DELETE /admin/i18n/languages/{locale}.
func (h *Handlers) DeleteByLocale(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	locale := chi.URLParam(r, "locale")
	if locale == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_locale", "locale path parameter is required")
		return
	}

	if err := h.Service.DeleteByLocale(r.Context(), tenantID, locale); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TranslateString translates a single translation string via the AI gateway
// into the requested target locale and upserts the result.
// POST /admin/i18n/strings/{id}/translate — body: {"target_locale": "fr"}.
func (h *Handlers) TranslateString(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.AIDispatcher == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "ai_unavailable", "AI translation is not configured")
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid translation ID")
		return
	}

	var input struct {
		TargetLocale string `json:"target_locale"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.TargetLocale == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_target_locale", "target_locale is required")
		return
	}

	// Fetch the source string by ID.
	src, err := h.Service.GetByID(r.Context(), tenantID, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Don't translate into the same locale.
	if src.Locale == input.TargetLocale {
		errpkg.WriteError(w, http.StatusBadRequest, "same_locale", "Source and target locales are identical")
		return
	}

	// Dispatch to the AI gateway (content_generation category).
	resp, err := h.AIDispatcher.Dispatch(r.Context(), "content_generation", ai.Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt":          "Translate the following text to locale " + input.TargetLocale + ". Return only the translation, no explanation.\n\n" + src.Value,
			"task":            "translate",
			"target_locale":   input.TargetLocale,
			"source_locale":   src.Locale,
			"translation_key": src.Key,
		},
	})
	if err != nil {
		slog.Error("i18n: AI translate failed", "key", src.Key, "error", err)
		errpkg.WriteError(w, http.StatusBadGateway, "ai_error", "AI translation failed")

		return
	}

	translated := extractAITranslationText(resp)
	if translated == "" {
		errpkg.WriteError(w, http.StatusBadGateway, "ai_empty", "AI returned an empty translation")
		return
	}

	// Upsert the translated value into the target locale.
	if err := h.Service.Upsert(r.Context(), tenantID, input.TargetLocale, src.Key, translated, src.Context); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{
		"locale": input.TargetLocale,
		"key":    src.Key,
		"value":  translated,
	}})
}

// AITranslateBatch enqueues a background task to AI-translate all missing
// strings from the source locale into the target locale. Fire-and-forget
// per Rule 8 — returns 202 immediately.
// POST /admin/i18n/strings/ai-translate-batch — body: {"source_locale":"en","target_locale":"fr"}.
func (h *Handlers) AITranslateBatch(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		SourceLocale string `json:"source_locale"`
		TargetLocale string `json:"target_locale"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.SourceLocale == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_source_locale", "source_locale is required")
		return
	}

	if input.TargetLocale == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_target_locale", "target_locale is required")
		return
	}

	if input.SourceLocale == input.TargetLocale {
		errpkg.WriteError(w, http.StatusBadRequest, "same_locale", "Source and target locales are identical")
		return
	}

	// Enqueue the fire-and-forget AI translate task (Rule 8).
	h.enqueueTranslatePack("", input.SourceLocale, input.TargetLocale, tenantID)

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]string{
			"status":        "queued",
			"source_locale": input.SourceLocale,
			"target_locale": input.TargetLocale,
		},
	})
}

// extractAITranslationText pulls the translated text out of an AI Response.
// Mirrors ai.extractReplyText: checks "reply" first, then choices[].message.content.
func extractAITranslationText(resp ai.Response) string {
	if resp.Raw == nil {
		return ""
	}

	if s, ok := resp.Raw["reply"].(string); ok {
		return s
	}

	if rawChoices, ok := resp.Raw["choices"]; ok {
		if choices, ok := rawChoices.([]any); ok && len(choices) > 0 {
			if first, ok := choices[0].(map[string]any); ok {
				if msg, ok := first["message"].(map[string]any); ok {
					if content, ok := msg["content"].(string); ok {
						return content
					}
				}
			}
		}
	}

	return ""
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

	// Catalog route param is :lang_code (was :locale).
	locale := chi.URLParam(r, "lang_code")
	if locale == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_locale", "locale is required")
		return
	}

	data, err := h.Service.GetTranslationsByLocale(r.Context(), tenantID, locale)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"locale": locale,
		"data":   data,
	})
}

// HandleListPublicLocales returns the list of enabled locales for the tenant.
// GET /api/v1/i18n/locales — public, no auth required.
func (h *Handlers) HandleListPublicLocales(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	locales, err := h.Service.ListLocales(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": locales})
}
