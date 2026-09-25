-- 000552_jce_score_factors.down.sql

BEGIN;

DROP TABLE IF EXISTS comparison_score_factors CASCADE;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'jce.score_cache_ttl_minutes';

UPDATE comparison_type_definitions
SET config = config - 'scoring_enabled' - 'default_weights' - 'smart_modes'
WHERE is_system = true;

COMMIT;
