package server

// Adapter satisfying publicapi.CommentService — Phase 4 comment writes for
// API keys (spec 2026-09-24-agent-data-api-phase3). Composes the session-side
// abuse model unchanged: visibility guard (404, never 403), spam pipeline
// check + rate-limit recording, space commenting/replying permissions,
// notification dispatch, and the engagement-count enqueue.
//
// Ref: .jeos/sections/47-public-api-developer-portal.md (Phase 4 comment writes)

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/comments"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/spam"
)

type publicCommentAdapter struct {
	comments *comments.Service
	spam     *spam.SpamPipeline
	space    comments.SpaceCommentChecker
	notifier comments.NotificationDispatcher
	queries  *gen.Queries
	onCreate func(entityType string, entityID pgtype.UUID, engagementType string, delta int)
}

func (a *publicCommentAdapter) CreateComment(ctx context.Context, userID, contentID, parentID pgtype.UUID, body, clientIP string) (publicapi.PublicComment, error) {
	// Existence+visibility guard — invisible content is 404, never 403.
	visible, err := a.queries.IsContentVisibleToUser(ctx, gen.IsContentVisibleToUserParams{
		ContentID: contentID,
		UserID:    userID,
	})
	if err != nil {
		return publicapi.PublicComment{}, fmt.Errorf("check content visibility: %w", err)
	}

	if !visible {
		return publicapi.PublicComment{}, fmt.Errorf("%w: content", errpkg.ErrNotFound)
	}

	// Spam pipeline — same check the session handler runs (BE-034).
	if a.spam != nil {
		spamResult, err := a.spam.Check(ctx, spam.SpamCheckRequest{
			Content:    body,
			UserID:     userID,
			IP:         clientIP,
			UserType:   "existing",
			EntityType: "comment",
		})
		if err != nil {
			slog.Warn("public comment: spam check failed", "error", err)
		} else if spamResult.Blocked {
			return publicapi.PublicComment{}, publicapi.ErrContentBlocked
		}
	}

	// Space permissions — commenting_perm for top-level, replying_perm for
	// replies (Section 05). Skipped when the content has no space.
	if a.space != nil {
		spaceID, err := a.comments.GetContentSpaceID(ctx, contentID)
		if err != nil {
			slog.Warn("public comment: failed to resolve content space", "content_id", contentID.String(), "error", err)
		} else if spaceID.Valid {
			var allowed bool
			if parentID.Valid {
				allowed, err = a.space.CanReplyInSpace(ctx, spaceID, userID)
			} else {
				allowed, err = a.space.CanCommentInSpace(ctx, spaceID, userID)
			}

			if err != nil {
				slog.Warn("public comment: space perm check failed", "space_id", spaceID.String(), "error", err)
			} else if !allowed {
				return publicapi.PublicComment{}, fmt.Errorf("%w: space commenting", errpkg.ErrForbidden)
			}
		}
	}

	// Plain text → minimal TipTap paragraph document (comments.body is jsonb).
	doc, err := json.Marshal(map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": body},
				},
			},
		},
	})
	if err != nil {
		return publicapi.PublicComment{}, fmt.Errorf("marshal comment doc: %w", err)
	}

	comment, err := a.comments.CreateComment(ctx, userID, comments.CreateCommentInput{
		ContentID: contentID,
		ParentID:  parentID,
		Body:      doc,
	})
	if err != nil {
		return publicapi.PublicComment{}, err
	}

	// Same post-create side effects as the session handler (BUG-004 rate
	// layer, §49 notifications, §32 engagement counter) — fire-and-forget.
	if a.spam != nil {
		a.spam.RecordContentCreation(ctx, userID)
	}

	a.comments.DispatchCommentNotifications(ctx, comment, a.notifier)

	if a.onCreate != nil {
		a.onCreate("content", contentID, "comment", 1)
	}

	out := publicapi.PublicComment{
		ID:        comment.ID.String(),
		ContentID: contentID.String(),
		Depth:     comment.Depth,
	}
	if comment.ParentID.Valid {
		out.ParentID = comment.ParentID.String()
	}

	if comment.CreatedAt.Valid {
		out.CreatedAt = comment.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}

	return out, nil
}
