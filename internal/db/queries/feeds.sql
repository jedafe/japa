-- Section 32: Activity Feed / Timeline

-- name: InsertFeedItem :exec
INSERT INTO feeds (tenant_id, user_id, content_id, space_id, content_type, score)
VALUES (@tenant_id, @user_id, @content_id, @space_id, @content_type, @score)
ON CONFLICT (user_id, content_id) DO NOTHING;

-- name: GetExploreFeed :many
-- Trending content for guests (no user_id filter), pull from content_scores
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       COALESCE(s.name, '') AS space_name,
       COALESCE(s.slug, '') AS space_slug,
       MAX(LEFT(c.body, 200))::text AS excerpt,
       cs.bookmark_count,
       c.read_time_minutes,
       m.storage_bucket AS cover_storage_bucket,
       m.storage_path AS cover_storage_path,
       COALESCE(string_agg(t.name, ',' ORDER BY t.name), '') AS tags
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN media m ON m.id = c.header_image_id AND m.deleted_at IS NULL
WHERE c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND c.visibility = 'public'
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count,
         u.username, up.display_name, up.avatar_url,
         s.name, s.slug,
         m.storage_bucket, m.storage_path
ORDER BY cs.velocity_score DESC, cs.vote_score DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetPrimaryFeed :many
-- User's personalized feed from feeds table (push side).
-- sort: 'latest' (default), 'top' (vote_score DESC), 'rising' (velocity_score DESC).
-- 'relevant' falls back to latest (pgvector relevance is only used in the for-you feed).
SELECT f.id, f.content_id, f.space_id, f.content_type, f.score, f.created_at,
       c.title, c.slug, c.author_id, c.created_at AS content_created_at,
       c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score, cs.bookmark_count,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       COALESCE(s.name, '') AS space_name,
       COALESCE(s.slug, '') AS space_slug,
       MAX(LEFT(c.body, 200))::text AS excerpt,
       c.read_time_minutes,
       m.storage_bucket AS cover_storage_bucket,
       m.storage_path AS cover_storage_path,
       COALESCE(string_agg(t.name, ',' ORDER BY t.name), '') AS tags
FROM feeds f
JOIN contents c ON c.id = f.content_id
LEFT JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN media m ON m.id = c.header_image_id AND m.deleted_at IS NULL
WHERE f.tenant_id = @tenant_id AND f.user_id = @user_id
  AND c.status = 'published'
  AND c.deleted_at IS NULL
GROUP BY f.id, f.content_id, f.space_id, f.content_type, f.score, f.created_at,
         c.title, c.slug, c.author_id, c.created_at,
         c.view_count, c.comment_count, cs.vote_score, cs.velocity_score, cs.bookmark_count,
         u.username, up.display_name, up.avatar_url,
         s.name, s.slug,
         c.read_time_minutes, m.storage_bucket, m.storage_path
ORDER BY
    CASE WHEN @sort = 'top' THEN cs.vote_score END DESC,
    CASE WHEN @sort = 'rising' THEN cs.velocity_score END DESC,
    f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetSpaceFeed :many
-- Posts in a specific space.
-- sort: 'latest' (default), 'top' (vote_score DESC), 'rising' (velocity_score DESC).
-- 'relevant' falls back to latest (pgvector relevance is only used in the for-you feed).
SELECT c.id, c.title, c.slug, c.content_type, c.author_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score,
       cs.bookmark_count,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       COALESCE(s.name, '') AS space_name,
       COALESCE(s.slug, '') AS space_slug,
       MAX(LEFT(c.body, 200))::text AS excerpt,
       c.read_time_minutes,
       m.storage_bucket AS cover_storage_bucket,
       m.storage_path AS cover_storage_path,
       COALESCE(string_agg(t.name, ',' ORDER BY t.name), '') AS tags
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN media m ON m.id = c.header_image_id AND m.deleted_at IS NULL
WHERE c.tenant_id = @tenant_id AND c.space_id = @space_id
  AND c.status = 'published'
  AND c.deleted_at IS NULL
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count,
         u.username, up.display_name, up.avatar_url,
         s.name, s.slug,
         m.storage_bucket, m.storage_path
ORDER BY
    CASE WHEN @sort = 'top' THEN cs.vote_score END DESC,
    CASE WHEN @sort = 'rising' THEN cs.velocity_score END DESC,
    c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetProfileFeed :many
-- All public posts by a specific user.
-- sort: 'latest' (default), 'top' (vote_score DESC), 'rising' (velocity_score DESC).
-- 'relevant' falls back to latest (pgvector relevance is only used in the for-you feed).
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score,
       cs.bookmark_count,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       COALESCE(s.name, '') AS space_name,
       COALESCE(s.slug, '') AS space_slug,
       MAX(LEFT(c.body, 200))::text AS excerpt,
       c.read_time_minutes,
       m.storage_bucket AS cover_storage_bucket,
       m.storage_path AS cover_storage_path,
       COALESCE(string_agg(t.name, ',' ORDER BY t.name), '') AS tags
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN media m ON m.id = c.header_image_id AND m.deleted_at IS NULL
WHERE c.tenant_id = @tenant_id AND c.author_id = @author_id
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND c.visibility = 'public'
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count,
         u.username, up.display_name, up.avatar_url,
         s.name, s.slug,
         m.storage_bucket, m.storage_path
ORDER BY
    CASE WHEN @sort = 'top' THEN cs.vote_score END DESC,
    CASE WHEN @sort = 'rising' THEN cs.velocity_score END DESC,
    c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetTopFeed :many
-- Top content in 7-day rolling window (for top sort)
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score,
       cs.bookmark_count,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       COALESCE(s.name, '') AS space_name,
       COALESCE(s.slug, '') AS space_slug,
       MAX(LEFT(c.body, 200))::text AS excerpt,
       c.read_time_minutes,
       m.storage_bucket AS cover_storage_bucket,
       m.storage_path AS cover_storage_path,
       COALESCE(string_agg(t.name, ',' ORDER BY t.name), '') AS tags
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN media m ON m.id = c.header_image_id AND m.deleted_at IS NULL
WHERE c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND c.visibility = 'public'
  AND c.created_at > now() - interval '7 days'
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count,
         u.username, up.display_name, up.avatar_url,
         s.name, s.slug,
         m.storage_bucket, m.storage_path
ORDER BY cs.vote_score DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetRisingFeed :many
-- Rising content sorted by velocity score
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score,
       cs.bookmark_count,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       COALESCE(s.name, '') AS space_name,
       COALESCE(s.slug, '') AS space_slug,
       MAX(LEFT(c.body, 200))::text AS excerpt,
       c.read_time_minutes,
       m.storage_bucket AS cover_storage_bucket,
       m.storage_path AS cover_storage_path,
       COALESCE(string_agg(t.name, ',' ORDER BY t.name), '') AS tags
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN media m ON m.id = c.header_image_id AND m.deleted_at IS NULL
WHERE c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND c.visibility = 'public'
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count,
         u.username, up.display_name, up.avatar_url,
         s.name, s.slug,
         m.storage_bucket, m.storage_path
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
-- Batch update velocity scores for published content < 30 days old.
-- velocity = (vote_score * 2 + comment_count + view_count / 100) /
--   (hours_since_publish ^ gravity)
-- gravity is passed as a parameter so it can be hot-reloaded from
-- system_settings (feed.velocity.gravity, default 1.8).
UPDATE content_scores cs
SET velocity_score = CASE
    WHEN c.published_at IS NOT NULL AND EXTRACT(EPOCH FROM (now() - c.published_at)) > 0
    THEN GREATEST(0, (cs.vote_score * 2 + cs.comment_count + cs.view_count / 100.0)::numeric
         / POWER(EXTRACT(EPOCH FROM (now() - c.published_at)) / 3600.0, @gravity))
    ELSE 0
END,
last_scored_at = now()
FROM contents c
WHERE cs.content_id = c.id
  AND c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND c.created_at >= now() - interval '30 days';

-- name: DeleteFeedItemsByContent :exec
DELETE FROM feeds WHERE content_id = @content_id AND tenant_id = @tenant_id;

-- name: DeleteFeedItemsByUser :exec
DELETE FROM feeds WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: GetFeedItemCount :one
SELECT COUNT(*)::int AS count FROM feeds WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: GetFollowedEntityIDsByType :many
-- Lean ID-only lookup of everything a user follows of one entity type.
SELECT entity_id
FROM follows
WHERE tenant_id = @tenant_id
  AND follower_id = @follower_id
  AND entity_type = @entity_type;

-- name: GetMemberSpaceIDs :many
-- Space IDs the user is a member of (affinity signal, distinct from follows).
SELECT space_id
FROM space_members
WHERE tenant_id = @tenant_id
  AND user_id = @user_id;

-- name: GetForYouCandidates :many
-- Bounded personalized candidate pool = UNION of (a) recent content from
-- followed authors/spaces/tags and (b) top recent content by velocity.
-- Emits per-row affinity flags for Go-side ranking. Excludes own posts.
-- Array params are COALESCEd to empty arrays so `= ANY(NULL)` never nullifies
-- the affinity predicate when a user follows nothing of a given type.
WITH affinity AS (
    SELECT c.id
    FROM contents c
    WHERE c.tenant_id = @tenant_id
      AND c.status = 'published'
      AND c.deleted_at IS NULL
      AND c.visibility = 'public'
      AND c.created_at >= @window_start
      AND c.author_id <> @exclude_author_id
      AND (
            c.author_id = ANY(COALESCE(@author_ids::uuid[], ARRAY[]::uuid[]))
         OR c.space_id  = ANY(COALESCE(@space_ids::uuid[], ARRAY[]::uuid[]))
         OR EXISTS (
              SELECT 1 FROM content_tags ct
              WHERE ct.content_id = c.id AND ct.tag_id = ANY(COALESCE(@tag_ids::uuid[], ARRAY[]::uuid[]))
         )
      )
    ORDER BY c.created_at DESC
    LIMIT @affinity_limit
),
velocity AS (
    SELECT c.id
    FROM contents c
    JOIN content_scores cs ON cs.content_id = c.id
    WHERE c.tenant_id = @tenant_id
      AND c.status = 'published'
      AND c.deleted_at IS NULL
      AND c.visibility = 'public'
      AND c.created_at >= @window_start
      AND c.author_id <> @exclude_author_id
    ORDER BY cs.velocity_score DESC
    LIMIT @velocity_limit
),
candidate_ids AS (
    SELECT id FROM affinity
    UNION
    SELECT id FROM velocity
)
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score, cs.bookmark_count,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       COALESCE(s.name, '') AS space_name,
       COALESCE(s.slug, '') AS space_slug,
       MAX(LEFT(c.body, 200))::text AS excerpt,
       c.read_time_minutes,
       m.storage_bucket AS cover_storage_bucket,
       m.storage_path   AS cover_storage_path,
       COALESCE(string_agg(t.name, ',' ORDER BY t.name), '') AS tags,
       (c.author_id = ANY(COALESCE(@author_ids::uuid[], ARRAY[]::uuid[])))            AS is_author_followed,
       (c.space_id  = ANY(COALESCE(@space_ids::uuid[], ARRAY[]::uuid[])))             AS is_space_followed,
       COUNT(DISTINCT t.id) FILTER (
           WHERE t.id = ANY(COALESCE(@tag_ids::uuid[], ARRAY[]::uuid[]))
       )::int                                              AS followed_tag_overlap,
       s.geo_code                                          AS space_geo_code
FROM contents c
JOIN candidate_ids ci ON ci.id = c.id
LEFT JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u  ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN media m ON m.id = c.header_image_id AND m.deleted_at IS NULL
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count,
         u.username, up.display_name, up.avatar_url,
         s.name, s.slug, s.geo_code,
         m.storage_bucket, m.storage_path
LIMIT @candidate_cap;

-- name: ListDigestFeedItems :many
SELECT c.id, c.title, c.slug, c.content_type
FROM feeds f
JOIN contents c ON c.id = f.content_id
WHERE f.user_id = @user_id AND f.tenant_id = @tenant_id
  AND f.created_at >= now() - interval '7 days'
ORDER BY f.created_at DESC
LIMIT 20;

-- name: PruneOldFeeds :exec
-- Intentionally global (no tenant_id filter): this is a retention prune that
-- runs weekly across all tenants. Ref: Section 32, asynq-tasks catalog.
DELETE FROM feeds WHERE created_at < now() - interval '30 days';
