-- name: CreateReport :one
INSERT INTO content_reports (reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls) VALUES (@reporter_user_id, @entity_type, @entity_id, @reason, @notes, @evidence_urls)

ON CONFLICT (reporter_user_id, entity_type, entity_id) WHERE reporter_user_id IS NOT NULL DO NOTHING
RETURNING id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
 status, reviewed_by, resolution_note, resolved_at, created_at;

-- name: GetReport :one
SELECT id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
 status, reviewed_by, resolution_note, resolved_at, created_at
FROM content_reports
WHERE id = @id;

-- name: ListReportsByReporter :many
SELECT id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
 status, reviewed_by, resolution_note, resolved_at, created_at
FROM content_reports
WHERE reporter_user_id = @reporter_user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListReports :many
SELECT cr.id, cr.reporter_user_id, cr.entity_type, cr.entity_id, cr.reason, cr.notes, cr.evidence_urls,
 cr.status, cr.reviewed_by, cr.resolution_note, cr.resolved_at, cr.created_at,
 u.username AS reporter_username, up.display_name AS reporter_display_name
FROM content_reports cr
LEFT JOIN users u ON u.id = cr.reporter_user_id
LEFT JOIN user_profiles up ON up.user_id = cr.reporter_user_id
WHERE (sqlc.narg(status)::text IS NULL OR cr.status = sqlc.narg(status))
 AND (sqlc.narg(entity_type)::text IS NULL OR cr.entity_type = sqlc.narg(entity_type))
 AND (sqlc.narg(q)::text IS NULL OR (
 cr.entity_id::text ILIKE '%' || sqlc.narg(q) || '%'
 OR cr.entity_type ILIKE '%' || sqlc.narg(q) || '%'
 OR cr.reporter_user_id::text ILIKE '%' || sqlc.narg(q) || '%'
 ))
ORDER BY cr.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateReportStatus :one
UPDATE content_reports
SET status = @status, reviewed_by = @reviewed_by, resolution_note = @resolution_note,
 resolved_at = CASE WHEN @status IN ('actioned', 'dismissed') THEN now() ELSE resolved_at END
WHERE id = @id
RETURNING id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
 status, reviewed_by, resolution_note, resolved_at, created_at;

-- name: CountPendingReportsForEntity :one
SELECT COUNT(*)::int FROM content_reports
WHERE entity_type = @entity_type
 AND entity_id = @entity_id
 AND status = 'pending';

-- name: DeleteReport :exec
DELETE FROM content_reports
WHERE id = @id;

-- name: AutoHideContent :one
UPDATE contents
SET auto_hidden = true, auto_hidden_at = now()
WHERE id = @id AND auto_hidden = false
RETURNING id, author_id, space_id, content_type, title, slug, body, body_html,
 field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
 canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
 status, visibility, is_locked, is_pinned, read_time_minutes,
 view_count, comment_count, share_count, reshare_count, publish_at, published_at,
 created_at, updated_at;

-- ─── Moderator workbench (spec .jeos/specs/2026-09-23-studio-moderation-tab.md) ───
-- Scoped moderation queue for /studio/moderation: the caller sees a report when
-- they hold a platform node (admin.moderation / reports.review — resolved in Go,
-- passed as @platform), the report is assigned to them (reviewed_by), or the
-- reported entity resolves into a space they moderate/own (space_moderators or
-- space_members.role='owner') or carries a tag they moderate (tag_moderators via
-- content_tags — forum tags are universal since migration 000719, so forum-tag
-- moderators arrive through the same tag_moderators path).

-- name: ListModeratedSpaces :many
-- Spaces the caller moderates: assigned via space_moderators OR owner via
-- space_members.role='owner' (owner implies spaces.moderate, Section 03).
-- Names/slugs are returned so the frontend filter dropdown needs no N+1 lookups.
SELECT s.id, s.name, s.slug FROM (
    SELECT space_id FROM space_moderators WHERE space_moderators.user_id = @user_id
    UNION
    SELECT space_id FROM space_members WHERE space_members.user_id = @user_id AND space_members.role = 'owner'
) m JOIN spaces s ON s.id = m.space_id
ORDER BY s.name;

-- name: ListModeratedTags :many
-- Tags the caller moderates (tag_moderators — runtime scope per the studio
-- moderation spec; previously informational only). Universal tags include
-- forum tags since migration 000719.
SELECT t.id, t.name, t.slug
FROM tag_moderators tm
JOIN tags t ON t.id = tm.tag_id
WHERE tm.user_id = @user_id
ORDER BY t.name;

-- name: CountAssignedReports :one
-- Reports explicitly assigned to the caller (reviewed_by = caller). Counts
-- all statuses — assignment is itself a scope grant, so a moderator whose
-- assigned reports are all resolved keeps workbench access.
SELECT COUNT(*)::int FROM content_reports
WHERE reviewed_by = @user_id;

-- name: ResolveReportIfOpen :one
-- Atomically transition a report to a terminal status, but only while it is
-- still open — prevents two moderators from double-actioning the same report
-- (e.g. two concurrent bans). ErrNoRows means already resolved.
UPDATE content_reports
SET status = @status, reviewed_by = @reviewed_by, resolution_note = @resolution_note,
 resolved_at = CASE WHEN @status IN ('actioned', 'dismissed') THEN now() ELSE resolved_at END
WHERE id = @id AND status IN ('pending', 'reviewing')
RETURNING id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
 status, reviewed_by, resolution_note, resolved_at, created_at;

-- name: ListModerationQueue :many
WITH scoped AS (
    SELECT cr.id, cr.reporter_user_id, cr.entity_type, cr.entity_id, cr.reason, cr.notes,
           cr.evidence_urls, cr.status, cr.reviewed_by, cr.resolution_note, cr.resolved_at, cr.created_at,
           ru.username AS reporter_username,
           rup.display_name AS reporter_display_name,
           COALESCE(c.title, pc.title, s.name, tu.username, isrc.name, '') AS entity_title,
           COALESCE(LEFT(c.body, 240), LEFT((SELECT string_agg(DISTINCT n #>> '{}', ' ')
                    FROM jsonb_path_query(cm.body, '$.** ? (@.text.type() == "string").text') AS n), 240),
                    s.description, LEFT(dm.body, 240), isrc.source_url, '') AS entity_excerpt,
           COALESCE(c.slug, pc.slug, s.slug, isrc.slug, '') AS entity_slug,
           COALESCE((SELECT thr.slug FROM forum_threads thr
             WHERE thr.content_id = COALESCE(c.id, pc.id) LIMIT 1), '')::text AS thread_slug,
           COALESCE(c.content_type, pc.content_type, '')::text AS entity_content_type,
           au.username AS entity_author_username,
           COALESCE(c.id, pc.id) AS resolved_content_id,
           CASE WHEN cr.entity_type = 'space' THEN cr.entity_id
                ELSE COALESCE(c.space_id, pc.space_id) END::uuid AS resolved_space_id,
           sp.name AS entity_space_name,
           sp.slug AS entity_space_slug,
           COALESCE(cr.reviewed_by = @caller::uuid, false)::bool AS scope_assigned,
           COALESCE(
               (cr.entity_type = 'space'   AND cr.entity_id = ANY(@space_ids::uuid[])) OR
               (cr.entity_type = 'content' AND c.space_id   = ANY(@space_ids::uuid[])) OR
               (cr.entity_type = 'comment' AND pc.space_id  = ANY(@space_ids::uuid[])),
               false
           )::bool AS scope_space,
           EXISTS (SELECT 1 FROM content_tags ct
                    WHERE ct.content_id = COALESCE(c.id, pc.id)
                      AND ct.tag_id = ANY(@tag_ids::uuid[]))::bool AS scope_tag
    FROM content_reports cr
    LEFT JOIN users ru          ON ru.id = cr.reporter_user_id
    LEFT JOIN user_profiles rup ON rup.user_id = cr.reporter_user_id
    LEFT JOIN contents c        ON cr.entity_type = 'content' AND c.id = cr.entity_id
    LEFT JOIN comments cm       ON cr.entity_type = 'comment' AND cm.id = cr.entity_id
    LEFT JOIN contents pc       ON pc.id = cm.content_id
    LEFT JOIN spaces s          ON cr.entity_type = 'space' AND s.id = cr.entity_id
    LEFT JOIN spaces sp         ON sp.id = COALESCE(c.space_id, pc.space_id)
    LEFT JOIN users tu          ON cr.entity_type = 'user' AND tu.id = cr.entity_id
    LEFT JOIN dm_messages dm    ON cr.entity_type = 'message' AND dm.id = cr.entity_id
    LEFT JOIN intel_sources isrc ON cr.entity_type = 'source' AND isrc.id = cr.entity_id
    LEFT JOIN users au          ON au.id = COALESCE(c.author_id, cm.author_user_id, dm.sender_id)
)
SELECT id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
       status, reviewed_by, resolution_note, resolved_at, created_at,
       reporter_username, reporter_display_name,
       entity_title, entity_excerpt, entity_slug, thread_slug, entity_content_type, entity_author_username,
       resolved_space_id, entity_space_name, entity_space_slug,
       scope_assigned, scope_space, scope_tag
FROM scoped
WHERE (@platform::bool OR scope_assigned OR scope_space OR scope_tag)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type))
  AND (sqlc.narg(content_type)::text IS NULL OR entity_content_type = sqlc.narg(content_type))
  AND (sqlc.narg(filter_space_id)::uuid IS NULL OR resolved_space_id = sqlc.narg(filter_space_id)::uuid)
  AND (sqlc.narg(filter_tag_id)::uuid IS NULL OR
       EXISTS (SELECT 1 FROM content_tags ct
                WHERE ct.content_id = resolved_content_id AND ct.tag_id = sqlc.narg(filter_tag_id)::uuid))
  AND (@assigned_only::bool = false OR scope_assigned)
  AND (@open_only::bool = false OR status IN ('pending', 'reviewing'))
  AND (sqlc.narg(q)::text IS NULL OR entity_id::text ILIKE '%' || sqlc.narg(q) || '%'
       OR reason ILIKE '%' || sqlc.narg(q) || '%'
       OR COALESCE(notes, '') ILIKE '%' || sqlc.narg(q) || '%'
       OR COALESCE(entity_title, '') ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountModerationQueue :one
-- Same scope + filters as ListModerationQueue, for pagination totals.
WITH scoped AS (
    SELECT cr.id,
           COALESCE(cr.reviewed_by = @caller::uuid, false)::bool AS scope_assigned,
           COALESCE(
               (cr.entity_type = 'space'   AND cr.entity_id = ANY(@space_ids::uuid[])) OR
               (cr.entity_type = 'content' AND c.space_id   = ANY(@space_ids::uuid[])) OR
               (cr.entity_type = 'comment' AND pc.space_id  = ANY(@space_ids::uuid[])),
               false
           )::bool AS scope_space,
           EXISTS (SELECT 1 FROM content_tags ct
                    WHERE ct.content_id = COALESCE(c.id, pc.id)
                      AND ct.tag_id = ANY(@tag_ids::uuid[]))::bool AS scope_tag,
           cr.status, cr.entity_type, cr.entity_id, cr.reason, cr.notes,
           COALESCE(c.id, pc.id) AS resolved_content_id,
           COALESCE(c.content_type, pc.content_type, '')::text AS entity_content_type,
           CASE WHEN cr.entity_type = 'space' THEN cr.entity_id
                ELSE COALESCE(c.space_id, pc.space_id) END::uuid AS resolved_space_id,
           COALESCE(c.title, pc.title, s.name, tu.username, '') AS entity_title
    FROM content_reports cr
    LEFT JOIN contents c        ON cr.entity_type = 'content' AND c.id = cr.entity_id
    LEFT JOIN comments cm       ON cr.entity_type = 'comment' AND cm.id = cr.entity_id
    LEFT JOIN contents pc       ON pc.id = cm.content_id
    LEFT JOIN spaces s          ON cr.entity_type = 'space' AND s.id = cr.entity_id
    LEFT JOIN users tu          ON cr.entity_type = 'user' AND tu.id = cr.entity_id
)
SELECT COUNT(*)::int FROM scoped
WHERE (@platform::bool OR scope_assigned OR scope_space OR scope_tag)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type))
  AND (sqlc.narg(content_type)::text IS NULL OR entity_content_type = sqlc.narg(content_type))
  AND (sqlc.narg(filter_space_id)::uuid IS NULL OR resolved_space_id = sqlc.narg(filter_space_id)::uuid)
  AND (sqlc.narg(filter_tag_id)::uuid IS NULL OR
       EXISTS (SELECT 1 FROM content_tags ct
                WHERE ct.content_id = resolved_content_id AND ct.tag_id = sqlc.narg(filter_tag_id)::uuid))
  AND (@assigned_only::bool = false OR scope_assigned)
  AND (@open_only::bool = false OR status IN ('pending', 'reviewing'))
  AND (sqlc.narg(q)::text IS NULL OR entity_id::text ILIKE '%' || sqlc.narg(q) || '%'
       OR reason ILIKE '%' || sqlc.narg(q) || '%'
       OR COALESCE(notes, '') ILIKE '%' || sqlc.narg(q) || '%'
       OR COALESCE(entity_title, '') ILIKE '%' || sqlc.narg(q) || '%');

-- name: GetModerationQueueItem :one
-- Single report with the same scope flags + entity preview. The handler checks
-- the returned flags (or @platform) for authorization.
SELECT cr.id, cr.reporter_user_id, cr.entity_type, cr.entity_id, cr.reason, cr.notes,
       cr.evidence_urls, cr.status, cr.reviewed_by, cr.resolution_note, cr.resolved_at, cr.created_at,
       ru.username AS reporter_username,
       rup.display_name AS reporter_display_name,
       COALESCE(c.title, pc.title, s.name, tu.username, isrc.name, '') AS entity_title,
       COALESCE(LEFT(c.body, 240), LEFT((SELECT string_agg(DISTINCT n #>> '{}', ' ')
                    FROM jsonb_path_query(cm.body, '$.** ? (@.text.type() == "string").text') AS n), 240),
                s.description, LEFT(dm.body, 240), isrc.source_url, '') AS entity_excerpt,
       COALESCE(c.slug, pc.slug, s.slug, isrc.slug, '') AS entity_slug,
       COALESCE((SELECT thr.slug FROM forum_threads thr
             WHERE thr.content_id = COALESCE(c.id, pc.id) LIMIT 1), '')::text AS thread_slug,
       COALESCE(c.content_type, pc.content_type, '')::text AS entity_content_type,
       au.username AS entity_author_username,
       CASE WHEN cr.entity_type = 'space' THEN cr.entity_id
            ELSE COALESCE(c.space_id, pc.space_id) END::uuid AS resolved_space_id,
       sp.name AS entity_space_name,
       sp.slug AS entity_space_slug,
       COALESCE(cr.reviewed_by = @caller::uuid, false)::bool AS scope_assigned,
       COALESCE(
           (cr.entity_type = 'space'   AND cr.entity_id = ANY(@space_ids::uuid[])) OR
           (cr.entity_type = 'content' AND c.space_id   = ANY(@space_ids::uuid[])) OR
           (cr.entity_type = 'comment' AND pc.space_id  = ANY(@space_ids::uuid[])),
           false
       )::bool AS scope_space,
       EXISTS (SELECT 1 FROM content_tags ct
                WHERE ct.content_id = COALESCE(c.id, pc.id)
                  AND ct.tag_id = ANY(@tag_ids::uuid[]))::bool AS scope_tag
FROM content_reports cr
LEFT JOIN users ru          ON ru.id = cr.reporter_user_id
LEFT JOIN user_profiles rup ON rup.user_id = cr.reporter_user_id
LEFT JOIN contents c        ON cr.entity_type = 'content' AND c.id = cr.entity_id
LEFT JOIN comments cm       ON cr.entity_type = 'comment' AND cm.id = cr.entity_id
LEFT JOIN contents pc       ON pc.id = cm.content_id
LEFT JOIN spaces s          ON cr.entity_type = 'space' AND s.id = cr.entity_id
LEFT JOIN spaces sp         ON sp.id = COALESCE(c.space_id, pc.space_id)
LEFT JOIN users tu          ON cr.entity_type = 'user' AND tu.id = cr.entity_id
LEFT JOIN dm_messages dm    ON cr.entity_type = 'message' AND dm.id = cr.entity_id
LEFT JOIN intel_sources isrc ON cr.entity_type = 'source' AND isrc.id = cr.entity_id
LEFT JOIN users au          ON au.id = COALESCE(c.author_id, cm.author_user_id, dm.sender_id)
WHERE cr.id = @id;
