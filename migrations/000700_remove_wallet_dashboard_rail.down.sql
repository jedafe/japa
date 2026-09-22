-- 000700_remove_wallet_dashboard_rail.down.sql
-- Renumbered from 000600 (duplicate version prefix). See the .up file.
-- 000600_remove_wallet_dashboard_rail.down.sql
-- Restore the wallet dashboard rail item.

BEGIN;

INSERT INTO menu_items (
    tenant_id,
    menu_id,
    label,
    url,
    icon,
    sort_order,
    visibility,
    target,
    is_ad_slot,
    status,
    required_group_ids,
    feature_flag
)
SELECT
    '00000000-0000-0000-0000-000000000001',
    m.id,
    'dashboard.nav.wallet',
    '/dashboard/wallet',
    'wallet',
    (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM menu_items WHERE menu_id = m.id AND tenant_id = '00000000-0000-0000-0000-000000000001'),
    'authenticated',
    '_self',
    false,
    'active',
    '[]'::jsonb,
    'feature.wallet'
FROM menus m
WHERE m.slug = 'dashboard_rail'
  AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

COMMIT;
