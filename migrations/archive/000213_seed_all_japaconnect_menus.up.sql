-- 000213_seed_all_japaconnect_menus.up.sql
-- Extends menu placement to support dropdown and footer placements, then seeds
-- all JapaConnect navigation menus (user dropdowns, footers, docs sidebar, learn
-- sidebar/bottom bar) so admins can manage every navigational surface from the
-- admin panel instead of hardcoded Svelte layouts.
-- Ref: ADR-054 (Navigation Menus)

BEGIN;

-- ============================================================
-- 1. Extend placement CHECK to include dropdown and footer
-- ============================================================

ALTER TABLE menus DROP CONSTRAINT IF EXISTS menus_placement_check;
ALTER TABLE menus ADD CONSTRAINT menus_placement_check
    CHECK (placement IN ('sidebar', 'bottom_bar', 'dropdown', 'footer'));

-- ============================================================
-- 2. Seed all JapaConnect menus for the default tenant
-- ============================================================

INSERT INTO menus (tenant_id, slug, name, placement) VALUES
    ('00000000-0000-0000-0000-000000000001', 'main_user_dropdown',  'Main User Dropdown',  'dropdown'),
    ('00000000-0000-0000-0000-000000000001', 'main_footer',         'Main Footer',         'footer'),
    ('00000000-0000-0000-0000-000000000001', 'docs_sidebar',        'Docs Sidebar',        'sidebar'),
    ('00000000-0000-0000-0000-000000000001', 'docs_user_dropdown',  'Docs User Dropdown',  'dropdown'),
    ('00000000-0000-0000-0000-000000000001', 'learn_sidebar',       'Learn Sidebar',       'sidebar'),
    ('00000000-0000-0000-0000-000000000001', 'learn_user_dropdown', 'Learn User Dropdown', 'dropdown'),
    ('00000000-0000-0000-0000-000000000001', 'learn_bottom_bar',    'Learn Mobile Bottom Bar', 'bottom_bar'),
    ('00000000-0000-0000-0000-000000000001', 'auth_footer',         'Auth Footer',         'footer')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- 3. Seed menu items — Main User Dropdown (authenticated)
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Profile', '/profile', 'user', 0, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Dashboard', '/dashboard', 'layout-dashboard', 1, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Bookmarks', '/dashboard/bookmarks', 'bookmark', 2, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Wallet', '/settings/wallet', 'wallet', 3, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Settings', '/settings/profile', 'settings', 4, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Logout', '/logout', 'log-out', 5, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'main_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- Main User Dropdown (guest)
INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Login', '/login', 'log-in', 0, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'main_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Sign up', '/signup', 'user', 1, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'main_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- 4. Seed menu items — Main Footer
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Explore', '/explore', NULL, 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Blog', '/blog', NULL, 1, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Search', '/search', NULL, 2, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'About', '/about', NULL, 3, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Docs', '/docs', NULL, 4, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Privacy', '/privacy', NULL, 5, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Terms', '/terms', NULL, 6, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'main_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Sign up', '/signup', NULL, 7, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'main_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Log in', '/login', NULL, 8, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'main_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- 5. Seed menu items — Docs Sidebar
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Home', '/docs', 'home', 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Getting Started', '/docs/getting-started', 'rocket', 1, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'API Reference', '/docs/api', 'code', 2, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Guides', '/docs/guides', 'book-open', 3, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Admin Guide', '/docs/admin', 'shield', 4, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Integrations', '/docs/integrations', 'plug', 5, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- 6. Seed menu items — Docs User Dropdown
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Login', '/login', 'log-in', 0, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Docs Home', '/docs', 'book', 0, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Main App', '/', 'home', 1, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Settings', '/settings/profile', 'settings', 2, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Logout', '/logout', 'log-out', 3, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'docs_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- 7. Seed menu items — Learn Sidebar
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Explore', '/', 'compass', 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'My Learning', '/my-learning', 'layout-dashboard', 1, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Certificates', '/certificates', 'trophy', 2, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Bookmarks', '/bookmarks', 'bookmark', 3, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Instructor Dashboard', '/dashboard/instructor', 'graduation-cap', 4, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Login', '/login', 'log-in', 10, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- 8. Seed menu items — Learn User Dropdown
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Profile', '/profile', 'user', 0, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'My Learning', '/my-learning', 'layout-dashboard', 1, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Certificates', '/certificates', 'trophy', 2, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Bookmarks', '/bookmarks', 'bookmark', 3, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Settings', '/settings', 'settings', 4, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Logout', '/logout', 'log-out', 5, 'authenticated', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Login', '/login', 'log-in', 0, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Sign up', '/signup', 'user', 1, 'guest', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_user_dropdown' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- 9. Seed menu items — Learn Mobile Bottom Bar
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Explore', '/', 'compass', 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_bottom_bar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'My Learning', '/my-learning', 'layout-dashboard', 1, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_bottom_bar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Certificates', '/certificates', 'trophy', 2, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_bottom_bar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Bookmarks', '/bookmarks', 'bookmark', 3, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_bottom_bar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Profile', '/profile', 'user', 4, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'learn_bottom_bar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- 10. Seed menu items — Auth Footer
-- ============================================================

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Terms', '/terms', NULL, 0, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'auth_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Privacy', '/privacy', NULL, 1, 'all', '_self', 'active'
FROM menus m WHERE m.slug = 'auth_footer' AND m.tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
