-- 001174_federated_actor_key_rotation.down.sql
BEGIN;

DELETE FROM role_permissions WHERE permission_id IN
    (SELECT id FROM permissions WHERE key = 'federation.actors.manage');
DELETE FROM permissions WHERE key = 'federation.actors.manage';
DELETE FROM system_settings WHERE key = 'federation.key_rotation.cooldown_hours';
ALTER TABLE federated_actors DROP COLUMN IF EXISTS key_rotated_at;

COMMIT;
