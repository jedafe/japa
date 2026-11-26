-- 001231_seed_dm_member_permissions.down.sql

BEGIN;

DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM user_roles WHERE slug IN ('member', 'pro_member', 'business_member'))
  AND permission_id IN (SELECT id FROM permissions WHERE key IN ('dm.view', 'dm.delete', 'dm.manage_privacy'));

COMMIT;
