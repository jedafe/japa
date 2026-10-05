// Package ai — tools_forum.go implements the forum Q&A tool handlers for
// the AI Agent Executor (Section 08 forum, Section 103 tool catalog).
//
// Tools implemented here:
//
//   - forum.threads.list   — list threads for a space or platform-wide
//   - forum.threads.read   — read one thread by ID or slug
//   - forum.threads.create — create a thread (question, optionally with a poll)
//   - forum.threads.update — edit a thread (author or moderator)
//   - forum.threads.delete — soft-delete a thread (author or moderator)
//   - forum.threads.solve  — mark a thread solved, optionally naming the
//     accepted reply (solver: author or forum.thread.solve holder)
//   - forum.replies.list   — list replies on a thread
//   - forum.replies.create — reply to a thread (optionally nested)
//   - forum.replies.update — edit a reply (author or moderator)
//   - forum.replies.delete — soft-delete a reply (author or moderator)
//   - forum.vote           — up/down-vote a thread or reply; value 0 removes
//
// Privacy: forum threads/replies are user-generated content. Author rows do
// not carry an ai_exempt flag; ownership checks and moderation fall to the
// forum service (edit_own/delete_own vs *_any RBAC nodes) — the executor's
// required_permission gate runs first, then the service enforces
// author-vs-moderator semantics per call.
//
// Ref: japaconnect-engineering/sections/08-community-qa-forum.md
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

// bodyToTipTap coerces an LLM-supplied body into TipTap JSON. A body that is
// already a JSON object/array is passed through; a plain string is wrapped in
// a minimal doc/paragraph/text document.
func bodyToTipTap(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return []byte(`{"type":"doc","content":[{"type":"paragraph"}]}`), nil
	}

	// If the model sent a JSON object, trust it as a TipTap document.
	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("body is not valid JSON: %w", err)
	}
	if _, isObj := probe.(map[string]any); isObj {
		return raw, nil
	}
	if _, isArr := probe.([]any); isArr {
		return []byte(fmt.Sprintf(`{"type":"doc","content":%s}`, raw)), nil
	}
	if s, isStr := probe.(string); isStr {
		return tiptapFromText(s), nil
	}
	return nil, fmt.Errorf("body must be a string or TipTap JSON object")
}

// tiptapFromText wraps plain text in a minimal TipTap document. Newlines
// become separate paragraphs.
func tiptapFromText(s string) []byte {
	type textNode struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type para struct {
		Type    string     `json:"type"`
		Content []textNode `json:"content,omitempty"`
	}
	type doc struct {
		Type    string `json:"type"`
		Content []para `json:"content"`
	}

	d := doc{Type: "doc"}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			p := para{Type: "paragraph"}
			if line := s[start:i]; line != "" {
				p.Content = []textNode{{Type: "text", Text: line}}
			}
			d.Content = append(d.Content, p)
			start = i + 1
		}
	}
	if len(d.Content) == 0 {
		d.Content = []para{{Type: "paragraph"}}
	}

	b, err := json.Marshal(d)
	if err != nil {
		return []byte(`{"type":"doc","content":[{"type":"paragraph"}]}`)
	}
	return b
}

func forumThreadToMap(t gen.ForumThread) map[string]any {
	return map[string]any{
		"id":             uuidString(t.ID),
		"content_id":     uuidString(t.ContentID),
		"space_id":       uuidString(t.SpaceID),
		"author_user_id": uuidString(t.AuthorUserID),
		"title":          t.Title,
		"slug":           t.Slug,
		"is_anonymous":   t.IsAnonymous,
		"solved":         t.Solved,
		"vote_score":     t.VoteScore,
		"reply_count":    t.ReplyCount,
		"view_count":     t.ViewCount,
		"is_edited":      t.IsEdited,
		"created_at":     t.CreatedAt.Time.UTC().Format(time.RFC3339),
		"deleted":        t.DeletedAt.Valid,
	}
}

func forumReplyToMap(r gen.ForumReply) map[string]any {
	return map[string]any{
		"id":              uuidString(r.ID),
		"thread_id":       uuidString(r.ThreadID),
		"parent_reply_id": uuidString(r.ParentReplyID),
		"author_user_id":  uuidString(r.AuthorUserID),
		"vote_score":      r.VoteScore,
		"is_anonymous":    r.IsAnonymous,
		"is_edited":       r.IsEdited,
		"ama_pending":     r.AmaPending,
		"created_at":      r.CreatedAt.Time.UTC().Format(time.RFC3339),
		"deleted":         r.DeletedAt.Valid,
	}
}

// ---------------------------------------------------------------------------
// forum.threads.list
// ---------------------------------------------------------------------------

type forumThreadsListArgs struct {
	SpaceID string `json:"space_id,omitempty"`
	Cursor  string `json:"cursor,omitempty"`
	Limit   int32  `json:"limit,omitempty"`
}

// ForumThreadsListHandler returns a ToolHandler listing forum threads,
// either within a space or platform-wide.
func ForumThreadsListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a forumThreadsListArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		limit := a.Limit
		if limit <= 0 || limit > 100 {
			limit = 25
		}

		var cursor pgtype.Timestamptz
		if a.Cursor != "" {
			ts, err := time.Parse(time.RFC3339, a.Cursor)
			if err != nil {
				return errorToolResult("invalid_cursor", "cursor must be an RFC3339 timestamp"), nil
			}
			cursor = pgtype.Timestamptz{Time: ts, Valid: true}
		}

		var threads []gen.ForumThread
		var err error
		if a.SpaceID != "" {
			spaceID, scanErr := scanUUID(a.SpaceID)
			if scanErr != nil {
				return errorToolResult("invalid_space_id", "space_id is not a valid UUID"), nil
			}
			threads, err = deps.Forum.ListThreadsBySpace(ctx, spaceID, cursor, limit)
		} else {
			threads, err = deps.Forum.ListThreadsByPlatform(ctx, cursor, limit)
		}
		if err != nil {
			return errorToolResult("forum_list_failed", err.Error()), nil
		}

		out := make([]map[string]any, 0, len(threads))
		var nextCursor string
		for _, t := range threads {
			out = append(out, forumThreadToMap(t))
			nextCursor = t.CreatedAt.Time.UTC().Format(time.RFC3339)
		}

		return successToolResult(map[string]any{
			"threads":     out,
			"count":       len(out),
			"next_cursor": nextCursor,
		}), nil
	}
}

// ---------------------------------------------------------------------------
// forum.threads.read
// ---------------------------------------------------------------------------

type forumThreadsReadArgs struct {
	ThreadID string `json:"thread_id,omitempty"`
	Slug     string `json:"slug,omitempty"`
	// IncludeReplies also fetches the reply list (default false).
	IncludeReplies bool `json:"include_replies,omitempty"`
}

// ForumThreadsReadHandler returns a ToolHandler that reads one thread by ID
// or slug, optionally including its replies.
func ForumThreadsReadHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a forumThreadsReadArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		var thread gen.ForumThread
		var err error
		switch {
		case a.ThreadID != "":
			id, scanErr := scanUUID(a.ThreadID)
			if scanErr != nil {
				return errorToolResult("invalid_thread_id", "thread_id is not a valid UUID"), nil
			}
			thread, err = deps.Forum.GetThread(ctx, id)
		case a.Slug != "":
			thread, err = deps.Forum.GetThreadBySlug(ctx, a.Slug)
		default:
			return errorToolResult("missing_identifier", "one of thread_id or slug is required"), nil
		}
		if err != nil {
			return errorToolResult("thread_not_found", fmt.Sprintf("thread not found: %v", err)), nil
		}

		m := forumThreadToMap(thread)
		m["body"] = json.RawMessage(thread.Body)

		if a.IncludeReplies {
			replies, err := deps.Forum.ListRepliesByThread(ctx, thread.ID)
			if err != nil {
				return errorToolResult("replies_list_failed", err.Error()), nil
			}
			out := make([]map[string]any, 0, len(replies))
			for _, r := range replies {
				rm := forumReplyToMap(r)
				rm["body"] = json.RawMessage(r.Body)
				out = append(out, rm)
			}
			m["replies"] = out
		}

		return successToolResult(m), nil
	}
}

// ---------------------------------------------------------------------------
// forum.threads.create
// ---------------------------------------------------------------------------

type forumThreadCreateArgs struct {
	SpaceID     string          `json:"space_id,omitempty"`
	ContentID   string          `json:"content_id,omitempty"`
	Title       string          `json:"title"`
	Slug        string          `json:"slug,omitempty"`
	Body        json.RawMessage `json:"body,omitempty"`
	IsAnonymous bool            `json:"is_anonymous,omitempty"`
	Poll        *struct {
		Question      string   `json:"question"`
		IsMultiSelect bool     `json:"is_multi_select,omitempty"`
		CloseAt       string   `json:"close_at,omitempty"`
		Options       []string `json:"options"`
	} `json:"poll,omitempty"`
}

// ForumThreadsCreateHandler returns a ToolHandler that creates a forum
// thread. Callers must hold forum.thread.create (or the AMA node for AMA
// boards — the service resolves the board type).
func ForumThreadsCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a forumThreadCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}

		in := ForumThreadInput{Title: a.Title, Slug: a.Slug, IsAnonymous: a.IsAnonymous}

		if a.SpaceID != "" {
			id, err := scanUUID(a.SpaceID)
			if err != nil {
				return errorToolResult("invalid_space_id", "space_id is not a valid UUID"), nil
			}
			in.SpaceID = id
		}
		if a.ContentID != "" {
			id, err := scanUUID(a.ContentID)
			if err != nil {
				return errorToolResult("invalid_content_id", "content_id is not a valid UUID"), nil
			}
			in.ContentID = id
		}

		body, err := bodyToTipTap(a.Body)
		if err != nil {
			return errorToolResult("invalid_body", err.Error()), nil
		}
		in.Body = body

		if a.Poll != nil {
			p := &ForumPollInput{
				Question:      a.Poll.Question,
				IsMultiSelect: a.Poll.IsMultiSelect,
			}
			if a.Poll.CloseAt != "" {
				ts, err := time.Parse(time.RFC3339, a.Poll.CloseAt)
				if err != nil {
					return errorToolResult("invalid_poll_close_at", "poll.close_at must be an RFC3339 timestamp"), nil
				}
				p.CloseAt = pgtype.Timestamptz{Time: ts, Valid: true}
			}
			for i, label := range a.Poll.Options {
				p.Options = append(p.Options, ForumPollOptionInput{Label: label, Position: int32(i)}) //nolint:gosec // bounded by validatePollInput
			}
			in.Poll = p
		}

		thread, err := deps.Forum.CreateThread(ctx, userID, in)
		if err != nil {
			return errorToolResult("thread_create_failed", err.Error()), nil
		}

		m := forumThreadToMap(thread)
		m["body"] = json.RawMessage(thread.Body)
		return successToolResult(m), nil
	}
}

// ---------------------------------------------------------------------------
// forum.threads.update / forum.threads.delete
// ---------------------------------------------------------------------------

type forumThreadUpdateArgs struct {
	ThreadID    string          `json:"thread_id"`
	Title       string          `json:"title,omitempty"`
	Body        json.RawMessage `json:"body,omitempty"`
	IsAnonymous bool            `json:"is_anonymous,omitempty"`
}

// ForumThreadsUpdateHandler returns a ToolHandler that edits a thread. The
// forum service enforces the edit window and edit_own/edit_any split.
func ForumThreadsUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a forumThreadUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		id, err := scanUUID(a.ThreadID)
		if err != nil {
			return errorToolResult("invalid_thread_id", "thread_id is not a valid UUID"), nil
		}

		body, err := bodyToTipTap(a.Body)
		if err != nil {
			return errorToolResult("invalid_body", err.Error()), nil
		}

		thread, err := deps.Forum.UpdateThread(ctx, id, userID, a.Title, body, a.IsAnonymous)
		if err != nil {
			return errorToolResult("thread_update_failed", err.Error()), nil
		}

		m := forumThreadToMap(thread)
		m["body"] = json.RawMessage(thread.Body)
		return successToolResult(m), nil
	}
}

// ForumThreadsDeleteHandler returns a ToolHandler that soft-deletes a
// thread (author or forum.thread.delete_any moderator; enforced in the
// service layer).
func ForumThreadsDeleteHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a struct {
			ThreadID string `json:"thread_id"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		id, err := scanUUID(a.ThreadID)
		if err != nil {
			return errorToolResult("invalid_thread_id", "thread_id is not a valid UUID"), nil
		}

		if err := deps.Forum.DeleteThread(ctx, id, userID); err != nil {
			return errorToolResult("thread_delete_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"deleted": true, "thread_id": a.ThreadID}), nil
	}
}

// ---------------------------------------------------------------------------
// forum.threads.solve
// ---------------------------------------------------------------------------

// ForumThreadsSolveHandler returns a ToolHandler that marks a thread solved
// (or unsolved when solve=false), optionally recording the accepted reply.
func ForumThreadsSolveHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a struct {
			ThreadID        string `json:"thread_id"`
			AcceptedReplyID string `json:"accepted_reply_id,omitempty"`
			Unsolve         bool   `json:"unsolve,omitempty"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		id, err := scanUUID(a.ThreadID)
		if err != nil {
			return errorToolResult("invalid_thread_id", "thread_id is not a valid UUID"), nil
		}

		if a.Unsolve {
			if err := deps.Forum.UnsolveThread(ctx, id, userID); err != nil {
				return errorToolResult("thread_unsolve_failed", err.Error()), nil
			}
			return successToolResult(map[string]any{"solved": false, "thread_id": a.ThreadID}), nil
		}

		var replyID pgtype.UUID
		if a.AcceptedReplyID != "" {
			replyID, err = scanUUID(a.AcceptedReplyID)
			if err != nil {
				return errorToolResult("invalid_reply_id", "accepted_reply_id is not a valid UUID"), nil
			}
		}

		if err := deps.Forum.SolveThread(ctx, id, userID, replyID); err != nil {
			return errorToolResult("thread_solve_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"solved": true, "thread_id": a.ThreadID}), nil
	}
}

// ---------------------------------------------------------------------------
// forum.replies.list / create / update / delete
// ---------------------------------------------------------------------------

// ForumRepliesListHandler returns a ToolHandler listing replies on a thread.
func ForumRepliesListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a struct {
			ThreadID string `json:"thread_id"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		threadID, err := scanUUID(a.ThreadID)
		if err != nil {
			return errorToolResult("invalid_thread_id", "thread_id is not a valid UUID"), nil
		}

		replies, err := deps.Forum.ListRepliesByThread(ctx, threadID)
		if err != nil {
			return errorToolResult("replies_list_failed", err.Error()), nil
		}
		out := make([]map[string]any, 0, len(replies))
		for _, r := range replies {
			rm := forumReplyToMap(r)
			rm["body"] = json.RawMessage(r.Body)
			out = append(out, rm)
		}
		return successToolResult(map[string]any{"replies": out, "count": len(out)}), nil
	}
}

type forumReplyCreateArgs struct {
	ThreadID      string          `json:"thread_id"`
	ParentReplyID string          `json:"parent_reply_id,omitempty"`
	Body          json.RawMessage `json:"body"`
	IsAnonymous   bool            `json:"is_anonymous,omitempty"`
}

// ForumRepliesCreateHandler returns a ToolHandler that posts a reply to a
// thread (optionally a nested reply to another reply).
func ForumRepliesCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a forumReplyCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		threadID, err := scanUUID(a.ThreadID)
		if err != nil {
			return errorToolResult("invalid_thread_id", "thread_id is not a valid UUID"), nil
		}

		in := ForumReplyInput{IsAnonymous: a.IsAnonymous}
		if a.ParentReplyID != "" {
			id, err := scanUUID(a.ParentReplyID)
			if err != nil {
				return errorToolResult("invalid_parent_reply_id", "parent_reply_id is not a valid UUID"), nil
			}
			in.ParentReplyID = id
		}

		body, err := bodyToTipTap(a.Body)
		if err != nil {
			return errorToolResult("invalid_body", err.Error()), nil
		}
		in.Body = body

		reply, err := deps.Forum.CreateReply(ctx, threadID, userID, in)
		if err != nil {
			return errorToolResult("reply_create_failed", err.Error()), nil
		}

		m := forumReplyToMap(reply)
		m["body"] = json.RawMessage(reply.Body)
		return successToolResult(m), nil
	}
}

// ForumRepliesUpdateHandler returns a ToolHandler that edits a reply
// (author or moderator, enforced in the service layer).
func ForumRepliesUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a struct {
			ReplyID string          `json:"reply_id"`
			Body    json.RawMessage `json:"body"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		id, err := scanUUID(a.ReplyID)
		if err != nil {
			return errorToolResult("invalid_reply_id", "reply_id is not a valid UUID"), nil
		}

		body, err := bodyToTipTap(a.Body)
		if err != nil {
			return errorToolResult("invalid_body", err.Error()), nil
		}

		reply, err := deps.Forum.UpdateReply(ctx, id, userID, body)
		if err != nil {
			return errorToolResult("reply_update_failed", err.Error()), nil
		}
		m := forumReplyToMap(reply)
		m["body"] = json.RawMessage(reply.Body)
		return successToolResult(m), nil
	}
}

// ForumRepliesDeleteHandler returns a ToolHandler that soft-deletes a reply
// (author or moderator, enforced in the service layer).
func ForumRepliesDeleteHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a struct {
			ReplyID string `json:"reply_id"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		id, err := scanUUID(a.ReplyID)
		if err != nil {
			return errorToolResult("invalid_reply_id", "reply_id is not a valid UUID"), nil
		}

		if err := deps.Forum.DeleteReply(ctx, id, userID); err != nil {
			return errorToolResult("reply_delete_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"deleted": true, "reply_id": a.ReplyID}), nil
	}
}

// ---------------------------------------------------------------------------
// forum.vote
// ---------------------------------------------------------------------------

// ForumVoteHandler returns a ToolHandler that casts or removes a vote on a
// thread or reply. value: 1 upvote, -1 downvote, 0 removes the vote.
func ForumVoteHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Forum == nil {
			return errorToolResult("forum_unavailable", "forum service is not configured"), nil
		}

		var a struct {
			EntityType string `json:"entity_type"` // thread | reply
			EntityID   string `json:"entity_id"`
			Value      int16  `json:"value"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		id, err := scanUUID(a.EntityID)
		if err != nil {
			return errorToolResult("invalid_entity_id", "entity_id is not a valid UUID"), nil
		}

		if a.Value == 0 {
			if err := deps.Forum.RemoveVote(ctx, userID, a.EntityType, id); err != nil {
				return errorToolResult("vote_remove_failed", err.Error()), nil
			}
			return successToolResult(map[string]any{"voted": false, "entity_type": a.EntityType, "entity_id": a.EntityID}), nil
		}
		if a.Value != 1 && a.Value != -1 {
			return errorToolResult("invalid_vote_value", "value must be 1 (up), -1 (down), or 0 (remove)"), nil
		}

		vote, err := deps.Forum.Vote(ctx, userID, a.EntityType, id, a.Value)
		if err != nil {
			return errorToolResult("vote_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{
			"voted":       true,
			"vote_id":     uuidString(vote.ID),
			"entity_type": vote.EntityType,
			"entity_id":   uuidString(vote.EntityID),
			"value":       vote.Value,
		}), nil
	}
}
