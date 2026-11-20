-- 001213_access_gates_dashboard_rail.up.sql
-- Adds the ops access-gates dashboard rail item, gated by permission_node
-- '*' — the wildcard is only resolvable by the super_admin group, so the
-- nav entry renders for super admins alone. Backend authorization lives in
-- /admin/access-gates (super_admin group check), not in this menu item.
-- Ref: sections/123-ops-access-gates.md, ADR-054

BEGIN;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status, permission_node)
SELECT
    '00000000-0000-0000-0000-000000000001',
    m.id,
    'dashboard.nav.access_gates',
    '/dashboard/access-gates',
    'shield',
    90,
    'authenticated',
    '_self',
    'active',
    '*'
FROM menus m
WHERE m.slug = 'dashboard_rail'
  AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

COMMIT;
