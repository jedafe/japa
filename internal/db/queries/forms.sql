-- Form, form step, and form submission queries.

-- =====================
-- Forms
-- =====================

-- name: ListFormsByTenant :many
SELECT id, tenant_id, name, slug, description, is_multi_step, submit_label,
       success_message, status, is_system, created_at
FROM forms
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC, id DESC;

-- name: GetFormByID :one
SELECT id, tenant_id, name, slug, description, is_multi_step, submit_label,
       success_message, status, is_system, created_at
FROM forms
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetFormBySlug :one
SELECT id, tenant_id, name, slug, description, is_multi_step, submit_label,
       success_message, status, is_system, created_at
FROM forms
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: CreateForm :one
INSERT INTO forms (tenant_id, name, slug, description, is_multi_step, submit_label, success_message, status, is_system)
VALUES (@tenant_id, @name, @slug, sqlc.narg(description), @is_multi_step, @submit_label, sqlc.narg(success_message), @status, @is_system)
RETURNING id, tenant_id, name, slug, description, is_multi_step, submit_label,
          success_message, status, is_system, created_at;

-- name: UpdateForm :one
UPDATE forms
SET name = @name, description = sqlc.narg(description), is_multi_step = @is_multi_step,
    submit_label = @submit_label, success_message = sqlc.narg(success_message), status = @status
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false
RETURNING id, tenant_id, name, slug, description, is_multi_step, submit_label,
          success_message, status, is_system, created_at;

-- name: DeleteForm :exec
DELETE FROM forms
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false;

-- =====================
-- Form Steps
-- =====================

-- name: ListFormStepsByForm :many
SELECT id, form_id, title, position
FROM form_steps
WHERE form_id = @form_id
ORDER BY position ASC;

-- name: CreateFormStep :one
INSERT INTO form_steps (form_id, title, position)
VALUES (@form_id, @title, @position)
RETURNING id, form_id, title, position;

-- name: DeleteFormStep :exec
DELETE FROM form_steps
WHERE id = @id AND form_id = @form_id;

-- =====================
-- Form Submissions
-- =====================

-- name: ListFormSubmissionsByForm :many
SELECT id, form_id, tenant_id, user_id, ip_address, field_values, status, created_at
FROM form_submissions
WHERE form_id = @form_id AND tenant_id = @tenant_id
  AND (sqlc.narg(cursor_created_at) IS NULL
       OR created_at < sqlc.narg(cursor_created_at)
       OR (created_at = sqlc.narg(cursor_created_at) AND id < sqlc.narg(cursor_id)))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: CreateFormSubmission :one
INSERT INTO form_submissions (form_id, tenant_id, user_id, ip_address, field_values, status)
VALUES (@form_id, @tenant_id, sqlc.narg(user_id), sqlc.narg(ip_address), @field_values, @status)
RETURNING id, form_id, tenant_id, user_id, ip_address, field_values, status, created_at;

-- name: GetFormSubmissionByID :one
SELECT id, form_id, tenant_id, user_id, ip_address, field_values, status, created_at
FROM form_submissions
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateFormSubmissionStatus :exec
UPDATE form_submissions
SET status = @status
WHERE id = @id AND tenant_id = @tenant_id;

-- name: DeleteFormSubmission :exec
DELETE FROM form_submissions
WHERE id = @id AND tenant_id = @tenant_id;
