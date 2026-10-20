-- assessment.sql — Japa Readiness Assessment queries (Section 71, ADR-071).
-- Covers assessment form retrieval, result storage, history, and admin stats.
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md
--      japaconnect-engineering/adr/ADR-071-japa-readiness-assessment.md

-- =====================================================================
-- Get the active assessment form (form_type='assessment', status='active').
-- =====================================================================

-- name: GetActiveAssessmentForm :one
SELECT * FROM forms
WHERE form_type = 'assessment'
 AND status = 'active'
ORDER BY created_at DESC
LIMIT 1;

-- =====================================================================
-- Get an assessment form by ID.
-- =====================================================================

-- name: GetAssessmentFormByID :one
SELECT * FROM forms
WHERE id = @id AND form_type = 'assessment';

-- =====================================================================
-- Update the assessment form's scoring_config.
-- =====================================================================

-- name: UpdateAssessmentFormScoring :one
UPDATE forms
SET scoring_config = @scoring_config
WHERE id = @id AND form_type = 'assessment'
RETURNING *;

-- =====================================================================
-- Create a form submission for an assessment (anonymous or authenticated).
-- =====================================================================

-- name: CreateAssessmentSubmission :one
INSERT INTO form_submissions (form_id, user_id, field_values, status) VALUES (@form_id, sqlc.narg(user_id), @field_values, 'new')

RETURNING *;

-- =====================================================================
-- Store an assessment result.
-- =====================================================================

-- name: CreateAssessmentResult :one
INSERT INTO assessment_results (user_id, submission_id, total_score, dimension_scores, threshold_label, roadmap_content) VALUES (sqlc.narg(user_id), @submission_id, @total_score, @dimension_scores, @threshold_label, @roadmap_content)

RETURNING *;

-- =====================================================================
-- Get a single assessment result by ID.
-- =====================================================================

-- name: GetAssessmentResultByID :one
SELECT * FROM assessment_results
WHERE id = @id;

-- =====================================================================
-- Get a single assessment result by submission_id. Used by the public
-- polling endpoint after an async submit (the client receives the
-- submission_id and polls until the Asynq scoring task writes the result).
-- Returns at most one row (there is a unique link submission_id -> result).
-- =====================================================================

-- name: GetAssessmentResultBySubmission :one
SELECT * FROM assessment_results
WHERE submission_id = @submission_id;

-- =====================================================================
-- List a user's assessment results (history).
-- =====================================================================

-- name: ListUserAssessmentResults :many
SELECT * FROM assessment_results
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountUserAssessmentResults :one
SELECT COUNT(*) AS count
FROM assessment_results
WHERE user_id = @user_id;

-- =====================================================================
-- Count prior assessment results for a user and form. Used to award
-- completion points only on the first submission (Rule 9 / Section 28).
-- =====================================================================

-- name: CountAssessmentResultsByUserAndForm :one
SELECT COUNT(*) AS count
FROM assessment_results ar
JOIN form_submissions fs ON fs.id = ar.submission_id
WHERE ar.user_id = @user_id
 AND fs.form_id = @form_id;

-- =====================================================================
-- Admin: list all users' results (paginated, filterable by score range).
-- =====================================================================

-- name: AdminListAssessmentResults :many
SELECT
 ar.*,
 u.username AS username,
 fs.field_values AS submission_values
FROM assessment_results ar
LEFT JOIN users u ON u.id = ar.user_id
JOIN form_submissions fs ON fs.id = ar.submission_id
WHERE (@min_score::int = 0 OR ar.total_score >= @min_score)
 AND (@max_score::int = 0 OR ar.total_score <= @max_score)
ORDER BY ar.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountAssessmentResults :one
SELECT COUNT(*) AS count
FROM assessment_results
WHERE (@min_score::int = 0 OR total_score >= @min_score)
 AND (@max_score::int = 0 OR total_score <= @max_score);

-- =====================================================================
-- Admin: score distribution — count of results per threshold label.
-- =====================================================================

-- name: AdminAssessmentScoreDistribution :many
SELECT
 threshold_label,
 COUNT(*) AS count,
 AVG(total_score)::int AS avg_score
FROM assessment_results

GROUP BY threshold_label
ORDER BY avg_score DESC;

-- =====================================================================
-- Admin: completion stats — total submissions, total scored, avg score.
-- =====================================================================

-- name: AdminAssessmentCompletionStats :one
SELECT
 COUNT(DISTINCT fs.id) AS total_submissions,
 COUNT(DISTINCT ar.id) AS total_scored,
 COALESCE(AVG(ar.total_score), 0)::int AS avg_score
FROM form_submissions fs
LEFT JOIN assessment_results ar ON ar.submission_id = fs.id
JOIN forms f ON f.id = fs.form_id
WHERE f.form_type = 'assessment';

-- =====================================================================
-- Get form fields for an assessment form (for scoring engine to resolve
-- per-answer scores from field config). Forms link to custom_fields via
-- custom_field_group_assignments (entity_class='form').
-- =====================================================================

-- name: GetAssessmentFormFields :many
SELECT cf.*
FROM custom_field_group_assignments a
JOIN custom_fields cf ON cf.group_id = a.group_id
WHERE a.entity_class = 'form' AND a.entity_id = @form_id
  AND cf.show_in_frontend = TRUE
ORDER BY cf.position;

-- =====================================================================
-- GDPR erasure: delete assessment_results by user_id. The FK has ON DELETE
-- CASCADE but we delete explicitly to document the erasure path and ensure
-- PII-adjacent data (dimension scores, roadmap) is removed before the final
-- HardDeleteUser (Section 15, ADR-014). Ref: §71, ADR-071.
-- =====================================================================

-- name: GdprDeleteAssessmentResultsByUser :exec
DELETE FROM assessment_results WHERE user_id = @user_id;

-- =====================================================================
-- Cleanup anonymous assessment results older than the retention window.
-- Anonymous results (user_id IS NULL) are persisted temporarily for the
-- async polling architecture (ADR-071 §5) and cleaned up periodically.
-- =====================================================================

-- name: CleanupAnonymousAssessmentResults :execrows
DELETE FROM assessment_results
WHERE user_id IS NULL AND created_at < now() - ($1::int || ' hours')::interval;

-- Delete anonymous assessment form_submissions past the retention window
-- once their result row is gone (either cleaned above or never written).
-- Their field_values carry journey context (origin/destination/visa route)
-- so they must not outlive the anonymous result retention window.

-- name: CleanupAnonymousAssessmentSubmissions :execrows
DELETE FROM form_submissions fs
USING forms f
WHERE fs.form_id = f.id
  AND f.form_type = 'assessment'
  AND fs.user_id IS NULL
  AND fs.created_at < now() - ($1::int || ' hours')::interval
  AND NOT EXISTS (
      SELECT 1 FROM assessment_results ar WHERE ar.submission_id = fs.id
  );

-- =====================================================================
-- Get the latest assessment result for a user, including the original
-- submission field_values (used to infer origin, destination, visa route).
-- Returns no rows if the user has not completed an assessment.
-- Ref: docs/superpowers/specs/2026-08-21-journey-aware-recommendations-design.md
-- =====================================================================

-- name: GetLatestAssessmentJourneyContext :one
SELECT
 ar.roadmap_content,
 ar.dimension_scores,
 ar.threshold_label,
 ar.created_at,
 fs.field_values AS submission_values
FROM assessment_results ar
JOIN form_submissions fs ON fs.id = ar.submission_id
WHERE ar.user_id = @user_id
ORDER BY ar.created_at DESC
LIMIT 1;
