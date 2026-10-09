-- name: ListPermissionPresets :many
SELECT id, name, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, is_system, created_at, updated_at
FROM space_permission_presets

ORDER BY created_at ASC, id ASC;

-- name: GetPermissionPresetByName :one
SELECT id, name, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, is_system, created_at, updated_at
FROM space_permission_presets
WHERE name = @name;

-- name: UpdatePermissionPreset :one
UPDATE space_permission_presets
SET visibility = @visibility, posting_perm = @posting_perm, replying_perm = @replying_perm,
 commenting_perm = @commenting_perm, joining_perm = @joining_perm
WHERE id = @id AND is_system = true
RETURNING id, name, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, is_system, created_at, updated_at;

-- =====================
-- Space Types
-- =====================

-- name: ListSpaceTypes :many
SELECT id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, category, wiki_only
FROM space_types

ORDER BY created_at ASC, id ASC;

-- name: ListSpaceTypesFiltered :many
-- Supports optional search (q), category filter, and a selectable-only flag
-- for the admin panel and the member space-creation flow. Pass empty strings
-- and false booleans to skip filters.
SELECT id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, category, wiki_only
FROM space_types
WHERE (@search::text = '' OR name ILIKE '%' || @search || '%' OR slug ILIKE '%' || @search || '%')
 AND (@category_filter::text = '' OR category = @category_filter)
 AND (@selectable_only::boolean = false OR category IS NOT NULL)
ORDER BY created_at ASC, id ASC;

-- name: GetSpaceTypeByID :one
SELECT id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, category, wiki_only
FROM space_types
WHERE id = @id;

-- name: GetSpaceTypeBySlug :one
SELECT id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, category, wiki_only
FROM space_types
WHERE slug = @slug;

-- name: GetSpaceTypesByIDs :many
-- Batch type lookup for list serializers that need per-row wiki_only/type
-- fields without N+1 queries.
SELECT id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, category, wiki_only
FROM space_types
WHERE id = ANY(@ids::uuid[]);

-- name: CreateSpaceType :one
INSERT INTO space_types (name, slug, description, icon, field_group_id, category, is_system) VALUES (@name, @slug, sqlc.narg(description), sqlc.narg(icon), sqlc.narg(field_group_id), sqlc.narg(category), @is_system)

RETURNING id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, category, wiki_only;

-- name: UpdateSpaceType :one
UPDATE space_types
SET name = @name, description = sqlc.narg(description), icon = sqlc.narg(icon),
 field_group_id = sqlc.narg(field_group_id), category = sqlc.narg(category),
 wiki_only = sqlc.narg(wiki_only)
WHERE id = @id AND is_system = false
RETURNING id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, category, wiki_only;

-- name: DeleteSpaceType :exec
DELETE FROM space_types
WHERE id = @id AND is_system = false;

-- =====================
-- Spaces
-- =====================

-- name: ListSpaces :many
-- Public discovery list. Filters by visibility so unauthenticated callers
-- only see public spaces; authenticated callers also see members/private
-- spaces they belong to (Section 05 line 272, §9 platform scoping).
-- Pass @viewer_user_id as NULL for unauthenticated requests.
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
 s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
 s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
 s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
 s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
WHERE s.deleted_at IS NULL
 AND (
 s.visibility = 'public'
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid AND sm.status = 'approved'
 )
 )
 )
ORDER BY s.created_at DESC, s.id DESC;

-- name: ListSpacesPaginated :many
-- SPACES-009/011: SQL-level paginated discovery list with search, geo,
-- level, region, subregion, space_type, joined-only, and sort support.
-- Replaces the in-memory filter+paginate in the ListSpaces handler.
-- Visibility filter matches ListSpaces.
-- sort accepts: '' (default → newest), 'name' (A–Z), 'members', 'followers',
-- 'posts' (all numeric DESC), 'activity' (most recently updated). Unknown
-- values fall back to newest. §05 line 319: sort=members|activity|newest.
-- @joined_only restricts the list to spaces the viewer is an approved
-- member of (the /spaces "My spaces" chip — Rule 16: server-side filter so
-- pagination stays correct). @space_type_id filters by space type.
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
 s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
 s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
 s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
 s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
WHERE s.deleted_at IS NULL
 AND (
 s.visibility = 'public'
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid AND sm.status = 'approved'
 )
 )
 )
 AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
 AND (@geo_code::text = '' OR s.geo_code = @geo_code)
 AND (@level::text = '' OR s.level = @level)
 AND (@region::text = '' OR s.region = @region)
 AND (@subregion::text = '' OR s.subregion = @subregion)
 AND (@space_type_id::uuid IS NULL OR s.space_type_id = @space_type_id::uuid)
 -- §80 wiki-only types (attraction) are browsable through the wiki
 -- directory only — never the member-facing /spaces listing.
 AND NOT EXISTS (
 SELECT 1 FROM space_types st WHERE st.id = s.space_type_id AND st.wiki_only
 )
 AND (
 @joined_only::bool = false
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members smj
 WHERE smj.space_id = s.id AND smj.user_id = @viewer_user_id::uuid AND smj.status = 'approved'
 )
 )
 )
ORDER BY
 CASE WHEN @sort::text = 'name' THEN s.name ELSE NULL END ASC,
 CASE
 WHEN @sort::text = 'members' THEN s.member_count
 WHEN @sort::text = 'followers' THEN s.follower_count
 WHEN @sort::text = 'posts' THEN s.post_count
 ELSE NULL
 END DESC,
 CASE WHEN @sort::text = 'activity' THEN s.updated_at ELSE NULL END DESC,
 s.created_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountSpaces :one
-- SPACES-009: total count for the paginated discovery list.
SELECT COUNT(*) AS total
FROM spaces s
WHERE s.deleted_at IS NULL
 AND (
 s.visibility = 'public'
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid AND sm.status = 'approved'
 )
 )
 )
 AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
 AND (@geo_code::text = '' OR s.geo_code = @geo_code)
 AND (@level::text = '' OR s.level = @level)
 AND (@region::text = '' OR s.region = @region)
 AND (@subregion::text = '' OR s.subregion = @subregion)
 AND (@space_type_id::uuid IS NULL OR s.space_type_id = @space_type_id::uuid)
 -- §80 wiki-only types (attraction) are browsable through the wiki
 -- directory only — never the member-facing /spaces listing.
 AND NOT EXISTS (
 SELECT 1 FROM space_types st WHERE st.id = s.space_type_id AND st.wiki_only
 )
 AND (
 @joined_only::bool = false
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members smj
 WHERE smj.space_id = s.id AND smj.user_id = @viewer_user_id::uuid AND smj.status = 'approved'
 )
 )
 );

-- name: ListSpacesByLevel :many
-- SPACES-001: apply the same visibility filter as ListSpaces so
-- unauthenticated callers only see public spaces; authenticated callers
-- also see members/private spaces they belong to (Section 05 line 272).
-- Pass @viewer_user_id as NULL for unauthenticated requests.
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code
FROM spaces
WHERE spaces.level = @level AND deleted_at IS NULL
 AND (
 visibility = 'public'
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = spaces.id AND sm.user_id = @viewer_user_id::uuid AND sm.status = 'approved'
 )
 )
 )
ORDER BY created_at DESC, id DESC;

-- name: ListSpacesByType :many
-- SPACES-001: apply the same visibility filter as ListSpaces.
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code
FROM spaces
WHERE spaces.space_type_id = @space_type_id AND spaces.deleted_at IS NULL
 AND (
 visibility = 'public'
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = spaces.id AND sm.user_id = @viewer_user_id::uuid AND sm.status = 'approved'
 )
 )
 )
ORDER BY created_at DESC, id DESC;

-- name: ListSpacesByTypePaginated :many
-- sort accepts: '' (default → newest), 'name' (A–Z), 'members', 'followers',
-- 'posts' (all numeric DESC). Unknown values fall back to newest.
-- The initials column comes from the university_fields custom field group
-- (key = 'initials', deterministic id 00000000-0000-0000-0000-000000000090)
-- and is left-joined so it is NULL for non-university space types.
-- Ref: japaconnect-engineering/sections/39-space-types-university.md
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description, s.wiki_content,
 s.logo_media_id, s.header_image_id, s.geo_code, s.level, s.member_count, s.post_count,
 s.follower_count, s.visibility, s.posting_perm, s.replying_perm, s.commenting_perm,
 s.joining_perm, s.status, s.created_at, s.updated_at, s.deleted_at, s.currency_code,
 cfv.value_text AS initials
FROM spaces s
LEFT JOIN custom_field_values cfv ON cfv.entity_id = s.id
 AND cfv.entity_type = 'space'
 AND cfv.field_id = '00000000-0000-0000-0000-000000000090'::uuid
WHERE s.space_type_id = @space_type_id AND s.deleted_at IS NULL
 AND (@search::text = ''
 OR s.name ILIKE '%' || @search || '%'
 OR s.slug ILIKE '%' || @search || '%'
 OR cfv.value_text ILIKE '%' || @search || '%')
 AND (@geo_code::text = '' OR s.geo_code = @geo_code)
 AND (@region::text = '' OR s.region = @region)
 AND (@subregion::text = '' OR s.subregion = @subregion)
 -- @within is a primary-space slug (country/state/city): match when the
 -- space is linked — via the space_parent_spaces junction or spaces.parent_id
 -- — to that space or any of its descendants (e.g. a state slug also matches
 -- spaces linked to one of its cities). The level guard keeps non-geo slugs
 -- from matching, and deleted ancestors can't satisfy the filter.
 AND (@within::text = '' OR EXISTS (
 SELECT 1
 FROM (
 SELECT parent_space_id AS pid FROM space_parent_spaces WHERE space_id = s.id
 UNION ALL
 SELECT s.parent_id WHERE s.parent_id IS NOT NULL
 ) links
 JOIN spaces g ON g.id = links.pid AND g.deleted_at IS NULL
 LEFT JOIN spaces gp ON gp.id = g.parent_id AND gp.deleted_at IS NULL
 LEFT JOIN spaces ggp ON ggp.id = gp.parent_id AND ggp.deleted_at IS NULL
 WHERE (g.slug = @within AND g.level IN ('country', 'state', 'city'))
 OR (gp.slug = @within AND gp.level IN ('country', 'state', 'city'))
 OR (ggp.slug = @within AND ggp.level IN ('country', 'state', 'city'))
 ))
ORDER BY
 CASE WHEN @sort::text = 'name' THEN s.name ELSE NULL END ASC,
 CASE
 WHEN @sort::text = 'members' THEN s.member_count
 WHEN @sort::text = 'followers' THEN s.follower_count
 WHEN @sort::text = 'posts' THEN s.post_count
 ELSE NULL
 END DESC,
 s.created_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountSpacesByType :one
-- Counts spaces with the same filters as ListSpacesByTypePaginated, including
-- searching the initials custom field for university spaces.
-- Ref: japaconnect-engineering/sections/39-space-types-university.md
SELECT COUNT(*) AS total
FROM spaces s
LEFT JOIN custom_field_values cfv ON cfv.entity_id = s.id
 AND cfv.entity_type = 'space'
 AND cfv.field_id = '00000000-0000-0000-0000-000000000090'::uuid
WHERE s.space_type_id = @space_type_id AND s.deleted_at IS NULL
 AND (@search::text = ''
 OR s.name ILIKE '%' || @search || '%'
 OR s.slug ILIKE '%' || @search || '%'
 OR cfv.value_text ILIKE '%' || @search || '%')
 AND (@geo_code::text = '' OR s.geo_code = @geo_code)
 AND (@region::text = '' OR s.region = @region)
 AND (@subregion::text = '' OR s.subregion = @subregion)
 -- @within mirrors ListSpacesByTypePaginated (junction + parent_id subtree).
 AND (@within::text = '' OR EXISTS (
 SELECT 1
 FROM (
 SELECT parent_space_id AS pid FROM space_parent_spaces WHERE space_id = s.id
 UNION ALL
 SELECT s.parent_id WHERE s.parent_id IS NOT NULL
 ) links
 JOIN spaces g ON g.id = links.pid AND g.deleted_at IS NULL
 LEFT JOIN spaces gp ON gp.id = g.parent_id AND gp.deleted_at IS NULL
 LEFT JOIN spaces ggp ON ggp.id = gp.parent_id AND ggp.deleted_at IS NULL
 WHERE (g.slug = @within AND g.level IN ('country', 'state', 'city'))
 OR (gp.slug = @within AND gp.level IN ('country', 'state', 'city'))
 OR (ggp.slug = @within AND ggp.level IN ('country', 'state', 'city'))
 ));

-- name: ListSpacesByParentPaginated :many
-- Directory ?parent= branch — children of a parent space via the canonical
-- space_parent_spaces junction, with the same filter/sort/visibility support
-- as ListSpacesPaginated (previously geo/region/subregion/sort were silently
-- dropped on this branch — audit fix).
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description, s.wiki_content,
 s.logo_media_id, s.header_image_id, s.geo_code, s.level, s.member_count, s.post_count,
 s.follower_count, s.visibility, s.posting_perm, s.replying_perm, s.commenting_perm,
 s.joining_perm, s.status, s.created_at, s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE s.deleted_at IS NULL
 AND sps.parent_space_id = @parent_id
 AND (
 s.visibility = 'public'
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid AND sm.status = 'approved'
 )
 )
 )
 -- §80 wiki-only types (attraction) are browsable through the wiki
 -- directory only — never the member-facing /spaces listing.
 AND NOT EXISTS (
 SELECT 1 FROM space_types st WHERE st.id = s.space_type_id AND st.wiki_only
 )
 AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
 AND (@geo_code::text = '' OR s.geo_code = @geo_code)
 AND (@level::text = '' OR s.level = @level)
 AND (@region::text = '' OR s.region = @region)
 AND (@subregion::text = '' OR s.subregion = @subregion)
 AND (
 @joined_only::bool = false
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members smj
 WHERE smj.space_id = s.id AND smj.user_id = @viewer_user_id::uuid AND smj.status = 'approved'
 )
 )
 )
ORDER BY
 CASE WHEN @sort::text = 'name' THEN s.name ELSE NULL END ASC,
 CASE
 WHEN @sort::text = 'members' THEN s.member_count
 WHEN @sort::text = 'followers' THEN s.follower_count
 WHEN @sort::text = 'posts' THEN s.post_count
 ELSE NULL
 END DESC,
 CASE WHEN @sort::text = 'activity' THEN s.updated_at ELSE NULL END DESC,
 s.name ASC, s.id ASC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountSpacesByParent :one
-- Total for ListSpacesByParentPaginated (same filter set).
SELECT COUNT(*) AS total
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE s.deleted_at IS NULL
 AND sps.parent_space_id = @parent_id
 AND (
 s.visibility = 'public'
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid AND sm.status = 'approved'
 )
 )
 )
 -- §80 wiki-only types (attraction) — same exclusion as the list query.
 AND NOT EXISTS (
 SELECT 1 FROM space_types st WHERE st.id = s.space_type_id AND st.wiki_only
 )
 AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
 AND (@geo_code::text = '' OR s.geo_code = @geo_code)
 AND (@level::text = '' OR s.level = @level)
 AND (@region::text = '' OR s.region = @region)
 AND (@subregion::text = '' OR s.subregion = @subregion)
 AND (
 @joined_only::bool = false
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members smj
 WHERE smj.space_id = s.id AND smj.user_id = @viewer_user_id::uuid AND smj.status = 'approved'
 )
 )
 );

-- name: ListViewerSpaceRelations :many
-- Batch viewer-scoped flags for a set of space ids: approved membership,
-- pending join request, and follow state. Used to stamp is_member /
-- is_pending / is_following on list and detail responses so the UI can
-- render correct Join/Pending/Joined and Follow state without guessing
-- from the capped /me/spaces list (audit fix).
SELECT r.space_id,
 BOOL_OR(r.is_member) AS is_member,
 BOOL_OR(r.is_pending) AS is_pending,
 BOOL_OR(r.is_following) AS is_following
FROM (
 SELECT sm.space_id,
 (sm.status = 'approved') AS is_member,
 (sm.status = 'pending') AS is_pending,
 FALSE AS is_following
 FROM space_members sm
 WHERE sm.user_id = @viewer_user_id
 AND sm.status IN ('approved', 'pending')
 AND sm.space_id = ANY(@space_ids::uuid[])
 UNION ALL
 SELECT f.entity_id AS space_id, FALSE, FALSE, TRUE
 FROM follows f
 WHERE f.follower_id = @viewer_user_id
 AND f.entity_type = 'space' AND f.entity_id = ANY(@space_ids::uuid[])
) r
GROUP BY r.space_id;

-- name: ListChildSpaces :many
-- Returns all children of a parent space via the space_parent_spaces junction.
-- SPACES-009: uses the canonical junction so geo and custom parents are found.
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description, s.wiki_content,
 s.logo_media_id, s.header_image_id, s.geo_code, s.level, s.member_count, s.post_count,
 s.follower_count, s.visibility, s.posting_perm, s.replying_perm, s.commenting_perm,
 s.joining_perm, s.status, s.created_at, s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE s.deleted_at IS NULL
 AND sps.parent_space_id = @parent_id
 AND (
 s.visibility = 'public'
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid AND sm.status = 'approved'
 )
 )
 )
ORDER BY s.name ASC;

-- name: ListSpacesByParentAndLevel :many
-- Returns spaces of a given level whose parent is linked via
-- space_parent_spaces (the canonical geo hierarchy: country → state → city).
-- Used by the JCE scope-chain picker for country/state/city traversal.
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description, s.wiki_content,
 s.logo_media_id, s.header_image_id, s.geo_code, s.level, s.member_count, s.post_count,
 s.follower_count, s.visibility, s.posting_perm, s.replying_perm, s.commenting_perm,
 s.joining_perm, s.status, s.created_at, s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE s.deleted_at IS NULL
 AND sps.parent_space_id = @parent_id
 AND s.level = @level
 AND (
 s.visibility = 'public'
 OR (
 @viewer_user_id::uuid IS NOT NULL
 AND EXISTS (
 SELECT 1 FROM space_members sm
 WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid AND sm.status = 'approved'
 )
 )
 )
ORDER BY s.name ASC;

-- name: ListSpacesByAncestorAndType :many
-- Returns spaces of a given space_type that are children of an ancestor
-- space via the space_parent_spaces junction. Used by the JCE resolvers for
-- state (ancestor=country), city (ancestor=state), and university
-- (ancestor=city/state/country).
-- sort accepts: '' (newest), 'name', 'members', 'followers', 'posts'.
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description, s.wiki_content,
 s.logo_media_id, s.header_image_id, s.geo_code, s.level, s.member_count, s.post_count,
 s.follower_count, s.visibility, s.posting_perm, s.replying_perm, s.commenting_perm,
 s.joining_perm, s.status, s.created_at, s.updated_at, s.deleted_at, s.currency_code,
 cfv.value_text AS initials
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
LEFT JOIN custom_field_values cfv ON cfv.entity_id = s.id
 AND cfv.entity_type = 'space'
 AND cfv.field_id = '00000000-0000-0000-0000-000000000090'::uuid
WHERE s.space_type_id = @space_type_id AND s.deleted_at IS NULL
 AND sps.parent_space_id = @ancestor_space_id
 AND (@search::text = ''
 OR s.name ILIKE '%' || @search || '%'
 OR s.slug ILIKE '%' || @search || '%'
 OR cfv.value_text ILIKE '%' || @search || '%')
ORDER BY
 CASE WHEN @sort::text = 'name' THEN s.name ELSE NULL END ASC,
 CASE
 WHEN @sort::text = 'members' THEN s.member_count
 WHEN @sort::text = 'followers' THEN s.follower_count
 WHEN @sort::text = 'posts' THEN s.post_count
 ELSE NULL
 END DESC,
 s.created_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountSpacesByAncestorAndType :one
SELECT COUNT(*) AS total
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
LEFT JOIN custom_field_values cfv ON cfv.entity_id = s.id
 AND cfv.entity_type = 'space'
 AND cfv.field_id = '00000000-0000-0000-0000-000000000090'::uuid
WHERE s.space_type_id = @space_type_id AND s.deleted_at IS NULL
 AND sps.parent_space_id = @ancestor_space_id
 AND (@search::text = ''
 OR s.name ILIKE '%' || @search || '%'
 OR s.slug ILIKE '%' || @search || '%'
 OR cfv.value_text ILIKE '%' || @search || '%');

-- name: ListDistinctRegions :many
SELECT DISTINCT region
FROM spaces
WHERE level = 'country' AND deleted_at IS NULL
 AND region IS NOT NULL AND region <> ''
ORDER BY region ASC;

-- name: ListDistinctSubregions :many
SELECT DISTINCT subregion
FROM spaces
WHERE level = 'country' AND deleted_at IS NULL
 AND region = @region
 AND subregion IS NOT NULL AND subregion <> ''
ORDER BY subregion ASC;

-- name: AdminListSpaces :many
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
 s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
 s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
 s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
 s.updated_at, s.deleted_at, s.currency_code,
 COALESCE(u.username, '') AS owner,
 COALESCE(u.id::text, '') AS owner_id,
 COALESCE(st.name, '') AS space_type
FROM spaces s
LEFT JOIN space_members sm ON sm.space_id = s.id AND sm.role = 'owner'
LEFT JOIN users u ON u.id = sm.user_id
LEFT JOIN space_types st ON st.id = s.space_type_id
WHERE (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
 AND (@status::text = '' OR s.status = @status)
 AND (@visibility::text = '' OR s.visibility = @visibility)
 AND (@space_type_id::uuid = '00000000-0000-0000-0000-000000000000' OR s.space_type_id = @space_type_id)
 -- Soft-deleted spaces are hidden by default; ?deleted=only|all surfaces
 -- them so admins can force-delete (same contract as AdminListWikiPages).
 AND CASE @deleted::text
       WHEN 'only' THEN s.deleted_at IS NOT NULL
       WHEN 'all'  THEN true
       ELSE s.deleted_at IS NULL
     END
 -- ORDER BY uses whitelisted CASE keys — only the active (sort_key, sort_dir)
 -- pair produces non-NULL values, so each inactive key is a no-op and the
 -- final created_at DESC fallback keeps the default newest-first ordering.
ORDER BY
 CASE WHEN @sort_key::text = 'name'         AND @sort_dir::text = 'asc'  THEN s.name         END ASC,
 CASE WHEN @sort_key::text = 'name'         AND @sort_dir::text = 'desc' THEN s.name         END DESC,
 CASE WHEN @sort_key::text = 'owner'        AND @sort_dir::text = 'asc'  THEN u.username     END ASC,
 CASE WHEN @sort_key::text = 'owner'        AND @sort_dir::text = 'desc' THEN u.username     END DESC,
 CASE WHEN @sort_key::text = 'member_count' AND @sort_dir::text = 'asc'  THEN s.member_count END ASC,
 CASE WHEN @sort_key::text = 'member_count' AND @sort_dir::text = 'desc' THEN s.member_count END DESC,
 CASE WHEN @sort_key::text = 'visibility'   AND @sort_dir::text = 'asc'  THEN s.visibility   END ASC,
 CASE WHEN @sort_key::text = 'visibility'   AND @sort_dir::text = 'desc' THEN s.visibility   END DESC,
 CASE WHEN @sort_key::text = 'status'       AND @sort_dir::text = 'asc'  THEN s.status       END ASC,
 CASE WHEN @sort_key::text = 'status'       AND @sort_dir::text = 'desc' THEN s.status       END DESC,
 CASE WHEN @sort_key::text = 'space_type'   AND @sort_dir::text = 'asc'  THEN st.name        END ASC,
 CASE WHEN @sort_key::text = 'space_type'   AND @sort_dir::text = 'desc' THEN st.name        END DESC,
 CASE WHEN @sort_key::text = 'created_at'   AND @sort_dir::text = 'asc'  THEN s.created_at   END ASC,
 CASE WHEN @sort_key::text = 'created_at'   AND @sort_dir::text = 'desc' THEN s.created_at   END DESC,
 s.created_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: AdminListSpacesCount :one
SELECT COUNT(*) AS total
FROM spaces s
WHERE (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
 AND (@status::text = '' OR s.status = @status)
 AND (@visibility::text = '' OR s.visibility = @visibility)
 AND (@space_type_id::uuid = '00000000-0000-0000-0000-000000000000' OR s.space_type_id = @space_type_id)
 AND CASE @deleted::text
       WHEN 'only' THEN s.deleted_at IS NOT NULL
       WHEN 'all'  THEN true
       ELSE s.deleted_at IS NULL
     END;

-- name: ListSpacesByMemberUser :many
-- Returns the lean fields the main-app nav, composer, and dashboard need
-- (id, name, slug, is_primary). is_primary is derived from the user's
-- current_geo_space_id so the dashboard can badge the primary space.
SELECT s.id, s.name, s.slug, s.level, s.space_type_id,
 (u.current_geo_space_id IS NOT NULL AND s.id = u.current_geo_space_id) AS is_primary
FROM spaces s
JOIN space_members sm ON sm.space_id = s.id
LEFT JOIN users u ON u.id = @user_id
WHERE sm.user_id = @user_id AND s.deleted_at IS NULL
 AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
 AND (@level::text = '' OR s.level = @level)
 AND (@space_type_id::uuid = '00000000-0000-0000-0000-000000000000' OR s.space_type_id = @space_type_id)
ORDER BY (u.current_geo_space_id IS NOT NULL AND s.id = u.current_geo_space_id) DESC, sm.joined_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: ListAllSpacesForAdmin :many
-- Returns every active space in the platform with no visibility or membership
-- filter. Used by GET /me/spaces when the caller is a super_admin so they can
-- post to any space without joining it first (Section 03 — admin override).
-- Selects the lean fields and flags the caller's primary geo space.
SELECT s.id, s.name, s.slug, s.level, s.space_type_id,
 (u.current_geo_space_id IS NOT NULL AND s.id = u.current_geo_space_id) AS is_primary
FROM spaces s
LEFT JOIN users u ON u.id = @user_id
WHERE s.deleted_at IS NULL
 AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
 AND (@level::text = '' OR s.level = @level)
 AND (@space_type_id::uuid = '00000000-0000-0000-0000-000000000000' OR s.space_type_id = @space_type_id)
ORDER BY (u.current_geo_space_id IS NOT NULL AND s.id = u.current_geo_space_id) DESC, s.created_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: ListSpacesByModeratorUser :many
-- Spaces the caller moderates. Returns lean, dashboard-friendly rows with the
-- caller's primary geo space flagged, and supports server-side search/pagination.
SELECT s.id, s.name, s.slug,
 (u.current_geo_space_id IS NOT NULL AND s.id = u.current_geo_space_id) AS is_primary
FROM spaces s
JOIN space_moderators sm ON sm.space_id = s.id
LEFT JOIN users u ON u.id = @user_id
WHERE sm.user_id = @user_id AND s.deleted_at IS NULL
 AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
ORDER BY (u.current_geo_space_id IS NOT NULL AND s.id = u.current_geo_space_id) DESC, sm.created_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: ListSpacesByOwnerUser :many
-- Spaces the caller owns (space_members.role = 'owner'). Returns lean rows with
-- the primary geo space flagged and supports server-side search/pagination.
-- Catalog: GET /me/spaces/owned.
SELECT s.id, s.name, s.slug,
 (u.current_geo_space_id IS NOT NULL AND s.id = u.current_geo_space_id) AS is_primary,
 s.status
FROM spaces s
JOIN space_members sm ON sm.space_id = s.id
LEFT JOIN users u ON u.id = @user_id
WHERE sm.user_id = @user_id AND sm.role = 'owner'
 AND s.deleted_at IS NULL
 AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
ORDER BY (u.current_geo_space_id IS NOT NULL AND s.id = u.current_geo_space_id) DESC, sm.joined_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountSpacesByOwnerUser :one
-- Counts spaces the user owns (role='owner'), for max_spaces_creatable quota.
SELECT COUNT(*)::int AS count
FROM spaces s
JOIN space_members sm ON sm.space_id = s.id
WHERE sm.user_id = @user_id AND sm.role = 'owner'
 AND s.deleted_at IS NULL;

-- name: GetUserMaxSpacesCreatable :one
-- Returns the max_spaces_creatable from the user's group. If the user is in
-- multiple groups, returns the highest value (most permissive). -1 = unlimited.
-- Ref: Section 03 line 44, §26 quantity gate.
SELECT COALESCE(MAX(ug.max_spaces_creatable), 0)::int AS max_spaces_creatable
FROM user_group_memberships ugm
JOIN user_groups ug ON ugm.group_id = ug.id
WHERE ugm.user_id = @user_id;

-- name: ListStatesByCountryGeoCode :many
-- Geo hierarchy (catalog: GET /geo/countries/:code/states). Countries and
-- states are seeded as spaces (level='country'/'state') parented via the
-- space_parent_spaces join table (migrations/000100_seed_geo.up.sql).
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
 s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
 s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
 s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
 s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
JOIN spaces country ON country.id = sps.parent_space_id
WHERE country.level = 'country'
 AND country.geo_code = @country_code AND s.level = 'state'
 AND s.deleted_at IS NULL AND country.deleted_at IS NULL
ORDER BY s.name;

-- name: ListCitiesByStateID :many
-- Geo hierarchy (catalog: GET /geo/states/:id/cities). Cities are
-- admin-created on demand (not bulk-seeded, per ADR-020) but follow the same
-- space_parent_spaces parenting as states.
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
 s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
 s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
 s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
 s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE sps.parent_space_id = @state_id AND s.level = 'city'
 AND s.deleted_at IS NULL
ORDER BY s.name;

-- name: GetSpaceByID :one
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code
FROM spaces
WHERE id = @id AND deleted_at IS NULL;

-- name: GetSpaceByGeoCode :one
-- Finds a country-level space by its geo_code for primary space auto-join (Section 05, ADR-005).
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code
FROM spaces
WHERE geo_code = $1 AND level = 'country' AND deleted_at IS NULL
LIMIT 1;

-- name: GetSpaceRegionByGeoCode :one
-- Returns the region for a country-level space identified by its geo_code.
-- Used to map a resolved country code to its UN region for feed ranking.
SELECT region
FROM spaces
WHERE geo_code = $1 AND level = 'country' AND deleted_at IS NULL
LIMIT 1;

-- name: GetSpaceCountryAncestorIDs :many
-- Resolves each input space's country-level ancestor by walking the
-- parent_id ∪ space_parent_spaces edge set upward (≤3 hops — the geo chain
-- never exceeds city → state → country). A country is its own ancestor;
-- spaces outside the geo chain (education/custom) return no row.
WITH RECURSIVE walk AS (
    SELECT s.id AS origin, s.id AS node, 0 AS depth
    FROM spaces s
    WHERE s.id = ANY(@space_ids::uuid[])
    UNION ALL
    SELECT w.origin, par.id, w.depth + 1
    FROM walk w
    JOIN (
        SELECT j.space_id AS child, j.parent_space_id AS parent
        FROM space_parent_spaces j
        UNION
        SELECT s2.id, s2.parent_id
        FROM spaces s2
        WHERE s2.parent_id IS NOT NULL
    ) e ON e.child = w.node
    JOIN spaces par ON par.id = e.parent
    WHERE w.depth < 3
)
SELECT DISTINCT w.origin AS space_id, c.id AS country_id
FROM walk w
JOIN spaces c ON c.id = w.node AND c.level = 'country';

-- name: ListSpaceAncestors :many
-- Every ancestor of a space via the parent_id ∪ space_parent_spaces edge
-- set (≤3 hops — the geo chain never exceeds city → state → country).
-- Returns each ancestor's level, type slug, and whether its wiki_content
-- carries a wikidata_id stamp — §80 attraction seeding links ancestors
-- directly (a geo parent without the stamp can't match a P131 QID) and
-- enqueues auto-populate for unstamped geo ancestors.
WITH RECURSIVE walk AS (
    SELECT par.id, 1 AS depth
    FROM (
        SELECT j.parent_space_id AS id FROM space_parent_spaces j WHERE j.space_id = @space_id
        UNION
        SELECT s2.parent_id FROM spaces s2 WHERE s2.id = @space_id AND s2.parent_id IS NOT NULL
    ) par
    UNION
    SELECT par.id, w.depth + 1
    FROM walk w
    JOIN LATERAL (
        SELECT j.parent_space_id AS id FROM space_parent_spaces j WHERE j.space_id = w.id
        UNION
        SELECT s2.parent_id FROM spaces s2 WHERE s2.id = w.id AND s2.parent_id IS NOT NULL
    ) par ON true
    WHERE w.depth < 3
)
SELECT DISTINCT s.id, s.level, COALESCE(st.slug, '') AS type_slug,
       ((s.wiki_content->>'wikidata_id') IS NOT NULL)::boolean AS has_wikidata_id
FROM walk w
JOIN spaces s ON s.id = w.id
LEFT JOIN space_types st ON st.id = s.space_type_id
WHERE s.deleted_at IS NULL AND s.id <> @space_id;

-- name: GetSpaceCountryCurrencyCode :one
-- Resolves a space's country-level ancestor's currency_code by walking the
-- parent_id ∪ space_parent_spaces edge set upward (≤3 hops — mirrors
-- GetSpaceCountryAncestorIDs). A country space resolves to its own
-- currency_code. Display context only — no currencies.is_active filter:
-- inactive reference rows still carry correct name/symbol for read
-- surfaces (Section 58, "Country → currency linkage").
WITH RECURSIVE walk AS (
    SELECT s.id AS node, 0 AS depth
    FROM spaces s
    WHERE s.id = @space_id
    UNION ALL
    SELECT par.id, w.depth + 1
    FROM walk w
    JOIN (
        SELECT j.space_id AS child, j.parent_space_id AS parent
        FROM space_parent_spaces j
        UNION
        SELECT s2.id, s2.parent_id
        FROM spaces s2
        WHERE s2.parent_id IS NOT NULL
    ) e ON e.child = w.node
    JOIN spaces par ON par.id = e.parent
    WHERE w.depth < 3
)
SELECT c.currency_code
FROM walk w
JOIN spaces c ON c.id = w.node AND c.level = 'country'
WHERE c.currency_code IS NOT NULL AND c.currency_code <> ''
ORDER BY w.depth ASC
LIMIT 1;

-- name: FindCitySpaceByName :one
-- ADR-064a: Finds a city-level space by name within a country-level space.
-- Used by the reverse-geocoding flow (POST /me/location) to match a
-- Nominatim-resolved city name to an existing seeded city Space.
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code
FROM spaces
WHERE lower(name) = lower(@city_name)
 AND level = 'city'
 AND deleted_at IS NULL
LIMIT 1;

-- name: FindStateSpaceByName :one
-- ADR-064a: Finds a state-level space by name. Used by the reverse-geocoding
-- flow (POST /me/location) as a fallback when no city-level Space matches
-- but a state/county name is available from Nominatim (BUG-006).
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code
FROM spaces
WHERE lower(name) = lower(@state_name)
 AND level = 'state'
 AND deleted_at IS NULL
LIMIT 1;

-- name: GetSpaceBySlug :one
-- The {slug} route param also accepts a space UUID — callers holding only
-- the id (e.g. service_directory dest/city filters) resolve the display
-- name through the same endpoint. The regex guard keeps @slug::uuid from
-- erroring on ordinary slugs.
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code
FROM spaces
WHERE deleted_at IS NULL
  AND (
    slug = @slug
    OR id = CASE
        WHEN @slug ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
        THEN @slug::uuid
        ELSE NULL
    END
  );

-- name: GetPublicSpaceBySlug :one
-- Public API (Section 47): returns a single space only if it is public,
-- active, and not soft-deleted. Private, member-only, and inactive spaces
-- are never exposed to external API consumers.
-- Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code
FROM spaces
WHERE slug = @slug
 AND visibility = 'public'
 AND status = 'active'
 AND deleted_at IS NULL;

-- name: GetPublicSpaceByGeoCode :one
-- Public API (Section 47 extension digest): resolves a country-level space
-- from an ISO geo_code, guarded identically to GetPublicSpaceBySlug —
-- non-public or inactive spaces are never exposed to external consumers.
-- Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
SELECT id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code
FROM spaces
WHERE geo_code = @geo_code
 AND level = 'country'
 AND visibility = 'public'
 AND status = 'active'
 AND deleted_at IS NULL;

-- name: CreateSpace :one
INSERT INTO spaces (parent_id, space_type_id, name, slug, description, wiki_content, geo_code, level, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status) VALUES (sqlc.narg(parent_id), @space_type_id, @name, @slug, sqlc.narg(description), sqlc.narg(wiki_content), sqlc.narg(geo_code), @level, @visibility, @posting_perm, @replying_perm, @commenting_perm, @joining_perm, @status)
RETURNING id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code;

-- name: UpdateSpace :one
UPDATE spaces
SET name = @name, description = sqlc.narg(description), wiki_content = sqlc.narg(wiki_content),
 geo_code = sqlc.narg(geo_code), visibility = @visibility, posting_perm = @posting_perm,
 replying_perm = @replying_perm, commenting_perm = @commenting_perm, joining_perm = @joining_perm,
 status = @status, updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code;

-- name: UpdateSpaceSlug :one
-- Updates only the slug of a space. Separate from UpdateSpace (which does not
-- touch slug) so university/space admin handlers can rename the slug without
-- rewriting every other column. Platform-scoped, soft-delete-aware. The
-- UNIQUE(slug) constraint rejects a collision with a 23505 error
-- the caller maps to a 409.
UPDATE spaces SET slug = @slug, updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code;

-- name: UpdateSpaceKeywords :exec
-- Updates only the keywords array on a space. Separate from UpdateSpace so
-- that keywords can be updated independently without affecting other fields.
UPDATE spaces SET keywords = @keywords, updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: GetSpaceKeywords :one
-- Returns only the keywords array for a space. Used by GetSpace handler
-- to include keywords in the response without changing the Space struct.
SELECT keywords FROM spaces
WHERE id = @id AND deleted_at IS NULL;

-- name: DeleteSpace :exec
-- Soft-delete: set deleted_at instead of hard-deleting (Rule 11, ADR-014).
-- The account:deletion_cleanup Asynq task may hard-delete after GDPR erasure.
UPDATE spaces SET deleted_at = now(), status = 'deleted'
WHERE id = @id AND deleted_at IS NULL;

-- name: HardDeleteSpace :execrows
-- Permanent delete — only for already-soft-deleted spaces (admin ?force=true).
-- Space-owned children cascade via ON DELETE CASCADE, nullable references
-- SET NULL. Child spaces are guarded: they are independent entities, not
-- owned content, so the row must have no subspaces (admin deletes children
-- first).
DELETE FROM spaces s
WHERE s.id = @id AND s.deleted_at IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM spaces c WHERE c.parent_id = s.id);

-- name: SpaceExists :one
-- Existence probe for the hard-delete path: distinguishes 404 not_found
-- from 409 not_deleted/in_use when the guarded DELETE affects no row.
SELECT EXISTS(SELECT 1 FROM spaces WHERE id = @id) AS exists;

-- name: SpaceHasChildSpaces :one
-- Dependency probe for the hard-delete path (subspaces must go first).
SELECT EXISTS(SELECT 1 FROM spaces WHERE parent_id = @id) AS exists;

-- name: IncrementSpaceCount :exec
UPDATE spaces
SET member_count = GREATEST(0, member_count + @delta)
WHERE id = @id;

-- =====================
-- Space Rules
-- =====================

-- name: ListSpaceRulesBySpace :many
SELECT id, space_id, title, body, position, created_at, updated_at
FROM space_rules
WHERE space_id = @space_id
ORDER BY position ASC, created_at ASC;

-- name: CreateSpaceRule :one
INSERT INTO space_rules (space_id, title, body, position) VALUES (@space_id, @title, @body, @position)

RETURNING id, space_id, title, body, position, created_at, updated_at;

-- name: UpdateSpaceRule :one
UPDATE space_rules
SET title = @title, body = @body, position = @position
WHERE id = @id AND space_id = @space_id
RETURNING id, space_id, title, body, position, created_at, updated_at;

-- name: DeleteSpaceRule :exec
DELETE FROM space_rules
WHERE id = @id AND space_id = @space_id;

-- =====================
-- Space Members
-- =====================

-- name: CheckSpaceMembership :one
-- SPACES-005: only count approved members. A pending join request
-- (status='pending') does NOT make the user a member until an admin
-- approves it (Section 05 lines 358-360).
SELECT EXISTS(
 SELECT 1 FROM space_members
 WHERE space_id = @space_id AND user_id = @user_id
 AND status = 'approved'
) AS is_member;

-- name: CheckSpaceOwnership :one
-- Returns true if the user is the owner (role='owner') of the space.
SELECT EXISTS(
 SELECT 1 FROM space_members
 WHERE space_id = @space_id AND user_id = @user_id
 AND role = 'owner'
) AS is_owner;

-- name: AddSpaceMember :exec
INSERT INTO space_members (space_id, user_id, role) VALUES (@space_id, @user_id, 'member')

ON CONFLICT (space_id, user_id) DO NOTHING;

-- name: AddSpaceOwner :exec
-- Inserts the creator as the space owner (role='owner'). Used by CreateSpace
-- so the creator can exercise the space_owner moderator-assignment path
-- (Section 05 lines 198-224). ON CONFLICT DO NOTHING keeps it idempotent.
INSERT INTO space_members (space_id, user_id, role) VALUES (@space_id, @user_id, 'owner')

ON CONFLICT (space_id, user_id) DO NOTHING;

-- name: AddSpaceMemberPending :exec
-- Inserts a pending join request (status='pending') for spaces with
-- joining_perm='request'. The admin approves/rejects via the join-requests
-- routes (Section 05 lines 358-360).
INSERT INTO space_members (space_id, user_id, role, status) VALUES (@space_id, @user_id, 'member', 'pending')

ON CONFLICT (space_id, user_id) DO NOTHING;

-- name: RemoveSpaceMember :exec
DELETE FROM space_members
WHERE space_id = @space_id AND user_id = @user_id;

-- name: CountSpaceMembers :one
-- SPACES-006: only count approved members, not pending join requests.
SELECT COUNT(*) AS count
FROM space_members
WHERE space_id = @space_id AND status = 'approved';

-- =====================
-- Space Moderators
-- =====================

-- name: AssignSpaceModerator :exec
INSERT INTO space_moderators (space_id, user_id, assigned_by, assigned_by_user_id) VALUES (@space_id, @user_id, @assigned_by, sqlc.narg(assigned_by_user_id))

ON CONFLICT (space_id, user_id) DO NOTHING;

-- name: RemoveSpaceModerator :exec
-- SPACES-015: platform-scoped for defense-in-depth.
DELETE FROM space_moderators
WHERE space_id = @space_id AND user_id = @user_id;

-- name: ListSpaceModerators :many
-- SPACES-015: platform-scoped for defense-in-depth.
SELECT sm.id, sm.space_id, sm.user_id, sm.assigned_by,
 sm.assigned_by_user_id, sm.created_at,
 u.username AS user_username, up.display_name AS user_display_name
FROM space_moderators sm
JOIN users u ON u.id = sm.user_id
LEFT JOIN user_profiles up ON up.user_id = sm.user_id
WHERE sm.space_id = @space_id
ORDER BY sm.created_at ASC;

-- name: GetSpaceModerator :one
-- SPACES-015: platform-scoped for defense-in-depth.
SELECT id, space_id, user_id, assigned_by, assigned_by_user_id, created_at
FROM space_moderators
WHERE space_id = @space_id AND user_id = @user_id;

-- =====================
-- Space Members (with profile data)
-- =====================

-- name: IsUserSpaceMember :one
SELECT EXISTS (
 SELECT 1 FROM space_members
 WHERE space_id = @space_id AND user_id = @user_id AND status = 'approved'
) AS is_member;

-- name: ListSpaceMembersWithProfile :many
-- SPACES-007: only list approved members, not pending join requests.
SELECT sm.user_id, u.username, up.display_name, up.avatar_url, sm.joined_at
FROM space_members sm
JOIN users u ON u.id = sm.user_id
LEFT JOIN user_profiles up ON up.user_id = sm.user_id
WHERE sm.space_id = @space_id AND sm.status = 'approved'
ORDER BY sm.joined_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Space Parent Spaces (Many-to-Many Hierarchy)
-- =====================

-- name: AddParentSpace :exec
INSERT INTO space_parent_spaces (space_id, parent_space_id) VALUES (@space_id, @parent_space_id)

ON CONFLICT DO NOTHING;

-- name: RemoveParentSpace :exec
DELETE FROM space_parent_spaces
WHERE space_id = @space_id AND parent_space_id = @parent_space_id;

-- name: GetParentSpacesBySpaceID :many
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
 s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
 s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
 s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
 s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
JOIN space_parent_spaces sps ON sps.parent_space_id = s.id
WHERE sps.space_id = @space_id AND s.deleted_at IS NULL
ORDER BY s.name;

-- name: GetChildSpacesByParentID :many
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
 s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
 s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
 s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
 s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE sps.parent_space_id = @parent_space_id AND s.deleted_at IS NULL
ORDER BY s.name;

-- name: UpdateSpaceSourceID :exec
-- Link a space to its canonical intel_sources row (v2 source directory).
UPDATE spaces
SET source_id = $1, updated_at = now()
WHERE id = $2;

-- name: UpdateSpaceParentID :exec
UPDATE spaces
SET parent_id = @parent_id, updated_at = now()
WHERE id = @id;

-- name: GetSpacePrimaryGeoParents :many
-- Returns the country/state/city primary spaces linked to a member space
-- via space_parent_spaces. Used to add geo parents to content created in
-- that space.
SELECT s.id
FROM spaces s
JOIN space_parent_spaces sps ON sps.parent_space_id = s.id
WHERE sps.space_id = @space_id
 AND s.level IN ('country', 'state', 'city')
 AND s.deleted_at IS NULL;

-- =====================
-- Space Admin Queries (handlers)
-- =====================

-- =====================
-- Space Join Requests
-- =====================

-- name: ListSpaceJoinRequests :many
SELECT sm.user_id, u.username, up.display_name, up.avatar_url, sm.joined_at, sm.status
FROM space_members sm
JOIN users u ON u.id = sm.user_id
LEFT JOIN user_profiles up ON up.user_id = sm.user_id
WHERE sm.space_id = @space_id AND sm.status = 'pending'
ORDER BY sm.joined_at ASC;

-- name: ListAllSpaceJoinRequests :many
-- Admin-wide pending join requests across every space in the admin's platform.
-- space_id is included so the admin panel can build a composite request id
-- (space_id::user_id) for the approve/reject endpoints.
SELECT sm.space_id, sm.user_id, u.username, up.display_name, up.avatar_url, sm.joined_at, sm.status,
 s.name AS space_name
FROM space_members sm
JOIN users u ON u.id = sm.user_id
LEFT JOIN user_profiles up ON up.user_id = sm.user_id
LEFT JOIN spaces s ON s.id = sm.space_id
WHERE sm.status = 'pending'
ORDER BY sm.joined_at ASC;

-- name: UpdateSpaceJoinRequest :exec
UPDATE space_members
SET status = @status
WHERE space_id = @space_id AND user_id = @user_id;

-- =====================
-- Space Content Views
-- =====================

-- name: SumSpaceContentViews :one
SELECT COUNT(*) AS count
FROM content_views
WHERE space_id = @space_id;

-- =====================
-- Space Pinned Posts
-- =====================

-- name: UpdateSpacePinPosition :exec
UPDATE space_pinned_posts
SET position = @position
WHERE space_id = @space_id AND content_id = @content_id;

-- name: UpdateSpaceStatus :exec
UPDATE spaces SET status = @status, updated_at = now() WHERE id = @id AND deleted_at IS NULL;

-- name: CountSpacePublishedContent :one
SELECT COUNT(*) AS count
FROM contents
WHERE space_id = @space_id AND status = 'published';

-- name: SuggestParentSpaces :many
-- SPACES-003: return primary spaces (country/state/city level) that can
-- serve as parents of a custom space (Section 05 lines 14, 190, 252-267).
-- The previous query filtered by space_types.slug = 'category' which does
-- not exist in the spec; the correct filter is by level.
SELECT s.id, s.name, s.slug FROM spaces s
WHERE s.status = 'active' AND s.deleted_at IS NULL
 AND s.level IN ('country', 'state', 'city')
 AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
ORDER BY s.name LIMIT 20;

-- =====================
-- Space Follower Count
-- =====================

-- name: IncrementSpaceFollowerCount :exec
UPDATE spaces SET follower_count = follower_count + 1
WHERE id = @id;

-- name: DecrementSpaceFollowerCount :exec
UPDATE spaces SET follower_count = follower_count - 1
WHERE id = @id AND follower_count > 0;

-- =====================
-- Public Profile Spaces Tab
-- =====================

-- name: ListPublicSpacesByMemberUser :many
-- Public profile Spaces tab: PUBLIC, active spaces the user is a member of.
-- Excludes 'members'/'private' visibility and non-active spaces. Public-safe.
-- Includes the member's role (sm.role) so the profile card can show it.
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
 s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
 s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
 s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
 s.updated_at, s.deleted_at, s.currency_code, sm.role AS member_role
FROM spaces s
JOIN space_members sm ON sm.space_id = s.id
WHERE sm.user_id = @user_id
 AND s.visibility = 'public' AND s.status = 'active' AND s.deleted_at IS NULL
ORDER BY sm.joined_at DESC, s.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListSpaceMemberUserIDs :many
SELECT user_id FROM space_members
WHERE space_id = @space_id
LIMIT @limit_val;
-- name: UpdateSpaceLogoMediaID :one
-- Updates only the logo_media_id on a space (used by university:fetch_logo task).
UPDATE spaces
SET logo_media_id = @logo_media_id
WHERE id = @id AND deleted_at IS NULL
RETURNING id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code;

-- name: UpdateSpaceHeaderImageID :one
-- Updates only the header_image_id on a space (banner image upload).
UPDATE spaces
SET header_image_id = @header_image_id
WHERE id = @id AND deleted_at IS NULL
RETURNING id, parent_id, space_type_id, name, slug, description, wiki_content,
 logo_media_id, header_image_id, geo_code, level, member_count, post_count,
 follower_count, visibility, posting_perm, replying_perm, commenting_perm,
 joining_perm, status, created_at, updated_at, deleted_at, currency_code;

-- name: FindSpacesByKeywords :many
-- Find primary (country/state/city) and public education spaces (including
-- universities) whose keywords, full name, or university initials overlap
-- with the given keyword list. Used for auto-assigning content to spaces
-- based on the content title. Multi-word contiguous phrases in the title
-- generate multi-word n-grams so that full names (e.g. "University of Lagos")
-- and multi-word keywords (e.g. "new york") match exactly. Matches are
-- ordered by specificity: initials match > full name match > keyword match,
-- then by city > state > country > education. Errors are logged by the caller.
WITH title_ngrams AS (
 SELECT
 value AS kw,
 array_length(regexp_split_to_array(value, '\s+'), 1) AS word_count
 FROM unnest(@keywords::text[]) AS value
),
keyword_matches AS (
 -- country/state/city spaces whose configured keywords overlap with any title n-gram.
 SELECT
 s.id,
 s.name,
 s.level,
 st.slug AS space_type_slug,
 (SELECT COALESCE(MAX(tn.word_count * 1000), 0)
 FROM title_ngrams tn
 WHERE tn.kw IN (SELECT lower(sk) FROM unnest(s.keywords) sk)) AS score
 FROM spaces s
 JOIN space_types st ON st.id = s.space_type_id
 WHERE s.deleted_at IS NULL
 AND s.visibility = 'public'
 AND s.status = 'active'
 AND (s.level IN ('country', 'state', 'city') OR st.category = 'education')
 AND s.keywords && @keywords::text[]
),
name_matches AS (
 -- primary spaces and education spaces whose full name exactly matches a title n-gram.
 SELECT
 s.id,
 s.name,
 s.level,
 st.slug AS space_type_slug,
 (tn.word_count * 1000 + 100) AS score
 FROM spaces s
 JOIN space_types st ON st.id = s.space_type_id
 CROSS JOIN title_ngrams tn
 WHERE s.deleted_at IS NULL
 AND s.visibility = 'public'
 AND s.status = 'active'
 AND (s.level IN ('country', 'state', 'city') OR st.category = 'education')
 AND LOWER(s.name) = tn.kw
),
initials_matches AS (
 -- universities whose initials custom-field value exactly matches a title n-gram.
 SELECT
 s.id,
 s.name,
 s.level,
 st.slug AS space_type_slug,
 (tn.word_count * 1000 + 500) AS score
 FROM spaces s
 JOIN space_types st ON st.id = s.space_type_id
 JOIN custom_fields cf ON cf.key = 'initials'
 JOIN custom_field_values cfv ON cfv.entity_id = s.id AND cfv.entity_type = 'space' AND cfv.field_id = cf.id
 CROSS JOIN title_ngrams tn
 WHERE s.deleted_at IS NULL
 AND s.visibility = 'public'
 AND s.status = 'active'
 AND st.slug = 'university'
 AND LOWER(cfv.value_text) = tn.kw
)
SELECT
 id,
 name,
 level,
 space_type_slug
FROM (
 SELECT * FROM keyword_matches
 UNION ALL
 SELECT * FROM name_matches
 UNION ALL
 SELECT * FROM initials_matches
) combined
GROUP BY id, name, level, space_type_slug
ORDER BY
 MAX(score) DESC,
 CASE level
 WHEN 'city' THEN 1
 WHEN 'state' THEN 2
 WHEN 'country' THEN 3
 ELSE 4
 END,
 name;

-- =====================
-- Space count statistics (members / posts / followers)
-- =====================

-- name: CountSpaceStats :many
-- Returns live member, post, and follower counts for a set of spaces.
-- post_count counts published content assigned via the authoritative
-- content_spaces junction (not just the backward-compat geo_space_id).
SELECT
 s.id,
 GREATEST(0, (
 SELECT COUNT(*) FROM space_members sm
 WHERE sm.space_id = s.id
 AND sm.status = 'approved'
 ))::int AS member_count,
 (
 SELECT COUNT(*) FROM content_spaces cs
 JOIN contents c ON c.id = cs.content_id
 WHERE cs.space_id = s.id
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND c.status = 'published'
 )::int AS post_count,
 (
 SELECT COUNT(*) FROM follows f
 WHERE f.entity_type = 'space' AND f.entity_id = s.id
 )::int AS follower_count
FROM spaces s
WHERE s.id = ANY(@space_ids::uuid[]);

-- =====================
-- §80 Attractions — wikidata_id dedup for the bulk seeder
-- =====================

-- name: SpaceExistsByWikidataID :one
-- True when any space already carries this Wikidata QID — the attraction
-- wikidata_id custom field is the authoritative check (it survives
-- wiki_content rewrites), with the wiki_content->>'wikidata_id' stamp the
-- seeder writes at insert as fallback for rows enriched before the field
-- existed.
SELECT EXISTS (
    SELECT 1 FROM custom_field_values cfv
    WHERE cfv.entity_type = 'space'
      AND cfv.field_id = '00000000-0000-0000-0000-000000000117'::uuid
      AND cfv.value_text = @qid
) OR EXISTS (
    SELECT 1 FROM spaces s
    WHERE s.deleted_at IS NULL
      AND s.wiki_content->>'wikidata_id' = @qid
) AS exists;

-- name: SpaceIDsByWikidataIDs :many
-- Batch QID → space resolution for multi-P131 attraction parent linking
-- (§80): matches the wikidata_id custom field and the wiki_content stamp
-- geo spaces carry after auto-population. Returns the space's type slug
-- so callers can keep geo parents (country/state/city) and skip anything
-- else that happens to carry the QID.
SELECT DISTINCT s.id, st.slug AS type_slug, (s.wiki_content->>'wikidata_id')::text AS qid
FROM spaces s
JOIN space_types st ON st.id = s.space_type_id
WHERE s.deleted_at IS NULL
  AND s.wiki_content->>'wikidata_id' = ANY(@qids::text[])
UNION
SELECT s.id, st.slug AS type_slug, cfv.value_text::text AS qid
FROM custom_field_values cfv
JOIN spaces s ON s.id = cfv.entity_id AND s.deleted_at IS NULL
JOIN space_types st ON st.id = s.space_type_id
WHERE cfv.entity_type = 'space'
  AND cfv.field_id = '00000000-0000-0000-0000-000000000117'::uuid
  AND cfv.value_text = ANY(@qids::text[]);
