-- name: GetUserProfile :one
SELECT DISTINCT ON (u.id)
       COALESCE(up.id, gen_random_uuid()) AS id,
       u.tenant_id,
       u.id AS user_id,
       up.display_name,
       up.avatar_url,
       up.avatar_media_id,
       up.header_media_id,
       up.bio,
       up.timezone,
       up.cover_url,
       COALESCE(up.created_at, u.created_at) AS created_at,
       COALESCE(up.updated_at, u.updated_at) AS updated_at,
       u.email, u.username, u.profile_type, u.status, u.follower_count, u.last_login_at,
       ug.slug AS user_group_name
FROM users u
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = u.tenant_id
LEFT JOIN user_group_memberships ugm ON ugm.user_id = u.id AND ugm.tenant_id = u.tenant_id
LEFT JOIN user_groups ug ON ug.id = ugm.group_id AND ug.tenant_id = ugm.tenant_id
WHERE u.tenant_id = @tenant_id AND u.id = @user_id
ORDER BY u.id, CASE WHEN ug.slug = 'super_admin' THEN 0 ELSE 1 END;

-- name: GetUserByID :one
SELECT id, tenant_id, email, username, profile_type, status, follower_count, created_at, updated_at
FROM users
WHERE tenant_id = @tenant_id AND id = @user_id;

-- name: GetUserEmailVerifiedStatus :one
-- Returns email_verified_at for the user. Used by the CanPostInSpace
-- "verified" posting_perm axis (Section 05 line 149, BUG-003).
-- Tenant-scoped per Section 04 (BUG-009).
SELECT email_verified_at FROM users WHERE id = @user_id AND tenant_id = @tenant_id;

-- name: GetUserByUsername :one
SELECT DISTINCT ON (u.id)
       COALESCE(up.id, gen_random_uuid()) AS id,
       u.tenant_id,
       u.id AS user_id,
       up.display_name,
       up.avatar_url,
       up.bio,
       up.timezone,
       up.cover_url,
       COALESCE(up.created_at, u.created_at) AS created_at,
       COALESCE(up.updated_at, u.updated_at) AS updated_at,
       u.email, u.username, u.profile_type, u.status, u.follower_count,
       ug.slug AS user_group_name
FROM users u
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = u.tenant_id
LEFT JOIN user_group_memberships ugm ON ugm.user_id = u.id AND ugm.tenant_id = u.tenant_id
LEFT JOIN user_groups ug ON ug.id = ugm.group_id AND ug.tenant_id = ugm.tenant_id
WHERE u.tenant_id = @tenant_id AND u.username = @username
ORDER BY u.id, CASE WHEN ug.slug = 'super_admin' THEN 0 ELSE 1 END;

-- name: UpsertUserProfile :exec
INSERT INTO user_profiles (tenant_id, user_id, display_name, avatar_url, avatar_media_id, header_media_id, bio, timezone, cover_url)
VALUES (@tenant_id, @user_id, @display_name, @avatar_url, @avatar_media_id, @header_media_id, @bio, @timezone, @cover_url)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    avatar_url = EXCLUDED.avatar_url,
    avatar_media_id = EXCLUDED.avatar_media_id,
    header_media_id = EXCLUDED.header_media_id,
    bio = EXCLUDED.bio,
    timezone = EXCLUDED.timezone,
    cover_url = EXCLUDED.cover_url,
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
SELECT u.id, u.tenant_id, u.email, u.username, u.profile_type,
       CASE
         WHEN EXISTS (
           SELECT 1 FROM user_operation_bans uob
           WHERE uob.user_id = u.id AND uob.deleted_at IS NULL
             AND (uob.expires_at IS NULL OR uob.expires_at > now())
         ) THEN 'restricted'
         ELSE u.status
       END AS status,
       u.follower_count, u.created_at, u.updated_at,
       (SELECT COALESCE(string_agg(ug.name, ', '), '')
        FROM user_group_memberships ugm
        JOIN user_groups ug ON ug.id = ugm.group_id AND ug.tenant_id = u.tenant_id
        WHERE ugm.user_id = u.id AND ugm.tenant_id = u.tenant_id) AS groups
FROM users u
WHERE u.tenant_id = @tenant_id
  AND (@search::text = '' OR u.username ILIKE '%' || @search || '%' OR u.email ILIKE '%' || @search || '%')
  AND (@status::text = '' OR
       (@status = 'restricted' AND EXISTS (
         SELECT 1 FROM user_operation_bans uob
         WHERE uob.user_id = u.id AND uob.deleted_at IS NULL
           AND (uob.expires_at IS NULL OR uob.expires_at > now())
       )) OR
       (@status != 'restricted' AND u.status = @status))
  AND (@group_slug::text = '' OR EXISTS (
    SELECT 1 FROM user_group_memberships ugm
    JOIN user_groups ug ON ug.id = ugm.group_id AND ug.tenant_id = @tenant_id
    WHERE ugm.user_id = u.id AND ugm.tenant_id = @tenant_id AND ug.slug = @group_slug
  ))
ORDER BY u.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountUsers :one
SELECT COUNT(*)::int AS count
FROM users u
WHERE u.tenant_id = @tenant_id
  AND (@search::text = '' OR u.username ILIKE '%' || @search || '%' OR u.email ILIKE '%' || @search || '%')
  AND (@status::text = '' OR
       (@status = 'restricted' AND EXISTS (
         SELECT 1 FROM user_operation_bans uob
         WHERE uob.user_id = u.id AND uob.deleted_at IS NULL
           AND (uob.expires_at IS NULL OR uob.expires_at > now())
       )) OR
       (@status != 'restricted' AND u.status = @status))
  AND (@group_slug::text = '' OR EXISTS (
    SELECT 1 FROM user_group_memberships ugm
    JOIN user_groups ug ON ug.id = ugm.group_id AND ug.tenant_id = @tenant_id
    WHERE ugm.user_id = u.id AND ugm.tenant_id = @tenant_id AND ug.slug = @group_slug
  ));

-- name: GetUserPrivacySettings :one
-- Tenant-scoped per Section 04 (BUG-009).
SELECT user_id, tenant_id, dm_privacy, profile_visibility, search_indexable,
       show_online_status, show_activity, created_at, updated_at
FROM user_privacy_settings
WHERE tenant_id = $1 AND user_id = $2;

-- name: UpsertUserPrivacySettings :exec
-- Tenant-scoped per Section 04 (BUG-009).
INSERT INTO user_privacy_settings (tenant_id, user_id, dm_privacy, profile_visibility, search_indexable, show_online_status, show_activity)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    dm_privacy = EXCLUDED.dm_privacy,
    profile_visibility = EXCLUDED.profile_visibility,
    search_indexable = EXCLUDED.search_indexable,
    show_online_status = EXCLUDED.show_online_status,
    show_activity = EXCLUDED.show_activity,
    updated_at = now();

-- name: GetUserPasswordHash :one
SELECT id, password_hash FROM users WHERE id = $1 AND tenant_id = $2;

-- name: UpdateUserPassword :exec
-- Tenant-scoped per Section 04 (BUG-009).
UPDATE users SET password_hash = $3, updated_at = now() WHERE id = $1 AND tenant_id = $2;

-- name: ListUserSessions :many
-- Tenant-scoped per Section 04 (BUG-009).
SELECT id, token_hash, expires_at, created_at
FROM refresh_tokens
WHERE tenant_id = $1 AND user_id = $2 AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: RevokeUserSession :exec
-- Tenant-scoped per Section 04 (BUG-009).
UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND tenant_id = $2 AND user_id = $3 AND revoked_at IS NULL;

-- name: RevokeAllUserSessions :exec
-- Tenant-scoped per Section 04 (BUG-009).
UPDATE refresh_tokens SET revoked_at = now() WHERE tenant_id = $1 AND user_id = $2 AND revoked_at IS NULL AND id != $3;

-- name: ListOAuthAccounts :many
SELECT id, provider, provider_user_id, created_at
FROM oauth_accounts
WHERE tenant_id = $1 AND user_id = $2;

-- name: DisconnectOAuthAccount :exec
DELETE FROM oauth_accounts WHERE tenant_id = $1 AND user_id = $2 AND provider = $3;

-- name: UserHasPassword :one
-- Tenant-scoped per Section 04 (BUG-009).
SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND tenant_id = $2 AND password_hash IS NOT NULL AND password_hash != '') AS has_password;

-- name: ListUserContent :many
SELECT c.id, c.title, c.content_type, c.status, c.created_at, c.updated_at
FROM contents c
WHERE c.tenant_id = $1 AND c.author_id = $2 AND c.deleted_at IS NULL
  AND ($3::text = '' OR c.status = $3)
ORDER BY c.created_at DESC
LIMIT $4 OFFSET $5;

-- name: GetUserDraft :one
-- Returns the caller's own draft content by ID (tenant + author scoped,
-- status = 'draft'). Used by the /create composer to restore an existing
-- draft for editing via GET /api/v1/me/drafts/{id}. Mirrors the column set
-- of GetContentByID so the response shape matches GET /content/{slug}.
SELECT c.id, c.tenant_id, c.author_id, c.organization_id, c.space_id,
       c.content_type, c.title, c.slug, c.body, c.body_html, c.field_values,
       c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned,
       c.read_time_minutes, c.view_count, c.comment_count, c.share_count,
       c.reshare_count, c.publish_at, c.published_at, c.created_at, c.updated_at
FROM contents c
WHERE c.id = $1 AND c.tenant_id = $2 AND c.author_id = $3
  AND c.status = 'draft' AND c.deleted_at IS NULL;

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
-- Tenant-scoped per Section 04 (BUG-009).
SELECT COALESCE(status, 'active') AS status
FROM users
WHERE id = $1 AND tenant_id = $2;

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

-- name: UpdateLastLoginAt :exec
UPDATE users SET last_login_at = now() WHERE tenant_id = $1 AND id = $2;

-- name: UpdateUserProfileType :exec
UPDATE users SET profile_type = $3, updated_at = now() WHERE tenant_id = $1 AND id = $2;

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
       up.cover_url,
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

-- name: ActiveUserExistsByID :one
-- Existence check for the public profile gate: is there an ACTIVE user with
-- this id in this tenant? Non-active (banned/suspended/deleted) or missing → false.
SELECT EXISTS (
    SELECT 1 FROM users
    WHERE tenant_id = @tenant_id AND id = @id AND status = 'active'
) AS exists;

-- name: GetUserContentAnalytics :many
-- Per-content analytics breakdown for the authenticated user (Section 40).
-- Returns each published/draft content item with its view/comment/share counts.
SELECT c.id, c.title, c.content_type, c.status, c.view_count, c.comment_count, c.share_count, c.created_at
FROM contents c
WHERE c.tenant_id = $1 AND c.author_id = $2 AND c.deleted_at IS NULL
ORDER BY c.view_count DESC NULLS LAST, c.created_at DESC
LIMIT $3 OFFSET $4;

-- name: GetUserFollowerGrowth :many
-- Daily follower growth for the authenticated user (Section 40).
-- Counts new follows where entity_type = 'user' and entity_id = the user,
-- grouped by day.
SELECT date_trunc('day', f.created_at)::date AS day, COUNT(*)::bigint AS new_followers
FROM follows f
WHERE f.tenant_id = $1
  AND f.entity_type = 'user'
  AND f.entity_id = $2
  AND ($3::date IS NULL OR f.created_at::date >= $3)
  AND ($4::date IS NULL OR f.created_at::date <= $4)
GROUP BY 1
ORDER BY 1 DESC;

-- name: UpdateUserGeoSpace :exec
-- Sets the user's current primary geo space (Section 05, ADR-005).
UPDATE users
SET current_geo_space_id = $1, updated_at = now()
WHERE id = $2 AND tenant_id = $3;

-- name: ListActiveUsersForDigest :many
SELECT u.id, u.tenant_id, u.email
FROM users u
WHERE u.status = 'active'
  AND u.email IS NOT NULL AND u.email != ''
  AND u.email_verified_at IS NOT NULL
  AND COALESCE(
    (SELECT value::bool FROM system_settings ss
     WHERE ss.tenant_id = u.tenant_id AND ss.key = 'email.digest_enabled'),
    true)
ORDER BY u.id;

-- ---------------------------------------------------------------------------
-- Account management (Section 50: PATCH /me/account)
-- Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
-- ---------------------------------------------------------------------------

-- name: GetUserAccountForUpdate :one
-- Fetches the fields needed to validate an account update (username cooldown,
-- email uniqueness, current verified state). Row-locks the user for the
-- duration of the transaction to prevent concurrent account edits.
SELECT id, username, email, email_verified_at, username_changed_at
FROM users
WHERE id = $1 AND tenant_id = $2
FOR UPDATE;

-- name: UpdateUserUsername :exec
-- Updates the username and stamps username_changed_at for cooldown tracking.
-- Section 50: username may be changed once per 30 days.
UPDATE users
SET username = $3, username_changed_at = now(), updated_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: UpdateUserEmail :exec
-- Updates the email and resets email_verified_at to NULL so the user must
-- re-verify the new address (Section 50: email change triggers re-verification).
UPDATE users
SET email = $3, email_verified_at = NULL, updated_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: EmailExistsForTenant :one
-- Checks whether an email is already taken by another user in the same tenant.
-- Returns the count (0 = available).
SELECT COUNT(*)::int AS count
FROM users
WHERE tenant_id = $1 AND email = $2 AND id != $3;

-- name: UsernameExistsForTenant :one
-- Checks whether a username is already taken by another user in the same tenant.
SELECT COUNT(*)::int AS count
FROM users
WHERE tenant_id = $1 AND username = $2 AND id != $3;

-- ---------------------------------------------------------------------------
-- Analytics counts (Section 40: BUG-012 — replace row iteration with SQL COUNT)
-- Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
-- ---------------------------------------------------------------------------

-- name: CountUserContent :one
-- Counts the user's content by status within the tenant.
SELECT
    COUNT(*)::int AS total,
    COUNT(*) FILTER (WHERE status = 'draft')::int AS draft_count,
    COUNT(*) FILTER (WHERE status = 'published')::int AS published_count
FROM contents
WHERE tenant_id = @tenant_id AND author_id = @user_id
  AND deleted_at IS NULL;

-- name: CountUserBookmarks :one
-- Counts the user's bookmarks.
SELECT COUNT(*)::int AS count
FROM bookmarks
WHERE user_id = @user_id AND tenant_id = @tenant_id;
