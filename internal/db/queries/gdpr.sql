-- name: RecordConsent :one
INSERT INTO consent_records (user_id, consent_type, granted, policy_version, ip_address, user_agent) VALUES (sqlc.narg(user_id), @consent_type, @granted, @policy_version, sqlc.narg(ip_address), sqlc.narg(user_agent))
RETURNING id, user_id, consent_type, granted, policy_version,
 ip_address, user_agent, created_at;

-- name: GetUserConsents :many
SELECT id, user_id, consent_type, granted, policy_version,
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
INSERT INTO consent_records (user_id, consent_type, granted, policy_version, ip_address, user_agent) VALUES (@user_id, @consent_type, false, @policy_version, sqlc.narg(ip_address), sqlc.narg(user_agent)) ;

-- name: ListConsents :many
SELECT cr.id, cr.user_id, cr.consent_type, cr.granted, cr.policy_version,
 cr.ip_address, cr.user_agent, cr.created_at,
 u.username, up.display_name
FROM consent_records cr
LEFT JOIN users u ON u.id = cr.user_id
LEFT JOIN user_profiles up ON up.user_id = cr.user_id
WHERE (@consent_type::text = '' OR cr.consent_type = @consent_type::text)
 AND (@q::text = '' OR cr.consent_type ILIKE '%' || @q || '%' OR cr.ip_address::text ILIKE '%' || @q || '%')
ORDER BY cr.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: DeleteConsent :one
DELETE FROM consent_records
WHERE id = @id
RETURNING id, user_id, consent_type, granted, policy_version,
 ip_address, user_agent, created_at;

-- =====================
-- GDPR Data Export Queries
-- =====================

-- name: GdprExportUserContent :many
SELECT id, title, slug, content_type, status, created_at
FROM contents
WHERE author_id = @user_id
ORDER BY created_at DESC;

-- name: GdprExportUserServiceListings :many
-- §98 BUG-008: export the user's service_listing_meta rows joined to
-- their content rows. The base content row is already exported by
-- GdprExportUserContent; this adds the structured service fields.
SELECT c.id AS content_id,
 c.title,
 c.slug,
 c.created_at,
 sm.service_category,
 sm.is_verified,
 sm.verification_type,
 sm.claim_status,
 sm.origin_communities,
 sm.is_remote,
 sm.website_url,
 sm.booking_url,
 sm.price_range
FROM contents c
JOIN service_listing_meta sm ON sm.content_id = c.id
WHERE c.author_id = @user_id
 AND c.content_type = 'service_listing'
 AND c.deleted_at IS NULL
ORDER BY c.created_at DESC;

-- name: GdprExportUserComments :many
SELECT id, content_id, body, parent_id, created_at
FROM comments
WHERE author_user_id = @user_id
ORDER BY created_at DESC;

-- name: GdprExportUserSpaces :many
SELECT s.id, s.name, s.slug, s.status, sm.role, sm.joined_at
FROM space_members sm
JOIN spaces s ON s.id = sm.space_id
WHERE sm.user_id = @user_id
ORDER BY sm.joined_at DESC;

-- name: GdprExportOwnedSpaceParents :many
-- Returns parent space IDs for spaces owned by the user (BUG-004: GDPR
-- export completeness for space_parent_spaces).
SELECT sps.space_id, sps.parent_space_id
FROM space_parent_spaces sps
JOIN space_members sm ON sm.space_id = sps.space_id
WHERE sm.user_id = @user_id AND sm.role = 'owner';

-- name: GdprExportOwnedSpaceRules :many
-- Returns rules for spaces owned by the user (BUG-004: GDPR export
-- completeness for space_rules).
SELECT sr.id, sr.space_id, sr.title, sr.body, sr.position
FROM space_rules sr
JOIN space_members sm ON sm.space_id = sr.space_id
WHERE sm.user_id = @user_id AND sm.role = 'owner'
ORDER BY sr.space_id, sr.position;

-- name: GdprExportUserPayments :many
SELECT id, provider, amount, currency, status, purpose_type, created_at
FROM payments
WHERE user_id = @user_id
ORDER BY created_at DESC;

-- name: GdprExportUserNotifications :many
SELECT id, notification_type, title, body, read_at, created_at
FROM notifications
WHERE recipient_id = @user_id
ORDER BY created_at DESC;

-- =====================
-- Data Export Requests
-- =====================

-- name: CreateDataExportRequest :one
INSERT INTO data_export_requests (user_id, status) VALUES (@user_id, 'pending')
RETURNING id, user_id, status, download_url, expires_at,
 completed_at, created_at;

-- name: GetDataExportRequest :one
SELECT id, user_id, status, download_url, expires_at,
 completed_at, created_at
FROM data_export_requests
WHERE id = @id;

-- name: GetLatestDataExportRequestByUser :one
SELECT id, user_id, status, download_url, expires_at,
 completed_at, created_at
FROM data_export_requests
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateDataExportRequestStatus :one
UPDATE data_export_requests
SET status = @status,
 download_url = sqlc.narg(download_url),
 expires_at = sqlc.narg(expires_at),
 completed_at = sqlc.narg(completed_at)
WHERE id = @id
RETURNING id, user_id, status, download_url, expires_at,
 completed_at, created_at;

-- name: ListPendingDataExportRequests :many
-- BUG-GDPR-002: scoped to the caller's platform so an admin in platform A
-- cannot see platform B's pending export requests (§5.3 cross-platform bleed).
SELECT id, user_id, status, download_url, expires_at,
 completed_at, created_at
FROM data_export_requests
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT @page_limit;

-- =====================
-- Account Deletion Requests
-- =====================

-- name: CreateAccountDeletionRequest :one
INSERT INTO account_deletion_requests (user_id, status, reason) VALUES (@user_id, 'pending', sqlc.narg(reason))
RETURNING id, user_id, status, reason, completed_at, created_at, progress_step;

-- name: GetAccountDeletionRequest :one
SELECT id, user_id, status, reason, completed_at, created_at, progress_step
FROM account_deletion_requests
WHERE id = @id;

-- name: UpdateAccountDeletionRequestStatus :one
UPDATE account_deletion_requests
SET status = @status,
 completed_at = sqlc.narg(completed_at)
WHERE id = @id
RETURNING id, user_id, status, reason, completed_at, created_at, progress_step;

-- name: UpdateAccountDeletionRequestProgress :exec
-- AUDIT-FRESH-GDPR-003: checkpoints the erasure flow so a retried job resumes
-- from the last completed step instead of re-running from the top.
UPDATE account_deletion_requests
SET progress_step = @progress_step
WHERE id = @id;

-- name: ListPendingDeletionRequests :many
-- BUG-GDPR-001: scoped to the caller's platform so an admin in platform A
-- cannot see platform B's pending deletion requests (§5.3 cross-platform bleed).
SELECT adr.id, adr.user_id, adr.status, adr.reason, adr.completed_at, adr.created_at, adr.progress_step,
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
WHERE id = @user_id;

-- name: AnonymizeUserProfile :exec
-- Clears PII columns in user_profiles during the GDPR retention window
-- (ADR-014 / ADR-057). Runs immediately after AnonymizeUser so the profile
-- row — display_name, bio, timezone, business info, content_country_preference
-- (which reveals country origin/preference), preferred_currency, avatar/cover
-- URLs — is scrubbed before the hard-delete phase. The row itself is preserved
-- (FK integrity) until HardDeleteUser cascades.
--
-- Also clears the japa_stage* columns (§87, ADR-095): japa_stage,
-- japa_stage_dest_space_id, japa_stage_history (an append-only log of the
-- member's relocation stage + destination over time — PII), and
-- japa_stage_is_public (revoked so no public directory leak survives).
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
 japa_stage = NULL,
 japa_stage_dest_space_id = NULL,
 japa_stage_history = '[]'::jsonb,
 japa_stage_is_public = false,
 japa_stage_updated_at = NULL,
 -- §102 Mentorship Matching (ADR-110): clear mentor PII — mentor_bio is
 -- personal text, mentor_embedding is derived PII, mentor_areas reveals
 -- expertise/destination knowledge, is_mentor reveals opt-in status.
 -- mentor_listing_content_id (000824) points at the mentor's shadow
 -- service listing; severing it keeps an erased mentor's profile from
 -- referencing the listing row during the retention window.
 is_mentor = false,
 mentor_capacity = NULL,
 active_mentee_count = NULL,
 mentor_areas = NULL,
 mentor_bio = NULL,
 mentor_since = NULL,
 mentor_embedding = NULL,
 mentor_listing_content_id = NULL,
 updated_at = now()
WHERE user_id = @user_id;

-- name: AnonymizeUserJourneys :exec
-- Scrubs PII from user_journeys rows (ADR-129): the member-supplied label,
-- destination, per-journey stage history, and visibility flag are erased
-- and every row is soft-deleted so nothing renders during the retention
-- window. Rows hard-delete with the user (user_id ON DELETE CASCADE) in
-- the later phase. Best-effort: a failure logs a warning but does not
-- abort the erasure.
UPDATE user_journeys
SET label = NULL,
    destination_space_id = NULL,
    history = '[]'::jsonb,
    is_public = false,
    deleted_at = now(),
    updated_at = now()
WHERE user_id = @user_id
  AND deleted_at IS NULL;

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
 WHERE ual.user_id = @user_id
);

-- name: AnonymizeCouponEvents :exec
-- Scrubs user_id PII from coupon_events recorded by the deleted user during
-- the GDPR retention window (Section 55, ADR-014). The events themselves are
-- retained for aggregate click/copy analytics; only the user association is
-- cleared. Best-effort: a failure logs a warning but does not abort the erasure.
UPDATE coupon_events
SET user_id = NULL
WHERE user_id = @user_id;

-- name: AnonymizeMentorshipShadowListings :exec
-- 000824: a mentor's shadow listing is a contents row (title =
-- '<name> — Mentorship', body = mentor_bio, slug = 'mentor-<username>')
-- backed by a listing_kind='mentorship' service_listing_meta row.
-- AnonymizeUserProfile severs user_profiles.mentor_listing_content_id but
-- leaves the shadow row itself live — published, discoverable, and
-- bookable with the mentor's name/bio intact — and ReassignContentAuthor
-- would later carry that PII to the placeholder user. This retires the
-- row (soft-delete per Rule 11 + status='archived' so it drops out of
-- discovery and booking probes) and blanks every PII-bearing column.
-- Already-tombstoned shadows are scrubbed too: SoftDeleteMentorshipShadow
-- (the opt-out path) only sets deleted_at and leaves title/body/slug
-- intact, so an opt-out-then-erased mentor would otherwise keep a
-- permanent PII tombstone. COALESCE preserves an existing deleted_at so
-- the scrub does not rewrite opt-out history.
-- slug is re-keyed to a deterministic per-row value so a restored or
-- re-created shadow can mint 'mentor-*' slugs without colliding on
-- idx_contents_slug; content_hash gets a per-row sha256 sentinel for the
-- same reason (idx_contents_content_hash is a partial unique over live
-- rows — an empty hash would collide across scrubbed rows if one were
-- ever un-deleted).
-- Must run in the anonymize phase, before the hard-delete reassignment;
-- hardDeleteUser also calls it inside its tx for the admin hard-delete
-- path that bypasses the anonymize pipeline.
UPDATE contents c
SET deleted_at = COALESCE(c.deleted_at, now()),
    status = 'archived',
    title = '',
    body = '',
    body_html = '',
    slug = 'mentor-deleted-' || c.id::text,
    content_hash = encode(sha256(convert_to('mentor-deleted:' || c.id::text, 'UTF8')), 'hex'),
    updated_at = now()
WHERE c.author_id = @user_id
  AND c.content_type = 'service_listing'
  AND EXISTS (
      SELECT 1 FROM service_listing_meta m
      WHERE m.content_id = c.id AND m.listing_kind = 'mentorship'
  );

-- name: DeleteUserTokens :exec
UPDATE refresh_tokens
SET revoked_at = now(), updated_at = now()
WHERE user_id = @user_id AND revoked_at IS NULL;

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
DELETE FROM totp_credentials WHERE user_id = @user_id;

-- name: DeleteUserOAuthAccounts :exec
-- oauth_accounts.provider_user_id is PII (the user's identity at the OAuth
-- provider). Delete the rows during anonymization so the user's social
-- identities are not recoverable during the retention window.
DELETE FROM oauth_accounts WHERE user_id = @user_id;

-- name: DeleteUserPasswordResetTokens :exec
-- password_reset_tokens.token_hash is a security credential. Delete the
-- rows during anonymization so no reset tokens remain usable.
DELETE FROM password_reset_tokens WHERE user_id = @user_id;

-- name: DeleteUserEmailVerifications :exec
-- email_verifications.email and token_hash are PII and security credentials.
-- Delete the rows during anonymization so the user's email and verification
-- tokens are not recoverable during the retention window.
DELETE FROM email_verifications WHERE user_id = @user_id;

-- name: DeleteUserPasswordHistory :exec
-- password_history.password_hash is PII (bcrypt hashes of previous
-- passwords). Delete the rows during anonymization so old password hashes
-- are not recoverable during the retention window.
DELETE FROM password_history WHERE user_id = @user_id;

-- name: HardDeleteUser :exec
-- Physically deletes the anonymized user row. ON DELETE CASCADE handles most
-- child tables; callers must SET NULL or reassign non-CASCADE FK references
-- before calling this.
DELETE FROM users
WHERE id = @user_id AND status = 'deleted';

-- name: ReassignContentAuthor :exec
-- Reassigns content authorship before user deletion to preserve thread continuity.
UPDATE contents SET author_id = @target_user_id
WHERE author_id = @user_id;

-- name: ReassignBlogSeriesAuthor :exec
-- C7: reassign blog_series authorship before user deletion to preserve
-- curated multi-part series (ADR-014 soft-delete-and-anonymize). Without
-- this, ON DELETE CASCADE on blog_series.author_user_id destroys all
-- user-authored series and cascades to blog_series_posts.
UPDATE blog_series SET author_user_id = @target_user_id
WHERE author_user_id = @user_id;

-- name: ReassignContentReviewReviewer :exec
-- C7: reassign content_reviews reviewer before user deletion to preserve
-- editorial audit history (ADR-014). Without this, ON DELETE CASCADE on
-- content_reviews.reviewer_user_id destroys all review records.
UPDATE content_reviews SET reviewer_user_id = @target_user_id
WHERE reviewer_user_id = @user_id;

-- name: ReassignMediaUploader :exec
-- BUG-009: reassign media uploader before user deletion so the user's
-- uploaded media survives the hard delete (Rule 11 — soft-delete only for
-- user content; only account:deletion_cleanup may hard-delete). Without
-- this, ON DELETE CASCADE on media.uploader_user_id physically deletes the
-- user's media rows, breaking featured/header images on posts whose
-- authorship was reassigned to the placeholder. The placeholder UUID is
-- resolved by hardDeleteUser (gdpr.deleted_user_placeholder_uuid setting).
-- Ref: japaconnect-engineering/sections/34-media-system.md, ADR-014.
UPDATE media SET uploader_user_id = @target_user_id
WHERE uploader_user_id = @user_id;

-- name: NullifyPaymentUserRefs :exec
-- Nulls payment-related user references for financial audit trail preservation.
UPDATE payments SET user_id = NULL
WHERE user_id = @user_id;

-- name: NullifySubscriptionUserRefs :exec
-- Nulls subscription user references for GDPR erasure (BUG-020).
-- Preserves subscription history for financial audit while removing PII.
UPDATE subscriptions SET user_id = NULL
WHERE user_id = @user_id;

-- name: NullifyDiscountCodeCreatorRefs :exec
-- Nulls discount code created_by references for GDPR erasure (BUG-020).
UPDATE discount_codes SET created_by = NULL
WHERE created_by = @user_id;

-- name: NullifyDiscountCodeUseUserRefs :exec
-- Nulls discount code use user_id references for GDPR erasure (BUG-020).
-- The PRIMARY KEY (code_id, user_id) requires setting user_id to a sentinel
-- UUID rather than NULL. We use the all-zeros UUID as the sentinel.
UPDATE discount_code_uses SET user_id = '00000000-0000-0000-0000-000000000000'::uuid
WHERE user_id = @user_id;

-- name: NullifyTicketSubmitter :exec
-- Nulls ticket submitter reference.
UPDATE tickets SET submitter_user_id = NULL
WHERE submitter_user_id = @user_id;

-- =====================
-- Audit Logs
-- =====================

-- name: CreateAuditLog :one
INSERT INTO audit_logs (actor_user_id, impersonated_user_id, action_type, target_type, target_id, metadata, ip_address) VALUES (sqlc.narg(actor_user_id), sqlc.narg(impersonated_user_id), @action_type, sqlc.narg(target_type), sqlc.narg(target_id), @metadata, sqlc.narg(ip_address))
RETURNING id, actor_user_id, impersonated_user_id, action_type,
 target_type, target_id, metadata, ip_address, created_at;

-- name: ListAuditLogs :many
SELECT id, actor_user_id, impersonated_user_id, action_type,
 target_type, target_id, metadata, ip_address, created_at
FROM audit_logs
WHERE (@actor_user_id = '00000000-0000-0000-0000-000000000000'::uuid OR actor_user_id = @actor_user_id)
 AND (@action_type = '' OR action_type = @action_type)
 AND (sqlc.narg('target_type')::text IS NULL OR target_type = sqlc.narg('target_type')::text)
 AND (sqlc.narg('start_date')::timestamptz IS NULL OR created_at >= sqlc.narg('start_date')::timestamptz)
 AND (sqlc.narg('end_date')::timestamptz IS NULL OR created_at <= sqlc.narg('end_date')::timestamptz)
 AND (sqlc.narg('q')::text = '' OR action_type ILIKE '%' || sqlc.narg('q') || '%' OR target_type ILIKE '%' || sqlc.narg('q') || '%' OR target_id::text ILIKE '%' || sqlc.narg('q') || '%' OR actor_user_id::text ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListStalePendingDeletionRequests :many
SELECT id, user_id
FROM account_deletion_requests
WHERE status = 'pending'
 AND created_at < now() - interval '30 minutes'
ORDER BY created_at ASC
LIMIT 50;

-- name: ListStalePendingDataExportRequests :many
-- GAP-GDPR-B: re-enqueue stale pending data export requests whose original
-- enqueue may have been lost (e.g. RequestDataExport's enqueuer.Enqueue
-- failed and only logged). Mirrors ListStalePendingDeletionRequests.
-- Platform-scoped at the call site; the sweep iterates all platforms.
SELECT id, user_id
FROM data_export_requests
WHERE status = 'pending'
 AND created_at < now() - interval '30 minutes'
ORDER BY created_at ASC
LIMIT 50;

-- name: ListRetentionWaitElapsed :many
-- BUG-GDPR-002 fix: COALESCE default must be '0' (immediate), not '30'.
-- ADR-057 and system-settings-catalog specify gdpr.retention_days
-- defaults to 0 (no waiting period). The previous '30' fallback caused
-- platforms without the explicit setting to wait 30 days instead of 0.
SELECT r.id
FROM account_deletion_requests r
LEFT JOIN system_settings s ON s.key = 'gdpr.retention_days'
WHERE r.status = 'retention_wait'
 AND r.created_at + ((COALESCE(s.value->>0, '0'))::int || ' days')::interval <= now()
ORDER BY r.created_at ASC
LIMIT 100;

-- name: ListExpiredDataExports :many
-- BUG-GDPR-005: find completed data export requests whose ExpiresAt has
-- passed so the sweep can delete the file from storage and null out the
-- download URL. Section 19 requires time-limited download links.
SELECT id
FROM data_export_requests
WHERE status = 'completed'
 AND expires_at IS NOT NULL
 AND expires_at < now()
 AND download_url IS NOT NULL
ORDER BY expires_at ASC
LIMIT 100;

-- name: ExpireDataExport :exec
-- BUG-GDPR-005: null out the download URL and set status to 'expired' so
-- the download handler returns 404 and the file is not re-cleaned.
UPDATE data_export_requests
SET download_url = NULL,
 status = 'expired'
WHERE id = $1;

-- name: PruneAuditLogs :exec
-- Global retention via system_settings 'audit.log_retention_days'
-- (default 365 days).
DELETE FROM audit_logs a
USING (
 SELECT (COALESCE((SELECT s.value->>0
 FROM system_settings s
 WHERE s.key = 'audit.log_retention_days'),
 '365')::int || ' days')::interval AS retention
) cfg
WHERE a.created_at < now() - cfg.retention;

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
WHERE moderator_user_id = @user_id;

-- name: DeleteTagModeratorsByUser :exec
-- tag_moderators.user_id has ON DELETE CASCADE but we delete explicitly to document
-- the erasure path and ensure the user's moderation history is cleared before the
-- final HardDeleteUser. Ref: Section 08/49, GDPR erasure coverage.
DELETE FROM tag_moderators WHERE user_id = @user_id;

-- name: DeleteTicketCommentsByUser :exec
-- ticket_comments.user_id has no CASCADE; delete the user's ticket comments.
DELETE FROM ticket_comments WHERE user_id = @user_id;

-- name: AnonymizeCommentsByAuthor :exec
-- GDPR-001: anonymize comments authored by the erased user. The
-- comments.author_user_id FK has ON DELETE SET NULL, so HardDeleteUser
-- will null it — but the comment body (TipTap JSON, potentially containing
-- PII and @mentions) must be cleared before the user is hard-deleted.
-- Sets body to a '[deleted]' JSON string and marks the comment as
-- soft-deleted so it is excluded from public queries. Ref: Section 15, ADR-014.
UPDATE comments
SET body = '"[deleted]"'::jsonb,
 deleted_at = now(),
 updated_at = now()
WHERE author_user_id = @user_id AND deleted_at IS NULL;

-- name: NullifyTicketEventsUser :exec
-- ticket_events.user_id is nullable and is an audit trail; nullify the actor, keep the event.
UPDATE ticket_events SET user_id = NULL WHERE user_id = @user_id;

-- name: DeleteUserPreferences :exec
-- user_preferences is user-owned config; remove it explicitly (CASCADE is redundant but explicit).
DELETE FROM user_preferences WHERE user_id = @user_id;

-- name: ReassignOrganizationOwner :exec
-- organizations.owner_user_id is NOT NULL; reassign to the deleted-user placeholder to preserve the org for remaining members.
UPDATE organizations SET owner_user_id = @target_user_id
WHERE owner_user_id = @user_id;

-- name: NullifyOrganizationInvitationInviter :exec
-- organization_invitations.invited_by is nullable; clear inviter reference.
UPDATE organization_invitations SET invited_by = NULL WHERE invited_by = @user_id;

-- name: DeleteOwnershipClaimsByClaimant :exec
-- ownership_claims.claimant_user_id is NOT NULL; delete the user's submitted claims.
DELETE FROM ownership_claims WHERE claimant_user_id = @user_id;

-- name: NullifyOwnershipClaimReviewer :exec
-- ownership_claims.reviewed_by is nullable; clear reviewer reference when an admin is erased.
UPDATE ownership_claims SET reviewed_by = NULL WHERE reviewed_by = @user_id;

-- name: DeleteOrganizationMembersByUser :exec
-- organization_members.user_id is NOT NULL; remove the erased user's memberships to
-- prevent re-identification via the member roster. member_count on the org row is
-- decremented for each removed row.
DELETE FROM organization_members WHERE user_id = @user_id;

-- name: NullifyConsentRecordUser :exec
-- consent_records.user_id is nullable; preserve the compliance record, detach the user reference.
UPDATE consent_records SET user_id = NULL WHERE user_id = @user_id;

-- name: ReassignDataExportRequestUser :exec
-- data_export_requests.user_id is NOT NULL; reassign to placeholder to preserve the export audit trail.
UPDATE data_export_requests SET user_id = @target_user_id
WHERE user_id = @user_id;

-- name: ReassignAccountDeletionRequestUser :exec
-- account_deletion_requests.user_id is NOT NULL; reassign to placeholder so the in-flight request row survives the delete.
UPDATE account_deletion_requests SET user_id = @target_user_id
WHERE user_id = @user_id;

-- name: GetUserIsBot :one
-- Returns the is_bot flag for a user. Used by RequestAccountDeletion to
-- reject erasure of AI persona backing accounts (ADR-068 compliance:
-- "Persona backing accounts are system accounts, not data subjects, so
-- user-erasure/export flows do not apply to them").
SELECT is_bot FROM users WHERE id = @user_id;

-- name: ReassignAIPersonaCreatedBy :exec
-- ai_personas.created_by is NOT NULL and references users(id). When an admin
-- who created personas is GDPR-erased, reassign created_by to the placeholder
-- so the persona config rows survive (BUG-016, ADR-068 compliance).
UPDATE ai_personas SET created_by = @target_user_id
WHERE created_by = @user_id;

-- name: AnonymizeAuditLogActor :exec
-- audit_logs must be preserved for compliance; repoint actor/impersonated refs to the placeholder "Deleted User" label.
UPDATE audit_logs
SET actor_user_id = @target_user_id,
 impersonated_user_id = CASE WHEN impersonated_user_id = @user_id THEN @target_user_id ELSE impersonated_user_id END
WHERE (actor_user_id = @user_id OR impersonated_user_id = @user_id);

-- =====================
-- GDPR Data Export — Additional User-Owned Tables (AUDIT-FRESH-GDPR-002)
-- Each query returns the user's rows as a JSON array so the service can attach
-- them to the export without per-table column mapping.
-- =====================

-- name: GdprExportUserProfiles :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM user_profiles WHERE user_id = @user_id) t;

-- name: GdprExportUserReactions :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM reactions WHERE user_id = @user_id) t;

-- name: GdprExportUserBookmarks :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM bookmarks WHERE user_id = @user_id) t;

-- name: GdprExportUserBookmarkFolders :one
-- Exports bookmark_folders for GDPR data export (BL-005).
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM bookmark_folders WHERE user_id = @user_id) t;

-- name: GdprExportUserContentShares :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM content_shares WHERE user_id = @user_id) t;

-- name: GdprExportUserContentViews :one
-- Exports user_content_views rows for GDPR data export (Section 14).
-- user_content_views contains user_id PII and must be included in the
-- right-to-access response and anonymized on erasure.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT content_id, session_id, country_code, viewed_at
 FROM user_content_views WHERE user_id = @user_id) t;

-- name: GdprDeleteUserContentViews :exec
-- Deletes user_content_views rows for the user being erased (Section 14,
-- ADR-014). The table has no the id column — deletion is by user_id
-- only, which is safe because user_ids are globally unique.
DELETE FROM user_content_views WHERE user_id = @user_id;

-- name: GdprDeleteAdEventsByUser :exec
-- Deletes ad_events (impression/click tracking) for the user being erased.
-- ad_events.user_id has ON DELETE CASCADE, but we delete explicitly to
-- document the erasure path and remove the user's ad-interaction PII
-- before the final HardDeleteUser. Ref: Section 25/61, GDPR erasure.
DELETE FROM ad_events WHERE user_id = @user_id;

-- name: GdprDeleteReactionsByUser :exec
-- Deletes reaction rows authored by the user. reactions.user_id has
-- ON DELETE CASCADE, but we delete explicitly so the erasure path is
-- self-documenting and survives future schema changes. Ref: BUG-011.
DELETE FROM reactions WHERE user_id = @user_id;

-- name: GdprDeleteContentSharesByUser :exec
-- Deletes content_shares rows authored by the user. content_shares.user_id
-- has ON DELETE CASCADE, but we delete explicitly so the erasure path is
-- self-documenting and survives future schema changes. Ref: BUG-011.
DELETE FROM content_shares WHERE user_id = @user_id;

-- name: GdprDeleteBookmarksByUser :exec
-- Deletes bookmark rows for the user. bookmarks.user_id has ON DELETE
-- CASCADE, but we delete explicitly so the erasure path is self-documenting
-- and survives future schema changes. Ref: BUG-011.
DELETE FROM bookmarks WHERE user_id = @user_id;

-- name: GdprDeleteBookmarkFoldersByUser :exec
-- Deletes custom bookmark folders for the user. bookmark_folders.user_id
-- has ON DELETE CASCADE, but we delete explicitly so the erasure path is
-- self-documenting and survives future schema changes. Ref: BUG-011.
DELETE FROM bookmark_folders WHERE user_id = @user_id;

-- name: GdprDeleteForumBookmarksByUser :exec
-- Deletes legacy forum bookmark rows for the user. forum_bookmarks.user_id has
-- ON DELETE CASCADE, but we delete explicitly so the erasure path is
-- self-documenting and survives future schema changes. Ref: BUG-011.
DELETE FROM forum_bookmarks WHERE user_id = @user_id;

-- name: GdprDeleteUnifiedForumBookmarksByUser :exec
-- Deletes consolidated forum bookmark rows from the unified bookmarks table.
-- Ref: §08 (forum bookmarks consolidated into bookmarks).
DELETE FROM bookmarks
WHERE user_id = @user_id AND entity_type IN ('forum_thread', 'forum_reply');

-- name: GdprNullifyAdCampaignCreator :exec
-- Nullifies ad_campaigns.created_by for campaigns created by the erased
-- user. The FK has no CASCADE/SET NULL, so without this step the final
-- HardDeleteUser would block on the residual foreign key. The campaign
-- row is preserved (it may belong to an organization). Ref: Section 25/61.
UPDATE ad_campaigns SET created_by = NULL WHERE created_by = @user_id;

-- name: GdprNullifyStudioJobCreator :exec
-- Nullifies studio_jobs.created_by for jobs created by the erased user. The
-- FK has no CASCADE/SET NULL, so without this step the final HardDeleteUser
-- would block on the residual foreign key. The job row is preserved (it may
-- have generated content in the review queue). Ref: Section 75, GDPR erasure.
UPDATE studio_jobs SET created_by = NULL WHERE created_by = @user_id;

-- name: GdprNullifyStudioBatchCreator :exec
-- Nullifies studio_batches.created_by for batches created by the erased user.
-- Same rationale as GdprNullifyStudioJobCreator. Ref: Section 75, GDPR erasure.
UPDATE studio_batches SET created_by = NULL WHERE created_by = @user_id;

-- name: GdprExportUserFollows :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM follows WHERE follower_id = @user_id) t;

-- name: GdprExportUserForumThreads :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM forum_threads WHERE author_user_id = @user_id) t;

-- name: GdprExportUserForumReplies :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM forum_replies WHERE author_user_id = @user_id) t;

-- name: GdprExportUserForumVotes :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM forum_votes WHERE user_id = @user_id) t;

-- name: GdprExportUserForumBookmarks :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT user_id, entity_type, entity_id, folder, folder_id, created_at
 FROM bookmarks
 WHERE user_id = @user_id
 AND entity_type IN ('forum_thread', 'forum_reply')
) t;

-- name: GdprExportUserForumPollVotes :one
-- BUG-022: forum_poll_votes has no the id column.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM forum_poll_votes WHERE user_id = @user_id) t;

-- name: GdprExportUserReplyViews :one
-- BUG-022: reply_views has no the id column.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM reply_views WHERE user_id = @user_id) t;

-- name: GdprExportUserForumThreadTags :one
-- BUG-022: junction rows for threads authored by the user.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT ftt.* FROM forum_thread_tags ftt JOIN forum_threads ft ON ft.id = ftt.thread_id WHERE ft.author_user_id = @user_id) t;

-- name: DeleteForumThreadTagsByAuthor :exec
-- BUG-022: delete junction rows for threads authored by the user during
-- GDPR erasure, so the final hard-delete doesn't leave orphaned rows.
DELETE FROM forum_thread_tags
WHERE thread_id IN (SELECT id FROM forum_threads WHERE author_user_id = @user_id);

-- name: GdprExportUserContentTags :one
-- Universal tag attachments on content authored by the user (Section 08/14 —
-- forum threads, blog posts, and every other content_type share content_tags).
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT ct.* FROM content_tags ct JOIN contents c ON c.id = ct.content_id WHERE c.author_id = @user_id) t;

-- name: DeleteContentTagsByAuthor :exec
-- Remove universal tag junction rows on the user's content during GDPR
-- erasure, matching the forum_thread_tags treatment above. The content_id FK
-- has ON DELETE CASCADE but contents may be retained soft-deleted.
DELETE FROM content_tags
WHERE content_id IN (SELECT id FROM contents WHERE author_id = @user_id);

-- name: GdprExportUserMedia :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM media WHERE uploader_user_id = @user_id) t;

-- name: GdprExportUserReviews :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM reviews WHERE reviewer_user_id = @user_id) t;

-- name: GdprExportUserSpaceMembers :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM space_members WHERE user_id = @user_id) t;

-- name: GdprExportUserSpaceModerators :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM space_moderators WHERE user_id = @user_id) t;

-- name: GdprExportUserTagModerators :one
-- Tag moderation history (Section 08/49). tag_moderators.user_id has ON DELETE CASCADE
-- so the final HardDeleteUser will remove these rows; this export captures them first.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM tag_moderators WHERE user_id = @user_id) t;

-- name: GdprExportUserConsentRecords :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM consent_records WHERE user_id = @user_id) t;

-- name: GdprExportUserWalletLedger :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM wallet_ledger WHERE user_id = @user_id) t;

-- name: GdprExportUserActivityEvents :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM activity_events WHERE user_id = @user_id) t;

-- name: GdprExportUserLessonProgress :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM academy_lesson_progress WHERE user_id = @user_id) t;

-- name: GdprExportUserAcademyCertificates :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM academy_certificates WHERE user_id = @user_id) t;

-- name: GdprExportUserMentorshipMatches :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM mentorship_matches WHERE (mentor_id = @user_id OR mentee_id = @user_id)) t;

-- name: GdprExportUserMentorshipConnections :one
-- 000824: mentorship_connections is dropped — connection state now lives in
-- service_bookings rows anchored to the mentor's listing_kind='mentorship'
-- shadow listing (provider_user_id = mentor, booker_user_id = mentee). The
-- export keeps the legacy field name + column vocabulary: booking status
-- maps back (confirmed→active, fulfilled→completed), goals reads
-- meta->>'goals' with the note column as fallback (migrated rows only carry
-- note), and responded_at comes from meta (DeclineServiceBooking / the
-- 000824 backfill) falling back to confirmed_at.
SELECT coalesce(json_agg(t ORDER BY t.created_at DESC), '[]'::json) AS data
FROM (
    SELECT sb.id,
           sb.provider_user_id AS mentor_id,
           sb.booker_user_id AS mentee_id,
           CASE sb.status
               WHEN 'confirmed' THEN 'active'
               WHEN 'fulfilled' THEN 'completed'
               ELSE sb.status
           END AS status,
           COALESCE(sb.meta->>'goals', sb.note) AS goals,
           sb.destination_space_id,
           COALESCE((sb.meta->>'responded_at')::timestamptz,
                    sb.confirmed_at) AS responded_at,
           sb.fulfilled_at AS completed_at,
           sb.cancelled_at,
           sb.cancel_reason,
           sb.created_at,
           sb.updated_at
    FROM service_bookings sb
    JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
    WHERE sm.listing_kind = 'mentorship'
      AND (sb.booker_user_id = @user_id OR sb.provider_user_id = @user_id)
) t;

-- name: GdprExportUserMentorProfiles :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM mentorship_profiles WHERE user_id = @user_id) t;

-- name: GdprExportUserMentorSessions :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM mentor_sessions WHERE mentee_id = @user_id) t;

-- name: GdprExportUserMentorReviews :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM mentor_reviews WHERE reviewer_id = @user_id) t;

-- name: GdprExportUserOrganizations :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM organizations WHERE owner_user_id = @user_id) t;

-- name: GdprExportUserApiKeys :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM api_keys WHERE user_id = @user_id) t;

-- name: GdprExportUserPreferences :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM user_preferences WHERE user_id = @user_id) t;

-- name: GdprExportUserMutes :one
-- BUG-008: export both muter and muted directions for GDPR portability.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM user_mutes WHERE (user_id = @user_id OR muted_user_id = @user_id)) t;

-- name: GdprExportUserSpaceMutes :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM space_mutes WHERE user_id = @user_id) t;

-- name: GdprExportUserTicketComments :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM ticket_comments WHERE user_id = @user_id) t;

-- =====================================================================
-- Comparison GDPR export (BUG-004)
-- comparison_items/dimensions/values are linked to content authored by the
-- user. The parent content rows are exported by GdprExportUserContent; these
-- queries export the linked comparison data so the user receives the full
-- data set they authored, per GDPR right to data portability.
-- =====================================================================

-- name: GdprExportUserComparisonItems :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT ci.id, ci.content_id, ci.title, ci.description, ci.position, ci.created_at
 FROM comparison_items ci
 JOIN contents c ON c.id = ci.content_id
 WHERE c.author_id = @user_id
 ORDER BY ci.created_at DESC
) t;

-- name: GdprExportUserComparisonDimensions :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT cd.id, cd.content_id, cd.label, cd.dimension_type, cd.position, cd.created_at
 FROM comparison_dimensions cd
 JOIN contents c ON c.id = cd.content_id
 WHERE c.author_id = @user_id
 ORDER BY cd.created_at DESC
) t;

-- name: GdprExportUserComparisonValues :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT cv.dimension_id, cv.item_id, cv.value, cv.value_rating, cv.value_boolean, cv.created_at
 FROM comparison_values cv
 JOIN comparison_items ci ON ci.id = cv.item_id
 JOIN contents c ON c.id = ci.content_id
 WHERE c.author_id = @user_id
 ORDER BY cv.created_at DESC
) t;

-- =====================================================================
-- Japa Academy GDPR cascade (ADR-063 / audit C2)
-- revenue_splits and academy_instructor_payouts are financial audit records:
-- anonymize the instructor reference (SET NULL), never delete the row.
-- academy_instructor_profiles.payout_method is sensitive jsonb — clear it before
-- the profile is cascade-deleted via users(id) ON DELETE CASCADE.
-- =====================================================================

-- name: ClearInstructorPayoutMethod :execrows
-- Clears the sensitive payout_method jsonb on the user's instructor
-- profile(s) before the user row is physically deleted. The profile row
-- itself is removed by the users(id) ON DELETE CASCADE FK.
UPDATE academy_instructor_profiles
SET payout_method = NULL,
 updated_at = now()
WHERE user_id = @user_id;

-- name: AnonymizeRevenueSplitsForInstructor :execrows
-- Severs the instructor reference on all revenue_split rows owned by
-- this user's instructor profile(s). The split rows survive for
-- regulatory retention; only the instructor link is anonymized.
UPDATE revenue_splits rs
SET instructor_id = NULL
WHERE rs.instructor_id IN (
 SELECT ip.id FROM academy_instructor_profiles ip
 WHERE ip.user_id = @user_id
 );

-- name: AnonymizeInstructorPayoutsForInstructor :execrows
-- Severs the instructor reference on all instructor_payout rows owned by
-- this user's instructor profile(s). The payout rows survive for
-- regulatory retention; only the instructor link is anonymized.
UPDATE academy_instructor_payouts ipo
SET instructor_id = NULL
WHERE ipo.instructor_id IN (
 SELECT ip.id FROM academy_instructor_profiles ip
 WHERE ip.user_id = @user_id
 );

-- =====================================================================
-- Japa Academy GDPR data export (ADR-063 / audit C2)
-- =====================================================================

-- name: GdprExportInstructorProfiles :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT id, display_name, headline, bio, social_links,
 tier, status, kyc_status, application_note, created_at, updated_at
 FROM academy_instructor_profiles
 WHERE user_id = @user_id
) t;

-- name: GdprExportRevenueSplits :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT rs.id, rs.track_id, rs.enrollment_id, rs.payment_id,
 rs.payout_id, rs.source, rs.gross_kobo, rs.creator_pct,
 rs.platform_pct, rs.creator_kobo, rs.platform_kobo,
 rs.currency, rs.created_at
 FROM revenue_splits rs
 JOIN academy_instructor_profiles ip ON ip.id = rs.instructor_id
 WHERE ip.user_id = @user_id
) t;

-- name: GdprExportInstructorPayouts :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT ipo.id, ipo.period_start, ipo.period_end,
 ipo.gross_kobo, ipo.platform_commission_kobo, ipo.net_kobo,
 ipo.currency, ipo.status, ipo.payout_reference,
 ipo.processed_at, ipo.created_at
 FROM academy_instructor_payouts ipo
 JOIN academy_instructor_profiles ip ON ip.id = ipo.instructor_id
 WHERE ip.user_id = @user_id
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
 AND c.author_id = @user_id
 AND ci.deleted_at IS NULL;

-- name: AnonymizeComparisonDimensionsByAuthor :execrows
-- BUG-003: anonymize comparison_dimensions.label for content authored by the
-- deleted user. The label is user-authored free text and can re-identify a
-- deleted user via their comparison content. Runs before HardDeleteUser.
UPDATE comparison_dimensions cd
SET label = '[anonymized]'
FROM contents c
WHERE cd.content_id = c.id
 AND c.author_id = @user_id
 AND cd.deleted_at IS NULL;

-- name: AnonymizeComparisonValuesByAuthor :execrows
-- BUG-003: anonymize comparison_values.value for content authored by the
-- deleted user. The value column is user-authored free text and can
-- re-identify a deleted user via their comparison content. Runs before
-- HardDeleteUser.
UPDATE comparison_values cv
SET value = '[anonymized]',
 value_rating = NULL,
 value_boolean = NULL
FROM comparison_items ci
JOIN contents c ON c.id = ci.content_id
WHERE cv.item_id = ci.id
 AND c.author_id = @user_id;

-- name: GdprExportUserComparisonDataIngestionJobs :one
-- JCE Sub-spec 4: export ingestion jobs for user-authored comparison content.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT j.id, j.content_id, j.status, j.source_kind,
 j.refreshed_count, j.error, j.started_at, j.completed_at,
 j.created_at, j.updated_at
 FROM comparison_data_ingestion_jobs j
 JOIN contents c ON c.id = j.content_id
 WHERE c.author_id = @user_id
 ORDER BY j.created_at
) t;

-- name: GdprExportUserComparisonDataSources :one
-- JCE Sub-spec 4: export user-owned data sources. This is empty until the
-- platform allows non-admin users to create data sources and populate
-- created_by_user_id.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT ds.id, ds.entity_type, ds.entity_id, ds.dimension_slug,
 ds.source_name, ds.source_url, ds.source_kind, ds.trust_level,
 ds.freshness_score, ds.last_verified_at, ds.config,
 ds.created_at, ds.updated_at
 FROM comparison_data_sources ds
 WHERE ds.created_by_user_id = @user_id
 ORDER BY ds.created_at
) t;

-- name: DeleteComparisonDataIngestionJobsByUser :exec
-- JCE Sub-spec 4: delete ingestion jobs tied to user-authored content during
-- GDPR erasure. comparison_data_ingestion_jobs.content_id is FK to contents
-- with ON DELETE CASCADE; we delete explicitly so the erasure path is
-- documented and survives future schema changes.
DELETE FROM comparison_data_ingestion_jobs j
USING contents c
WHERE j.content_id = c.id
 AND c.author_id = @user_id;

-- name: DeleteComparisonDataSourcesByUser :exec
-- JCE Sub-spec 4: delete user-owned data sources when the column is populated
-- (future non-admin data-source creation). Admin-created sources leave
-- created_by_user_id NULL and are therefore not removed on a member erasure.
DELETE FROM comparison_data_sources
WHERE created_by_user_id = @user_id;

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
 WHERE user_id = @user_id
 ORDER BY created_at
) t;

-- name: GdprExportUserDMConversations :one
-- BUG-022: export dm_conversations where the user is a participant so the
-- GDPR data export includes DM conversation metadata.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT id, participant_1_id, participant_2_id,
 last_message_at, last_message_preview,
 p1_unread_count, p2_unread_count, created_at, updated_at
 FROM dm_conversations
 WHERE (participant_1_id = @user_id OR participant_2_id = @user_id)
 ORDER BY created_at
) t;

-- name: GdprExportUserDMMessages :one
-- BUG-022: export dm_messages authored by the user so the GDPR data export
-- includes DM message content (PII: message bodies).
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT dm.id, dm.conversation_id, dm.sender_id, dm.body, dm.created_at, dm.read_at, dm.edited_at, dm.deleted_at
 FROM dm_messages dm
 WHERE dm.sender_id = @user_id
 ORDER BY dm.created_at
) t;

-- name: GdprExportUserDMRequests :one
-- BUG-022: export dm_requests where the user is requester or recipient so
-- the GDPR data export includes DM request metadata and preview messages.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT id, requester_id, recipient_id, conversation_id,
 preview_message, status, created_at, updated_at, resolved_at
 FROM dm_requests
 WHERE (requester_id = @user_id OR recipient_id = @user_id)
 ORDER BY created_at
) t;

-- name: GdprExportUserDMSettings :one
-- BUG-022: export user_dm_settings for the user so the GDPR data export
-- includes DM privacy preferences.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT user_id, dm_privacy, updated_at
 FROM user_dm_settings
 WHERE user_id = @user_id
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
WHERE sender_id = @user_id;

-- name: ReassignDMConversationParticipant1 :exec
-- BUG-024: reassign dm_conversations.participant_1_id to the placeholder
-- during GDPR erasure so the conversation survives for the other participant.
UPDATE dm_conversations
SET participant_1_id = @target_user_id
WHERE participant_1_id = @user_id;

-- name: ReassignDMConversationParticipant2 :exec
-- BUG-024: reassign dm_conversations.participant_2_id to the placeholder
-- during GDPR erasure so the conversation survives for the other participant.
UPDATE dm_conversations
SET participant_2_id = @target_user_id
WHERE participant_2_id = @user_id;

-- name: AnonymizeDMRequestsByUser :exec
-- BUG-024: anonymize dm_requests where the user is requester or recipient
-- during GDPR erasure. The preview_message is cleared and the user_id
-- fields are reassigned to the placeholder.
UPDATE dm_requests
SET preview_message = '',
 requester_id = CASE WHEN requester_id = @user_id THEN @target_user_id ELSE requester_id END,
 recipient_id = CASE WHEN recipient_id = @user_id THEN @target_user_id ELSE recipient_id END
WHERE (requester_id = @user_id OR recipient_id = @user_id);

-- name: DeleteUserDMSettingsByUser :exec
-- BUG-024: delete user_dm_settings for the user during GDPR erasure.
-- dm_privacy is user-owned config with no PII in the row itself.
DELETE FROM user_dm_settings WHERE user_id = @user_id;

-- name: DeleteUserBlocksByUser :exec
-- BUG-024: delete user_blocks where the user is blocker or blocked during
-- GDPR erasure. Block relationships are user-owned config.
DELETE FROM user_blocks
WHERE blocker_id = @user_id OR blocked_id = @user_id;

-- name: DeleteUserMutesByUser :exec
-- BE-2: delete user_mutes where the user is the muter or the muted party
-- during GDPR erasure. Mute relationships are user-owned config and must not
-- survive anonymization (the muted/muter UUID would reference an anonymized
-- user). Ref: Section 52, Section 15.
DELETE FROM user_mutes
WHERE user_id = @user_id OR muted_user_id = @user_id;

-- name: DeleteSpaceMutesByUser :exec
-- BE-2: delete space_mutes for the user during GDPR erasure. Space mutes are
-- user-owned feed-filter config. Ref: Section 52, Section 15.
DELETE FROM space_mutes
WHERE user_id = @user_id;

-- name: DeleteNotificationsByUser :exec
-- BUG-M006: delete notifications addressed to the erased user during GDPR
-- erasure (Section 19). Notification rows contain recipient_id and may
-- reference actor_user_id; the recipient's notifications are user-owned
-- data and must be removed. Actor references in other users' notifications
-- are anonymized separately if needed.
DELETE FROM notifications
WHERE recipient_id = @user_id;

-- name: DeleteNotificationPreferencesByUser :exec
-- BUG-011: delete notification_preferences for the erased user during GDPR
-- erasure (Section 35). Preferences are user-owned config with no audit
-- requirement; safe to hard-delete (not user content per Rule 11).
DELETE FROM notification_preferences WHERE user_id = @user_id;

-- name: DeleteNotificationBatchWindowsByUser :exec
-- BUG-012: delete notification_batch_windows for the erased user during GDPR
-- erasure (Section 35). Orphaned batch windows would dispatch notifications
-- to a deleted user; removing them prevents that.
DELETE FROM notification_batch_windows WHERE recipient_id = @user_id;

-- name: DeletePushSubscriptionsByUser :exec
-- BUG-010: delete push_subscriptions for the erased user during GDPR erasure
-- (Section 35). The endpoint URL and p256dh/auth keys are device-identifying
-- PII and must be removed.
DELETE FROM push_subscriptions WHERE user_id = @user_id;

-- name: GdprExportAttestationCommunityUpdates :one
-- §97: export attestation_community_updates authored by the user.
-- The table has no the id column; platform scoping is via the parent
-- pathway's the id.
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT u.id, u.pathway_id, u.step_index, u.fee_estimate,
 u.turnaround_days, u.note, u.status, u.reviewed_by, u.created_at
 FROM attestation_community_updates u
 JOIN attestation_pathways p ON p.id = u.pathway_id
 WHERE u.user_id = @user_id AND u.deleted_at IS NULL
) t;

-- name: SoftDeleteAttestationCommunityUpdatesByUser :exec
-- §97: soft-delete attestation_community_updates authored by the user
-- during GDPR erasure. Sets deleted_at = now() to preserve audit trail
-- while removing the user's PII association.
UPDATE attestation_community_updates
SET deleted_at = now()
WHERE user_id = @user_id AND deleted_at IS NULL;

-- name: SoftDeleteCredentialCommunityNotesByUser :exec
-- §94: soft-delete credential_community_notes authored by the user
-- during GDPR erasure. Sets deleted_at = now() to preserve the note
-- text while removing the user's PII association.
UPDATE credential_community_notes
SET deleted_at = now()
WHERE user_id = @user_id AND deleted_at IS NULL;

-- name: GdprExportUserCalendarEntries :one
-- §86: export calendar_entries submitted by the user (BUG-025).
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT e.id, e.title, e.description, e.category,
 e.destination_space_id, e.origin_space_id,
 e.occurs_at, e.ends_at, e.is_recurring, e.recurrence_pattern,
 e.source_url, e.status, e.created_at
 FROM calendar_entries e
 WHERE e.submitted_by = @user_id AND e.deleted_at IS NULL
) t;

-- name: GdprExportUserCalendarSaves :one
-- §86: export calendar_saves for the user (BUG-025).
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT s.entry_id, s.saved_at
 FROM calendar_saves s
 WHERE s.user_id = @user_id
) t;

-- name: GdprExportUserCalendarSubscriptions :one
-- §86: export calendar_subscriptions for the user (BUG-025).
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT sub.destination_space_id, sub.category, sub.created_at
 FROM calendar_subscriptions sub
 WHERE sub.user_id = @user_id
) t;

-- name: GdprReassignCalendarEntriesSubmittedBy :exec
-- §86: reassign calendar_entries.submitted_by to placeholder during GDPR
-- erasure (BUG-025). The entries are community content, not user PII.
UPDATE calendar_entries
SET submitted_by = @target_user_id
WHERE submitted_by = @user_id;

-- name: GdprReassignCalendarEntriesReviewedBy :exec
-- §86: reassign calendar_entries.reviewed_by to placeholder during GDPR
-- erasure (BUG-025).
UPDATE calendar_entries
SET reviewed_by = @target_user_id
WHERE reviewed_by = @user_id;

-- name: GdprDeleteCalendarSavesByUser :exec
-- §86: delete calendar_saves for the user during GDPR erasure (BUG-025).
DELETE FROM calendar_saves
WHERE user_id = @user_id;

-- name: GdprDeleteCalendarSubscriptionsByUser :exec
-- §86: delete calendar_subscriptions for the user during GDPR erasure (BUG-025).
DELETE FROM calendar_subscriptions
WHERE user_id = @user_id;

-- name: GdprExportReferralCodes :one
-- BUG-013: export referral_codes owned by the user so the GDPR data export
-- includes the user's referral code and click count.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md, §60
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT id, code, click_count, created_at
 FROM referral_codes
 WHERE user_id = @user_id
 ORDER BY created_at
) t;

-- name: GdprExportReferralsAsReferrer :one
-- BUG-013: export referrals where the user is the referrer so the GDPR data
-- export includes referral history (status, reward, dates). Referee PII is
-- NOT included — only the referral_code, status, and timestamps.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md, §60
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT id, referral_code, status, reward_amount, flagged_reason,
 qualified_at, credited_at, created_at
 FROM referrals
 WHERE referrer_id = @user_id
 ORDER BY created_at
) t;

-- name: GdprExportReferralsAsReferee :one
-- BUG-013: export the referral row where the user is the referee (the user
-- was referred). Includes the referral_code and status only — referrer PII
-- is excluded per §60 "Referral history exposes status/count, not referee PII"
-- (symmetric: referee export excludes referrer PII).
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md, §60
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT id, referral_code, status, reward_amount, qualified_at, credited_at, created_at
 FROM referrals
 WHERE referee_id = @user_id
 ORDER BY created_at
) t;

-- name: GdprExportAIUsageLog :one
-- BUG-012: export the user's AI usage log entries for GDPR data export.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md, §20
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT id, provider, model, task_type, prompt_tokens, completion_tokens, total_tokens, cost_usd, request_ref, created_at
 FROM ai_usage_log
 WHERE user_id = @user_id
 ORDER BY created_at DESC
) t;

-- name: GdprExportAIAgentSessions :one
-- BUG-012: export the user's AI agent sessions for GDPR data export.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md, §103
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT id, prompt, geo_space_id, presearch_level, status, tool_call_count, result_summary, error_message, total_tokens, estimated_cost, started_at, completed_at, created_at
 FROM ai_agent_sessions
 WHERE user_id = @user_id
 ORDER BY created_at DESC
) t;

-- name: GdprExportAIAgentToolCalls :one
-- BUG-012: export AI agent tool calls for sessions owned by the user.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md, §103
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT atc.id, atc.session_id, atc.tool_name, atc.arguments, atc.result, atc.status, atc.error_message, atc.latency_ms, atc.rbac_checked, atc.privacy_checked, atc.hitl_review_id, atc.created_at
 FROM ai_agent_tool_calls atc
 JOIN ai_agent_sessions s ON s.id = atc.session_id
 WHERE s.user_id = @user_id
 ORDER BY atc.created_at DESC
) t;

-- name: GdprExportAIHITLReviews :one
-- BUG-012: export HITL reviews where the user is the reviewer or the review
-- targets content authored by the user. This first cut uses reviewer_id.
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md, §20
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (
 SELECT id, content_id, content_type, ai_output, human_feedback, status, reason, reviewed_at, created_at, source, target_table
 FROM ai_hitl_reviews
 WHERE reviewer_id = @user_id
 ORDER BY created_at DESC
) t;

-- name: AnonymizeModerationAppeals :exec
UPDATE moderation_appeals
SET reason = '[erased]', resolution_note = NULL, updated_at = now()
WHERE user_id = $1;

-- §125 tips: retain the financial ledger rows but pseudonymize the sender
-- side — sender_user_id + external_sender_name cleared. The recipient side
-- keeps pointing at the (separately anonymized) users row, matching §19's
-- wallet/payment handling.
-- name: AnonymizeTipsForUser :exec
UPDATE tips SET sender_user_id = NULL, external_sender_name = NULL
WHERE sender_user_id = $1;

-- name: DeleteTipSettingsForUser :exec
DELETE FROM tip_settings WHERE owner_type = 'user' AND owner_id = $1;

-- name: GdprExportTipsSent :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM tips WHERE sender_user_id = @user_id ORDER BY created_at DESC) t;

-- name: GdprExportTipsReceived :one
SELECT coalesce(json_agg(t), '[]'::json) AS data
FROM (SELECT * FROM tips WHERE recipient_user_id = @user_id ORDER BY created_at DESC) t;

-- name: ReassignTipsRecipientToPlaceholder :exec
-- tips.recipient_user_id has a NO ACTION FK to users — received tips would
-- block the hard-delete. Reassign to the deleted-user placeholder so the
-- financial record survives pseudonymized (§19 retention).
UPDATE tips SET recipient_user_id = @target_user_id
WHERE recipient_user_id = @user_id;
