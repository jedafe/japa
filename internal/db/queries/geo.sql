-- Geo hierarchy queries: space parent relationships and geo space lookups.

-- =====================
-- Space Parent Spaces
-- =====================

-- name: GetSpaceParentSpaces :many
SELECT sps.parent_space_id, s.name AS parent_name, s.slug AS parent_slug,
 s.level AS parent_level, s.geo_code AS parent_geo_code
FROM space_parent_spaces sps
JOIN spaces s ON s.id = sps.parent_space_id AND s.tenant_id = sps.space_id
WHERE sps.space_id = @space_id
ORDER BY s.level ASC;

-- name: GetSpaceChildSpaces :many
SELECT sps.space_id, s.name AS child_name, s.slug AS child_slug,
 s.level AS child_level, s.geo_code AS child_geo_code
FROM space_parent_spaces sps
JOIN spaces s ON s.id = sps.space_id AND s.tenant_id = sps.parent_space_id
WHERE sps.parent_space_id = @parent_space_id
ORDER BY s.name ASC;

-- name: AddSpaceParentSpace :exec
INSERT INTO space_parent_spaces (space_id, parent_space_id) VALUES (@space_id, @parent_space_id)

ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- name: RemoveSpaceParentSpace :exec
DELETE FROM space_parent_spaces
WHERE space_id = @space_id AND parent_space_id = @parent_space_id;

-- name: RemoveAllSpaceParentSpaces :exec
DELETE FROM space_parent_spaces
WHERE space_id = @space_id;

-- =====================
-- Geo Space Lookups
-- =====================

-- name: ListCountries :many
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at
FROM spaces
WHERE level = 'country'
 AND status = 'active'
 AND deleted_at IS NULL
ORDER BY name ASC;

-- name: ListStatesByCountry :many
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
 s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
 s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
 s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at, s.updated_at,
 s.deleted_at
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE sps.parent_space_id = @country_space_id
 AND s.level = 'state'
 AND s.deleted_at IS NULL
ORDER BY s.name ASC;

-- name: GetGeoHierarchy :many
WITH RECURSIVE hierarchy AS (
 SELECT s.id, s.name, s.slug, s.level, s.geo_code, s.parent_id,
 0 AS depth
 FROM spaces s
 WHERE s.id = @space_id

 UNION ALL

 SELECT s.id, s.name, s.slug, s.level, s.geo_code, s.parent_id,
 h.depth + 1
 FROM spaces s
 JOIN space_parent_spaces sps ON sps.parent_space_id = s.id
 JOIN hierarchy h ON h.id = sps.space_id
 WHERE h.depth < 5
)
SELECT id, name, slug, level, geo_code, depth
FROM hierarchy
ORDER BY depth DESC;

-- name: SearchGeoSpaces :many
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at
FROM spaces
WHERE level IN ('country', 'state', 'city')
 AND status = 'active'
 AND deleted_at IS NULL
 AND name ILIKE '%' || @query || '%'
ORDER BY
 CASE WHEN name ILIKE @query || '%' THEN 0 ELSE 1 END,
 name ASC
LIMIT @limit_count;
