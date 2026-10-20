-- 000917_wallet_member_rbac_grants.up.sql
-- Grants the enforced member-facing wallet permission nodes to the
-- member-tier roles. The §28 routes enforce `wallet.points.balance`
-- (GET /me/wallet, /me/wallet/points, /me/wallet/credits, /me/wallet/ledger,
-- /me/wallet/settings) and `wallet.points.convert` (POST /me/wallet/convert),
-- but no role held either node — every non-admin caller received 403, so the
-- member wallet surface (balances, conversion, ledger history) was dead.
-- The older `wallet.view`/`wallet.convert` grants are enforced by no route
-- and are left in place (harmless legacy).
--
-- Grants per .jeos/conventions/permission-key-naming.md and sections/28:
--   wallet.points.balance  → member+ (view own balance, ledgers, settings)
--   wallet.points.convert  → member+ (convert points to credits)
--
-- super_admin is unaffected (covered by the group wildcard bypass).
-- Grants are additive; the per-user permission cache (jc:perm:{user_id})
-- refreshes on its TTL.

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key IN (
    'wallet.points.balance',
    'wallet.points.convert'
)
WHERE r.slug IN ('member', 'pro_member', 'business_member')
ON CONFLICT DO NOTHING;

COMMIT;
