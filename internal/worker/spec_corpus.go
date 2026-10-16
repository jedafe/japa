// Package worker — spec_corpus.go wires the rag:reindex_specs Asynq task to
// the §121 spec corpus indexer. The worker cannot import internal/rag (rag
// imports worker for task-type constants), so the indexer is registered as
// a plain function at startup — the same registration pattern as
// RegisterRAGIndexer / RegisterAIProcessor.
//
// Ref: japaconnect-engineering/sections/121-engineering-spec-corpus.md
//
//	japaconnect-engineering/sections/11-job-queue-asynq.md (task catalog)
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// SpecCorpusReindexFunc is the worker-facing signature of
// rag.Service.ReindexSpecCorpus minus its stats return — the worker only
// needs the error. dir may be empty; the service applies its default.
// Stable failures (e.g. feature flag off) should be returned already
// wrapped in asynq.SkipRetry by the registering closure so no retries burn.
type SpecCorpusReindexFunc func(ctx context.Context, dir string) error

var specCorpusReindex SpecCorpusReindexFunc

// RegisterSpecCorpusIndexer registers the spec corpus reindex function for
// the rag:reindex_specs task. Call once at startup before the worker begins
// processing tasks. If never called, the handler skips (graceful
// degradation).
func RegisterSpecCorpusIndexer(fn SpecCorpusReindexFunc) {
	specCorpusReindex = fn
}

// EnqueueSpecCorpusReindex enqueues rag:reindex_specs for the §121 spec
// corpus sweep. dir may be empty — the worker resolves the configured
// corpus root. Best-effort: a nil enqueuer is a no-op.
func EnqueueSpecCorpusReindex(dir string) error {
	if taskEnqueuer == nil {
		return errors.New("worker: no task enqueuer configured")
	}

	payload, err := json.Marshal(map[string]string{"dir": dir})
	if err != nil {
		return fmt.Errorf("worker: marshal rag:reindex_specs payload: %w", err)
	}

	return taskEnqueuer.Enqueue(TypeRAGReindexSpecs, payload)
}

// HandleRAGReindexSpecs processes rag:reindex_specs — walks the spec corpus
// root, indexes changed files as private spec_doc chunks, and sweeps deleted
// paths (§121). The payload is optional: {"dir": "..."} overrides the
// configured corpus root.
func HandleRAGReindexSpecs(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:rag:reindex_specs")

	if specCorpusReindex == nil {
		slog.Warn("task:rag:reindex_specs: no spec corpus indexer registered, skipping")
		return nil
	}

	var payload struct {
		Dir string `json:"dir"`
	}

	if len(t.Payload()) > 0 {
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("task:rag:reindex_specs: invalid payload: %w: %w", err, asynq.SkipRetry)
		}
	}

	if err := specCorpusReindex(ctx, payload.Dir); err != nil {
		return fmt.Errorf("task:rag:reindex_specs: %w", err)
	}

	return nil
}
