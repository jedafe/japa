-- Section 32: Activity Feed / Timeline

-- name: InsertFeedItem :exec
INSERT INTO feeds (user_id, content_id, space_id, content_type, score) VALUES (@user_id, @content_id, @space_id, @content_type, @score)

ON CONFLICT (user_id, content_id) DO NOTHING;

-- name: GetExploreFeed :many
-- Trending content for guests (no user_id filter), pull from content_scores.
-- Ranks by the content's best linked geo proximity to the visitor, falling
-- back to velocity and vote score for tie-breaking.
WITH content_geo AS (
 SELECT cs.content_id,
 COALESCE(MAX(CASE
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'city' THEN 5
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'state' THEN 4
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'country' THEN 3
 WHEN COALESCE(cp.region, sp.region, s.region) = @visitor_region THEN 2
 ELSE 1
 END), 1) AS geo_proximity
 FROM content_spaces cs
 JOIN contents c2 ON c2.id = cs.content_id
 AND c2.status = 'published'
 AND c2.deleted_at IS NULL
 AND c2.visibility = 'public'
 JOIN spaces s ON s.id = cs.space_id AND s.level IN ('country', 'state', 'city') AND s.deleted_at IS NULL
 LEFT JOIN spaces sp ON sp.id = s.parent_id AND sp.deleted_at IS NULL
 LEFT JOIN spaces cp ON cp.id = sp.parent_id AND cp.deleted_at IS NULL
 WHERE COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) IS NOT NULL
 AND COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) != ''
 GROUP BY cs.content_id
)
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
 c.created_at, c.view_count, c.comment_count,
 cs.vote_score, cs.velocity_score,
 COALESCE(up.display_name, u.username, '') AS author_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(u.username, '') AS author_username,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 MAX(LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400))::text AS excerpt,
 cs.bookmark_count,
 COALESCE(cs.reaction_count, 0) AS reaction_count,
 c.read_time_minutes,
 c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.guide_category,
 c.organization_id,
 o.name AS organization_name,
 o.slug AS organization_slug,
 om.storage_bucket AS organization_logo_storage_bucket,
 om.storage_path AS organization_logo_storage_path,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 vs.source_url AS video_source_url,
 vs.source_type AS video_source_type,
 vs.duration_secs AS video_duration_secs,
 vs.thumbnail_url AS video_thumbnail_url,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 COALESCE(string_agg(DISTINCT t.name, ',' ORDER BY t.name), '') AS tags,
 COALESCE(MAX(cg.geo_proximity), 1) AS geo_proximity
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN video_sources vs ON vs.id = c.video_source_id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id, vs.thumbnail_media_id) AND m.deleted_at IS NULL
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
LEFT JOIN content_geo cg ON cg.content_id = c.id
WHERE c.status = 'published'
 AND c.deleted_at IS NULL
 AND c.visibility = 'public'
 -- §106: synthetic video channels are excluded from discovery feeds so
 -- imported channel content does not surface as normal user activity.
 -- COALESCE keeps rows whose author join produced no users row.
 AND COALESCE(u.is_synthetic, false) = false
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count, cs.reaction_count,
 u.username, up.display_name, up.avatar_url,
 s.name, s.slug,
 m.storage_bucket, m.storage_path,
 o.name, o.slug, om.storage_bucket, om.storage_path,
 sm.storage_bucket, sm.storage_path,
 vs.source_url, vs.source_type, vs.duration_secs, vs.thumbnail_url
ORDER BY COALESCE(MAX(cg.geo_proximity), 1) DESC, cs.velocity_score DESC, cs.reaction_count DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetPrimaryFeed :many
-- User's personalized feed from feeds table (push side).
-- sort: 'latest' (default), 'top' (vote_score DESC), 'rising' (velocity_score DESC).
-- 'relevant' falls back to latest (pgvector relevance is only used in the for-you feed).
SELECT f.id, f.content_id, f.space_id, f.content_type, f.score, f.created_at,
 c.title, c.slug, c.author_id, c.created_at AS content_created_at,
 c.view_count, c.comment_count,
 cs.vote_score, cs.velocity_score, cs.bookmark_count,
 COALESCE(cs.reaction_count, 0) AS reaction_count,
 COALESCE(up.display_name, u.username, '') AS author_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(u.username, '') AS author_username,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 MAX(LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400))::text AS excerpt,
 c.read_time_minutes,
 c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.guide_category,
 c.organization_id,
 o.name AS organization_name,
 o.slug AS organization_slug,
 om.storage_bucket AS organization_logo_storage_bucket,
 om.storage_path AS organization_logo_storage_path,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 vs.source_url AS video_source_url,
 vs.source_type AS video_source_type,
 vs.duration_secs AS video_duration_secs,
 vs.thumbnail_url AS video_thumbnail_url,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 COALESCE(string_agg(DISTINCT t.name, ',' ORDER BY t.name), '') AS tags
FROM feeds f
JOIN contents c ON c.id = f.content_id
LEFT JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN video_sources vs ON vs.id = c.video_source_id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id, vs.thumbnail_media_id) AND m.deleted_at IS NULL
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
WHERE f.user_id = @user_id
 AND c.status = 'published'
 AND c.deleted_at IS NULL
 AND NOT EXISTS (
 SELECT 1 FROM user_blocks ub
 WHERE ((ub.blocker_id = @user_id AND ub.blocked_id = c.author_id)
 OR (ub.blocker_id = c.author_id AND ub.blocked_id = @user_id))
 )
GROUP BY f.id, f.content_id, f.space_id, f.content_type, f.score, f.created_at,
 c.id, c.title, c.slug, c.author_id, c.created_at,
 c.view_count, c.comment_count, cs.vote_score, cs.velocity_score, cs.bookmark_count, cs.reaction_count,
 u.username, up.display_name, up.avatar_url,
 s.name, s.slug,
 c.read_time_minutes, m.storage_bucket, m.storage_path,
 c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.guide_category,
 o.name, o.slug, om.storage_bucket, om.storage_path,
 sm.storage_bucket, sm.storage_path,
 vs.source_url, vs.source_type, vs.duration_secs, vs.thumbnail_url
ORDER BY
 CASE WHEN @sort = 'top' THEN cs.reaction_count END DESC,
 CASE WHEN @sort = 'rising' THEN cs.velocity_score END DESC,
 f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetSpaceFeed :many
-- Posts in a specific space.
-- sort: 'latest' (default), 'top' (vote_score DESC), 'rising' (velocity_score DESC).
-- 'relevant' falls back to latest (pgvector relevance is only used in the for-you feed).
-- @viewer_id is nullable — when present, mutual user_blocks are excluded (§41).
SELECT c.id, c.title, c.slug, c.content_type, c.author_id,
 c.created_at, c.view_count, c.comment_count,
 cs.vote_score, cs.velocity_score,
 cs.bookmark_count,
 COALESCE(cs.reaction_count, 0) AS reaction_count,
 COALESCE(up.display_name, u.username, '') AS author_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(u.username, '') AS author_username,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 MAX(LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400))::text AS excerpt,
 c.read_time_minutes,
 c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.guide_category,
 c.organization_id,
 o.name AS organization_name,
 o.slug AS organization_slug,
 om.storage_bucket AS organization_logo_storage_bucket,
 om.storage_path AS organization_logo_storage_path,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 vs.source_url AS video_source_url,
 vs.source_type AS video_source_type,
 vs.duration_secs AS video_duration_secs,
 vs.thumbnail_url AS video_thumbnail_url,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 COALESCE(string_agg(DISTINCT t.name, ',' ORDER BY t.name), '') AS tags
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN video_sources vs ON vs.id = c.video_source_id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id, vs.thumbnail_media_id) AND m.deleted_at IS NULL
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
WHERE c.space_id = @space_id
 AND c.status = 'published'
 AND c.deleted_at IS NULL
 AND NOT (
 sqlc.narg(viewer_id)::uuid IS NOT NULL AND EXISTS (
 SELECT 1 FROM user_blocks ub
 WHERE ((ub.blocker_id = sqlc.narg(viewer_id)::uuid AND ub.blocked_id = c.author_id)
 OR (ub.blocker_id = c.author_id AND ub.blocked_id = sqlc.narg(viewer_id)::uuid))
 )
 )
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count, cs.reaction_count,
 u.username, up.display_name, up.avatar_url,
 s.name, s.slug,
 m.storage_bucket, m.storage_path,
 o.name, o.slug, om.storage_bucket, om.storage_path,
 sm.storage_bucket, sm.storage_path,
 vs.source_url, vs.source_type, vs.duration_secs, vs.thumbnail_url
ORDER BY
 CASE WHEN @sort = 'top' THEN cs.reaction_count END DESC,
 CASE WHEN @sort = 'rising' THEN cs.velocity_score END DESC,
 c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetProfileFeed :many
-- All public posts by a specific user.
-- sort: 'latest' (default), 'top' (vote_score DESC), 'rising' (velocity_score DESC).
-- 'relevant' falls back to latest (pgvector relevance is only used in the for-you feed).
-- @viewer_id is nullable — when present, mutual user_blocks are excluded (§41).
-- If the viewer has blocked the profile owner (or vice versa), the profile
-- feed returns no rows, enforcing mutual invisibility.
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
 c.created_at, c.view_count, c.comment_count,
 cs.vote_score, cs.velocity_score,
 cs.bookmark_count,
 COALESCE(cs.reaction_count, 0) AS reaction_count,
 COALESCE(up.display_name, u.username, '') AS author_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(u.username, '') AS author_username,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 MAX(LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400))::text AS excerpt,
 c.read_time_minutes,
 c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.guide_category,
 c.organization_id,
 o.name AS organization_name,
 o.slug AS organization_slug,
 om.storage_bucket AS organization_logo_storage_bucket,
 om.storage_path AS organization_logo_storage_path,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 vs.source_url AS video_source_url,
 vs.source_type AS video_source_type,
 vs.duration_secs AS video_duration_secs,
 vs.thumbnail_url AS video_thumbnail_url,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 COALESCE(string_agg(DISTINCT t.name, ',' ORDER BY t.name), '') AS tags
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN video_sources vs ON vs.id = c.video_source_id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id, vs.thumbnail_media_id) AND m.deleted_at IS NULL
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
WHERE c.author_id = @author_id
 AND c.status = 'published'
 AND c.deleted_at IS NULL
 AND c.visibility = 'public'
 AND NOT (
 sqlc.narg(viewer_id)::uuid IS NOT NULL AND EXISTS (
 SELECT 1 FROM user_blocks ub
 WHERE ((ub.blocker_id = sqlc.narg(viewer_id)::uuid AND ub.blocked_id = c.author_id)
 OR (ub.blocker_id = c.author_id AND ub.blocked_id = sqlc.narg(viewer_id)::uuid))
 )
 )
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count, cs.reaction_count,
 u.username, up.display_name, up.avatar_url,
 s.name, s.slug,
 m.storage_bucket, m.storage_path,
 o.name, o.slug, om.storage_bucket, om.storage_path,
 sm.storage_bucket, sm.storage_path,
 vs.source_url, vs.source_type, vs.duration_secs, vs.thumbnail_url
ORDER BY
 CASE WHEN @sort = 'top' THEN cs.reaction_count END DESC,
 CASE WHEN @sort = 'rising' THEN cs.velocity_score END DESC,
 c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetTopFeed :many
-- Top content in 7-day rolling window (for top sort), ranked by geo proximity.
WITH content_geo AS (
 SELECT cs.content_id,
 COALESCE(MAX(CASE
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'city' THEN 5
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'state' THEN 4
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'country' THEN 3
 WHEN COALESCE(cp.region, sp.region, s.region) = @visitor_region THEN 2
 ELSE 1
 END), 1) AS geo_proximity
 FROM content_spaces cs
 JOIN contents c2 ON c2.id = cs.content_id
 AND c2.status = 'published'
 AND c2.deleted_at IS NULL
 AND c2.visibility = 'public'
 JOIN spaces s ON s.id = cs.space_id AND s.level IN ('country', 'state', 'city') AND s.deleted_at IS NULL
 LEFT JOIN spaces sp ON sp.id = s.parent_id AND sp.deleted_at IS NULL
 LEFT JOIN spaces cp ON cp.id = sp.parent_id AND cp.deleted_at IS NULL
 WHERE COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) IS NOT NULL
 AND COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) != ''
 GROUP BY cs.content_id
)
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
 c.created_at, c.view_count, c.comment_count,
 cs.vote_score, cs.velocity_score,
 cs.bookmark_count,
 COALESCE(cs.reaction_count, 0) AS reaction_count,
 COALESCE(up.display_name, u.username, '') AS author_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(u.username, '') AS author_username,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 MAX(LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400))::text AS excerpt,
 c.read_time_minutes,
 c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.guide_category,
 c.organization_id,
 o.name AS organization_name,
 o.slug AS organization_slug,
 om.storage_bucket AS organization_logo_storage_bucket,
 om.storage_path AS organization_logo_storage_path,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 vs.source_url AS video_source_url,
 vs.source_type AS video_source_type,
 vs.duration_secs AS video_duration_secs,
 vs.thumbnail_url AS video_thumbnail_url,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 COALESCE(string_agg(DISTINCT t.name, ',' ORDER BY t.name), '') AS tags,
 COALESCE(MAX(cg.geo_proximity), 1) AS geo_proximity
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN video_sources vs ON vs.id = c.video_source_id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id, vs.thumbnail_media_id) AND m.deleted_at IS NULL
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
LEFT JOIN content_geo cg ON cg.content_id = c.id
WHERE c.status = 'published'
 AND c.deleted_at IS NULL
 AND c.visibility = 'public'
 AND c.created_at > now() - interval '7 days'
 -- §106: exclude synthetic video channel content from discovery feeds.
 AND COALESCE(u.is_synthetic, false) = false
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count, cs.reaction_count,
 u.username, up.display_name, up.avatar_url,
 s.name, s.slug,
 m.storage_bucket, m.storage_path,
 o.name, o.slug, om.storage_bucket, om.storage_path,
 sm.storage_bucket, sm.storage_path,
 vs.source_url, vs.source_type, vs.duration_secs, vs.thumbnail_url
ORDER BY COALESCE(MAX(cg.geo_proximity), 1) DESC, cs.reaction_count DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetRisingFeed :many
-- Rising content sorted by velocity score, ranked by geo proximity.
WITH content_geo AS (
 SELECT cs.content_id,
 COALESCE(MAX(CASE
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'city' THEN 5
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'state' THEN 4
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'country' THEN 3
 WHEN COALESCE(cp.region, sp.region, s.region) = @visitor_region THEN 2
 ELSE 1
 END), 1) AS geo_proximity
 FROM content_spaces cs
 JOIN contents c2 ON c2.id = cs.content_id
 AND c2.status = 'published'
 AND c2.deleted_at IS NULL
 AND c2.visibility = 'public'
 JOIN spaces s ON s.id = cs.space_id AND s.level IN ('country', 'state', 'city') AND s.deleted_at IS NULL
 LEFT JOIN spaces sp ON sp.id = s.parent_id AND sp.deleted_at IS NULL
 LEFT JOIN spaces cp ON cp.id = sp.parent_id AND cp.deleted_at IS NULL
 WHERE COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) IS NOT NULL
 AND COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) != ''
 GROUP BY cs.content_id
)
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
 c.created_at, c.view_count, c.comment_count,
 cs.vote_score, cs.velocity_score,
 cs.bookmark_count,
 COALESCE(cs.reaction_count, 0) AS reaction_count,
 COALESCE(up.display_name, u.username, '') AS author_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(u.username, '') AS author_username,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 MAX(LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400))::text AS excerpt,
 c.read_time_minutes,
 c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.guide_category,
 c.organization_id,
 o.name AS organization_name,
 o.slug AS organization_slug,
 om.storage_bucket AS organization_logo_storage_bucket,
 om.storage_path AS organization_logo_storage_path,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 vs.source_url AS video_source_url,
 vs.source_type AS video_source_type,
 vs.duration_secs AS video_duration_secs,
 vs.thumbnail_url AS video_thumbnail_url,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 COALESCE(string_agg(DISTINCT t.name, ',' ORDER BY t.name), '') AS tags,
 COALESCE(MAX(cg.geo_proximity), 1) AS geo_proximity
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN video_sources vs ON vs.id = c.video_source_id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id, vs.thumbnail_media_id) AND m.deleted_at IS NULL
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
LEFT JOIN content_geo cg ON cg.content_id = c.id
WHERE c.status = 'published'
 AND c.deleted_at IS NULL
 AND c.visibility = 'public'
 -- §106: exclude synthetic video channel content from discovery feeds.
 AND COALESCE(u.is_synthetic, false) = false
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count, cs.reaction_count,
 u.username, up.display_name, up.avatar_url,
 s.name, s.slug,
 m.storage_bucket, m.storage_path,
 o.name, o.slug, om.storage_bucket, om.storage_path,
 sm.storage_bucket, sm.storage_path,
 vs.source_url, vs.source_type, vs.duration_secs, vs.thumbnail_url
ORDER BY COALESCE(MAX(cg.geo_proximity), 1) DESC, cs.velocity_score DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateContentScore :exec
-- Update a specific score column on content_scores
UPDATE content_scores
SET vote_score = @vote_score,
 comment_count = @comment_count,
 view_count = @view_count,
 bookmark_count = @bookmark_count,
 reaction_count = @reaction_count,
 last_scored_at = now()
WHERE content_id = @content_id;

-- name: UpdateVelocityScores :exec
-- Batch update velocity scores for published content < @window_days days old.
-- velocity = (reaction_count * 2 + comment_count + view_count / 100) /
-- (hours_since_publish + 2 ^ gravity) — the +2 smoothing prevents fresh posts
-- from exploding to near-infinite velocity (Section 32 formula).
-- gravity is passed as a parameter so it can be hot-reloaded from
-- system_settings (feed.velocity.gravity, default 1.8); the window comes from
-- feed.velocity_window_days (default 30).
-- Windowing uses published_at (not created_at) so long-lived drafts and
-- scheduled posts are scored from when they actually went live.
UPDATE content_scores cs
SET velocity_score = CASE
 WHEN c.published_at IS NOT NULL AND EXTRACT(EPOCH FROM (now() - c.published_at)) > 0
 THEN GREATEST(0, (cs.reaction_count * 2 + cs.comment_count + cs.view_count / 100.0)::numeric
 / POWER(EXTRACT(EPOCH FROM (now() - c.published_at)) / 3600.0 + 2, @gravity))
 ELSE 0
END,
last_scored_at = now()
FROM contents c
WHERE cs.content_id = c.id
 AND c.status = 'published'
 AND c.deleted_at IS NULL
 AND c.published_at >= now() - make_interval(days => @window_days::int);

-- name: DeleteFeedItemsByContent :exec
DELETE FROM feeds WHERE content_id = @content_id;

-- name: DeleteFeedItemsByUser :exec
DELETE FROM feeds WHERE user_id = @user_id;

-- name: GetFeedItemCount :one
SELECT COUNT(*)::int AS count FROM feeds WHERE user_id = @user_id;

-- name: GetFollowedEntityIDsByType :many
-- Lean ID-only lookup of everything a user follows of one entity type.
-- Ordered newest-first so truncation (feed.max_followed_spaces_in_feed)
-- keeps the most recent follows rather than an arbitrary subset (ALG-018).
SELECT entity_id
FROM follows
WHERE follower_id = @follower_id
 AND entity_type = @entity_type
ORDER BY created_at DESC;

-- name: GetMemberSpaceIDs :many
-- Space IDs the user is a member of (affinity signal, distinct from follows).
SELECT space_id
FROM space_members
WHERE user_id = @user_id;

-- name: GetForYouCandidates :many
-- Bounded personalized candidate pool = UNION of (a) recent content from
-- followed authors/spaces/tags and (b) top recent content by velocity.
-- Emits per-row affinity flags for Go-side ranking. Excludes own posts.
-- Array params are COALESCEd to empty arrays so `= ANY(NULL)` never nullifies
-- the affinity predicate when a user follows nothing of a given type.
WITH affinity AS (
 SELECT c.id
 FROM contents c
 WHERE c.status = 'published'
 AND c.deleted_at IS NULL
 AND c.visibility = 'public'
 AND c.created_at >= @window_start
 AND c.author_id <> @exclude_author_id
 AND NOT EXISTS (
 SELECT 1 FROM user_blocks ub
 WHERE ((ub.blocker_id = @exclude_author_id AND ub.blocked_id = c.author_id)
 OR (ub.blocker_id = c.author_id AND ub.blocked_id = @exclude_author_id))
 )
 AND (
 c.author_id = ANY(COALESCE(@author_ids::uuid[], ARRAY[]::uuid[]))
 OR c.space_id = ANY(COALESCE(@space_ids::uuid[], ARRAY[]::uuid[]))
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
 WHERE c.status = 'published'
 AND c.deleted_at IS NULL
 AND c.visibility = 'public'
 AND c.created_at >= @window_start
 AND c.author_id <> @exclude_author_id
 AND NOT EXISTS (
 SELECT 1 FROM user_blocks ub
 WHERE ((ub.blocker_id = @exclude_author_id AND ub.blocked_id = c.author_id)
 OR (ub.blocker_id = c.author_id AND ub.blocked_id = @exclude_author_id))
 )
 -- §106: the velocity (global discovery) branch excludes synthetic
 -- video channel content. The affinity branch above is left
 -- unfiltered so channels a user explicitly follows still surface.
 AND NOT EXISTS (
 SELECT 1 FROM users su
 WHERE su.id = c.author_id AND su.is_synthetic = true
 )
 ORDER BY cs.velocity_score DESC
 LIMIT @velocity_limit
),
candidate_ids AS (
 SELECT id FROM affinity
 UNION
 SELECT id FROM velocity
),
content_geo AS (
 SELECT cs.content_id,
 COALESCE(MAX(CASE
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'city' THEN 5
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'state' THEN 4
 WHEN COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = @visitor_country_code AND s.level = 'country' THEN 3
 WHEN COALESCE(cp.region, sp.region, s.region) = @visitor_region THEN 2
 ELSE 1
 END), 1) AS geo_proximity
 FROM content_spaces cs
 JOIN candidate_ids ci ON ci.id = cs.content_id
 JOIN spaces s ON s.id = cs.space_id AND s.level IN ('country', 'state', 'city') AND s.deleted_at IS NULL
 LEFT JOIN spaces sp ON sp.id = s.parent_id AND sp.deleted_at IS NULL
 LEFT JOIN spaces cp ON cp.id = sp.parent_id AND cp.deleted_at IS NULL
 WHERE COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) IS NOT NULL
 AND COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) != ''
 GROUP BY cs.content_id
)
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
 c.created_at, c.view_count, c.comment_count,
 cs.vote_score, cs.velocity_score, cs.bookmark_count,
 COALESCE(cs.reaction_count, 0) AS reaction_count,
 COALESCE(up.display_name, u.username, '') AS author_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(u.username, '') AS author_username,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 MAX(LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400))::text AS excerpt,
 c.read_time_minutes,
 c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.organization_id,
 o.name AS organization_name,
 o.slug AS organization_slug,
 om.storage_bucket AS organization_logo_storage_bucket,
 om.storage_path AS organization_logo_storage_path,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 vs.source_url AS video_source_url,
 vs.source_type AS video_source_type,
 vs.duration_secs AS video_duration_secs,
 vs.thumbnail_url AS video_thumbnail_url,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 COALESCE(string_agg(DISTINCT t.name, ',' ORDER BY t.name), '') AS tags,
 COALESCE(c.author_id = ANY(COALESCE(@author_ids::uuid[], ARRAY[]::uuid[])), false)::bool AS is_author_followed,
 COALESCE(c.space_id = ANY(COALESCE(@space_ids::uuid[], ARRAY[]::uuid[])), false)::bool AS is_space_followed,
 COUNT(DISTINCT t.id) FILTER (
 WHERE t.id = ANY(COALESCE(@tag_ids::uuid[], ARRAY[]::uuid[]))
 )::int AS followed_tag_overlap,
 s.geo_code AS space_geo_code,
 COALESCE(cg.geo_proximity, 1) AS geo_proximity
FROM contents c
JOIN candidate_ids ci ON ci.id = c.id
LEFT JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN video_sources vs ON vs.id = c.video_source_id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id, vs.thumbnail_media_id) AND m.deleted_at IS NULL
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
LEFT JOIN content_geo cg ON cg.content_id = c.id
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count, cs.reaction_count,
 u.username, up.display_name, up.avatar_url,
 s.name, s.slug, s.geo_code,
 c.journey_stage, c.visa_route, c.source_country, c.target_country,
 m.storage_bucket, m.storage_path,
 cg.geo_proximity,
 o.name, o.slug, om.storage_bucket, om.storage_path,
 sm.storage_bucket, sm.storage_path,
 vs.source_url, vs.source_type, vs.duration_secs, vs.thumbnail_url
LIMIT @candidate_cap;

-- name: ListDigestFeedItems :many
SELECT c.id, c.title, c.slug, c.content_type
FROM feeds f
JOIN contents c ON c.id = f.content_id
WHERE f.user_id = @user_id
 AND f.created_at >= now() - interval '7 days'
ORDER BY f.created_at DESC
LIMIT 20;

-- name: PruneOldFeeds :exec
-- Intentionally global (no the id filter): this is a retention prune that
-- runs weekly across the platform. Ref: Section 32, asynq-tasks catalog.
DELETE FROM feeds WHERE created_at < now() - interval '30 days';

-- name: ListSourcesByContentIDs :many
-- Batch source attribution lookup for feed items. Returns one row per content
-- that has an intel_source_id, so callers can attach a source badge to feed
-- cards without a per-card query.
-- Ref: docs/superpowers/specs/2026-09-11-sources-directory-design.md
SELECT c.id AS content_id,
 src.id AS source_id,
 src.name AS source_name,
 src.slug AS source_slug,
 src.logo_url AS source_logo_url,
 src.source_url AS source_source_url,
 src.trust_level AS source_trust_level
FROM contents c
JOIN intel_sources src ON src.id = c.intel_source_id
WHERE c.id = ANY(@content_ids::uuid[]);
