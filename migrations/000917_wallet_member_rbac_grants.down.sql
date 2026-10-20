-- 000917_wallet_member_rbac_grants.down.sql
-- Revokes the member-tier wallet grants added by 000917.

BEGIN;

DELETE FROM role_permissions rp
USING user_roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.slug IN ('member', 'pro_member', 'business_member')
  AND p.key IN (
      'wallet.points.balance',
      'wallet.points.convert'
  );

COMMIT;
