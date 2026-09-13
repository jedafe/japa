-- name: CreateComment :one
INSERT INTO comments (content_id, author_user_id, parent_id, body, depth, is_anonymous) VALUES (@content_id, @author_user_id, sqlc.narg(parent_id), @body, @depth, @is_anonymous)

RETURNING *;

-- name: GetComment :one
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.status,
 c.is_anonymous,
 u.username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE c.id = @id;

-- name: ListCommentsByContent :many
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = @viewer_id
 LIMIT 1
 ), 0)::smallint AS user_vote,
 c.is_anonymous,
 u.username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE c.content_id = @content_id AND c.deleted_at IS NULL
 AND c.status = 'published'
 AND (c.parent_id IS NULL)
ORDER BY c.is_pinned DESC, c.vote_score DESC, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPublicCommentsByContent :many
-- Public API (Section 47): top-level published comments for a content item,
-- but ONLY if the parent content itself is published and public. Comments on
-- drafts, private, or member-only content are never exposed to external API
-- consumers.
-- Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = @viewer_id
 LIMIT 1
 ), 0)::smallint AS user_vote,
 c.is_anonymous,
 u.username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
JOIN contents parent ON parent.id = c.content_id
WHERE c.content_id = @content_id
 AND c.deleted_at IS NULL
 AND c.status = 'published'
 AND (c.parent_id IS NULL)
 AND parent.status = 'published'
 AND parent.visibility = 'public'
 AND parent.deleted_at IS NULL
ORDER BY c.is_pinned DESC, c.vote_score DESC, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListCommentsByContentNew :many
-- Section 49 ?sort=new — newest first (pinned still surface on top).
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = @viewer_id
 LIMIT 1
 ), 0)::smallint AS user_vote,
 c.is_anonymous,
 u.username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE c.content_id = @content_id AND c.deleted_at IS NULL
 AND c.status = 'published'
 AND (c.parent_id IS NULL)
ORDER BY c.is_pinned DESC, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListCommentsByContentOldest :many
-- Section 49 ?sort=oldest — oldest first (pinned still surface on top).
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = @viewer_id
 LIMIT 1
 ), 0)::smallint AS user_vote,
 c.is_anonymous,
 u.username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE c.content_id = @content_id AND c.deleted_at IS NULL
 AND c.status = 'published'
 AND (c.parent_id IS NULL)
ORDER BY c.is_pinned DESC, c.created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListRepliesByParent :many
SELECT c.id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
 c.vote_score, c.upvote_count, c.downvote_count, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
 c.reply_count,
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = @viewer_id
 LIMIT 1
 ), 0)::smallint AS user_vote,
 c.is_anonymous,
 u.username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE c.parent_id = @parent_id AND c.deleted_at IS NULL
 AND c.status = 'published'
ORDER BY c.vote_score DESC, c.created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateComment :one
-- Section 49 — edit after the edit window: marks is_edited = true and stamps
-- edited_at. Used when comments.edit_window_minutes has elapsed since creation.
UPDATE comments
SET body = @body, is_edited = true, edited_at = now(), updated_at = now()
WHERE id = @id AND author_user_id = @author_user_id AND deleted_at IS NULL
RETURNING *;

-- name: UpdateCommentUnmarked :one
-- Section 49 — edit within the edit window (grace period): does NOT set
-- is_edited so the comment shows no "edited" indicator. Used when the edit
-- occurs within comments.edit_window_minutes of creation.
UPDATE comments
SET body = @body, updated_at = now()
WHERE id = @id AND author_user_id = @author_user_id AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteComment :exec
UPDATE comments
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: SoftDeleteCommentByID :exec
UPDATE comments SET deleted_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: SoftDeleteCommentByAuthor :exec
-- Section 49 — soft-delete own comment. The author_user_id clause makes the
-- ownership check atomic so a comments.delete_own holder cannot delete
-- another author's comment (BUG-003).
UPDATE comments
SET deleted_at = now(), updated_at = now()
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
WHERE id = @id AND deleted_at IS NULL;

-- name: UnpinComment :exec
UPDATE comments SET is_pinned = false, updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- =====================
-- Comment Votes
-- =====================

-- name: UpsertCommentVote :one
INSERT INTO comment_votes (comment_id, user_id, value) VALUES (@comment_id, @user_id, @value)

ON CONFLICT (comment_id, user_id) DO UPDATE SET value = EXCLUDED.value
RETURNING comment_id, user_id, value, created_at;

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
-- Comment Counts
-- =====================

-- name: CountCommentsByContent :one
SELECT COUNT(*)::int AS count
FROM comments
WHERE content_id = @content_id AND deleted_at IS NULL
 AND status = 'published';

-- name: CountRepliesByParent :one
SELECT COUNT(*)::int AS count
FROM comments
WHERE parent_id = @parent_id AND deleted_at IS NULL
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
ORDER BY cs.relevance_embedding <=> @embedding
LIMIT @page_limit;

-- name: ListCommentsByAuthor :many
-- Public "Replies" tab: comments the user authored on published+public content,
-- with parent-content title/slug for context. Public-safe: comments on
-- draft/private content and soft-deleted comments are excluded.
SELECT c.id, c.content_id, c.parent_id, c.body, c.vote_score,
 c.created_at,
 ct.title AS content_title, ct.slug AS content_slug
FROM comments c
JOIN contents ct ON ct.id = c.content_id
WHERE c.author_user_id = @author_user_id
 AND c.deleted_at IS NULL
 AND c.status = 'published'
 AND c.is_anonymous = false
 AND ct.status = 'published'
 AND ct.visibility = 'public'
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountCommentsByAuthor :one
SELECT COUNT(*)::int AS count
FROM comments c
JOIN contents ct ON ct.id = c.content_id
WHERE c.author_user_id = @author_user_id
 AND c.deleted_at IS NULL
 AND c.is_anonymous = false
 AND ct.status = 'published'
 AND ct.visibility = 'public';

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
SELECT l.id, l.content_id, l.thread_id, l.link_count, l.created_at,
 ct.title AS thread_title, ct.slug AS thread_slug,
 ct.comment_count AS thread_comment_count,
 ct.author_id AS thread_author_id
FROM content_thread_links l
JOIN contents ct ON ct.id = l.thread_id
WHERE l.content_id = @content_id
 AND ct.deleted_at IS NULL
ORDER BY l.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountContentThreadLinks :one
SELECT COUNT(*)::int AS count
FROM content_thread_links
WHERE content_id = @content_id;
