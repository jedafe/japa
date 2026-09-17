-- jobs.sql — Jobs Board queries (Section 63, ADR-081).
-- Covers job application CRUD, finder queries on published job_listing
-- content, and employer/admin operations.
-- Ref: japaconnect-engineering/sections/63-jobs-board.md
--      japaconnect-engineering/adr/ADR-081-jobs-board.md

-- =====================================================================
-- Apply to a job (create application record).
-- =====================================================================

-- name: CreateJobApplication :one
INSERT INTO job_applications (job_id, applicant_id, submission_id) VALUES (@job_id, @applicant_id, sqlc.narg(submission_id))

ON CONFLICT (job_id, applicant_id) DO NOTHING
RETURNING *;

-- =====================================================================
-- Update application status (employer or admin).
-- =====================================================================

-- name: UpdateJobApplicationStatus :one
UPDATE job_applications
SET
 status = @status,
 employer_notes = COALESCE(sqlc.narg(employer_notes), employer_notes),
 status_updated_at = now()
WHERE id = @id
RETURNING *;

-- =====================================================================
-- Get a single application by ID.
-- =====================================================================

-- name: GetJobApplicationByID :one
SELECT * FROM job_applications
WHERE id = @id;

-- =====================================================================
-- List applications for a specific job (employer view).
-- =====================================================================

-- name: ListJobApplicationsByJob :many
SELECT
 ja.*,
 u.username AS applicant_username,
 u.email AS applicant_email
FROM job_applications ja
JOIN users u ON u.id = ja.applicant_id
WHERE ja.job_id = @job_id
 AND (@status::text = '' OR ja.status = @status)
ORDER BY ja.applied_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountJobApplicationsByJob :one
SELECT COUNT(*) AS count
FROM job_applications
WHERE job_id = @job_id
 AND (@status::text = '' OR status = @status);

-- =====================================================================
-- List an applicant's application history.
-- =====================================================================

-- name: ListUserJobApplications :many
SELECT
 ja.*,
 c.title AS job_title,
 c.slug AS job_slug
FROM job_applications ja
JOIN contents c ON c.id = ja.job_id
WHERE ja.applicant_id = @user_id
ORDER BY ja.applied_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountUserJobApplications :one
SELECT COUNT(*) AS count
FROM job_applications
WHERE applicant_id = @user_id;

-- =====================================================================
-- Admin: list all applications across all jobs.
-- =====================================================================

-- name: AdminListJobApplications :many
SELECT
 ja.*,
 u.username AS applicant_username,
 c.title AS job_title
FROM job_applications ja
JOIN users u ON u.id = ja.applicant_id
JOIN contents c ON c.id = ja.job_id
WHERE (@status::text = '' OR ja.status = @status)
ORDER BY ja.applied_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountJobApplications :one
SELECT COUNT(*) AS count
FROM job_applications
WHERE (@status::text = '' OR status = @status);

-- =====================================================================
-- Admin: application status distribution.
-- =====================================================================

-- name: AdminJobApplicationStatusDistribution :many
SELECT
 status,
 COUNT(*) AS count
FROM job_applications

GROUP BY status
ORDER BY count DESC;

-- =====================================================================
-- Expire listings past deadline — archive job_listing content whose
-- application_deadline custom field value is in the past.
-- This is a direct SQL query rather than sqlc because it joins
-- contents with custom_field_values and updates status.
-- =====================================================================

-- name: ExpireJobListingsPastDeadline :execrows
-- Archive published job_listing content whose application_deadline custom
-- field value is in the past. custom_field_values is EAV: field_id →
-- custom_fields.id, entity_type='content', entity_id = contents.id. The
-- application_deadline field is field_type='date' so its value is stored in
-- the value_date column (customfields.FieldTypeValueColumn). The value_text
-- fallback is guarded by an ISO-date regex — Postgres may evaluate the cast
-- before the cf.key predicate, and unrelated fields (e.g. destination_country)
-- store non-date text in value_text, which would otherwise fail the whole
-- task with SQLSTATE 22007.
UPDATE contents c
SET status = 'archived', updated_at = now()
WHERE c.content_type = 'job_listing'
 AND c.status = 'published'
 AND c.deleted_at IS NULL
 AND EXISTS (
 SELECT 1
 FROM custom_field_values cfv
 JOIN custom_fields cf ON cf.id = cfv.field_id
 WHERE cfv.entity_type = 'content'
 AND cfv.entity_id = c.id
 AND cf.key = 'application_deadline'
 AND (
   cfv.value_date < now()
   OR CASE WHEN cfv.value_text ~ '^\d{4}-\d{2}-\d{2}'
           THEN cfv.value_text::timestamptz END < now()
 )
 );
