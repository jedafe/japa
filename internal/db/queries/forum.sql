-- Forum Q&A / Solved — threads, replies, votes, polls, tags, bookmarks, reply views.

-- =====================
-- Threads
-- =====================

-- name: CreateForumThread :one
INSERT INTO forum_threads (
    tenant_id, content_id, space_id, author_user_id, parent_reply_id,
    title, slug, body, is_anonymous, edit_window_minutes
) VALUES (
    @tenant_id, sqlc.narg(content_id), sqlc.narg(space_id), @author_user_id,
    sqlc.narg(parent_reply_id), @title, @slug, @body, @is_anonymous,
    @edit_window_minutes
)
RETURNING id, tenant_id, content_id, space_id, author_user_id, parent_reply_id,
          title, slug, body, is_anonymous, solved, solved_at, solved_by_user_id,
          vote_score, reply_count, view_count, bookmark_count, edit_window_minutes,
          is_edited, created_at, updated_at, deleted_at;

-- name: GetForumThreadByID :one
SELECT id, tenant_id, content_id, space_id, author_user_id, parent_reply_id,
       title, slug, body, is_anonymous, solved, solved_at, solved_by_user_id,
       vote_score, reply_count, view_count, bookmark_count, edit_window_minutes,
       is_edited, created_at, updated_at, deleted_at
FROM forum_threads
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: GetForumThreadBySlug :one
SELECT id, tenant_id, content_id, space_id, author_user_id, parent_reply_id,
       title, slug, body, is_anonymous, solved, solved_at, solved_by_user_id,
       vote_score, reply_count, view_count, bookmark_count, edit_window_minutes,
       is_edited, created_at, updated_at, deleted_at
FROM forum_threads
WHERE slug = @slug AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ListForumThreadsBySpace :many
SELECT id, tenant_id, content_id, space_id, author_user_id, parent_reply_id,
       title, slug, body, is_anonymous, solved, solved_at, solved_by_user_id,
       vote_score, reply_count, view_count, bookmark_count, edit_window_minutes,
       is_edited, created_at, updated_at, deleted_at
FROM forum_threads
WHERE tenant_id = @tenant_id
  AND space_id = @space_id
  AND (created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: ListForumThreadsByAuthor :many
SELECT id, tenant_id, content_id, space_id, author_user_id, parent_reply_id,
       title, slug, body, is_anonymous, solved, solved_at, solved_by_user_id,
       vote_score, reply_count, view_count, bookmark_count, edit_window_minutes,
       is_edited, created_at, updated_at, deleted_at
FROM forum_threads
WHERE tenant_id = @tenant_id
  AND author_user_id = @author_user_id
  AND (created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: ListForumThreadsByTenant :many
SELECT id, tenant_id, content_id, space_id, author_user_id, parent_reply_id,
       title, slug, body, is_anonymous, solved, solved_at, solved_by_user_id,
       vote_score, reply_count, view_count, bookmark_count, edit_window_minutes,
       is_edited, created_at, updated_at, deleted_at
FROM forum_threads
WHERE tenant_id = @tenant_id
  AND deleted_at IS NULL
  AND (created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: ListForumThreadsByTag :many
SELECT ft.id, ft.tenant_id, ft.content_id, ft.space_id, ft.author_user_id,
       ft.parent_reply_id, ft.title, ft.slug, ft.body, ft.is_anonymous,
       ft.solved, ft.solved_at, ft.solved_by_user_id, ft.vote_score,
       ft.reply_count, ft.view_count, ft.bookmark_count, ft.edit_window_minutes,
       ft.is_edited, ft.created_at, ft.updated_at, ft.deleted_at
FROM forum_threads ft
JOIN forum_thread_tags ftt ON ftt.thread_id = ft.id
WHERE ft.tenant_id = @tenant_id
  AND ftt.tag_id = @tag_id
  AND ft.deleted_at IS NULL
  AND (ft.created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY ft.created_at DESC, ft.id DESC
LIMIT @page_limit;

-- name: UpdateForumThread :one
UPDATE forum_threads
SET title = @title, body = @body, is_anonymous = @is_anonymous,
    is_edited = true, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, content_id, space_id, author_user_id, parent_reply_id,
          title, slug, body, is_anonymous, solved, solved_at, solved_by_user_id,
          vote_score, reply_count, view_count, bookmark_count, edit_window_minutes,
          is_edited, created_at, updated_at, deleted_at;

-- name: SoftDeleteForumThread :exec
UPDATE forum_threads
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- name: SolveForumThread :exec
UPDATE forum_threads
SET solved = true, solved_at = now(), solved_by_user_id = @solver_user_id, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- name: IncrementThreadReplyCount :exec
UPDATE forum_threads
SET reply_count = reply_count + @delta
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Replies
-- =====================

-- name: CreateForumReply :one
INSERT INTO forum_replies (
    tenant_id, thread_id, parent_reply_id, author_user_id,
    body, is_anonymous
) VALUES (
    @tenant_id, @thread_id, sqlc.narg(parent_reply_id), @author_user_id,
    @body, @is_anonymous
)
RETURNING id, tenant_id, thread_id, parent_reply_id, author_user_id,
          body, vote_score, view_count, is_anonymous, is_edited,
          created_at, updated_at, deleted_at;

-- name: GetForumReplyByID :one
SELECT id, tenant_id, thread_id, parent_reply_id, author_user_id,
       body, vote_score, view_count, is_anonymous, is_edited,
       created_at, updated_at, deleted_at
FROM forum_replies
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ListForumRepliesByThread :many
SELECT id, tenant_id, thread_id, parent_reply_id, author_user_id,
       body, vote_score, view_count, is_anonymous, is_edited,
       created_at, updated_at, deleted_at
FROM forum_replies
WHERE thread_id = @thread_id
  AND tenant_id = @tenant_id
  AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: UpdateForumReply :one
UPDATE forum_replies
SET body = @body, is_edited = true, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, thread_id, parent_reply_id, author_user_id,
          body, vote_score, view_count, is_anonymous, is_edited,
          created_at, updated_at, deleted_at;

-- name: SoftDeleteForumReply :exec
UPDATE forum_replies
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Votes
-- =====================

-- name: UpsertForumVote :one
INSERT INTO forum_votes (tenant_id, user_id, entity_type, entity_id, value)
VALUES (@tenant_id, @user_id, @entity_type, @entity_id, @value)
ON CONFLICT (user_id, entity_type, entity_id) DO UPDATE
    SET value = @value
RETURNING id, tenant_id, user_id, entity_type, entity_id, value, created_at;

-- name: GetForumVote :one
SELECT id, tenant_id, user_id, entity_type, entity_id, value, created_at
FROM forum_votes
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: GetForumVoteScore :one
SELECT COALESCE(SUM(value), 0)::int AS score
FROM forum_votes
WHERE entity_type = @entity_type AND entity_id = @entity_id;

-- name: DeleteForumVote :exec
DELETE FROM forum_votes
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: UpdateThreadVoteScore :exec
UPDATE forum_threads
SET vote_score = (SELECT COALESCE(SUM(value), 0)::int FROM forum_votes WHERE entity_type = 'thread' AND entity_id = @entity_id)
WHERE id = @entity_id;

-- name: UpdateReplyVoteScore :exec
UPDATE forum_replies
SET vote_score = (SELECT COALESCE(SUM(value), 0)::int FROM forum_votes WHERE entity_type = 'reply' AND entity_id = @entity_id)
WHERE id = @entity_id;

-- =====================
-- Polls
-- =====================

-- name: CreateForumPoll :one
INSERT INTO forum_polls (thread_id, question, is_multi_select, close_at)
VALUES (@thread_id, @question, @is_multi_select, sqlc.narg(close_at))
RETURNING id, thread_id, question, is_multi_select, close_at, created_at;

-- name: AddForumPollOption :one
INSERT INTO forum_poll_options (poll_id, label, position)
VALUES (@poll_id, @label, @position)
RETURNING id, poll_id, label, position;

-- name: VoteForumPoll :one
INSERT INTO forum_poll_votes (poll_id, option_id, user_id)
VALUES (@poll_id, @option_id, @user_id)
ON CONFLICT (poll_id, user_id) DO UPDATE
    SET option_id = @option_id
RETURNING id, poll_id, option_id, user_id, created_at;

-- name: GetForumPollResults :many
SELECT
    fpo.id AS option_id,
    fpo.label,
    fpo.position,
    COUNT(fpv.id)::int AS vote_count
FROM forum_poll_options fpo
LEFT JOIN forum_poll_votes fpv ON fpv.option_id = fpo.id
WHERE fpo.poll_id = @poll_id
GROUP BY fpo.id, fpo.label, fpo.position
ORDER BY fpo.position ASC;

-- name: GetForumPollByID :one
SELECT id, thread_id, question, is_multi_select, close_at, created_at
FROM forum_polls
WHERE id = @id;

-- name: GetForumPollByThreadID :one
SELECT id, thread_id, question, is_multi_select, close_at, created_at
FROM forum_polls
WHERE thread_id = @thread_id;

-- name: GetForumPollUserVote :one
SELECT fpv.id, fpv.poll_id, fpv.option_id, fpv.user_id, fpv.created_at
FROM forum_poll_votes fpv
WHERE fpv.poll_id = @poll_id AND fpv.user_id = @user_id;

-- =====================
-- Tags
-- =====================

-- name: CreateForumTag :one
INSERT INTO forum_tags (tenant_id, name, slug, description, rules, cover_image_id, moderator_user_id)
VALUES (@tenant_id, @name, @slug, sqlc.narg(description), sqlc.narg(rules),
        sqlc.narg(cover_image_id), sqlc.narg(moderator_user_id))
RETURNING id, tenant_id, name, slug, description, rules, cover_image_id,
          follower_count, moderator_user_id, created_at;

-- name: GetForumTagByID :one
SELECT id, tenant_id, name, slug, description, rules, cover_image_id,
       follower_count, moderator_user_id, created_at
FROM forum_tags
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetForumTagBySlug :one
SELECT id, tenant_id, name, slug, description, rules, cover_image_id,
       follower_count, moderator_user_id, created_at
FROM forum_tags
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: ListForumTags :many
SELECT id, tenant_id, name, slug, description, rules, cover_image_id,
       follower_count, moderator_user_id, created_at
FROM forum_tags
WHERE tenant_id = @tenant_id
ORDER BY follower_count DESC, name ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: AddForumThreadTag :exec
INSERT INTO forum_thread_tags (thread_id, tag_id)
VALUES (@thread_id, @tag_id)
ON CONFLICT DO NOTHING;

-- name: RemoveForumThreadTag :exec
DELETE FROM forum_thread_tags
WHERE thread_id = @thread_id AND tag_id = @tag_id;

-- name: UpdateForumTagFollowerCount :exec
UPDATE forum_tags
SET follower_count = GREATEST(0, follower_count + @delta)
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateForumTag :one
UPDATE forum_tags
SET name = @name,
    slug = @slug,
    description = @description,
    rules = @rules,
    cover_image_id = @cover_image_id,
    moderator_user_id = @moderator_user_id
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, name, slug, description, rules, cover_image_id,
          follower_count, moderator_user_id, created_at;

-- name: SoftDeleteForumTag :exec
UPDATE forum_tags SET deleted_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: IncrementForumThreadViewCount :exec
UPDATE forum_threads SET view_count = view_count + 1
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- =====================
-- Bookmarks
-- =====================

-- name: AddForumBookmark :one
INSERT INTO forum_bookmarks (tenant_id, user_id, entity_type, entity_id, folder)
VALUES (@tenant_id, @user_id, @entity_type, @entity_id, @folder)
ON CONFLICT (user_id, entity_type, entity_id) DO UPDATE
    SET folder = @folder
RETURNING id, tenant_id, user_id, entity_type, entity_id, folder, created_at;

-- name: RemoveForumBookmark :exec
DELETE FROM forum_bookmarks
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: ListForumBookmarksByUser :many
SELECT id, tenant_id, user_id, entity_type, entity_id, folder, created_at
FROM forum_bookmarks
WHERE user_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: IsForumBookmarked :one
SELECT EXISTS(
    SELECT 1 FROM forum_bookmarks
    WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id
) AS is_bookmarked;

-- name: IncrementEntityBookmarkCount :exec
UPDATE forum_threads
SET bookmark_count = bookmark_count + @delta
WHERE id = @entity_id;

-- =====================
-- Reply Views
-- =====================

-- name: InsertReplyView :exec
INSERT INTO reply_views (reply_id, user_id)
VALUES (@reply_id, @user_id)
ON CONFLICT (reply_id, user_id) DO NOTHING;

-- name: GetReplyViewCount :one
SELECT view_count FROM forum_replies WHERE id = @reply_id;
