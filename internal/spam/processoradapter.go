// Package spam — processoradapter.go implements worker.ModerationProcessor by
// running content through the SpamPipeline.
package spam

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// WorkerAdapter wraps a SpamPipeline to satisfy the worker.ModerationProcessor
// interface. Registered at startup so the content:scan_moderation background
// handler can invoke real moderation checks instead of the noop fallback.
type WorkerAdapter struct {
	pipeline *SpamPipeline
}

// NewWorkerAdapter creates a WorkerAdapter backed by the given pipeline.
func NewWorkerAdapter(p *SpamPipeline) *WorkerAdapter {
	return &WorkerAdapter{pipeline: p}
}

// ScanContent runs the moderation pipeline on the given entity.
// For content entities, it looks up the body text from the database
// and runs the full spam pipeline against it.
func (a *WorkerAdapter) ScanContent(ctx context.Context, entityType, entityID, tenantID string) error {
	if a.pipeline == nil {
		slog.Warn("spam:worker_adapter: no pipeline configured, skipping",
			"entity_type", entityType, "entity_id", entityID)
		return nil
	}

	var tid pgtype.UUID
	if err := tid.Scan(tenantID); err != nil {
		return fmt.Errorf("spam: invalid tenant_id %q: %w", tenantID, err)
	}

	// Look up the entity body so the pipeline scores real text instead of
	// an empty string. Content handlers enqueue with entityType "content"
	// (internal/content/handlers.go); the legacy "post" alias is also
	// accepted for backward compatibility. Comments are stored in the
	// comments table with a []byte body column.
	var body string

	if a.pipeline.db != nil {
		var eid pgtype.UUID
		if err := eid.Scan(entityID); err == nil {
			queries := gen.New(a.pipeline.db.Reader)

			switch entityType {
			case "post", "content":
				if row, err := queries.GetContentByID(ctx, gen.GetContentByIDParams{
					ID:       eid,
					TenantID: tid,
				}); err == nil {
					body = row.Body
				}
			case "comment":
				if row, err := queries.GetComment(ctx, gen.GetCommentParams{
					ID:       eid,
					TenantID: tid,
				}); err == nil {
					body = string(row.Body)
				}
			}
		}
	}

	result, err := a.pipeline.Check(ctx, tid, SpamCheckRequest{
		EntityType: entityType,
		Content:    body,
	})
	if err != nil {
		return fmt.Errorf("spam: pipeline check failed for %s/%s: %w", entityType, entityID, err)
	}

	switch {
	case result.Blocked:
		slog.Warn("spam:worker_adapter: content blocked",
			"entity_type", entityType, "entity_id", entityID,
			"score", result.Score, "reasons", result.Reasons)
	case result.Flagged:
		slog.Info("spam:worker_adapter: content flagged for review",
			"entity_type", entityType, "entity_id", entityID,
			"score", result.Score, "reasons", result.Reasons)
	default:
		slog.Debug("spam:worker_adapter: content clean",
			"entity_type", entityType, "entity_id", entityID, "score", result.Score)
	}

	return nil
}
