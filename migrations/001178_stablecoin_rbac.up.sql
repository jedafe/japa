-- 001178_stablecoin_rbac.up.sql
-- §128 RBAC seeds: admin-side nodes for member-wallet state reads,
-- treasury registry, sweep approval, and escrow arbitration.
-- stablecoin.wallet.view exposes member wallet metadata only — keys are
-- never serialized (funds are non-custodial). escrow.arbitrate grants
-- decision authority, not unilateral fund control (onchain co-sign
-- required). Member-side access is feature.stablecoin_wallet + auth.
--
-- Ref: .jeos/sections/128-stablecoin-wallet-custody.md,
--      .jeos/conventions/permission-key-naming.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('stablecoin.wallet.view',   'View member stablecoin wallet state and mirrored onchain activity'),
    ('treasury.view',            'View platform treasury registry and sweep proposals'),
    ('treasury.sweep_approve',   'Approve hot-to-cold treasury sweep proposals (Squads ceremony queue)'),
    ('escrow.arbitrate',         'Record dispute rulings on service escrows (onchain co-signer still required)')
ON CONFLICT (key) DO NOTHING;

-- Admin-tier grants: wallet view, treasury view, escrow arbitration.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('admin-group-role', 'super_admin', 'super_admin_role')
  AND p.key IN ('stablecoin.wallet.view', 'treasury.view', 'escrow.arbitrate')
ON CONFLICT DO NOTHING;

-- Sweep approval is super-admin only — the platform's own money movement.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key = 'treasury.sweep_approve'
ON CONFLICT DO NOTHING;

COMMIT;
