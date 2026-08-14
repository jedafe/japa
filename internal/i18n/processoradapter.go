package i18n

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
)

// ProcessorAdapter implements worker.AITranslateProcessor by delegating to
// the i18n service's translation methods.
type ProcessorAdapter struct {
	svc       *Service
	dispatcher AIDispatcher
}

// NewProcessorAdapter creates a ProcessorAdapter backed by the given i18n service.
func NewProcessorAdapter(svc *Service) *ProcessorAdapter {
	return &ProcessorAdapter{svc: svc}
}

// SetAIDispatcher wires the AI gateway dispatcher so TranslatePack can
// actually translate strings instead of copying source values verbatim.
func (a *ProcessorAdapter) SetAIDispatcher(d AIDispatcher) {
	a.dispatcher = d
}

// TranslatePack translates all untranslated strings in a language pack
// from sourceLang to targetLang. It fetches all keys from the source locale,
// checks which ones are missing in the target, dispatches each to the AI
// gateway for translation, and upserts the results.
func (a *ProcessorAdapter) TranslatePack(ctx context.Context, packID, sourceLang, targetLang, tenantID string) error {
	slog.Info("i18n:translate_language_pack",
		"pack_id", packID,
		"source", sourceLang,
		"target", targetLang,
		"tenant_id", tenantID,
	)

	if a.svc == nil {
		slog.Warn("i18n:translate_language_pack — service not initialized, skipping")
		return nil
	}

	var tid pgtype.UUID
	if err := tid.Scan(tenantID); err != nil {
		slog.Warn("i18n:translate_language_pack — invalid tenant_id", "error", err)
		return nil
	}

	// Fetch all translations from the source locale.
	sourceTranslations, err := a.svc.GetTranslationsByLocale(ctx, tid, sourceLang)
	if err != nil {
		slog.Error("i18n:translate_language_pack — failed to fetch source translations", "error", err)
		return err
	}

	// Fetch existing translations in the target locale.
	targetTranslations, err := a.svc.GetTranslationsByLocale(ctx, tid, targetLang)
	if err != nil {
		slog.Warn("i18n:translate_language_pack — no existing target translations, will create all", "error", err)
		targetTranslations = map[string]string{}
	}

	// Find keys that exist in source but not in target.
	type pendingItem struct {
		key   string
		value string
	}
	var pending []pendingItem
	for key, value := range sourceTranslations {
		if _, exists := targetTranslations[key]; !exists && value != "" {
			pending = append(pending, pendingItem{key: key, value: value})
		}
	}

	if len(pending) == 0 {
		slog.Info("i18n:translate_language_pack — no missing translations")
		return nil
	}

	// If no AI dispatcher is wired, fall back to copying source values so
	// the locale is at least populated (admin can edit translations later).
	if a.dispatcher == nil {
		slog.Warn("i18n:translate_language_pack — no AI dispatcher, copying source values as fallback",
			"count", len(pending), "target", targetLang)
		items := make([]UpsertBatchItem, 0, len(pending))
		for _, p := range pending {
			items = append(items, UpsertBatchItem{
				Locale: targetLang,
				Key:    p.key,
				Value:  p.value,
			})
		}
		if err := a.svc.UpsertBatch(ctx, tid, items); err != nil {
			slog.Error("i18n:translate_language_pack — fallback batch upsert failed", "error", err, "count", len(items))
			return err
		}
		slog.Info("i18n:translate_language_pack — fallback copy completed", "copied", len(items), "target", targetLang)
		return nil
	}

	// Translate each missing key via the AI gateway.
	items := make([]UpsertBatchItem, 0, len(pending))
	for _, p := range pending {
		resp, err := a.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
			Category: "content_generation",
			Payload: map[string]any{
				"prompt":         "Translate the following text to locale " + targetLang + ". Return only the translation, no explanation.\n\n" + p.value,
				"task":           "translate",
				"target_locale":  targetLang,
				"source_locale":  sourceLang,
				"translation_key": p.key,
			},
		})
		if err != nil {
			slog.Error("i18n:translate_language_pack — AI dispatch failed, using source as fallback",
				"key", p.key, "error", err)
			items = append(items, UpsertBatchItem{
				Locale: targetLang,
				Key:    p.key,
				Value:  p.value,
			})
			continue
		}

		translated := extractAITranslationText(resp)
		if translated == "" {
			slog.Warn("i18n:translate_language_pack — AI returned empty, using source as fallback", "key", p.key)
			translated = p.value
		}

		items = append(items, UpsertBatchItem{
			Locale: targetLang,
			Key:    p.key,
			Value:  translated,
		})
	}

	// Batch upsert the translated strings.
	if err := a.svc.UpsertBatch(ctx, tid, items); err != nil {
		slog.Error("i18n:translate_language_pack — batch upsert failed", "error", err, "count", len(items))
		return err
	}

	slog.Info("i18n:translate_language_pack — completed", "translated", len(items), "target", targetLang)
	return nil
}

// Ensure ProcessorAdapter satisfies the interface at compile time.
var _ interface {
	TranslatePack(ctx context.Context, packID, sourceLang, targetLang, tenantID string) error
} = (*ProcessorAdapter)(nil)
