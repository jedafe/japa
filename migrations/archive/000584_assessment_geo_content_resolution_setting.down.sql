-- 000584_assessment_geo_content_resolution_setting.down.sql
-- Remove the assessment.geo_content_resolution_enabled setting.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'assessment.geo_content_resolution_enabled';

COMMIT;
