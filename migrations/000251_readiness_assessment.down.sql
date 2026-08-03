-- 000251_readiness_assessment.down.sql
-- Reverses 000251_readiness_assessment.up.sql.

BEGIN;

-- Revoke assessment.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('assessment.take', 'assessment.manage', 'assessment.view_results');

-- Remove assessment.* permission nodes.
DELETE FROM permissions WHERE key IN ('assessment.take', 'assessment.manage', 'assessment.view_results');

-- Remove assessment.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('assessment.enabled', 'assessment.ai_roadmap_enabled', 'assessment.completion_points');

-- Drop the assessment_results table.
DROP TABLE IF EXISTS assessment_results;

-- Remove the added columns from forms.
ALTER TABLE forms DROP COLUMN IF EXISTS scoring_config;
ALTER TABLE forms DROP COLUMN IF EXISTS form_type;

-- Remove config column from custom_fields.
ALTER TABLE custom_fields DROP COLUMN IF EXISTS config;

COMMIT;
