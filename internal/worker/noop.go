// Package worker — noop.go provides no-op implementations of every
// processor interface. These are used at startup when a real implementation
// is not available, ensuring the server boots cleanly while clearly logging
// that the feature is disabled. Each noop method logs the call and returns
// nil (graceful degradation).
package worker

import (
	"context"
	"log/slog"
)

// noopImageProcessor is a no-op ImageProcessor that logs and returns nil.
type noopImageProcessor struct{}

func (noopImageProcessor) ProcessImage(_ context.Context, mediaID, operation string, _, _ int) error {
	slog.Debug("worker: noop image processor", "media_id", mediaID, "operation", operation)
	return nil
}

// noopAIProcessor is a no-op AIProcessor that logs and returns nil.
type noopAIProcessor struct{}

func (noopAIProcessor) ProcessAITask(_ context.Context, taskType, _, _, requestID string) error {
	slog.Debug("worker: noop ai processor", "task_type", taskType, "request_id", requestID)
	return nil
}

// noopModerationProcessor is a no-op ModerationProcessor that logs and returns nil.
type noopModerationProcessor struct{}

func (noopModerationProcessor) ScanContent(_ context.Context, entityType, entityID string) error {
	slog.Debug("worker: noop moderation processor", "entity_type", entityType, "entity_id", entityID)
	return nil
}

// noopLinkPreviewProcessor is a no-op LinkPreviewProcessor that logs and returns nil.
type noopLinkPreviewProcessor struct{}

func (noopLinkPreviewProcessor) FetchAndStore(_ context.Context, url, _ string) error {
	slog.Debug("worker: noop link preview processor", "url", url)
	return nil
}

// noopAITranslateProcessor is a no-op AITranslateProcessor that logs and returns nil.
type noopAITranslateProcessor struct{}

func (noopAITranslateProcessor) TranslatePack(_ context.Context, packID, sourceLang, targetLang string) error {
	slog.Debug("worker: noop ai translate processor", "pack_id", packID, "source", sourceLang, "target", targetLang)
	return nil
}

// noopAcademyProcessor is a no-op AcademyProcessor that logs and returns nil.
type noopAcademyProcessor struct{}

func (noopAcademyProcessor) UpdateTrackCompletion(_ context.Context, userID, trackID string) error {
	slog.Debug("worker: noop learning processor: update track completion", "user_id", userID, "track_id", trackID)
	return nil
}

func (noopAcademyProcessor) IssueCertificate(_ context.Context, userID, trackID, courseName string) error {
	slog.Debug("worker: noop learning processor: issue certificate", "user_id", userID, "track_id", trackID, "course_name", courseName)
	return nil
}

// noopSpaceAutoPopulateProcessor is a no-op SpaceAutoPopulateProcessor that logs and returns nil.
type noopSpaceAutoPopulateProcessor struct{}

func (noopSpaceAutoPopulateProcessor) AutoPopulateSpace(_ context.Context, spaceID string) error {
	slog.Debug("worker: noop space auto-populate processor", "space_id", spaceID)
	return nil
}

// noopWikiSeedAttractionsProcessor is a no-op WikiSeedAttractionsProcessor.
type noopWikiSeedAttractionsProcessor struct{}

func (noopWikiSeedAttractionsProcessor) SeedAttractions(_ context.Context, _ string, _ int, _ bool) error {
	slog.Debug("worker: noop wiki seed attractions processor")
	return nil
}

// RegisterNoopProcessors registers no-op implementations for every
// processor interface that hasn't already been registered. Call this
// at startup after all real processors have been registered, so
// handlers gracefully degrade instead of crashing.
func RegisterNoopProcessors() {
	if imageProcessor == nil {
		RegisterImageProcessor(noopImageProcessor{})
	}

	if aiProcessor == nil {
		RegisterAIProcessor(noopAIProcessor{})
	}

	if moderationProcessor == nil {
		RegisterModerationProcessor(noopModerationProcessor{})
	}

	if linkPreviewProcessor == nil {
		RegisterLinkPreviewProcessor(noopLinkPreviewProcessor{})
	}

	if aiTranslateProcessor == nil {
		RegisterAITranslateProcessor(noopAITranslateProcessor{})
	}

	if academyProcessor == nil {
		RegisterAcademyProcessor(noopAcademyProcessor{})
	}

	if spaceAutoPopulateProcessor == nil {
		RegisterSpaceAutoPopulateProcessor(noopSpaceAutoPopulateProcessor{})
	}

	if wikiSeedAttractionsProcessor == nil {
		RegisterWikiSeedAttractionsProcessor(noopWikiSeedAttractionsProcessor{})
	}

	if docsPageGenerator == nil {
		RegisterDocsPageGenerator(noopDocsPageGenerator{})
	}
}

// noopDocsPageGenerator is a no-op implementation of DocsPageGenerator (TASK-004).
type noopDocsPageGenerator struct{}

func (noopDocsPageGenerator) GenerateDocsPage(_ context.Context, _ GenerateDocsPageRequest) error {
	slog.Warn("noop: docs:generate_page — no docs page generator registered")
	return nil
}
