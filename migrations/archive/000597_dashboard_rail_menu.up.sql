-- 000597_dashboard_rail_menu.up.sql
-- Adds the dashboard_rail placement to menus and seeds the default dashboard
-- rail menu for the main app's /dashboard secondary left nav.
-- Ref: ADR-054, ADR-058, Section 50

BEGIN;

-- Widen the placement check constraint to allow dashboard_rail.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'menus'::regclass
          AND contype = 'c'
          AND pg_get_constraintdef(oid) LIKE '%placement%'
    ) THEN
        ALTER TABLE menus DROP CONSTRAINT menus_placement_check;
    END IF;
END $$;

-- The Go ValidPlacement set already permitted dropdown, footer, and header;
-- align the check constraint with the full valid set before adding dashboard_rail.
ALTER TABLE menus
    ADD CONSTRAINT menus_placement_check
    CHECK (placement IN ('sidebar', 'bottom_bar', 'dropdown', 'footer', 'header', 'dashboard_rail'));

-- Seed the dashboard rail menu for the default tenant.
INSERT INTO menus (tenant_id, slug, name, placement, status)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'dashboard_rail',
    'Dashboard Rail',
    'dashboard_rail',
    'active'
)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Seed default dashboard rail items. Labels are i18n keys rendered by the hub.
INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.overview', '/dashboard/overview', 'layout-dashboard', 0, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.analytics', '/dashboard/analytics', 'bar-chart-3', 1, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.content', '/dashboard/content', 'file-text', 2, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.relationships', '/dashboard/relationships', 'users', 3, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.learning', '/dashboard/learning', 'graduation-cap', 4, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.certificates', '/dashboard/certificates', 'award', 5, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.knowledge_base', '/dashboard/knowledge-base', 'book-open', 6, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.tickets', '/dashboard/tickets', 'ticket', 7, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.bookmarks', '/dashboard/bookmarks', 'bookmark', 8, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.media', '/dashboard/media', 'image', 9, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.spaces', '/dashboard/spaces', 'users-round', 10, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.organizations', '/dashboard/organizations', 'building-2', 11, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.profiles', '/dashboard/profiles', 'briefcase', 12, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

COMMIT;
