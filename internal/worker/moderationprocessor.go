// Package worker — moderationprocessor.go defines the ModerationProcessor
// interface that the moderation_scan task delegates to. The spam package
// (or a noop implementation) implements this interface and registers itself
// at startup via RegisterModerationProcessor.
package worker

import "context"

// ModerationProcessor defines the contract for scanning content through
// the spam/moderation pipeline — checking for violations, spam, and
// policy breaches.
type ModerationProcessor interface {
	ScanContent(ctx context.Context, entityType, entityID, tenantID string) error
}

var moderationProcessor ModerationProcessor

// RegisterModerationProcessor registers the moderation processor for the
// moderation_scan background task. Call once at startup before the worker
// begins processing tasks.
func RegisterModerationProcessor(p ModerationProcessor) {
	moderationProcessor = p
}
