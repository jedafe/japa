// Package comments implements the unified comment system for JapaConnect
// (Section 49). Comments are attached to any content type via content_id.
// Nested replies support up to depth 3. Votes use upsert semantics.
package comments

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/mentions"
	"github.com/jedafe/japa/internal/wallet"
)

// defaultEditWindowMinutes is the fallback for comments.edit_window_minutes
// when the setting is unset or no settings reader is wired (BL-001). Edits
// within this window are "clean" (no is_edited mark); edits after are still
// allowed but marked as edited per Section 49.
const defaultEditWindowMinutes = 60

// settingsReader is the subset of *config.Settings the comments service needs
// for hot-reloadable operational limits (edit window). *config.Settings
// satisfies this structurally. When nil, compiled-in defaults are used.
type settingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// Service provides comment operations with reader/writer pool separation.
type Service struct {
	db              *db.DB
	read            *gen.Queries
	write           *gen.Queries
	settings        settingsReader
	mentionCheckers *mentions.DBCheckers // nil disables privacy filtering (BUG-004)
	activity        wallet.ActivityEmitter
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		db:    database,
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// SetSettings wires the hot-reloadable settings reader (BL-009). Optional —
// when not called, the service falls back to the compiled-in default for
// comments.edit_window_minutes.
func (s *Service) SetSettings(sr settingsReader) { s.settings = sr }

// SetActivityEmitter injects the wallet activity emitter used to record
// comment.created events so §64 badge/challenge triggers (e.g. first_comment)
// can fire. Optional — nil-safe.
func (s *Service) SetActivityEmitter(e wallet.ActivityEmitter) { s.activity = e }

// emitActivity records a wallet activity event and enqueues its processing.
// Fire-and-forget (Rule 8): failures are logged, never propagated. A
// duplicate reference hits ON CONFLICT DO NOTHING and returns ErrNoRows,
// which is treated as "already recorded" rather than an error.
func (s *Service) emitActivity(ctx context.Context, tenantID, userID pgtype.UUID, key, reference string, payload map[string]interface{}) {
	if s.activity == nil || !userID.Valid {
		return
	}

	event, err := s.activity.EmitActivityEvent(ctx, tenantID, userID, wallet.EmitActivityEventInput{
		ActivityKey: key,
		Reference:   reference,
		Payload:     payload,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("comments: emit activity event failed", "activity_key", key, "error", err)
		}

		return
	}

	if err := s.activity.EnqueueProcessActivityEvent(ctx, event.ID, tenantID); err != nil {
		slog.Warn("comments: enqueue activity event failed", "activity_key", key, "error", err)
	}
}

// SetMentionCheckers wires the DB-backed privacy checkers (blocks, erased
// users, private-space membership) used by FilterMentions for comment
// mention notifications (BUG-004). nil disables privacy filtering.
func (s *Service) SetMentionCheckers(c *mentions.DBCheckers) { s.mentionCheckers = c }
