-- Mute system queries (Section 52).

-- =====================
-- User Mutes
-- =====================

-- name: MuteUser :exec
INSERT INTO user_mutes (user_id, muted_user_id) VALUES (@user_id, @muted_user_id)

ON CONFLICT (user_id, muted_user_id) DO NOTHING;

-- name: UnmuteUser :exec
DELETE FROM user_mutes
WHERE user_id = @user_id AND muted_user_id = @muted_user_id;

-- name: IsUserMuted :one
SELECT EXISTS(
 SELECT 1 FROM user_mutes
 WHERE user_id = @user_id AND muted_user_id = @muted_user_id
) AS is_muted;

-- name: ListMutedUsers :many
SELECT um.id, um.muted_user_id, COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url,
 um.created_at
FROM user_mutes um
JOIN users u ON u.id = um.muted_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE um.user_id = @user_id
ORDER BY um.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMutedUsers :one
SELECT COUNT(*)::int AS count
FROM user_mutes
WHERE user_id = @user_id;

-- =====================
-- Space Mutes
-- =====================

-- name: MuteSpace :exec
INSERT INTO space_mutes (user_id, space_id) VALUES (@user_id, @space_id)

ON CONFLICT (user_id, space_id) DO NOTHING;

-- name: UnmuteSpace :exec
DELETE FROM space_mutes
WHERE user_id = @user_id AND space_id = @space_id;

-- name: IsSpaceMuted :one
SELECT EXISTS(
 SELECT 1 FROM space_mutes
 WHERE user_id = @user_id AND space_id = @space_id
) AS is_muted;

-- name: ListMutedSpaces :many
SELECT sm.id, sm.space_id, s.name AS space_name, s.slug AS space_slug,
 s.level, s.space_type_id,
 (u.current_geo_space_id IS NOT NULL AND s.id = u.current_geo_space_id) AS is_primary,
 sm.created_at
FROM space_mutes sm
JOIN spaces s ON s.id = sm.space_id
LEFT JOIN users u ON u.id = @user_id
WHERE sm.user_id = @user_id
 AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
 AND (@level::text = '' OR s.level = @level)
 AND (@space_type_id::uuid = '00000000-0000-0000-0000-000000000000' OR s.space_type_id = @space_type_id)
ORDER BY (u.current_geo_space_id IS NOT NULL AND s.id = u.current_geo_space_id) DESC, sm.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMutedSpaces :one
SELECT COUNT(*)::int AS count
FROM space_mutes
WHERE user_id = @user_id;

-- name: GetMutedUserIDs :many
SELECT muted_user_id
FROM user_mutes
WHERE user_id = @user_id;

-- name: GetMutedSpaceIDs :many
SELECT space_id
FROM space_mutes
WHERE user_id = @user_id;
