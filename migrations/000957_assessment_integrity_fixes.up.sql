-- 000957_assessment_integrity_fixes.up.sql
-- §71/ADR-071 hardening (audit findings):
-- 1. UNIQUE index on assessment_results.submission_id — enforces the
--    documented "unique link submission_id -> result" so an Asynq retry can
--    never create a duplicate result for the same submission (BUG-BL-005).
-- 2. readiness_assessment.* agent tools require HITL — create/update/delete/
--    assign_geo on the live assessment form must go through human review
--    (BUG-RBAC-001). requires_hitl lives inside ai_tools.config JSONB.

BEGIN;

-- Remove duplicate results (keep the earliest per submission; ties break on
-- id) before adding the unique index so the DDL cannot fail on dupes.
DELETE FROM assessment_results a
USING assessment_results b
WHERE a.submission_id = b.submission_id
  AND (a.created_at > b.created_at OR (a.created_at = b.created_at AND a.id > b.id));

CREATE UNIQUE INDEX IF NOT EXISTS idx_assessment_results_submission_uniq
    ON assessment_results (submission_id);

UPDATE ai_tools
SET config = jsonb_set(config, '{requires_hitl}', 'true')
WHERE name IN (
    'readiness_assessment.create',
    'readiness_assessment.update',
    'readiness_assessment.delete',
    'readiness_assessment.assign_geo'
);

COMMIT;
