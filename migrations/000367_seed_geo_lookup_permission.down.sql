-- 000367_seed_geo_lookup_permission.down.sql
-- Reverse: remove the geo.lookup permission node added by the up migration.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
  SELECT id FROM permissions WHERE key = 'geo.lookup'
);

DELETE FROM permissions
WHERE key = 'geo.lookup';

COMMIT;
