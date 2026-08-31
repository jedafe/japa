-- 000578_jce_subspec4_settings.down.sql
-- Reverts 000578: removes the JCE Sub-spec 4 system settings.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'jce.freshness_stale_hours',
    'jce.freshness_decay_per_hour',
    'jce.freshness_official_weight',
    'jce.freshness_third_party_weight',
    'jce.freshness_ai_estimate_weight',
    'jce.freshness_cache_ttl_minutes',
    'jce.advisor_model',
    'jce.advisor_max_tokens',
    'jce.advisor_cache_ttl_minutes'
  );

COMMIT;
