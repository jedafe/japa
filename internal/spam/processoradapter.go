// Package spam — processoradapter.go implements worker.ModerationProcessor by
// running content through the SpamPipeline.
package spam

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// WorkerAdapter wraps a SpamPipeline to satisfy the worker.ModerationProcessor
// interface. Registered at startup so the content:scan_moderation background
// handler can invoke real moderation checks instead of the noop fallback.
type WorkerAdapter struct {
	pipeline *SpamPipeline
	// activity emits the spam.detected points penalty (ADR-136).
	activity ActivityEmitter
}

// NewWorkerAdapter creates a WorkerAdapter backed by the given pipeline.
func NewWorkerAdapter(p *SpamPipeline) *WorkerAdapter {
	return &WorkerAdapter{pipeline: p}
}

// ActivityEmitter records a wallet activity event — the spam.detected
// penalty (ADR-136). Local interface avoids a spam→wallet import edge.
type ActivityEmitter interface {
	EmitActivityEvent(ctx context.Context, userID pgtype.UUID, input SpamActivityInput) (SpamActivityEvent, error)
	EnqueueProcessActivityEvent(ctx context.Context, eventID pgtype.UUID) error
}

// SpamActivityInput mirrors wallet.EmitActivityEventInput.
type SpamActivityInput struct {
	ActivityKey string
	Reference   string
}

// SpamActivityEvent mirrors the event id the enqueue call needs.
type SpamActivityEvent struct {
	ID pgtype.UUID
}

// SetActivityEmitter injects the wallet emitter (ADR-136).
func (a *WorkerAdapter) SetActivityEmitter(e ActivityEmitter) { a.activity = e }

// emitSpamPenalty deducts points for spam-flagged/blocked content —
// fire-and-forget; deduped per entity via (activity_key, reference).
func (a *WorkerAdapter) emitSpamPenalty(ctx context.Context, authorID pgtype.UUID, key, reference string) {
	if a.activity == nil || !authorID.Valid {
		return
	}
	event, err := a.activity.EmitActivityEvent(ctx, authorID, SpamActivityInput{
		ActivityKey: key,
		Reference:   reference,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("spam: emit activity event failed", "activity_key", key, "error", err)
		}
		return
	}
	_ = a.activity.EnqueueProcessActivityEvent(ctx, event.ID)
}

// ScanContent runs the moderation pipeline on the given entity.
// For content entities, it looks up the body text from the database
// and runs the full spam pipeline against it.
func (a *WorkerAdapter) ScanContent(ctx context.Context, entityType, entityID string) error {
	if a.pipeline == nil {
		slog.Warn("spam:worker_adapter: no pipeline configured, skipping",
			"entity_type", entityType, "entity_id", entityID)
		return nil
	}

	// Look up the entity body so the pipeline scores real text instead of
	// an empty string. Content handlers enqueue with entityType "content"
	// (internal/content/handlers.go); the legacy "post" alias is also
	// accepted for backward compatibility. Comments are stored in the
	// comments table with a []byte body column.
	var body string
	var authorID pgtype.UUID

	if a.pipeline.db != nil {
		var eid pgtype.UUID
		if err := eid.Scan(entityID); err == nil {
			queries := gen.New(a.pipeline.db.Reader)

			switch entityType {
			case "post", "content":
				if row, err := queries.GetContentByID(ctx,
					eid,
				); err == nil {
					body = row.Body
				}
				if aid, aerr := queries.GetContentAuthorID(ctx, eid); aerr == nil {
					authorID = aid
				}
			case "comment":
				if row, err := queries.GetComment(ctx,
					eid,
				); err == nil {
					body = string(row.Body)
					authorID = row.AuthorUserID
				}
			}
		}
	}

	result, err := a.pipeline.Check(ctx, SpamCheckRequest{
		EntityType: entityType,
		Content:    body,
	})
	if err != nil {
		return fmt.Errorf("spam: pipeline check failed for %s/%s: %w", entityType, entityID, err)
	}

	switch {
	case result.Blocked:
		a.emitSpamPenalty(ctx, authorID, "spam.detected", "spam:"+entityType+":"+entityID)
		slog.Warn("spam:worker_adapter: content blocked",
			"entity_type", entityType, "entity_id", entityID,
			"score", result.Score, "reasons", result.Reasons)
	case result.Flagged:
		// ADR-136: auto-flagged content may emit spam.flagged (seeded
		// disabled — opt-in; only confirmed blocks penalize by default).
		a.emitSpamPenalty(ctx, authorID, "spam.flagged", "spamflag:"+entityType+":"+entityID)
		slog.Info("spam:worker_adapter: content flagged for review",
			"entity_type", entityType, "entity_id", entityID,
			"score", result.Score, "reasons", result.Reasons)
	default:
		slog.Debug("spam:worker_adapter: content clean",
			"entity_type", entityType, "entity_id", entityID, "score", result.Score)
	}

	return nil
}
