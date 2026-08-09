-- Form, form step, and form submission queries.

-- =====================
-- Forms
-- =====================

-- name: ListFormsByTenant :many
SELECT f.id, f.tenant_id, f.space_id, f.name, f.slug, f.description, f.is_multi_step,
       f.submit_label, f.success_message, f.status, f.is_system, f.submission_fee_credits, f.created_at,
       s.name AS space_name
FROM forms f
LEFT JOIN spaces s ON s.id = f.space_id AND s.tenant_id = f.tenant_id
WHERE f.tenant_id = @tenant_id
ORDER BY f.created_at DESC, f.id DESC;

-- name: ListFormsBySpace :many
SELECT f.id, f.tenant_id, f.space_id, f.name, f.slug, f.description,
       f.is_multi_step, f.submit_label, f.success_message, f.status,
       f.is_system, f.submission_fee_credits, f.created_at,
       COALESCE(fc.cnt, 0)::bigint AS submission_count
FROM forms f
LEFT JOIN LATERAL (
    SELECT count(*) AS cnt
    FROM form_submissions fs
    WHERE fs.form_id = f.id
) fc ON true
WHERE f.tenant_id = @tenant_id AND f.space_id = @space_id
ORDER BY f.created_at DESC, f.id DESC;

-- name: GetFormByID :one
SELECT id, tenant_id, space_id, name, slug, description, is_multi_step,
       submit_label, success_message, status, is_system, submission_fee_credits, created_at
FROM forms
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetFormBySlug :one
SELECT id, tenant_id, space_id, name, slug, description, is_multi_step,
       submit_label, success_message, status, is_system, submission_fee_credits, created_at
FROM forms
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: CreateForm :one
INSERT INTO forms (tenant_id, space_id, name, slug, description, is_multi_step,
                    submit_label, success_message, status, is_system, submission_fee_credits)
VALUES (@tenant_id, sqlc.narg(space_id), @name, @slug, sqlc.narg(description),
        @is_multi_step, @submit_label, sqlc.narg(success_message), @status, @is_system,
        @submission_fee_credits)
RETURNING id, tenant_id, space_id, name, slug, description, is_multi_step,
          submit_label, success_message, status, is_system, submission_fee_credits, created_at;

-- name: UpdateForm :one
UPDATE forms
SET name = @name, description = sqlc.narg(description), is_multi_step = @is_multi_step,
    submit_label = @submit_label, success_message = sqlc.narg(success_message), status = @status,
    submission_fee_credits = @submission_fee_credits
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false
RETURNING id, tenant_id, space_id, name, slug, description, is_multi_step,
          submit_label, success_message, status, is_system, submission_fee_credits, created_at;

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

-- name: UpdateFormStep :one
UPDATE form_steps
SET title = @title, position = @position
WHERE id = @id AND form_id = @form_id
RETURNING id, form_id, title, position;

-- name: UpdateFormStepPosition :exec
UPDATE form_steps
SET position = @position
WHERE id = @id AND form_id = @form_id;

-- name: DeleteFormStep :exec
DELETE FROM form_steps
WHERE id = @id AND form_id = @form_id;

-- =====================
-- Form Submissions
-- =====================

-- name: ListFormSubmissionsByForm :many
SELECT id, form_id, tenant_id, user_id, ip_address, field_values, status, payment_id, created_at
FROM form_submissions
WHERE form_id = @form_id AND tenant_id = @tenant_id
  AND (sqlc.narg(status_filter) IS NULL OR status = sqlc.narg(status_filter))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountFormSubmissionsByForm :one
SELECT count(*) FROM form_submissions
WHERE form_id = @form_id AND tenant_id = @tenant_id
  AND (sqlc.narg(status_filter) IS NULL OR status = sqlc.narg(status_filter));

-- name: CreateFormSubmission :one
INSERT INTO form_submissions (form_id, tenant_id, user_id, ip_address, field_values, status, payment_id)
VALUES (@form_id, @tenant_id, sqlc.narg(user_id), sqlc.narg(ip_address), @field_values, @status, sqlc.narg(payment_id))
RETURNING id, form_id, tenant_id, user_id, ip_address, field_values, status, payment_id, created_at;

-- name: GetFormSubmissionByID :one
SELECT id, form_id, tenant_id, user_id, ip_address, field_values, status, payment_id, created_at
FROM form_submissions
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateFormSubmissionStatus :exec
UPDATE form_submissions
SET status = @status
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateFormSubmissionPaymentID :exec
-- Links a payment record to a form submission (for paid submissions confirmed
-- via payment gateway webhook). Ref: Section 06 Paid Form Submissions.
UPDATE form_submissions
SET payment_id = sqlc.narg(payment_id)
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetFormSubmissionByPaymentID :one
-- Finds a form submission by its linked payment_id. Used by the payment webhook
-- worker to update the submission status when a gateway payment is confirmed.
-- Ref: Section 06 Paid Form Submissions.
SELECT id, form_id, tenant_id, user_id, ip_address, field_values, status, payment_id, created_at
FROM form_submissions
WHERE payment_id = @payment_id AND tenant_id = @tenant_id;

-- name: DeleteFormSubmission :exec
DELETE FROM form_submissions
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Form Submission Triggers (Section 06)
-- =====================

-- name: ListFormSubmissionTriggersByForm :many
SELECT id, form_id, trigger_type, config, position
FROM form_submission_triggers
WHERE form_id = @form_id
ORDER BY position ASC;

-- name: CreateFormSubmissionTrigger :one
INSERT INTO form_submission_triggers (form_id, trigger_type, config, position)
VALUES (@form_id, @trigger_type, @config, @position)
RETURNING id, form_id, trigger_type, config, position;

-- name: UpdateFormSubmissionTrigger :one
UPDATE form_submission_triggers
SET trigger_type = @trigger_type, config = @config, position = @position
WHERE id = @id AND form_id = @form_id
RETURNING id, form_id, trigger_type, config, position;

-- name: DeleteFormSubmissionTrigger :exec
DELETE FROM form_submission_triggers
WHERE id = @id AND form_id = @form_id;

-- name: UpdateFormSubmissionTriggerPosition :exec
UPDATE form_submission_triggers
SET position = @position
WHERE id = @id AND form_id = @form_id;
