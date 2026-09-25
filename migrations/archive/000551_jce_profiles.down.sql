-- 000551_jce_profiles.down.sql

BEGIN;

DROP TABLE IF EXISTS comparison_profiles CASCADE;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('jce.default_goals', 'jce.profile_cache_ttl_minutes');

UPDATE comparison_type_definitions
SET config = config - 'allowed_goals' - 'profile_defaults' - 'eligibility_rules' - 'goal_factor_map'
WHERE is_system = true;

COMMIT;
