-- 000658_seed_sources_main_sidebar.up.sql
-- Add a discoverable main-sidebar link for the public Sources directory.
-- Ref: docs/superpowers/specs/2026-09-11-sources-directory-design.md

BEGIN;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Sources', '/sources', 'newspaper', 3, 'all', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/sources');

COMMIT;
