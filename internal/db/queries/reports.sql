-- Content Reports & Flagging (JEOS Section 51).

-- name: CreateReport :one
INSERT INTO content_reports (tenant_id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls)
VALUES (@tenant_id, @reporter_user_id, @entity_type, @entity_id, @reason, @notes, @evidence_urls)
ON CONFLICT (reporter_user_id, entity_type, entity_id) WHERE reporter_user_id IS NOT NULL DO NOTHING
RETURNING id, tenant_id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
    status, reviewed_by, resolution_note, resolved_at, created_at;

-- name: GetReport :one
SELECT id, tenant_id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
    status, reviewed_by, resolution_note, resolved_at, created_at
FROM content_reports
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListReportsByReporter :many
SELECT id, tenant_id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
    status, reviewed_by, resolution_note, resolved_at, created_at
FROM content_reports
WHERE tenant_id = @tenant_id AND reporter_user_id = @reporter_user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListReports :many
SELECT id, tenant_id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
    status, reviewed_by, resolution_note, resolved_at, created_at
FROM content_reports
WHERE tenant_id = @tenant_id
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateReportStatus :one
UPDATE content_reports
SET status = @status, reviewed_by = @reviewed_by, resolution_note = @resolution_note,
    resolved_at = CASE WHEN @status IN ('actioned', 'dismissed') THEN now() ELSE resolved_at END
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, reporter_user_id, entity_type, entity_id, reason, notes, evidence_urls,
    status, reviewed_by, resolution_note, resolved_at, created_at;
