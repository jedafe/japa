// Package ai — processoradapter.go implements worker.AIProcessor by
// dispatching tasks through the AI Gateway.
package ai

import (
	"context"
	"fmt"
	"log/slog"
)

// WorkerAdapter wraps a Dispatcher to satisfy the worker.AIProcessor interface.
// Registered at startup so the ai:process_task background handler can invoke
// real AI providers instead of the noop fallback.
type WorkerAdapter struct {
	dispatcher *Dispatcher
}

// NewWorkerAdapter creates a WorkerAdapter backed by the given dispatcher.
func NewWorkerAdapter(d *Dispatcher) *WorkerAdapter {
	return &WorkerAdapter{dispatcher: d}
}

// ProcessAITask dispatches an AI task through the configured providers.
func (a *WorkerAdapter) ProcessAITask(ctx context.Context, taskType, prompt, modelID, requestID string) error {
	if a.dispatcher == nil {
		slog.Warn("ai:worker_adapter: no dispatcher configured, skipping", "request_id", requestID)
		return nil
	}

	category := "content_generation"
	if taskType != "" {
		category = taskType
	}

	req := Request{
		Category: category,
		Payload: map[string]any{
			"prompt":     prompt,
			"model_id":   modelID,
			"request_id": requestID,
		},
	}

	resp, err := a.dispatcher.Dispatch(ctx, category, req)
	if err != nil {
		return fmt.Errorf("ai: dispatch failed for request %s: %w", requestID, err)
	}

	slog.Info("ai:worker_adapter: task processed",
		"request_id", requestID,
		"provider_ref", resp.ProviderRef,
		"status", resp.Status)

	return nil
}
