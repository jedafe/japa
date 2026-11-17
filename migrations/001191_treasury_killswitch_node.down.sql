-- 001191_treasury_killswitch_node.down.sql

BEGIN;

DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'treasury.killswitch'
);
DELETE FROM permissions WHERE key = 'treasury.killswitch';

COMMIT;
