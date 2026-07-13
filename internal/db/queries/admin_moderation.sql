-- Admin moderation queries (migrated from inline SQL in admin/moderation.go)

-- =====================
-- Flagged Reviews
-- =====================

-- name: ListFlaggedReviews :many
SELECT r.id, r.body, r.status, r.created_at, r.reviewer_user_id,
       COALESCE(u.username, 'unknown') AS author_username
FROM reviews r
LEFT JOIN users u ON u.id = r.reviewer_user_id
WHERE r.tenant_id = $1
  AND r.status = 'flagged'
ORDER BY r.created_at DESC
LIMIT $2 OFFSET $3;

-- name: AdminUpdateReviewStatus :exec
UPDATE reviews
SET status = $1, updated_at = now()
WHERE id = $2 AND tenant_id = $3 AND status = 'flagged';

-- name: AppealReviewToFlagged :execrows
UPDATE reviews
SET status = 'flagged', updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND status IN ('removed');

-- =====================
-- User Moderation Actions
-- =====================

-- name: CreateUserBan :exec
INSERT INTO user_moderation_actions (user_id, tenant_id, action, reason, expires_at, created_at)
VALUES ($1, (SELECT tenant_id FROM users WHERE id = $1), 'ban', $2,
        CASE WHEN $3 > 0 THEN now() + ($3 || ' days')::interval ELSE NULL END, now())
ON CONFLICT DO NOTHING;

-- name: SoftDeleteUserBan :exec
UPDATE user_moderation_actions SET deleted_at = now()
WHERE user_id = $1 AND action = 'ban' AND deleted_at IS NULL;

-- name: ListActiveUserBans :many
SELECT id, action, reason, expires_at, created_at
FROM user_moderation_actions
WHERE user_id = $1 AND action = 'ban' AND deleted_at IS NULL
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC;

-- name: CreateUserWarning :exec
INSERT INTO user_moderation_actions (user_id, tenant_id, action, reason, created_at)
VALUES ($1, (SELECT tenant_id FROM users WHERE id = $1), 'warning', $2, now());

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
INSERT INTO user_group_memberships (user_id, group_id)
VALUES ($1, $2)
ON CONFLICT (user_id, group_id) DO NOTHING;

-- =====================
-- Organization Verification
-- =====================

-- name: VerifyOrganization :exec
UPDATE organizations SET verified = true, updated_at = now() WHERE id = $1;

-- name: UnverifyOrganization :exec
UPDATE organizations SET verified = false, updated_at = now() WHERE id = $1;

-- =====================
-- Appeals
-- =====================

-- name: ListModerationAppeals :many
SELECT id, user_id, entity_type, entity_id, reason, status, created_at
FROM moderation_appeals
ORDER BY created_at DESC
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
WHERE id = $4 AND tenant_id = $5;

-- =====================
-- Content Moderation
-- =====================

-- name: RemoveContentByAdmin :exec
UPDATE contents SET status = 'removed', updated_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: RestoreContentByAdmin :exec
UPDATE contents SET status = 'draft', updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND status = 'removed';

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
SELECT id FROM users WHERE tenant_id = $1 AND status = 'active';

-- name: CreateBroadcastNotifications :exec
INSERT INTO notifications (tenant_id, recipient_id, notification_type, actor_type, actor_id, title, body, created_at, updated_at)
SELECT $1, unnest($2::uuid[]), 'system_announcement', 'system', $3, $4, $5, now(), now();

-- name: CreateAdminBroadcastRecord :exec
INSERT INTO admin_broadcasts (title, body, sent_by, recipient_count, sent_at)
VALUES ($1, $2, $3, $4, now());
