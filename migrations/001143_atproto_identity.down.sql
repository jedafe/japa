-- 001143_atproto_identity.down.sql — drop §127 A1 identity schema + RBAC grants.
BEGIN;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key IN ('atproto.use','atproto.accounts.view');

DELETE FROM permissions WHERE key IN ('atproto.use','atproto.accounts.view');

DROP TABLE IF EXISTS atproto_accounts;

COMMIT;
