-- 001142_activitypub_foundation.down.sql
-- Reverses 001142 (§126 ActivityPub Federation): RBAC grants + nodes,
-- then the six federation tables.

BEGIN;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key IN ('federation.use','federation.actors.view','federation.instances.manage');

DELETE FROM permissions
WHERE key IN ('federation.use','federation.actors.view','federation.instances.manage');

DROP TABLE IF EXISTS federation_instance_blocks;
DROP TABLE IF EXISTS federation_inbound;
DROP TABLE IF EXISTS federation_deliveries;
DROP TABLE IF EXISTS federation_follows;
DROP TABLE IF EXISTS remote_actors;
DROP TABLE IF EXISTS federated_actors;

COMMIT;
