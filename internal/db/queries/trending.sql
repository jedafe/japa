-- Trending & Discovery (Section 53)
-- Pre-computed trending snapshots and suggestion queries.

-- =====================
-- Trending Snapshots
-- =====================

-- name: UpsertTrendingSnapshot :exec
INSERT INTO trending_snapshots (entity_type, entity_id, score, "window", geo_code, expires_at) VALUES ($1, $2, $3, $4, $5, $6)

ON CONFLICT (entity_type, entity_id, "window", geo_code)
DO UPDATE SET score = EXCLUDED.score, computed_at = now(), expires_at = EXCLUDED.expires_at;

-- name: GetTrendingByType :many
SELECT id, entity_type, entity_id, score, "window", geo_code, computed_at, expires_at
FROM trending_snapshots
WHERE entity_type = $1
 AND "window" = $2
 AND expires_at > now()
 AND ($3::text IS NULL OR geo_code = $3)
ORDER BY score DESC
LIMIT $4;

-- name: GetTrendingContent :many
-- Dedupe by content: an entity can hold one snapshot per geo_code, so an
-- unfiltered (global) read would list the same content once per scope.
SELECT id, entity_id, score, "window", geo_code, computed_at,
 content_id AS id, title, slug, content_type, author_id, space_id, view_count
FROM (
 SELECT DISTINCT ON (c.id)
 ts.id, ts.entity_id, ts.score, ts."window", ts.geo_code, ts.computed_at,
 c.id AS content_id, c.title, c.slug, c.content_type, c.author_id, c.space_id, c.view_count
 FROM trending_snapshots ts
 JOIN contents c ON c.id = ts.entity_id
 WHERE ts.entity_type = 'content'
 AND ts."window" = $1
 AND ts.expires_at > now()
 AND ($2::text IS NULL OR $2::text = '' OR ts.geo_code = $2)
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND c.deleted_at IS NULL
 ORDER BY c.id, ts.score DESC
) dedup
ORDER BY score DESC
LIMIT $3;

-- name: GetTrendingTags :many
SELECT id, entity_id, score, "window", geo_code, computed_at,
 tag_id AS id, name, slug, follower_count
FROM (
 SELECT DISTINCT ON (t.id)
 ts.id, ts.entity_id, ts.score, ts."window", ts.geo_code, ts.computed_at,
 t.id AS tag_id, t.name, t.slug, t.follower_count
 FROM trending_snapshots ts
 JOIN tags t ON t.id = ts.entity_id
 WHERE ts.entity_type = 'tag'
 AND ts."window" = $1
 AND ts.expires_at > now()
 AND ($2::text IS NULL OR $2::text = '' OR ts.geo_code = $2)
 ORDER BY t.id, ts.score DESC
) dedup
ORDER BY score DESC
LIMIT $3;

-- name: GetTrendingSpaces :many
SELECT id, entity_id, score, "window", geo_code, computed_at,
 space_id AS id, name, slug, space_geo_code, member_count, post_count
FROM (
 SELECT DISTINCT ON (s.id)
 ts.id, ts.entity_id, ts.score, ts."window", ts.geo_code, ts.computed_at,
 s.id AS space_id, s.name, s.slug, s.geo_code AS space_geo_code, s.member_count, s.post_count
 FROM trending_snapshots ts
 JOIN spaces s ON s.id = ts.entity_id
 WHERE ts.entity_type = 'space'
 AND ts."window" = $1
 AND ts.expires_at > now()
 AND ($2::text IS NULL OR $2::text = '' OR ts.geo_code = $2)
 AND s.deleted_at IS NULL
 -- Member-created spaces only — primary (auto-joined geo + platform-seeded)
 -- spaces are not a discovery surface (Section 05 amendment).
 AND s.level = 'custom'
 AND s.creator_user_id IS NOT NULL
 -- §80 wiki-only types (attraction) — never a trending surface.
 AND NOT EXISTS (
 SELECT 1 FROM space_types st WHERE st.id = s.space_type_id AND st.wiki_only
 )
 ORDER BY s.id, ts.score DESC
) dedup
ORDER BY score DESC
LIMIT $3;

-- name: GetTrendingUsers :many
SELECT id, entity_id, score, "window", geo_code, computed_at,
 user_id AS id, username, display_name, avatar_url, bio
FROM (
 SELECT DISTINCT ON (u.id)
 ts.id, ts.entity_id, ts.score, ts."window", ts.geo_code, ts.computed_at,
 u.id AS user_id, COALESCE(u.public_username, u.username, '') AS username, up.display_name, up.avatar_url, up.bio
 FROM trending_snapshots ts
 JOIN users u ON u.id = ts.entity_id
 LEFT JOIN user_profiles up ON up.user_id = u.id
 WHERE ts.entity_type = 'user'
 AND ts."window" = $1
 AND ts.expires_at > now()
 AND ($2::text IS NULL OR $2::text = '' OR ts.geo_code = $2)
 AND u.status = 'active'
 AND u.is_synthetic = false
 ORDER BY u.id, ts.score DESC
) dedup
ORDER BY score DESC
LIMIT $3;

-- name: DeleteExpiredTrendingSnapshots :exec
DELETE FROM trending_snapshots
WHERE expires_at < now();

-- name: DeleteTrendingSnapshotsByType :exec
DELETE FROM trending_snapshots
WHERE entity_type = $1 AND "window" = $2 AND ($3::text IS NULL OR geo_code = $3);

-- =====================
-- Search Analytics (for trending queries)
-- =====================

-- name: GetTrendingSearchQueries :many
SELECT query, COUNT(*) AS search_count, AVG(results_count) AS avg_results
FROM search_analytics
WHERE created_at >= now() - INTERVAL '24 hours'
GROUP BY query
ORDER BY search_count DESC
LIMIT $1;

-- name: GetZeroResultTrendingQueries :many
SELECT query, COUNT(*) AS search_count
FROM search_analytics
WHERE results_count = 0
 AND created_at >= now() - INTERVAL '7 days'
GROUP BY query
ORDER BY search_count DESC
LIMIT $1;

-- =====================
-- Suggestions (computed from user relationships)
-- =====================

-- name: GetSuggestedUsers :many
-- ADR-064: when geoCode is provided, users whose shared spaces match the
-- visitor's ContentCountry are preferred (re-weighted, not hidden). Empty
-- geoCode = global ranking (original behavior).
-- display_name falls back to username: the LEFT JOIN on user_profiles can
-- produce NULL (no profile row / empty name) and the API contract exposes
-- display_name as a non-null string consumed directly by suggestion UIs.
SELECT DISTINCT u.id, COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(NULLIF(up.display_name, ''), u.public_username, u.username, '') AS display_name,
 up.avatar_url, up.bio,
 COUNT(DISTINCT sm2.space_id) AS shared_spaces,
 COUNT(DISTINCT CASE WHEN sp.geo_code = sqlc.narg(geo_code) THEN sm2.space_id END) AS geo_shared_spaces
FROM users u
JOIN space_members sm ON sm.user_id = $1
JOIN space_members sm2 ON sm2.space_id = sm.space_id AND sm2.user_id = u.id
LEFT JOIN spaces sp ON sp.id = sm2.space_id AND sp.deleted_at IS NULL
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE u.id != $1
 AND u.status = 'active'
 AND u.is_synthetic = false
 AND NOT EXISTS (
 SELECT 1 FROM follows f
 WHERE f.follower_id = $1 AND f.entity_type = 'user' AND f.entity_id = u.id
 )
GROUP BY u.id, u.username, u.public_username, up.display_name, up.avatar_url, up.bio
ORDER BY geo_shared_spaces DESC, shared_spaces DESC
LIMIT $2;

-- name: GetSuggestedSpaces :many
-- ADR-064: when geoCode is provided, spaces matching the visitor's
-- ContentCountry are preferred (re-weighted, not hidden). Empty geoCode =
-- global ranking (original behavior).
-- ADR-128: spaces whose space_types.slug appears in demote_space_types are
-- ordered last after geo weighting (demoted, not hidden — the member can
-- still discover them). NULL/empty array = no demotion.
SELECT s.id, s.name, s.slug, s.member_count, s.post_count, s.geo_code,
 st.slug AS space_type_slug,
 COUNT(DISTINCT sm2.user_id) AS shared_members
FROM spaces s
JOIN space_members sm ON sm.user_id = $1
JOIN space_members sm2 ON sm2.space_id = s.id
LEFT JOIN space_types st ON st.id = s.space_type_id
WHERE s.id NOT IN (
 SELECT space_id FROM space_members WHERE user_id = $1
)
AND s.deleted_at IS NULL
-- Member-created spaces only — primary (auto-joined geo + platform-seeded)
-- spaces are not a suggestion surface (Section 05 amendment).
AND s.level = 'custom'
AND s.creator_user_id IS NOT NULL
-- §80 wiki-only types (attraction) — never a suggestion surface.
AND NOT st.wiki_only
GROUP BY s.id, s.name, s.slug, s.member_count, s.post_count, s.geo_code, st.slug
ORDER BY (CASE WHEN s.geo_code = sqlc.narg(geo_code) THEN 0 ELSE 1 END),
 (CASE WHEN st.slug = ANY(COALESCE(sqlc.narg(demote_space_types)::text[], '{}'::text[])) THEN 1 ELSE 0 END),
 shared_members DESC
LIMIT $2;

-- name: GetSuggestedTags :many
-- ADR-064: when geoCode is provided, tags from content in the visitor's
-- geo region are preferred (re-weighted, not hidden). Empty geoCode =
-- global ranking (original behavior).
SELECT DISTINCT t.id, t.name, t.slug, t.follower_count,
 (CASE WHEN sp.geo_code = sqlc.narg(geo_code) THEN 0 ELSE 1 END) AS geo_weight
FROM tags t
JOIN contents c ON c.id IN (
 SELECT ct.content_id FROM content_tags ct
 JOIN contents c2 ON c2.id = ct.content_id AND c2.author_id = $1
 AND c2.deleted_at IS NULL AND c2.status = 'published'
)
LEFT JOIN spaces sp ON sp.id = c.geo_space_id
WHERE c.deleted_at IS NULL
 AND c.status = 'published'
AND NOT EXISTS (
 SELECT 1 FROM follows f
 WHERE f.follower_id = $1 AND f.entity_type = 'tag' AND f.entity_id = t.id
)
ORDER BY (CASE WHEN sp.geo_code = sqlc.narg(geo_code) THEN 0 ELSE 1 END), t.follower_count DESC
LIMIT $2;

-- name: GetSuggestedContent :many
-- Section 53: "You might also like" — pgvector cosine similarity between the
-- seed content's relevance_embedding and all other published content.
-- Falls back to trending content ordering when no seed is provided
-- (seed_content_id IS NULL → returns top trending content by score).
-- Params: $1=window, $2=geo_code(narg), $3=seed_content_id, $4=limit
WITH seed AS (
 SELECT cs.relevance_embedding AS emb
 FROM content_scores cs
 WHERE cs.content_id = $3::uuid
)
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
 c.view_count,
 CASE
 WHEN (SELECT emb FROM seed) IS NOT NULL
 THEN 1 - (cs.relevance_embedding <=> (SELECT emb FROM seed))
 ELSE ts.score
 END AS similarity_score,
 ts.score AS trending_score
FROM contents c
JOIN trending_snapshots ts ON ts.entity_id = c.id
 AND ts.entity_type = 'content'
 AND ts."window" = $1
 AND ts.expires_at > now()
 AND ($2::text IS NULL OR $2::text = '' OR ts.geo_code = $2)
LEFT JOIN content_scores cs ON cs.content_id = c.id
WHERE c.status = 'published'
 AND c.visibility = 'public'
 AND c.deleted_at IS NULL
 AND ($3::uuid IS NULL OR c.id != $3::uuid)
 AND ($3::uuid IS NULL OR cs.relevance_embedding IS NOT NULL)
ORDER BY
 CASE
 WHEN (SELECT emb FROM seed) IS NOT NULL
 THEN cs.relevance_embedding <=> (SELECT emb FROM seed)
 ELSE -ts.score
 END
LIMIT $4;
