-- Admin moderation queries (migrated from inline SQL in admin/moderation.go)

-- =====================
-- Flagged Reviews
-- =====================

-- name: ListFlaggedReviews :many
SELECT r.id, r.body, r.status, r.created_at, r.reviewer_user_id,
 COALESCE(u.username, 'unknown') AS author_username
FROM reviews r
LEFT JOIN users u ON u.id = r.reviewer_user_id
WHERE r.status = 'flagged'
ORDER BY r.created_at DESC
LIMIT $1 OFFSET $2;

-- name: AdminUpdateReviewStatus :exec
UPDATE reviews
SET status = $1, updated_at = now()
WHERE id = $2 AND status = 'flagged';

-- name: AppealReviewToFlagged :execrows
UPDATE reviews
SET status = 'flagged', updated_at = now()
WHERE id = $1 AND status IN ('removed');

-- =====================
-- User Moderation Actions
-- =====================

-- name: CreateUserBan :exec
INSERT INTO user_moderation_actions (user_id, action, reason, expires_at, created_at) VALUES ($1, 'ban', $2, CASE WHEN $3 > 0 THEN now() + ($3 || ' days')::interval ELSE NULL END, now())

ON CONFLICT DO NOTHING;

-- name: SoftDeleteUserBan :exec
UPDATE user_moderation_actions SET deleted_at = now()
WHERE user_id = $1 AND action = 'ban' AND deleted_at IS NULL;

-- name: CreateUserUnban :exec
INSERT INTO user_moderation_actions (user_id, action, reason, created_at) VALUES ($1, 'unban', 'Admin unbanned', now())
;

-- name: ListActiveUserBans :many
SELECT id, action, reason, expires_at, created_at
FROM user_moderation_actions
WHERE user_id = $1 AND action = 'ban' AND deleted_at IS NULL
 AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC;

-- name: CheckUserBanExists :one
-- AUDIT-BE-DB-001: replaces raw SQL existence check in ban enforcement middleware.
-- Returns true when an active (non-expired) ban or suspension exists for the user.
SELECT EXISTS(
 SELECT 1 FROM user_moderation_actions
 WHERE user_id = $1 AND action IN ('ban', 'suspension')
 AND (expires_at IS NULL OR expires_at > now())
);

-- name: GetUserBanExpiration :one
-- AUDIT-BE-DB-002: replaces raw SQL expiration lookup in ban enforcement middleware.
-- Returns the expires_at of the most recent active ban/suspension (may be NULL = permanent).
SELECT expires_at FROM user_moderation_actions
WHERE user_id = $1 AND action IN ('ban', 'suspension')
AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC LIMIT 1;

-- name: CreateUserWarning :exec
INSERT INTO user_moderation_actions (user_id, action, reason, created_at) VALUES ($1, 'warning', $2, now())
;

-- name: ListUserWarnings :many
SELECT id, reason, created_at
FROM user_moderation_actions
WHERE user_id = $1 AND action = 'warning' AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: SoftDeleteUserWarning :exec
UPDATE user_moderation_actions SET deleted_at = now()
WHERE id = $1 AND action = 'warning' AND deleted_at IS NULL;

-- =====================
-- User Status & Group
-- =====================

-- name: UpdateUserStatus :exec
UPDATE users SET status = $1, updated_at = now() WHERE id = $2;

-- name: AddUserGroupMembership :exec
INSERT INTO user_group_memberships (user_id, group_id) VALUES ($1, $2)

ON CONFLICT (user_id, group_id) DO NOTHING;

-- =====================
-- Organization Verification
-- =====================

-- name: VerifyOrganization :exec
-- Platform-scoped to prevent cross-platform verification (BUG-002).
UPDATE organizations SET verified = true, updated_at = now()
WHERE id = $1;

-- name: UnverifyOrganization :exec
-- Platform-scoped and clears verification_type (BUG-002, BUG-016).
UPDATE organizations SET verified = false, verification_type = NULL, updated_at = now()
WHERE id = $1;

-- =====================
-- Appeals
-- =====================

-- name: ListModerationAppeals :many
SELECT ma.id, ma.user_id, ma.entity_type, ma.entity_id, ma.reason, ma.status, ma.created_at,
 u.username, up.display_name
FROM moderation_appeals ma
LEFT JOIN users u ON u.id = ma.user_id
LEFT JOIN user_profiles up ON up.user_id = ma.user_id
WHERE (sqlc.narg(search)::text IS NULL OR (
 ma.entity_id::text ILIKE '%' || sqlc.narg(search) || '%'
 OR ma.reason ILIKE '%' || sqlc.narg(search) || '%'
 OR ma.entity_type ILIKE '%' || sqlc.narg(search) || '%'
 ))
ORDER BY ma.created_at DESC
LIMIT 50;

-- name: GetModerationAppeal :one
SELECT id, user_id, entity_type, entity_id, reason, status, created_at
FROM moderation_appeals
WHERE id = $1;

-- name: UpdateAppealStatus :exec
UPDATE moderation_appeals SET status = $1, updated_at = now() WHERE id = $2;

-- =====================
-- Content Reports
-- =====================

-- name: AssignContentReport :exec
UPDATE content_reports SET reviewed_by = $1, status = 'reviewing' WHERE id = $2;

-- name: ModerateContentReport :exec
UPDATE content_reports
SET status = $1, reviewed_by = $2, resolution_note = $3
WHERE id = $4;

-- =====================
-- Content Moderation
-- =====================

-- name: RemoveContentByAdmin :exec
UPDATE contents SET status = 'archived', auto_hidden = true, deleted_at = now(), updated_at = now()
WHERE id = $1;

-- name: RestoreContentByAdmin :exec
UPDATE contents SET status = 'draft', auto_hidden = false, deleted_at = NULL, updated_at = now()
WHERE id = $1 AND deleted_at IS NOT NULL;

-- name: ListDeletedContents :many
SELECT c.id, c.author_id, c.title, c.content_type, c.status,
 c.deleted_at, c.updated_at, c.created_at,
 u.username, up.display_name
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE c.deleted_at IS NOT NULL
ORDER BY c.deleted_at DESC NULLS LAST, c.updated_at DESC
LIMIT $1;

-- Admin list with soft-delete view selection: '' = live only (default),
-- 'only' = deleted only, 'all' = both. Powers the /admin/content page's
-- Deleted filter; kept separate from the public ListContents query so the
-- deleted view can never leak to unauthenticated callers.
-- name: AdminListContents :many
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body,
 c.status, c.visibility, c.view_count, c.comment_count, c.published_at,
 c.created_at, c.updated_at, c.deleted_at,
 EXISTS(SELECT 1 FROM content_featured cf WHERE cf.content_id = c.id
 AND (cf.featured_until IS NULL OR cf.featured_until > now())) AS is_featured,
 u.username AS author_username, up.display_name AS author_display_name,
 s.name AS space_name
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
WHERE (
 CASE @deleted::text
 WHEN 'only' THEN c.deleted_at IS NOT NULL
 WHEN 'all' THEN TRUE
 ELSE c.deleted_at IS NULL
 END
 )
 AND (@content_type::text = '' OR c.content_type = @content_type)
 AND (@status::text = '' OR c.status = @status)
 AND (@q::text = '' OR c.title ILIKE '%' || @q || '%' OR c.slug ILIKE '%' || @q || '%' OR c.body ILIKE '%' || @q || '%')
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountContents :one
SELECT COUNT(*)::int AS count
FROM contents c
WHERE (
 CASE @deleted::text
 WHEN 'only' THEN c.deleted_at IS NOT NULL
 WHEN 'all' THEN TRUE
 ELSE c.deleted_at IS NULL
 END
 )
 AND (@content_type::text = '' OR c.content_type = @content_type)
 AND (@status::text = '' OR c.status = @status)
 AND (@q::text = '' OR c.title ILIKE '%' || @q || '%' OR c.slug ILIKE '%' || @q || '%' OR c.body ILIKE '%' || @q || '%');

-- =====================
-- Broadcasts
-- =====================

-- name: ListAdminBroadcasts :many
SELECT id, title, body, sent_by, sent_at, recipient_count
FROM admin_broadcasts
ORDER BY sent_at DESC
LIMIT 50;

-- name: DeleteAdminBroadcast :exec
DELETE FROM admin_broadcasts WHERE id = $1;

-- =====================
-- Broadcast Helpers
-- =====================

-- name: ListActiveUserIDs :many
SELECT id FROM users WHERE status = 'active';

-- name: ListActiveUserIDsByGeoSpace :many
SELECT u.id FROM users u
JOIN space_members sm ON sm.user_id = u.id
WHERE u.status = 'active' AND sm.space_id = $1;

-- name: ListActiveUserIDsByGroup :many
SELECT u.id FROM users u
JOIN user_group_memberships gm ON gm.user_id = u.id
WHERE u.status = 'active' AND gm.group_id = $1;

-- name: ListSystemAdminIDs :many
-- Returns active user IDs in the system super_admin group.
-- Used by the notify_admin form trigger (Section 06) to fan out admin
-- notifications via the notification service.
SELECT u.id FROM users u
JOIN user_group_memberships gm ON gm.user_id = u.id
JOIN user_groups ug ON ug.id = gm.group_id
WHERE u.status = 'active' AND ug.slug = 'super_admin' AND ug.is_system = true;

-- name: CreateBroadcastNotifications :exec
INSERT INTO notifications (recipient_id, notification_type, actor_type, actor_id, title, body, created_at, updated_at)
SELECT $1, unnest($2::uuid[]), 'system_announcement', 'system', $3, $4, $5, now(), now();

-- name: CreateAdminBroadcastRecord :exec
INSERT INTO admin_broadcasts (title, body, sent_by, recipient_count, sent_at) VALUES ($1, $2, $3, $4, now())
;
