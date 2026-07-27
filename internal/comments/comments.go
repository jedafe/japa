// Package comments implements the unified comment system for JapaConnect
// (Section 49). Comments are attached to any content type via content_id.
// Nested replies support up to depth 3. Votes use upsert semantics.
package comments

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ErrEditWindowExpired is returned when the author tries to edit a comment
// after the `comments.edit_window_minutes` window has elapsed (BL-009 /
// Section 49). The sqlc UpdateComment query already enforces author-only
// edits, so this check only applies to the author.
var ErrEditWindowExpired = errors.New("comment edit window has expired")

// defaultEditWindowMinutes is the fallback for comments.edit_window_minutes
// when the setting is unset or no settings reader is wired (BL-009).
const defaultEditWindowMinutes = 60

// settingsReader is the subset of *config.Settings the comments service needs
// for hot-reloadable operational limits (edit window). *config.Settings
// satisfies this structurally. When nil, compiled-in defaults are used.
type settingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// Service provides comment operations with reader/writer pool separation.
type Service struct {
	db       *db.DB
	read     *gen.Queries
	write    *gen.Queries
	settings settingsReader
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
