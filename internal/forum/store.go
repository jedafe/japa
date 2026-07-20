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
		ID:                r.ID,
		TenantID:          r.TenantID,
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
		ID:                r.ID,
		TenantID:          r.TenantID,
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
		ID:                r.ID,
		TenantID:          r.TenantID,
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
		ID:                r.ID,
		TenantID:          r.TenantID,
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
		ID:                r.ID,
		TenantID:          r.TenantID,
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
		ID:                r.ID,
		TenantID:          r.TenantID,
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

func forumThreadFromListTenantRow(r gen.ListForumThreadsByTenantRow) gen.ForumThread {
	return gen.ForumThread{
		ID:                r.ID,
		TenantID:          r.TenantID,
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
		ID:                r.ID,
		TenantID:          r.TenantID,
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

// getThreadByID returns a thread by ID and tenant.
func (s *Store) getThreadByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ForumThread, error) {
	t, err := s.read.GetForumThreadByID(ctx, gen.GetForumThreadByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("getting forum thread by ID: %w", err)
	}

	return forumThreadFromByIDRow(t), nil
}

// getThreadBySlug returns a thread by slug and tenant.
func (s *Store) getThreadBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.ForumThread, error) {
	t, err := s.read.GetForumThreadBySlug(ctx, gen.GetForumThreadBySlugParams{Slug: slug, TenantID: tenantID})
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("getting forum thread by slug: %w", err)
	}

	return forumThreadFromBySlugRow(t), nil
}

// listThreadsBySpace returns paginated threads for a space.
func (s *Store) listThreadsBySpace(ctx context.Context, tenantID, spaceID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsBySpace(ctx, gen.ListForumThreadsBySpaceParams{
		TenantID:        tenantID,
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
func (s *Store) listThreadsByAuthor(ctx context.Context, tenantID, authorUserID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsByAuthor(ctx, gen.ListForumThreadsByAuthorParams{
		TenantID:        tenantID,
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

// listThreadsByTenant returns all paginated threads for a tenant (public listing).
func (s *Store) listThreadsByTenant(ctx context.Context, tenantID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsByTenant(ctx, gen.ListForumThreadsByTenantParams{
		TenantID:        tenantID,
		CursorCreatedAt: cursorCreatedAt,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum threads by tenant: %w", err)
	}

	threads := make([]gen.ForumThread, len(rows))
	for i, r := range rows {
		threads[i] = forumThreadFromListTenantRow(r)
	}

	return threads, nil
}

// listThreadsByTag returns paginated threads for a tag.
func (s *Store) listThreadsByTag(ctx context.Context, tenantID, tagID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumThreadsByTag(ctx, gen.ListForumThreadsByTagParams{
		TenantID:        tenantID,
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
func (s *Store) softDeleteThread(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.SoftDeleteForumThread(ctx, gen.SoftDeleteForumThreadParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("soft-deleting forum thread: %w", err)
	}

	return nil
}

// solveThread marks a thread as solved.
func (s *Store) solveThread(ctx context.Context, id, tenantID, solverUserID pgtype.UUID) error {
	if err := s.write.SolveForumThread(ctx, gen.SolveForumThreadParams{
		ID:           id,
		TenantID:     tenantID,
		SolverUserID: solverUserID,
	}); err != nil {
		return fmt.Errorf("solving forum thread: %w", err)
	}

	return nil
}

// incrementReplyCount adjusts the reply_count on a thread.
func (s *Store) incrementReplyCount(ctx context.Context, id, tenantID pgtype.UUID, delta int32) error {
	if err := s.write.IncrementThreadReplyCount(ctx, gen.IncrementThreadReplyCountParams{
		ID:       id,
		TenantID: tenantID,
		Delta:    delta,
	}); err != nil {
		return fmt.Errorf("incrementing thread reply count: %w", err)
	}

	return nil
}

// incrementThreadViewCount atomically increments the view_count on a thread.
// Called fire-and-forget from the GetThread handler.
func (s *Store) incrementThreadViewCount(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.IncrementForumThreadViewCount(ctx, gen.IncrementForumThreadViewCountParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("incrementing thread view count: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Replies
// ---------------------------------------------------------------------------

// createReply inserts a new forum reply.
func (s *Store) createReply(ctx context.Context, p gen.CreateForumReplyParams) (gen.ForumReply, error) {
	r, err := s.write.CreateForumReply(ctx, p)
	if err != nil {
		return gen.ForumReply{}, fmt.Errorf("creating forum reply: %w", err)
	}

	return r, nil
}

// getReplyByID returns a reply by ID and tenant.
func (s *Store) getReplyByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ForumReply, error) {
	r, err := s.read.GetForumReplyByID(ctx, gen.GetForumReplyByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.ForumReply{}, fmt.Errorf("getting forum reply by ID: %w", err)
	}

	return r, nil
}

// listRepliesByThread returns all replies for a thread.
func (s *Store) listRepliesByThread(ctx context.Context, threadID, tenantID pgtype.UUID) ([]gen.ForumReply, error) {
	rows, err := s.read.ListForumRepliesByThread(ctx, gen.ListForumRepliesByThreadParams{
		ThreadID: threadID,
		TenantID: tenantID,
	})
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
func (s *Store) softDeleteReply(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.SoftDeleteForumReply(ctx, gen.SoftDeleteForumReplyParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("soft-deleting forum reply: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Votes
// ---------------------------------------------------------------------------

// upsertVote inserts or updates a vote and returns the new row.
func (s *Store) upsertVote(ctx context.Context, p gen.UpsertForumVoteParams) (gen.ForumVote, error) {
	v, err := s.write.UpsertForumVote(ctx, p)
	if err != nil {
		return gen.ForumVote{}, fmt.Errorf("upserting forum vote: %w", err)
	}

	return v, nil
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

// addBookmark inserts a bookmark.
func (s *Store) addBookmark(ctx context.Context, p gen.AddForumBookmarkParams) (gen.ForumBookmark, error) {
	b, err := s.write.AddForumBookmark(ctx, p)
	if err != nil {
		return gen.ForumBookmark{}, fmt.Errorf("adding forum bookmark: %w", err)
	}

	return b, nil
}

// removeBookmark deletes a bookmark.
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

// listBookmarksByUser returns paginated bookmarks for a user.
func (s *Store) listBookmarksByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.ForumBookmark, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumBookmarksByUser(ctx, gen.ListForumBookmarksByUserParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum bookmarks: %w", err)
	}

	return rows, nil
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
// ---------------------------------------------------------------------------

// createTag inserts a new forum tag.
func (s *Store) createTag(ctx context.Context, p gen.CreateForumTagParams) (gen.ForumTag, error) {
	t, err := s.write.CreateForumTag(ctx, p)
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("creating forum tag: %w", err)
	}

	return t, nil
}

// getTagByID returns a tag by ID and tenant.
func (s *Store) getTagByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ForumTag, error) {
	t, err := s.read.GetForumTagByID(ctx, gen.GetForumTagByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("getting forum tag by ID: %w", err)
	}

	return t, nil
}

// getTagBySlug returns a tag by slug and tenant.
func (s *Store) getTagBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.ForumTag, error) {
	t, err := s.read.GetForumTagBySlug(ctx, gen.GetForumTagBySlugParams{Slug: slug, TenantID: tenantID})
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("getting forum tag by slug: %w", err)
	}

	return t, nil
}

// listTags returns paginated tags.
func (s *Store) listTags(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ForumTag, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListForumTags(ctx, gen.ListForumTagsParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forum tags: %w", err)
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

// updateTagFollowerCount adjusts a tag's follower_count.
func (s *Store) updateTagFollowerCount(ctx context.Context, id, tenantID pgtype.UUID, delta int32) error {
	if err := s.write.UpdateForumTagFollowerCount(ctx, gen.UpdateForumTagFollowerCountParams{
		ID:       id,
		TenantID: tenantID,
		Delta:    delta,
	}); err != nil {
		return fmt.Errorf("updating forum tag follower count: %w", err)
	}

	return nil
}

// updateTag updates a forum tag's mutable fields.
func (s *Store) updateTag(ctx context.Context, id, tenantID pgtype.UUID, name, slug, description, rules string, coverImageID, moderatorUserID pgtype.UUID) (gen.ForumTag, error) {
	tag, err := s.write.UpdateForumTag(ctx, gen.UpdateForumTagParams{
		ID:              id,
		TenantID:        tenantID,
		Name:            name,
		Slug:            slug,
		Description:     strPtr(description),
		Rules:           strPtr(rules),
		CoverImageID:    coverImageID,
		ModeratorUserID: moderatorUserID,
	})
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("updating forum tag: %w", err)
	}
	return tag, nil
}

// softDeleteTag marks a forum tag as deleted.
func (s *Store) softDeleteTag(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.SoftDeleteForumTag(ctx, gen.SoftDeleteForumTagParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("soft-deleting forum tag: %w", err)
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

// votePoll inserts or updates a user's poll vote.
func (s *Store) votePoll(ctx context.Context, p gen.VoteForumPollParams) (gen.ForumPollVote, error) {
	v, err := s.write.VoteForumPoll(ctx, p)
	if err != nil {
		return gen.ForumPollVote{}, fmt.Errorf("voting on forum poll: %w", err)
	}

	return v, nil
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

// getPollUserVote returns a user's vote on a poll.
func (s *Store) getPollUserVote(ctx context.Context, pollID, userID pgtype.UUID) (gen.ForumPollVote, error) {
	v, err := s.read.GetForumPollUserVote(ctx, gen.GetForumPollUserVoteParams{
		PollID: pollID,
		UserID: userID,
	})
	if err != nil {
		return gen.ForumPollVote{}, fmt.Errorf("getting forum poll user vote: %w", err)
	}

	return v, nil
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
