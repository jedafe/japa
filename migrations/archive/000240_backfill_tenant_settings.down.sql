-- 000240_backfill_tenant_settings.down.sql
-- Reverse the 000240 backfill. Only removes settings for non-default tenants
-- whose value still matches the default tenant's seed value, so any tenant
-- that customized a setting after the backfill keeps its override.

BEGIN;

DELETE FROM system_settings s
WHERE s.tenant_id <> '00000000-0000-0000-0000-000000000001'
  AND EXISTS (
      SELECT 1
      FROM system_settings d
      WHERE d.tenant_id = '00000000-0000-0000-0000-000000000001'
        AND d.key = s.key
        AND d.value = s.value
  );

COMMIT;
