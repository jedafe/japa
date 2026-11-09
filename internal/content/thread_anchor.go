package content

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Thread-anchor lifecycle hooks (unified-discussion spec). Every published,
// public, non-discussion contents row gets its own canonical `discussion`
// thread row — minted here on the publish seams (create, status transition,
// PATCH-driven publish) and dropped on delete. The callback is wired to
// comments.Service.CreateAnchor/DeleteAnchor in server.go — the content
// package can't import comments (comments already imports content), so the
// dependency is injected as plain functions.
//
// Minting is best-effort and idempotent: CreateAnchor is get-or-create, so
// a row that already has an anchor resolves cheaply; a failure is logged
// and never blocks the write. Lazy paths (POST /threads/anchor from the
// entity page, the /thread/{slug} loader) backstop any missed transition.

// threadAnchorMinter mints the canonical discussion thread for a content row.
type threadAnchorMinter func(ctx context.Context, contentID, authorID pgtype.UUID) error

// threadAnchorDeleter drops the canonical discussion thread when the content
// row is deleted — the thread page dies with its article.
type threadAnchorDeleter func(ctx context.Context, contentID pgtype.UUID) error

// SetThreadAnchorMinter wires the mint-on-publish callback.
func (s *Service) SetThreadAnchorMinter(m threadAnchorMinter) { s.anchorMinter = m }

// SetThreadAnchorDeleter wires the drop-on-delete callback.
func (s *Service) SetThreadAnchorDeleter(d threadAnchorDeleter) { s.anchorDeleter = d }

// anchorableContentType reports whether the row gets its own canonical
// thread — discussions and forum backing rows already ARE threads.
func anchorableContentType(contentType string) bool {
	return contentType != "discussion" && contentType != "forum_thread"
}

// maybeMintThreadAnchor mints the canonical thread for a published content
// row, fire-and-forget. Skips thread-type rows (discussion/forum_thread) —
// minting a thread for a thread would recurse.
func (s *Service) maybeMintThreadAnchor(ctx context.Context, contentType string, id, authorID pgtype.UUID) {
	if s.anchorMinter == nil || !anchorableContentType(contentType) {
		return
	}
	if err := s.anchorMinter(ctx, id, authorID); err != nil {
		slog.Warn("content: thread anchor mint failed",
			"content_id", id.String(), "error", err)
	}
}

// maybeDeleteThreadAnchor drops the canonical thread for a deleted content
// row, fire-and-forget — no-op when the row was never anchored.
func (s *Service) maybeDeleteThreadAnchor(ctx context.Context, contentType string, id pgtype.UUID) {
	if s.anchorDeleter == nil || !anchorableContentType(contentType) {
		return
	}
	if err := s.anchorDeleter(ctx, id); err != nil {
		slog.Warn("content: thread anchor delete failed",
			"content_id", id.String(), "error", err)
	}
}

// reconcileThreadAnchorSpace re-evaluates the thread's space gate after a
// primary-space change: the mint gate only admits public+active spaces,
// so re-filing a live post into a closed space takes its /thread page
// down with it, and moving back into a public space re-mints eagerly
// (CreateAnchor is get-or-create — a still-live anchor resolves cheaply).
// A transient space read leaves the anchor as-is — the flag/space
// sweepers and the resolve gate backstop.
func (s *Service) reconcileThreadAnchorSpace(ctx context.Context, contentID, spaceID pgtype.UUID) {
	if s.anchorMinter == nil && s.anchorDeleter == nil {
		return
	}
	row, err := s.read.GetContentByID(ctx, contentID)
	if err != nil || row.Status != StatusPublished || row.Visibility != VisibilityPublic {
		return
	}
	space, err := s.read.GetSpaceByID(ctx, spaceID)
	switch {
	case err != nil:
		if errors.Is(err, pgx.ErrNoRows) {
			s.maybeDeleteThreadAnchor(ctx, row.ContentType, row.ID)
		}
	case space.Visibility == "public" && space.Status == "active":
		s.maybeMintThreadAnchor(ctx, row.ContentType, row.ID, row.AuthorID)
	default:
		s.maybeDeleteThreadAnchor(ctx, row.ContentType, row.ID)
	}
}
