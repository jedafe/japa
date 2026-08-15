-- 000486_seed_cv_standards_all_countries.down.sql
-- Reverses 000486: removes all seeded cv_standards and resets the feature flag.

BEGIN;

-- Remove all seeded standards (admin-created standards with idempotent
-- ON CONFLICT DO NOTHING are preserved if they were created before this
-- migration; we only delete standards for country-level spaces that this
-- migration seeded).
DELETE FROM cv_standards
WHERE destination_space_id IN (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND level = 'country'
);

-- Reset the feature flag to disabled (matches 000345 default).
UPDATE system_settings
SET value = 'false'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'cv_formatter.enabled';

COMMIT;
