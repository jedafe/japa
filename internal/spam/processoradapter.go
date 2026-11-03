// Package spam — processoradapter.go implements worker.ModerationProcessor by
// running content through the SpamPipeline.
package spam

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
)

// WorkerAdapter wraps a SpamPipeline to satisfy the worker.ModerationProcessor
// interface. Registered at startup so the content:scan_moderation background
// handler can invoke real moderation checks instead of the noop fallback.
type WorkerAdapter struct {
	pipeline *SpamPipeline
	// activity emits the spam.detected points penalty (ADR-136).
	activity ActivityEmitter
	// notifier warns the author about flagged content.
	notifier Notifier
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
	// Payload carries the pipeline verdict (score + reasons) so the points
	// ledger entry explains itself and admins can judge reversals.
	Payload map[string]any
}

// SpamActivityEvent mirrors the event id the enqueue call needs.
type SpamActivityEvent struct {
	ID pgtype.UUID
}

// SetActivityEmitter injects the wallet emitter (ADR-136).
func (a *WorkerAdapter) SetActivityEmitter(e ActivityEmitter) { a.activity = e }

// Notifier warns the author when their content trips the pipeline — the
// violation reason travels with the notice so a false positive is fixable,
// not a silent points loss. Local interface avoids a spam→notifications
// import edge.
type Notifier interface {
	NotifySpamFlag(ctx context.Context, userID pgtype.UUID, entityType, entityID string, blocked bool, reasons []string) error
}

// SetNotifier injects the notification dispatcher.
func (a *WorkerAdapter) SetNotifier(n Notifier) { a.notifier = n }

// emitSpamPenalty deducts points for spam-flagged/blocked content —
// fire-and-forget; deduped per entity via (activity_key, reference).
// The verdict rides the event payload so the ledger shows WHY.
func (a *WorkerAdapter) emitSpamPenalty(ctx context.Context, authorID pgtype.UUID, key, reference string, result SpamCheckResult) {
	if a.activity == nil || !authorID.Valid {
		return
	}
	event, err := a.activity.EmitActivityEvent(ctx, authorID, SpamActivityInput{
		ActivityKey: key,
		Reference:   reference,
		Payload: map[string]any{
			"score":   result.Score,
			"reasons": result.Reasons,
		},
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("spam: emit activity event failed", "activity_key", key, "error", err)
		}
		return
	}
	_ = a.activity.EnqueueProcessActivityEvent(ctx, event.ID)
}

// notifyAuthor warns the author about the flagged content (fire-and-forget).
// Deduped per entity via a Valkey SETNX — repeated flagged saves of the same
// post notify once per window instead of inbox-spamming the author.
func (a *WorkerAdapter) notifyAuthor(ctx context.Context, authorID pgtype.UUID, entityType, entityID string, blocked bool, reasons []string) {
	if a.notifier == nil || !authorID.Valid {
		return
	}
	if a.pipeline.rdb != nil {
		key := valkey.MustKey("spam:notified:%s:%s", entityType, entityID)
		set, err := a.pipeline.rdb.Redis().SetNX(ctx, key, "1", 24*time.Hour).Result()
		if err == nil && !set {
			return // already notified within the window
		}
	}
	if err := a.notifier.NotifySpamFlag(ctx, authorID, entityType, entityID, blocked, reasons); err != nil {
		slog.Warn("spam: author notification failed", "entity_type", entityType, "entity_id", entityID, "error", err)
	}
}

// queueReport inserts a pending content_reports row so a Blocked verdict
// lands in the moderation queue for human review — previously a blocked
// score only hit the logs. Deduped on an existing pending report for the
// same entity.
func (a *WorkerAdapter) queueReport(ctx context.Context, entityType, entityID string, reasons []string) {
	if a.pipeline.db == nil || a.pipeline.db.Writer == nil {
		return
	}
	var eid pgtype.UUID
	if err := eid.Scan(entityID); err != nil {
		return
	}
	var existing pgtype.UUID
	if err := a.pipeline.db.Writer.QueryRow(ctx,
		`SELECT id FROM content_reports
		  WHERE entity_type = $1 AND entity_id = $2 AND status = 'pending' LIMIT 1`,
		entityType, eid).Scan(&existing); err == nil {
		return // already queued
	}
	notes := "auto-flag score: " + strings.Join(reasons, "; ")
	if _, err := gen.New(a.pipeline.db.Writer).CreateReport(ctx, gen.CreateReportParams{
		EntityType: entityType,
		EntityID:   eid,
		Reason:     "spam",
		Notes:      &notes,
	}); err != nil {
		slog.Warn("spam: queue report failed", "entity_type", entityType, "entity_id", entityID, "error", err)
	}
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
		a.queueReport(ctx, entityType, entityID, result.Reasons)
		a.notifyAuthor(ctx, authorID, entityType, entityID, true, result.Reasons)
		a.emitSpamPenalty(ctx, authorID, "spam.detected", "spam:"+entityType+":"+entityID, result)
		slog.Warn("spam:worker_adapter: content blocked",
			"entity_type", entityType, "entity_id", entityID,
			"score", result.Score, "reasons", result.Reasons)
	case result.Flagged:
		a.queueReport(ctx, entityType, entityID, result.Reasons)
		a.notifyAuthor(ctx, authorID, entityType, entityID, false, result.Reasons)
		// ADR-136: auto-flagged content may emit spam.flagged (seeded
		// disabled — opt-in; only confirmed blocks penalize by default).
		a.emitSpamPenalty(ctx, authorID, "spam.flagged", "spamflag:"+entityType+":"+entityID, result)
		slog.Info("spam:worker_adapter: content flagged for review",
			"entity_type", entityType, "entity_id", entityID,
			"score", result.Score, "reasons", result.Reasons)
	default:
		slog.Debug("spam:worker_adapter: content clean",
			"entity_type", entityType, "entity_id", entityID, "score", result.Score)
	}

	return nil
}
