-- 000351_seed_mentorship_settings.down.sql
-- Rollback: remove mentorship settings and comparison type.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('mentorship.max_capacity');

DELETE FROM comparison_type_definitions
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'mentors';

COMMIT;
