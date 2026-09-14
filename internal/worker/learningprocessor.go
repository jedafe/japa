// Package worker — learningprocessor.go defines the LearningProcessor
// interface that the learning background tasks delegate to. The learning
// package (or a noop implementation) implements this interface and registers
// itself at startup via RegisterLearningProcessor.
package worker

import "context"

// LearningProcessor defines the contract for learning platform background
// tasks — computing track completion percentages and issuing certificates.
type LearningProcessor interface {
	UpdateTrackCompletion(ctx context.Context, userID, trackID string) error
	IssueCertificate(ctx context.Context, userID, trackID, courseName string) error
}

var learningProcessor LearningProcessor

// RegisterLearningProcessor registers the learning processor for the
// learning background tasks. Call once at startup before the worker
// begins processing tasks.
func RegisterLearningProcessor(p LearningProcessor) {
	learningProcessor = p
}
