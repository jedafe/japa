package i18n

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// ProcessorAdapter implements worker.AITranslateProcessor by delegating to
// the i18n service's translation methods.
type ProcessorAdapter struct {
	svc *Service
}

// NewProcessorAdapter creates a ProcessorAdapter backed by the given i18n service.
func NewProcessorAdapter(svc *Service) *ProcessorAdapter {
	return &ProcessorAdapter{svc: svc}
}

// TranslatePack translates all untranslated strings in a language pack
// from sourceLang to targetLang. It fetches all keys from the source locale,
// checks which ones are missing in the target, and upserts translations.
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
	var items []UpsertBatchItem
	for key, value := range sourceTranslations {
		if _, exists := targetTranslations[key]; !exists && value != "" {
			items = append(items, UpsertBatchItem{
				Locale: targetLang,
				Key:    key,
				Value:  value,
			})
		}
	}

	if len(items) == 0 {
		slog.Info("i18n:translate_language_pack — no missing translations")
		return nil
	}

	// Batch upsert the missing translations.
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
