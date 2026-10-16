// Package ai — tools_comments.go implements the comments tool handlers for
// the AI Agent Executor (Section 49 comments, Section 103 tool catalog).
//
// Tools implemented here:
//
//   - comments.list   — list comments on a content item, or replies to a
//     comment when parent_id is given
//   - comments.create — post a comment or nested reply
//   - comments.update — edit a comment (author edit window enforced in the
//     service layer)
//   - comments.delete — soft-delete own comment (moderator removal stays on
//     the moderation surfaces)
//   - comments.vote   — up/down-vote a comment; value 0 removes the vote
//
// Ref: japaconnect-engineering/sections/49-comments.md
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// comments.list
// ---------------------------------------------------------------------------

// CommentsListHandler returns a ToolHandler listing comments on a content
// item (or replies under a parent comment when parent_id is supplied).
func CommentsListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Comments == nil {
			return errorToolResult("comments_unavailable", "comments service is not configured"), nil
		}

		var a struct {
			ContentID string `json:"content_id,omitempty"`
			ParentID  string `json:"parent_id,omitempty"`
			Sort      string `json:"sort,omitempty"` // new | oldest | top
			Limit     int32  `json:"limit,omitempty"`
			Offset    int32  `json:"offset,omitempty"`
		}

		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		limit := a.Limit
		if limit <= 0 || limit > 100 {
			limit = 25
		}

		if a.ParentID != "" {
			parentID, err := scanUUID(a.ParentID)
			if err != nil {
				return errorToolResult("invalid_parent_id", "parent_id is not a valid UUID"), nil
			}

			replies, err := deps.Comments.ListRepliesByParent(ctx, parentID, userID, limit, a.Offset)
			if err != nil {
				return errorToolResult("comments_list_failed", err.Error()), nil
			}

			out := make([]any, 0, len(replies))
			for _, r := range replies {
				out = append(out, r)
			}

			return successToolResult(map[string]any{"replies": out, "count": len(out)}), nil
		}

		if a.ContentID == "" {
			return errorToolResult("missing_content_id", "content_id is required (or pass parent_id to list replies)"), nil
		}

		contentID, err := scanUUID(a.ContentID)
		if err != nil {
			return errorToolResult("invalid_content_id", "content_id is not a valid UUID"), nil
		}

		sort := a.Sort
		if sort == "" {
			sort = "new"
		}

		comments, err := deps.Comments.ListCommentsByContent(ctx, contentID, userID, sort, limit, a.Offset)
		if err != nil {
			return errorToolResult("comments_list_failed", err.Error()), nil
		}

		out := make([]any, 0, len(comments))
		for _, c := range comments {
			out = append(out, c)
		}

		return successToolResult(map[string]any{"comments": out, "count": len(out)}), nil
	}
}

// ---------------------------------------------------------------------------
// comments.create
// ---------------------------------------------------------------------------

// CommentsCreateHandler returns a ToolHandler that posts a comment on a
// content item, or a nested reply when parent_id is supplied.
func CommentsCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Comments == nil {
			return errorToolResult("comments_unavailable", "comments service is not configured"), nil
		}

		var a struct {
			ContentID   string          `json:"content_id"`
			ParentID    string          `json:"parent_id,omitempty"`
			Body        json.RawMessage `json:"body"`
			IsAnonymous bool            `json:"is_anonymous,omitempty"`
		}

		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		in := CommentCreateInput{IsAnonymous: a.IsAnonymous}

		contentID, err := scanUUID(a.ContentID)
		if err != nil {
			return errorToolResult("invalid_content_id", "content_id is not a valid UUID"), nil
		}

		in.ContentID = contentID

		if a.ParentID != "" {
			parentID, err := scanUUID(a.ParentID)
			if err != nil {
				return errorToolResult("invalid_parent_id", "parent_id is not a valid UUID"), nil
			}

			in.ParentID = parentID
		}

		body, err := bodyToTipTap(a.Body)
		if err != nil {
			return errorToolResult("invalid_body", err.Error()), nil
		}

		in.Body = body

		comment, err := deps.Comments.CreateComment(ctx, userID, in)
		if err != nil {
			return errorToolResult("comment_create_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"id":         uuidString(comment.ID),
			"content_id": uuidString(comment.ContentID),
			"parent_id":  uuidString(comment.ParentID),
			"depth":      comment.Depth,
			"created_at": comment.CreatedAt.Time.UTC().Format(time.RFC3339),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// comments.update / comments.delete
// ---------------------------------------------------------------------------

// CommentsUpdateHandler returns a ToolHandler that edits a comment body.
// The comments service enforces the author edit window.
func CommentsUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Comments == nil {
			return errorToolResult("comments_unavailable", "comments service is not configured"), nil
		}

		var a struct {
			CommentID string          `json:"comment_id"`
			Body      json.RawMessage `json:"body"`
		}

		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		id, err := scanUUID(a.CommentID)
		if err != nil {
			return errorToolResult("invalid_comment_id", "comment_id is not a valid UUID"), nil
		}

		body, err := bodyToTipTap(a.Body)
		if err != nil {
			return errorToolResult("invalid_body", err.Error()), nil
		}

		comment, err := deps.Comments.UpdateComment(ctx, id, userID, body)
		if err != nil {
			return errorToolResult("comment_update_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"id":        uuidString(comment.ID),
			"is_edited": comment.IsEdited,
		}), nil
	}
}

// CommentsDeleteHandler returns a ToolHandler that soft-deletes the caller's
// own comment. Moderator removals (comments.delete_any) stay on the
// moderation REST surface — see Section 49.
func CommentsDeleteHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Comments == nil {
			return errorToolResult("comments_unavailable", "comments service is not configured"), nil
		}

		var a struct {
			CommentID string `json:"comment_id"`
		}

		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		id, err := scanUUID(a.CommentID)
		if err != nil {
			return errorToolResult("invalid_comment_id", "comment_id is not a valid UUID"), nil
		}

		if err := deps.Comments.SoftDeleteCommentByAuthor(ctx, id, userID); err != nil {
			return errorToolResult("comment_delete_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{"deleted": true, "comment_id": a.CommentID}), nil
	}
}

// ---------------------------------------------------------------------------
// comments.vote
// ---------------------------------------------------------------------------

// CommentsVoteHandler returns a ToolHandler that casts or removes a vote on
// a comment. value: 1 upvote, -1 downvote, 0 removes the vote.
func CommentsVoteHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Comments == nil {
			return errorToolResult("comments_unavailable", "comments service is not configured"), nil
		}

		var a struct {
			CommentID string `json:"comment_id"`
			Value     int16  `json:"value"`
		}

		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		id, err := scanUUID(a.CommentID)
		if err != nil {
			return errorToolResult("invalid_comment_id", "comment_id is not a valid UUID"), nil
		}

		if a.Value == 0 {
			if err := deps.Comments.RemoveCommentVote(ctx, id, userID); err != nil {
				return errorToolResult("vote_remove_failed", err.Error()), nil
			}

			return successToolResult(map[string]any{"voted": false, "comment_id": a.CommentID}), nil
		}

		if a.Value != 1 && a.Value != -1 {
			return errorToolResult("invalid_vote_value", "value must be 1 (up), -1 (down), or 0 (remove)"), nil
		}

		if err := deps.Comments.VoteComment(ctx, id, userID, a.Value); err != nil {
			return errorToolResult("vote_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{"voted": true, "comment_id": a.CommentID, "value": a.Value}), nil
	}
}
