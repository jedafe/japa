-- Email Templates — Tenant-customizable transactional emails (ADR-054)

-- name: ListEmailTemplates :many
SELECT id, tenant_id, type, name, slug, subject, html_body, plain_text_body,
       is_active, variables, status, created_at, updated_at
FROM email_templates
WHERE tenant_id = @tenant_id
  AND (@type = '' OR type = @type)
  AND (@is_active = '' OR is_active = (@is_active = 'true'))
ORDER BY type ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountEmailTemplates :one
SELECT COUNT(*)::int AS count
FROM email_templates
WHERE tenant_id = @tenant_id
  AND (@type = '' OR type = @type)
  AND (@is_active = '' OR is_active = (@is_active = 'true'));

-- name: GetEmailTemplate :one
SELECT id, tenant_id, type, name, slug, subject, html_body, plain_text_body,
       is_active, variables, status, created_at, updated_at
FROM email_templates
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetEmailTemplateByType :one
SELECT id, tenant_id, type, name, slug, subject, html_body, plain_text_body,
       is_active, variables, status, created_at, updated_at
FROM email_templates
WHERE tenant_id = @tenant_id AND type = @type;

-- name: UpdateEmailTemplate :one
UPDATE email_templates
SET subject = @subject, html_body = @html_body,
    plain_text_body = @plain_text_body, status = @status,
    is_active = @is_active,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, type, name, slug, subject, html_body, plain_text_body,
    is_active, variables, status, created_at, updated_at;

-- name: ResetEmailTemplate :exec
UPDATE email_templates
SET html_body = '', updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetEmailTemplateBySlug :one
SELECT id, tenant_id, type, name, slug, subject, html_body, plain_text_body,
       is_active, variables, status, created_at, updated_at
FROM email_templates
WHERE slug = @slug AND tenant_id = @tenant_id AND is_active = true;
