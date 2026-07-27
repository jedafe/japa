-- GDPR / NDPR Compliance — Section 19

-- =====================
-- Consent Records
-- =====================

-- name: RecordConsent :one
INSERT INTO consent_records (
    tenant_id, user_id, consent_type, granted, policy_version, ip_address, user_agent
) VALUES (
    @tenant_id, sqlc.narg(user_id), @consent_type, @granted, @policy_version,
    sqlc.narg(ip_address), sqlc.narg(user_agent)
)
RETURNING id, tenant_id, user_id, consent_type, granted, policy_version,
    ip_address, user_agent, created_at;

-- name: GetUserConsents :many
SELECT id, tenant_id, user_id, consent_type, granted, policy_version,
    ip_address, user_agent, created_at
FROM consent_records
WHERE user_id = @user_id
  AND (@consent_type = '' OR consent_type = @consent_type)
ORDER BY created_at DESC;

-- name: HasConsent :one
SELECT granted
FROM consent_records
WHERE user_id = @user_id AND consent_type = @consent_type
ORDER BY created_at DESC
LIMIT 1;

-- name: RevokeConsent :exec
INSERT INTO consent_records (
    tenant_id, user_id, consent_type, granted, policy_version, ip_address, user_agent
) VALUES (
    @tenant_id, @user_id, @consent_type, false, @policy_version,
    sqlc.narg(ip_address), sqlc.narg(user_agent)
);

-- name: ListConsentsByTenant :many
SELECT id, tenant_id, user_id, consent_type, granted, policy_version,
    ip_address, user_agent, created_at
FROM consent_records
WHERE tenant_id = @tenant_id
  AND (@consent_type::text = '' OR consent_type = @consent_type::text)
  AND (@q::text = '' OR consent_type ILIKE '%' || @q || '%' OR ip_address::text ILIKE '%' || @q || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: DeleteAuditLog :exec
DELETE FROM audit_logs WHERE id = @id;

-- name: DeleteConsent :one
DELETE FROM consent_records
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, user_id, consent_type, granted, policy_version,
    ip_address, user_agent, created_at;

-- =====================
-- GDPR Data Export Queries
-- =====================

-- name: GdprExportUserContent :many
SELECT id, title, slug, content_type, status, created_at
FROM contents
WHERE author_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: GdprExportUserComments :many
SELECT id, content_id, body, parent_id, created_at
FROM comments
WHERE author_user_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: GdprExportUserSpaces :many
SELECT s.id, s.name, s.slug, s.status, sm.role, sm.joined_at
FROM space_members sm
JOIN spaces s ON s.id = sm.space_id
WHERE sm.user_id = @user_id AND sm.tenant_id = @tenant_id
ORDER BY sm.joined_at DESC;

-- name: GdprExportUserPayments :many
SELECT id, provider, amount, currency, status, purpose_type, created_at
FROM payments
WHERE user_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: GdprExportUserNotifications :many
SELECT id, notification_type, title, body, read_at, created_at
FROM notifications
WHERE recipient_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC;

-- =====================
-- Data Export Requests
-- =====================

-- name: CreateDataExportRequest :one
INSERT INTO data_export_requests (
    tenant_id, user_id, status
) VALUES (
    @tenant_id, @user_id, 'pending'
)
RETURNING id, tenant_id, user_id, status, download_url, expires_at,
    completed_at, created_at;

-- name: GetDataExportRequest :one
SELECT id, tenant_id, user_id, status, download_url, expires_at,
    completed_at, created_at
FROM data_export_requests
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetLatestDataExportRequestByUser :one
SELECT id, tenant_id, user_id, status, download_url, expires_at,
    completed_at, created_at
FROM data_export_requests
WHERE user_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateDataExportRequestStatus :one
UPDATE data_export_requests
SET status = @status,
    download_url = sqlc.narg(download_url),
    expires_at = sqlc.narg(expires_at),
    completed_at = sqlc.narg(completed_at)
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, user_id, status, download_url, expires_at,
    completed_at, created_at;

-- name: ListPendingDataExportRequests :many
SELECT id, tenant_id, user_id, status, download_url, expires_at,
    completed_at, created_at
FROM data_export_requests
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT @page_limit;

-- =====================
-- Account Deletion Requests
-- =====================

-- name: CreateAccountDeletionRequest :one
INSERT INTO account_deletion_requests (
    tenant_id, user_id, status, reason
) VALUES (
    @tenant_id, @user_id, 'pending', sqlc.narg(reason)
)
RETURNING id, tenant_id, user_id, status, reason, completed_at, created_at, progress_step;

-- name: GetAccountDeletionRequest :one
SELECT id, tenant_id, user_id, status, reason, completed_at, created_at, progress_step
FROM account_deletion_requests
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateAccountDeletionRequestStatus :one
UPDATE account_deletion_requests
SET status = @status,
    completed_at = sqlc.narg(completed_at)
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, user_id, status, reason, completed_at, created_at, progress_step;

-- name: UpdateAccountDeletionRequestProgress :exec
-- AUDIT-FRESH-GDPR-003: checkpoints the erasure flow so a retried job resumes
-- from the last completed step instead of re-running from the top.
UPDATE account_deletion_requests
SET progress_step = @progress_step
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListPendingDeletionRequests :many
SELECT id, tenant_id, user_id, status, reason, completed_at, created_at, progress_step
FROM account_deletion_requests
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT @page_limit;

-- name: AnonymizeUser :exec
UPDATE users
SET username = 'Deleted User',
    email = 'deleted-' || id::text,
    password_hash = '',
    profile_type = NULL,
    status = 'deleted',
    updated_at = now()
WHERE id = @user_id AND tenant_id = @tenant_id;

-- name: DeleteUserTokens :exec
UPDATE refresh_tokens
SET revoked_at = now(), updated_at = now()
WHERE user_id = @user_id AND tenant_id = @tenant_id AND revoked_at IS NULL;

-- name: HardDeleteUser :exec
-- Physically deletes the anonymized user row. ON DELETE CASCADE handles most
-- child tables; callers must SET NULL or reassign non-CASCADE FK references
-- before calling this.
DELETE FROM users
WHERE id = @user_id AND tenant_id = @tenant_id AND status = 'deleted';

-- name: ReassignContentAuthor :exec
-- Reassigns content authorship before user deletion to preserve thread continuity.
UPDATE contents SET author_id = @target_user_id
WHERE author_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifyPaymentUserRefs :exec
-- Nulls payment-related user references for financial audit trail preservation.
UPDATE payments SET user_id = NULL
WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifyTicketSubmitter :exec
-- Nulls ticket submitter reference.
UPDATE tickets SET submitter_user_id = NULL
WHERE submitter_user_id = @user_id AND tenant_id = @tenant_id;

-- =====================
-- Audit Logs
-- =====================

-- name: CreateAuditLog :one
INSERT INTO audit_logs (
    tenant_id, actor_user_id, impersonated_user_id, action_type,
    target_type, target_id, metadata, ip_address
) VALUES (
    @tenant_id, sqlc.narg(actor_user_id), sqlc.narg(impersonated_user_id),
    @action_type, sqlc.narg(target_type), sqlc.narg(target_id),
    @metadata, sqlc.narg(ip_address)
)
RETURNING id, tenant_id, actor_user_id, impersonated_user_id, action_type,
    target_type, target_id, metadata, ip_address, created_at;

-- name: ListAuditLogs :many
SELECT id, tenant_id, actor_user_id, impersonated_user_id, action_type,
    target_type, target_id, metadata, ip_address, created_at
FROM audit_logs
WHERE tenant_id = @tenant_id
  AND (@actor_user_id = '00000000-0000-0000-0000-000000000000'::uuid OR actor_user_id = @actor_user_id)
  AND (@action_type = '' OR action_type = @action_type)
  AND (sqlc.narg(target_type) IS NULL OR target_type = @target_type)
  AND (sqlc.narg(start_date) IS NULL OR created_at >= @start_date)
  AND (sqlc.narg(end_date) IS NULL OR created_at <= @end_date)
  AND (@q::text = '' OR action_type ILIKE '%' || @q || '%' OR target_type ILIKE '%' || @q || '%' OR target_id::text ILIKE '%' || @q || '%' OR actor_user_id::text ILIKE '%' || @q || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListStalePendingDeletionRequests :many
SELECT id, tenant_id, user_id
FROM account_deletion_requests
WHERE status = 'pending'
  AND created_at < now() - interval '30 minutes'
ORDER BY created_at ASC
LIMIT 50;

-- name: ListRetentionWaitElapsed :many
SELECT r.id, r.tenant_id
FROM account_deletion_requests r
LEFT JOIN system_settings s ON s.tenant_id = r.tenant_id AND s.key = 'gdpr.retention_days'
WHERE r.status = 'retention_wait'
  AND r.created_at + ((COALESCE(s.value->>0, '30'))::int || ' days')::interval <= now()
ORDER BY r.created_at ASC
LIMIT 100;

-- name: PruneAuditLogs :exec
DELETE FROM audit_logs WHERE created_at < now() - interval '90 days';

-- =====================
-- Hard-Delete Erasure — Non-CASCADE FK Cleanup (AUDIT-FRESH-GDPR-001)
-- These tables reference users(id) without ON DELETE CASCADE/SET NULL, so the
-- final HardDeleteUser would block on a residual FK. Each query is run inside
-- the hardDeleteUser transaction before the physical DELETE.
-- =====================

-- name: DeleteContentRevisionsByEditor :exec
-- content_revisions.edited_by has no CASCADE; drop the user's edit history so the FK clears.
DELETE FROM content_revisions WHERE edited_by = @user_id;

-- name: DeleteSpacePinnedPostsByUser :exec
-- space_pinned_posts.pinned_by has no CASCADE; remove pins made by the user.
DELETE FROM space_pinned_posts WHERE pinned_by = @user_id;

-- name: NullifyForumTagModerator :exec
-- forum_tags.moderator_user_id is nullable; clear it so the tag survives without the user.
UPDATE forum_tags SET moderator_user_id = NULL
WHERE moderator_user_id = @user_id AND tenant_id = @tenant_id;

-- name: DeleteTicketCommentsByUser :exec
-- ticket_comments.user_id has no CASCADE; delete the user's ticket comments.
DELETE FROM ticket_comments WHERE user_id = @user_id;

-- name: NullifyTicketEventsUser :exec
-- ticket_events.user_id is nullable and is an audit trail; nullify the actor, keep the event.
UPDATE ticket_events SET user_id = NULL WHERE user_id = @user_id;

-- name: DeleteUserPreferences :exec
-- user_preferences is user-owned config; remove it explicitly (CASCADE is redundant but explicit).
DELETE FROM user_preferences WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: DeleteProfessionalProfilesByOwner :exec
-- professional_profiles.owner_user_id has no CASCADE; delete the user's owned profiles (cascades members/invitations).
DELETE FROM professional_profiles WHERE owner_user_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifyProfessionalProfileInvitationInviter :exec
-- professional_profile_invitations.invited_by is nullable; clear inviter reference on others' profiles.
UPDATE professional_profile_invitations SET invited_by = NULL WHERE invited_by = @user_id;

-- name: ReassignOrganizationOwner :exec
-- organizations.owner_user_id is NOT NULL; reassign to the deleted-user placeholder to preserve the org for remaining members.
UPDATE organizations SET owner_user_id = @target_user_id
WHERE owner_user_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifyOrganizationInvitationInviter :exec
-- organization_invitations.invited_by is nullable; clear inviter reference.
UPDATE organization_invitations SET invited_by = NULL WHERE invited_by = @user_id;

-- name: DeleteOwnershipClaimsByClaimant :exec
-- ownership_claims.claimant_user_id is NOT NULL; delete the user's submitted claims (reviewed_by is SET NULL).
DELETE FROM ownership_claims WHERE claimant_user_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifyConsentRecordUser :exec
-- consent_records.user_id is nullable; preserve the compliance record, detach the user reference.
UPDATE consent_records SET user_id = NULL WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: ReassignDataExportRequestUser :exec
-- data_export_requests.user_id is NOT NULL; reassign to placeholder to preserve the export audit trail.
UPDATE data_export_requests SET user_id = @target_user_id
WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: ReassignAccountDeletionRequestUser :exec
-- account_deletion_requests.user_id is NOT NULL; reassign to placeholder so the in-flight request row survives the delete.
UPDATE account_deletion_requests SET user_id = @target_user_id
WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: AnonymizeAuditLogActor :exec
-- audit_logs must be preserved for compliance; repoint actor/impersonated refs to the placeholder "Deleted User" label.
UPDATE audit_logs
SET actor_user_id = @target_user_id,
    impersonated_user_id = CASE WHEN impersonated_user_id = @user_id THEN @target_user_id ELSE impersonated_user_id END
WHERE (actor_user_id = @user_id OR impersonated_user_id = @user_id) AND tenant_id = @tenant_id;

-- =====================
-- GDPR Data Export — Additional User-Owned Tables (AUDIT-FRESH-GDPR-002)
-- Each query returns the user's rows as a JSON array so the service can attach
-- them to the export without per-table column mapping.
-- =====================

-- name: GdprExportUserProfiles :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM user_profiles WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserReactions :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM reactions WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserBookmarks :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM bookmarks WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserContentShares :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM content_shares WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserFollows :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM follows WHERE follower_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserForumThreads :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM forum_threads WHERE author_user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserForumReplies :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM forum_replies WHERE author_user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserForumVotes :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM forum_votes WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserForumBookmarks :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM forum_bookmarks WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserMedia :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM media WHERE uploader_user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserReviews :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM reviews WHERE reviewer_user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserSpaceMembers :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM space_members WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserConsentRecords :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM consent_records WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserWalletLedger :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM wallet_ledger WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserLessonProgress :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM lesson_progress WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserLearningCertificates :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM learning_certificates WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserMentorshipMatches :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM mentorship_matches WHERE (mentor_id = @user_id OR mentee_id = @user_id) AND tenant_id = @tenant_id) t;

-- name: GdprExportUserProfessionalProfiles :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM professional_profiles WHERE owner_user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserOrganizations :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM organizations WHERE owner_user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserApiKeys :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM api_keys WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserPreferences :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM user_preferences WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserMutes :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM user_mutes WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserSpaceMutes :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM space_mutes WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserTicketComments :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM ticket_comments WHERE user_id = @user_id) t;
