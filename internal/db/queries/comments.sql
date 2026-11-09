-- name: CreateComment :one
INSERT INTO comments (content_id, author_user_id, parent_id, body, depth, is_anonymous) VALUES (@content_id, @author_user_id, sqlc.narg(parent_id), @body, @depth, @is_anonymous)

RETURNING *;

-- name: GetComment :one
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.view_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.status,
 c.is_anonymous,
 COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE c.id = @id;

-- name: ListCommentsByContent :many
-- Soft-deleted comments with live replies are returned as tombstones
-- (Section 49 — body scrubbed by the handler) so reply trees stay intact.
-- Leaf soft-deletes stay filtered out.
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.view_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = @viewer_id
 LIMIT 1
 ), 0)::smallint AS user_vote,
 c.is_anonymous,
 COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE c.content_id = @content_id
 AND (c.deleted_at IS NULL OR c.reply_count > 0)
 AND c.status = 'published'
 AND (c.parent_id IS NULL) AND (
   @viewer_id::uuid IS NULL
   OR NOT EXISTS (
     SELECT 1 FROM user_blocks ub
     WHERE (ub.blocker_id = @viewer_id::uuid AND ub.blocked_id = c.author_user_id)
        OR (ub.blocker_id = c.author_user_id AND ub.blocked_id = @viewer_id::uuid)
   )
 )
ORDER BY c.is_pinned DESC, c.vote_score DESC, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPublicCommentsByContent :many
-- Public API (Section 47): top-level published comments for a content item,
-- but ONLY if the parent content itself is published and public. Comments on
-- drafts, private, or member-only content are never exposed to external API
-- consumers.
-- Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.view_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = @viewer_id
 LIMIT 1
 ), 0)::smallint AS user_vote,
 c.is_anonymous,
 COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
JOIN contents parent ON parent.id = c.content_id
WHERE c.content_id = @content_id
 AND (c.deleted_at IS NULL OR c.reply_count > 0)
 AND c.status = 'published'
 AND (c.parent_id IS NULL)
 AND parent.status = 'published'
 AND parent.visibility = 'public'
 AND parent.deleted_at IS NULL
ORDER BY c.is_pinned DESC, c.vote_score DESC, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPublicRepliesByParent :many
-- Public API parity with ListRepliesByParent: replies for a parent comment,
-- only when the parent content is public (tombstones included).
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.view_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 0::smallint AS user_vote,
 c.is_anonymous,
 COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
JOIN contents parent ON parent.id = c.content_id
WHERE c.parent_id = @parent_id AND c.content_id = @content_id
 AND (c.deleted_at IS NULL OR c.reply_count > 0)
 AND c.status = 'published'
 AND parent.status = 'published'
 AND parent.visibility = 'public'
 AND parent.deleted_at IS NULL
ORDER BY c.created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListCommentsByContentNew :many
-- Section 49 ?sort=new — newest first (pinned still surface on top).
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.view_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = @viewer_id
 LIMIT 1
 ), 0)::smallint AS user_vote,
 c.is_anonymous,
 COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE c.content_id = @content_id
 AND (c.deleted_at IS NULL OR c.reply_count > 0)
 AND c.status = 'published'
 AND (c.parent_id IS NULL) AND (
   @viewer_id::uuid IS NULL
   OR NOT EXISTS (
     SELECT 1 FROM user_blocks ub
     WHERE (ub.blocker_id = @viewer_id::uuid AND ub.blocked_id = c.author_user_id)
        OR (ub.blocker_id = c.author_user_id AND ub.blocked_id = @viewer_id::uuid)
   )
 )
ORDER BY c.is_pinned DESC, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListCommentsByContentOldest :many
-- Section 49 ?sort=oldest — oldest first (pinned still surface on top).
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.view_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = @viewer_id
 LIMIT 1
 ), 0)::smallint AS user_vote,
 c.is_anonymous,
 COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE c.content_id = @content_id
 AND (c.deleted_at IS NULL OR c.reply_count > 0)
 AND c.status = 'published'
 AND (c.parent_id IS NULL) AND (
   @viewer_id::uuid IS NULL
   OR NOT EXISTS (
     SELECT 1 FROM user_blocks ub
     WHERE (ub.blocker_id = @viewer_id::uuid AND ub.blocked_id = c.author_user_id)
        OR (ub.blocker_id = c.author_user_id AND ub.blocked_id = @viewer_id::uuid)
   )
 )
ORDER BY c.is_pinned DESC, c.created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListRepliesByParent :many
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.view_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = @viewer_id
 LIMIT 1
 ), 0)::smallint AS user_vote,
 c.is_anonymous,
 COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE c.parent_id = @parent_id AND c.content_id = @content_id
 AND (c.deleted_at IS NULL OR c.reply_count > 0)
 AND c.status = 'published' AND (
   @viewer_id::uuid IS NULL
   OR NOT EXISTS (
     SELECT 1 FROM user_blocks ub
     WHERE (ub.blocker_id = @viewer_id::uuid AND ub.blocked_id = c.author_user_id)
        OR (ub.blocker_id = c.author_user_id AND ub.blocked_id = @viewer_id::uuid)
   )
 )
ORDER BY c.vote_score DESC, c.created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateComment :one
-- Section 49 — edit after the edit window: marks is_edited = true and stamps
-- edited_at. Used when comments.edit_window_minutes has elapsed since creation.
UPDATE comments
SET body = @body, is_edited = true, edited_at = now(), updated_at = now()
WHERE id = @id AND author_user_id = @author_user_id AND deleted_at IS NULL
 AND status = 'published'
RETURNING *;

-- name: UpdateCommentUnmarked :one
-- Section 49 — edit within the edit window (grace period): does NOT set
-- is_edited so the comment shows no "edited" indicator. Used when the edit
-- occurs within comments.edit_window_minutes of creation.
UPDATE comments
SET body = @body, updated_at = now()
WHERE id = @id AND author_user_id = @author_user_id AND deleted_at IS NULL
 AND status = 'published'
RETURNING *;

-- name: UpdateCommentByID :one
-- Moderator edit (comments.edit_any) — no author filter; moderators bypass
-- the author edit window per Section 49.
UPDATE comments
SET body = @body, is_edited = true, edited_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL
 AND status = 'published'
RETURNING *;

-- name: SoftDeleteComment :exec
-- is_pinned is cleared so a deleted comment can't keep the top slot.
UPDATE comments
SET deleted_at = now(), is_pinned = false, updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: SoftDeleteCommentByID :exec
UPDATE comments SET deleted_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: SoftDeleteCommentByAuthor :exec
-- Section 49 — soft-delete own comment. The author_user_id clause makes the
-- ownership check atomic so a comments.delete_own holder cannot delete
-- another author's comment (BUG-003).
UPDATE comments
SET deleted_at = now(), is_pinned = false, updated_at = now()
WHERE id = @id AND author_user_id = @author_user_id
 AND deleted_at IS NULL;

-- name: RemoveComment :exec
-- Section 49 — moderator removal-by-status. Sets status='removed' and
-- records the acting moderator + reason, leaving the row visible for audit
-- (unlike a soft-delete which hides it from all queries). Used by the admin
-- moderation surface (BUG-010).
UPDATE comments
SET status = 'removed',
 removed_by = @removed_by,
 removed_reason = @removed_reason,
 is_pinned = false,
 updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: IncrementCommentReplyCount :exec
-- Section 49 — keep the denormalized parent reply_count in sync when a reply
-- is created (BUG-008).
UPDATE comments
SET reply_count = reply_count + 1, updated_at = now()
WHERE id = @parent_id AND deleted_at IS NULL;

-- name: DecrementCommentReplyCount :exec
-- Decrement the parent reply_count when a reply is deleted or removed (clamped at 0).
UPDATE comments
SET reply_count = GREATEST(reply_count - 1, 0), updated_at = now()
WHERE id = @parent_id AND deleted_at IS NULL;

-- name: PinComment :exec
UPDATE comments SET is_pinned = true, updated_at = now()
WHERE id = @id AND deleted_at IS NULL
 AND status = 'published';

-- name: UnpinComment :exec
UPDATE comments SET is_pinned = false, updated_at = now()
WHERE id = @id AND deleted_at IS NULL
 AND status = 'published';

-- =====================
-- Comment Votes
-- =====================

-- name: UpsertCommentVote :one
INSERT INTO comment_votes (comment_id, user_id, value, updated_at) VALUES (@comment_id, @user_id, @value, clock_timestamp())

ON CONFLICT (comment_id, user_id) DO UPDATE SET value = EXCLUDED.value, updated_at = clock_timestamp()
RETURNING comment_id, user_id, value, created_at, updated_at;

-- name: DeleteCommentVote :exec
DELETE FROM comment_votes
WHERE comment_id = @comment_id AND user_id = @user_id;

-- name: RecalculateCommentVoteScore :exec
UPDATE comments
SET vote_score = COALESCE((
 SELECT SUM(cv.value)::int
 FROM comment_votes cv
 WHERE cv.comment_id = @comment_id
), 0),
upvote_count = COALESCE((
 SELECT COUNT(*)::int
 FROM comment_votes cv
 WHERE cv.comment_id = @comment_id AND cv.value = 1
), 0),
downvote_count = COALESCE((
 SELECT COUNT(*)::int
 FROM comment_votes cv
 WHERE cv.comment_id = @comment_id AND cv.value = -1
), 0),
updated_at = now()
WHERE id = @comment_id;

-- name: GetCommentVote :one
SELECT comment_id, user_id, value, created_at
FROM comment_votes
WHERE comment_id = @comment_id AND user_id = @user_id;

-- =====================
-- Content Votes
-- =====================

-- name: UpsertContentVote :one
INSERT INTO content_votes (content_id, user_id, value, updated_at) VALUES (@content_id, @user_id, @value, clock_timestamp())

ON CONFLICT (content_id, user_id) DO UPDATE SET value = EXCLUDED.value, updated_at = clock_timestamp()
RETURNING content_id, user_id, value, created_at, updated_at;

-- name: DeleteContentVote :exec
DELETE FROM content_votes
WHERE content_id = @content_id AND user_id = @user_id;

-- name: RecalculateContentVoteScore :one
-- Same UPDATE shape as RecalculateCommentVoteScore; RETURNS the new
-- vote_score so the service can hand it to the VoteRail response.
UPDATE contents
SET vote_score = COALESCE((
 SELECT SUM(cv.value)::int
 FROM content_votes cv
 WHERE cv.content_id = @content_id
), 0),
upvote_count = COALESCE((
 SELECT COUNT(*)::int
 FROM content_votes cv
 WHERE cv.content_id = @content_id AND cv.value = 1
), 0),
downvote_count = COALESCE((
 SELECT COUNT(*)::int
 FROM content_votes cv
 WHERE cv.content_id = @content_id AND cv.value = -1
), 0),
updated_at = now()
WHERE id = @content_id
RETURNING vote_score;

-- name: GetContentVote :one
SELECT content_id, user_id, value, created_at
FROM content_votes
WHERE content_id = @content_id AND user_id = @user_id;

-- =====================
-- Comment Counts
-- =====================

-- name: CountCommentsByContent :one
SELECT COUNT(*)::int AS count
FROM comments
WHERE content_id = @content_id AND deleted_at IS NULL
 AND status = 'published';

-- name: CountRepliesByParent :one
-- Mirrors the ListRepliesByParent predicate (tombstones included,
-- content-scoped).
SELECT COUNT(*)::int AS count
FROM comments
WHERE parent_id = @parent_id AND content_id = @content_id
 AND (deleted_at IS NULL OR reply_count > 0)
 AND status = 'published';

-- =====================
-- Related Content (pgvector cosine similarity)
-- =====================

-- name: GetRelatedContent :many
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at,
 cs.relevance_embedding <=> @embedding AS similarity
FROM content_scores cs
JOIN contents c ON c.id = cs.content_id
WHERE c.status = 'published'
 AND c.visibility = 'public'
 AND c.deleted_at IS NULL
 AND c.id != @exclude_id
 AND cs.relevance_embedding IS NOT NULL
 AND (@content_type::text = '' OR c.content_type = @content_type)
ORDER BY cs.relevance_embedding <=> @embedding
LIMIT @page_limit;

-- name: ListCommentsByAuthor :many
-- Public "Replies" tab: comments the user authored on published+public content,
-- with parent-content title/slug for context. Public-safe: comments on
-- draft/private content and soft-deleted comments are excluded.
-- §42: gated_types carries the content_types whose feature flag is
-- unreachable for this caller — replies on those parents are excluded so
-- page totals and rows agree (no empty-page drift or count leak).
SELECT c.id, c.content_id, c.parent_id, c.body, c.vote_score,
 c.created_at,
 ct.title AS content_title, ct.slug AS content_slug, ct.content_type
FROM comments c
JOIN contents ct ON ct.id = c.content_id
WHERE c.author_user_id = @author_user_id
 AND c.deleted_at IS NULL
 AND c.status = 'published'
 AND c.is_anonymous = false
 AND ct.status = 'published'
 AND ct.visibility = 'public'
 AND NOT (ct.content_type = ANY(COALESCE(@gated_types::text[], '{}'::text[])))
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountCommentsByAuthor :one
SELECT COUNT(*)::int AS count
FROM comments c
JOIN contents ct ON ct.id = c.content_id
WHERE c.author_user_id = @author_user_id
 AND c.deleted_at IS NULL
 AND c.status = 'published'
 AND c.is_anonymous = false
 AND ct.status = 'published'
 AND ct.visibility = 'public'
 AND NOT (ct.content_type = ANY(COALESCE(@gated_types::text[], '{}'::text[])));

-- name: CountTopLevelCommentsByContent :one
-- Top-level published comments only — drives the frontend load-more meta.
-- Mirrors the list predicate (deleted parents with live replies count as
-- tombstones).
SELECT COUNT(*)::int AS count
FROM comments
WHERE content_id = @content_id AND parent_id IS NULL
 AND (deleted_at IS NULL OR reply_count > 0)
 AND status = 'published';

-- name: CanViewComments :one
-- Section 49 (C-001): whether the viewer may read/create comments on this
-- content. Mirrors IsContentVisibleToUser (contents.sql) plus the
-- members-only-needs-auth nuance — the session list route serves anonymous
-- guests, for whom 'members' content comments must stay hidden. Private
-- content comments are author-only (moderators use admin surfaces).
SELECT EXISTS (
    SELECT 1
    FROM contents c
    LEFT JOIN spaces s ON s.id = c.space_id
    WHERE c.id = @content_id
      AND c.status = 'published'
      AND c.deleted_at IS NULL
      AND c.auto_hidden = false
      AND (
          s.id IS NULL
          OR (s.deleted_at IS NULL AND (
              s.visibility = 'public'
              OR EXISTS (
                  SELECT 1 FROM space_members sm
                  WHERE sm.space_id = s.id
                    AND sm.user_id = @user_id
                    AND sm.status = 'approved'
              )
          ))
      )
      AND (
          c.visibility = 'public'
          OR (c.visibility = 'members' AND @authed::boolean)
          OR (c.visibility = 'private' AND c.author_id = @user_id)
      )
) AS visible;

-- =====================
-- Content Thread Links (Section 49 — blog/non-forum ↔ discussion cross-links)
-- =====================

-- name: CreateContentThreadLink :one
-- Upsert a cross-link; bump link_count when the pair already exists so
-- repeated "Start A Conversation" shares are reflected.
INSERT INTO content_thread_links (content_id, thread_id) VALUES (@content_id, @thread_id)

ON CONFLICT (content_id, thread_id) DO UPDATE
 SET link_count = content_thread_links.link_count + 1,
 created_at = now()
RETURNING id, content_id, thread_id, link_count, created_at;

-- name: ListContentThreadLinks :many
-- Linked discussion threads for a blog/content, most recently linked first.
-- Threads are only as visible as their own row: drafts, removed, and
-- audience-restricted threads (members/private, private-space) are filtered
-- per-viewer — mirrors the CanViewComments predicate (Section 49, C-021).
SELECT l.id, l.content_id, l.thread_id, l.link_count, l.created_at,
 ct.title AS thread_title, ct.slug AS thread_slug,
 ct.comment_count AS thread_comment_count,
 ct.author_id AS thread_author_id
FROM content_thread_links l
JOIN contents ct ON ct.id = l.thread_id
LEFT JOIN spaces ts ON ts.id = ct.space_id
WHERE l.content_id = @content_id
 AND ct.deleted_at IS NULL
 AND ct.status = 'published'
 AND ct.auto_hidden = false
 AND (
     ts.id IS NULL
     OR (ts.deleted_at IS NULL AND (
         ts.visibility = 'public'
         OR EXISTS (
             SELECT 1 FROM space_members sm
             WHERE sm.space_id = ts.id
               AND sm.user_id = @user_id
               AND sm.status = 'approved'
         )
     ))
 )
 AND (
     ct.visibility = 'public'
     OR (ct.visibility = 'members' AND @authed::boolean)
     OR (ct.visibility = 'private' AND ct.author_id = @user_id)
 )
ORDER BY l.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountContentThreadLinks :one
-- Mirrors the ListContentThreadLinks visibility predicate so pagination meta
-- matches the rows a viewer actually receives.
SELECT COUNT(*)::int AS count
FROM content_thread_links l
JOIN contents ct ON ct.id = l.thread_id
LEFT JOIN spaces ts ON ts.id = ct.space_id
WHERE l.content_id = @content_id
 AND ct.deleted_at IS NULL
 AND ct.status = 'published'
 AND ct.auto_hidden = false
 AND (
     ts.id IS NULL
     OR (ts.deleted_at IS NULL AND (
         ts.visibility = 'public'
         OR EXISTS (
             SELECT 1 FROM space_members sm
             WHERE sm.space_id = ts.id
               AND sm.user_id = @user_id
               AND sm.status = 'approved'
         )
     ))
 )
 AND (
     ct.visibility = 'public'
     OR (ct.visibility = 'members' AND @authed::boolean)
     OR (ct.visibility = 'private' AND ct.author_id = @user_id)
 );

-- name: InsertCommentView :exec
-- Section 49 — optional reply view tracking (admin-toggleable per Section
-- 08). Idempotent via the (comment_id, user_id) PK; the
-- trg_comment_views_increment trigger bumps comments.view_count.
-- The EXISTS guard silently skips nonexistent, deleted, or removed comments
-- so a view ping can never 500 on a dangling FK.
-- §42: gated_types carries the content_types whose feature flag is
-- unreachable for this caller — comments on those parents accrue no views.
INSERT INTO comment_views (comment_id, user_id)
SELECT @comment_id, @user_id
WHERE EXISTS (
    SELECT 1 FROM comments c
    JOIN contents ct ON ct.id = c.content_id
    WHERE c.id = @comment_id AND c.deleted_at IS NULL AND c.status = 'published'
      AND NOT (ct.content_type = ANY(COALESCE(@gated_types::text[], '{}'::text[])))
)
ON CONFLICT DO NOTHING;

-- name: InsertCommentViews :exec
-- Batch view recording (Guide §4.3 — one round trip per thread, not per
-- comment). Same live-comment guard + idempotency as InsertCommentView.
-- §42: gated_types skips comments on feature-flag-unreachable parents.
INSERT INTO comment_views (comment_id, user_id)
SELECT v.comment_id, @user_id
FROM unnest(@comment_ids::uuid[]) AS v(comment_id)
WHERE EXISTS (
    SELECT 1 FROM comments c
    JOIN contents ct ON ct.id = c.content_id
    WHERE c.id = v.comment_id AND c.deleted_at IS NULL AND c.status = 'published'
      AND NOT (ct.content_type = ANY(COALESCE(@gated_types::text[], '{}'::text[])))
)
ON CONFLICT DO NOTHING;

-- name: IncrementLinkedContentCommentCount :exec
-- Propagate a comment on a linked/anchored discussion thread up to the
-- parent content's denormalized count — feed cards and entity pages for
-- the article show the conversation total (unified-discussion spec).
UPDATE contents SET comment_count = comment_count + 1
WHERE id IN (SELECT content_id FROM content_thread_links WHERE thread_id = @thread_id);

-- name: DecrementLinkedContentCommentCount :exec
-- Mirror of IncrementLinkedContentCommentCount for comment delete/remove.
UPDATE contents SET comment_count = GREATEST(0, comment_count - 1)
WHERE id IN (SELECT content_id FROM content_thread_links WHERE thread_id = @thread_id);
