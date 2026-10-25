-- ---------------------------------------------------------------------------
-- Appeals (§22) — user-facing appeal submissions backed by moderation_appeals
-- + a linked §38 'appeal' ticket (staff-facing thread).
-- ---------------------------------------------------------------------------

-- name: CreateAppeal :one
INSERT INTO moderation_appeals (user_id, entity_type, entity_id, reason, subject_label)
VALUES (@user_id, @entity_type, @entity_id, @reason, sqlc.narg(subject_label))
RETURNING id, user_id, entity_type, entity_id, reason, subject_label, status, ticket_id,
  reviewed_by, resolution_note, created_at, updated_at, resolved_at;

-- name: GetAppealByID :one
-- t.ticket_number gives callers the public JC-<n> reference for the linked
-- ticket (§38) — the UUID ticket_id stays internal.
SELECT ma.id, ma.user_id, ma.entity_type, ma.entity_id, ma.reason, ma.subject_label, ma.status, ma.ticket_id,
  ma.reviewed_by, ma.resolution_note, ma.created_at, ma.updated_at, ma.resolved_at,
  t.ticket_number
FROM moderation_appeals ma
LEFT JOIN tickets t ON t.id = ma.ticket_id
WHERE ma.id = @id;

-- name: ListAppealsByUser :many
SELECT ma.id, ma.user_id, ma.entity_type, ma.entity_id, ma.reason, ma.subject_label, ma.status, ma.ticket_id,
  ma.reviewed_by, ma.resolution_note, ma.created_at, ma.updated_at, ma.resolved_at,
  t.ticket_number
FROM moderation_appeals ma
LEFT JOIN tickets t ON t.id = ma.ticket_id
WHERE ma.user_id = @user_id
ORDER BY ma.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPendingAppealSubjects :many
-- (entity_type, entity_id) pairs the caller already has a pending appeal
-- for — filters the eligible-items picker (GET /me/appeals/eligible).
SELECT entity_type, entity_id
FROM moderation_appeals
WHERE user_id = @user_id AND status = 'pending';

-- name: CountPendingAppealByUserSubject :one
SELECT COUNT(*) FROM moderation_appeals
WHERE user_id = @user_id
  AND entity_type = ANY(@entity_types::text[])
  AND entity_id = @entity_id
  AND status = 'pending';

-- name: SetAppealTicketID :exec
UPDATE moderation_appeals
SET ticket_id = @ticket_id, updated_at = now()
WHERE id = @id;

-- name: ResolveAppeal :execrows
UPDATE moderation_appeals
SET status = @status,
    reviewed_by = @reviewed_by,
    resolution_note = sqlc.narg(resolution_note),
    resolved_at = now(),
    updated_at = now()
WHERE id = @id
  AND status = 'pending';

-- name: GetTicketTypeIDBySlug :one
SELECT id FROM ticket_types WHERE slug = @slug;

-- name: GetModerationActionByID :one
-- Appeal eligibility check for ban/suspension/warning subjects. Expired
-- rows are excluded — there is no live decision to contest (the picker
-- query ListAppealableModerationActions applies the same expiry filter).
SELECT id, user_id, action, created_at
FROM user_moderation_actions
WHERE id = @id
  AND deleted_at IS NULL
  AND (expires_at IS NULL OR expires_at > now());

-- name: GetModerationActionForOverturn :one
-- Overturn-path lookup — no expiry filter: an action that expired between
-- appeal submission and resolution still needs its status/cache effects
-- (users.status restore + jc:ban invalidation) applied.
SELECT id, user_id, action
FROM user_moderation_actions
WHERE id = @id
  AND deleted_at IS NULL;

-- name: LiftUserActionByID :execrows
-- Overturn of ban/suspension/warning appeals — soft-deletes the action row
-- the same way admin lift flows do.
UPDATE user_moderation_actions
SET deleted_at = now()
WHERE id = @id
  AND deleted_at IS NULL;

-- name: LiftUserActionsByUserAndType :execrows
-- Overturn for 'user'-typed appeals (entity_id = the user id) — lifts every
-- live action of the given type (ban | suspension | warning).
UPDATE user_moderation_actions
SET deleted_at = now()
WHERE user_id = @user_id
  AND action = @action
  AND deleted_at IS NULL;

-- name: GetContentForAppeal :one
-- Appeal eligibility for content_removal: the appellant must be the author
-- and the content must be moderation-removed — auto_hidden marks both admin
-- removal (status=archived + deleted_at) and report auto-hide. Author
-- self-deletes set only deleted_at and are NOT appealable.
SELECT id, author_id, title, content_type
FROM contents
WHERE id = @id
  AND auto_hidden = true;

-- name: RestoreContentOnAppeal :execrows
-- Overturn of a content_removal appeal. Covers both removal shapes:
-- admin removal (status='archived' + deleted_at + auto_hidden) and report
-- auto-hide (auto_hidden only, status/deleted_at untouched). Admin-removed
-- rows come back as 'draft' for re-review; auto-hidden rows keep their
-- original status.
UPDATE contents
SET status = CASE WHEN status = 'archived' THEN 'draft' ELSE status END,
    auto_hidden = false,
    auto_hidden_at = NULL,
    deleted_at = NULL,
    updated_at = now()
WHERE id = @id
  AND auto_hidden = true;

-- name: GetOwnershipClaimForAppeal :one
SELECT oc.id, oc.claimant_user_id, oc.status, o.name AS organization_name
FROM ownership_claims oc
LEFT JOIN organizations o ON o.id = oc.organization_id
WHERE oc.id = @id;

-- name: GetVideoChannelClaimForAppeal :one
SELECT vc.id, vc.claimant_user_id, vc.status, vs.title AS video_title
FROM video_channel_claims vc
LEFT JOIN video_sources vs ON vs.id = vc.video_source_id
WHERE vc.id = @id;

-- name: GetServiceClaimMetaForAppeal :one
-- entity_id for service_claim appeals is the contents.id of the listing.
SELECT m.content_id, m.claimed_by_org_id, m.claim_status, c.title AS listing_title
FROM service_listing_meta m
LEFT JOIN contents c ON c.id = m.content_id
WHERE m.content_id = @content_id;

-- ---------------------------------------------------------------------------
-- Additional subjects (§22 registry): operation_ban, media, comment,
-- wiki_revision, content_report, scam_report, academy_instructor.
-- ---------------------------------------------------------------------------

-- name: GetOperationBanForAppeal :one
SELECT id, user_id, operation::text
FROM user_operation_bans
WHERE id = @id
  AND deleted_at IS NULL
  AND (expires_at IS NULL OR expires_at > now());

-- name: LiftOperationBanByID :execrows
UPDATE user_operation_bans
SET deleted_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: GetMediaForAppeal :one
SELECT id, uploader_user_id, confirmed, rejected_at, quarantine_reason, original_filename
FROM media
WHERE id = @id AND deleted_at IS NULL;

-- name: ApproveMediaOnAppeal :execrows
-- Overturn of a media rejection/quarantine: confirm the asset and clear the
-- rejection/quarantine markers so it leaves moderation.
UPDATE media
SET confirmed = true,
    rejected_at = NULL,
    quarantine_reason = NULL,
    quarantined_at = NULL,
    reviewed_by = @reviewed_by,
    reviewed_at = now()
WHERE id = @id
  AND confirmed = false
  AND deleted_at IS NULL;

-- name: GetCommentForAppeal :one
-- comments.body is TipTap JSONB — extract the text nodes for the excerpt.
SELECT id, author_user_id, status,
 left(COALESCE((SELECT string_agg(n #>> '{}', ' ') FROM jsonb_path_query(body, '$.**.text') AS n), ''), 80) AS excerpt
FROM comments
WHERE id = @id AND deleted_at IS NULL;

-- name: RestoreCommentOnAppeal :one
-- Returns the restored row so the overturn can re-increment the
-- denormalized comment_count/reply_count that RemoveComment decremented.
UPDATE comments
SET status = 'published', removed_by = NULL, removed_reason = NULL, updated_at = now()
WHERE id = @id AND status = 'removed' AND deleted_at IS NULL
RETURNING id, content_id, parent_id;

-- name: GetWikiRevisionForAppeal :one
SELECT wr.id, wr.submitted_by, wr.status, p.title AS page_title
FROM wiki_revisions wr
LEFT JOIN wiki_pages p ON p.id = wr.wiki_page_id
WHERE wr.id = @id AND wr.deleted_at IS NULL;

-- name: ReopenWikiRevisionOnAppeal :execrows
-- Overturn returns the revision to pending for a fresh review — ops
-- application is not run on appeal (needs human re-review).
UPDATE wiki_revisions
SET status = 'pending', reviewed_by = NULL, reviewed_at = NULL
WHERE id = @id AND status = 'rejected' AND deleted_at IS NULL;

-- name: GetContentReportForAppeal :one
SELECT id, reporter_user_id, status, entity_type
FROM content_reports
WHERE id = @id;

-- name: ReopenContentReportOnAppeal :execrows
UPDATE content_reports
SET status = 'pending', reviewed_by = NULL, resolution_note = NULL, resolved_at = NULL
WHERE id = @id AND status = 'dismissed';

-- name: GetScamReportForAppeal :one
SELECT id, reporter_id, status, scam_type
FROM scam_reports
WHERE id = @id AND deleted_at IS NULL;

-- name: ReopenScamReportOnAppeal :execrows
UPDATE scam_reports
SET status = 'under_review', reviewed_by = NULL, reviewed_at = NULL, updated_at = now()
WHERE id = @id AND status IN ('debunked', 'duplicate');

-- name: GetAcademyInstructorForAppeal :one
SELECT id, user_id, status
FROM academy_instructor_profiles
WHERE id = @id;

-- name: ReopenAcademyInstructorOnAppeal :execrows
UPDATE academy_instructor_profiles
SET status = 'pending', reviewed_by = NULL, reviewed_at = NULL, updated_at = now()
WHERE id = @id AND status IN ('rejected', 'suspended');

-- ---------------------------------------------------------------------------
-- Eligibility discovery (GET /me/appeals/eligible): one query per subject
-- lists the caller's currently appealable entities.
-- ---------------------------------------------------------------------------

-- name: ListAppealableModerationActions :many
-- Active ban/suspension/warning rows for the caller — entity_id is the
-- user_moderation_actions row id.
SELECT id, action, reason, expires_at, created_at
FROM user_moderation_actions
WHERE user_id = @user_id
  AND deleted_at IS NULL
  AND action IN ('ban', 'suspension', 'warning')
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC;

-- name: ListAppealableOperationBans :many
SELECT id, operation::text, reason, expires_at
FROM user_operation_bans
WHERE user_id = @user_id
  AND deleted_at IS NULL
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC;

-- name: ListAppealableRemovedContents :many
-- Moderation-removed/auto-hidden only (auto_hidden=true) — self-deleted
-- posts (deleted_at alone) are not a moderation decision to appeal.
SELECT id, title, content_type
FROM contents
WHERE author_id = @user_id AND auto_hidden = true
ORDER BY COALESCE(deleted_at, auto_hidden_at) DESC;

-- name: ListAppealableRemovedComments :many
SELECT id, content_id,
 left(COALESCE((SELECT string_agg(n #>> '{}', ' ') FROM jsonb_path_query(body, '$.**.text') AS n), ''), 120) AS excerpt
FROM comments
WHERE author_user_id = @user_id AND status = 'removed' AND deleted_at IS NULL
ORDER BY updated_at DESC;

-- name: ListAppealableMedia :many
SELECT id, original_filename
FROM media
WHERE uploader_user_id = @user_id
  AND deleted_at IS NULL
  AND confirmed = false
  AND (rejected_at IS NOT NULL OR (quarantine_reason IS NOT NULL AND quarantine_reason != ''))
ORDER BY created_at DESC;

-- name: ListAppealableWikiRevisions :many
SELECT wr.id, wr.wiki_page_id, p.title AS page_title
FROM wiki_revisions wr
LEFT JOIN wiki_pages p ON p.id = wr.wiki_page_id
WHERE wr.submitted_by = @user_id AND wr.status = 'rejected' AND wr.deleted_at IS NULL
ORDER BY wr.created_at DESC;

-- name: ListAppealableContentReports :many
SELECT id, entity_type, entity_id
FROM content_reports
WHERE reporter_user_id = @user_id AND status = 'dismissed'
ORDER BY created_at DESC;

-- name: ListAppealableScamReports :many
SELECT id, scam_type::text
FROM scam_reports
WHERE reporter_id = @user_id
  AND status IN ('debunked', 'duplicate')
  AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: ListAppealableAcademyInstructor :many
SELECT id
FROM academy_instructor_profiles
WHERE user_id = @user_id AND status IN ('rejected', 'suspended');

-- name: ListAppealableServiceClaims :many
-- Rejected service claims where the caller belongs to the claiming org.
SELECT m.content_id, c.title AS listing_title
FROM service_listing_meta m
JOIN organization_members om
  ON om.organization_id = m.claimed_by_org_id
LEFT JOIN contents c ON c.id = m.content_id
WHERE om.user_id = @user_id
  AND m.claim_status = 'rejected';

-- name: ListAppealableOrgClaims :many
SELECT oc.id, oc.organization_id, o.name AS organization_name
FROM ownership_claims oc
LEFT JOIN organizations o ON o.id = oc.organization_id
WHERE oc.claimant_user_id = @user_id AND oc.status = 'rejected';

-- name: GetVerificationRequestForAppeal :one
-- §07 verification requests: only the requester can appeal, only while rejected.
SELECT vr.id, vr.requester_user_id, vr.status, vr.entity_type, o.name AS organization_name
FROM verification_requests vr
LEFT JOIN organizations o ON o.id = vr.organization_id
WHERE vr.id = @id;

-- name: ListAppealableVerificationRequests :many
SELECT vr.id, vr.entity_type, o.name AS organization_name
FROM verification_requests vr
LEFT JOIN organizations o ON o.id = vr.organization_id
WHERE vr.requester_user_id = @user_id AND vr.status = 'rejected';

-- name: ListAppealableVideoClaims :many
SELECT vc.id, vc.video_source_id, vs.title AS video_title
FROM video_channel_claims vc
LEFT JOIN video_sources vs ON vs.id = vc.video_source_id
WHERE vc.claimant_user_id = @user_id AND vc.status = 'rejected';

-- ---------------------------------------------------------------------------
-- Extended subjects (§22 registry round 2): review, mutual_aid_post,
-- video_source, roadmap_item, organization, space, academy_track,
-- affiliate_member, referral, withdrawal, ad_campaign,
-- content_review, community_org, community_org_claim.
-- ---------------------------------------------------------------------------

-- name: GetReviewForAppeal :one
SELECT id, reviewer_user_id, status, entity_type, entity_id, title
FROM reviews
WHERE id = @id AND status = 'removed';

-- name: RestoreReviewOnAppeal :one
-- Overturn restores the review to published and clears the removal stamp.
-- entity_type/entity_id are returned so the service can recompute the
-- denormalized aggregate (best-effort, same as the flag/remove paths).
UPDATE reviews
SET status = 'published', removed_by = NULL, updated_at = now()
WHERE id = @id AND status = 'removed'
RETURNING id, entity_type, entity_id;

-- name: ListAppealableReviews :many
SELECT id, title, entity_type, entity_id
FROM reviews
WHERE reviewer_user_id = @user_id AND status = 'removed'
ORDER BY updated_at DESC;

-- name: GetMutualAidPostForAppeal :one
SELECT id, user_id, status, title
FROM mutual_aid_posts
WHERE id = @id AND deleted_at IS NULL;

-- name: RestoreMutualAidPostOnAppeal :execrows
UPDATE mutual_aid_posts
SET status = 'active', updated_at = now()
WHERE id = @id AND status = 'removed' AND deleted_at IS NULL;

-- name: ListAppealableMutualAidPosts :many
SELECT id, title
FROM mutual_aid_posts
WHERE user_id = @user_id AND status = 'removed' AND deleted_at IS NULL
ORDER BY updated_at DESC;

-- name: GetVideoSourceForAppeal :one
SELECT id, author_user_id, status, title
FROM video_sources
WHERE id = @id;

-- name: RestoreVideoSourceOnAppeal :execrows
-- Takedown approval flips the source to 'removed'; overturn returns it to
-- 'active'.
UPDATE video_sources
SET status = 'active', updated_at = now()
WHERE id = @id AND status = 'removed';

-- name: ListAppealableVideoSources :many
SELECT id, title, status
FROM video_sources
WHERE author_user_id = @user_id AND status = 'removed'
ORDER BY updated_at DESC;

-- name: GetRoadmapItemForAppeal :one
SELECT id, submitted_by, status, title
FROM roadmap_items
WHERE id = @id AND deleted_at IS NULL;

-- name: ReopenRoadmapItemOnAppeal :execrows
-- Declined suggestion re-enters the review queue rather than flipping to a
-- committed state.
UPDATE roadmap_items
SET status = 'under_review', updated_at = now()
WHERE id = @id AND status = 'declined' AND deleted_at IS NULL;

-- name: ListAppealableRoadmapItems :many
SELECT id, title
FROM roadmap_items
WHERE submitted_by = @user_id AND status = 'declined' AND deleted_at IS NULL
ORDER BY updated_at DESC;

-- name: GetOrganizationForAppeal :one
SELECT id, owner_user_id, status, name, deleted_at
FROM organizations
WHERE id = @id;

-- name: RestoreOrganizationOnAppeal :execrows
-- Covers suspension and soft-delete alike — both are admin decisions the
-- owner can contest.
UPDATE organizations
SET status = 'active', deleted_at = NULL, updated_at = now()
WHERE id = @id AND (status = 'suspended' OR deleted_at IS NOT NULL);

-- name: ListAppealableOrganizations :many
SELECT id, name, status
FROM organizations
WHERE owner_user_id = @user_id
  AND (status = 'suspended' OR deleted_at IS NOT NULL)
ORDER BY updated_at DESC;

-- name: GetSpaceForAppeal :one
SELECT id, creator_user_id, status, name, deleted_at
FROM spaces
WHERE id = @id;

-- name: IsSpaceOwnerMember :one
-- Space owner-role member (space_members.role='owner') may appeal a space
-- suspension alongside the creator.
SELECT EXISTS(
  SELECT 1 FROM space_members
  WHERE space_id = @space_id AND user_id = @user_id
    AND role = 'owner' AND status = 'approved'
) AS is_owner;

-- name: RestoreSpaceOnAppeal :execrows
UPDATE spaces
SET status = 'active', deleted_at = NULL, updated_at = now()
WHERE id = @id AND (status = 'suspended' OR deleted_at IS NOT NULL);

-- name: ListAppealableSpaces :many
SELECT s.id, s.name, s.status
FROM spaces s
LEFT JOIN space_members sm
  ON sm.space_id = s.id AND sm.user_id = @user_id
 AND sm.role = 'owner' AND sm.status = 'approved'
WHERE (s.creator_user_id = @user_id OR sm.user_id IS NOT NULL)
  AND (s.status = 'suspended' OR s.deleted_at IS NOT NULL)
ORDER BY s.updated_at DESC;

-- name: GetAcademyTrackForAppeal :one
-- Appellant is the instructor's user (academy_tracks.instructor_id →
-- academy_instructor_profiles.user_id).
SELECT t.id, ip.user_id AS instructor_user_id, t.academy_submission_status, t.title
FROM academy_tracks t
JOIN academy_instructor_profiles ip ON ip.id = t.instructor_id
WHERE t.id = @id;

-- name: ReopenAcademyTrackOnAppeal :execrows
UPDATE academy_tracks
SET academy_submission_status = 'pending',
    academy_reviewed_at = NULL,
    academy_rejection_reason = NULL,
    updated_at = now()
WHERE id = @id AND academy_submission_status = 'rejected';

-- name: ListAppealableAcademyTracks :many
SELECT t.id, t.title
FROM academy_tracks t
JOIN academy_instructor_profiles ip ON ip.id = t.instructor_id
WHERE ip.user_id = @user_id AND t.academy_submission_status = 'rejected'
ORDER BY t.academy_submitted_at DESC;

-- name: GetAffiliateMemberForAppeal :one
-- affiliate_program_members is keyed by user_id — entity_id IS the user id.
SELECT user_id, status
FROM affiliate_program_members
WHERE user_id = @id AND status IN ('suspended', 'banned');

-- name: RestoreAffiliateMemberOnAppeal :execrows
UPDATE affiliate_program_members
SET status = 'active'
WHERE user_id = @id AND status IN ('suspended', 'banned');

-- name: GetReferralForAppeal :one
SELECT r.id, r.referrer_id, r.status,
       COALESCE(rp.display_name, ru.username, r.referral_code) AS referee_name
FROM referrals r
LEFT JOIN users ru ON ru.id = r.referee_id
LEFT JOIN user_profiles rp ON rp.user_id = ru.id
WHERE r.id = @id;

-- name: ReopenReferralOnAppeal :execrows
UPDATE referrals
SET status = 'pending', flagged_reason = NULL
WHERE id = @id AND status = 'rejected';

-- name: ListAppealableReferrals :many
SELECT r.id, r.status,
       COALESCE(rp.display_name, ru.username, r.referral_code) AS referee_name
FROM referrals r
LEFT JOIN users ru ON ru.id = r.referee_id
LEFT JOIN user_profiles rp ON rp.user_id = ru.id
WHERE r.referrer_id = @user_id AND r.status = 'rejected'
ORDER BY r.created_at DESC;

-- name: GetWithdrawalForAppeal :one
SELECT id, user_id, status, credit_amount
FROM wallet_withdrawals
WHERE id = @id;

-- name: ListAppealableWithdrawals :many
SELECT id, credit_amount
FROM wallet_withdrawals
WHERE user_id = @user_id AND status = 'rejected'
ORDER BY created_at DESC;
-- (The withdrawal overturn itself lives in internal/wallet — ReopenOnAppeal
-- re-reserves the refunded credits inside the wallet advisory lock and is
-- registered via appeals.RegisterOverturn from server.go.)

-- name: GetAdCampaignForAppeal :one
SELECT id, created_by, organization_id, status, name, budget_credits
FROM ad_campaigns
WHERE id = @id;

-- name: ReopenAdCampaignOnAppeal :execrows
-- Rejected self-serve ads return to pending_review. budget_credits is
-- re-escrowed by the caller (adsselfserve.ReopenCampaignOnAppeal re-charges
-- the refunded amount inside the wallet advisory lock before reopening —
-- a reopened campaign must never serve against an already-refunded budget).
UPDATE ad_campaigns
SET status = 'pending_review', budget_credits = @budget_credits, updated_at = now()
WHERE id = @id AND status = 'rejected';

-- name: GetAdCampaignRefundAmount :one
-- The reject-time refund writes a credits ledger row keyed
-- 'deduct:{owner}:ad_campaign_refund:{campaign}'. Recovers the refunded
-- amount so an appeal overturn can re-escrow it.
SELECT amount
FROM wallet_ledger
WHERE idempotency_key = @idempotency_key;

-- name: ListAppealableAdCampaigns :many
SELECT c.id, c.name
FROM ad_campaigns c
LEFT JOIN organization_members om
  ON om.organization_id = c.organization_id AND om.user_id = @user_id
WHERE (c.created_by = @user_id OR om.user_id IS NOT NULL)
  AND c.status = 'rejected'
ORDER BY c.updated_at DESC;

-- name: GetContentReviewForAppeal :one
-- The appellant is the content's author, not the reviewer.
SELECT cr.id, c.author_id, cr.status, c.title
FROM content_reviews cr
JOIN contents c ON c.id = cr.content_id
WHERE cr.id = @id AND cr.deleted_at IS NULL;

-- name: ReopenContentReviewOnAppeal :execrows
UPDATE content_reviews
SET status = 'pending', reviewer_user_id = NULL, feedback = NULL, updated_at = now()
WHERE id = @id AND status = 'rejected' AND deleted_at IS NULL;

-- name: ListAppealableContentReviews :many
SELECT cr.id, c.title
FROM content_reviews cr
JOIN contents c ON c.id = cr.content_id
WHERE c.author_id = @user_id AND cr.status = 'rejected' AND cr.deleted_at IS NULL
ORDER BY cr.updated_at DESC;

-- name: GetCommunityOrgForAppeal :one
SELECT id, submitted_by, status, name, deleted_at, claim_status,
       claimed_by_user_id, claim_rejected_at
FROM community_organisations
WHERE id = @id;

-- name: RestoreCommunityOrgOnAppeal :execrows
-- Deleted or archived community submissions return to 'pending' for a
-- fresh moderation pass rather than straight to 'published'.
UPDATE community_organisations
SET status = 'pending', deleted_at = NULL, is_active = false, updated_at = now()
WHERE id = @id
  AND (deleted_at IS NOT NULL OR status = 'archived');

-- name: ListAppealableCommunityOrgs :many
SELECT id, name, status
FROM community_organisations
WHERE submitted_by = @user_id
  AND (deleted_at IS NOT NULL OR status = 'archived')
ORDER BY updated_at DESC;

-- name: GetCommunityOrgClaimForAppeal :one
-- claim_rejected_at IS NOT NULL marks "the last claim was rejected"
-- (claim_status itself resets to 'unclaimed' so another claimant can try).
SELECT id, claimed_by_user_id, claim_rejected_at, name
FROM community_organisations
WHERE id = @id AND claim_rejected_at IS NOT NULL AND deleted_at IS NULL;

-- name: ApproveCommunityOrgClaimOnAppeal :execrows
UPDATE community_organisations
SET claim_status = 'claimed', claim_rejected_at = NULL, updated_at = now()
WHERE id = @id AND claim_rejected_at IS NOT NULL;

-- name: ListAppealableCommunityOrgClaims :many
SELECT id, name
FROM community_organisations
WHERE claimed_by_user_id = @user_id AND claim_rejected_at IS NOT NULL
  AND deleted_at IS NULL
ORDER BY claim_rejected_at DESC;
