BEGIN;
DROP INDEX IF EXISTS idx_assessment_results_submission_uniq;
UPDATE ai_tools
SET config = jsonb_set(config, '{requires_hitl}', 'false')
WHERE name IN (
    'readiness_assessment.create',
    'readiness_assessment.update',
    'readiness_assessment.delete',
    'readiness_assessment.assign_geo'
);
COMMIT;
