-- 000214_seed_header_menus.up.sql
-- Adds 'header' to the menu placement CHECK constraint and seeds header
-- navigation menus for the main, learn, and docs apps.
-- Ref: ADR-054 (Navigation Menus)

BEGIN;

-- ============================================================
-- 1. Extend placement CHECK to include header
-- ============================================================

ALTER TABLE menus DROP CONSTRAINT IF EXISTS menus_placement_check;
ALTER TABLE menus ADD CONSTRAINT menus_placement_check
    CHECK (placement IN ('sidebar', 'bottom_bar', 'dropdown', 'footer', 'header'));

-- ============================================================
-- 2. Seed header menus for all three apps
-- ============================================================

INSERT INTO menus (tenant_id, slug, name, placement) VALUES
    ('00000000-0000-0000-0000-000000000001', 'main_header', 'Main Header', 'header'),
    ('00000000-0000-0000-0000-000000000001', 'learn_header', 'Learn Header', 'header'),
    ('00000000-0000-0000-0000-000000000001', 'docs_header', 'Docs Header', 'header')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- 3. Main Header items (guest-oriented top bar)
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Explore', '/explore', 'compass', 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Learn', '/learn', 'graduation-cap', 1, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Blog', '/blog', 'newspaper', 2, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Docs', '/docs', 'book-open', 3, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Login', '/login', 'log-in', 10, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'main_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Sign up', '/signup', 'user-plus', 11, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'main_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Home', '/', 'home', 0, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Notifications', '/notifications', 'bell', 1, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Messages', '/messages', 'mail', 2, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Bookmarks', '/bookmarks', 'bookmark', 3, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- 4. Learn Header items
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Explore', '/', 'compass', 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'My Learning', '/my-learning', 'layout-dashboard', 1, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Certificates', '/certificates', 'trophy', 2, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Instructor Dashboard', '/dashboard/instructor', 'graduation-cap', 3, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Main App', '/', 'home', 4, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Docs', '/docs', 'book-open', 5, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Login', '/login', 'log-in', 10, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Sign up', '/signup', 'user-plus', 11, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- 5. Docs Header items
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Docs Home', '/docs', 'book-open', 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Getting Started', '/docs/getting-started', 'rocket', 1, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'API Reference', '/docs/api', 'code', 2, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Guides', '/docs/guides', 'book-open', 3, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Main App', '/', 'home', 4, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Learn', '/learn', 'graduation-cap', 5, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Login', '/login', 'log-in', 10, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Sign up', '/signup', 'user-plus', 11, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_header' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
