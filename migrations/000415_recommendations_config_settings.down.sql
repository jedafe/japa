-- 000415_recommendations_config_settings.down.sql
-- Rollback: remove the recommendation engine config settings added by 000415.

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'geo_personalization.recommendations_cache_ttl_seconds',
    'geo_personalization.recommendations_max_limit'
  );
