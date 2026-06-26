-- Section 32: Activity Feed / Timeline

-- name: InsertFeedItem :exec
INSERT INTO feeds (tenant_id, user_id, content_id, space_id, content_type, score)
VALUES (@tenant_id, @user_id, @content_id, @space_id, @content_type, @score)
ON CONFLICT (user_id, content_id) DO NOTHING;

-- name: GetExploreFeed :many
-- Trending content for guests (no user_id filter), pull from content_scores
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
WHERE c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.visibility = 'public'
ORDER BY cs.velocity_score DESC, cs.vote_score DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetPrimaryFeed :many
-- User's personalized feed from feeds table (push side)
SELECT f.id, f.content_id, f.space_id, f.content_type, f.score, f.created_at,
       c.title, c.slug, c.author_id, c.created_at AS content_created_at
FROM feeds f
JOIN contents c ON c.id = f.content_id
WHERE f.tenant_id = @tenant_id AND f.user_id = @user_id
  AND c.status = 'published'
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetSpaceFeed :many
-- Posts in a specific space
SELECT c.id, c.title, c.slug, c.content_type, c.author_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
WHERE c.tenant_id = @tenant_id AND c.space_id = @space_id
  AND c.status = 'published'
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetProfileFeed :many
-- All public posts by a specific user
SELECT c.id, c.title, c.slug, c.content_type, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
WHERE c.tenant_id = @tenant_id AND c.author_id = @author_id
  AND c.status = 'published'
  AND c.visibility = 'public'
ORDER BY c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetTopFeed :many
-- Top content in 7-day rolling window (for top sort)
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
WHERE c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.created_at > now() - interval '7 days'
ORDER BY cs.vote_score DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetRisingFeed :many
-- Rising content sorted by velocity score
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
WHERE c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.visibility = 'public'
ORDER BY cs.velocity_score DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateContentScore :exec
-- Update a specific score column on content_scores
UPDATE content_scores
SET vote_score = @vote_score,
    comment_count = @comment_count,
    view_count = @view_count,
    bookmark_count = @bookmark_count,
    last_scored_at = now()
WHERE content_id = @content_id;

-- name: UpdateVelocityScores :exec
-- Batch update velocity scores for posts < 7 days old
-- velocity = engagement / (hours_old + 2)^1.8
UPDATE content_scores cs
SET velocity_score = CASE
    WHEN EXTRACT(EPOCH FROM (now() - c.created_at)) / 3600 + 2 > 0
    THEN (cs.vote_score + cs.comment_count + cs.view_count + cs.bookmark_count)::numeric
         / (POWER(EXTRACT(EPOCH FROM (now() - c.created_at)) / 3600 + 2, 1.8))
    ELSE 0
END,
last_scored_at = now()
FROM contents c
WHERE cs.content_id = c.id
  AND c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.created_at > now() - interval '7 days';

-- name: DeleteFeedItemsByContent :exec
DELETE FROM feeds WHERE content_id = @content_id;

-- name: DeleteFeedItemsByUser :exec
DELETE FROM feeds WHERE user_id = @user_id;

-- name: GetFeedItemCount :one
SELECT COUNT(*)::int AS count FROM feeds WHERE user_id = @user_id;
