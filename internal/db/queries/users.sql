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

-- name: GetUserEmail :one
-- Returns the user's email address for notification email delivery (§35).
-- Tenant-scoped per §04.
SELECT email FROM users WHERE id = @user_id AND tenant_id = @tenant_id;

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
INSERT INTO user_profiles (
    tenant_id, user_id, display_name, avatar_url, avatar_media_id, header_media_id,
    bio, timezone, cover_url, location, banner_media_id
) VALUES (
    @tenant_id, @user_id, @display_name, @avatar_url, @avatar_media_id, @header_media_id,
    @bio, @timezone, @cover_url, sqlc.narg(location), sqlc.narg(banner_media_id)
)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    avatar_url = EXCLUDED.avatar_url,
    avatar_media_id = EXCLUDED.avatar_media_id,
    header_media_id = EXCLUDED.header_media_id,
    bio = EXCLUDED.bio,
    timezone = EXCLUDED.timezone,
    cover_url = EXCLUDED.cover_url,
    location = COALESCE(EXCLUDED.location, user_profiles.location),
    banner_media_id = COALESCE(EXCLUDED.banner_media_id, user_profiles.banner_media_id),
    updated_at = now();

-- name: GetUserProfileForUpdate :one
-- Row-locks the user_profiles row for the read-merge-upsert pattern in
-- UpdateProfile (BUG-002). The FOR UPDATE lock is held until the
-- transaction commits or rolls back, preventing concurrent profile edits
-- from racing on the merge step.
SELECT id, tenant_id, user_id, display_name, avatar_url, avatar_media_id,
       header_media_id, bio, timezone, cover_url, created_at, updated_at
FROM user_profiles
WHERE tenant_id = @tenant_id AND user_id = @user_id
FOR UPDATE;

-- name: GetUserProfileID :one
-- ADR-064a: Returns the user_profiles.id for a given user. Used as the
-- entity_id when upserting custom field values (entity_type='user_profile').
-- Returns pgx.ErrNoRows when the profile doesn't exist yet.
SELECT id
FROM user_profiles
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: GetUserPreferences :one
SELECT id, tenant_id, user_id, dark_mode, language, created_at, updated_at
FROM user_preferences
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: GetUserPreferencesForUpdate :one
-- Row-locks the user_preferences row for the read-merge-upsert pattern in
-- UpdatePreferences (BUG-003). The FOR UPDATE lock is held until the
-- transaction commits or rolls back, preventing concurrent preference
-- edits from racing on the merge step.
SELECT id, tenant_id, user_id, dark_mode, language, created_at, updated_at
FROM user_preferences
WHERE tenant_id = @tenant_id AND user_id = @user_id
FOR UPDATE;

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
       show_online_status, show_activity, show_points_publicly, created_at, updated_at
FROM user_privacy_settings
WHERE tenant_id = $1 AND user_id = $2;

-- name: GetUserPrivacySettingsForUpdate :one
-- Row-locks the user_privacy_settings row for the read-merge-upsert pattern
-- in UpdatePrivacySettings (BUG-004). The FOR UPDATE lock is held until
-- the transaction commits or rolls back, preventing concurrent privacy
-- edits from racing on the merge step.
SELECT user_id, tenant_id, dm_privacy, profile_visibility, search_indexable,
       show_online_status, show_activity, show_points_publicly, created_at, updated_at
FROM user_privacy_settings
WHERE tenant_id = $1 AND user_id = $2
FOR UPDATE;

-- name: UpsertUserPrivacySettings :exec
-- Tenant-scoped per Section 04 (BUG-009).
INSERT INTO user_privacy_settings (tenant_id, user_id, dm_privacy, profile_visibility, search_indexable, show_online_status, show_activity, show_points_publicly)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    dm_privacy = EXCLUDED.dm_privacy,
    profile_visibility = EXCLUDED.profile_visibility,
    search_indexable = EXCLUDED.search_indexable,
    show_online_status = EXCLUDED.show_online_status,
    show_activity = EXCLUDED.show_activity,
    show_points_publicly = EXCLUDED.show_points_publicly,
    updated_at = now();

-- name: GetUserPasswordHash :one
SELECT id, password_hash FROM users WHERE id = $1 AND tenant_id = $2;

-- name: UpdateUserPassword :exec
-- Tenant-scoped per Section 04 (BUG-009).
-- Sets password_changed_at so max-password-age policies can be enforced
-- (Ref: sections/02-authentication-sessions.md).
UPDATE users
SET password_hash = $3, password_changed_at = now(), updated_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: ListUserSessions :many
-- Tenant-scoped per Section 04 (BUG-009). Returns device_name, ip_address,
-- last_used_at per JEOS §50 contract.
SELECT id, token_hash, expires_at, created_at, device_name, ip_address, last_used_at
FROM refresh_tokens
WHERE tenant_id = $1 AND user_id = $2 AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: GetSessionIDByHash :one
-- Resolves the current session's id from the jc_refresh cookie hash so
-- /me/sessions can flag the current session and RevokeAllSessions can keep
-- it (JEOS §50: "revoke all other sessions (keep current one)").
SELECT id
FROM refresh_tokens
WHERE tenant_id = $1 AND user_id = $2 AND token_hash = $3 AND revoked_at IS NULL;

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
-- Returns the caller's own content, optionally filtered by status,
-- content_type, and a free-text search over title/slug. An empty status,
-- content_type, q, or sort string means "no filter" / "default newest".
-- Supports ?sort=newest|oldest|most_viewed for GET /me/content (JEOS §50).
-- Used by GET /me/content (published), /me/drafts (draft), and /me/scheduled (scheduled).
SELECT c.id, c.title, c.slug, c.content_type, c.status, c.view_count, c.comment_count, c.created_at, c.updated_at
FROM contents c
WHERE c.tenant_id = @tenant_id AND c.author_id = @author_id AND c.deleted_at IS NULL
  AND (@status::text = '' OR c.status = @status)
  AND (@content_type::text = '' OR c.content_type = @content_type)
  AND (@q::text = '' OR c.title ILIKE '%' || @q || '%' OR c.slug ILIKE '%' || @q || '%')
ORDER BY
  CASE
    WHEN @sort::text = 'oldest' THEN EXTRACT(EPOCH FROM c.created_at)
    WHEN @sort::text = 'most_viewed' THEN -c.view_count::float8
    ELSE -EXTRACT(EPOCH FROM c.created_at)
  END ASC,
  c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetUserDraft :one
-- Returns the caller's own draft content by ID (tenant + author scoped,
-- status = 'draft'). Used by the /create composer to restore an existing
-- draft for editing via GET /api/v1/me/drafts/{id}. Mirrors the column set
-- of GetContentByID so the response shape matches GET /content/{slug}.
SELECT c.id, c.tenant_id, c.author_id, c.organization_id, c.space_id,
       c.content_type, c.title, c.slug, c.body, c.body_html, c.field_values,
       c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.video_source_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned,
       c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
       c.read_time_minutes, c.view_count, c.comment_count, c.share_count,
       c.reshare_count, c.publish_at, c.published_at, c.created_at, c.updated_at
FROM contents c
WHERE c.id = $1 AND c.tenant_id = $2 AND c.author_id = $3
  AND c.status = 'draft' AND c.deleted_at IS NULL;

-- name: GetUserContent :one
-- Returns the caller's own content by ID (tenant + author scoped, any
-- status). Used by GET /api/v1/me/content/{id} so the /create composer can
-- restore published, scheduled, or draft content for editing — the
-- status='draft'-scoped /me/drafts/{id} route only covers drafts. Mirrors
-- the GetUserDraft column set so the response shape is identical. A missing
-- row or content owned by another user/tenant resolves to pgx.ErrNoRows → 404.
SELECT c.id, c.tenant_id, c.author_id, c.organization_id, c.space_id,
       c.content_type, c.title, c.slug, c.body, c.body_html, c.field_values,
       c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.video_source_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned,
       c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
       c.read_time_minutes, c.view_count, c.comment_count, c.share_count,
       c.reshare_count, c.publish_at, c.published_at, c.created_at, c.updated_at
FROM contents c
WHERE c.id = $1 AND c.tenant_id = $2 AND c.author_id = $3
  AND c.deleted_at IS NULL;

-- name: GetContentByIDForEditor :one
-- Returns any content by ID within the tenant (no author filter). Used by
-- GET /api/v1/me/content/{id} as a fallback when the caller is a super-admin
-- or holds the RBAC `content.edit` permission but is not the content owner.
-- Mirrors the GetUserContent column set so the response shape is identical.
-- A missing row or a different tenant's content resolves to pgx.ErrNoRows → 404.
SELECT c.id, c.tenant_id, c.author_id, c.organization_id, c.space_id,
       c.content_type, c.title, c.slug, c.body, c.body_html, c.field_values,
       c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.video_source_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned,
       c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
       c.read_time_minutes, c.view_count, c.comment_count, c.share_count,
       c.reshare_count, c.publish_at, c.published_at, c.created_at, c.updated_at
FROM contents c
WHERE c.id = $1 AND c.tenant_id = $2
  AND c.deleted_at IS NULL;

-- name: ListUserBookmarks :many
-- Enriches each bookmark with the title + slug of the bookmarked entity.
-- entity_type drives which table is joined (content/blog_post→contents,
-- forum_thread→forum_threads, forum_reply→forum_replies/forum_threads,
-- space→spaces, tag→tags, learning_track→learning_tracks,
-- doc_page→doc_pages). Unknown types get NULL title/slug; the frontend
-- falls back to the entity_id.
-- Tenant-scoped (BL-006), optional folder_id filter (BL-002), and optional
-- entity_type filter (so the learn app can request only learning entities).
SELECT
    b.id, b.entity_type, b.entity_id, b.folder, b.folder_id, b.created_at,
    CASE
        WHEN b.entity_type IN ('content', 'blog_post') THEN c.title
        WHEN b.entity_type = 'forum_thread'           THEN ft.title
        WHEN b.entity_type = 'forum_reply'            THEN frt.title
        WHEN b.entity_type = 'space'                   THEN s.name
        WHEN b.entity_type = 'tag'                     THEN t.name
        WHEN b.entity_type = 'learning_track'          THEN lt.title
        WHEN b.entity_type = 'doc_page'                THEN dp.title
    END AS title,
    CASE
        WHEN b.entity_type IN ('content', 'blog_post') THEN c.slug
        WHEN b.entity_type = 'forum_thread'           THEN ft.slug
        WHEN b.entity_type = 'forum_reply'            THEN frt.slug
        WHEN b.entity_type = 'space'                   THEN s.slug
        WHEN b.entity_type = 'tag'                     THEN t.slug
        WHEN b.entity_type = 'learning_track'          THEN lt.slug
        WHEN b.entity_type = 'doc_page'                THEN dp.slug
    END AS slug,
    CASE
        WHEN b.entity_type = 'doc_page' THEN dp.audience
    END AS audience
FROM bookmarks b
LEFT JOIN contents c
    ON b.entity_type IN ('content', 'blog_post') AND b.entity_id = c.id
LEFT JOIN forum_threads ft
    ON b.entity_type = 'forum_thread' AND b.entity_id = ft.id
LEFT JOIN forum_replies fr
    ON b.entity_type = 'forum_reply' AND b.entity_id = fr.id
LEFT JOIN forum_threads frt
    ON b.entity_type = 'forum_reply' AND fr.thread_id = frt.id
LEFT JOIN spaces s
    ON b.entity_type = 'space' AND b.entity_id = s.id
LEFT JOIN tags t
    ON b.entity_type = 'tag' AND b.entity_id = t.id
LEFT JOIN learning_tracks lt
    ON b.entity_type = 'learning_track' AND b.entity_id = lt.id
LEFT JOIN doc_pages dp
    ON b.entity_type = 'doc_page' AND b.entity_id = dp.id
WHERE b.user_id = @user_id AND b.tenant_id = @tenant_id
  AND (sqlc.narg(folder_id)::uuid IS NULL OR b.folder_id = sqlc.narg(folder_id)::uuid)
  AND (sqlc.narg(entity_types)::text[] IS NULL OR b.entity_type = ANY(sqlc.narg(entity_types)::text[]))
ORDER BY b.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

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
-- Joins user_privacy_settings to surface show_points_publicly so the service
-- layer can conditionally include the user's Points balance (Section 28).
-- is_synthetic/synthetic_source identify imported video channels; location and
-- banner_media_id carry the channel's public metadata (migration 000633).
SELECT DISTINCT ON (u.id)
       u.id AS user_id,
       u.username,
       u.profile_type,
       u.follower_count,
       u.is_synthetic,
       u.synthetic_source,
       up.display_name,
       up.avatar_url,
       up.bio,
       up.cover_url,
       up.location,
       up.banner_media_id,
       COALESCE(up.created_at, u.created_at) AS created_at,
       COALESCE(ups.show_points_publicly, false) AS show_points_publicly,
       (SELECT COUNT(*)::int
          FROM follows f
         WHERE f.tenant_id = u.tenant_id AND f.follower_id = u.id) AS following_count
FROM users u
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = u.tenant_id
LEFT JOIN user_privacy_settings ups ON ups.user_id = u.id AND ups.tenant_id = u.tenant_id
WHERE u.tenant_id = @tenant_id
  AND u.username = @username
  AND u.status = 'active'
ORDER BY u.id;

-- name: ListPublicUsers :many
-- Public user directory at GET /users — PII-safe (no email/status/last_login/groups).
-- Only active users who have not opted out of search indexability. Search filters
-- by username or display_name. profile_type filter is optional (empty = all).
-- Ordered by follower_count DESC then created_at DESC so the directory surfaces
-- prominent users first.
-- Ref: japaconnect-engineering/sections/02-authentication-sessions.md
--      japaconnect-engineering/sections/19-privacy-gdpr.md (search_indexable opt-out)
SELECT u.id AS user_id,
       u.username,
       u.profile_type,
       u.follower_count,
       u.is_synthetic,
       u.synthetic_source,
       up.display_name,
       up.avatar_url,
       up.bio,
       up.cover_url,
       up.location,
       COALESCE(up.created_at, u.created_at) AS created_at,
       (SELECT COUNT(*)::int
          FROM follows f
         WHERE f.tenant_id = u.tenant_id AND f.follower_id = u.id) AS following_count
FROM users u
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = u.tenant_id
LEFT JOIN user_privacy_settings ups ON ups.user_id = u.id AND ups.tenant_id = u.tenant_id
WHERE u.tenant_id = @tenant_id
  AND u.status = 'active'
  AND COALESCE(ups.search_indexable, true) = true
  AND (@search::text = '' OR u.username ILIKE '%' || @search || '%' OR up.display_name ILIKE '%' || @search || '%')
  AND (@profile_type::text = '' OR u.profile_type = @profile_type)
ORDER BY u.follower_count DESC NULLS LAST, u.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPublicUsers :one
SELECT COUNT(*)::int AS count
FROM users u
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = u.tenant_id
LEFT JOIN user_privacy_settings ups ON ups.user_id = u.id AND ups.tenant_id = u.tenant_id
WHERE u.tenant_id = @tenant_id
  AND u.status = 'active'
  AND COALESCE(ups.search_indexable, true) = true
  AND (@search::text = '' OR u.username ILIKE '%' || @search || '%' OR up.display_name ILIKE '%' || @search || '%')
  AND (@profile_type::text = '' OR u.profile_type = @profile_type);

-- name: GetPublicUserByID :one
-- Public API (Section 47): PII-safe single-user lookup by ID. Returns only
-- id, username, profile_type, follower_count, display_name, avatar_url, bio,
-- and created_at — NO email, status, last_login, or group membership.
-- Only active users who have not opted out of search indexability.
-- Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
--      japaconnect-engineering/sections/19-privacy-gdpr.md (search_indexable opt-out)
SELECT u.id AS user_id,
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
LEFT JOIN user_privacy_settings ups ON ups.user_id = u.id AND ups.tenant_id = u.tenant_id
WHERE u.tenant_id = @tenant_id
  AND u.id = @user_id
  AND u.status = 'active'
  AND COALESCE(ups.search_indexable, true) = true;

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

-- name: GetUserTopContentByPeriod :many
-- Top content for the authenticated user within a date range, ranked by
-- actual views in that range. Falls back to all-time view_count for ordering
-- when a post had no views in the period. Used by GET /me/analytics.
-- Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
WITH period_views AS (
    SELECT ucv.content_id, COUNT(*)::bigint AS views
    FROM user_content_views ucv
    JOIN contents c ON c.id = ucv.content_id
    WHERE c.tenant_id = $1
      AND c.author_id = $2
      AND c.deleted_at IS NULL
      AND ($3::timestamptz IS NULL OR ucv.viewed_at >= $3)
      AND ($4::timestamptz IS NULL OR ucv.viewed_at <= $4)
    GROUP BY ucv.content_id
)
SELECT c.id, c.title, c.content_type, c.status,
       COALESCE(pv.views, 0)::bigint AS view_count,
       c.comment_count, c.share_count, c.created_at
FROM contents c
LEFT JOIN period_views pv ON pv.content_id = c.id
WHERE c.tenant_id = $1
  AND c.author_id = $2
  AND c.deleted_at IS NULL
ORDER BY COALESCE(pv.views, 0) DESC, c.view_count DESC NULLS LAST, c.created_at DESC
LIMIT 10;

-- name: GetUserContentByPeriod :many
-- Paginated, period-scoped per-content analytics for the authenticated user.
-- Returns all non-deleted content items owned by the user, ranked by views
-- within the supplied date range. Falls back to all-time view_count for ordering
-- when a post had no views in the period. Supports GET /me/analytics/content?period=.
-- Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
WITH period_views AS (
    SELECT ucv.content_id, COUNT(*)::bigint AS views
    FROM user_content_views ucv
    JOIN contents c ON c.id = ucv.content_id
    WHERE c.tenant_id = $1
      AND c.author_id = $2
      AND c.deleted_at IS NULL
      AND ($3::timestamptz IS NULL OR ucv.viewed_at >= $3)
      AND ($4::timestamptz IS NULL OR ucv.viewed_at <= $4)
    GROUP BY ucv.content_id
)
SELECT c.id, c.title, c.content_type, c.status,
       COALESCE(pv.views, 0)::bigint AS view_count,
       c.comment_count, c.share_count, c.created_at
FROM contents c
LEFT JOIN period_views pv ON pv.content_id = c.id
WHERE c.tenant_id = $1
  AND c.author_id = $2
  AND c.deleted_at IS NULL
ORDER BY COALESCE(pv.views, 0) DESC, c.view_count DESC NULLS LAST, c.created_at DESC
LIMIT $5 OFFSET $6;

-- name: GetUserFollowerGrowth :many
-- Section 40: daily follower AND following growth for the authenticated user.
-- Counts both users who followed this user (new_followers) and users this
-- user followed (new_following), grouped by day.
WITH followers AS (
    SELECT date_trunc('day', f.created_at)::date AS day,
           COUNT(*)::bigint AS new_followers
    FROM follows f
    WHERE f.tenant_id = $1
      AND f.entity_type = 'user'
      AND f.entity_id = $2
      AND ($3::date IS NULL OR f.created_at::date >= $3)
      AND ($4::date IS NULL OR f.created_at::date <= $4)
    GROUP BY 1
),
following AS (
    SELECT date_trunc('day', f.created_at)::date AS day,
           COUNT(*)::bigint AS new_following
    FROM follows f
    WHERE f.tenant_id = $1
      AND f.entity_type = 'user'
      AND f.follower_id = $2
      AND ($3::date IS NULL OR f.created_at::date >= $3)
      AND ($4::date IS NULL OR f.created_at::date <= $4)
    GROUP BY 1
),
days AS (
    SELECT day FROM followers
    UNION
    SELECT day FROM following
)
SELECT d.day,
       COALESCE(f.new_followers, 0) AS new_followers,
       COALESCE(g.new_following, 0) AS new_following
FROM days d
LEFT JOIN followers f ON f.day = d.day
LEFT JOIN following g ON g.day = d.day
ORDER BY d.day DESC;

-- name: UpdateUserGeoSpace :exec
-- Sets the user's current primary geo space (Section 05, ADR-005).
UPDATE users
SET current_geo_space_id = $1, updated_at = now()
WHERE id = $2 AND tenant_id = $3;

-- name: GetUserCurrentGeoSpace :one
-- ADR-064a: Fetches the user's current_geo_space_id and resolves the space
-- hierarchy (city → state → country) to extract geo_code and names for
-- VisitorContext city-level personalization. Returns the city-level space,
-- its parent (state), and the country-level space's geo_code in one round-trip.
-- BUG-VIS-002: city_name is only populated when the space is city-level
-- (CASE WHEN city.level = 'city'). space_name is always the space's name
-- regardless of level, used by GET /me/location to display the user's
-- location even when it's a state or country (BUG-VIS-003).
SELECT
    u.current_geo_space_id,
    city.id AS city_id,
    COALESCE(CASE WHEN city.level = 'city' THEN city.name END, '')::text AS city_name,
    city.geo_code AS city_geo_code,
    city.name AS space_name,
    state.id AS state_id,
    state.name AS state_name,
    country.id AS country_id,
    country.name AS country_name,
    country.geo_code AS country_geo_code
FROM users u
LEFT JOIN spaces city ON city.id = u.current_geo_space_id AND city.tenant_id = u.tenant_id AND city.deleted_at IS NULL
LEFT JOIN spaces state ON state.id = COALESCE(
    -- If the current geo space is a state, use it directly.
    CASE WHEN city.level = 'state' THEN city.id END,
    -- If it's a city, use its parent state.
    CASE WHEN city.level = 'city' AND city.parent_id IS NOT NULL THEN city.parent_id END
) AND state.tenant_id = u.tenant_id AND state.deleted_at IS NULL
LEFT JOIN spaces country ON country.id = COALESCE(
    -- If the current geo space is a country itself, use it.
    CASE WHEN city.level = 'country' THEN city.id END,
    -- If it's a state, find its parent country.
    CASE WHEN city.level = 'state' AND city.parent_id IS NOT NULL THEN city.parent_id END,
    -- If it's a city, find its grandparent country (city → state → country).
    (SELECT s2.parent_id FROM spaces s2 WHERE s2.id = city.parent_id AND s2.level = 'state' LIMIT 1)
) AND country.tenant_id = u.tenant_id AND country.deleted_at IS NULL
WHERE u.id = @user_id AND u.tenant_id = @tenant_id;

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

-- name: CountUserContentCreatedInPeriod :one
-- Counts content created by the user within a date range, broken down by status.
-- Used for the 'Posts' metric card period-over-period delta.
-- Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
SELECT
    COUNT(*)::int AS total,
    COUNT(*) FILTER (WHERE status = 'draft')::int AS draft_count,
    COUNT(*) FILTER (WHERE status = 'published')::int AS published_count
FROM contents
WHERE tenant_id = $1 AND author_id = $2 AND deleted_at IS NULL
  AND ($3::timestamptz IS NULL OR created_at >= $3)
  AND ($4::timestamptz IS NULL OR created_at <= $4);

-- name: CountUserBookmarks :one
-- Counts the user's bookmarks in the unified bookmarks table.
SELECT COUNT(*)::int AS count
FROM bookmarks b
WHERE b.user_id = @user_id AND b.tenant_id = @tenant_id;

-- name: CountUserFollowStats :one
-- Returns the follower_count (denormalized on users) and following_count
-- (computed from the follows table) for the user.
-- Ref: japaconnect-engineering/sections/31-follow-system.md (denormalized counts)
SELECT
    COALESCE(u.follower_count, 0)::int AS follower_count,
    (SELECT COUNT(*)::int FROM follows f
       WHERE f.tenant_id = u.tenant_id AND f.follower_id = u.id) AS following_count
FROM users u
WHERE u.id = @user_id AND u.tenant_id = @tenant_id;

-- name: CountUserFollowersGained :one
-- Number of new user-entity follows for the user within a date range.
-- Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
SELECT COUNT(*)::bigint AS count
FROM follows
WHERE tenant_id = $1
  AND entity_type = 'user'
  AND entity_id = $2
  AND ($3::timestamptz IS NULL OR created_at >= $3)
  AND ($4::timestamptz IS NULL OR created_at <= $4);

-- name: GetUserDailyImpressions :many
-- Daily unique content views for the user's own content within a date range.
-- Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
SELECT
    date_trunc('day', viewed_at AT TIME ZONE 'UTC')::date AS day,
    COUNT(*)::bigint AS views
FROM user_content_views ucv
JOIN contents c ON c.id = ucv.content_id
WHERE c.tenant_id = $1
  AND c.author_id = $2
  AND c.deleted_at IS NULL
  AND ($3::timestamptz IS NULL OR ucv.viewed_at >= $3)
  AND ($4::timestamptz IS NULL OR ucv.viewed_at <= $4)
GROUP BY 1
ORDER BY 1 ASC;

-- name: GetUserReadersByCountry :many
-- Top countries reading the user's content within a date range.
-- NULL/empty country codes are grouped together; the frontend maps those
-- rows to the "Unknown" label so the chart is never empty in development
-- or when GeoIP cannot resolve the viewer's country.
-- Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
SELECT
    COALESCE(NULLIF(ucv.country_code, ''), '')::text AS country_code,
    COUNT(*)::bigint AS readers
FROM user_content_views ucv
JOIN contents c ON c.id = ucv.content_id
WHERE c.tenant_id = $1
  AND c.author_id = $2
  AND c.deleted_at IS NULL
  AND ($3::timestamptz IS NULL OR ucv.viewed_at >= $3)
  AND ($4::timestamptz IS NULL OR ucv.viewed_at <= $4)
GROUP BY 1
ORDER BY 2 DESC
LIMIT 10;

-- name: GetUserPeriodEngagement :one
-- Total views, reactions and comments on the user's own content for a period.
-- Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
SELECT
    COALESCE(v.views, 0)::bigint AS views,
    COALESCE(r.reactions, 0)::bigint AS reactions,
    COALESCE(cm.comments, 0)::bigint AS comments
FROM
    (SELECT COUNT(*)::bigint AS views
     FROM user_content_views ucv
     JOIN contents c ON c.id = ucv.content_id
     WHERE c.tenant_id = $1
       AND c.author_id = $2
       AND c.deleted_at IS NULL
       AND ($3::timestamptz IS NULL OR ucv.viewed_at >= $3)
       AND ($4::timestamptz IS NULL OR ucv.viewed_at <= $4)
    ) v,
    (SELECT COUNT(*)::bigint AS reactions
     FROM reactions r
     JOIN contents c ON c.id = r.entity_id AND r.entity_type = 'content'
     WHERE c.tenant_id = $1
       AND c.author_id = $2
       AND c.deleted_at IS NULL
       AND ($3::timestamptz IS NULL OR r.created_at >= $3)
       AND ($4::timestamptz IS NULL OR r.created_at <= $4)
    ) r,
    (SELECT COUNT(*)::bigint AS comments
     FROM comments cm
     JOIN contents c ON c.id = cm.content_id
     WHERE c.tenant_id = $1
       AND c.author_id = $2
       AND c.deleted_at IS NULL
       AND ($3::timestamptz IS NULL OR cm.created_at >= $3)
       AND ($4::timestamptz IS NULL OR cm.created_at <= $4)
    ) cm;

-- name: GetSyntheticUserByUsername :one
SELECT u.id
FROM users u
WHERE u.tenant_id = @tenant_id AND u.username = @username AND u.is_synthetic = true;

-- name: CreateSyntheticUser :one
INSERT INTO users (
    tenant_id, email, username, password_hash, profile_type, is_synthetic, synthetic_source
) VALUES (
    @tenant_id, @email, @username, '', 'regular', true, @synthetic_source
)
RETURNING id;

-- name: GetUserForVideoClaim :one
-- Resolves a username to a user identity for the video claim/takedown flow.
SELECT id, is_synthetic, synthetic_source
FROM users
WHERE tenant_id = @tenant_id AND username = @username;

-- name: GetUserSyntheticStatusByID :one
-- Returns whether the user is a synthetic channel account.
SELECT is_synthetic
FROM users
WHERE tenant_id = @tenant_id AND id = @user_id;

-- name: UpdateUserSourceID :exec
-- Link a user to its canonical intel_sources row (v2 source directory).
UPDATE users
SET source_id = $1, updated_at = now()
WHERE id = $2 AND tenant_id = $3;

-- name: UpdateUserSyntheticToReal :exec
-- Promote a synthetic channel user to a real user after a claim is approved.
-- The user row is kept; only the synthetic flag and source marker are cleared.
UPDATE users
SET is_synthetic = false, synthetic_source = NULL, updated_at = now()
WHERE id = @user_id AND tenant_id = @tenant_id AND is_synthetic = true;
