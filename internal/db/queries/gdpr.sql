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
SELECT cr.id, cr.tenant_id, cr.user_id, cr.consent_type, cr.granted, cr.policy_version,
    cr.ip_address, cr.user_agent, cr.created_at,
    u.username, up.display_name
FROM consent_records cr
LEFT JOIN users u ON u.id = cr.user_id
LEFT JOIN user_profiles up ON up.user_id = cr.user_id
WHERE cr.tenant_id = @tenant_id
  AND (@consent_type::text = '' OR cr.consent_type = @consent_type::text)
  AND (@q::text = '' OR cr.consent_type ILIKE '%' || @q || '%' OR cr.ip_address::text ILIKE '%' || @q || '%')
ORDER BY cr.created_at DESC
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
SELECT adr.id, adr.tenant_id, adr.user_id, adr.status, adr.reason, adr.completed_at, adr.created_at, adr.progress_step,
       u.username, up.display_name
FROM account_deletion_requests adr
LEFT JOIN users u ON u.id = adr.user_id
LEFT JOIN user_profiles up ON up.user_id = adr.user_id
WHERE adr.status = 'pending'
ORDER BY adr.created_at ASC
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

-- name: AnonymizeUserProfile :exec
-- Clears PII columns in user_profiles during the GDPR retention window
-- (ADR-014 / ADR-057). Runs immediately after AnonymizeUser so the profile
-- row — display_name, bio, timezone, business info, content_country_preference
-- (which reveals country origin/preference), preferred_currency, avatar/cover
-- URLs — is scrubbed before the hard-delete phase. The row itself is preserved
-- (FK integrity) until HardDeleteUser cascades.
UPDATE user_profiles
SET display_name = 'Deleted User',
    avatar_url = NULL,
    cover_url = NULL,
    bio = NULL,
    timezone = NULL,
    business_name = NULL,
    registration_number = NULL,
    operating_hours = NULL,
    content_country_preference = NULL,
    preferred_currency = NULL,
    updated_at = now()
WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: AnonymizeAffiliateLinkEvents :exec
-- Scrubs PII (ip_hash, user_agent_hash) from affiliate_link_events owned by
-- the deleted user's affiliate links during the GDPR retention window
-- (Section 56, ADR-014). The events themselves are retained for commission
-- audit; only the tracking hashes are cleared. Best-effort: a failure logs a
-- warning but does not abort the erasure.
UPDATE affiliate_link_events
SET ip_hash = NULL, user_agent_hash = NULL
WHERE link_id IN (
    SELECT ual.id FROM user_affiliate_links ual
    WHERE ual.user_id = @user_id AND ual.tenant_id = @tenant_id
);

-- name: DeleteUserTokens :exec
UPDATE refresh_tokens
SET revoked_at = now(), updated_at = now()
WHERE user_id = @user_id AND tenant_id = @tenant_id AND revoked_at IS NULL;

-- =====================
-- Auth PII Scrub (AUDIT-AUTH-008)
-- These auth-related tables contain sensitive PII (encrypted TOTP secrets,
-- OAuth provider user IDs, email addresses, password hashes, reset tokens)
-- that must be scrubbed during the GDPR anonymization phase — not just at
-- hard delete. Without this, an admin querying these tables during the
-- retention window can see the deleted user's OAuth identities, TOTP
-- secrets, and email addresses. Ref: sections/02-authentication-sessions.md,
-- ADR-014.
-- =====================

-- name: DeleteUserTOTPCredentials :exec
-- totp_credentials.secret_encrypted and backup_codes are PII. Delete the
-- rows during anonymization so 2FA secrets are not recoverable during the
-- retention window. The table has ON DELETE CASCADE from users(id), so this
-- is redundant at hard-delete but necessary for the anonymization phase.
DELETE FROM totp_credentials WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: DeleteUserOAuthAccounts :exec
-- oauth_accounts.provider_user_id is PII (the user's identity at the OAuth
-- provider). Delete the rows during anonymization so the user's social
-- identities are not recoverable during the retention window.
DELETE FROM oauth_accounts WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: DeleteUserPasswordResetTokens :exec
-- password_reset_tokens.token_hash is a security credential. Delete the
-- rows during anonymization so no reset tokens remain usable.
DELETE FROM password_reset_tokens WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: DeleteUserEmailVerifications :exec
-- email_verifications.email and token_hash are PII and security credentials.
-- Delete the rows during anonymization so the user's email and verification
-- tokens are not recoverable during the retention window.
DELETE FROM email_verifications WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: DeleteUserPasswordHistory :exec
-- password_history.password_hash is PII (bcrypt hashes of previous
-- passwords). Delete the rows during anonymization so old password hashes
-- are not recoverable during the retention window.
DELETE FROM password_history WHERE user_id = @user_id AND tenant_id = @tenant_id;

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

-- name: ReassignBlogSeriesAuthor :exec
-- C7: reassign blog_series authorship before user deletion to preserve
-- curated multi-part series (ADR-014 soft-delete-and-anonymize). Without
-- this, ON DELETE CASCADE on blog_series.author_user_id destroys all
-- user-authored series and cascades to blog_series_posts.
UPDATE blog_series SET author_user_id = @target_user_id
WHERE author_user_id = @user_id AND tenant_id = @tenant_id;

-- name: ReassignContentReviewReviewer :exec
-- C7: reassign content_reviews reviewer before user deletion to preserve
-- editorial audit history (ADR-014). Without this, ON DELETE CASCADE on
-- content_reviews.reviewer_user_id destroys all review records.
UPDATE content_reviews SET reviewer_user_id = @target_user_id
WHERE reviewer_user_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifyPaymentUserRefs :exec
-- Nulls payment-related user references for financial audit trail preservation.
UPDATE payments SET user_id = NULL
WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifySubscriptionUserRefs :exec
-- Nulls subscription user references for GDPR erasure (BUG-020).
-- Preserves subscription history for financial audit while removing PII.
UPDATE subscriptions SET user_id = NULL
WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifyDiscountCodeCreatorRefs :exec
-- Nulls discount code created_by references for GDPR erasure (BUG-020).
UPDATE discount_codes SET created_by = NULL
WHERE created_by = @user_id AND tenant_id = @tenant_id;

-- name: NullifyDiscountCodeUseUserRefs :exec
-- Nulls discount code use user_id references for GDPR erasure (BUG-020).
-- The PRIMARY KEY (code_id, user_id) requires setting user_id to a sentinel
-- UUID rather than NULL. We use the all-zeros UUID as the sentinel.
UPDATE discount_code_uses SET user_id = '00000000-0000-0000-0000-000000000000'::uuid
WHERE user_id = @user_id;

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
-- Per-tenant retention via system_settings 'audit.log_retention_days'
-- (default 365 days). Tenants without the setting keep the catalog default.
DELETE FROM audit_logs a
USING (
  SELECT a2.tenant_id,
         (COALESCE((SELECT s.value->>0
                    FROM system_settings s
                    WHERE s.tenant_id = a2.tenant_id AND s.key = 'audit.log_retention_days'),
                   '365')::int || ' days')::interval AS retention
  FROM (SELECT DISTINCT tenant_id FROM audit_logs) a2
) cfg
WHERE a.tenant_id = cfg.tenant_id
  AND a.created_at < now() - cfg.retention;

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

-- name: DeleteTagModeratorsByUser :exec
-- tag_moderators.user_id has ON DELETE CASCADE but we delete explicitly to document
-- the erasure path and ensure the user's moderation history is cleared before the
-- final HardDeleteUser. Ref: Section 08/49, GDPR erasure coverage.
DELETE FROM tag_moderators WHERE user_id = @user_id AND tenant_id = @tenant_id;

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
-- ownership_claims.claimant_user_id is NOT NULL; delete the user's submitted claims.
DELETE FROM ownership_claims WHERE claimant_user_id = @user_id AND tenant_id = @tenant_id;

-- name: NullifyOwnershipClaimReviewer :exec
-- ownership_claims.reviewed_by is nullable; clear reviewer reference when an admin is erased.
UPDATE ownership_claims SET reviewed_by = NULL WHERE reviewed_by = @user_id AND tenant_id = @tenant_id;

-- name: DeleteOrganizationMembersByUser :exec
-- organization_members.user_id is NOT NULL; remove the erased user's memberships to
-- prevent re-identification via the member roster. member_count on the org row is
-- decremented for each removed row.
DELETE FROM organization_members WHERE user_id = @user_id;

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

-- name: ReassignAIPersonaCreatedBy :exec
-- ai_personas.created_by is NOT NULL and references users(id). When an admin
-- who created personas is GDPR-erased, reassign created_by to the placeholder
-- so the persona config rows survive (BUG-016, ADR-068 compliance).
UPDATE ai_personas SET created_by = @target_user_id
WHERE created_by = @user_id AND tenant_id = @tenant_id;

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

-- name: GdprExportUserBookmarkFolders :one
-- Exports bookmark_folders for GDPR data export (BL-005).
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM bookmark_folders WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserContentShares :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM content_shares WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserContentViews :one
-- Exports user_content_views rows for GDPR data export (Section 14).
-- user_content_views contains user_id PII and must be included in the
-- right-to-access response and anonymized on erasure.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT content_id, session_id, country_code, viewed_at
      FROM user_content_views WHERE user_id = @user_id) t;

-- name: GdprDeleteUserContentViews :exec
-- Deletes user_content_views rows for the user being erased (Section 14,
-- ADR-014). The table has no tenant_id column — deletion is by user_id
-- only, which is safe because user_ids are globally unique.
DELETE FROM user_content_views WHERE user_id = @user_id;

-- name: GdprDeleteAdEventsByUser :exec
-- Deletes ad_events (impression/click tracking) for the user being erased.
-- ad_events.user_id has ON DELETE CASCADE, but we delete explicitly to
-- document the erasure path and remove the user's ad-interaction PII
-- before the final HardDeleteUser. Ref: Section 25/61, GDPR erasure.
DELETE FROM ad_events WHERE user_id = @user_id;

-- name: GdprNullifyAdCampaignCreator :exec
-- Nullifies ad_campaigns.created_by for campaigns created by the erased
-- user. The FK has no CASCADE/SET NULL, so without this step the final
-- HardDeleteUser would block on the residual foreign key. The campaign
-- row is preserved (it may belong to an organization). Ref: Section 25/61.
UPDATE ad_campaigns SET created_by = NULL WHERE created_by = @user_id;

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

-- name: GdprExportUserForumPollVotes :one
-- BUG-022: forum_poll_votes has no tenant_id column.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM forum_poll_votes WHERE user_id = @user_id) t;

-- name: GdprExportUserReplyViews :one
-- BUG-022: reply_views has no tenant_id column.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM reply_views WHERE user_id = @user_id) t;

-- name: GdprExportUserForumThreadTags :one
-- BUG-022: junction rows for threads authored by the user.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT ftt.* FROM forum_thread_tags ftt JOIN forum_threads ft ON ft.id = ftt.thread_id WHERE ft.author_user_id = @user_id AND ft.tenant_id = @tenant_id) t;

-- name: DeleteForumThreadTagsByAuthor :exec
-- BUG-022: delete junction rows for threads authored by the user during
-- GDPR erasure, so the final hard-delete doesn't leave orphaned rows.
DELETE FROM forum_thread_tags
WHERE thread_id IN (SELECT id FROM forum_threads WHERE author_user_id = @user_id AND tenant_id = @tenant_id);

-- name: GdprExportUserMedia :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM media WHERE uploader_user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserReviews :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM reviews WHERE reviewer_user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserSpaceMembers :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM space_members WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserSpaceModerators :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM space_moderators WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserTagModerators :one
-- Tag moderation history (Section 08/49). tag_moderators.user_id has ON DELETE CASCADE
-- so the final HardDeleteUser will remove these rows; this export captures them first.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM tag_moderators WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserConsentRecords :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM consent_records WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserWalletLedger :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM wallet_ledger WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserActivityEvents :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM activity_events WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserLessonProgress :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM lesson_progress WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserLearningCertificates :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM learning_certificates WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserMentorshipMatches :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM mentorship_matches WHERE (mentor_id = @user_id OR mentee_id = @user_id) AND tenant_id = @tenant_id) t;

-- name: GdprExportUserMentorshipConnections :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM mentorship_connections WHERE (mentor_id = @user_id OR mentee_id = @user_id) AND tenant_id = @tenant_id) t;

-- name: GdprExportUserMentorProfiles :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM mentorship_profiles WHERE user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserMentorSessions :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM mentor_sessions WHERE mentee_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserMentorReviews :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM mentor_reviews WHERE reviewer_id = @user_id) t;

-- name: GdprExportUserProfessionalProfiles :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM professional_profiles WHERE owner_user_id = @user_id AND tenant_id = @tenant_id) t;

-- name: GdprExportUserProfessionalRelocations :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT pr.* FROM professional_relocations pr
      JOIN professional_profiles pp ON pp.id = pr.profile_id
      WHERE pp.owner_user_id = @user_id AND pr.tenant_id = @tenant_id) t;

-- name: GdprExportUserProfessionalDocuments :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT pd.* FROM professional_documents pd
      JOIN professional_profiles pp ON pp.id = pd.profile_id
      WHERE pp.owner_user_id = @user_id AND pd.tenant_id = @tenant_id) t;

-- name: GdprExportUserProfessionalVerifiers :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT pv.* FROM professional_verifiers pv
      JOIN professional_profiles pp ON pp.id = pv.profile_id
      WHERE pp.owner_user_id = @user_id AND pv.tenant_id = @tenant_id) t;

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

-- =====================================================================
-- Japa University GDPR cascade (ADR-063 / audit C2)
-- revenue_splits and instructor_payouts are financial audit records:
-- anonymize the instructor reference (SET NULL), never delete the row.
-- instructor_profiles.payout_method is sensitive jsonb — clear it before
-- the profile is cascade-deleted via users(id) ON DELETE CASCADE.
-- =====================================================================

-- name: ClearInstructorPayoutMethod :execrows
-- Clears the sensitive payout_method jsonb on the user's instructor
-- profile(s) before the user row is physically deleted. The profile row
-- itself is removed by the users(id) ON DELETE CASCADE FK.
UPDATE instructor_profiles
SET payout_method = NULL,
    updated_at    = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: AnonymizeRevenueSplitsForInstructor :execrows
-- Severs the instructor reference on all revenue_split rows owned by
-- this user's instructor profile(s). The split rows survive for
-- regulatory retention; only the instructor link is anonymized.
UPDATE revenue_splits rs
SET instructor_id = NULL
WHERE rs.tenant_id = @tenant_id
  AND rs.instructor_id IN (
      SELECT ip.id FROM instructor_profiles ip
      WHERE ip.tenant_id = @tenant_id AND ip.user_id = @user_id
  );

-- name: AnonymizeInstructorPayoutsForInstructor :execrows
-- Severs the instructor reference on all instructor_payout rows owned by
-- this user's instructor profile(s). The payout rows survive for
-- regulatory retention; only the instructor link is anonymized.
UPDATE instructor_payouts ipo
SET instructor_id = NULL
WHERE ipo.tenant_id = @tenant_id
  AND ipo.instructor_id IN (
      SELECT ip.id FROM instructor_profiles ip
      WHERE ip.tenant_id = @tenant_id AND ip.user_id = @user_id
  );

-- =====================================================================
-- Japa University GDPR data export (ADR-063 / audit C2)
-- =====================================================================

-- name: GdprExportInstructorProfiles :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
    SELECT id, tenant_id, display_name, headline, bio, social_links,
           tier, status, kyc_status, application_note, created_at, updated_at
    FROM instructor_profiles
    WHERE tenant_id = @tenant_id AND user_id = @user_id
) t;

-- name: GdprExportRevenueSplits :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
    SELECT rs.id, rs.tenant_id, rs.track_id, rs.enrollment_id, rs.payment_id,
           rs.payout_id, rs.source, rs.gross_kobo, rs.creator_pct,
           rs.platform_pct, rs.creator_kobo, rs.platform_kobo,
           rs.currency, rs.created_at
    FROM revenue_splits rs
    JOIN instructor_profiles ip ON ip.id = rs.instructor_id
    WHERE rs.tenant_id = @tenant_id AND ip.user_id = @user_id
) t;

-- name: GdprExportInstructorPayouts :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
    SELECT ipo.id, ipo.tenant_id, ipo.period_start, ipo.period_end,
           ipo.gross_kobo, ipo.platform_commission_kobo, ipo.net_kobo,
           ipo.currency, ipo.status, ipo.payout_reference,
           ipo.processed_at, ipo.created_at
    FROM instructor_payouts ipo
    JOIN instructor_profiles ip ON ip.id = ipo.instructor_id
    WHERE ipo.tenant_id = @tenant_id AND ip.user_id = @user_id
) t;

-- name: AnonymizeComparisonItemsByAuthor :execrows
-- BUG-029: anonymize comparison_items.title/description for content authored by
-- the deleted user, so soft-deleted contents don't re-identify the user via
-- comparison item text. Runs before HardDeleteUser.
UPDATE comparison_items ci
SET title = '[anonymized]',
    description = NULL,
    image_id = NULL
FROM contents c
WHERE ci.content_id = c.id
  AND c.tenant_id = @tenant_id
  AND c.author_id = @user_id
  AND ci.deleted_at IS NULL;

-- name: DeleteCustomFieldValuesByEntity :execrows
-- BUG-018: delete custom_field_values for the user's profile entity so PII
-- stored in custom fields (e.g. Japa Status, phone, address) does not survive
-- account deletion. The custom_field_values table is polymorphic on
-- (entity_type, entity_id); we delete rows where entity_type='user_profile'
-- and entity_id is the user's ID. Also covers entity_type='user' if used.
DELETE FROM custom_field_values
WHERE entity_type IN ('user_profile', 'user') AND entity_id = @user_id;

-- name: AnonymizeFormSubmissionsByUser :execrows
-- BUG-019: anonymize form_submissions authored by the deleted user. The
-- submission row is preserved for analytics/audit (form submission counts,
-- trigger audit trail), but the user_id reference is severed and the PII
-- fields (ip_address, field_values jsonb) are scrubbed.
UPDATE form_submissions
SET user_id = NULL,
    ip_address = NULL,
    field_values = '{}'::jsonb
WHERE user_id = @user_id;

-- name: GdprExportCustomFieldValues :one
-- BUG-020: export custom_field_values for the user's profile entity so the
-- GDPR data export includes PII stored in custom fields (Japa Status, phone,
-- address, etc.). The custom_field_values table is polymorphic on
-- (entity_type, entity_id); we export rows where entity_type is
-- 'user_profile' or 'user' and entity_id is the user's ID.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
    SELECT cfv.field_id, cfv.entity_type, cfv.entity_id,
           cfv.value_text, cfv.value_number, cfv.value_bool,
           cfv.value_date, cfv.value_json, cfv.created_at, cfv.updated_at,
           cf.key AS field_key, cf.label AS field_label
    FROM custom_field_values cfv
    LEFT JOIN custom_fields cf ON cfv.field_id = cf.id
    WHERE cfv.entity_type IN ('user_profile', 'user') AND cfv.entity_id = @user_id
    ORDER BY cfv.created_at
) t;

-- name: GdprExportPushTokens :one
-- BUG-014: export push_tokens for the user so the GDPR data export includes
-- device push notification tokens (PII-adjacent device identifiers).
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
    SELECT id, platform, token, device_name, created_at, last_used_at
    FROM push_tokens
    WHERE user_id = @user_id AND tenant_id = @tenant_id
    ORDER BY created_at
) t;

-- name: GdprExportUserDMConversations :one
-- BUG-022: export dm_conversations where the user is a participant so the
-- GDPR data export includes DM conversation metadata.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
    SELECT id, tenant_id, participant_1_id, participant_2_id,
           last_message_at, last_message_preview,
           p1_unread_count, p2_unread_count, created_at, updated_at
    FROM dm_conversations
    WHERE tenant_id = @tenant_id
      AND (participant_1_id = @user_id OR participant_2_id = @user_id)
    ORDER BY created_at
) t;

-- name: GdprExportUserDMMessages :one
-- BUG-022: export dm_messages authored by the user so the GDPR data export
-- includes DM message content (PII: message bodies).
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
    SELECT dm.id, dm.conversation_id, dm.sender_id, dm.tenant_id,
           dm.body, dm.created_at, dm.read_at, dm.edited_at, dm.deleted_at
    FROM dm_messages dm
    WHERE dm.tenant_id = @tenant_id AND dm.sender_id = @user_id
    ORDER BY dm.created_at
) t;

-- name: GdprExportUserDMRequests :one
-- BUG-022: export dm_requests where the user is requester or recipient so
-- the GDPR data export includes DM request metadata and preview messages.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
    SELECT id, tenant_id, requester_id, recipient_id, conversation_id,
           preview_message, status, created_at, updated_at, resolved_at
    FROM dm_requests
    WHERE tenant_id = @tenant_id
      AND (requester_id = @user_id OR recipient_id = @user_id)
    ORDER BY created_at
) t;

-- name: GdprExportUserDMSettings :one
-- BUG-022: export user_dm_settings for the user so the GDPR data export
-- includes DM privacy preferences.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
    SELECT user_id, tenant_id, dm_privacy, updated_at
    FROM user_dm_settings
    WHERE tenant_id = @tenant_id AND user_id = @user_id
) t;

-- name: GdprExportUserBlocks :one
-- BUG-022: export user_blocks where the user is blocker or blocked so the
-- GDPR data export includes block relationships (PII-adjacent).
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
    SELECT blocker_id, blocked_id, created_at
    FROM user_blocks
    WHERE blocker_id = @user_id OR blocked_id = @user_id
    ORDER BY created_at
) t;

-- name: AnonymizeDMMessagesBySender :exec
-- BUG-024: anonymize dm_messages authored by the user during GDPR erasure.
-- The message body is cleared and the sender_id is reassigned to the
-- placeholder so the row survives HardDeleteUser for the other participant.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md, ADR-014
UPDATE dm_messages
SET body = '', sender_id = @target_user_id
WHERE sender_id = @user_id AND tenant_id = @tenant_id;

-- name: ReassignDMConversationParticipant1 :exec
-- BUG-024: reassign dm_conversations.participant_1_id to the placeholder
-- during GDPR erasure so the conversation survives for the other participant.
UPDATE dm_conversations
SET participant_1_id = @target_user_id
WHERE participant_1_id = @user_id AND tenant_id = @tenant_id;

-- name: ReassignDMConversationParticipant2 :exec
-- BUG-024: reassign dm_conversations.participant_2_id to the placeholder
-- during GDPR erasure so the conversation survives for the other participant.
UPDATE dm_conversations
SET participant_2_id = @target_user_id
WHERE participant_2_id = @user_id AND tenant_id = @tenant_id;

-- name: AnonymizeDMRequestsByUser :exec
-- BUG-024: anonymize dm_requests where the user is requester or recipient
-- during GDPR erasure. The preview_message is cleared and the user_id
-- fields are reassigned to the placeholder.
UPDATE dm_requests
SET preview_message = '',
    requester_id = CASE WHEN requester_id = @user_id THEN @target_user_id ELSE requester_id END,
    recipient_id = CASE WHEN recipient_id = @user_id THEN @target_user_id ELSE recipient_id END
WHERE tenant_id = @tenant_id
  AND (requester_id = @user_id OR recipient_id = @user_id);

-- name: DeleteUserDMSettingsByUser :exec
-- BUG-024: delete user_dm_settings for the user during GDPR erasure.
-- dm_privacy is user-owned config with no PII in the row itself.
DELETE FROM user_dm_settings WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: DeleteUserBlocksByUser :exec
-- BUG-024: delete user_blocks where the user is blocker or blocked during
-- GDPR erasure. Block relationships are user-owned config.
DELETE FROM user_blocks
WHERE blocker_id = @user_id OR blocked_id = @user_id;
