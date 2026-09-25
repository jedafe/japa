-- 000700_remove_wallet_dashboard_rail.down.sql
-- Renumbered from 000600 (duplicate version prefix). See the .up file.
-- 000600_remove_wallet_dashboard_rail.down.sql
-- Restore the wallet dashboard rail item.
-- De-tenanted form: menu_items.tenant_id and menus.tenant_id were dropped by
-- 000668, so the tenant column/filters are gone.

BEGIN;

INSERT INTO menu_items (
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
    m.id,
    'dashboard.nav.wallet',
    '/dashboard/wallet',
    'wallet',
    (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM menu_items WHERE menu_id = m.id),
    'authenticated',
    '_self',
    false,
    'active',
    '[]'::jsonb,
    'feature.wallet'
FROM menus m
WHERE m.slug = 'dashboard_rail'
ON CONFLICT DO NOTHING;

COMMIT;
