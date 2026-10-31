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

-- name: SoftDeleteUserSuspension :exec
-- Unsuspend path: removes the 'suspension' moderation-action row created by
-- POST /admin/users/{id}/suspend so ban-enforcement stops blocking the user.
UPDATE user_moderation_actions SET deleted_at = now()
WHERE user_id = $1 AND action = 'suspension' AND deleted_at IS NULL;

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
 AND deleted_at IS NULL
 AND (expires_at IS NULL OR expires_at > now())
);

-- name: GetUserBanExpiration :one
-- AUDIT-BE-DB-002: replaces raw SQL expiration lookup in ban enforcement middleware.
-- Returns the expires_at of the most recent active ban/suspension (may be NULL = permanent).
SELECT expires_at FROM user_moderation_actions
WHERE user_id = $1 AND action IN ('ban', 'suspension')
AND deleted_at IS NULL
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
-- §7: verification_type is set when supplied and preserved otherwise —
-- §111 package publishing gates on it (business|professional_body).
UPDATE organizations SET verified = true,
  verification_type = COALESCE(sqlc.narg(verification_type)::text, verification_type),
  updated_at = now()
WHERE id = $1;

-- name: UnverifyOrganization :exec
-- Platform-scoped and clears verification_type (BUG-002, BUG-016).
UPDATE organizations SET verified = false, verification_type = NULL, updated_at = now()
WHERE id = $1;

-- =====================
-- Appeals
-- =====================

-- name: ListModerationAppeals :many
SELECT ma.id, ma.user_id, ma.entity_type, ma.entity_id, ma.reason, ma.subject_label, ma.status, ma.created_at,
 ma.ticket_id, ma.resolution_note, t.ticket_number,
 u.username, up.display_name
FROM moderation_appeals ma
LEFT JOIN users u ON u.id = ma.user_id
LEFT JOIN user_profiles up ON up.user_id = ma.user_id
LEFT JOIN tickets t ON t.id = ma.ticket_id
WHERE (sqlc.narg(status)::text IS NULL OR ma.status = sqlc.narg(status)::text)
  AND (sqlc.narg(search)::text IS NULL OR (
 ma.entity_id::text ILIKE '%' || sqlc.narg(search) || '%'
 OR ma.reason ILIKE '%' || sqlc.narg(search) || '%'
 OR ma.entity_type ILIKE '%' || sqlc.narg(search) || '%'
 OR ma.subject_label ILIKE '%' || sqlc.narg(search) || '%'
 OR u.username ILIKE '%' || sqlc.narg(search) || '%'
 OR up.display_name ILIKE '%' || sqlc.narg(search) || '%'
 ))
ORDER BY ma.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountModerationAppeals :one
SELECT COUNT(*)
FROM moderation_appeals ma
LEFT JOIN users u ON u.id = ma.user_id
LEFT JOIN user_profiles up ON up.user_id = ma.user_id
WHERE (sqlc.narg(status)::text IS NULL OR ma.status = sqlc.narg(status)::text)
  AND (sqlc.narg(search)::text IS NULL OR (
 ma.entity_id::text ILIKE '%' || sqlc.narg(search) || '%'
 OR ma.reason ILIKE '%' || sqlc.narg(search) || '%'
 OR ma.entity_type ILIKE '%' || sqlc.narg(search) || '%'
 OR ma.subject_label ILIKE '%' || sqlc.narg(search) || '%'
 OR u.username ILIKE '%' || sqlc.narg(search) || '%'
 OR up.display_name ILIKE '%' || sqlc.narg(search) || '%'
 ));

-- name: GetModerationAppeal :one
SELECT ma.id, ma.user_id, ma.entity_type, ma.entity_id, ma.reason, ma.subject_label, ma.status, ma.created_at,
 ma.ticket_id, ma.resolution_note, ma.reviewed_by, ma.resolved_at, ma.updated_at, t.ticket_number,
 u.username, up.display_name
FROM moderation_appeals ma
LEFT JOIN tickets t ON t.id = ma.ticket_id
LEFT JOIN users u ON u.id = ma.user_id
LEFT JOIN user_profiles up ON up.user_id = ma.user_id
WHERE ma.id = $1;

-- name: UpdateAppealStatus :exec
UPDATE moderation_appeals SET status = $1, updated_at = now() WHERE id = $2;

-- =====================
-- Content Reports
-- =====================

-- name: AssignContentReport :execrows
UPDATE content_reports SET reviewed_by = $1, status = 'reviewing', updated_at = now() WHERE id = $2;

-- name: AssignContentReview :execrows
-- Unified-queue assign target — persona/studio content_reviews rows treat
-- reviewer_user_id as the reviewer (set again on approve/reject).
UPDATE content_reviews SET reviewer_user_id = $1, updated_at = now() WHERE id = $2;

-- name: AssignHitlReview :execrows
UPDATE ai_hitl_reviews SET reviewer_id = $1 WHERE id = $2;

-- name: AssignEcaCommunityUpdate :execrows
UPDATE eca_community_updates SET reviewed_by = $1 WHERE id = $2;

-- name: AssignReview :execrows
-- Migration 000988 assigned_to — reviewer_user_id is the author, not assignee.
UPDATE reviews SET assigned_to = $1 WHERE id = $2;

-- name: AssignCrsRuleset :execrows
UPDATE crs_rulesets SET assigned_to = $1 WHERE id = $2;

-- name: AssignPofRequirement :execrows
UPDATE pof_requirements SET assigned_to = $1 WHERE id = $2;

-- name: AssignEcaGuide :execrows
UPDATE eca_guides SET assigned_to = $1 WHERE id = $2;

-- name: AssignSopTemplate :execrows
UPDATE sop_templates SET assigned_to = $1 WHERE id = $2;

-- name: AssignSopSample :execrows
UPDATE sop_samples SET assigned_to = $1 WHERE id = $2;

-- name: ModerateContentReport :exec
-- resolved_at is stamped on terminal transitions — the §22 moderation
-- history keys resolution events off it (was previously left NULL here).
UPDATE content_reports
SET status = $1, reviewed_by = $2, resolution_note = $3,
 resolved_at = CASE WHEN $1 IN ('actioned', 'dismissed') THEN now() ELSE resolved_at END,
 updated_at = now()
WHERE id = $4;

-- =====================
-- Content Moderation
-- =====================

-- name: RemoveContentByAdmin :execrows
-- deleted_at IS NULL keeps the action single-fire: a repeat remove returns
-- 0 rows so the handler can 409 instead of writing a duplicate audit entry.
UPDATE contents SET status = 'archived', auto_hidden = true, deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: RestoreContentByAdmin :execrows
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
-- §22 adds author_id/space_id filters, reports_count (open reports per
-- §51), and ?sort/?sort_dir ordering over a whitelisted column set
-- (EntityPage column keys). Unknown/empty sort falls back to newest-first;
-- sort_dir is 'asc'/'desc' (validated in the handler).
-- name: AdminListContents :many
SELECT * FROM (
 SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body,
  c.status, c.visibility, c.view_count, c.comment_count, c.published_at,
  c.created_at, c.updated_at, c.deleted_at,
  EXISTS(SELECT 1 FROM content_featured cf WHERE cf.content_id = c.id
  AND (cf.featured_until IS NULL OR cf.featured_until > now())) AS is_featured,
  u.username AS author_username, up.display_name AS author_display_name,
  s.name AS space_name,
  (SELECT COUNT(*)::int FROM content_reports cr
   WHERE cr.entity_type = 'content' AND cr.entity_id = c.id
   AND cr.status IN ('pending', 'reviewing')) AS reports_count
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
  AND (sqlc.narg(author_id)::uuid IS NULL OR c.author_id = sqlc.narg(author_id))
  AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id))
) sub
ORDER BY
 CASE WHEN @sort::text = 'title' AND @sort_dir::text = 'desc' THEN sub.title END DESC,
 CASE WHEN @sort::text = 'title' AND @sort_dir::text <> 'desc' THEN sub.title END ASC,
 CASE WHEN @sort::text IN ('author', 'author_display_name') AND @sort_dir::text = 'desc' THEN sub.author_display_name END DESC NULLS LAST,
 CASE WHEN @sort::text IN ('author', 'author_display_name') AND @sort_dir::text <> 'desc' THEN sub.author_display_name END ASC NULLS LAST,
 CASE WHEN @sort::text = 'content_type' AND @sort_dir::text = 'desc' THEN sub.content_type END DESC,
 CASE WHEN @sort::text = 'content_type' AND @sort_dir::text <> 'desc' THEN sub.content_type END ASC,
 CASE WHEN @sort::text = 'space_name' AND @sort_dir::text = 'desc' THEN sub.space_name END DESC NULLS LAST,
 CASE WHEN @sort::text = 'space_name' AND @sort_dir::text <> 'desc' THEN sub.space_name END ASC NULLS LAST,
 CASE WHEN @sort::text = 'status' AND @sort_dir::text = 'desc' THEN sub.status END DESC,
 CASE WHEN @sort::text = 'status' AND @sort_dir::text <> 'desc' THEN sub.status END ASC,
 CASE WHEN @sort::text = 'view_count' AND @sort_dir::text = 'asc' THEN sub.view_count END ASC,
 CASE WHEN @sort::text = 'view_count' AND @sort_dir::text <> 'asc' THEN sub.view_count END DESC,
 CASE WHEN @sort::text = 'reports_count' AND @sort_dir::text = 'asc' THEN sub.reports_count END ASC,
 CASE WHEN @sort::text = 'reports_count' AND @sort_dir::text <> 'asc' THEN sub.reports_count END DESC,
 CASE WHEN @sort::text = 'created_at' AND @sort_dir::text = 'asc' THEN sub.created_at END ASC,
 CASE WHEN @sort::text NOT IN ('title', 'author', 'author_display_name', 'content_type', 'space_name', 'status', 'view_count', 'reports_count', 'created_at')
  OR (@sort::text = 'created_at' AND @sort_dir::text <> 'asc') THEN sub.created_at END DESC,
 sub.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminGetContent :one
-- Admin single-content detail — same joined shape as AdminListContents plus
-- the full contents row (c.* keeps the response a strict superset of the
-- legacy bare-row payload). Includes soft-deleted rows so the view modal
-- works on removed content; report count covers open reports (§22, §51).
SELECT c.*,
 EXISTS(SELECT 1 FROM content_featured cf WHERE cf.content_id = c.id
 AND (cf.featured_until IS NULL OR cf.featured_until > now())) AS is_featured,
 u.username AS author_username, up.display_name AS author_display_name,
 s.name AS space_name,
 (SELECT COUNT(*)::int FROM content_reports cr
  WHERE cr.entity_type = 'content' AND cr.entity_id = c.id
  AND cr.status IN ('pending', 'reviewing')) AS reports_count
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
WHERE c.id = @id;

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
 AND (@q::text = '' OR c.title ILIKE '%' || @q || '%' OR c.slug ILIKE '%' || @q || '%' OR c.body ILIKE '%' || @q || '%')
 AND (sqlc.narg(author_id)::uuid IS NULL OR c.author_id = sqlc.narg(author_id))
 AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id));

-- name: AdminListContentModerationHistory :many
-- §22 moderation-history projection for GET /admin/content/{id}: a unified,
-- newest-first timeline merging user reports and their resolutions (§51),
-- admin audit actions targeting the content, moderation appeals and their
-- resolutions, editorial/persona content reviews, AI HITL flags, and the
-- system auto-hide event. Each row is one event; actor_name resolves to the
-- actor's display name or username when the actor is a user.
SELECT * FROM (
 -- 1. Reports filed against the content (§51).
 SELECT 'report'::text AS kind,
        'report_filed'::text AS event,
        cr.status,
        (cr.reason || COALESCE(' — ' || cr.notes, '')) AS note,
        cr.reporter_user_id AS actor_id,
        COALESCE(up.display_name, u.username, '') AS actor_name,
        cr.created_at
 FROM content_reports cr
 LEFT JOIN users u ON u.id = cr.reporter_user_id
 LEFT JOIN user_profiles up ON up.user_id = cr.reporter_user_id
 WHERE cr.entity_type = 'content' AND cr.entity_id = @id

 UNION ALL

 -- 2. Report resolutions — actor is the reviewer. Keyed on resolved_at
 -- (not status) so a resolution survives a resolved report being flipped
 -- back to 'reviewing' by a later assignment; COALESCE covers legacy rows
 -- resolved before resolved_at was stamped.
 SELECT 'report'::text,
        ('report_' || cr.status)::text,
        cr.status,
        COALESCE(cr.resolution_note, ''),
        cr.reviewed_by,
        COALESCE(rup.display_name, ru.username, ''),
        COALESCE(cr.resolved_at, cr.updated_at)
 FROM content_reports cr
 LEFT JOIN users ru ON ru.id = cr.reviewed_by
 LEFT JOIN user_profiles rup ON rup.user_id = cr.reviewed_by
 WHERE cr.entity_type = 'content' AND cr.entity_id = @id
   AND cr.resolved_at IS NOT NULL

 UNION ALL

 -- 2b. A report a moderator has claimed/assigned — work in progress, still
 -- part of the timeline so reviewers see who picked it up and when.
 SELECT 'report'::text,
        'report_reviewing'::text,
        cr.status,
        ''::text,
        cr.reviewed_by,
        COALESCE(iup.display_name, iu.username, ''),
        cr.updated_at
 FROM content_reports cr
 LEFT JOIN users iu ON iu.id = cr.reviewed_by
 LEFT JOIN user_profiles iup ON iup.user_id = cr.reviewed_by
 WHERE cr.entity_type = 'content' AND cr.entity_id = @id
   AND cr.status = 'reviewing'

 UNION ALL

 -- 2c. Reopened reports — an appeal overturn flips a dismissed report back
 -- to pending (ReopenContentReportOnAppeal clears resolution fields), so
 -- updated_at > created_at is the only durable signal it happened.
 SELECT 'report'::text,
        'report_reopened'::text,
        cr.status,
        ''::text,
        cr.reviewed_by,
        COALESCE(iup.display_name, iu.username, ''),
        cr.updated_at
 FROM content_reports cr
 LEFT JOIN users iu ON iu.id = cr.reviewed_by
 LEFT JOIN user_profiles iup ON iup.user_id = cr.reviewed_by
 WHERE cr.entity_type = 'content' AND cr.entity_id = @id
   AND cr.status = 'pending' AND cr.updated_at > cr.created_at

 UNION ALL

 -- 3. Admin audit actions targeting the content (remove/restore/etc.).
 SELECT 'audit'::text,
        al.action_type,
        ''::text,
        COALESCE(al.metadata->>'reason', al.metadata->>'resolution_note', al.metadata->>'note', ''),
        al.actor_user_id,
        COALESCE(aup.display_name, au.username, ''),
        al.created_at
 FROM audit_logs al
 LEFT JOIN users au ON au.id = al.actor_user_id
 LEFT JOIN user_profiles aup ON aup.user_id = al.actor_user_id
 WHERE al.target_type = 'content' AND al.target_id = @id

 UNION ALL

 -- 4. Appeals filed against moderation decisions on the content.
 SELECT 'appeal'::text,
        'appeal_filed'::text,
        ma.status,
        ma.reason,
        ma.user_id,
        COALESCE(pup.display_name, pu.username, ''),
        ma.created_at
 FROM moderation_appeals ma
 LEFT JOIN users pu ON pu.id = ma.user_id
 LEFT JOIN user_profiles pup ON pup.user_id = ma.user_id
 WHERE ma.entity_type IN ('content', 'content_removal') AND ma.entity_id = @id

 UNION ALL

 -- 5. Appeal resolutions — actor is the reviewer. The §22 vocabulary
 -- (migration 000818) is upheld/overturn/dismissed, not approved/rejected.
 SELECT 'appeal'::text,
        ('appeal_' || ma.status)::text,
        ma.status,
        COALESCE(ma.resolution_note, ''),
        ma.reviewed_by,
        COALESCE(mrup.display_name, mru.username, ''),
        ma.resolved_at
 FROM moderation_appeals ma
 LEFT JOIN users mru ON mru.id = ma.reviewed_by
 LEFT JOIN user_profiles mrup ON mrup.user_id = ma.reviewed_by
 WHERE ma.entity_type IN ('content', 'content_removal') AND ma.entity_id = @id
   AND ma.status IN ('upheld', 'overturn', 'dismissed') AND ma.resolved_at IS NOT NULL

 UNION ALL

 -- 6. Editorial / persona review decisions on the content. persona_action
 -- rows are AI-persona review actions — the event name is distinct so the
 -- UI can label them differently from human editorial decisions.
 SELECT 'review'::text,
        ((CASE WHEN crv.source = 'persona_action' THEN 'persona_review_' ELSE 'review_' END) || crv.status)::text,
        crv.status,
        COALESCE(crv.feedback, ''),
        crv.reviewer_user_id,
        COALESCE(vup.display_name, vu.username, ''),
        crv.updated_at
 FROM content_reviews crv
 LEFT JOIN users vu ON vu.id = crv.reviewer_user_id
 LEFT JOIN user_profiles vup ON vup.user_id = crv.reviewer_user_id
 WHERE crv.content_id = @id AND crv.deleted_at IS NULL

 UNION ALL

 -- 7. AI HITL flags/reviews raised against the content.
 SELECT 'ai_review'::text,
        ('hitl_' || hr.status)::text,
        hr.status,
        COALESCE(hr.reason, ''),
        hr.reviewer_id,
        COALESCE(hup.display_name, hu.username, ''),
        COALESCE(hr.reviewed_at, hr.created_at)
 FROM ai_hitl_reviews hr
 LEFT JOIN users hu ON hu.id = hr.reviewer_id
 LEFT JOIN user_profiles hup ON hup.user_id = hr.reviewer_id
 WHERE hr.content_id = @id AND hr.target_table = 'contents'

 UNION ALL

 -- 8. System auto-hide event — the moderation:auto_hide worker flips
 -- auto_hidden when open reports cross moderation.auto_hide_report_count;
 -- contents.auto_hidden_at is the only record of when it fired.
 SELECT 'system'::text,
        'auto_hidden'::text,
        ''::text,
        ''::text,
        NULL::uuid,
        ''::text,
        c.auto_hidden_at
 FROM contents c
 WHERE c.id = @id AND c.auto_hidden AND c.auto_hidden_at IS NOT NULL

 UNION ALL

 -- 9. Edit revisions — the §22 "audit trail" half of the contract. Every
 -- save writes a content_revisions row stamped with edited_by.
 SELECT 'revision'::text,
        'edited'::text,
        ''::text,
        ''::text,
        rv.edited_by,
        COALESCE(eup.display_name, eu.username, ''),
        rv.created_at
 FROM content_revisions rv
 LEFT JOIN users eu ON eu.id = rv.edited_by
 LEFT JOIN user_profiles eup ON eup.user_id = rv.edited_by
 WHERE rv.content_id = @id
) hist
ORDER BY hist.created_at DESC, hist.kind, hist.event
LIMIT 200;

-- =====================
-- Broadcasts
-- =====================

-- name: ListAdminBroadcasts :many
-- BUG-013: ?status=draft|scheduled|sent filter + offset pagination (§35 L132).
-- Includes target/scheduled_at/status so the admin table can render and
-- gate the Send action. Rows are ordered newest-first by whichever
-- timestamp exists.
SELECT id, title, body, sent_by, sent_at, recipient_count, scheduled_at, target, target_id,
 CASE WHEN sent_at IS NOT NULL THEN 'sent'
      WHEN scheduled_at IS NOT NULL THEN 'scheduled'
      ELSE 'draft' END AS status
FROM admin_broadcasts
WHERE (@status::text = '' OR
       (@status::text = 'sent' AND sent_at IS NOT NULL) OR
       (@status::text = 'scheduled' AND sent_at IS NULL AND scheduled_at IS NOT NULL) OR
       (@status::text = 'draft' AND sent_at IS NULL AND scheduled_at IS NULL))
ORDER BY COALESCE(sent_at, scheduled_at) DESC NULLS LAST, id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAdminBroadcasts :one
SELECT COUNT(*)::int AS count
FROM admin_broadcasts
WHERE (@status::text = '' OR
       (@status::text = 'sent' AND sent_at IS NOT NULL) OR
       (@status::text = 'scheduled' AND sent_at IS NULL AND scheduled_at IS NOT NULL) OR
       (@status::text = 'draft' AND sent_at IS NULL AND scheduled_at IS NULL));

-- name: GetAdminBroadcastByID :one
-- BUG-010: POST /admin/broadcasts/{id}/send must load the stored draft —
-- subject/body/target come from the row, not the request body.
SELECT id, title, body, sent_by, sent_at, recipient_count, scheduled_at, target, target_id
FROM admin_broadcasts
WHERE id = $1;

-- name: DeleteAdminBroadcast :execrows
-- BUG-020: spec §35 is "delete draft broadcast" — a sent record is history,
-- not a draft; refuse to delete it (0 rows → 404). Also refuses to delete a
-- broadcast claimed mid-dispatch.
DELETE FROM admin_broadcasts WHERE id = $1 AND sent_at IS NULL AND claimed_at IS NULL;

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
-- BUG-001: the previous version mapped the SENDER into recipient_id,
-- unnested recipient UUIDs into notification_type, put 'system' into the
-- uuid actor_id, and inserted into a nonexistent updated_at column — every
-- broadcast INSERT failed at runtime. Correct shape: one row per recipient,
-- 'broadcast' type (seeded §35 catalog), system actor (NULL actor_id —
-- actor_id is uuid and 'system' is not a valid uuid).
INSERT INTO notifications (recipient_id, notification_type, actor_type, title, body, created_at)
SELECT unnest($1::uuid[]), 'broadcast', 'system', $2, $3, $4;

-- name: CreateAdminBroadcastRecord :exec
INSERT INTO admin_broadcasts (title, body, sent_by, recipient_count, sent_at) VALUES ($1, $2, $3, $4, now())
;

-- §35 scheduled broadcasts (000870)
-- name: CreateScheduledBroadcast :one
INSERT INTO admin_broadcasts (title, body, sent_by, scheduled_at, target, target_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListDueScheduledBroadcasts :many
SELECT * FROM admin_broadcasts
WHERE scheduled_at IS NOT NULL AND sent_at IS NULL AND scheduled_at <= now()
 AND (claimed_at IS NULL OR claimed_at < now() - interval '10 minutes')
ORDER BY scheduled_at
LIMIT 50;

-- name: ClaimBroadcast :execrows
-- Send-vs-sweep race: a claimant stamps claimed_at atomically — only one
-- claimant can win (0 rows → already claimed/in-flight or sent).
UPDATE admin_broadcasts
SET claimed_at = now()
WHERE id = $1 AND sent_at IS NULL
 AND (claimed_at IS NULL OR claimed_at < now() - interval '10 minutes');

-- name: UnclaimBroadcast :exec
-- Roll back a claim when dispatch fails so the row can be retried.
UPDATE admin_broadcasts
SET claimed_at = NULL
WHERE id = $1 AND sent_at IS NULL;

-- name: MarkBroadcastSent :exec
UPDATE admin_broadcasts
SET sent_at = now(), recipient_count = $2, claimed_at = NULL
WHERE id = $1;
