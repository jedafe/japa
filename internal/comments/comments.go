// Package comments implements the unified comment system for JapaConnect
// (Section 49). Comments are attached to any content type via content_id.
// Nested replies support up to depth 3. Votes use upsert semantics.
package comments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
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
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// FeatureChecker is the subset of *features.Checker the comments service
// needs to gate comment paths on the parent content's feature flag (§42).
// Nil is permitted — no gating.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
	EvalFlag(ctx context.Context, key string, sc features.CallerScope) features.EvalResult
}

// Service provides comment operations with reader/writer pool separation.
type Service struct {
	db              *db.DB
	read            *gen.Queries
	write           *gen.Queries
	settings        settingsReader
	mentionCheckers *mentions.DBCheckers // nil disables privacy filtering (BUG-004)
	activity        wallet.ActivityEmitter
	flags           FeatureChecker // nil disables §42 parent-type gating
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

// SetFeatureChecker injects the feature flag checker so comment paths on a
// parent whose content_type is flagged off become invisible (§42).
// Optional — nil-safe.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.flags = c }

// parentFlagUnreachable reports whether the parent content's type is
// feature-flagged unreachable for this caller (§42) — the .Enabled Gate
// contract, so a hidden-but-enabled type still serves comments. A missing
// parent reports not-gated so callers keep their existing not_found
// semantics.
func (s *Service) parentFlagUnreachable(ctx context.Context, contentID pgtype.UUID) (bool, error) {
	if s.flags == nil {
		return false, nil
	}
	row, err := s.read.GetContentByID(ctx, contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("checking parent feature flag: %w", err)
	}
	res, ok := features.EvalContentType(ctx, s.flags, row.ContentType)
	return ok && !res.Enabled, nil
}

// emitActivity records a wallet activity event and enqueues its processing.
// Fire-and-forget (Rule 8): failures are logged, never propagated. A
// duplicate reference hits ON CONFLICT DO NOTHING and returns ErrNoRows,
// which is treated as "already recorded" rather than an error.
func (s *Service) emitActivity(ctx context.Context, userID pgtype.UUID, key, reference string, payload map[string]interface{}) {
	if s.activity == nil || !userID.Valid {
		return
	}

	event, err := s.activity.EmitActivityEvent(ctx, userID, wallet.EmitActivityEventInput{
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

	if err := s.activity.EnqueueProcessActivityEvent(ctx, event.ID); err != nil {
		slog.Warn("comments: enqueue activity event failed", "activity_key", key, "error", err)
	}
}

// SetMentionCheckers wires the DB-backed privacy checkers (blocks, erased
// users, private-space membership) used by FilterMentions for comment
// mention notifications (BUG-004). nil disables privacy filtering.
func (s *Service) SetMentionCheckers(c *mentions.DBCheckers) { s.mentionCheckers = c }
