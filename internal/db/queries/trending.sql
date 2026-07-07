-- Trending & Discovery (Section 53)
-- Pre-computed trending snapshots and suggestion queries.

-- =====================
-- Trending Snapshots
-- =====================

-- name: UpsertTrendingSnapshot :exec
INSERT INTO trending_snapshots (tenant_id, entity_type, entity_id, score, "window", geo_code, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, entity_type, entity_id, "window", geo_code)
DO UPDATE SET score = EXCLUDED.score, computed_at = now(), expires_at = EXCLUDED.expires_at;

-- name: GetTrendingByType :many
SELECT id, tenant_id, entity_type, entity_id, score, "window", geo_code, computed_at, expires_at
FROM trending_snapshots
WHERE tenant_id = $1
  AND entity_type = $2
  AND "window" = $3
  AND expires_at > now()
  AND ($4::text IS NULL OR geo_code = $4)
ORDER BY score DESC
LIMIT $5;

-- name: GetTrendingContent :many
SELECT ts.id, ts.entity_id, ts.score, ts."window", ts.geo_code, ts.computed_at,
       c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id, c.view_count
FROM trending_snapshots ts
JOIN contents c ON c.id = ts.entity_id AND c.tenant_id = ts.tenant_id
WHERE ts.tenant_id = $1
  AND ts.entity_type = 'content'
  AND ts."window" = $2
  AND ts.expires_at > now()
  AND ($3::text IS NULL OR ts.geo_code = $3)
  AND c.status = 'published'
  AND c.visibility = 'public'
ORDER BY ts.score DESC
LIMIT $4;

-- name: GetTrendingTags :many
SELECT ts.id, ts.entity_id, ts.score, ts."window", ts.geo_code, ts.computed_at,
       t.id, t.name, t.slug, t.follower_count
FROM trending_snapshots ts
JOIN tags t ON t.id = ts.entity_id AND t.tenant_id = ts.tenant_id
WHERE ts.tenant_id = $1
  AND ts.entity_type = 'tag'
  AND ts."window" = $2
  AND ts.expires_at > now()
  AND ($3::text IS NULL OR ts.geo_code = $3)
ORDER BY ts.score DESC
LIMIT $4;

-- name: GetTrendingSpaces :many
SELECT ts.id, ts.entity_id, ts.score, ts."window", ts.geo_code, ts.computed_at,
       s.id, s.name, s.slug, s.member_count, s.post_count
FROM trending_snapshots ts
JOIN spaces s ON s.id = ts.entity_id AND s.tenant_id = ts.tenant_id
WHERE ts.tenant_id = $1
  AND ts.entity_type = 'space'
  AND ts."window" = $2
  AND ts.expires_at > now()
  AND ($3::text IS NULL OR ts.geo_code = $3)
ORDER BY ts.score DESC
LIMIT $4;

-- name: GetTrendingUsers :many
SELECT ts.id, ts.entity_id, ts.score, ts."window", ts.geo_code, ts.computed_at,
       u.id, u.username, up.display_name, up.avatar_url, up.bio
FROM trending_snapshots ts
JOIN users u ON u.id = ts.entity_id AND u.tenant_id = ts.tenant_id
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = ts.tenant_id
WHERE ts.tenant_id = $1
  AND ts.entity_type = 'user'
  AND ts."window" = $2
  AND ts.expires_at > now()
  AND ($3::text IS NULL OR ts.geo_code = $3)
ORDER BY ts.score DESC
LIMIT $4;

-- name: DeleteExpiredTrendingSnapshots :exec
DELETE FROM trending_snapshots
WHERE tenant_id = $1 AND expires_at < now();

-- name: DeleteTrendingSnapshotsByType :exec
DELETE FROM trending_snapshots
WHERE tenant_id = $1 AND entity_type = $2 AND "window" = $3 AND ($4::text IS NULL OR geo_code = $4);

-- =====================
-- Search Analytics (for trending queries)
-- =====================

-- name: GetTrendingSearchQueries :many
SELECT query, COUNT(*) AS search_count, AVG(results_count) AS avg_results
FROM search_analytics
WHERE tenant_id = $1
  AND created_at >= now() - INTERVAL '24 hours'
GROUP BY query
ORDER BY search_count DESC
LIMIT $2;

-- name: GetZeroResultTrendingQueries :many
SELECT query, COUNT(*) AS search_count
FROM search_analytics
WHERE tenant_id = $1
  AND results_count = 0
  AND created_at >= now() - INTERVAL '7 days'
GROUP BY query
ORDER BY search_count DESC
LIMIT $2;

-- =====================
-- Suggestions (computed from user relationships)
-- =====================

-- name: GetSuggestedUsers :many
SELECT DISTINCT u.id, u.username, up.display_name, up.avatar_url, up.bio,
       COUNT(DISTINCT sm2.space_id) AS shared_spaces
FROM users u
JOIN space_members sm ON sm.user_id = $1 AND sm.tenant_id = $2
JOIN space_members sm2 ON sm2.space_id = sm.space_id AND sm2.user_id = u.id AND sm2.tenant_id = $2
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = $2
WHERE u.id != $1
  AND u.tenant_id = $2
  AND NOT EXISTS (
      SELECT 1 FROM follows f
      WHERE f.follower_id = $1 AND f.entity_type = 'user' AND f.entity_id = u.id AND f.tenant_id = $2
  )
GROUP BY u.id, u.username, up.display_name, up.avatar_url, up.bio
ORDER BY shared_spaces DESC
LIMIT $3;

-- name: GetSuggestedSpaces :many
SELECT DISTINCT s.id, s.name, s.slug, s.member_count, s.post_count,
       COUNT(DISTINCT sm2.user_id) AS shared_members
FROM spaces s
JOIN space_members sm ON sm.user_id = $1 AND sm.tenant_id = $2
JOIN space_members sm2 ON sm2.space_id = s.id AND sm2.tenant_id = $2
WHERE s.id NOT IN (
    SELECT space_id FROM space_members WHERE user_id = $1 AND tenant_id = $2
)
AND s.tenant_id = $2
GROUP BY s.id, s.name, s.slug, s.member_count, s.post_count
ORDER BY shared_members DESC
LIMIT $3;

-- name: GetSuggestedTags :many
SELECT DISTINCT t.id, t.name, t.slug, t.follower_count
FROM tags t
JOIN contents c ON c.tenant_id = t.tenant_id AND c.id IN (
    SELECT ct.content_id FROM content_tags ct
    JOIN contents c2 ON c2.id = ct.content_id AND c2.author_id = $1 AND c2.tenant_id = $2
)
WHERE t.tenant_id = $2
AND NOT EXISTS (
    SELECT 1 FROM follows f
    WHERE f.follower_id = $1 AND f.entity_type = 'tag' AND f.entity_id = t.id AND f.tenant_id = $2
)
ORDER BY t.follower_count DESC
LIMIT $3;
