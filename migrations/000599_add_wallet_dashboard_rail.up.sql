-- 000599_add_wallet_dashboard_rail.up.sql
-- Adds the wallet dashboard rail item for the authenticated main-app nav,
-- gated on the feature.wallet feature flag.
-- Ref: ADR-054, ADR-058, Section 50

BEGIN;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status, feature_flag)
SELECT
    '00000000-0000-0000-0000-000000000001',
    m.id,
    'dashboard.nav.wallet',
    '/dashboard/wallet',
    'wallet',
    13,
    'authenticated',
    '_self',
    'active',
    'feature.wallet'
FROM menus m
WHERE m.slug = 'dashboard_rail'
  AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

COMMIT;
