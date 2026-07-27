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

func (noopImageProcessor) ProcessImage(_ context.Context, mediaID, _, operation string, _, _ int) error {
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

func (noopModerationProcessor) ScanContent(_ context.Context, entityType, entityID, _ string) error {
	slog.Debug("worker: noop moderation processor", "entity_type", entityType, "entity_id", entityID)
	return nil
}

// noopLinkPreviewProcessor is a no-op LinkPreviewProcessor that logs and returns nil.
type noopLinkPreviewProcessor struct{}

func (noopLinkPreviewProcessor) FetchAndStore(_ context.Context, url, _, _ string) error {
	slog.Debug("worker: noop link preview processor", "url", url)
	return nil
}

// noopAITranslateProcessor is a no-op AITranslateProcessor that logs and returns nil.
type noopAITranslateProcessor struct{}

func (noopAITranslateProcessor) TranslatePack(_ context.Context, packID, sourceLang, targetLang, _ string) error {
	slog.Debug("worker: noop ai translate processor", "pack_id", packID, "source", sourceLang, "target", targetLang)
	return nil
}

// noopLearningProcessor is a no-op LearningProcessor that logs and returns nil.
type noopLearningProcessor struct{}

func (noopLearningProcessor) UpdateTrackCompletion(_ context.Context, userID, trackID, _ string) error {
	slog.Debug("worker: noop learning processor: update track completion", "user_id", userID, "track_id", trackID)
	return nil
}

func (noopLearningProcessor) IssueCertificate(_ context.Context, userID, trackID, _, courseName string) error {
	slog.Debug("worker: noop learning processor: issue certificate", "user_id", userID, "track_id", trackID, "course_name", courseName)
	return nil
}

// noopSpaceAutoPopulateProcessor is a no-op SpaceAutoPopulateProcessor that logs and returns nil.
type noopSpaceAutoPopulateProcessor struct{}

func (noopSpaceAutoPopulateProcessor) AutoPopulateSpace(_ context.Context, spaceID, _ string) error {
	slog.Debug("worker: noop space auto-populate processor", "space_id", spaceID)
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

	if learningProcessor == nil {
		RegisterLearningProcessor(noopLearningProcessor{})
	}

	if spaceAutoPopulateProcessor == nil {
		RegisterSpaceAutoPopulateProcessor(noopSpaceAutoPopulateProcessor{})
	}

	if docsPageGenerator == nil {
		RegisterDocsPageGenerator(noopDocsPageGenerator{})
	}
}

// noopDocsPageGenerator is a no-op implementation of DocsPageGenerator (TASK-004).
type noopDocsPageGenerator struct{}

func (noopDocsPageGenerator) GenerateDocsPage(ctx context.Context, prompt, userID, tenantID string) error {
	slog.Warn("noop: docs:generate_page — no docs page generator registered")
	return nil
}
