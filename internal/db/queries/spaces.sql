-- Space type, space, space rule, and permission preset queries.

-- =====================
-- Space Permission Presets
-- =====================

-- name: ListPermissionPresetsByTenant :many
SELECT id, tenant_id, name, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, is_system, created_at, updated_at
FROM space_permission_presets
WHERE tenant_id = @tenant_id
ORDER BY created_at ASC, id ASC;

-- name: GetPermissionPresetByName :one
SELECT id, tenant_id, name, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, is_system, created_at, updated_at
FROM space_permission_presets
WHERE name = @name AND tenant_id = @tenant_id;

-- name: UpdatePermissionPreset :one
UPDATE space_permission_presets
SET visibility = @visibility, posting_perm = @posting_perm, replying_perm = @replying_perm,
    commenting_perm = @commenting_perm, joining_perm = @joining_perm
WHERE id = @id AND tenant_id = @tenant_id AND is_system = true
RETURNING id, tenant_id, name, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, is_system, created_at, updated_at;

-- =====================
-- Space Types
-- =====================

-- name: ListSpaceTypesByTenant :many
SELECT id, tenant_id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, is_university
FROM space_types
WHERE tenant_id = @tenant_id
ORDER BY created_at ASC, id ASC;

-- name: GetSpaceTypeByID :one
SELECT id, tenant_id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, is_university
FROM space_types
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetSpaceTypeBySlug :one
SELECT id, tenant_id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, is_university
FROM space_types
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: CreateSpaceType :one
INSERT INTO space_types (tenant_id, name, slug, description, icon, field_group_id, is_university, is_system)
VALUES (@tenant_id, @name, @slug, sqlc.narg(description), sqlc.narg(icon), sqlc.narg(field_group_id), @is_university, @is_system)
RETURNING id, tenant_id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, is_university;

-- name: UpdateSpaceType :one
UPDATE space_types
SET name = @name, description = sqlc.narg(description), icon = sqlc.narg(icon),
    field_group_id = sqlc.narg(field_group_id), is_university = @is_university
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false
RETURNING id, tenant_id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at, is_university;

-- name: DeleteSpaceType :exec
DELETE FROM space_types
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false;

-- =====================
-- Spaces
-- =====================

-- name: ListSpacesByTenant :many
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC, id DESC;

-- name: ListSpacesByLevel :many
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE tenant_id = @tenant_id AND level = @level
ORDER BY created_at DESC, id DESC;

-- name: ListSpacesByType :many
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE tenant_id = @tenant_id AND space_type_id = @space_type_id
ORDER BY created_at DESC, id DESC;

-- name: ListChildSpaces :many
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE parent_id = @parent_id AND tenant_id = @tenant_id
ORDER BY created_at ASC, id ASC;

-- name: AdminListSpaces :many
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
       s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
       s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
       s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
       s.updated_at,
       COALESCE(u.username, '') AS owner,
       COALESCE(u.id::text, '') AS owner_id,
       COALESCE(st.name, '') AS space_type
FROM spaces s
LEFT JOIN space_members sm ON sm.space_id = s.id AND sm.tenant_id = s.tenant_id AND sm.role = 'owner'
LEFT JOIN users u ON u.id = sm.user_id AND u.tenant_id = s.tenant_id
LEFT JOIN space_types st ON st.id = s.space_type_id AND st.tenant_id = s.tenant_id
WHERE s.tenant_id = @tenant_id
ORDER BY s.created_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: AdminListSpacesCount :one
SELECT COUNT(*) AS total
FROM spaces s
WHERE s.tenant_id = @tenant_id;

-- name: ListSpacesByMemberUser :many
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
       s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
       s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
       s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
       s.updated_at
FROM spaces s
JOIN space_members sm ON sm.space_id = s.id
WHERE sm.user_id = @user_id AND sm.tenant_id = @tenant_id
ORDER BY sm.joined_at DESC, s.id DESC;

-- name: ListSpacesByModeratorUser :many
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
       s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
       s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
       s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
       s.updated_at
FROM spaces s
JOIN space_moderators sm ON sm.space_id = s.id
WHERE sm.user_id = @user_id AND sm.tenant_id = @tenant_id
ORDER BY sm.created_at DESC, s.id DESC;

-- name: ListStatesByCountryGeoCode :many
-- Geo hierarchy (catalog: GET /geo/countries/:code/states). Countries and
-- states are seeded as spaces (level='country'/'state') parented via the
-- space_parent_spaces join table (migrations/000100_seed_geo.up.sql).
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
       s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
       s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
       s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
       s.updated_at
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
JOIN spaces country ON country.id = sps.parent_space_id
WHERE country.tenant_id = @tenant_id AND country.level = 'country'
  AND country.geo_code = @country_code AND s.level = 'state'
ORDER BY s.name;

-- name: ListCitiesByStateID :many
-- Geo hierarchy (catalog: GET /geo/states/:id/cities). Cities are
-- admin-created on demand (not bulk-seeded, per ADR-020) but follow the same
-- space_parent_spaces parenting as states.
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
       s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
       s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
       s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
       s.updated_at
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE sps.parent_space_id = @state_id AND s.tenant_id = @tenant_id AND s.level = 'city'
ORDER BY s.name;

-- name: GetSpaceByID :one
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetSpaceBySlug :one
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: CreateSpace :one
INSERT INTO spaces (
    tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
    geo_code, level, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status
) VALUES (
    @tenant_id, sqlc.narg(parent_id), @space_type_id, @name, @slug, sqlc.narg(description),
    sqlc.narg(wiki_content), sqlc.narg(geo_code), @level, @visibility, @posting_perm,
    @replying_perm, @commenting_perm, @joining_perm, @status
)
RETURNING id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
          logo_media_id, header_image_id, geo_code, level, member_count, post_count,
          follower_count, visibility, posting_perm, replying_perm, commenting_perm,
          joining_perm, status, created_at, updated_at;

-- name: UpdateSpace :one
UPDATE spaces
SET name = @name, description = sqlc.narg(description), wiki_content = sqlc.narg(wiki_content),
    geo_code = sqlc.narg(geo_code), visibility = @visibility, posting_perm = @posting_perm,
    replying_perm = @replying_perm, commenting_perm = @commenting_perm, joining_perm = @joining_perm,
    status = @status
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
          logo_media_id, header_image_id, geo_code, level, member_count, post_count,
          follower_count, visibility, posting_perm, replying_perm, commenting_perm,
          joining_perm, status, created_at, updated_at;

-- name: DeleteSpace :exec
DELETE FROM spaces
WHERE id = @id AND tenant_id = @tenant_id;

-- name: IncrementSpaceCount :exec
UPDATE spaces
SET member_count = member_count + @delta
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Space Rules
-- =====================

-- name: ListSpaceRulesBySpace :many
SELECT id, space_id, title, body, position, created_at, updated_at
FROM space_rules
WHERE space_id = @space_id
ORDER BY position ASC, created_at ASC;

-- name: CreateSpaceRule :one
INSERT INTO space_rules (space_id, title, body, position)
VALUES (@space_id, @title, @body, @position)
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
SELECT EXISTS(
    SELECT 1 FROM space_members
    WHERE space_id = @space_id AND user_id = @user_id AND tenant_id = @tenant_id
) AS is_member;

-- name: AddSpaceMember :exec
INSERT INTO space_members (space_id, user_id, tenant_id, role)
VALUES (@space_id, @user_id, @tenant_id, 'member')
ON CONFLICT (space_id, user_id) DO NOTHING;

-- name: RemoveSpaceMember :exec
DELETE FROM space_members
WHERE space_id = @space_id AND user_id = @user_id AND tenant_id = @tenant_id;

-- name: CountSpaceMembers :one
SELECT COUNT(*) AS count
FROM space_members
WHERE space_id = @space_id AND tenant_id = @tenant_id;

-- =====================
-- Space Moderators
-- =====================

-- name: AssignSpaceModerator :exec
INSERT INTO space_moderators (tenant_id, space_id, user_id, assigned_by, assigned_by_user_id)
VALUES (@tenant_id, @space_id, @user_id, @assigned_by, sqlc.narg(assigned_by_user_id))
ON CONFLICT (tenant_id, space_id, user_id) DO NOTHING;

-- name: RemoveSpaceModerator :exec
DELETE FROM space_moderators
WHERE space_id = @space_id AND user_id = @user_id;

-- name: ListSpaceModerators :many
SELECT sm.id, sm.tenant_id, sm.space_id, sm.user_id, sm.assigned_by,
       sm.assigned_by_user_id, sm.created_at,
       u.username AS user_username, up.display_name AS user_display_name
FROM space_moderators sm
JOIN users u ON u.id = sm.user_id
LEFT JOIN user_profiles up ON up.user_id = sm.user_id AND up.tenant_id = sm.tenant_id
WHERE sm.space_id = @space_id
ORDER BY sm.created_at ASC;

-- name: GetSpaceModerator :one
SELECT id, tenant_id, space_id, user_id, assigned_by, assigned_by_user_id, created_at
FROM space_moderators
WHERE space_id = @space_id AND user_id = @user_id;

-- =====================
-- Space Members (with profile data)
-- =====================

-- name: ListSpaceMembersWithProfile :many
SELECT sm.user_id, u.username, up.display_name, up.avatar_url, sm.joined_at
FROM space_members sm
JOIN users u ON u.id = sm.user_id AND u.tenant_id = sm.tenant_id
LEFT JOIN user_profiles up ON up.user_id = sm.user_id AND up.tenant_id = sm.tenant_id
WHERE sm.space_id = @space_id AND sm.tenant_id = @tenant_id
ORDER BY sm.joined_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Space Parent Spaces (Many-to-Many Hierarchy)
-- =====================

-- name: AddParentSpace :exec
INSERT INTO space_parent_spaces (space_id, parent_space_id)
VALUES (@space_id, @parent_space_id)
ON CONFLICT DO NOTHING;

-- name: RemoveParentSpace :exec
DELETE FROM space_parent_spaces
WHERE space_id = @space_id AND parent_space_id = @parent_space_id;

-- name: GetParentSpacesBySpaceID :many
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
       s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
       s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
       s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
       s.updated_at
FROM spaces s
JOIN space_parent_spaces sps ON sps.parent_space_id = s.id
WHERE sps.space_id = @space_id AND s.tenant_id = @tenant_id
ORDER BY s.name;

-- name: GetChildSpacesByParentID :many
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
       s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
       s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
       s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
       s.updated_at
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE sps.parent_space_id = @parent_space_id AND s.tenant_id = @tenant_id
ORDER BY s.name;

-- =====================
-- Space Admin Queries (handlers)
-- =====================

-- =====================
-- Space Join Requests
-- =====================

-- name: ListSpaceJoinRequests :many
SELECT sm.user_id, u.username, up.display_name, up.avatar_url, sm.joined_at, sm.status
FROM space_members sm
JOIN users u ON u.id = sm.user_id AND u.tenant_id = sm.tenant_id
LEFT JOIN user_profiles up ON up.user_id = sm.user_id AND up.tenant_id = sm.tenant_id
WHERE sm.space_id = @space_id AND sm.tenant_id = @tenant_id AND sm.status = 'pending'
ORDER BY sm.joined_at ASC;

-- name: ListAllSpaceJoinRequests :many
-- Admin-wide pending join requests across every space in the admin's tenant.
-- space_id is included so the admin panel can build a composite request id
-- (space_id::user_id) for the approve/reject endpoints.
SELECT sm.space_id, sm.user_id, u.username, up.display_name, up.avatar_url, sm.joined_at, sm.status
FROM space_members sm
JOIN users u ON u.id = sm.user_id AND u.tenant_id = sm.tenant_id
LEFT JOIN user_profiles up ON up.user_id = sm.user_id AND up.tenant_id = sm.tenant_id
WHERE sm.tenant_id = @tenant_id AND sm.status = 'pending'
ORDER BY sm.joined_at ASC;

-- name: UpdateSpaceJoinRequest :exec
UPDATE space_members
SET status = @status
WHERE space_id = @space_id AND user_id = @user_id AND tenant_id = @tenant_id;

-- =====================
-- Space Content Views
-- =====================

-- name: SumSpaceContentViews :one
SELECT COUNT(*) AS count
FROM content_views
WHERE space_id = @space_id AND tenant_id = @tenant_id;

-- =====================
-- Space Pinned Posts
-- =====================

-- name: UpdateSpacePinPosition :exec
UPDATE space_pinned_posts
SET position = @position
WHERE space_id = @space_id AND content_id = @content_id;

-- name: UpdateSpaceStatus :exec
UPDATE spaces SET status = @status, updated_at = now() WHERE id = @id AND tenant_id = @tenant_id;

-- name: CountSpacePublishedContent :one
SELECT COUNT(*) AS count
FROM contents
WHERE space_id = @space_id AND tenant_id = @tenant_id AND status = 'published';

-- name: SuggestParentSpaces :many
SELECT s.id, s.name, s.slug FROM spaces s
WHERE s.tenant_id = @tenant_id AND s.status = 'active'
  AND s.space_type_id IN (SELECT id FROM space_types WHERE slug = 'category')
ORDER BY s.name LIMIT 20;

-- =====================
-- Space Follower Count
-- =====================

-- name: IncrementSpaceFollowerCount :exec
UPDATE spaces SET follower_count = follower_count + 1
WHERE id = @id AND tenant_id = @tenant_id;

-- name: DecrementSpaceFollowerCount :exec
UPDATE spaces SET follower_count = follower_count - 1
WHERE id = @id AND tenant_id = @tenant_id AND follower_count > 0;

-- =====================
-- Public Profile Spaces Tab
-- =====================

-- name: ListPublicSpacesByMemberUser :many
-- Public profile Spaces tab: PUBLIC, active spaces the user is a member of.
-- Excludes 'members'/'private' visibility and non-active spaces. Public-safe.
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
       s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
       s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
       s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
       s.updated_at
FROM spaces s
JOIN space_members sm ON sm.space_id = s.id AND sm.tenant_id = s.tenant_id
WHERE sm.user_id = @user_id AND s.tenant_id = @tenant_id
  AND s.visibility = 'public' AND s.status = 'active'
ORDER BY sm.joined_at DESC, s.id DESC
LIMIT @page_limit OFFSET @page_offset;