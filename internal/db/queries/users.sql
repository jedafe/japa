-- name: GetUserProfile :one
SELECT up.id, up.tenant_id, up.user_id, up.display_name, up.avatar_url, up.bio,
       up.timezone, up.created_at, up.updated_at,
       u.email, u.username, u.profile_type, u.status, u.follower_count
FROM user_profiles up
JOIN users u ON u.id = up.user_id AND u.tenant_id = up.tenant_id
WHERE up.tenant_id = @tenant_id AND up.user_id = @user_id;

-- name: GetUserByID :one
SELECT id, tenant_id, email, username, profile_type, status, follower_count, created_at, updated_at
FROM users
WHERE tenant_id = @tenant_id AND id = @user_id;

-- name: GetUserByUsername :one
SELECT up.id, up.tenant_id, up.user_id, up.display_name, up.avatar_url, up.bio,
       up.timezone, up.created_at, up.updated_at,
       u.email, u.username, u.profile_type, u.status, u.follower_count
FROM user_profiles up
JOIN users u ON u.id = up.user_id AND u.tenant_id = up.tenant_id
WHERE up.tenant_id = @tenant_id AND u.username = @username;

-- name: UpsertUserProfile :exec
INSERT INTO user_profiles (tenant_id, user_id, display_name, avatar_url, bio, timezone)
VALUES (@tenant_id, @user_id, @display_name, @avatar_url, @bio, @timezone)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    avatar_url = EXCLUDED.avatar_url,
    bio = EXCLUDED.bio,
    timezone = EXCLUDED.timezone,
    updated_at = now();

-- name: GetUserPreferences :one
SELECT id, tenant_id, user_id, dark_mode, language, created_at, updated_at
FROM user_preferences
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: UpsertUserPreferences :exec
INSERT INTO user_preferences (tenant_id, user_id, dark_mode, language)
VALUES (@tenant_id, @user_id, @dark_mode, @language)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    dark_mode = EXCLUDED.dark_mode,
    language = EXCLUDED.language,
    updated_at = now();

-- name: ListUsers :many
SELECT id, tenant_id, email, username, profile_type, status, follower_count, created_at, updated_at
FROM users
WHERE tenant_id = @tenant_id
  AND (@search::text = '' OR username ILIKE '%' || @search || '%' OR email ILIKE '%' || @search || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountUsers :one
SELECT COUNT(*)::int AS count
FROM users
WHERE tenant_id = @tenant_id
  AND (@search::text = '' OR username ILIKE '%' || @search || '%' OR email ILIKE '%' || @search || '%');
