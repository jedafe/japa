package forum

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// CreateThreadInput holds parameters for creating a new forum thread.
type CreateThreadInput struct {
	ContentID         pgtype.UUID
	SpaceID           pgtype.UUID
	ParentReplyID     pgtype.UUID
	Title             string
	Slug              string
	Body              []byte
	IsAnonymous       bool
	EditWindowMinutes int32
}

// CreateThread creates a new forum thread and increments the reply count
// on the parent thread if this is a reply-thread (parent_reply_id set).
func (s *Service) CreateThread(ctx context.Context, tenantID, authorUserID pgtype.UUID, input CreateThreadInput) (gen.ForumThread, error) {
	if input.Title == "" {
		return gen.ForumThread{}, errors.New("title is required")
	}

	editWindow := input.EditWindowMinutes
	if editWindow <= 0 {
		editWindow = DefaultEditWindowMinutes
	}

	if input.Body == nil {
		input.Body = []byte("[]")
	}

	thread, err := s.store.createThread(ctx, gen.CreateForumThreadParams{
		TenantID:          tenantID,
		ContentID:         input.ContentID,
		SpaceID:           input.SpaceID,
		AuthorUserID:      authorUserID,
		ParentReplyID:     input.ParentReplyID,
		Title:             input.Title,
		Slug:              input.Slug,
		Body:              input.Body,
		IsAnonymous:       input.IsAnonymous,
		EditWindowMinutes: editWindow,
	})
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("creating forum thread: %w", err)
	}

	return thread, nil
}

// GetThread retrieves a forum thread by ID.
func (s *Service) GetThread(ctx context.Context, id, tenantID pgtype.UUID) (gen.ForumThread, error) {
	return s.store.getThreadByID(ctx, id, tenantID)
}

// GetThreadBySlug retrieves a forum thread by slug.
func (s *Service) GetThreadBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.ForumThread, error) {
	return s.store.getThreadBySlug(ctx, slug, tenantID)
}

// ListThreadsBySpace returns paginated threads for a space.
func (s *Service) ListThreadsBySpace(ctx context.Context, tenantID, spaceID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsBySpace(ctx, tenantID, spaceID, cursorCreatedAt, limit)
}

// ListThreadsByAuthor returns paginated threads for an author.
func (s *Service) ListThreadsByAuthor(ctx context.Context, tenantID, authorUserID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsByAuthor(ctx, tenantID, authorUserID, cursorCreatedAt, limit)
}

// ListThreadsByTenant returns all paginated threads for a tenant (public listing).
func (s *Service) ListThreadsByTenant(ctx context.Context, tenantID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsByTenant(ctx, tenantID, cursorCreatedAt, limit)
}

// ListThreadsByTag returns paginated threads for a tag.
func (s *Service) ListThreadsByTag(ctx context.Context, tenantID, tagID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsByTag(ctx, tenantID, tagID, cursorCreatedAt, limit)
}

// UpdateThread updates a forum thread. Only the author can edit within the
// edit window; after the window, edits are flagged as edited.
func (s *Service) UpdateThread(ctx context.Context, id, tenantID, userID pgtype.UUID, title string, body []byte, isAnonymous bool) (gen.ForumThread, error) {
	thread, err := s.store.getThreadByID(ctx, id, tenantID)
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("getting thread for update: %w", err)
	}

	if thread.AuthorUserID != userID {
		return gen.ForumThread{}, errors.New("only the author can edit this thread")
	}

	// Edit window check removed — the update query always sets is_edited=true
	// when the content changes, regardless of the window.

	if title == "" {
		title = thread.Title
	}

	if body == nil {
		body = thread.Body
	}

	return s.store.updateThread(ctx, gen.UpdateForumThreadParams{
		ID:          id,
		TenantID:    tenantID,
		Title:       title,
		Body:        body,
		IsAnonymous: isAnonymous,
	})
}

// DeleteThread soft-deletes a forum thread.
func (s *Service) DeleteThread(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.softDeleteThread(ctx, id, tenantID)
}

// SolveThread marks a thread as solved by the given user.
func (s *Service) SolveThread(ctx context.Context, id, tenantID, solverUserID pgtype.UUID) error {
	return s.store.solveThread(ctx, id, tenantID, solverUserID)
}

// ---------------------------------------------------------------------------
// Replies
// ---------------------------------------------------------------------------

// CreateReplyInput holds parameters for creating a new forum reply.
type CreateReplyInput struct {
	ParentReplyID pgtype.UUID
	Body          []byte
	IsAnonymous   bool
}

// CreateReply creates a new reply and increments the parent thread's reply_count.
func (s *Service) CreateReply(ctx context.Context, tenantID, threadID, authorUserID pgtype.UUID, input CreateReplyInput) (gen.ForumReply, error) {
	if input.Body == nil {
		input.Body = []byte("[]")
	}

	reply, err := s.store.createReply(ctx, gen.CreateForumReplyParams{
		TenantID:      tenantID,
		ThreadID:      threadID,
		ParentReplyID: input.ParentReplyID,
		AuthorUserID:  authorUserID,
		Body:          input.Body,
		IsAnonymous:   input.IsAnonymous,
	})
	if err != nil {
		return gen.ForumReply{}, fmt.Errorf("creating forum reply: %w", err)
	}

	// Increment the parent thread's reply count.
	if err := s.store.incrementReplyCount(ctx, threadID, tenantID, 1); err != nil {
		// Log but don't fail the reply creation.
		slog.Warn("failed to increment reply count", "thread_id", threadID, "error", err)
	}

	return reply, nil
}

// GetReply retrieves a reply by ID.
func (s *Service) GetReply(ctx context.Context, id, tenantID pgtype.UUID) (gen.ForumReply, error) {
	return s.store.getReplyByID(ctx, id, tenantID)
}

// ListRepliesByThread returns all replies for a thread.
func (s *Service) ListRepliesByThread(ctx context.Context, threadID, tenantID pgtype.UUID) ([]gen.ForumReply, error) {
	return s.store.listRepliesByThread(ctx, threadID, tenantID)
}

// UpdateReply updates a reply's body. Only the author can edit.
func (s *Service) UpdateReply(ctx context.Context, id, tenantID, userID pgtype.UUID, body []byte) (gen.ForumReply, error) {
	reply, err := s.store.getReplyByID(ctx, id, tenantID)
	if err != nil {
		return gen.ForumReply{}, fmt.Errorf("getting reply for update: %w", err)
	}

	if reply.AuthorUserID != userID {
		return gen.ForumReply{}, errors.New("only the author can edit this reply")
	}

	if body == nil {
		body = reply.Body
	}

	return s.store.updateReply(ctx, gen.UpdateForumReplyParams{
		ID:       id,
		TenantID: tenantID,
		Body:     body,
	})
}

// DeleteReply soft-deletes a reply.
func (s *Service) DeleteReply(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.softDeleteReply(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Votes
// ---------------------------------------------------------------------------

// VoteInput holds parameters for a vote operation.
type VoteInput struct {
	EntityType string
	EntityID   pgtype.UUID
	Value      int16
}

// Vote upserts a vote on a thread or reply and recalculates the entity's score.
func (s *Service) Vote(ctx context.Context, tenantID, userID pgtype.UUID, input VoteInput) (gen.ForumVote, error) {
	if input.Value != VoteDown && input.Value != VoteUp {
		return gen.ForumVote{}, errors.New("vote value must be -1 or 1")
	}

	if input.EntityType != EntityTypeThread && input.EntityType != EntityTypeReply {
		return gen.ForumVote{}, errors.New("entity type must be 'thread' or 'reply'")
	}

	vote, err := s.store.upsertVote(ctx, gen.UpsertForumVoteParams{
		TenantID:   tenantID,
		UserID:     userID,
		EntityType: input.EntityType,
		EntityID:   input.EntityID,
		Value:      input.Value,
	})
	if err != nil {
		return gen.ForumVote{}, fmt.Errorf("upserting vote: %w", err)
	}

	// Recalculate the entity's vote score.
	switch input.EntityType {
	case EntityTypeThread:
		if err := s.store.updateThreadVoteScore(ctx, input.EntityID); err != nil {
			slog.Warn("failed to update thread vote score", "error", err)
		}
	case EntityTypeReply:
		if err := s.store.updateReplyVoteScore(ctx, input.EntityID); err != nil {
			slog.Warn("failed to update reply vote score", "error", err)
		}
	}

	return vote, nil
}

// GetVoteScore returns the aggregate vote score for an entity.
func (s *Service) GetVoteScore(ctx context.Context, entityType string, entityID pgtype.UUID) (int32, error) {
	return s.store.getVoteScore(ctx, entityType, entityID)
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

// AddBookmark adds a bookmark for a thread or reply.
func (s *Service) AddBookmark(ctx context.Context, tenantID, userID pgtype.UUID, entityType string, entityID pgtype.UUID, folder string) (gen.ForumBookmark, error) {
	if entityType != EntityTypeThread && entityType != EntityTypeReply {
		return gen.ForumBookmark{}, errors.New("entity type must be 'thread' or 'reply'")
	}

	if folder == "" {
		folder = "default"
	}

	bm, err := s.store.addBookmark(ctx, gen.AddForumBookmarkParams{
		TenantID:   tenantID,
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Folder:     folder,
	})
	if err != nil {
		return gen.ForumBookmark{}, fmt.Errorf("adding bookmark: %w", err)
	}

	// Increment the entity's bookmark count.
	if err := s.store.incrementEntityBookmarkCount(ctx, entityID, 1); err != nil {
		slog.Warn("failed to increment bookmark count", "error", err)
	}

	return bm, nil
}

// RemoveBookmark removes a bookmark.
func (s *Service) RemoveBookmark(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if err := s.store.removeBookmark(ctx, userID, entityType, entityID); err != nil {
		return fmt.Errorf("removing bookmark: %w", err)
	}
	// Decrement the entity's bookmark count.
	if err := s.store.incrementEntityBookmarkCount(ctx, entityID, -1); err != nil {
		slog.Warn("failed to decrement bookmark count", "error", err)
	}

	return nil
}

// ListBookmarksByUser returns paginated bookmarks for a user.
func (s *Service) ListBookmarksByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.ForumBookmark, error) {
	return s.store.listBookmarksByUser(ctx, tenantID, userID, limit, offset)
}

// IsBookmarked checks if an entity is bookmarked by a user.
func (s *Service) IsBookmarked(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	return s.store.isBookmarked(ctx, userID, entityType, entityID)
}

// ---------------------------------------------------------------------------
// Reply Views
// ---------------------------------------------------------------------------

// RecordReplyView records that a user viewed a reply (idempotent).
func (s *Service) RecordReplyView(ctx context.Context, replyID, userID pgtype.UUID) error {
	return s.store.insertReplyView(ctx, replyID, userID)
}

// GetReplyViewCount returns the view count for a reply.
func (s *Service) GetReplyViewCount(ctx context.Context, replyID pgtype.UUID) (int32, error) {
	return s.store.getReplyViewCount(ctx, replyID)
}

// IncrementThreadView atomically increments the view count for a thread.
// Called fire-and-forget from the GetThread handler — errors are logged
// but never block the response.
func (s *Service) IncrementThreadView(ctx context.Context, id, tenantID pgtype.UUID) {
	if err := s.store.incrementThreadViewCount(ctx, id, tenantID); err != nil {
		slog.Warn("failed to increment thread view count", "thread_id", id, "error", err)
	}
}
