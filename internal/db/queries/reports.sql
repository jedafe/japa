-- name: CreateReport :one
INSERT INTO content_reports (reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls) VALUES (@reporter_user_id, @entity_type, @entity_id, @reason, @notes, @evidence_urls)

ON CONFLICT (reporter_user_id, entity_type, entity_id) WHERE reporter_user_id IS NOT NULL DO NOTHING
RETURNING id, tenant_id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
    status, reviewed_by, resolution_note, resolved_at, created_at;

-- name: GetReport :one
SELECT id, tenant_id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
    status, reviewed_by, resolution_note, resolved_at, created_at
FROM content_reports
WHERE id = @id;

-- name: ListReportsByReporter :many
SELECT id, tenant_id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
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
RETURNING id, tenant_id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
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
