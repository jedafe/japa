-- 001191_treasury_killswitch_node.up.sql
-- §128: the signer killswitch halts ALL delegated-signer money movement —
-- a broader blast radius than approving one sweep. It gets its own node
-- seeded to super_admin only (was bundled under treasury.sweep_approve).
--
-- Ref: .jeos/sections/128-stablecoin-wallet-custody.md,
--      .jeos/sections/03-rbac.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('treasury.killswitch', 'Engage/disengage the stablecoin delegated-signer killswitch (halts all member spend + platform fee legs)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key = 'treasury.killswitch'
ON CONFLICT DO NOTHING;

COMMIT;
