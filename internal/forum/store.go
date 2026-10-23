package forum

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for forum operations. The Service
// layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
	db    *db.DB
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries, database *db.DB) *Store {
	return &Store{read: read, write: write, db: database}
}

// ---------------------------------------------------------------------------
// Thread type conversions — sqlc generates *Row types for queries with JOINs;
// these helpers convert them to the canonical ForumThread model.
// ---------------------------------------------------------------------------

func forumThreadFromCreateRow(r gen.CreateForumThreadRow) gen.ForumThread {
	return gen.ForumThread{
		ID: r.ID,

		ContentID:         r.ContentID,
		SpaceID:           r.SpaceID,
		AuthorUserID:      r.AuthorUserID,
		ParentReplyID:     r.ParentReplyID,
		Title:             r.Title,
		Slug:              r.Slug,
		Body:              r.Body,
		IsAnonymous:       r.IsAnonymous,
		Solved:            r.Solved,
		SolvedAt:          r.SolvedAt,
		SolvedByUserID:    r.SolvedByUserID,
		VoteScore:         r.VoteScore,
		ReplyCount:        r.ReplyCount,
		ViewCount:         r.ViewCount,
		BookmarkCount:     r.BookmarkCount,
		EditWindowMinutes: r.EditWindowMinutes,
		IsEdited:          r.IsEdited,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		DeletedAt:         r.DeletedAt,
	}
}

func forumThreadFromByIDRow(r gen.GetForumThreadByIDRow) gen.ForumThread {
	return gen.ForumThread{
		ID: r.ID,

		ContentID:         r.ContentID,
		SpaceID:           r.SpaceID,
		AuthorUserID:      r.AuthorUserID,
		ParentReplyID:     r.ParentReplyID,
		Title:             r.Title,
		Slug:              r.Slug,
		Body:              r.Body,
		IsAnonymous:       r.IsAnonymous,
		Solved:            r.Solved,
		SolvedAt:          r.SolvedAt,
		SolvedByUserID:    r.SolvedByUserID,
		VoteScore:         r.VoteScore,
		ReplyCount:        r.ReplyCount,
		ViewCount:         r.ViewCount,
		BookmarkCount:     r.BookmarkCount,
		EditWindowMinutes: r.EditWindowMinutes,
		IsEdited:          r.IsEdited,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		DeletedAt:         r.DeletedAt,
	}
}

func forumThreadFromBySlugRow(r gen.GetForumThreadBySlugRow) gen.ForumThread {
	return gen.ForumThread{
		ID: r.ID,

		ContentID:         r.ContentID,
		SpaceID:           r.SpaceID,
		AuthorUserID:      r.AuthorUserID,
		ParentReplyID:     r.ParentReplyID,
		Title:             r.Title,
		Slug:              r.Slug,
		Body:              r.Body,
		IsAnonymous:       r.IsAnonymous,
		Solved:            r.Solved,
		SolvedAt:          r.SolvedAt,
		SolvedByUserID:    r.SolvedByUserID,
		VoteScore:         r.VoteScore,
		ReplyCount:        r.ReplyCount,
		ViewCount:         r.ViewCount,
		BookmarkCount:     r.BookmarkCount,
		EditWindowMinutes: r.EditWindowMinutes,
		IsEdited:          r.IsEdited,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		DeletedAt:         r.DeletedAt,
	}
}

func forumThreadFromListSpaceRow(r gen.ListForumThreadsBySpaceRow) gen.ForumThread {
	return gen.ForumThread{
		ID: r.ID,

		ContentID:         r.ContentID,
		SpaceID:           r.SpaceID,
		AuthorUserID:      r.AuthorUserID,
		ParentReplyID:     r.ParentReplyID,
		Title:             r.Title,
		Slug:              r.Slug,
		Body:              r.Body,
		IsAnonymous:       r.IsAnonymous,
		Solved:            r.Solved,
		SolvedAt:          r.SolvedAt,
		SolvedByUserID:    r.SolvedByUserID,
		VoteScore:         r.VoteScore,
		ReplyCount:        r.ReplyCount,
		ViewCount:         r.ViewCount,
		BookmarkCount:     r.BookmarkCount,
		EditWindowMinutes: r.EditWindowMinutes,
		IsEdited:          r.IsEdited,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		DeletedAt:         r.DeletedAt,
	}
}

func forumThreadFromListAuthorRow(r gen.ListForumThreadsByAuthorRow) gen.ForumThread {
	return gen.ForumThread{
		ID: r.ID,

		ContentID:         r.ContentID,
		SpaceID:           r.SpaceID,
		AuthorUserID:      r.AuthorUserID,
		ParentReplyID:     r.ParentReplyID,
		Title:             r.Title,
		Slug:              r.Slug,
		Body:              r.Body,
		IsAnonymous:       r.IsAnonymous,
		Solved:            r.Solved,
		SolvedAt:          r.SolvedAt,
		SolvedByUserID:    r.SolvedByUserID,
		VoteScore:         r.VoteScore,
		ReplyCount:        r.ReplyCount,
		ViewCount:         r.ViewCount,
		BookmarkCount:     r.BookmarkCount,
		EditWindowMinutes: r.EditWindowMinutes,
		IsEdited:          r.IsEdited,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		DeletedAt:         r.DeletedAt,
	}
}

func forumThreadFromListTagRow(r gen.ListForumThreadsByTagRow) gen.ForumThread {
	return gen.ForumThread{
		ID: r.ID,

		ContentID:         r.ContentID,
		SpaceID:           r.SpaceID,
		AuthorUserID:      r.AuthorUserID,
		ParentReplyID:     r.ParentReplyID,
		Title:             r.Title,
		Slug:              r.Slug,
		Body:              r.Body,
		IsAnonymous:       r.IsAnonymous,
		Solved:            r.Solved,
		SolvedAt:          r.SolvedAt,
		SolvedByUserID:    r.SolvedByUserID,
		VoteScore:         r.VoteScore,
		ReplyCount:        r.ReplyCount,
		ViewCount:         r.ViewCount,
		BookmarkCount:     r.BookmarkCount,
		EditWindowMinutes: r.EditWindowMinutes,
		IsEdited:          r.IsEdited,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		DeletedAt:         r.DeletedAt,
	}
}

func forumThreadFromListPlatformRow(r gen.ListForumThreadsRow) gen.ForumThread {
	return gen.ForumThread{
		ID: r.ID,

		ContentID:         r.ContentID,
		SpaceID:           r.SpaceID,
		AuthorUserID:      r.AuthorUserID,
		ParentReplyID:     r.ParentReplyID,
		Title:             r.Title,
		Slug:              r.Slug,
		Body:              r.Body,
		IsAnonymous:       r.IsAnonymous,
		Solved:            r.Solved,
		SolvedAt:          r.SolvedAt,
		SolvedByUserID:    r.SolvedByUserID,
		VoteScore:         r.VoteScore,
		ReplyCount:        r.ReplyCount,
		ViewCount:         r.ViewCount,
		BookmarkCount:     r.BookmarkCount,
		EditWindowMinutes: r.EditWindowMinutes,
		IsEdited:          r.IsEdited,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		DeletedAt:         r.DeletedAt,
	}
}

func forumThreadFromUpdateRow(r gen.UpdateForumThreadRow) gen.ForumThread {
	return gen.ForumThread{
		ID: r.ID,

		ContentID:         r.ContentID,
		SpaceID:           r.SpaceID,
		AuthorUserID:      r.AuthorUserID,
		ParentReplyID:     r.ParentReplyID,
		Title:             r.Title,
		Slug:              r.Slug,
		Body:              r.Body,
		IsAnonymous:       r.IsAnonymous,
		Solved:            r.Solved,
		SolvedAt:          r.SolvedAt,
		SolvedByUserID:    r.SolvedByUserID,
		VoteScore:         r.VoteScore,
		ReplyCount:        r.ReplyCount,
		ViewCount:         r.ViewCount,
		BookmarkCount:     r.BookmarkCount,
		EditWindowMinutes: r.EditWindowMinutes,
		IsEdited:          r.IsEdited,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		DeletedAt:         r.DeletedAt,
	}
}

// ---------------------------------------------------------------------------
// Threads
// ---------------------------------------------------------------------------

// createThread inserts a new forum thread.
func (s *Store) createThread(ctx context.Context, p gen.CreateForumThreadParams) (gen.ForumThread, error) {
	t, err := s.write.CreateForumThread(ctx, p)
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("creating forum thread: %w", err)
	}

	return forumThreadFromCreateRow(t), nil
}

// getThreadByID returns a thread by ID and platform.
func (s *Store) getThreadByID(ctx context.Context, id pgtype.UUID) (gen.ForumThread, error) {
	t, err := s.read.GetForumThreadByID(ctx, id)
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("getting forum thread by ID: %w", err)
	}

	return forumThreadFromByIDRow(t), nil
}

// getThreadBySlug returns a thread by slug and platform.
func (s *Store) getThreadBySlug(ctx context.Context, slug string) (gen.ForumThread, error) {
	t, err := s.read.GetForumThreadBySlug(ctx, slug)
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("getting forum thread by slug: %w", err)
	}

	return forumThreadFromBySlugRow(t), nil
}

// listThreadsBySpace returns paginated threads for a space.
func (s *Store) listThreadsBySpace(ctx context.Context, spaceID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsBySpace(ctx, gen.ListForumThreadsBySpaceParams{
		SpaceID:         spaceID,
		CursorCreatedAt: cursorCreatedAt,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum threads by space: %w", err)
	}

	threads := make([]gen.ForumThread, len(rows))
	for i, r := range rows {
		threads[i] = forumThreadFromListSpaceRow(r)
	}

	return threads, nil
}

// listThreadsByAuthor returns paginated threads for an author.
func (s *Store) listThreadsByAuthor(ctx context.Context, authorUserID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsByAuthor(ctx, gen.ListForumThreadsByAuthorParams{
		AuthorUserID:    authorUserID,
		CursorCreatedAt: cursorCreatedAt,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum threads by author: %w", err)
	}

	threads := make([]gen.ForumThread, len(rows))
	for i, r := range rows {
		threads[i] = forumThreadFromListAuthorRow(r)
	}

	return threads, nil
}

// listThreadsByPlatform returns all paginated threads for a platform (public listing).
func (s *Store) listThreadsByPlatform(ctx context.Context, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreads(ctx, gen.ListForumThreadsParams{
		CursorCreatedAt: cursorCreatedAt,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum threads by platform: %w", err)
	}

	threads := make([]gen.ForumThread, len(rows))
	for i, r := range rows {
		threads[i] = forumThreadFromListPlatformRow(r)
	}

	return threads, nil
}

// listThreadsByTag returns paginated threads for a tag.
func (s *Store) listThreadsByTag(ctx context.Context, tagID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsByTag(ctx, gen.ListForumThreadsByTagParams{
		TagID:           tagID,
		CursorCreatedAt: cursorCreatedAt,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum threads by tag: %w", err)
	}

	threads := make([]gen.ForumThread, len(rows))
	for i, r := range rows {
		threads[i] = forumThreadFromListTagRow(r)
	}

	return threads, nil
}

// updateThread updates a forum thread's title, body, and anonymous flag.
func (s *Store) updateThread(ctx context.Context, p gen.UpdateForumThreadParams) (gen.ForumThread, error) {
	t, err := s.write.UpdateForumThread(ctx, p)
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("updating forum thread: %w", err)
	}

	return forumThreadFromUpdateRow(t), nil
}

// softDeleteThread marks a thread as deleted.
func (s *Store) softDeleteThread(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.SoftDeleteForumThread(ctx, id); err != nil {
		return fmt.Errorf("soft-deleting forum thread: %w", err)
	}

	return nil
}

// solveThread marks a thread as solved and records the accepted reply.
func (s *Store) solveThread(ctx context.Context, id, solverUserID, parentReplyID pgtype.UUID) error {
	if err := s.write.SolveForumThread(ctx, gen.SolveForumThreadParams{
		ID: id,

		SolverUserID:  solverUserID,
		ParentReplyID: parentReplyID,
	}); err != nil {
		return fmt.Errorf("solving forum thread: %w", err)
	}

	return nil
}

// unsolveThread reverts the solved flag (DELETE /forum/{id}/solve).
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Solved flag).
func (s *Store) unsolveThread(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.UnsolveForumThread(ctx,
		id,
	); err != nil {
		return fmt.Errorf("unsolving forum thread: %w", err)
	}

	return nil
}

// isSpaceModerator reports whether the given user is a moderator of the
// given space. Used by SolveThread/UpdateThread/UpdateReply to enforce the
// "OP or moderator" rule from Section 08. Returns false when the space is
// unset (forum threads may have a null space_id).
func (s *Store) isSpaceModerator(ctx context.Context, userID, spaceID pgtype.UUID) bool {
	if !spaceID.Valid {
		return false
	}

	if _, err := s.read.GetSpaceModerator(ctx, gen.GetSpaceModeratorParams{
		SpaceID: spaceID,
		UserID:  userID,
	}); err != nil {
		return false
	}

	return true
}

// incrementReplyCount adjusts the reply_count on a thread.
// incrementThreadViewCount atomically increments the view_count on a thread by
// the requested delta. Called fire-and-forget from the GetThread handler and
// from the Valkey view-count flush worker.
func (s *Store) incrementThreadViewCount(ctx context.Context, id pgtype.UUID, delta int32) error {
	if err := s.write.IncrementForumThreadViewCount(ctx, gen.IncrementForumThreadViewCountParams{
		Delta: delta,
		ID:    id,
	}); err != nil {
		return fmt.Errorf("incrementing thread view count: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Replies
// ---------------------------------------------------------------------------

// createReply inserts a new forum reply.
// getReplyByID returns a reply by ID and platform.
func (s *Store) getReplyByID(ctx context.Context, id pgtype.UUID) (gen.ForumReply, error) {
	r, err := s.read.GetForumReplyByID(ctx, id)
	if err != nil {
		return gen.ForumReply{}, fmt.Errorf("getting forum reply by ID: %w", err)
	}

	return r, nil
}

// listRepliesByThread returns all replies for a thread.
func (s *Store) listRepliesByThread(ctx context.Context, threadID pgtype.UUID) ([]gen.ForumReply, error) {
	rows, err := s.read.ListForumRepliesByThread(ctx,
		threadID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing forum replies by thread: %w", err)
	}

	return rows, nil
}

// updateReply updates a reply's body.
func (s *Store) updateReply(ctx context.Context, p gen.UpdateForumReplyParams) (gen.ForumReply, error) {
	r, err := s.write.UpdateForumReply(ctx, p)
	if err != nil {
		return gen.ForumReply{}, fmt.Errorf("updating forum reply: %w", err)
	}

	return r, nil
}

// softDeleteReply marks a reply as deleted.
// ---------------------------------------------------------------------------
// Votes
// ---------------------------------------------------------------------------

// upsertVote inserts or updates a vote and returns the new row.
// deleteVote removes a user's vote on an entity (DELETE /forum/{id}/vote).
// getUserVotesForThread returns the current user's vote values for a thread
// and all of its replies.
func (s *Store) getUserVotesForThread(ctx context.Context, threadID, userID pgtype.UUID) ([]gen.ListUserVotesForThreadRow, error) {
	rows, err := s.read.ListUserVotesForThread(ctx, gen.ListUserVotesForThreadParams{
		UserID:   userID,
		ThreadID: threadID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing user votes for thread: %w", err)
	}

	if rows == nil {
		return []gen.ListUserVotesForThreadRow{}, nil
	}

	return rows, nil
}

// getVoteScore returns the aggregate vote score for an entity.
func (s *Store) getVoteScore(ctx context.Context, entityType string, entityID pgtype.UUID) (int32, error) {
	score, err := s.read.GetForumVoteScore(ctx, gen.GetForumVoteScoreParams{
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return 0, fmt.Errorf("getting forum vote score: %w", err)
	}

	return score, nil
}

// updateThreadVoteScore recalculates and writes the thread's vote_score.
func (s *Store) updateThreadVoteScore(ctx context.Context, entityID pgtype.UUID) error {
	if err := s.write.UpdateThreadVoteScore(ctx, entityID); err != nil {
		return fmt.Errorf("updating thread vote score: %w", err)
	}

	return nil
}

// updateReplyVoteScore recalculates and writes the reply's vote_score.
func (s *Store) updateReplyVoteScore(ctx context.Context, entityID pgtype.UUID) error {
	if err := s.write.UpdateReplyVoteScore(ctx, entityID); err != nil {
		return fmt.Errorf("updating reply vote score: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

func bookmarkFromAddForumBookmarkRow(r gen.AddForumBookmarkRow) gen.Bookmark {
	return gen.Bookmark{
		ID: r.ID,

		UserID:     r.UserID,
		EntityType: r.EntityType,
		EntityID:   r.EntityID,
		Folder:     r.Folder,
		CreatedAt:  r.CreatedAt,
		FolderID:   r.FolderID,
	}
}

func bookmarkFromGetForumBookmarkByIDRow(r gen.GetForumBookmarkByIDRow) gen.Bookmark {
	return gen.Bookmark{
		ID: r.ID,

		UserID:     r.UserID,
		EntityType: r.EntityType,
		EntityID:   r.EntityID,
		Folder:     r.Folder,
		CreatedAt:  r.CreatedAt,
		FolderID:   r.FolderID,
	}
}

func bookmarkFromGetForumBookmarkByEntityRow(r gen.GetForumBookmarkByEntityRow) gen.Bookmark {
	return gen.Bookmark{
		ID: r.ID,

		UserID:     r.UserID,
		EntityType: r.EntityType,
		EntityID:   r.EntityID,
		Folder:     r.Folder,
		CreatedAt:  r.CreatedAt,
		FolderID:   r.FolderID,
	}
}

func bookmarksFromListForumBookmarksByUserRow(rows []gen.ListForumBookmarksByUserRow) []gen.Bookmark {
	out := make([]gen.Bookmark, len(rows))
	for i, r := range rows {
		out[i] = gen.Bookmark{
			ID: r.ID,

			UserID:     r.UserID,
			EntityType: r.EntityType,
			EntityID:   r.EntityID,
			Folder:     r.Folder,
			CreatedAt:  r.CreatedAt,
			FolderID:   r.FolderID,
		}
	}

	return out
}

// addBookmark inserts a bookmark into the unified bookmarks table.
func (s *Store) addBookmark(ctx context.Context, p gen.AddForumBookmarkParams) (gen.Bookmark, error) {
	b, err := s.write.AddForumBookmark(ctx, p)
	if err != nil {
		return gen.Bookmark{}, fmt.Errorf("adding forum bookmark: %w", err)
	}

	return bookmarkFromAddForumBookmarkRow(b), nil
}

// removeBookmark deletes a bookmark from the unified bookmarks table.
func (s *Store) removeBookmark(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if err := s.write.RemoveForumBookmark(ctx, gen.RemoveForumBookmarkParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return fmt.Errorf("removing forum bookmark: %w", err)
	}

	return nil
}

// getBookmarkByID fetches a bookmark by its primary key (and owner).
// Used by DELETE /forum/bookmarks/{id} to resolve the entity before
// decrementing the denormalized bookmark_count.
func (s *Store) getBookmarkByID(ctx context.Context, id, userID pgtype.UUID) (gen.Bookmark, error) {
	bm, err := s.read.GetForumBookmarkByID(ctx, gen.GetForumBookmarkByIDParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		return gen.Bookmark{}, fmt.Errorf("getting forum bookmark by id: %w", err)
	}

	return bookmarkFromGetForumBookmarkByIDRow(bm), nil
}

// deleteBookmarkByID deletes a bookmark by its primary key (and owner).
func (s *Store) deleteBookmarkByID(ctx context.Context, id, userID pgtype.UUID) error {
	if err := s.write.DeleteForumBookmarkByID(ctx, gen.DeleteForumBookmarkByIDParams{
		ID:     id,
		UserID: userID,
	}); err != nil {
		return fmt.Errorf("deleting forum bookmark by id: %w", err)
	}

	return nil
}

// listBookmarksByUser returns paginated forum bookmarks for a user.
func (s *Store) listBookmarksByUser(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.Bookmark, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumBookmarksByUser(ctx, gen.ListForumBookmarksByUserParams{
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum bookmarks: %w", err)
	}

	return bookmarksFromListForumBookmarksByUserRow(rows), nil
}

// isBookmarked checks if an entity is bookmarked by a user.
func (s *Store) isBookmarked(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	result, err := s.read.IsForumBookmarked(ctx, gen.IsForumBookmarkedParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return false, fmt.Errorf("checking bookmark: %w", err)
	}

	return result, nil
}

// getBookmarkByEntity fetches a bookmark by (user_id, entity_type, entity_id).
// Returns the bookmark row so the frontend can call DELETE /forum/bookmarks/{id}
// without first listing all bookmarks.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Bookmarks).
func (s *Store) getBookmarkByEntity(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.Bookmark, error) {
	bm, err := s.read.GetForumBookmarkByEntity(ctx, gen.GetForumBookmarkByEntityParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return gen.Bookmark{}, fmt.Errorf("getting forum bookmark by entity: %w", err)
	}

	return bookmarkFromGetForumBookmarkByEntityRow(bm), nil
}

// incrementEntityBookmarkCount adjusts the bookmark_count on a thread.
func (s *Store) incrementEntityBookmarkCount(ctx context.Context, entityID pgtype.UUID, delta int32) error {
	if err := s.write.IncrementEntityBookmarkCount(ctx, gen.IncrementEntityBookmarkCountParams{
		EntityID: entityID,
		Delta:    delta,
	}); err != nil {
		return fmt.Errorf("incrementing entity bookmark count: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Tags
// Forum tags are universal — they live in the shared `tags`/`content_tags`
// tables (Sections 08, 14). The wire shape keeps the legacy ForumTag fields
// (cover_image_id maps to tags.cover_media_id) so existing consumers are
// unaffected.
// ---------------------------------------------------------------------------

// firstModerators returns the earliest-assigned tag_moderators.user_id per
// tag, projected into the legacy single-moderator ForumTag shape (§08/49).
// Failures are non-fatal: the field is cosmetic metadata, not authorization.
func (s *Store) firstModerators(ctx context.Context, tagIDs []pgtype.UUID) map[pgtype.UUID]pgtype.UUID {
	out := make(map[pgtype.UUID]pgtype.UUID, len(tagIDs))
	if len(tagIDs) == 0 {
		return out
	}

	rows, err := s.read.ListFirstTagModerators(ctx, tagIDs)
	if err != nil {
		return out
	}

	for _, r := range rows {
		out[r.TagID] = r.UserID
	}

	return out
}

// createTag inserts a new universal tag.
func (s *Store) createTag(ctx context.Context, p gen.CreateTagParams) (gen.ForumTag, error) {
	t, err := s.write.CreateTag(ctx, p)
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("creating tag: %w", err)
	}

	return gen.ForumTag{
		ID:              t.ID,
		Name:            t.Name,
		Slug:            t.Slug,
		Description:     t.Description,
		Rules:           t.Rules,
		CoverImageID:    t.CoverMediaID,
		FollowerCount:   t.FollowerCount,
		ModeratorUserID: s.firstModerators(ctx, []pgtype.UUID{t.ID})[t.ID],
		CreatedAt:       t.CreatedAt,
	}, nil
}

// getTagByID returns a universal tag by ID.
func (s *Store) getTagByID(ctx context.Context, id pgtype.UUID) (gen.ForumTag, error) {
	t, err := s.read.GetTagByID(ctx, id)
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("getting tag by ID: %w", err)
	}

	return gen.ForumTag{
		ID:              t.ID,
		Name:            t.Name,
		Slug:            t.Slug,
		Description:     t.Description,
		Rules:           t.Rules,
		CoverImageID:    t.CoverMediaID,
		FollowerCount:   t.FollowerCount,
		ModeratorUserID: s.firstModerators(ctx, []pgtype.UUID{t.ID})[t.ID],
		CreatedAt:       t.CreatedAt,
	}, nil
}

// getTagBySlug returns a universal tag by slug.
func (s *Store) getTagBySlug(ctx context.Context, slug string) (gen.ForumTag, error) {
	t, err := s.read.GetTagBySlug(ctx, slug)
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("getting tag by slug: %w", err)
	}

	return gen.ForumTag{
		ID:              t.ID,
		Name:            t.Name,
		Slug:            t.Slug,
		Description:     t.Description,
		Rules:           t.Rules,
		CoverImageID:    t.CoverMediaID,
		FollowerCount:   t.FollowerCount,
		ModeratorUserID: s.firstModerators(ctx, []pgtype.UUID{t.ID})[t.ID],
		CreatedAt:       t.CreatedAt,
	}, nil
}

// getUniversalTagByID returns the full universal tag row so updates can
// preserve fields the forum surface does not manage (color, is_system,
// field_values, logo_media_id).
func (s *Store) getUniversalTagByID(ctx context.Context, id pgtype.UUID) (gen.GetTagByIDRow, error) {
	return s.read.GetTagByID(ctx, id)
}

// listTags returns paginated universal tags ordered by recency.
func (s *Store) listTags(ctx context.Context, limit, offset int32) ([]gen.ForumTag, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListTagsPaginated(ctx, gen.ListTagsPaginatedParams{
		Q:          "",
		Sort:       "",
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}

	ids := make([]pgtype.UUID, 0, len(rows))
	for _, t := range rows {
		ids = append(ids, t.ID)
	}

	mods := s.firstModerators(ctx, ids)

	out := make([]gen.ForumTag, 0, len(rows))
	for _, t := range rows {
		out = append(out, gen.ForumTag{
			ID:              t.ID,
			Name:            t.Name,
			Slug:            t.Slug,
			Description:     t.Description,
			Rules:           t.Rules,
			CoverImageID:    t.CoverMediaID,
			FollowerCount:   t.FollowerCount,
			ModeratorUserID: mods[t.ID],
			CreatedAt:       t.CreatedAt,
		})
	}

	return out, nil
}

// listThreadTags returns the tags attached to a thread, joining forum_tags
// for details. Excludes soft-deleted tags.
// Ref: Section 08 (tags are first-class on discussions) — GET /forum/{id}/tags.
func (s *Store) listThreadTags(ctx context.Context, threadID pgtype.UUID) ([]gen.ListForumThreadTagsRow, error) {
	rows, err := s.read.ListForumThreadTags(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("listing forum thread tags: %w", err)
	}

	return rows, nil
}

// addThreadTag adds a tag to a thread.
func (s *Store) addThreadTag(ctx context.Context, threadID, tagID pgtype.UUID) error {
	if err := s.write.AddForumThreadTag(ctx, gen.AddForumThreadTagParams{
		ThreadID: threadID,
		TagID:    tagID,
	}); err != nil {
		return fmt.Errorf("adding forum thread tag: %w", err)
	}

	return nil
}

// countThreadTags returns the number of tags currently assigned to a thread.
// Used to enforce the "max 5 tags per post" rule. Ref: Section 08 Tags.
func (s *Store) countThreadTags(ctx context.Context, threadID pgtype.UUID) (int32, error) {
	count, err := s.read.CountForumThreadTags(ctx, threadID)
	if err != nil {
		return 0, fmt.Errorf("counting forum thread tags: %w", err)
	}

	return count, nil
}

// removeThreadTag removes a tag from a thread.
func (s *Store) removeThreadTag(ctx context.Context, threadID, tagID pgtype.UUID) error {
	if err := s.write.RemoveForumThreadTag(ctx, gen.RemoveForumThreadTagParams{
		ThreadID: threadID,
		TagID:    tagID,
	}); err != nil {
		return fmt.Errorf("removing forum thread tag: %w", err)
	}

	return nil
}

// updateTag updates a universal tag's mutable fields.
func (s *Store) updateTag(ctx context.Context, p gen.UpdateTagParams) (gen.ForumTag, error) {
	t, err := s.write.UpdateTag(ctx, p)
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("updating tag: %w", err)
	}

	return gen.ForumTag{
		ID:              t.ID,
		Name:            t.Name,
		Slug:            t.Slug,
		Description:     t.Description,
		Rules:           t.Rules,
		CoverImageID:    t.CoverMediaID,
		FollowerCount:   t.FollowerCount,
		ModeratorUserID: s.firstModerators(ctx, []pgtype.UUID{t.ID})[t.ID],
		CreatedAt:       t.CreatedAt,
	}, nil
}

// countContentByTag returns how many content rows reference a tag. Used by
// the delete guard: tags may only be deleted while nothing is tagged
// (Section 08 Admin Tags).
func (s *Store) countContentByTag(ctx context.Context, tagID pgtype.UUID) (int32, error) {
	count, err := s.read.CountContentByTag(ctx, tagID)
	if err != nil {
		return 0, fmt.Errorf("counting content by tag: %w", err)
	}

	return count, nil
}

// deleteTag removes a universal tag row. content_tags and tag_moderators
// rows cascade.
func (s *Store) deleteTag(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteTag(ctx, id); err != nil {
		return fmt.Errorf("deleting tag: %w", err)
	}

	return nil
}

// listTagModerators returns the moderators assigned to a universal tag.
func (s *Store) listTagModerators(ctx context.Context, tagID pgtype.UUID) ([]gen.ListTagModeratorsRow, error) {
	rows, err := s.read.ListTagModerators(ctx, tagID)
	if err != nil {
		return nil, fmt.Errorf("listing tag moderators: %w", err)
	}

	return rows, nil
}

// addTagModerator grants a user moderator rights on a universal tag.
func (s *Store) addTagModerator(ctx context.Context, tagID, userID, assignedByUserID pgtype.UUID) error {
	if _, err := s.write.AddTagModerator(ctx, gen.AddTagModeratorParams{
		TagID:            tagID,
		UserID:           userID,
		AssignedBy:       "site_admin",
		AssignedByUserID: assignedByUserID,
	}); err != nil {
		return fmt.Errorf("adding tag moderator: %w", err)
	}

	return nil
}

// removeTagModerator revokes a user's moderator rights on a universal tag.
func (s *Store) removeTagModerator(ctx context.Context, tagID, userID pgtype.UUID) error {
	if err := s.write.RemoveTagModerator(ctx, gen.RemoveTagModeratorParams{
		TagID:  tagID,
		UserID: userID,
	}); err != nil {
		return fmt.Errorf("removing tag moderator: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Polls
// ---------------------------------------------------------------------------

// createPoll inserts a new forum poll.
func (s *Store) createPoll(ctx context.Context, p gen.CreateForumPollParams) (gen.ForumPoll, error) {
	poll, err := s.write.CreateForumPoll(ctx, p)
	if err != nil {
		return gen.ForumPoll{}, fmt.Errorf("creating forum poll: %w", err)
	}

	return poll, nil
}

// addPollOption inserts a poll option.
func (s *Store) addPollOption(ctx context.Context, p gen.AddForumPollOptionParams) (gen.ForumPollOption, error) {
	opt, err := s.write.AddForumPollOption(ctx, p)
	if err != nil {
		return gen.ForumPollOption{}, fmt.Errorf("adding forum poll option: %w", err)
	}

	return opt, nil
}

// votePoll inserts or updates a single poll option vote.
func (s *Store) votePoll(ctx context.Context, p gen.VoteForumPollParams) (gen.ForumPollVote, error) {
	v, err := s.write.VoteForumPoll(ctx, p)
	if err != nil {
		return gen.ForumPollVote{}, fmt.Errorf("voting on forum poll: %w", err)
	}

	return v, nil
}

// deletePollVotesByUser removes all of a user's votes on a poll.
func (s *Store) deletePollVotesByUser(ctx context.Context, pollID, userID pgtype.UUID) error {
	if err := s.write.DeleteForumPollVotesByUser(ctx, gen.DeleteForumPollVotesByUserParams{
		PollID: pollID,
		UserID: userID,
	}); err != nil {
		return fmt.Errorf("deleting forum poll votes by user: %w", err)
	}

	return nil
}

// getPollByOptionID resolves the poll that owns a given option. Used by
// POST/DELETE /poll-options/{id}/vote where the URL param is the option_id.
// Ref: api-routes-catalog L612-613.
func (s *Store) getPollByOptionID(ctx context.Context, optionID pgtype.UUID) (gen.ForumPoll, error) {
	poll, err := s.read.GetForumPollByOptionID(ctx, optionID)
	if err != nil {
		return gen.ForumPoll{}, fmt.Errorf("getting forum poll by option id: %w", err)
	}

	return poll, nil
}

// retractPollVote removes a user's vote for a specific poll option
// (DELETE /poll-options/{id}/vote).
func (s *Store) retractPollVote(ctx context.Context, optionID, userID pgtype.UUID) error {
	if err := s.write.RetractForumPollVote(ctx, gen.RetractForumPollVoteParams{
		OptionID: optionID,
		UserID:   userID,
	}); err != nil {
		return fmt.Errorf("retracting forum poll vote: %w", err)
	}

	return nil
}

// getPollResults returns aggregated vote counts for a poll.
func (s *Store) getPollResults(ctx context.Context, pollID pgtype.UUID) ([]gen.GetForumPollResultsRow, error) {
	rows, err := s.read.GetForumPollResults(ctx, pollID)
	if err != nil {
		return nil, fmt.Errorf("getting forum poll results: %w", err)
	}

	return rows, nil
}

// getPollByID returns a poll by ID.
func (s *Store) getPollByID(ctx context.Context, id pgtype.UUID) (gen.ForumPoll, error) {
	poll, err := s.read.GetForumPollByID(ctx, id)
	if err != nil {
		return gen.ForumPoll{}, fmt.Errorf("getting forum poll by ID: %w", err)
	}

	return poll, nil
}

// getPollByThreadID returns a poll by thread ID.
func (s *Store) getPollByThreadID(ctx context.Context, threadID pgtype.UUID) (gen.ForumPoll, error) {
	poll, err := s.read.GetForumPollByThreadID(ctx, threadID)
	if err != nil {
		return gen.ForumPoll{}, fmt.Errorf("getting forum poll by thread ID: %w", err)
	}

	return poll, nil
}

// listPollUserVotes returns every option this user has voted on for a poll.
// Multi-select polls can produce multiple rows.
func (s *Store) listPollUserVotes(ctx context.Context, pollID, userID pgtype.UUID) ([]gen.ForumPollVote, error) {
	rows, err := s.read.ListForumPollUserVotes(ctx, gen.ListForumPollUserVotesParams{
		PollID: pollID,
		UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum poll user votes: %w", err)
	}

	if rows == nil {
		return []gen.ForumPollVote{}, nil
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Reply Views
// ---------------------------------------------------------------------------

// insertReplyView records a view for a reply (idempotent via PK).
func (s *Store) insertReplyView(ctx context.Context, replyID, userID pgtype.UUID) error {
	if err := s.write.InsertReplyView(ctx, gen.InsertReplyViewParams{
		ReplyID: replyID,
		UserID:  userID,
	}); err != nil {
		return fmt.Errorf("inserting reply view: %w", err)
	}

	return nil
}

// getReplyViewCount returns the view count for a reply.
func (s *Store) getReplyViewCount(ctx context.Context, replyID pgtype.UUID) (int32, error) {
	count, err := s.read.GetReplyViewCount(ctx, replyID)
	if err != nil {
		return 0, fmt.Errorf("getting reply view count: %w", err)
	}

	return count, nil
}

// ---------------------------------------------------------------------------
// Enriched queries — join author + space details for public endpoints.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
// These return the generated Row types directly; the interface{} author fields
// serialize correctly as JSON null (anonymous) or string (named author).
// ---------------------------------------------------------------------------

// getThreadBySlugWithDetails returns a thread plus joined author + space data.
func (s *Store) getThreadBySlugWithDetails(ctx context.Context, slug string) (gen.GetForumThreadBySlugWithDetailsRow, error) {
	row, err := s.read.GetForumThreadBySlugWithDetails(ctx,
		slug,
	)
	if err != nil {
		return gen.GetForumThreadBySlugWithDetailsRow{}, fmt.Errorf("getting forum thread by slug with details: %w", err)
	}

	return row, nil
}

// listThreadsByPlatformWithDetails returns paginated threads with author + space.
func (s *Store) listThreadsByPlatformWithDetails(ctx context.Context, viewerID pgtype.UUID, solved *bool, sortBy string, cursorCreatedAt pgtype.Timestamptz, cursorID pgtype.UUID, cursorVoteScore *int32, limit int32) ([]gen.ListForumThreadsWithDetailsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsWithDetails(ctx, gen.ListForumThreadsWithDetailsParams{
		ViewerID:        viewerID,
		Solved:          solved,
		SortBy:          sortBy,
		CursorVoteScore: cursorVoteScore,
		CursorCreatedAt: cursorCreatedAt,
		CursorID:        cursorID,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum threads by platform with details: %w", err)
	}

	return rows, nil
}

// listThreadsBySpaceWithDetails returns paginated threads for a space with author + space.
func (s *Store) listThreadsBySpaceWithDetails(ctx context.Context, spaceID pgtype.UUID, solved *bool, sortBy string, cursorCreatedAt pgtype.Timestamptz, cursorID pgtype.UUID, cursorVoteScore *int32, limit int32) ([]gen.ListForumThreadsBySpaceWithDetailsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsBySpaceWithDetails(ctx, gen.ListForumThreadsBySpaceWithDetailsParams{
		SpaceID:         spaceID,
		Solved:          solved,
		SortBy:          sortBy,
		CursorVoteScore: cursorVoteScore,
		CursorCreatedAt: cursorCreatedAt,
		CursorID:        cursorID,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum threads by space with details: %w", err)
	}

	return rows, nil
}

// listThreadsByAuthorWithDetails returns paginated threads for an author with author + space.
func (s *Store) listThreadsByAuthorWithDetails(ctx context.Context, viewerID, authorUserID pgtype.UUID, solved *bool, sortBy string, cursorCreatedAt pgtype.Timestamptz, cursorID pgtype.UUID, cursorVoteScore *int32, limit int32) ([]gen.ListForumThreadsByAuthorWithDetailsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsByAuthorWithDetails(ctx, gen.ListForumThreadsByAuthorWithDetailsParams{
		ViewerID:        viewerID,
		AuthorUserID:    authorUserID,
		Solved:          solved,
		SortBy:          sortBy,
		CursorVoteScore: cursorVoteScore,
		CursorCreatedAt: cursorCreatedAt,
		CursorID:        cursorID,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum threads by author with details: %w", err)
	}

	return rows, nil
}

// listThreadsByTagWithDetails returns paginated threads for a tag with author + space.
func (s *Store) listThreadsByTagWithDetails(ctx context.Context, viewerID, tagID pgtype.UUID, solved *bool, sortBy string, cursorCreatedAt pgtype.Timestamptz, cursorID pgtype.UUID, cursorVoteScore *int32, limit int32) ([]gen.ListForumThreadsByTagWithDetailsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsByTagWithDetails(ctx, gen.ListForumThreadsByTagWithDetailsParams{
		ViewerID:        viewerID,
		TagID:           tagID,
		Solved:          solved,
		SortBy:          sortBy,
		CursorVoteScore: cursorVoteScore,
		CursorCreatedAt: cursorCreatedAt,
		CursorID:        cursorID,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum threads by tag with details: %w", err)
	}

	return rows, nil
}

// listRepliesByThreadWithAuthor returns all replies for a thread with author data.
func (s *Store) listRepliesByThreadWithAuthor(ctx context.Context, threadID pgtype.UUID) ([]gen.ListForumRepliesByThreadWithAuthorRow, error) {
	rows, err := s.read.ListForumRepliesByThreadWithAuthor(ctx,
		threadID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing forum replies by thread with author: %w", err)
	}

	return rows, nil
}

// getThreadSpaceIDByID returns the space_id for a thread by ID (no platform
// scope — used for WebSocket broadcasts where platform is not available).
func (s *Store) getThreadSpaceIDByID(ctx context.Context, id pgtype.UUID) (pgtype.UUID, error) {
	spaceID, err := s.read.GetForumThreadSpaceIDByID(ctx, id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("getting thread space_id by ID: %w", err)
	}

	return spaceID, nil
}

// getThreadSpaceByID returns the space for a thread by ID without requiring a
// platform filter. Used for WebSocket broadcasts.
func (s *Store) getThreadSpaceByID(ctx context.Context, id pgtype.UUID) (pgtype.UUID, error) {
	spaceID, err := s.read.GetForumThreadAndSpaceByID(ctx, id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("getting thread space by ID: %w", err)
	}

	return spaceID, nil
}

// ---------------------------------------------------------------------------
// Votes — delete by entity (for vote retraction)
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
// ---------------------------------------------------------------------------

// deleteVoteByEntity deletes a user's vote on a specific entity.
func (s *Store) deleteVoteByEntity(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if err := s.write.DeleteForumVoteByEntity(ctx, gen.DeleteForumVoteByEntityParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return fmt.Errorf("deleting forum vote by entity: %w", err)
	}

	return nil
}
