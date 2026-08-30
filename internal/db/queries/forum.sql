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

-- =====================
-- Threads with joined author + space details.
-- These variants power the public list/detail endpoints so the frontend
-- gets author username/display_name/avatar and space name/slug in one
-- response. When is_anonymous = true, author fields are nulled out so
-- the frontend never receives identifying data for anonymous posts.
-- =====================

-- name: GetForumThreadBySlugWithDetails :one
SELECT ft.id, ft.tenant_id, ft.content_id, ft.space_id, ft.author_user_id,
       ft.parent_reply_id, ft.title, ft.slug, ft.body, ft.is_anonymous,
       ft.solved, ft.solved_at, ft.solved_by_user_id, ft.vote_score,
       ft.reply_count, ft.view_count, ft.bookmark_count, ft.edit_window_minutes,
       ft.is_edited, ft.created_at, ft.updated_at, ft.deleted_at,
       CASE WHEN ft.is_anonymous THEN NULL ELSE u.username END AS author_username,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.display_name END AS author_display_name,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.avatar_url END AS author_avatar_url,
       CASE WHEN ft.solved_by_user_id IS NULL THEN NULL ELSE su.username END AS solved_by_username,
       CASE WHEN ft.solved_by_user_id IS NULL THEN NULL ELSE sup.display_name END AS solved_by_display_name,
       s.name AS space_name,
       s.slug AS space_slug,
       s.level AS space_level
FROM forum_threads ft
LEFT JOIN users u ON u.id = ft.author_user_id
LEFT JOIN user_profiles up ON up.user_id = ft.author_user_id
LEFT JOIN users su ON su.id = ft.solved_by_user_id
LEFT JOIN user_profiles sup ON sup.user_id = ft.solved_by_user_id
LEFT JOIN spaces s ON s.id = ft.space_id
WHERE ft.slug = @slug AND ft.tenant_id = @tenant_id AND ft.deleted_at IS NULL;

-- name: ListForumThreadsByTenantWithDetails :many
SELECT ft.id, ft.tenant_id, ft.content_id, ft.space_id, ft.author_user_id,
       ft.parent_reply_id, ft.title, ft.slug, ft.body, ft.is_anonymous,
       ft.solved, ft.solved_at, ft.solved_by_user_id, ft.vote_score,
       ft.reply_count, ft.view_count, ft.bookmark_count, ft.edit_window_minutes,
       ft.is_edited, ft.created_at, ft.updated_at, ft.deleted_at,
       CASE WHEN ft.is_anonymous THEN NULL ELSE u.username END AS author_username,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.display_name END AS author_display_name,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.avatar_url END AS author_avatar_url,
       s.name AS space_name,
       s.slug AS space_slug,
       s.level AS space_level
FROM forum_threads ft
LEFT JOIN users u ON u.id = ft.author_user_id
LEFT JOIN user_profiles up ON up.user_id = ft.author_user_id
LEFT JOIN spaces s ON s.id = ft.space_id
WHERE ft.tenant_id = @tenant_id
  AND ft.deleted_at IS NULL
  AND ft.solved = COALESCE(sqlc.narg(solved)::bool, ft.solved)
  AND (
    (@sort_by = 'top' AND (
      sqlc.narg(cursor_vote_score)::int IS NULL
      OR ft.vote_score < sqlc.narg(cursor_vote_score)::int
      OR (ft.vote_score = sqlc.narg(cursor_vote_score)::int AND (
        ft.created_at < @cursor_created_at
        OR (ft.created_at = @cursor_created_at AND ft.id < sqlc.narg(cursor_id)::uuid)
        OR @cursor_created_at IS NULL
      ))
    ))
    OR
    (@sort_by != 'top' AND (
      @cursor_created_at IS NULL
      OR ft.created_at < @cursor_created_at
      OR (ft.created_at = @cursor_created_at AND ft.id < sqlc.narg(cursor_id)::uuid)
    ))
  )
ORDER BY CASE WHEN @sort_by = 'top' THEN ft.vote_score END DESC,
         ft.created_at DESC, ft.id DESC
LIMIT @page_limit;

-- name: ListForumThreadsBySpaceWithDetails :many
SELECT ft.id, ft.tenant_id, ft.content_id, ft.space_id, ft.author_user_id,
       ft.parent_reply_id, ft.title, ft.slug, ft.body, ft.is_anonymous,
       ft.solved, ft.solved_at, ft.solved_by_user_id, ft.vote_score,
       ft.reply_count, ft.view_count, ft.bookmark_count, ft.edit_window_minutes,
       ft.is_edited, ft.created_at, ft.updated_at, ft.deleted_at,
       CASE WHEN ft.is_anonymous THEN NULL ELSE u.username END AS author_username,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.display_name END AS author_display_name,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.avatar_url END AS author_avatar_url,
       s.name AS space_name,
       s.slug AS space_slug,
       s.level AS space_level
FROM forum_threads ft
LEFT JOIN users u ON u.id = ft.author_user_id
LEFT JOIN user_profiles up ON up.user_id = ft.author_user_id
LEFT JOIN spaces s ON s.id = ft.space_id
WHERE ft.tenant_id = @tenant_id
  AND ft.space_id = @space_id
  AND ft.deleted_at IS NULL
  AND ft.solved = COALESCE(sqlc.narg(solved)::bool, ft.solved)
  AND (
    (@sort_by = 'top' AND (
      sqlc.narg(cursor_vote_score)::int IS NULL
      OR ft.vote_score < sqlc.narg(cursor_vote_score)::int
      OR (ft.vote_score = sqlc.narg(cursor_vote_score)::int AND (
        ft.created_at < @cursor_created_at
        OR (ft.created_at = @cursor_created_at AND ft.id < sqlc.narg(cursor_id)::uuid)
        OR @cursor_created_at IS NULL
      ))
    ))
    OR
    (@sort_by != 'top' AND (
      @cursor_created_at IS NULL
      OR ft.created_at < @cursor_created_at
      OR (ft.created_at = @cursor_created_at AND ft.id < sqlc.narg(cursor_id)::uuid)
    ))
  )
ORDER BY CASE WHEN @sort_by = 'top' THEN ft.vote_score END DESC,
         ft.created_at DESC, ft.id DESC
LIMIT @page_limit;

-- name: ListForumThreadsByAuthorWithDetails :many
SELECT ft.id, ft.tenant_id, ft.content_id, ft.space_id, ft.author_user_id,
       ft.parent_reply_id, ft.title, ft.slug, ft.body, ft.is_anonymous,
       ft.solved, ft.solved_at, ft.solved_by_user_id, ft.vote_score,
       ft.reply_count, ft.view_count, ft.bookmark_count, ft.edit_window_minutes,
       ft.is_edited, ft.created_at, ft.updated_at, ft.deleted_at,
       CASE WHEN ft.is_anonymous THEN NULL ELSE u.username END AS author_username,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.display_name END AS author_display_name,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.avatar_url END AS author_avatar_url,
       s.name AS space_name,
       s.slug AS space_slug,
       s.level AS space_level
FROM forum_threads ft
LEFT JOIN users u ON u.id = ft.author_user_id
LEFT JOIN user_profiles up ON up.user_id = ft.author_user_id
LEFT JOIN spaces s ON s.id = ft.space_id
WHERE ft.tenant_id = @tenant_id
  AND ft.author_user_id = @author_user_id
  AND ft.deleted_at IS NULL
  AND ft.solved = COALESCE(sqlc.narg(solved)::bool, ft.solved)
  AND (
    (@sort_by = 'top' AND (
      sqlc.narg(cursor_vote_score)::int IS NULL
      OR ft.vote_score < sqlc.narg(cursor_vote_score)::int
      OR (ft.vote_score = sqlc.narg(cursor_vote_score)::int AND (
        ft.created_at < @cursor_created_at
        OR (ft.created_at = @cursor_created_at AND ft.id < sqlc.narg(cursor_id)::uuid)
        OR @cursor_created_at IS NULL
      ))
    ))
    OR
    (@sort_by != 'top' AND (
      @cursor_created_at IS NULL
      OR ft.created_at < @cursor_created_at
      OR (ft.created_at = @cursor_created_at AND ft.id < sqlc.narg(cursor_id)::uuid)
    ))
  )
ORDER BY CASE WHEN @sort_by = 'top' THEN ft.vote_score END DESC,
         ft.created_at DESC, ft.id DESC
LIMIT @page_limit;

-- name: ListForumThreadsByTagWithDetails :many
SELECT ft.id, ft.tenant_id, ft.content_id, ft.space_id, ft.author_user_id,
       ft.parent_reply_id, ft.title, ft.slug, ft.body, ft.is_anonymous,
       ft.solved, ft.solved_at, ft.solved_by_user_id, ft.vote_score,
       ft.reply_count, ft.view_count, ft.bookmark_count, ft.edit_window_minutes,
       ft.is_edited, ft.created_at, ft.updated_at, ft.deleted_at,
       CASE WHEN ft.is_anonymous THEN NULL ELSE u.username END AS author_username,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.display_name END AS author_display_name,
       CASE WHEN ft.is_anonymous THEN NULL ELSE up.avatar_url END AS author_avatar_url,
       s.name AS space_name,
       s.slug AS space_slug,
       s.level AS space_level
FROM forum_threads ft
JOIN forum_thread_tags ftt ON ftt.thread_id = ft.id
LEFT JOIN users u ON u.id = ft.author_user_id
LEFT JOIN user_profiles up ON up.user_id = ft.author_user_id
LEFT JOIN spaces s ON s.id = ft.space_id
WHERE ft.tenant_id = @tenant_id
  AND ftt.tag_id = @tag_id
  AND ft.deleted_at IS NULL
  AND ft.solved = COALESCE(sqlc.narg(solved)::bool, ft.solved)
  AND (
    (@sort_by = 'top' AND (
      sqlc.narg(cursor_vote_score)::int IS NULL
      OR ft.vote_score < sqlc.narg(cursor_vote_score)::int
      OR (ft.vote_score = sqlc.narg(cursor_vote_score)::int AND (
        ft.created_at < @cursor_created_at
        OR (ft.created_at = @cursor_created_at AND ft.id < sqlc.narg(cursor_id)::uuid)
        OR @cursor_created_at IS NULL
      ))
    ))
    OR
    (@sort_by != 'top' AND (
      @cursor_created_at IS NULL
      OR ft.created_at < @cursor_created_at
      OR (ft.created_at = @cursor_created_at AND ft.id < sqlc.narg(cursor_id)::uuid)
    ))
  )
ORDER BY CASE WHEN @sort_by = 'top' THEN ft.vote_score END DESC,
         ft.created_at DESC, ft.id DESC
LIMIT @page_limit;

-- name: UpdateForumThread :one
UPDATE forum_threads
SET title = @title, body = @body, is_anonymous = @is_anonymous,
    is_edited = CASE
      WHEN edit_window_minutes IS NULL OR edit_window_minutes = 0 THEN true
      WHEN now() > created_at + (edit_window_minutes * interval '1 minute') THEN true
      ELSE false
    END,
    updated_at = now()
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
SET solved = true, solved_at = now(), solved_by_user_id = @solver_user_id,
    parent_reply_id = @parent_reply_id, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: UnsolveForumThread :exec
-- Reverts the solved flag (DELETE /forum/{id}/solve). Ref: Section 08 Solved flag.
UPDATE forum_threads
SET solved = false, solved_at = NULL, solved_by_user_id = NULL,
    parent_reply_id = NULL, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

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
          created_at, updated_at, deleted_at, ama_pending, ama_approved_at;

-- name: GetForumReplyByID :one
SELECT id, tenant_id, thread_id, parent_reply_id, author_user_id,
       body, vote_score, view_count, is_anonymous, is_edited,
       created_at, updated_at, deleted_at, ama_pending, ama_approved_at
FROM forum_replies
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: GetForumReplyByIDWithAuthor :one
-- Returns a single reply with author details. Used for the accepted-answer
-- in QAPage JSON-LD and the solved thread highlight (BL-012).
SELECT fr.id, fr.tenant_id, fr.thread_id, fr.parent_reply_id, fr.author_user_id,
       fr.body, fr.vote_score, fr.view_count, fr.is_anonymous, fr.is_edited,
       fr.created_at, fr.updated_at, fr.deleted_at,
       CASE WHEN fr.is_anonymous THEN NULL ELSE u.username END AS author_username,
       CASE WHEN fr.is_anonymous THEN NULL ELSE up.display_name END AS author_display_name,
       CASE WHEN fr.is_anonymous THEN NULL ELSE up.avatar_url END AS author_avatar_url
FROM forum_replies fr
LEFT JOIN users u ON u.id = fr.author_user_id
LEFT JOIN user_profiles up ON up.user_id = fr.author_user_id
WHERE fr.id = @id
  AND fr.tenant_id = @tenant_id
  AND fr.deleted_at IS NULL;

-- name: ListForumRepliesByThread :many
SELECT id, tenant_id, thread_id, parent_reply_id, author_user_id,
       body, vote_score, view_count, is_anonymous, is_edited,
       created_at, updated_at, deleted_at, ama_pending, ama_approved_at
FROM forum_replies
WHERE thread_id = @thread_id
  AND tenant_id = @tenant_id
  AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: ListForumRepliesByThreadWithAuthor :many
SELECT fr.id, fr.tenant_id, fr.thread_id, fr.parent_reply_id, fr.author_user_id,
       fr.body, fr.vote_score, fr.view_count, fr.is_anonymous, fr.is_edited,
       fr.created_at, fr.updated_at, fr.deleted_at, fr.ama_pending, fr.ama_approved_at,
       CASE WHEN fr.is_anonymous THEN NULL ELSE u.username END AS author_username,
       CASE WHEN fr.is_anonymous THEN NULL ELSE up.display_name END AS author_display_name,
       CASE WHEN fr.is_anonymous THEN NULL ELSE up.avatar_url END AS author_avatar_url
FROM forum_replies fr
LEFT JOIN users u ON u.id = fr.author_user_id
LEFT JOIN user_profiles up ON up.user_id = fr.author_user_id
WHERE fr.thread_id = @thread_id
  AND fr.tenant_id = @tenant_id
  AND fr.deleted_at IS NULL
ORDER BY fr.created_at ASC;

-- name: GetTopReplyForThread :one
-- Returns the highest-voted non-deleted reply for a thread. Used as the
-- accepted-answer candidate for QAPage JSON-LD on solved threads (BL-012).
SELECT fr.id, fr.tenant_id, fr.thread_id, fr.parent_reply_id, fr.author_user_id,
       fr.body, fr.vote_score, fr.view_count, fr.is_anonymous, fr.is_edited,
       fr.created_at, fr.updated_at, fr.deleted_at,
       CASE WHEN fr.is_anonymous THEN NULL ELSE u.username END AS author_username,
       CASE WHEN fr.is_anonymous THEN NULL ELSE up.display_name END AS author_display_name,
       CASE WHEN fr.is_anonymous THEN NULL ELSE up.avatar_url END AS author_avatar_url
FROM forum_replies fr
LEFT JOIN users u ON u.id = fr.author_user_id
LEFT JOIN user_profiles up ON up.user_id = fr.author_user_id
WHERE fr.thread_id = @thread_id
  AND fr.tenant_id = @tenant_id
  AND fr.deleted_at IS NULL
ORDER BY fr.vote_score DESC, fr.created_at ASC
LIMIT 1;

-- name: UpdateForumReply :one
UPDATE forum_replies fr
SET body = @body,
    is_edited = CASE
      WHEN ft.edit_window_minutes IS NULL OR ft.edit_window_minutes = 0 THEN true
      WHEN now() > fr.created_at + (ft.edit_window_minutes * interval '1 minute') THEN true
      ELSE false
    END,
    updated_at = now()
FROM forum_threads ft
WHERE fr.id = @id AND fr.tenant_id = @tenant_id AND fr.deleted_at IS NULL
  AND ft.id = fr.thread_id AND ft.tenant_id = fr.tenant_id
RETURNING fr.id, fr.tenant_id, fr.thread_id, fr.parent_reply_id, fr.author_user_id,
          fr.body, fr.vote_score, fr.view_count, fr.is_anonymous, fr.is_edited,
          fr.created_at, fr.updated_at, fr.deleted_at, fr.ama_pending, fr.ama_approved_at;

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

-- name: ListUserVotesForThread :many
-- Returns all forum vote values for a given user on a thread and its replies.
-- Used to hydrate per-entity user_vote state in the detail page (BUG-006).
SELECT fv.entity_type, fv.entity_id, fv.value
FROM forum_votes fv
WHERE fv.tenant_id = @tenant_id
  AND fv.user_id = @user_id
  AND ((fv.entity_type = 'thread' AND fv.entity_id = @thread_id)
       OR (fv.entity_type = 'reply' AND fv.entity_id IN (
           SELECT id FROM forum_replies WHERE thread_id = @thread_id AND tenant_id = @tenant_id
       )));

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
-- Casts a vote for a single poll option. The unique constraint is on
-- (option_id, user_id) to support multi-select polls (BUG-002). For
-- single-select polls the service clears the user's existing votes first.
INSERT INTO forum_poll_votes (poll_id, option_id, user_id)
VALUES (@poll_id, @option_id, @user_id)
ON CONFLICT (option_id, user_id) DO UPDATE
    SET created_at = now()
RETURNING id, poll_id, option_id, user_id, created_at;

-- name: DeleteForumPollVotesByUser :exec
-- Removes all of a user's votes on a given poll. Used when switching answers
-- in a single-select poll.
DELETE FROM forum_poll_votes
WHERE poll_id = @poll_id AND user_id = @user_id;

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

-- name: ListForumPollUserVotes :many
-- Returns every option this user has voted on for a given poll.
-- Used for multi-select polls (BUG-002).
SELECT fpv.id, fpv.poll_id, fpv.option_id, fpv.user_id, fpv.created_at
FROM forum_poll_votes fpv
WHERE fpv.poll_id = @poll_id AND fpv.user_id = @user_id;

-- name: GetForumPollByOptionID :one
-- Resolves the poll that owns a given option. Used by POST/DELETE
-- /poll-options/{id}/vote where the URL param is the option_id, not the
-- poll_id. Ref: api-routes-catalog L612-613.
SELECT p.id, p.thread_id, p.question, p.is_multi_select, p.close_at, p.created_at
FROM forum_polls p
JOIN forum_poll_options o ON o.poll_id = p.id
WHERE o.id = @option_id;

-- name: RetractForumPollVote :exec
-- Removes a user's vote for a specific poll option (DELETE /poll-options/{id}/vote).
DELETE FROM forum_poll_votes
WHERE option_id = @option_id AND user_id = @user_id;

-- =====================
-- Tags
-- =====================

-- name: CreateForumTag :one
INSERT INTO forum_tags (tenant_id, name, slug, description, rules, cover_image_id, moderator_user_id)
VALUES (@tenant_id, @name, @slug, sqlc.narg(description), sqlc.narg(rules),
        sqlc.narg(cover_image_id), sqlc.narg(moderator_user_id))
RETURNING id, tenant_id, name, slug, description, rules, cover_image_id,
          follower_count, moderator_user_id, created_at, deleted_at;

-- name: GetForumTagByID :one
SELECT id, tenant_id, name, slug, description, rules, cover_image_id,
       follower_count, moderator_user_id, created_at, deleted_at
FROM forum_tags
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: GetForumTagBySlug :one
SELECT id, tenant_id, name, slug, description, rules, cover_image_id,
       follower_count, moderator_user_id, created_at, deleted_at
FROM forum_tags
WHERE slug = @slug AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ListForumTags :many
SELECT id, tenant_id, name, slug, description, rules, cover_image_id,
       follower_count, moderator_user_id, created_at, deleted_at
FROM forum_tags
WHERE tenant_id = @tenant_id AND deleted_at IS NULL
ORDER BY follower_count DESC, name ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListForumThreadTags :many
-- Lists the tags attached to a thread, joining forum_tags for details.
-- Excludes soft-deleted tags. Ref: Section 08 (tags are first-class on
-- discussions) — GET /forum/{id}/tags.
SELECT ft.id, ft.tenant_id, ft.name, ft.slug, ft.description, ft.rules,
       ft.cover_image_id, ft.follower_count, ft.moderator_user_id, ft.created_at
FROM forum_thread_tags ftt
JOIN forum_tags ft ON ft.id = ftt.tag_id
WHERE ftt.thread_id = @thread_id AND ft.deleted_at IS NULL
ORDER BY ft.name ASC;

-- name: AddForumThreadTag :exec
INSERT INTO forum_thread_tags (thread_id, tag_id)
VALUES (@thread_id, @tag_id)
ON CONFLICT DO NOTHING;

-- name: CountForumThreadTags :one
-- Enforces the "max 5 tags per post" rule at the handler/service layer.
-- Ref: Section 08 Tags Schema.
SELECT COUNT(*)::int AS tag_count
FROM forum_thread_tags
WHERE thread_id = @thread_id;

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
          follower_count, moderator_user_id, created_at, deleted_at;

-- name: SoftDeleteForumTag :exec
UPDATE forum_tags SET deleted_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: IncrementForumThreadViewCount :exec
UPDATE forum_threads SET view_count = view_count + @delta
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- =====================
-- Bookmarks
-- Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Bookmarks)
-- Forum bookmarks live in the unified bookmarks table with entity_type
-- 'forum_thread' or 'forum_reply'.
-- =====================

-- name: AddForumBookmark :one
INSERT INTO bookmarks (tenant_id, user_id, entity_type, entity_id, folder, folder_id)
VALUES (@tenant_id, @user_id, @entity_type, @entity_id, @folder, @folder_id)
ON CONFLICT (user_id, entity_type, entity_id) DO UPDATE
    SET folder = EXCLUDED.folder, folder_id = EXCLUDED.folder_id
RETURNING id, tenant_id, user_id, entity_type, entity_id, folder, folder_id, created_at;

-- name: RemoveForumBookmark :exec
DELETE FROM bookmarks
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id AND tenant_id = @tenant_id;

-- name: GetForumBookmarkByID :one
-- Fetches a bookmark by its primary key (and owner) so DELETE
-- /forum/bookmarks/{id} can resolve the entity to decrement the
-- denormalized bookmark_count. Ref: api-routes-catalog L610.
SELECT id, tenant_id, user_id, entity_type, entity_id, folder, folder_id, created_at
FROM bookmarks
WHERE id = @id AND user_id = @user_id AND tenant_id = @tenant_id;

-- name: DeleteForumBookmarkByID :exec
DELETE FROM bookmarks
WHERE id = @id AND user_id = @user_id AND tenant_id = @tenant_id;

-- name: ListForumBookmarksByUser :many
SELECT id, tenant_id, user_id, entity_type, entity_id, folder, folder_id, created_at
FROM bookmarks
WHERE user_id = @user_id AND tenant_id = @tenant_id
  AND entity_type IN ('forum_thread', 'forum_reply')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: IsForumBookmarked :one
SELECT EXISTS(
    SELECT 1 FROM bookmarks
    WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id
) AS is_bookmarked;

-- name: GetForumBookmarkByEntity :one
-- Fetches a bookmark by (user_id, entity_type, entity_id) so the frontend
-- can call DELETE /forum/bookmarks/{id} without first listing all bookmarks.
SELECT id, tenant_id, user_id, entity_type, entity_id, folder, folder_id, created_at
FROM bookmarks
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

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

-- =====================
-- Solved / Unsolve
-- Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
-- =====================

-- name: GetForumThreadSpaceIDByID :one
SELECT space_id FROM forum_threads WHERE id = @id AND deleted_at IS NULL;

-- name: GetForumThreadTenantAndSpaceByID :one
-- Returns the tenant and space for a thread without requiring a tenant filter.
-- Used by background/WS paths that only have the thread_id (e.g. poll broadcasts).
SELECT tenant_id, space_id FROM forum_threads WHERE id = @id AND deleted_at IS NULL;

-- =====================
-- Polls — option-based lookup + vote retraction
-- Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
-- =====================

-- name: DeleteForumVoteByEntity :exec
DELETE FROM forum_votes
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;
