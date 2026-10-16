package server

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/users"
)

// publicBookmarkAdapter satisfies publicapi.BookmarkService by composing the
// session-side bookmark services: users.Service owns the polymorphic list +
// idempotent create (folder resolution included), content.Service owns the
// entity-scoped remove and folder-ownership check. Engagement counters go
// through the same Asynq task the session endpoints use, via a method value
// on content.Handlers.
//
// Ref: spec 2026-09-24-agent-data-api-phase3 (Phase 3 bookmarks).
type publicBookmarkAdapter struct {
	users   *users.Service
	content *content.Service
	// queries carries the IsContentVisibleToUser guard — the PUT handler
	// must 404 (never 403) on invisible/absent content so the endpoint
	// can't probe for restricted rows. Read pool is fine: bookmarks are
	// not a publish-path check.
	queries  *gen.Queries
	onChange func(entityType string, entityID pgtype.UUID, engagementType string, delta int)
}

func (a *publicBookmarkAdapter) ListBookmarks(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]publicapi.PublicBookmarkItem, error) {
	rows, err := a.users.ListUserBookmarks(ctx, userID, pgtype.UUID{}, nil, limit, offset)
	if err != nil {
		return nil, err
	}

	items := make([]publicapi.PublicBookmarkItem, len(rows))
	for i, b := range rows {
		items[i] = publicapi.PublicBookmarkItem{
			ID:         b.ID,
			EntityType: b.EntityType,
			EntityID:   b.EntityID,
			Folder:     b.Folder,
			CreatedAt:  b.CreatedAt,
		}
		if b.FolderID != nil {
			items[i].FolderID = *b.FolderID
		}

		if b.Title != nil {
			items[i].Title = *b.Title
		}

		if b.Slug != nil {
			items[i].Slug = *b.Slug
		}
	}

	return items, nil
}

func (a *publicBookmarkAdapter) AddContentBookmark(ctx context.Context, userID, contentID, folderID pgtype.UUID) (publicapi.PublicBookmarkItem, error) {
	// Existence+visibility guard before any write — the row must be a
	// published, viewer-visible content item.
	visible, err := a.queries.IsContentVisibleToUser(ctx, gen.IsContentVisibleToUserParams{
		ContentID: contentID,
		UserID:    userID,
	})
	if err != nil {
		return publicapi.PublicBookmarkItem{}, fmt.Errorf("check content visibility: %w", err)
	}

	if !visible {
		return publicapi.PublicBookmarkItem{}, fmt.Errorf("%w: content", errpkg.ErrNotFound)
	}

	if folderID.Valid {
		if err := a.content.ValidateFolderOwnership(ctx, folderID, userID); err != nil {
			return publicapi.PublicBookmarkItem{}, err
		}
	}

	// The bookmarks upsert makes PUT idempotent, but the engagement counter
	// must only move when the row is actually new.
	already, err := a.content.IsBookmarked(ctx, userID, content.EntityContent, contentID)
	if err != nil {
		return publicapi.PublicBookmarkItem{}, fmt.Errorf("check bookmark: %w", err)
	}

	bm, err := a.users.CreateBookmark(ctx, userID, contentID, folderID, content.EntityContent, "")
	if err != nil {
		return publicapi.PublicBookmarkItem{}, err
	}

	if !already && a.onChange != nil {
		a.onChange(content.EntityContent, contentID, "bookmark", 1)
	}

	return publicapi.PublicBookmarkItem{
		ID:         bm.ID.String(),
		EntityType: bm.EntityType,
		EntityID:   bm.EntityID.String(),
		Folder:     bm.Folder,
		CreatedAt:  bm.CreatedAt.Time.UTC().Format(time.RFC3339),
	}, nil
}

func (a *publicBookmarkAdapter) RemoveContentBookmark(ctx context.Context, userID, contentID pgtype.UUID) error {
	exists, err := a.content.IsBookmarked(ctx, userID, content.EntityContent, contentID)
	if err != nil {
		return fmt.Errorf("check bookmark: %w", err)
	}

	if !exists {
		return fmt.Errorf("%w: bookmark", errpkg.ErrNotFound)
	}

	if err := a.content.RemoveBookmark(ctx, userID, content.EntityContent, contentID); err != nil {
		return err
	}

	if a.onChange != nil {
		a.onChange(content.EntityContent, contentID, "bookmark", -1)
	}

	return nil
}

var _ publicapi.BookmarkService = (*publicBookmarkAdapter)(nil)
