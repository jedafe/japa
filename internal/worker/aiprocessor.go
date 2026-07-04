// Package worker — aiprocessor.go defines the AIProcessor interface that
// the ai_task_processing task delegates to. The ai package (or a noop
// implementation) implements this interface and registers itself at startup
// via RegisterAIProcessor.
package worker

import "context"

// AIProcessor defines the contract for dispatching AI tasks — calling the
// AI gateway with a prompt, storing results, and updating task status.
type AIProcessor interface {
	ProcessAITask(ctx context.Context, taskType, prompt, modelID, requestID string) error
}

var aiProcessor AIProcessor

// RegisterAIProcessor registers the AI task processor for the
// ai_task_processing background task. Call once at startup before the worker
// begins processing tasks.
func RegisterAIProcessor(p AIProcessor) {
	aiProcessor = p
}
