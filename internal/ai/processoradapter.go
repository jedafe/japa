// Package ai — processoradapter.go implements worker.AIProcessor by
// dispatching tasks through the AI Gateway.
package ai

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jedafe/japa/internal/config"
)

// WorkerAdapter wraps a Dispatcher to satisfy the worker.AIProcessor interface.
// Registered at startup so the ai:process_task background handler can invoke
// real AI providers instead of the noop fallback.
type WorkerAdapter struct {
	dispatcher *Dispatcher
	settings   *config.Settings // optional: enables ai.humanize.* for background generation
}

// NewWorkerAdapter creates a WorkerAdapter backed by the given dispatcher.
func NewWorkerAdapter(d *Dispatcher) *WorkerAdapter {
	return &WorkerAdapter{dispatcher: d}
}

// SetSettings injects the hot-reloadable settings reader so background AI
// tasks (studio drafts, persona generation) honor the global ai.humanize.*
// toggle. Optional — when nil, humanization is off for background tasks.
func (a *WorkerAdapter) SetSettings(s *config.Settings) {
	a.settings = s
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

	payload := map[string]any{
		"prompt":     prompt,
		"model_id":   modelID,
		"request_id": requestID,
	}

	// Apply the global humanization setting to background generation. There is
	// no per-request override here (the task payload is a flat prompt string),
	// so the global ai.humanize.* setting alone decides. The zero platform UUID
	// matches the global-setting read pattern used by participation.isEnabled.
	if a.settings != nil {
		enabled := a.settings.GetBoolOr(ctx, "ai.humanize.enabled", false)

		level := HumanizeMedium
		if lvl := a.settings.GetStringOr(ctx, "ai.humanize.level", "medium"); lvl != "" {
			level = HumanizeLevel(lvl)
		}

		writeHumanizePayload(payload, enabled, level)
	}

	resp, err := a.dispatcher.Dispatch(ctx, category, Request{
		Category: category,
		Payload:  payload,
	})
	if err != nil {
		return fmt.Errorf("ai: dispatch failed for request %s: %w", requestID, err)
	}

	slog.Info("ai:worker_adapter: task processed",
		"request_id", requestID,
		"provider_ref", resp.ProviderRef,
		"status", resp.Status)

	return nil
}
