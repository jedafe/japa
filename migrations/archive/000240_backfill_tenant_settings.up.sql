-- 000240_backfill_tenant_settings.up.sql
-- Backfill the seed system_settings (originally seeded only for the default
-- tenant by 000099) into every other tenant that is missing them.
--
-- Background: the maintenance task `update_velocity_scores` iterates every
-- tenant and reads `feed.velocity.gravity` via Settings.GetFloatOr. Tenants
-- created after the seed (or test tenants) lack the row, so GetSetting logs
-- "settings: cache miss + DB miss" on every run even though GetFloatOr
-- falls back to its default. The same applies to the other 60 seed keys.
--
-- This copy is idempotent: it only inserts keys a tenant does not already
-- have, so tenant-specific overrides are never clobbered. Values are copied
-- from the default tenant (00000000-0000-0000-0000-000000000001), which is
-- the canonical source of the seed defaults.

BEGIN;

INSERT INTO system_settings (tenant_id, key, value, is_public)
SELECT t.id, s.key, s.value, s.is_public
FROM tenants t
CROSS JOIN system_settings s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND NOT EXISTS (
      SELECT 1
      FROM system_settings s2
      WHERE s2.tenant_id = t.id
        AND s2.key = s.key
  )
  AND t.id <> '00000000-0000-0000-0000-000000000001'
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
