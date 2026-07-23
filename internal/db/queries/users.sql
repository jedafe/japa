-- name: GetUserProfile :one
SELECT DISTINCT ON (up.user_id)
       up.id, up.tenant_id, up.user_id, up.display_name, up.avatar_url, up.bio,
       up.timezone, up.created_at, up.updated_at,
       u.email, u.username, u.profile_type, u.status, u.follower_count,
       ug.slug AS user_group_name
FROM user_profiles up
JOIN users u ON u.id = up.user_id AND u.tenant_id = up.tenant_id
LEFT JOIN user_group_memberships ugm ON ugm.user_id = u.id AND ugm.tenant_id = u.tenant_id
LEFT JOIN user_groups ug ON ug.id = ugm.group_id AND ug.tenant_id = ugm.tenant_id
WHERE up.tenant_id = @tenant_id AND up.user_id = @user_id
ORDER BY up.user_id, CASE WHEN ug.slug = 'super_admin' THEN 0 ELSE 1 END;

-- name: GetUserByID :one
SELECT id, tenant_id, email, username, profile_type, status, follower_count, created_at, updated_at
FROM users
WHERE tenant_id = @tenant_id AND id = @user_id;

-- name: GetUserByUsername :one
SELECT DISTINCT ON (up.user_id)
       up.id, up.tenant_id, up.user_id, up.display_name, up.avatar_url, up.bio,
       up.timezone, up.created_at, up.updated_at,
       u.email, u.username, u.profile_type, u.status, u.follower_count,
       ug.slug AS user_group_name
FROM user_profiles up
JOIN users u ON u.id = up.user_id AND u.tenant_id = up.tenant_id
LEFT JOIN user_group_memberships ugm ON ugm.user_id = u.id AND ugm.tenant_id = u.tenant_id
LEFT JOIN user_groups ug ON ug.id = ugm.group_id AND ug.tenant_id = ugm.tenant_id
WHERE up.tenant_id = @tenant_id AND u.username = @username
ORDER BY up.user_id, CASE WHEN ug.slug = 'super_admin' THEN 0 ELSE 1 END;

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

-- name: GetUserPrivacySettings :one
SELECT user_id, dm_privacy, profile_visibility, search_indexable,
       show_online_status, show_activity, created_at, updated_at
FROM user_privacy_settings
WHERE user_id = $1;

-- name: UpsertUserPrivacySettings :exec
INSERT INTO user_privacy_settings (user_id, dm_privacy, profile_visibility, search_indexable, show_online_status, show_activity)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id) DO UPDATE SET
    dm_privacy = EXCLUDED.dm_privacy,
    profile_visibility = EXCLUDED.profile_visibility,
    search_indexable = EXCLUDED.search_indexable,
    show_online_status = EXCLUDED.show_online_status,
    show_activity = EXCLUDED.show_activity,
    updated_at = now();

-- name: GetUserPasswordHash :one
SELECT id, password_hash FROM users WHERE id = $1 AND tenant_id = $2;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: ListUserSessions :many
SELECT id, token_hash, expires_at, created_at
FROM refresh_tokens
WHERE user_id = $1 AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: RevokeUserSession :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: RevokeAllUserSessions :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL AND id != $2;

-- name: ListOAuthAccounts :many
SELECT id, provider, provider_user_id, created_at
FROM oauth_accounts
WHERE tenant_id = $1 AND user_id = $2;

-- name: DisconnectOAuthAccount :exec
DELETE FROM oauth_accounts WHERE tenant_id = $1 AND user_id = $2 AND provider = $3;

-- name: UserHasPassword :one
SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND password_hash IS NOT NULL AND password_hash != '') AS has_password;

-- name: ListUserContent :many
SELECT c.id, c.title, c.content_type, c.status, c.created_at, c.updated_at
FROM contents c
WHERE c.tenant_id = $1 AND c.author_id = $2 AND c.deleted_at IS NULL
  AND ($3::text = '' OR c.status = $3)
ORDER BY c.created_at DESC
LIMIT $4 OFFSET $5;

-- name: ListUserBookmarks :many
SELECT b.id, b.entity_type, b.entity_id, b.folder, b.created_at
FROM bookmarks b
WHERE b.user_id = $1
ORDER BY b.created_at DESC
LIMIT $2 OFFSET $3;

-- name: DeactivateUser :exec
UPDATE users SET status = 'suspended', updated_at = now() WHERE id = $1 AND tenant_id = $2;

-- name: SoftDeleteUser :exec
UPDATE users SET status = 'deleted', updated_at = now() WHERE id = $1 AND tenant_id = $2;

-- name: GetUserStatus :one
SELECT COALESCE(status, 'active') AS status
FROM users
WHERE id = $1;

-- name: ListUserActivity :many
-- Recent authored contents and comments merged into one feed, newest first
-- (admin user-detail activity tab). Comment bodies are truncated so a long
-- comment cannot bloat the list payload.
SELECT activity.type, activity.content, activity.created_at
FROM (
    SELECT 'content'::text AS type, c.title AS content, c.created_at
    FROM contents c
    WHERE c.tenant_id = $1 AND c.author_id = $2 AND c.deleted_at IS NULL
    UNION ALL
    SELECT 'comment'::text AS type, left(cm.body, 240) AS content, cm.created_at
    FROM comments cm
    WHERE cm.tenant_id = $1 AND cm.author_user_id = $2 AND cm.deleted_at IS NULL
) activity
ORDER BY activity.created_at DESC
LIMIT $3;

-- name: CountUserActivity :one
SELECT (SELECT count(*) FROM contents c
        WHERE c.tenant_id = $1 AND c.author_id = $2 AND c.deleted_at IS NULL)
     + (SELECT count(*) FROM comments cm
        WHERE cm.tenant_id = $1 AND cm.author_user_id = $2 AND cm.deleted_at IS NULL) AS total;

-- name: GetPublicProfileByUsername :one
-- Public profile header for /u/[username]. NO PII (no email/status/last_login/groups).
SELECT DISTINCT ON (u.id)
       u.id AS user_id,
       u.username,
       u.profile_type,
       u.follower_count,
       up.display_name,
       up.avatar_url,
       up.bio,
       COALESCE(up.created_at, u.created_at) AS created_at,
       (SELECT COUNT(*)::int
          FROM follows f
         WHERE f.tenant_id = u.tenant_id AND f.follower_id = u.id) AS following_count
FROM users u
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = u.tenant_id
WHERE u.tenant_id = @tenant_id
  AND u.username = @username
  AND u.status = 'active'
ORDER BY u.id;
