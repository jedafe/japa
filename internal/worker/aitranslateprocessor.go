// Package worker — aitranslateprocessor.go defines the AITranslateProcessor
// interface that the language_pack_ai_translate task delegates to. The i18n
// package (or a noop implementation) implements this interface and registers
// itself at startup via RegisterAITranslateProcessor.
package worker

import "context"

// AITranslateProcessor defines the contract for translating missing locale
// strings via the AI gateway. The worker calls TranslatePack from the
// language_pack_ai_translate task handler.
type AITranslateProcessor interface {
	TranslatePack(ctx context.Context, packID, sourceLang, targetLang string) error
}

var aiTranslateProcessor AITranslateProcessor

// RegisterAITranslateProcessor registers the AI translate processor for the
// language_pack_ai_translate background task. Call once at startup before the
// worker begins processing tasks.
func RegisterAITranslateProcessor(p AITranslateProcessor) {
	aiTranslateProcessor = p
}
