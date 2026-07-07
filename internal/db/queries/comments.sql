-- Comments system queries (Section 49 — Unified Comments).

-- =====================
-- Comments CRUD
-- =====================

-- name: CreateComment :one
INSERT INTO comments (tenant_id, content_id, author_user_id, parent_id, body, depth)
VALUES (@tenant_id, @content_id, @author_user_id, sqlc.narg(parent_id), @body, @depth)
RETURNING id, tenant_id, content_id, author_user_id, parent_id, body, depth,
          vote_score, is_pinned, is_edited, edited_at, created_at, updated_at, deleted_at;

-- name: GetComment :one
SELECT c.id, c.tenant_id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
       c.vote_score, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
       u.username,
       COALESCE(up.display_name, '') AS display_name,
       COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = c.tenant_id
WHERE c.id = @id AND c.tenant_id = @tenant_id;

-- name: ListCommentsByContent :many
SELECT c.id, c.tenant_id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
       c.vote_score, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
       u.username,
       COALESCE(up.display_name, '') AS display_name,
       COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = c.tenant_id
WHERE c.content_id = @content_id AND c.tenant_id = @tenant_id AND c.deleted_at IS NULL
  AND (c.parent_id IS NULL)
ORDER BY c.is_pinned DESC, c.vote_score DESC, c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListRepliesByParent :many
SELECT c.id, c.tenant_id, c.content_id, c.author_user_id, c.parent_id, c.body, c.depth,
       c.vote_score, c.is_pinned, c.is_edited, c.edited_at, c.created_at, c.updated_at, c.deleted_at,
       u.username,
       COALESCE(up.display_name, '') AS display_name,
       COALESCE(up.avatar_url, '') AS avatar_url
FROM comments c
JOIN users u ON u.id = c.author_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = c.tenant_id
WHERE c.parent_id = @parent_id AND c.tenant_id = @tenant_id AND c.deleted_at IS NULL
ORDER BY c.vote_score DESC, c.created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateComment :one
UPDATE comments
SET body = @body, is_edited = true, edited_at = now(), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND author_user_id = @author_user_id AND deleted_at IS NULL
RETURNING id, tenant_id, content_id, author_user_id, parent_id, body, depth,
          vote_score, is_pinned, is_edited, edited_at, created_at, updated_at, deleted_at;

-- name: SoftDeleteComment :exec
UPDATE comments
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: PinComment :exec
UPDATE comments SET is_pinned = true, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: UnpinComment :exec
UPDATE comments SET is_pinned = false, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- =====================
-- Comment Votes
-- =====================

-- name: UpsertCommentVote :one
INSERT INTO comment_votes (tenant_id, comment_id, user_id, value)
VALUES (@tenant_id, @comment_id, @user_id, @value)
ON CONFLICT (comment_id, user_id) DO UPDATE SET value = EXCLUDED.value
RETURNING id, tenant_id, comment_id, user_id, value, created_at;

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
updated_at = now()
WHERE id = @comment_id;

-- name: GetCommentVote :one
SELECT id, tenant_id, comment_id, user_id, value, created_at
FROM comment_votes
WHERE comment_id = @comment_id AND user_id = @user_id;

-- =====================
-- Comment Counts
-- =====================

-- name: CountCommentsByContent :one
SELECT COUNT(*)::int AS count
FROM comments
WHERE content_id = @content_id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: CountRepliesByParent :one
SELECT COUNT(*)::int AS count
FROM comments
WHERE parent_id = @parent_id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- =====================
-- Related Content (pgvector cosine similarity)
-- =====================

-- name: GetRelatedContent :many
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at,
       cs.relevance_embedding <=> @embedding AS similarity
FROM content_scores cs
JOIN contents c ON c.id = cs.content_id
WHERE c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.id != @exclude_id
  AND cs.relevance_embedding IS NOT NULL
ORDER BY cs.relevance_embedding <=> @embedding
LIMIT @page_limit;
